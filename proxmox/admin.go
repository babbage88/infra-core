package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	coredeploy "github.com/babbage88/infra-core/deployment"
	coressh "github.com/babbage88/infra-core/ssh"
	"github.com/google/uuid"
)

const DefaultManagerRoleName = "InfraCtlProxmoxManager"

type CreatePVEUserRequest struct {
	SSH          coredeploy.SSHOptions
	HostServerID *uuid.UUID
	Node         string
	Username     string
	Realm        string
	Comment      string
	Password     string
	Force        bool
}

type CreatePVEUserResult struct {
	Host      string
	UserID    string
	Username  string
	Realm     string
	Created   bool
	Recreated bool
}

type CreateAPITokenRequest struct {
	SSH               coredeploy.SSHOptions
	HostServerID      *uuid.UUID
	Node              string
	HostURL           string
	UserID            string
	Username          string
	Realm             string
	TokenID           string
	Comment           string
	Role              string
	ACLPath           string
	ExpirationDate    string
	DaysValid         int
	Privsep           bool
	Force             bool
	Yolo              bool
	Verify            *bool
	StoreAsUserSecret *bool
}

type CreateAPITokenResult struct {
	Host                string
	Node                string
	HostURL             string
	UserID              string
	TokenID             string
	FullTokenID         string
	Secret              string
	APIToken            string
	Role                string
	ACLPath             string
	ExpiresAtUnix       int64
	Privsep             bool
	Yolo                bool
	AssignedRoles       []string
	AssignedPrivileges  []string
	DirectChecks        []string
	InferredChecks      []string
	MissingCapabilities []string
	StoredSecretID      *uuid.UUID
}

type createdToken struct {
	FullTokenID string
	Secret      string
}

type roleInfo struct {
	RoleID string
	Privs  []string
}

type aclInfo struct {
	Path      string
	Principal string
	RoleID    string
	Propagate bool
}

type tokenVerification struct {
	AssignedRoles       []string
	AssignedPrivileges  []string
	DirectChecks        []string
	InferredChecks      []string
	MissingCapabilities []string
}

func CreatePVEUser(req CreatePVEUserRequest) (CreatePVEUserResult, error) {
	sshOpts, cleanup, err := prepareAdminSSH(req.SSH, req.Node)
	if err != nil {
		return CreatePVEUserResult{}, err
	}
	defer cleanup()

	if strings.TrimSpace(req.Username) == "" {
		return CreatePVEUserResult{}, fmt.Errorf("username is required")
	}
	if strings.TrimSpace(req.Realm) == "" {
		req.Realm = "pve"
	}
	userID := fmt.Sprintf("%s@%s", strings.TrimSpace(req.Username), strings.TrimSpace(req.Realm))

	sshClient, err := coredeploy.InitializeSshClient(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return CreatePVEUserResult{}, fmt.Errorf("initialize SSH client: %w", err)
	}
	defer sshClient.Close()

	exists, err := proxmoxUserExistsOverSSH(sshClient, userID)
	if err != nil {
		return CreatePVEUserResult{}, fmt.Errorf("check whether proxmox user %s exists: %w", userID, err)
	}

	recreated := false
	if exists {
		if !req.Force {
			return CreatePVEUserResult{}, fmt.Errorf("proxmox user %s already exists", userID)
		}
		if err := deleteProxmoxUserOverSSH(sshClient, userID); err != nil {
			return CreatePVEUserResult{}, fmt.Errorf("delete existing proxmox user %s: %w", userID, err)
		}
		recreated = true
	}

	args := []string{"pveum", "user", "add", userID}
	if strings.TrimSpace(req.Comment) != "" {
		args = append(args, "--comment", req.Comment)
	}
	if strings.TrimSpace(req.Password) != "" {
		args = append(args, "--password", req.Password)
	}
	if _, err := runRemoteQuotedCommand(sshClient, args...); err != nil {
		return CreatePVEUserResult{}, err
	}

	return CreatePVEUserResult{
		Host:      sshOpts.Host,
		UserID:    userID,
		Username:  strings.TrimSpace(req.Username),
		Realm:     strings.TrimSpace(req.Realm),
		Created:   true,
		Recreated: recreated,
	}, nil
}

