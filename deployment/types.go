package deployment

import (
	"fmt"

	"github.com/google/uuid"
)

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

type SystemdAppDeployRequest struct {
	SSH               SSHOptions        `json:"ssh,omitempty"`
	AppName           string            `json:"app_name,omitempty"`
	EnvVars           map[string]string `json:"env_vars,omitempty"`
	ServiceUser       string            `json:"service_user,omitempty"`
	ServiceUID        int64             `json:"service_uid,omitempty"`
	DestinationBinary string            `json:"destination_binary,omitempty"`
	InstallDir        string            `json:"install_dir,omitempty"`
	SystemdDir        string            `json:"systemd_dir,omitempty"`
	SourceDir         string            `json:"source_dir,omitempty"`
	SourceBin         string            `json:"source_bin,omitempty"`
	SourceGoModule    string            `json:"source_go_module,omitempty"`
	SourceRepo        string            `json:"source_repo,omitempty"`
	SourceRef         string            `json:"source_ref,omitempty"`
	SourcePackage     string            `json:"source_package,omitempty"`
	SourceExcludes    []string          `json:"source_excludes,omitempty"`
}

type SystemdAppDeployResult struct {
	Host         string `json:"host"`
	AppName      string `json:"app_name"`
	ServiceName  string `json:"service_name"`
	ServiceUser  string `json:"service_user"`
	InstallDir   string `json:"install_dir"`
	BinaryPath   string `json:"binary_path"`
	SystemdUnit  string `json:"systemd_unit"`
	SourceBinary string `json:"source_binary,omitempty"`
}

type PostgresAppSetupRequest struct {
	SSH                           SSHOptions `json:"ssh,omitempty"`
	DatabaseName                  string     `json:"db_name,omitempty"`
	Username                      string     `json:"username,omitempty"`
	Password                      string     `json:"password,omitempty"`
	SchemaName                    string     `json:"schema_name,omitempty"`
	CreateDB                      *bool      `json:"create_db,omitempty"`
	DropFirst                     *bool      `json:"drop_first,omitempty"`
	PostgresUser                  string     `json:"postgres_user,omitempty"`
	PostgresPassword              string     `json:"postgres_password,omitempty"`
	PostgresHost                  string     `json:"postgres_host,omitempty"`
	PostgresPort                  int        `json:"postgres_port,omitempty"`
	PostgresConnDB                string     `json:"postgres_conn_db,omitempty"`
	SetupRemotePostgres           *bool      `json:"setup_remote_postgres,omitempty"`
	RemotePostgresHBACIDR         string     `json:"remote_postgres_hba_cidr,omitempty"`
	RemotePostgresAuthMethod      string     `json:"remote_postgres_auth_method,omitempty"`
	RemotePostgresListenAddresses string     `json:"remote_postgres_listen_addresses,omitempty"`
}

type PostgresAppSetupResult struct {
	Host         string `json:"host"`
	DatabaseName string `json:"db_name"`
	Username     string `json:"username"`
	SchemaName   string `json:"schema_name"`
	PostgresHost string `json:"postgres_host"`
	PostgresPort int    `json:"postgres_port"`
	URI          string `json:"uri"`
}

