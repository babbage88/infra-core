package deployment

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

func DefaultProxyInstallRequest(name string) (ProxyInstallRequest, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "nginx":
		return ProxyInstallRequest{Name: "nginx", PackageName: "nginx", BinaryName: "nginx", ServiceName: "nginx", ConfigPath: "/etc/nginx/nginx.conf"}, nil
	case "haproxy":
		return ProxyInstallRequest{Name: "haproxy", PackageName: "haproxy", BinaryName: "haproxy", ServiceName: "haproxy", ConfigPath: "/etc/haproxy/haproxy.cfg"}, nil
	case "angie":
		return ProxyInstallRequest{Name: "angie", PackageName: "angie", BinaryName: "angie", ServiceName: "angie", ConfigPath: "/etc/angie/angie.conf"}, nil
	default:
		return ProxyInstallRequest{}, fmt.Errorf("unsupported proxy %q", name)
	}
}

func MergeProxyInstallDefaults(req, defaults ProxyInstallRequest) ProxyInstallRequest {
	if strings.TrimSpace(req.Name) == "" {
		req.Name = defaults.Name
	}
	if strings.TrimSpace(req.PackageName) == "" {
		req.PackageName = defaults.PackageName
	}
	if strings.TrimSpace(req.BinaryName) == "" {
		req.BinaryName = defaults.BinaryName
	}
	if strings.TrimSpace(req.ServiceName) == "" {
		req.ServiceName = defaults.ServiceName
	}
	if strings.TrimSpace(req.ConfigPath) == "" {
		req.ConfigPath = defaults.ConfigPath
	}
	if strings.TrimSpace(req.LocalConfigPath) == "" {
		req.LocalConfigPath = defaults.LocalConfigPath
	}
	req.SSH = MergeSSHDefaults(req.SSH, defaults.SSH)
	return req
}

func MergeSSHDefaults(req, defaults SSHOptions) SSHOptions {
	if strings.TrimSpace(req.Host) == "" {
		req.Host = defaults.Host
	}
	if strings.TrimSpace(req.User) == "" {
		req.User = defaults.User
	}
	if !hasSSHKeySource(req) {
		req.KeyPath = defaults.KeyPath
		req.PrivateKeyPEM = defaults.PrivateKeyPEM
		req.PrivateKeyBase64 = defaults.PrivateKeyBase64
	}
	if strings.TrimSpace(req.Passphrase) == "" {
		req.Passphrase = defaults.Passphrase
	}
	if req.Port == 0 {
		req.Port = defaults.Port
	}
	if !req.UseAgent {
		req.UseAgent = defaults.UseAgent
	}
	if req.Port == 0 {
		req.Port = 22
	}
	return req
}

func hasSSHKeySource(opts SSHOptions) bool {
	return strings.TrimSpace(opts.KeyPath) != "" ||
		strings.TrimSpace(opts.PrivateKeyPEM) != "" ||
		strings.TrimSpace(opts.PrivateKeyBase64) != ""
}

func InstallProxy(req ProxyInstallRequest) (ProxyInstallResult, error) {
	req.Name = strings.ToLower(strings.TrimSpace(req.Name))
	if req.Name == "" {
		return ProxyInstallResult{}, fmt.Errorf("proxy name is required")
	}
	if strings.TrimSpace(req.SSH.Host) == "" {
		return ProxyInstallResult{}, fmt.Errorf("ssh.host is required")
	}
	if strings.TrimSpace(req.SSH.User) == "" {
		return ProxyInstallResult{}, fmt.Errorf("ssh.user is required")
	}
	sshOpts, cleanupSSHKey, err := PrepareSSHOptions(req.SSH)
	if err != nil {
		return ProxyInstallResult{}, err
	}
	defer cleanupSSHKey()

	installer, err := NewRemoteWebProxyInstallerWithSsh(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return ProxyInstallResult{}, fmt.Errorf("initialize SSH client: %w", err)
	}
	defer installer.SshClient.Close()
	cfg := WebProxyInstallConfig{
		Name:            req.Name,
		PackageName:     req.PackageName,
		BinaryName:      req.BinaryName,
		ServiceName:     req.ServiceName,
		ConfigPath:      req.ConfigPath,
		LocalConfigPath: req.LocalConfigPath,
	}
	if err := installer.EnsureInstalledAndConfigured(cfg); err != nil {
		return ProxyInstallResult{}, err
	}
	return ProxyInstallResult{
		Host:            req.SSH.Host,
		Name:            req.Name,
		PackageName:     req.PackageName,
		BinaryName:      req.BinaryName,
		ServiceName:     req.ServiceName,
		ConfigPath:      req.ConfigPath,
		LocalConfigPath: req.LocalConfigPath,
	}, nil
}

