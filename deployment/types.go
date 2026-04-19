package deployment

type SSHOptions struct {
	Host       string `json:"host,omitempty"`
	User       string `json:"user,omitempty"`
	KeyPath    string `json:"key_path,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	UseAgent   bool   `json:"use_agent,omitempty"`
	Port       uint   `json:"port,omitempty"`
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
