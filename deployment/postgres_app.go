package deployment

import (
	"fmt"
	"net/url"
	"strings"
)

func DefaultPostgresAppSetupRequest() PostgresAppSetupRequest {
	return PostgresAppSetupRequest{
		SchemaName:                    "public",
		CreateDB:                      BoolPtr(true),
		DropFirst:                     BoolPtr(false),
		PostgresUser:                  "postgres",
		PostgresPort:                  5432,
		PostgresConnDB:                "postgres",
		SetupRemotePostgres:           BoolPtr(false),
		RemotePostgresHBACIDR:         "0.0.0.0/0",
		RemotePostgresAuthMethod:      "scram-sha-256",
		RemotePostgresListenAddresses: "*",
	}
}

func MergePostgresAppSetupDefaults(req, defaults PostgresAppSetupRequest) PostgresAppSetupRequest {
	if strings.TrimSpace(req.DatabaseName) == "" {
		req.DatabaseName = defaults.DatabaseName
	}
	if strings.TrimSpace(req.Username) == "" {
		req.Username = defaults.Username
	}
	if strings.TrimSpace(req.Password) == "" {
		req.Password = defaults.Password
	}
	if strings.TrimSpace(req.SchemaName) == "" {
		req.SchemaName = defaults.SchemaName
	}
	if req.CreateDB == nil {
		req.CreateDB = defaults.CreateDB
	}
	if req.DropFirst == nil {
		req.DropFirst = defaults.DropFirst
	}
	if strings.TrimSpace(req.PostgresUser) == "" {
		req.PostgresUser = defaults.PostgresUser
	}
	if strings.TrimSpace(req.PostgresPassword) == "" {
		req.PostgresPassword = defaults.PostgresPassword
	}
	if strings.TrimSpace(req.PostgresHost) == "" {
		req.PostgresHost = defaults.PostgresHost
	}
	if req.PostgresPort == 0 {
		req.PostgresPort = defaults.PostgresPort
	}
	if strings.TrimSpace(req.PostgresConnDB) == "" {
		req.PostgresConnDB = defaults.PostgresConnDB
	}
	if req.SetupRemotePostgres == nil {
		req.SetupRemotePostgres = defaults.SetupRemotePostgres
	}
	if strings.TrimSpace(req.RemotePostgresHBACIDR) == "" {
		req.RemotePostgresHBACIDR = defaults.RemotePostgresHBACIDR
	}
	if strings.TrimSpace(req.RemotePostgresAuthMethod) == "" {
		req.RemotePostgresAuthMethod = defaults.RemotePostgresAuthMethod
	}
	if strings.TrimSpace(req.RemotePostgresListenAddresses) == "" {
		req.RemotePostgresListenAddresses = defaults.RemotePostgresListenAddresses
	}
	req.SSH = MergeSSHDefaults(req.SSH, defaults.SSH)
	return req
}