func CreateAPIToken(req CreateAPITokenRequest) (CreateAPITokenResult, error) {
	sshOpts, cleanup, err := prepareAdminSSH(req.SSH, req.Node)
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	defer cleanup()

	req, expireUnix, err := normalizeTokenRequest(req, sshOpts.Host)
	if err != nil {
		return CreateAPITokenResult{}, err
	}

	sshClient, err := coredeploy.InitializeSshClient(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return CreateAPITokenResult{}, fmt.Errorf("initialize SSH client: %w", err)
	}
	defer sshClient.Close()

	exists, err := proxmoxTokenExistsOverSSH(sshClient, req.UserID, req.TokenID)
	if err != nil {
		return CreateAPITokenResult{}, fmt.Errorf("check whether API token %s!%s exists: %w", req.UserID, req.TokenID, err)
	}
	if exists {
		if !req.Force {
			return CreateAPITokenResult{}, fmt.Errorf("proxmox API token %s!%s already exists", req.UserID, req.TokenID)
		}
		if err := deleteProxmoxAPITokenOverSSH(sshClient, req.UserID, req.TokenID); err != nil {
			return CreateAPITokenResult{}, fmt.Errorf("delete existing API token %s!%s: %w", req.UserID, req.TokenID, err)
		}
	}

	userRoleName, tokenRoleName, err := ensureInfractlRolesForTokenOverSSH(sshClient, req)
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	if userRoleName != "" {
		if err := assignRoleToProxmoxPrincipalOverSSH(sshClient, req.ACLPath, "user", req.UserID, userRoleName); err != nil {
			return CreateAPITokenResult{}, fmt.Errorf("apply ACL for user %s on %s: %w", req.UserID, req.ACLPath, err)
		}
	}

	privsepValue := "0"
	if req.Privsep {
		privsepValue = "1"
	}
	tokenArgs := []string{
		"pveum", "user", "token", "add", req.UserID, req.TokenID,
		"--privsep", privsepValue,
		"--output-format", "json",
	}
	if strings.TrimSpace(req.Comment) != "" {
		tokenArgs = append(tokenArgs, "--comment", req.Comment)
	}
	if expireUnix > 0 {
		tokenArgs = append(tokenArgs, "--expire", strconv.FormatInt(expireUnix, 10))
	}
	out, err := runRemoteQuotedCommand(sshClient, tokenArgs...)
	if err != nil {
		return CreateAPITokenResult{}, fmt.Errorf("create API token %s for %s: %w", req.TokenID, req.UserID, err)
	}

	created, err := parseCreatedProxmoxToken(req.UserID, req.TokenID, out)
	if err != nil {
		return CreateAPITokenResult{}, err
	}
	if tokenRoleName != "" {
		if err := assignRoleToProxmoxPrincipalOverSSH(sshClient, req.ACLPath, "token", created.FullTokenID, tokenRoleName); err != nil {
			return CreateAPITokenResult{}, fmt.Errorf("apply ACL for token %s on %s: %w", created.FullTokenID, req.ACLPath, err)
		}
	}

	result := CreateAPITokenResult{
		Host:          sshOpts.Host,
		Node:          req.Node,
		HostURL:       req.HostURL,
		UserID:        req.UserID,
		TokenID:       req.TokenID,
		FullTokenID:   created.FullTokenID,
		Secret:        created.Secret,
		APIToken:      created.FullTokenID + "=" + created.Secret,
		Role:          firstNonEmptyString(tokenRoleName, userRoleName, req.Role),
		ACLPath:       req.ACLPath,
		ExpiresAtUnix: expireUnix,
		Privsep:       req.Privsep,
		Yolo:          req.Yolo,
	}

	verify := true
	if req.Verify != nil {
		verify = *req.Verify
	}
	if !verify {
		return result, nil
	}

	verification, err := verifyTokenCoversInfraCtlCommands(sshClient, req, created)
	if err != nil {
		return CreateAPITokenResult{}, fmt.Errorf("post-create token verification failed: %w", err)
	}
	result.AssignedRoles = verification.AssignedRoles
	result.AssignedPrivileges = verification.AssignedPrivileges
	result.DirectChecks = verification.DirectChecks
	result.InferredChecks = verification.InferredChecks
	result.MissingCapabilities = verification.MissingCapabilities
	return result, nil
}