func DefaultMariaDBInstallRequest() MariaDBInstallRequest {
	return MariaDBInstallRequest{Bind: "0.0.0.0", Port: 3306}
}

func MergeMariaDBInstallDefaults(req, defaults MariaDBInstallRequest) MariaDBInstallRequest {
	if strings.TrimSpace(req.DatabaseName) == "" {
		req.DatabaseName = defaults.DatabaseName
	}
	if strings.TrimSpace(req.Username) == "" {
		req.Username = defaults.Username
	}
	if strings.TrimSpace(req.Password) == "" {
		req.Password = defaults.Password
	}
	if strings.TrimSpace(req.Bind) == "" {
		req.Bind = defaults.Bind
	}
	if req.Port == 0 {
		req.Port = defaults.Port
	}
	req.SSH = MergeSSHDefaults(req.SSH, defaults.SSH)
	return req
}

func InstallMariaDB(req MariaDBInstallRequest) (MariaDBInstallResult, error) {
	if strings.TrimSpace(req.SSH.Host) == "" {
		return MariaDBInstallResult{}, fmt.Errorf("ssh.host is required")
	}
	if strings.TrimSpace(req.SSH.User) == "" {
		return MariaDBInstallResult{}, fmt.Errorf("ssh.user is required")
	}
	if strings.TrimSpace(req.DatabaseName) == "" {
		return MariaDBInstallResult{}, fmt.Errorf("db_name is required")
	}
	if strings.TrimSpace(req.Username) == "" {
		return MariaDBInstallResult{}, fmt.Errorf("username is required")
	}
	if strings.TrimSpace(req.Password) == "" {
		return MariaDBInstallResult{}, fmt.Errorf("password is required")
	}
	if req.Port <= 0 {
		return MariaDBInstallResult{}, fmt.Errorf("port must be greater than zero")
	}
	sshOpts, cleanupSSHKey, err := PrepareSSHOptions(req.SSH)
	if err != nil {
		return MariaDBInstallResult{}, err
	}
	defer cleanupSSHKey()
	installer, err := NewRemoteMariaDBInstallerWithSsh(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return MariaDBInstallResult{}, fmt.Errorf("initialize SSH client: %w", err)
	}
	defer installer.SshClient.Close()
	if err := installer.EnsureInstalledAndConfigured(req.DatabaseName, req.Username, req.Password, req.Bind, req.Port); err != nil {
		return MariaDBInstallResult{}, err
	}
	return MariaDBInstallResult{
		Host:         sshOpts.Host,
		Port:         req.Port,
		DatabaseName: req.DatabaseName,
		Username:     req.Username,
		URI:          BuildMariaDBURL(sshOpts.Host, req.Port, req.DatabaseName, req.Username, req.Password),
	}, nil
}

func BuildMariaDBURL(host string, port int, dbname, username, password string) string {
	return fmt.Sprintf("mysql://%s:%s@%s:%d/%s", url.QueryEscape(username), url.QueryEscape(password), host, port, url.QueryEscape(dbname))
}

func DefaultValkeyInstallRequest() ValkeyInstallRequest {
	return ValkeyInstallRequest{Bind: "0.0.0.0", Port: 6379}
}