type ProxmoxAuthOptions struct {
	HostURL    string `json:"host_url,omitempty"`
	APIToken   string `json:"api_token,omitempty"` // Format: user@realm!tokenid=secret.
	APITokenID string `json:"api_token_id,omitempty"`
	APISecret  string `json:"api_secret,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	UseToken   *bool  `json:"use_token,omitempty"`
	SkipTLS    *bool  `json:"skip_tls,omitempty"`
}

type ProxmoxVM struct {
	VMID            int     `json:"vmid"`
	Name            string  `json:"name"`
	Status          string  `json:"status"`
	CPU             float64 `json:"cpu,omitempty"`
	MaxMem          int64   `json:"maxmem,omitempty"`
	MemHost         int     `json:"memhost,omitempty"`
	Mem             int64   `json:"mem,omitempty"`
	MaxDisk         int64   `json:"maxdisk,omitempty"`
	NetIn           int     `json:"netin,omitempty"`
	NetOut          int     `json:"netout,omitempty"`
	Disk            int64   `json:"disk,omitempty"`
	DiskRead        int     `json:"diskread,omitempty"`
	DiskWrite       int     `json:"diskwrite,omitempty"`
	Node            string  `json:"node,omitempty"`
	PID             int     `json:"pid,omitempty"`
	PressureCPUFull float64 `json:"pressurecpufull,omitempty"`
	PressureCPUSome float64 `json:"pressurecpusome,omitempty"`
	PressureIOFull  float64 `json:"pressureiofull,omitempty"`
	PressureIOSome  float64 `json:"pressureiosome,omitempty"`
	PressureMemFull float64 `json:"pressurememoryfull,omitempty"`
	PressureMemSome float64 `json:"pressurememorysome,omitempty"`
	QMStatus        string  `json:"qmstatus,omitempty"`
	RunningMachine  string  `json:"running-machine,omitempty"`
	RunningQemu     string  `json:"running-qemu,omitempty"`
	Serial          int     `json:"serial,omitempty"`
	Tags            string  `json:"tags,omitempty"`
	Template        int     `json:"template,omitempty"`
	Uptime          int64   `json:"uptime,omitempty"`
}

type ProxmoxVMListRequest struct {
	Auth            ProxmoxAuthOptions `json:"auth,omitempty"`
	HostServerID    *uuid.UUID         `json:"host_server_id,omitempty"`
	ProxmoxSecretID *uuid.UUID         `json:"proxmox_secret_id,omitempty"`
	Node            string             `json:"node,omitempty"`
	Full            *bool              `json:"full,omitempty"`
}

func (proxmoxReq *ProxmoxVMListRequest) DeriveProxmoxHostUrlFromHostname() error {
	if proxmoxReq.Auth.HostURL != "" {
		return fmt.Errorf("Pve HostURL already defined: %s", proxmoxReq.Auth.HostURL)
	} else {
		proxmoxReq.Auth.HostURL = fmt.Sprintf("https://%s:8006", proxmoxReq.Node)
	}
	return nil
}

type ProxmoxVMListResult struct {
	Node string      `json:"node"`
	VMs  []ProxmoxVM `json:"vms"`
}

type ProxmoxVMStartRequest struct {
	Auth            ProxmoxAuthOptions `json:"auth,omitempty"`
	HostServerID    *uuid.UUID         `json:"host_server_id,omitempty"`
	ProxmoxSecretID *uuid.UUID         `json:"proxmox_secret_id,omitempty"`
	Node            string             `json:"node,omitempty"`
	VMID            int                `json:"vmid,omitempty"`
}

type ProxmoxVMStartResult struct {
	Node string `json:"node"`
	VMID int    `json:"vmid"`
	UPID string `json:"upid,omitempty"`
}

type ProxmoxVMHardwareResult struct {
	Node          string            `json:"node"`
	VMID          int               `json:"vmid"`
	Name          string            `json:"name,omitempty"`
	MemoryMB      int               `json:"memory_mb,omitempty"`
	Sockets       int               `json:"sockets,omitempty"`
	Cores         int               `json:"cores,omitempty"`
	Bridge        string            `json:"bridge,omitempty"`
	VLANTag       string            `json:"vlan_tag,omitempty"`
	NICModel      string            `json:"nic_model,omitempty"`
	MACAddress    string            `json:"mac_address,omitempty"`
	DiskInterface string            `json:"disk_interface,omitempty"`
	DiskSize      string            `json:"disk_size,omitempty"`
	Raw           map[string]string `json:"raw,omitempty"`
}

type ProxmoxVMHardwareUpdateRequest struct {
	Auth            ProxmoxAuthOptions `json:"auth,omitempty"`
	HostServerID    *uuid.UUID         `json:"host_server_id,omitempty"`
	ProxmoxSecretID *uuid.UUID         `json:"proxmox_secret_id,omitempty"`
	Node            string             `json:"node,omitempty"`
	VMID            int                `json:"vmid,omitempty"`
	MemoryMB        *int               `json:"memory_mb,omitempty"`
	Sockets         *int               `json:"sockets,omitempty"`
	Cores           *int               `json:"cores,omitempty"`
	Bridge          *string            `json:"bridge,omitempty"`
	VLANTag         *string            `json:"vlan_tag,omitempty"`
	DiskSizeGB      *int               `json:"disk_size_gb,omitempty"`
}

type ProxmoxLXCResourcesResult struct {
	Node       string            `json:"node"`
	VMID       int               `json:"vmid"`
	Hostname   string            `json:"hostname,omitempty"`
	MemoryMB   int               `json:"memory_mb,omitempty"`
	SwapMB     int               `json:"swap_mb,omitempty"`
	Cores      int               `json:"cores,omitempty"`
	Bridge     string            `json:"bridge,omitempty"`
	VLANTag    string            `json:"vlan_tag,omitempty"`
	RootFS     string            `json:"rootfs,omitempty"`
	RootFSSize string            `json:"rootfs_size,omitempty"`
	Storage    string            `json:"storage,omitempty"`
	Raw        map[string]string `json:"raw,omitempty"`
}

type ProxmoxLXCResourcesUpdateRequest struct {
	Auth            ProxmoxAuthOptions `json:"auth,omitempty"`
	HostServerID    *uuid.UUID         `json:"host_server_id,omitempty"`
	ProxmoxSecretID *uuid.UUID         `json:"proxmox_secret_id,omitempty"`
	Node            string             `json:"node,omitempty"`
	VMID            int                `json:"vmid,omitempty"`
	MemoryMB        *int               `json:"memory_mb,omitempty"`
	SwapMB          *int               `json:"swap_mb,omitempty"`
	Cores           *int               `json:"cores,omitempty"`
	Bridge          *string            `json:"bridge,omitempty"`
	VLANTag         *string            `json:"vlan_tag,omitempty"`
	RootFSSizeGB    *int               `json:"rootfs_size_gb,omitempty"`
}

type ProxmoxLXCRequest struct {
	Auth            ProxmoxAuthOptions `json:"auth,omitempty"`
	HostServerID    *uuid.UUID         `json:"host_server_id,omitempty"`
	ProxmoxSecretID *uuid.UUID         `json:"proxmox_secret_id,omitempty"`
	Node            string             `json:"node,omitempty"`
	VMID            int                `json:"vmid,omitempty"`
	Hostname        string             `json:"hostname,omitempty"`
	Password        string             `json:"password,omitempty"`
	OSTemplate      string             `json:"ostemplate,omitempty"`
	SshPublicKeys   []string           `json:"ssh_public_keys,omitempty"`
	Storage         string             `json:"storage,omitempty"`
	RootFSSize      string             `json:"rootfs_size,omitempty"`
	Memory          int                `json:"memory,omitempty"`
	Swap            int                `json:"swap,omitempty"`
	Cores           int                `json:"cores,omitempty"`
	CPULimit        int                `json:"cpu_limit,omitempty"`
	CPUUnits        int                `json:"cpu_units,omitempty"`
	Net0            string             `json:"net0,omitempty"`
	Arch            string             `json:"arch,omitempty"`
	Cmode           string             `json:"cmode,omitempty"`
	Features        string             `json:"features,omitempty"`
	Nameserver      string             `json:"nameserver,omitempty"`
	SearchDomain    string             `json:"search_domain,omitempty"`
	Description     string             `json:"description,omitempty"`
	Unprivileged    *bool              `json:"unprivileged,omitempty"`
	Start           *bool              `json:"start,omitempty"`
	Console         *bool              `json:"console,omitempty"`
}

type ProxmoxLXCResult struct {
	Node     string `json:"node"`
	VMID     int    `json:"vmid"`
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
	Started  bool   `json:"started"`
	Console  bool   `json:"console"`
}

type ProxmoxVMCreateRequest struct {
	Auth              ProxmoxAuthOptions `json:"auth,omitempty"`
	SSH               SSHOptions         `json:"ssh,omitempty"`
	HostServerID      *uuid.UUID         `json:"host_server_id,omitempty"`
	ProxmoxSecretID   *uuid.UUID         `json:"proxmox_secret_id,omitempty"`
	Node              string             `json:"node,omitempty"`
	VMID              int                `json:"vmid,omitempty"`
	TemplateVMID      int                `json:"template_vmid,omitempty"`
	Name              string             `json:"name,omitempty"`
	MemoryMB          int                `json:"memory_mb,omitempty"`
	Sockets           int                `json:"sockets,omitempty"`
	Cores             int                `json:"cores,omitempty"`
	Description       string             `json:"description,omitempty"`
	Storage           string             `json:"storage,omitempty"`
	FullClone         *bool              `json:"full_clone,omitempty"`
	Start             *bool              `json:"start,omitempty"`
	CIUser            string             `json:"ci_user,omitempty"`
	CIPassword        string             `json:"ci_password,omitempty"`
	SshPublicKeys     []string           `json:"ssh_public_keys,omitempty"`
	IPConfig0         string             `json:"ipconfig0,omitempty"`
	Nameserver        string             `json:"nameserver,omitempty"`
	SearchDomain      string             `json:"search_domain,omitempty"`
	CISnippetsStorage string             `json:"ci_snippets_storage,omitempty"`
	CICustomScript    string             `json:"ci_custom_script,omitempty"`
}

type ProxmoxVMCreateResult struct {
	Node         string `json:"node"`
	VMID         int    `json:"vmid"`
	TemplateVMID int    `json:"template_vmid"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Started      bool   `json:"started"`
}

