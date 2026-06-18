package ssh

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/babbage88/goph/v2"
	sshconfig "github.com/kevinburke/ssh_config"
	skknownhosts "github.com/skeema/knownhosts"
	cryptossh "golang.org/x/crypto/ssh"
)

type RemoteAppDeploymentAgent struct {
	SshClient           *goph.Client      `json:"-"`
	SourceUtilsDir      string            `json:"srcUtilsDir"`
	DestinationUtilsDir string            `json:"dstUtilsDir"`
	EnvVars             map[string]string `json:"envVars"`
	RemoteCommand       *goph.Cmd         `json:"remoteCommands"`
}

type Runner interface {
	Run(cmd string) ([]byte, error)
}

type Uploader interface {
	Upload(srcPath, dstPath string) error
}

type Client interface {
	Runner
	Uploader
	Close() error
}

type PublicKeyOption struct {
	Path    string
	Content string
}

var ignoreHostKeyVerification bool

func SetIgnoreHostKeyVerification(ignore bool) {
	ignoreHostKeyVerification = ignore
}

func VerifyHost(host string, remote net.Addr, key cryptossh.PublicKey) error {
	return verifyKnownHost(host, host, remote, key)
}

func initializeSshClient(host string, user string, port uint, sshKeyPath string, sshPassphrase string, agent bool) (*goph.Client, error) {
	originalHost := host
	explicitKeyProvided := strings.TrimSpace(sshKeyPath) != ""
	host, user, port, sshKeyPath = resolveSSHConfig(host, user, port, sshKeyPath)
	authSource := determineSSHAuthSource(explicitKeyProvided, sshKeyPath, agent)
	slog.Info("SSH auth selection", "alias", originalHost, "hostname", host, "user", user, "port", port, "auth_source", authSource, "ssh_key", sshKeyPath, "ssh_agent_requested", agent, "ssh_agent_available", goph.HasAgent())

	auth, err := buildSSHAuthMethods(sshKeyPath, sshPassphrase, agent)
	if err != nil {
		return nil, err
	}

	callback := makeHostKeyCallback(originalHost, host)
	client, err := goph.NewConn(&goph.Config{
		User:     user,
		Addr:     host,
		Port:     port,
		Auth:     auth,
		Timeout:  goph.DefaultTimeout,
		Callback: callback,
	})
	if err != nil {
		return nil, err
	}
	return client, err
}

func makeHostKeyCallback(originalHost, resolvedHost string) func(string, net.Addr, cryptossh.PublicKey) error {
	if ignoreHostKeyVerification {
		slog.Warn("SSH host key verification disabled", "alias", originalHost, "hostname", resolvedHost)
		return cryptossh.InsecureIgnoreHostKey()
	}

	return func(_ string, remote net.Addr, key cryptossh.PublicKey) error {
		return verifyKnownHost(originalHost, resolvedHost, remote, key)
	}
}

func resolveSSHConfig(host, user string, port uint, sshKeyPath string) (string, string, uint, string) {
	if strings.TrimSpace(host) == "" {
		return host, user, port, sshKeyPath
	}

	allowConfigIdentityFile := strings.TrimSpace(sshKeyPath) == "" || isAutoDetectedDefaultSSHKeyPath(sshKeyPath)
	resolvedHost := strings.TrimSpace(sshconfig.Get(host, "HostName"))
	if resolvedHost == "" {
		resolvedHost = host
	}

	if strings.TrimSpace(user) == "" {
		if configUser := strings.TrimSpace(sshconfig.Get(host, "User")); configUser != "" {
			user = configUser
		}
	}

	if configPort := strings.TrimSpace(sshconfig.Get(host, "Port")); configPort != "" {
		if port == 0 || port == 22 {
			if parsedPort, err := strconv.ParseUint(configPort, 10, 16); err == nil {
				port = uint(parsedPort)
			}
		}
	}

	if allowConfigIdentityFile {
		for _, identityFile := range sshconfig.GetAll(host, "IdentityFile") {
			expanded := expandSSHConfigPath(identityFile)
			if expanded == "" {
				continue
			}
			if info, err := os.Stat(expanded); err == nil && !info.IsDir() {
				sshKeyPath = expanded
				break
			}
		}
	}

	if resolvedHost != host {
		slog.Info("Resolved SSH host via ~/.ssh/config", "alias", host, "hostname", resolvedHost, "user", user, "port", port, "ssh_key", sshKeyPath)
	}

	return resolvedHost, user, port, sshKeyPath
}

func isAutoDetectedDefaultSSHKeyPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	for _, candidate := range resolveSSHKeyPaths("") {
		if candidate == path {
			return true
		}
	}
	return false
}