func MergeValkeyInstallDefaults(req, defaults ValkeyInstallRequest) ValkeyInstallRequest {
	if strings.TrimSpace(req.Username) == "" {
		req.Username = defaults.Username
	}
	if strings.TrimSpace(req.Password) == "" {
		req.Password = defaults.Password
	}
	if strings.TrimSpace(req.Bind) == "" {
		req.Bind = defaults.Bind
	}
	if req.Port == 0 {
		req.Port = defaults.Port
	}
	if strings.TrimSpace(req.ACLFile) == "" {
		req.ACLFile = defaults.ACLFile
	}
	req.SSH = MergeSSHDefaults(req.SSH, defaults.SSH)
	return req
}

func InstallValkey(req ValkeyInstallRequest) (ValkeyInstallResult, error) {
	if strings.TrimSpace(req.SSH.Host) == "" {
		return ValkeyInstallResult{}, fmt.Errorf("ssh.host is required")
	}
	if strings.TrimSpace(req.SSH.User) == "" {
		return ValkeyInstallResult{}, fmt.Errorf("ssh.user is required")
	}
	if strings.TrimSpace(req.Username) == "" {
		return ValkeyInstallResult{}, fmt.Errorf("username is required")
	}
	if strings.TrimSpace(req.Password) == "" {
		return ValkeyInstallResult{}, fmt.Errorf("password is required")
	}
	if req.Port <= 0 {
		return ValkeyInstallResult{}, fmt.Errorf("port must be greater than zero")
	}
	sshOpts, cleanupSSHKey, err := PrepareSSHOptions(req.SSH)
	if err != nil {
		return ValkeyInstallResult{}, err
	}
	defer cleanupSSHKey()
	installer, err := NewRemoteValkeyInstallerWithSsh(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return ValkeyInstallResult{}, fmt.Errorf("initialize SSH client: %w", err)
	}
	defer installer.SshClient.Close()
	if err := installer.EnsureInstalledAndConfigured(req.Username, req.Password, req.Bind, req.Port, req.ACLFile); err != nil {
		return ValkeyInstallResult{}, err
	}
	return ValkeyInstallResult{
		Host:     sshOpts.Host,
		Port:     req.Port,
		Username: req.Username,
		URI:      BuildValkeyURL(sshOpts.Host, req.Port, req.Username, req.Password),
	}, nil
}

func BuildValkeyURL(host string, port int, username, password string) string {
	return fmt.Sprintf("redis://%s:%s@%s:%d", url.QueryEscape(username), url.QueryEscape(password), host, port)
}

func DefaultGarageTokenRequest() GarageTokenRequest {
	return GarageTokenRequest{
		CreateBucket:       BoolPtr(true),
		AllowCreateBuckets: BoolPtr(false),
		AllowRead:          BoolPtr(true),
		AllowWrite:         BoolPtr(true),
		AllowOwner:         BoolPtr(true),
		BinaryPath:         "/usr/local/bin/garage",
		ConfigPath:         "/etc/garage.toml",
		LayoutZone:         "dc1",
		LayoutCapacity:     "100G",
	}
}