func prepareAdminSSH(opts coredeploy.SSHOptions, fallbackHost string) (coredeploy.SSHOptions, func(), error) {
	if strings.TrimSpace(opts.Host) == "" {
		opts.Host = strings.TrimSpace(fallbackHost)
	}
	if strings.TrimSpace(opts.Host) == "" {
		return opts, func() {}, fmt.Errorf("ssh.host is required")
	}
	if strings.TrimSpace(opts.User) == "" {
		opts.User = "root"
	}
	if opts.Port == 0 {
		opts.Port = 22
	}
	return coredeploy.PrepareSSHOptions(opts)
}

func normalizeTokenRequest(req CreateAPITokenRequest, sshHost string) (CreateAPITokenRequest, int64, error) {
	if strings.TrimSpace(req.Node) == "" {
		req.Node = strings.TrimSpace(sshHost)
	}
	if strings.TrimSpace(req.HostURL) == "" {
		req.HostURL = defaultHostURL(req.Node)
	}
	if req.Yolo {
		req.UserID = "root@pam"
		req.Username = "root"
		req.Realm = "pam"
		req.ACLPath = "/"
		req.Privsep = false
		req.Role = ""
	}
	if strings.TrimSpace(req.UserID) == "" {
		if strings.TrimSpace(req.Username) == "" {
			return req, 0, fmt.Errorf("username or userid is required")
		}
		if strings.TrimSpace(req.Realm) == "" {
			req.Realm = "pve"
		}
		req.UserID = fmt.Sprintf("%s@%s", strings.TrimSpace(req.Username), strings.TrimSpace(req.Realm))
	}
	if strings.TrimSpace(req.TokenID) == "" {
		return req, 0, fmt.Errorf("token_id is required")
	}
	if strings.TrimSpace(req.ACLPath) == "" {
		req.ACLPath = "/"
	}
	if strings.TrimSpace(req.Role) == "" && !req.Yolo {
		req.Role = DefaultManagerRoleName
	}
	expireUnix, err := resolveExpiration(req.ExpirationDate, req.DaysValid)
	if err != nil {
		return req, 0, err
	}
	return req, expireUnix, nil
}

func resolveExpiration(expirationDate string, daysValid int) (int64, error) {
	expirationDate = strings.TrimSpace(expirationDate)
	if expirationDate != "" && daysValid > 0 {
		return 0, fmt.Errorf("use either expiration_date or days_valid, not both")
	}
	if daysValid < 0 {
		return 0, fmt.Errorf("days_valid must be zero or greater")
	}
	if daysValid > 0 {
		return time.Now().AddDate(0, 0, daysValid).Unix(), nil
	}
	if expirationDate == "" {
		return 0, nil
	}
	if parsed, err := time.Parse(time.RFC3339, expirationDate); err == nil {
		return parsed.Unix(), nil
	}
	parsedDate, err := time.ParseInLocation("2006-01-02", expirationDate, time.Local)
	if err != nil {
		return 0, fmt.Errorf("parse expiration_date %q: use YYYY-MM-DD or RFC3339", expirationDate)
	}
	return parsedDate.AddDate(0, 0, 1).Add(-time.Second).Unix(), nil
}

func ensureInfractlRolesForTokenOverSSH(sshClient coressh.Client, cfg CreateAPITokenRequest) (string, string, error) {
	if cfg.Yolo {
		return "", "", nil
	}
	if strings.TrimSpace(cfg.Role) != "" && cfg.Role != DefaultManagerRoleName {
		if cfg.Privsep {
			return "", cfg.Role, nil
		}
		return cfg.Role, "", nil
	}
	managerPrivs := infractlManagerPrivileges()
	if err := ensureProxmoxRoleOverSSH(sshClient, DefaultManagerRoleName, managerPrivs); err != nil {
		return "", "", fmt.Errorf("ensure manager role %s: %w", DefaultManagerRoleName, err)
	}
	if cfg.Privsep {
		return "", DefaultManagerRoleName, nil
	}
	return DefaultManagerRoleName, "", nil
}