func SetupPostgresApp(req PostgresAppSetupRequest) (PostgresAppSetupResult, error) {
	if strings.TrimSpace(req.SSH.Host) == "" {
		return PostgresAppSetupResult{}, fmt.Errorf("ssh.host is required")
	}
	if strings.TrimSpace(req.SSH.User) == "" {
		return PostgresAppSetupResult{}, fmt.Errorf("ssh.user is required")
	}
	if strings.TrimSpace(req.DatabaseName) == "" {
		return PostgresAppSetupResult{}, fmt.Errorf("db_name is required")
	}
	if strings.TrimSpace(req.Username) == "" {
		return PostgresAppSetupResult{}, fmt.Errorf("username is required")
	}
	if strings.TrimSpace(req.Password) == "" {
		return PostgresAppSetupResult{}, fmt.Errorf("password is required")
	}
	if strings.TrimSpace(req.PostgresUser) == "" {
		return PostgresAppSetupResult{}, fmt.Errorf("postgres_user is required")
	}
	if req.PostgresPort <= 0 {
		return PostgresAppSetupResult{}, fmt.Errorf("postgres_port must be greater than zero")
	}
	if strings.TrimSpace(req.SchemaName) == "" {
		req.SchemaName = "public"
	}
	if strings.TrimSpace(req.PostgresHost) == "" {
		req.PostgresHost = req.SSH.Host
	}

	sshOpts, cleanupSSHKey, err := PrepareSSHOptions(req.SSH)
	if err != nil {
		return PostgresAppSetupResult{}, err
	}
	defer cleanupSSHKey()

	sshClient, err := InitializeSshClient(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return PostgresAppSetupResult{}, fmt.Errorf("initialize SSH client: %w", err)
	}
	defer sshClient.Close()

	if boolValue(req.SetupRemotePostgres) {
		if err := ConfigureRemotePostgresAccess(sshClient, req.RemotePostgresHBACIDR, req.RemotePostgresAuthMethod, req.RemotePostgresListenAddresses); err != nil {
			return PostgresAppSetupResult{}, err
		}
	}

	if boolValue(req.DropFirst) {
		if err := dropAndRecreateDatabaseViaSSH(sshClient, req.PostgresUser, req.DatabaseName); err != nil {
			return PostgresAppSetupResult{}, err
		}
	} else if boolValue(req.CreateDB) {
		exists, err := execSQLViaSSHBool(sshClient, req.PostgresUser, req.PostgresConnDB, fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = %s)", pgQuoteLiteral(req.DatabaseName)))
		if err != nil {
			return PostgresAppSetupResult{}, err
		}
		if !exists {
			createStmt := fmt.Sprintf(`CREATE DATABASE %s WITH OWNER = postgres ENCODING = %s TEMPLATE = template0;`, pgQuoteIdentifier(req.DatabaseName), pgQuoteLiteral("UTF8"))
			if err := execSQLViaSSH(sshClient, req.PostgresUser, req.PostgresConnDB, createStmt); err != nil {
				return PostgresAppSetupResult{}, err
			}
		}
	}

	userExists, err := execSQLViaSSHBool(sshClient, req.PostgresUser, req.PostgresConnDB, fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = %s)", pgQuoteLiteral(req.Username)))
	if err != nil {
		return PostgresAppSetupResult{}, err
	}
	if !userExists {
		if err := execSQLViaSSH(sshClient, req.PostgresUser, req.PostgresConnDB, fmt.Sprintf(`CREATE ROLE %s WITH LOGIN;`, pgQuoteIdentifier(req.Username))); err != nil {
			return PostgresAppSetupResult{}, err
		}
	}
	if err := execSQLViaSSH(sshClient, req.PostgresUser, req.PostgresConnDB, fmt.Sprintf(`ALTER USER %s WITH PASSWORD %s;`, pgQuoteIdentifier(req.Username), pgQuoteLiteral(req.Password))); err != nil {
		return PostgresAppSetupResult{}, err
	}
	if err := execSQLViaSSH(sshClient, req.PostgresUser, req.PostgresConnDB, fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE %s TO %s;`, pgQuoteIdentifier(req.DatabaseName), pgQuoteIdentifier(req.Username))); err != nil {
		return PostgresAppSetupResult{}, err
	}
	if err := execSQLViaSSH(sshClient, req.PostgresUser, req.DatabaseName, fmt.Sprintf(`GRANT ALL ON SCHEMA %s TO %s;`, pgQuoteIdentifier(req.SchemaName), pgQuoteIdentifier(req.Username))); err != nil {
		return PostgresAppSetupResult{}, err
	}
	if err := execSQLViaSSH(sshClient, req.PostgresUser, req.DatabaseName, fmt.Sprintf(`ALTER SCHEMA %s OWNER TO %s;`, pgQuoteIdentifier(req.SchemaName), pgQuoteIdentifier(req.Username))); err != nil {
		return PostgresAppSetupResult{}, err
	}

	return PostgresAppSetupResult{
		Host:         sshOpts.Host,
		DatabaseName: req.DatabaseName,
		Username:     req.Username,
		SchemaName:   req.SchemaName,
		PostgresHost: req.PostgresHost,
		PostgresPort: req.PostgresPort,
		URI:          BuildPostgresURL(req.PostgresHost, req.PostgresPort, req.DatabaseName, req.Username, req.Password),
	}, nil
}

func execSQLViaSSH(sshClient interface{ Run(string) ([]byte, error) }, pgUser, dbname, stmt string) error {
	cmdStr := buildRemotePsqlCommand(pgUser, dbname, stmt, false)
	out, err := sshClient.Run(cmdStr)
	if err != nil {
		return FormatExecError(err, out)
	}
	return nil
}

func execSQLViaSSHBool(sshClient interface{ Run(string) ([]byte, error) }, pgUser, dbname, stmt string) (bool, error) {
	cmdStr := buildRemotePsqlCommand(pgUser, dbname, stmt, true)
	out, err := sshClient.Run(cmdStr)
	if err != nil {
		return false, FormatExecError(err, out)
	}
	switch strings.TrimSpace(string(out)) {
	case "t", "true", "1":
		return true, nil
	case "f", "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("unexpected boolean query result: %q", strings.TrimSpace(string(out)))
	}
}

func buildRemotePsqlCommand(pgUser, dbname, stmt string, tuplesOnly bool) string {
	args := []string{
		"sudo", "-u", pgUser,
		"psql",
		"-v", "ON_ERROR_STOP=1",
		"-d", dbname,
	}
	if tuplesOnly {
		args = append(args, "-tA")
	}
	args = append(args, "-c", stmt)
	return shellJoinWords(args)
}

func dropAndRecreateDatabaseViaSSH(sshClient interface{ Run(string) ([]byte, error) }, pgUser, dbname string) error {
	statements := []string{
		fmt.Sprintf(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = %s AND pid <> pg_backend_pid();`, pgQuoteLiteral(dbname)),
		fmt.Sprintf(`DROP DATABASE IF EXISTS %s;`, pgQuoteIdentifier(dbname)),
		fmt.Sprintf(`CREATE DATABASE %s WITH OWNER = postgres ENCODING = %s TEMPLATE = template0;`, pgQuoteIdentifier(dbname), pgQuoteLiteral("UTF8")),
	}
	for _, stmt := range statements {
		if err := execSQLViaSSH(sshClient, pgUser, "postgres", stmt); err != nil {
			return err
		}
	}
	return nil
}

func BuildPostgresURL(host string, port int, dbname, username, password string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		url.QueryEscape(username),
		url.QueryEscape(password),
		host,
		port,
		url.QueryEscape(dbname),
	)
}

func pgQuoteLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func pgQuoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