func MergeGarageTokenDefaults(req, defaults GarageTokenRequest) GarageTokenRequest {
	if strings.TrimSpace(req.BucketName) == "" {
		req.BucketName = defaults.BucketName
	}
	if strings.TrimSpace(req.KeyName) == "" {
		req.KeyName = defaults.KeyName
	}
	if req.CreateBucket == nil {
		req.CreateBucket = defaults.CreateBucket
	}
	if req.AllowCreateBuckets == nil {
		req.AllowCreateBuckets = defaults.AllowCreateBuckets
	}
	if boolValue(req.AllowCreateBuckets) && strings.TrimSpace(req.BucketName) == "" && req.AllowRead == nil && req.AllowWrite == nil && req.AllowOwner == nil {
		req.AllowRead = BoolPtr(false)
		req.AllowWrite = BoolPtr(false)
		req.AllowOwner = BoolPtr(false)
	}
	if req.AllowRead == nil {
		req.AllowRead = defaults.AllowRead
	}
	if req.AllowWrite == nil {
		req.AllowWrite = defaults.AllowWrite
	}
	if req.AllowOwner == nil {
		req.AllowOwner = defaults.AllowOwner
	}
	if strings.TrimSpace(req.BinaryPath) == "" {
		req.BinaryPath = defaults.BinaryPath
	}
	if strings.TrimSpace(req.ConfigPath) == "" {
		req.ConfigPath = defaults.ConfigPath
	}
	if strings.TrimSpace(req.S3Endpoint) == "" {
		req.S3Endpoint = defaults.S3Endpoint
	}
	if strings.TrimSpace(req.LayoutZone) == "" {
		req.LayoutZone = defaults.LayoutZone
	}
	if strings.TrimSpace(req.LayoutCapacity) == "" {
		req.LayoutCapacity = defaults.LayoutCapacity
	}
	req.SSH = MergeSSHDefaults(req.SSH, defaults.SSH)
	return req
}

func CreateGarageToken(req GarageTokenRequest) (GarageTokenResult, error) {
	if strings.TrimSpace(req.SSH.Host) == "" {
		return GarageTokenResult{}, fmt.Errorf("ssh.host is required")
	}
	if strings.TrimSpace(req.SSH.User) == "" {
		return GarageTokenResult{}, fmt.Errorf("ssh.user is required")
	}
	if strings.TrimSpace(req.KeyName) == "" {
		return GarageTokenResult{}, fmt.Errorf("key_name is required")
	}
	createBucket := boolValue(req.CreateBucket)
	allowCreateBuckets := boolValue(req.AllowCreateBuckets)
	allowRead := boolValue(req.AllowRead)
	allowWrite := boolValue(req.AllowWrite)
	allowOwner := boolValue(req.AllowOwner)
	if strings.TrimSpace(req.BucketName) == "" && !allowCreateBuckets {
		return GarageTokenResult{}, fmt.Errorf("bucket_name is required unless allow_create_buckets is true")
	}
	if strings.TrimSpace(req.S3Endpoint) == "" {
		req.S3Endpoint = fmt.Sprintf("http://%s:3900", req.SSH.Host)
	}
	sshOpts, cleanupSSHKey, err := PrepareSSHOptions(req.SSH)
	if err != nil {
		return GarageTokenResult{}, err
	}
	defer cleanupSSHKey()
	installer, err := NewRemoteGarageInstallerWithSsh(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return GarageTokenResult{}, fmt.Errorf("initialize SSH client: %w", err)
	}
	defer installer.SshClient.Close()
	tokenReq := GarageTokenInternalRequest{
		BucketName:         req.BucketName,
		KeyName:            req.KeyName,
		CreateBucket:       createBucket,
		AllowCreateBuckets: allowCreateBuckets,
		AllowRead:          allowRead,
		AllowWrite:         allowWrite,
		AllowOwner:         allowOwner,
		BinaryPath:         req.BinaryPath,
		ConfigPath:         req.ConfigPath,
		LayoutZone:         req.LayoutZone,
		LayoutCapacity:     req.LayoutCapacity,
	}
	creds, err := installer.CreateS3Token(tokenReq)
	if err != nil {
		return GarageTokenResult{}, err
	}
	return GarageTokenResult{
		Host:              sshOpts.Host,
		S3Endpoint:        req.S3Endpoint,
		BucketName:        creds.BucketName,
		KeyName:           creds.KeyName,
		AccessKeyID:       creds.AccessKeyID,
		SecretAccessKey:   creds.SecretAccessKey,
		MCAliasSetCommand: fmt.Sprintf("mc alias set garage %s %s %s", req.S3Endpoint, creds.AccessKeyID, creds.SecretAccessKey),
	}, nil
}