func infractlManagerPrivileges() []string {
	return []string{
		"Datastore.AllocateSpace",
		"Datastore.Audit",
		"Pool.Allocate",
		"SDN.Use",
		"VM.Allocate",
		"VM.Audit",
		"VM.Clone",
		"VM.Console",
		"VM.Config.CDROM",
		"VM.Config.CPU",
		"VM.Config.Cloudinit",
		"VM.Config.Disk",
		"VM.Config.HWType",
		"VM.Config.Memory",
		"VM.Config.Network",
		"VM.Config.Options",
		"VM.Migrate",
		"VM.PowerMgmt",
	}
}

func ensureProxmoxRoleOverSSH(sshClient coressh.Client, roleName string, privs []string) error {
	privs = dedupeAndSortStrings(privs)
	privString := strings.Join(privs, " ")
	if _, err := runRemoteQuotedCommand(sshClient, "pveum", "role", "modify", roleName, "--privs", privString); err == nil {
		return nil
	}
	_, err := runRemoteQuotedCommand(sshClient, "pveum", "role", "add", roleName, "--privs", privString)
	return err
}

func assignRoleToProxmoxPrincipalOverSSH(sshClient coressh.Client, aclPath, principalKind, principalID, roleName string) error {
	args := []string{"pveum", "aclmod", aclPath, "-role", roleName}
	switch principalKind {
	case "token":
		args = append(args, "-token", principalID)
	default:
		args = append(args, "-user", principalID)
	}
	_, err := runRemoteQuotedCommand(sshClient, args...)
	return err
}

func proxmoxUserExistsOverSSH(sshClient coressh.Client, userID string) (bool, error) {
	out, err := runRemoteQuotedCommand(sshClient, "pveum", "user", "list", "--output-format", "json")
	if err != nil {
		return false, err
	}
	var payload []map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		return false, fmt.Errorf("parse proxmox user list JSON: %w", err)
	}
	for _, item := range payload {
		if strings.TrimSpace(fmt.Sprintf("%v", item["userid"])) == userID {
			return true, nil
		}
	}
	return false, nil
}

func proxmoxTokenExistsOverSSH(sshClient coressh.Client, userID, tokenID string) (bool, error) {
	out, err := runRemoteQuotedCommand(sshClient, "pveum", "user", "token", "list", userID, "--output-format", "json")
	if err != nil {
		return proxmoxTokenExistsFallbackOverSSH(sshClient, userID, tokenID)
	}
	var payload []map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		return proxmoxTokenExistsFallbackOverSSH(sshClient, userID, tokenID)
	}
	for _, item := range payload {
		if strings.TrimSpace(fmt.Sprintf("%v", item["tokenid"])) == tokenID {
			return true, nil
		}
	}
	return false, nil
}

func proxmoxTokenExistsFallbackOverSSH(sshClient coressh.Client, userID, tokenID string) (bool, error) {
	out, err := runRemoteQuotedCommand(sshClient, "pveum", "user", "token", "list", userID)
	if err == nil {
		return proxmoxTokenListContainsToken(string(out), tokenID), nil
	}
	out, err = runRemoteQuotedCommand(sshClient, "pveum", "user", "token", "permissions", userID+"!"+tokenID)
	if err == nil {
		return true, nil
	}
	rawErr := strings.ToLower(err.Error())
	if strings.Contains(rawErr, "not exist") || strings.Contains(rawErr, "does not exist") || strings.Contains(rawErr, "no such") {
		return false, nil
	}
	return false, fmt.Errorf("fallback token existence checks failed: %w", err)
}

func proxmoxTokenListContainsToken(output, tokenID string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) > 0 && fields[0] == tokenID {
			return true
		}
	}
	return false
}

func deleteProxmoxUserOverSSH(sshClient coressh.Client, userID string) error {
	_, err := runRemoteQuotedCommand(sshClient, "pveum", "user", "delete", userID)
	return err
}

func deleteProxmoxAPITokenOverSSH(sshClient coressh.Client, userID, tokenID string) error {
	_, err := runRemoteQuotedCommand(sshClient, "pveum", "user", "token", "delete", userID, tokenID)
	return err
}

func runRemoteQuotedCommand(sshClient coressh.Runner, args ...string) ([]byte, error) {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, coressh.ShellQuote(arg))
	}
	command := coressh.WithDefaultTERM(strings.Join(quoted, " "))
	out, err := sshClient.Run(command)
	if err != nil {
		return nil, coressh.FormatExecError(err, out)
	}
	return out, nil
}