func determineSSHAuthSource(explicitKeyProvided bool, sshKeyPath string, agentRequested bool) string {
	switch {
	case explicitKeyProvided && strings.TrimSpace(sshKeyPath) != "":
		return "explicit-key"
	case strings.TrimSpace(sshKeyPath) != "" && !isAutoDetectedDefaultSSHKeyPath(sshKeyPath):
		return "ssh-config-identityfile"
	case strings.TrimSpace(sshKeyPath) != "":
		if agentRequested || goph.HasAgent() {
			return "default-key+agent"
		}
		return "default-key"
	case agentRequested || goph.HasAgent():
		return "agent-only"
	default:
		return "none"
	}
}

func expandSSHConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") || path == "~" {
		homeDir, err := os.UserHomeDir()
		if err == nil && homeDir != "" {
			return filepath.Join(homeDir, strings.TrimPrefix(path, "~/"))
		}
	}
	return os.ExpandEnv(path)
}

func verifyKnownHost(originalHost, resolvedHost string, remote net.Addr, key cryptossh.PublicKey) error {
	hostsToTry := uniqueNonEmptyHosts(originalHost, resolvedHost)
	knownHostsFiles := resolveKnownHostsFiles(originalHost, resolvedHost)

	for _, host := range hostsToTry {
		matched, err := checkKnownHost(host, remote, key, knownHostsFiles)
		if err != nil {
			return err
		}
		if matched {
			return nil
		}
	}

	promptHost := originalHost
	if strings.TrimSpace(promptHost) == "" {
		promptHost = resolvedHost
	}
	if askIsHostTrusted(promptHost, key) == false {
		return errors.New("you typed no, aborted!")
	}
	if err := appendKnownHosts(hostsToTry, remote, key, knownHostsFiles); err != nil {
		return err
	}
	return nil
}

func checkKnownHost(host string, remote net.Addr, key cryptossh.PublicKey, knownHostsFiles []string) (bool, error) {
	existingFiles := existingKnownHostsFiles(knownHostsFiles)
	if len(existingFiles) == 0 {
		return false, nil
	}

	callback, err := skknownhosts.New(existingFiles...)
	if err != nil {
		return false, fmt.Errorf("load known_hosts files: %w", err)
	}

	for _, target := range knownHostTargets(host, remote) {
		err = callback(target, remote, key)
		if err == nil {
			return true, nil
		}
		if skknownhosts.IsHostUnknown(err) {
			continue
		}
		return false, err
	}
	return false, nil
}

func appendKnownHosts(hosts []string, remote net.Addr, key cryptossh.PublicKey, knownHostsFiles []string) error {
	targetFile := primaryKnownHostsFile(knownHostsFiles)
	if targetFile == "" {
		return fmt.Errorf("no known_hosts file available for writing")
	}
	if err := os.MkdirAll(filepath.Dir(targetFile), 0o700); err != nil {
		return fmt.Errorf("create known_hosts directory: %w", err)
	}
	file, err := os.OpenFile(targetFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open known_hosts file %q: %w", targetFile, err)
	}
	defer file.Close()

	for _, host := range hosts {
		if err := skknownhosts.WriteKnownHost(file, host, remote, key); err != nil {
			return fmt.Errorf("write host %q to known_hosts: %w", host, err)
		}
	}
	return nil
}

func resolveKnownHostsFiles(originalHost, resolvedHost string) []string {
	configHost := strings.TrimSpace(originalHost)
	if configHost == "" {
		configHost = strings.TrimSpace(resolvedHost)
	}

	var files []string
	if configHost != "" {
		for _, value := range sshconfig.GetAll(configHost, "UserKnownHostsFile") {
			for _, part := range splitSSHConfigValues(value) {
				expanded := expandSSHConfigPath(part)
				if expanded == "" || strings.EqualFold(expanded, "none") {
					continue
				}
				files = append(files, expanded)
			}
		}
	}
	if len(files) > 0 {
		return uniqueNonEmptyHosts(files...)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return nil
	}
	return []string{filepath.Join(homeDir, ".ssh", "known_hosts")}
}

func splitSSHConfigValues(value string) []string { return strings.Fields(value) }

func existingKnownHostsFiles(files []string) []string {
	existing := make([]string, 0, len(files))
	for _, file := range files {
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			existing = append(existing, file)
		}
	}
	return existing
}

func primaryKnownHostsFile(files []string) string {
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file != "" {
			return file
		}
	}
	return ""
}

func knownHostTargets(host string, remote net.Addr) []string {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	_, port, err := net.SplitHostPort(remote.String())
	if err != nil || port == "" {
		return []string{net.JoinHostPort(host, "22")}
	}
	return []string{net.JoinHostPort(host, port)}
}