func DefaultGarageNodeRequest() GarageNodeRequest {
	return GarageNodeRequest{
		Version:           "v2.2.0",
		BinaryPath:        "/usr/local/bin/garage",
		ConfigPath:        "/etc/garage.toml",
		MetadataDir:       "/var/lib/garage/meta",
		DataDir:           "/var/lib/garage/data",
		DBEngine:          "sqlite",
		ReplicationFactor: 1,
		RPCBindAddr:       "[::]:3901",
		S3APIBindAddr:     "[::]:3900",
		S3Region:          "garage",
		S3RootDomain:      ".s3.local",
		S3WebBindAddr:     "[::]:3902",
		S3WebRootDomain:   ".web.local",
		S3WebIndex:        "index.html",
		K2VAPIBindAddr:    "[::]:3904",
		AdminAPIBindAddr:  "[::]:3903",
		LogLevel:          "garage=info",
	}
}

func MergeGarageNodeDefaults(req, defaults GarageNodeRequest) GarageNodeRequest {
	if strings.TrimSpace(req.Version) == "" {
		req.Version = defaults.Version
	}
	if strings.TrimSpace(req.BinaryPath) == "" {
		req.BinaryPath = defaults.BinaryPath
	}
	if strings.TrimSpace(req.ConfigPath) == "" {
		req.ConfigPath = defaults.ConfigPath
	}
	if strings.TrimSpace(req.MetadataDir) == "" {
		req.MetadataDir = defaults.MetadataDir
	}
	if strings.TrimSpace(req.DataDir) == "" {
		req.DataDir = defaults.DataDir
	}
	if strings.TrimSpace(req.DBEngine) == "" {
		req.DBEngine = defaults.DBEngine
	}
	if req.ReplicationFactor == 0 {
		req.ReplicationFactor = defaults.ReplicationFactor
	}
	if strings.TrimSpace(req.RPCBindAddr) == "" {
		req.RPCBindAddr = defaults.RPCBindAddr
	}
	if strings.TrimSpace(req.RPCPublicAddr) == "" {
		req.RPCPublicAddr = defaults.RPCPublicAddr
	}
	if strings.TrimSpace(req.RPCSecret) == "" {
		req.RPCSecret = defaults.RPCSecret
	}
	if strings.TrimSpace(req.S3APIBindAddr) == "" {
		req.S3APIBindAddr = defaults.S3APIBindAddr
	}
	if strings.TrimSpace(req.S3Region) == "" {
		req.S3Region = defaults.S3Region
	}
	if strings.TrimSpace(req.S3RootDomain) == "" {
		req.S3RootDomain = defaults.S3RootDomain
	}
	if strings.TrimSpace(req.S3WebBindAddr) == "" {
		req.S3WebBindAddr = defaults.S3WebBindAddr
	}
	if strings.TrimSpace(req.S3WebRootDomain) == "" {
		req.S3WebRootDomain = defaults.S3WebRootDomain
	}
	if strings.TrimSpace(req.S3WebIndex) == "" {
		req.S3WebIndex = defaults.S3WebIndex
	}
	if strings.TrimSpace(req.K2VAPIBindAddr) == "" {
		req.K2VAPIBindAddr = defaults.K2VAPIBindAddr
	}
	if strings.TrimSpace(req.AdminAPIBindAddr) == "" {
		req.AdminAPIBindAddr = defaults.AdminAPIBindAddr
	}
	if strings.TrimSpace(req.AdminToken) == "" {
		req.AdminToken = defaults.AdminToken
	}
	if strings.TrimSpace(req.MetricsToken) == "" {
		req.MetricsToken = defaults.MetricsToken
	}
	if strings.TrimSpace(req.LogLevel) == "" {
		req.LogLevel = defaults.LogLevel
	}
	req.SSH = MergeSSHDefaults(req.SSH, defaults.SSH)
	return req
}