func parseCreatedProxmoxToken(userID, tokenID string, out []byte) (createdToken, error) {
	fullTokenID := fmt.Sprintf("%s!%s", userID, tokenID)
	trimmed := strings.TrimSpace(string(out))

	var payload map[string]any
	if err := json.Unmarshal(out, &payload); err == nil {
		if value := strings.TrimSpace(fmt.Sprintf("%v", payload["value"])); value != "" && value != "<nil>" {
			if full := strings.TrimSpace(fmt.Sprintf("%v", payload["full-tokenid"])); full != "" && full != "<nil>" {
				fullTokenID = full
			}
			return createdToken{FullTokenID: fullTokenID, Secret: value}, nil
		}
	}
	if jsonPayload := extractJSONObjectFromOutput(trimmed); jsonPayload != "" {
		if err := json.Unmarshal([]byte(jsonPayload), &payload); err == nil {
			if value := strings.TrimSpace(fmt.Sprintf("%v", payload["value"])); value != "" && value != "<nil>" {
				if full := strings.TrimSpace(fmt.Sprintf("%v", payload["full-tokenid"])); full != "" && full != "<nil>" {
					fullTokenID = full
				}
				return createdToken{FullTokenID: fullTokenID, Secret: value}, nil
			}
		}
	}
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err := json.Unmarshal([]byte(line), &payload); err == nil {
			if value := strings.TrimSpace(fmt.Sprintf("%v", payload["value"])); value != "" && value != "<nil>" {
				if full := strings.TrimSpace(fmt.Sprintf("%v", payload["full-tokenid"])); full != "" && full != "<nil>" {
					fullTokenID = full
				}
				return createdToken{FullTokenID: fullTokenID, Secret: value}, nil
			}
		}
		if strings.HasPrefix(strings.ToLower(line), "value") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
				return createdToken{FullTokenID: fullTokenID, Secret: strings.TrimSpace(parts[1])}, nil
			}
		}
	}
	return createdToken{}, fmt.Errorf("created token but could not parse the token secret from output: %s", trimmed)
}

func extractJSONObjectFromOutput(output string) string {
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return strings.TrimSpace(output[start : end+1])
}

func listProxmoxRolesOverSSH(sshClient coressh.Client) ([]roleInfo, error) {
	out, err := runRemoteQuotedCommand(sshClient, "pveum", "role", "list", "--output-format", "json")
	if err != nil {
		return listProxmoxRolesFallbackOverSSH(sshClient)
	}
	var payload []map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		return listProxmoxRolesFallbackOverSSH(sshClient)
	}
	roles := make([]roleInfo, 0, len(payload))
	for _, item := range payload {
		roleID := strings.TrimSpace(fmt.Sprintf("%v", item["roleid"]))
		if roleID == "" {
			continue
		}
		privs := splitPrivilegeString(fmt.Sprintf("%v", item["privs"]))
		roles = append(roles, roleInfo{RoleID: roleID, Privs: privs})
	}
	return roles, nil
}

func listProxmoxACLsOverSSH(sshClient coressh.Client) ([]aclInfo, error) {
	out, err := runRemoteQuotedCommand(sshClient, "pveum", "acl", "list", "--output-format", "json")
	if err != nil {
		return listProxmoxACLsFallbackOverSSH(sshClient)
	}
	var payload []map[string]any
	if err := json.Unmarshal(out, &payload); err != nil {
		return listProxmoxACLsFallbackOverSSH(sshClient)
	}
	acls := make([]aclInfo, 0, len(payload))
	for _, item := range payload {
		principal := strings.TrimSpace(fmt.Sprintf("%v", item["ugid"]))
		if principal == "" {
			continue
		}
		roleID := strings.TrimSpace(fmt.Sprintf("%v", item["roleid"]))
		path := strings.TrimSpace(fmt.Sprintf("%v", item["path"]))
		propagate := strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", item["propagate"])), "1") ||
			strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", item["propagate"])), "true")
		acls = append(acls, aclInfo{Path: path, Principal: principal, RoleID: roleID, Propagate: propagate})
	}
	return acls, nil
}