type ProxmoxVMTemplateRequest struct {
	Auth             ProxmoxAuthOptions `json:"auth,omitempty"`
	SSH              SSHOptions         `json:"ssh,omitempty"`
	HostServerID     *uuid.UUID         `json:"host_server_id,omitempty"`
	ProxmoxSecretID  *uuid.UUID         `json:"proxmox_secret_id,omitempty"`
	Node             string             `json:"node,omitempty"`
	VMID             int                `json:"vmid,omitempty"`
	Name             string             `json:"name,omitempty"`
	ImageURL         string             `json:"image_url,omitempty"`
	Storage          string             `json:"storage,omitempty"`
	CloudInitStorage string             `json:"cloudinit_storage,omitempty"`
	MemoryMB         int                `json:"memory_mb,omitempty"`
	Sockets          int                `json:"sockets,omitempty"`
	Cores            int                `json:"cores,omitempty"`
	Description      string             `json:"description,omitempty"`
	Net0             string             `json:"net0,omitempty"`
	SCSIHW           string             `json:"scsihw,omitempty"`
	DiskBus          string             `json:"disk_bus,omitempty"`
	BootOrder        string             `json:"boot_order,omitempty"`
	Agent            *bool              `json:"agent,omitempty"`
	SerialConsole    *bool              `json:"serial_console,omitempty"`
	CleanupImage     *bool              `json:"cleanup_image,omitempty"`
}

type ProxmoxVMTemplateResult struct {
	Node             string `json:"node"`
	VMID             int    `json:"vmid"`
	Name             string `json:"name"`
	ImportedVolumeID string `json:"imported_volume_id"`
	Template         bool   `json:"template"`
}