func DeployGarageNode(req GarageNodeRequest) (GarageNodeResult, error) {
	if strings.TrimSpace(req.SSH.Host) == "" {
		return GarageNodeResult{}, fmt.Errorf("ssh.host is required")
	}
	if strings.TrimSpace(req.SSH.User) == "" {
		return GarageNodeResult{}, fmt.Errorf("ssh.user is required")
	}
	if req.ReplicationFactor <= 0 {
		return GarageNodeResult{}, fmt.Errorf("replication_factor must be greater than zero")
	}
	if strings.TrimSpace(req.RPCPublicAddr) == "" {
		req.RPCPublicAddr = fmt.Sprintf("%s:3901", req.SSH.Host)
	}
	var err error
	if strings.TrimSpace(req.RPCSecret) == "" {
		req.RPCSecret, err = randomHexString(32)
		if err != nil {
			return GarageNodeResult{}, err
		}
	}
	if strings.TrimSpace(req.AdminToken) == "" {
		req.AdminToken, err = randomBase64String(32)
		if err != nil {
			return GarageNodeResult{}, err
		}
	}
	if strings.TrimSpace(req.MetricsToken) == "" {
		req.MetricsToken, err = randomBase64String(32)
		if err != nil {
			return GarageNodeResult{}, err
		}
	}
	sshOpts, cleanupSSHKey, err := PrepareSSHOptions(req.SSH)
	if err != nil {
		return GarageNodeResult{}, err
	}
	defer cleanupSSHKey()
	installer, err := NewRemoteGarageInstallerWithSsh(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return GarageNodeResult{}, fmt.Errorf("initialize SSH client: %w", err)
	}
	defer installer.SshClient.Close()
	cfg := GarageNodeConfig{
		Version: req.Version, BinaryPath: req.BinaryPath, ConfigPath: req.ConfigPath,
		MetadataDir: req.MetadataDir, DataDir: req.DataDir, DBEngine: req.DBEngine,
		ReplicationFactor: req.ReplicationFactor, RPCBindAddr: req.RPCBindAddr, RPCPublicAddr: req.RPCPublicAddr, RPCSecret: req.RPCSecret,
		S3Region: req.S3Region, S3APIBindAddr: req.S3APIBindAddr, S3RootDomain: req.S3RootDomain,
		S3WebBindAddr: req.S3WebBindAddr, S3WebRootDomain: req.S3WebRootDomain, S3WebIndex: req.S3WebIndex,
		K2VAPIBindAddr: req.K2VAPIBindAddr, AdminAPIBindAddr: req.AdminAPIBindAddr, AdminToken: req.AdminToken, MetricsToken: req.MetricsToken, LogLevel: req.LogLevel,
	}
	if err := installer.EnsureInstalledAndConfigured(cfg); err != nil {
		return GarageNodeResult{}, err
	}
	return GarageNodeResult{
		Host: sshOpts.Host, BinaryPath: req.BinaryPath, ConfigPath: req.ConfigPath, ServiceName: "garage",
		RPCPublicAddr: req.RPCPublicAddr, S3Endpoint: "http://" + garageS3BindAddrToAdvertised(sshOpts.Host, req.S3APIBindAddr),
		AdminEndpoint: "http://" + garageBindAddrToAdvertised(sshOpts.Host, req.AdminAPIBindAddr), AdminToken: req.AdminToken, MetricsToken: req.MetricsToken,
	}, nil
}

func boolValue(value *bool) bool { return value != nil && *value }

func garageBindAddrToAdvertised(host, bindAddr string) string {
	if strings.HasPrefix(bindAddr, "[::]:") || strings.HasPrefix(bindAddr, "0.0.0.0:") || strings.HasPrefix(bindAddr, ":") {
		_, port, _ := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(bindAddr, "[::]"), "0.0.0.0"), ":"), ":")
		if port == "" {
			port = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(bindAddr, "[::]"), "0.0.0.0"), ":")
		}
		return host + ":" + port
	}
	return bindAddr
}

func garageS3BindAddrToAdvertised(host, bindAddr string) string {
	return garageBindAddrToAdvertised(host, bindAddr)
}

func randomHexString(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomBase64String(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(buf), nil
}