func listProxmoxRolesFallbackOverSSH(sshClient coressh.Client) ([]roleInfo, error) {
	out, err := runRemoteQuotedCommand(sshClient, "pveum", "role", "list")
	if err != nil {
		return nil, fmt.Errorf("list proxmox roles via plain-text fallback: %w", err)
	}
	roles := make([]roleInfo, 0)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "ROLEID") || strings.HasPrefix(upper, "USAGE:") || strings.HasPrefix(strings.ToLower(line), "user config -") || strings.HasPrefix(line, "400 ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		roleID := strings.TrimSpace(fields[0])
		if roleID == "" {
			continue
		}
		privs := splitPrivilegeString(strings.Join(fields[1:], " "))
		roles = append(roles, roleInfo{RoleID: roleID, Privs: privs})
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("parse proxmox role list fallback output: no roles found")
	}
	return roles, nil
}

func listProxmoxACLsFallbackOverSSH(sshClient coressh.Client) ([]aclInfo, error) {
	out, err := runRemoteQuotedCommand(sshClient, "pveum", "acl", "list")
	if err != nil {
		return nil, fmt.Errorf("list proxmox ACLs via plain-text fallback: %w", err)
	}
	acls := make([]aclInfo, 0)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "PATH") || strings.HasPrefix(upper, "ACLPATH") || strings.HasPrefix(upper, "USAGE:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		propagate := false
		lastField := strings.ToLower(fields[len(fields)-1])
		if lastField == "0" || lastField == "1" || lastField == "true" || lastField == "false" {
			propagate = lastField == "1" || lastField == "true"
			fields = fields[:len(fields)-1]
		}
		if len(fields) < 3 {
			continue
		}
		acls = append(acls, aclInfo{
			Path:      strings.TrimSpace(fields[0]),
			Principal: strings.TrimSpace(fields[1]),
			RoleID:    strings.TrimSpace(fields[2]),
			Propagate: propagate,
		})
	}
	if len(acls) == 0 {
		return nil, fmt.Errorf("parse proxmox ACL list fallback output: no ACLs found")
	}
	return acls, nil
}

func inspectProxmoxAuthOverSSH(sshClient coressh.Client, userID, tokenFullID string) ([]string, []string, error) {
	roleInfos, err := listProxmoxRolesOverSSH(sshClient)
	if err != nil {
		return nil, nil, err
	}
	acls, err := listProxmoxACLsOverSSH(sshClient)
	if err != nil {
		return nil, nil, err
	}
	roleMap := make(map[string][]string, len(roleInfos))
	for _, role := range roleInfos {
		roleMap[role.RoleID] = role.Privs
	}
	roleSet := make(map[string]struct{})
	privSet := make(map[string]struct{})
	for _, acl := range acls {
		if acl.Principal != userID && acl.Principal != tokenFullID {
			continue
		}
		roleSet[acl.RoleID] = struct{}{}
		for _, priv := range roleMap[acl.RoleID] {
			privSet[priv] = struct{}{}
		}
	}
	return mapKeysSorted(roleSet), mapKeysSorted(privSet), nil
}