func uniqueNonEmptyHosts(values ...string) []string {
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
	return result
}

func buildSSHAuthMethods(sshKeyPath string, sshPassphrase string, useAgent bool) (goph.Auth, error) {
	keyPaths := resolveSSHKeyPaths(sshKeyPath)
	authMethods := make(goph.Auth, 0, len(keyPaths)+1)

	for _, keyPath := range keyPaths {
		keyAuth, err := goph.Key(keyPath, sshPassphrase)
		if err != nil {
			return nil, fmt.Errorf("load SSH key %q: %w", keyPath, err)
		}
		authMethods = append(authMethods, keyAuth...)
	}

	shouldUseAgent := useAgent || goph.HasAgent()
	if shouldUseAgent {
		agentAuth, err := goph.UseAgent()
		if err != nil {
			if useAgent && len(keyPaths) == 0 {
				return nil, fmt.Errorf("use ssh agent: %w", err)
			}
			if useAgent {
				slog.Warn("SSH agent unavailable, continuing with SSH key authentication", "error", err.Error())
			}
		} else {
			authMethods = append(authMethods, agentAuth...)
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH authentication method configured; provide --ssh-key or enable ssh-agent")
	}
	return authMethods, nil
}

func resolveSSHKeyPaths(explicitPath string) []string {
	if explicitPath != "" {
		return []string{explicitPath}
	}
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return nil
	}
	var keyPaths []string
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		candidate := filepath.Join(homeDir, ".ssh", name)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			keyPaths = append(keyPaths, candidate)
		}
	}
	return keyPaths
}

func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

func CurrentUserName() string {
	if username := os.Getenv("USER"); username != "" {
		return username
	}
	curUser, err := user.Current()
	if err == nil && curUser.Username != "" {
		return curUser.Username
	}
	return "root"
}

func DefaultPrivateKeyPath() string {
	for _, candidate := range []string{"~/.ssh/id_ed25519", "~/.ssh/id_rsa"} {
		expanded := ExpandPath(candidate)
		if info, err := os.Stat(expanded); err == nil && !info.IsDir() {
			return expanded
		}
	}
	return ""
}

func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func WithDefaultTERM(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return `TERM="${TERM:-dumb}"`
	}
	return `TERM="${TERM:-dumb}" ` + command
}

func FormatExecError(err error, out []byte) error {
	output := strings.TrimSpace(string(out))
	if output == "" {
		return fmt.Errorf("SSH execution failed: %w", err)
	}
	return fmt.Errorf("SSH execution failed: %w: %s", err, output)
}

func DiscoverPublicKeyContents(explicitPrivateKeyPath string) []string {
	options := DiscoverPublicKeyOptions(explicitPrivateKeyPath)
	keys := make([]string, 0, len(options))
	for _, option := range options {
		keys = append(keys, option.Content)
	}
	return keys
}

func DiscoverPublicKeyOptions(explicitPrivateKeyPath string) []PublicKeyOption {
	candidates := make([]string, 0, 16)
	if explicitPrivateKeyPath = strings.TrimSpace(explicitPrivateKeyPath); explicitPrivateKeyPath != "" {
		candidates = append(candidates, ExpandPath(explicitPrivateKeyPath)+".pub")
	}

	sshDir := ExpandPath("~/.ssh")
	if entries, err := os.ReadDir(sshDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pub") {
				continue
			}
			candidates = append(candidates, filepath.Join(sshDir, entry.Name()))
		}
	}

	options := make([]PublicKeyOption, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		content, err := os.ReadFile(candidate)
		if err != nil {
			continue
		}
		key := strings.TrimSpace(string(content))
		if key == "" {
			continue
		}
		options = append(options, PublicKeyOption{Path: candidate, Content: key})
	}
	return options
}

func InitializeSshClient(hostname, username, sshKey, sshPassphrase string, useAgent bool, port uint) (*goph.Client, error) {
	client, err := initializeSshClient(hostname, username, port, sshKey, sshPassphrase, useAgent)
	if err != nil {
		return client, SshErrorWrapper(500, err, "Error initializing ssh client")
	}
	slog.Info("ssh client initalized successfully")
	return client, nil
}

func RunCommandAndCaptureOutput(c *goph.Client, remoteCmd string, args []string) ([]byte, error) {
	cmd, err := c.Command(remoteCmd, args...)
	if err != nil {
		return nil, err
	}
	combinedOutput, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("error attempting command", "error", err.Error())
		return nil, err
	}
	return combinedOutput, err
}
