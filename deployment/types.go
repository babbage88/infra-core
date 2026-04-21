package deployment

type SSHOptions struct {
	Host             string `json:"host,omitempty"`
	User             string `json:"user,omitempty"`
	KeyPath          string `json:"key_path,omitempty"` // Deprecated for HTTP callers; server-local path only.
	PrivateKeyPEM    string `json:"private_key_pem,omitempty"`
	PrivateKeyBase64 string `json:"private_key_base64,omitempty"`
	Passphrase       string `json:"passphrase,omitempty"`
	UseAgent         bool   `json:"use_agent,omitempty"`
	Port             uint   `json:"port,omitempty"`
}

type ProxyInstallRequest struct {
	SSH             SSHOptions `json:"ssh,omitempty"`
	Name            string     `json:"name,omitempty"`
	PackageName     string     `json:"package_name,omitempty"`
	BinaryName      string     `json:"binary_name,omitempty"`
	ServiceName     string     `json:"service_name,omitempty"`
	ConfigPath      string     `json:"config_path,omitempty"`
	LocalConfigPath string     `json:"local_config_path,omitempty"`
}

type ProxyInstallResult struct {
	Host            string `json:"host"`
	Name            string `json:"name"`
	PackageName     string `json:"package_name"`
	BinaryName      string `json:"binary_name"`
	ServiceName     string `json:"service_name"`
	ConfigPath      string `json:"config_path"`
	LocalConfigPath string `json:"local_config_path,omitempty"`
}

type GarageTokenRequest struct {
	SSH                SSHOptions `json:"ssh,omitempty"`
	BucketName         string     `json:"bucket_name,omitempty"`
	KeyName            string     `json:"key_name,omitempty"`
	CreateBucket       *bool      `json:"create_bucket,omitempty"`
	AllowCreateBuckets *bool      `json:"allow_create_buckets,omitempty"`
	AllowRead          *bool      `json:"allow_read,omitempty"`
	AllowWrite         *bool      `json:"allow_write,omitempty"`
	AllowOwner         *bool      `json:"allow_owner,omitempty"`
	BinaryPath         string     `json:"binary_path,omitempty"`
	ConfigPath         string     `json:"config_path,omitempty"`
	S3Endpoint         string     `json:"s3_endpoint,omitempty"`
	LayoutZone         string     `json:"layout_zone,omitempty"`
	LayoutCapacity     string     `json:"layout_capacity,omitempty"`
}

type GarageTokenResult struct {
	Host              string `json:"host"`
	S3Endpoint        string `json:"s3_endpoint"`
	BucketName        string `json:"bucket_name,omitempty"`
	KeyName           string `json:"key_name"`
	AccessKeyID       string `json:"access_key_id"`
	SecretAccessKey   string `json:"secret_access_key"`
	MCAliasSetCommand string `json:"mc_alias_set_command"`
}

type GarageNodeRequest struct {
	SSH               SSHOptions `json:"ssh,omitempty"`
	Version           string     `json:"version,omitempty"`
	BinaryPath        string     `json:"binary_path,omitempty"`
	ConfigPath        string     `json:"config_path,omitempty"`
	MetadataDir       string     `json:"metadata_dir,omitempty"`
	DataDir           string     `json:"data_dir,omitempty"`
	DBEngine          string     `json:"db_engine,omitempty"`
	ReplicationFactor int        `json:"replication_factor,omitempty"`
	RPCBindAddr       string     `json:"rpc_bind_addr,omitempty"`
	RPCPublicAddr     string     `json:"rpc_public_addr,omitempty"`
	RPCSecret         string     `json:"rpc_secret,omitempty"`
	S3APIBindAddr     string     `json:"s3_api_bind_addr,omitempty"`
	S3Region          string     `json:"s3_region,omitempty"`
	S3RootDomain      string     `json:"s3_root_domain,omitempty"`
	S3WebBindAddr     string     `json:"s3_web_bind_addr,omitempty"`
	S3WebRootDomain   string     `json:"s3_web_root_domain,omitempty"`
	S3WebIndex        string     `json:"s3_web_index,omitempty"`
	K2VAPIBindAddr    string     `json:"k2v_api_bind_addr,omitempty"`
	AdminAPIBindAddr  string     `json:"admin_api_bind_addr,omitempty"`
	AdminToken        string     `json:"admin_token,omitempty"`
	MetricsToken      string     `json:"metrics_token,omitempty"`
	LogLevel          string     `json:"log_level,omitempty"`
}

type GarageNodeResult struct {
	Host          string `json:"host"`
	BinaryPath    string `json:"binary_path"`
	ConfigPath    string `json:"config_path"`
	ServiceName   string `json:"service_name"`
	RPCPublicAddr string `json:"rpc_public_addr"`
	S3Endpoint    string `json:"s3_endpoint"`
	AdminEndpoint string `json:"admin_endpoint"`
	AdminToken    string `json:"admin_token"`
	MetricsToken  string `json:"metrics_token"`
}

type ValkeyInstallRequest struct {
	SSH      SSHOptions `json:"ssh,omitempty"`
	Username string     `json:"username,omitempty"`
	Password string     `json:"password,omitempty"`
	Bind     string     `json:"bind,omitempty"`
	Port     int        `json:"port,omitempty"`
	ACLFile  string     `json:"acl_file,omitempty"`
}

type ValkeyInstallResult struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	URI      string `json:"uri"`
}

type MariaDBInstallRequest struct {
	SSH          SSHOptions `json:"ssh,omitempty"`
	DatabaseName string     `json:"db_name,omitempty"`
	Username     string     `json:"username,omitempty"`
	Password     string     `json:"password,omitempty"`
	Bind         string     `json:"bind,omitempty"`
	Port         int        `json:"port,omitempty"`
}

type MariaDBInstallResult struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	DatabaseName string `json:"db_name"`
	Username     string `json:"username"`
	URI          string `json:"uri"`
}