func verifyTokenCoversInfraCtlCommands(sshClient coressh.Client, cfg CreateAPITokenRequest, created createdToken) (*tokenVerification, error) {
	assignedRoles, assignedPrivs, err := inspectProxmoxAuthOverSSH(sshClient, cfg.UserID, created.FullTokenID)
	if err != nil {
		return nil, fmt.Errorf("inspect assigned ACLs and roles: %w", err)
	}

	tokenID, secret, err := ParseAPIToken(fmt.Sprintf("%s=%s", created.FullTokenID, created.Secret))
	if err != nil {
		return nil, fmt.Errorf("parse created token for verification: %w", err)
	}
	client, err := NewClientToken(cfg.HostURL, tokenID, secret, true)
	if err != nil {
		return nil, fmt.Errorf("create proxmox client for verification: %w", err)
	}

	verification := &tokenVerification{
		AssignedRoles:      assignedRoles,
		AssignedPrivileges: assignedPrivs,
	}
	if strings.TrimSpace(cfg.ACLPath) != "" && strings.TrimSpace(cfg.ACLPath) != "/" {
		verification.MissingCapabilities = append(verification.MissingCapabilities,
			fmt.Sprintf("ACL path %s is narrower than /, so verification does not imply cluster-wide access for every infractl proxmox subcommand", cfg.ACLPath))
	}

	ctx := context.Background()
	if _, err := client.ListVMs(ctx, cfg.Node, false); err != nil {
		verification.MissingCapabilities = append(verification.MissingCapabilities, fmt.Sprintf("proxmox vm list direct API smoke test failed: %v", err))
	} else {
		verification.DirectChecks = append(verification.DirectChecks, "proxmox vm list")
	}
	if storages, err := client.ListNodeStorage(ctx, cfg.Node); err != nil {
		verification.MissingCapabilities = append(verification.MissingCapabilities, fmt.Sprintf("proxmox lxc create storage lookup failed: %v", err))
	} else {
		verification.DirectChecks = append(verification.DirectChecks, fmt.Sprintf("proxmox lxc create storage lookup (%d storages)", len(storages)))
	}
	if templates, err := client.ListLxcTemplates(ctx, cfg.Node); err != nil {
		verification.MissingCapabilities = append(verification.MissingCapabilities, fmt.Sprintf("proxmox lxc create template lookup failed: %v", err))
	} else {
		verification.DirectChecks = append(verification.DirectChecks, fmt.Sprintf("proxmox lxc create template lookup (%d templates)", len(templates)))
	}
	if cfg.Yolo && !cfg.Privsep && cfg.UserID == "root@pam" {
		verification.InferredChecks = append(verification.InferredChecks,
			"proxmox vm get (inherited from root@pam via non-privsep token)",
			"proxmox vm start (inherited from root@pam via non-privsep token)",
			"proxmox vm set (inherited from root@pam via non-privsep token)",
			"proxmox vm create (inherited from root@pam via non-privsep token)",
			"proxmox lxc create (inherited from root@pam via non-privsep token)",
			"proxmox lxc batch (inherited from root@pam via non-privsep token)",
		)
		return verification, nil
	}

	verifyCapability := func(command string, required []string) {
		missing := missingPrivileges(assignedPrivs, required)
		if len(missing) == 0 {
			verification.InferredChecks = append(verification.InferredChecks, fmt.Sprintf("%s (via privileges)", command))
			return
		}
		verification.MissingCapabilities = append(verification.MissingCapabilities, fmt.Sprintf("%s missing privileges: %s", command, strings.Join(missing, ", ")))
	}
	verifyCapability("proxmox vm get", []string{"VM.Audit"})
	verifyCapability("proxmox vm start", []string{"VM.PowerMgmt"})
	verifyCapability("proxmox vm set", []string{"VM.Config.CPU", "VM.Config.Memory", "VM.Config.Options"})
	verifyCapability("proxmox vm create", []string{"VM.Allocate", "VM.Config.CPU", "VM.Config.Memory", "VM.Config.Options"})
	verifyCapability("proxmox lxc create", []string{"VM.Allocate", "VM.Config.CPU", "VM.Config.Memory", "VM.Config.Network", "VM.Config.Options", "Datastore.Audit", "Datastore.AllocateSpace"})
	verifyCapability("proxmox lxc batch", []string{"VM.Allocate", "VM.Config.CPU", "VM.Config.Memory", "VM.Config.Network", "VM.Config.Options", "Datastore.Audit", "Datastore.AllocateSpace"})
	return verification, nil
}

func splitPrivilegeString(value string) []string {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", " ")
	fields := strings.Fields(value)
	filtered := make([]string, 0, len(fields))
	for _, field := range fields {
		if isLikelyProxmoxPrivilege(field) {
			filtered = append(filtered, field)
		}
	}
	return dedupeAndSortStrings(filtered)
}

func isLikelyProxmoxPrivilege(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !strings.Contains(value, ".") {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' {
			continue
		}
		return false
	}
	return true
}

func dedupeAndSortStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func mapKeysSorted[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func missingPrivileges(have []string, required []string) []string {
	haveSet := make(map[string]struct{}, len(have))
	for _, value := range have {
		haveSet[value] = struct{}{}
	}
	missing := make([]string, 0)
	for _, value := range required {
		if _, ok := haveSet[value]; ok {
			continue
		}
		missing = append(missing, value)
	}
	return dedupeAndSortStrings(missing)
}

func defaultHostURL(node string) string {
	node = strings.TrimSpace(node)
	if node == "" {
		return ""
	}
	return fmt.Sprintf("https://%s:8006", node)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
