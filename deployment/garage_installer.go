package deployment

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/babbage88/goph/v2"
)

type GarageNodeConfig struct {
	Version           string
	BinaryPath        string
	ConfigPath        string
	MetadataDir       string
	DataDir           string
	DBEngine          string
	ReplicationFactor int
	RPCBindAddr       string
	RPCPublicAddr     string
	RPCSecret         string
	S3Region          string
	S3APIBindAddr     string
	S3RootDomain      string
	S3WebBindAddr     string
	S3WebRootDomain   string
	S3WebIndex        string
	K2VAPIBindAddr    string
	AdminAPIBindAddr  string
	AdminToken        string
	MetricsToken      string
	LogLevel          string
}

type RemoteGarageInstaller struct {
	RemoteHostname string
	SshClient      *goph.Client
	RemoteSshUser  string
	OsInfo         map[string]string
}

func NewRemoteGarageInstallerWithSsh(hostname, sshUser, sshKey, sshPassphrase string, useSshAgent bool, port uint) (*RemoteGarageInstaller, error) {
	client, err := InitializeSshClient(hostname, sshUser, sshKey, sshPassphrase, useSshAgent, port)
	if err != nil {
		return nil, err
	}
	return &RemoteGarageInstaller{
		SshClient:      client,
		RemoteHostname: hostname,
		RemoteSshUser:  sshUser,
	}, nil
}

func (rgi *RemoteGarageInstaller) EnsureInstalledAndConfigured(cfg GarageNodeConfig) error {
	if strings.TrimSpace(cfg.BinaryPath) == "" {
		return fmt.Errorf("garage binary path is required")
	}
	if strings.TrimSpace(cfg.ConfigPath) == "" {
		return fmt.Errorf("garage config path is required")
	}
	if strings.TrimSpace(cfg.MetadataDir) == "" || strings.TrimSpace(cfg.DataDir) == "" {
		return fmt.Errorf("garage metadata and data directories are required")
	}
	if strings.TrimSpace(cfg.RPCPublicAddr) == "" || strings.TrimSpace(cfg.RPCSecret) == "" {
		return fmt.Errorf("garage rpc public address and secret are required")
	}
	if strings.TrimSpace(cfg.AdminToken) == "" || strings.TrimSpace(cfg.MetricsToken) == "" {
		return fmt.Errorf("garage admin and metrics tokens are required")
	}
	if err := rgi.loadOsInfo(); err != nil {
		return err
	}
	if err := rgi.ensureGarageInstalled(cfg); err != nil {
		return err
	}
	if err := rgi.writeGarageConfig(cfg); err != nil {
		return err
	}
	if err := rgi.installSystemdService(cfg); err != nil {
		return err
	}
	if err := rgi.restartService(); err != nil {
		return err
	}
	return rgi.verifyGarageStatus(cfg)
}

func (rgi *RemoteGarageInstaller) loadOsInfo() error {
	if rgi.OsInfo != nil {
		return nil
	}
	out, err := rgi.SshClient.Run("cat /etc/os-release")
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("read /etc/os-release: %w", err), out)
	}
	osInfoMap := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		osInfoMap[key] = value
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	rgi.OsInfo = osInfoMap
	return nil
}

func (rgi *RemoteGarageInstaller) ensureGarageInstalled(cfg GarageNodeConfig) error {
	checkCmd := shellJoinWords([]string{cfg.BinaryPath, "--version"})
	if out, err := rgi.SshClient.Run("sh -c " + ShellQuote(checkCmd)); err == nil {
		_ = out
		return nil
	}
	osID := rgi.OsInfo["ID"]
	osLike := rgi.OsInfo["ID_LIKE"]
	combinedID := strings.TrimSpace(osID + " " + osLike)
	var installScript string
	switch {
	case strings.Contains(combinedID, "alpine"):
		installScript = "sudo apk add --no-cache garage"
	case strings.Contains(combinedID, "arch"):
		installScript = "sudo pacman -Sy --noconfirm garage"
	default:
		version := strings.TrimSpace(cfg.Version)
		if version == "" {
			version = "v2.2.0"
		}
		installScript = fmt.Sprintf(
			`set -e
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO- "$1"; }
else
  echo "curl or wget is required to download Garage" >&2
  exit 1
fi
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) target="x86_64-unknown-linux-musl" ;;
  aarch64|arm64) target="aarch64-unknown-linux-musl" ;;
  armv6l|armv7l) target="armv6l-unknown-linux-musleabihf" ;;
  *) echo "unsupported architecture for official Garage binary download: $arch" >&2; exit 1 ;;
esac
tmp_bin=$(mktemp)
fetch "https://garagehq.deuxfleurs.fr/_releases/%s/${target}/garage" > "$tmp_bin"
chmod +x "$tmp_bin"
sudo install -m 0755 "$tmp_bin" %s
rm -f "$tmp_bin"`,
			version,
			ShellQuote(cfg.BinaryPath),
		)
	}
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(installScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("install garage: %w", err), out)
	}
	verifyCmd := shellJoinWords([]string{cfg.BinaryPath, "--version"})
	out, err = rgi.SshClient.Run("sh -c " + ShellQuote(verifyCmd))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("verify garage installation: %w", err), out)
	}
	return nil
}

func (rgi *RemoteGarageInstaller) writeGarageConfig(cfg GarageNodeConfig) error {
	configContent := buildGarageConfig(cfg)
	writeScript := fmt.Sprintf(
		`set -e
config_dir=$(dirname %s)
mkdir -p "$config_dir"
sudo mkdir -p %s %s
cat > /tmp/garage.toml <<'EOF'
%s
EOF
sudo install -m 0644 /tmp/garage.toml %s
rm -f /tmp/garage.toml`,
		ShellQuote(cfg.ConfigPath),
		ShellQuote(cfg.MetadataDir),
		ShellQuote(cfg.DataDir),
		configContent,
		ShellQuote(cfg.ConfigPath),
	)
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(writeScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("write garage config: %w", err), out)
	}
	return nil
}

func (rgi *RemoteGarageInstaller) installSystemdService(cfg GarageNodeConfig) error {
	serviceContent := fmt.Sprintf(`[Unit]
Description=Garage Data Store
After=network-online.target
Wants=network-online.target

[Service]
Environment='RUST_LOG=%s' 'RUST_BACKTRACE=1'
ExecStart=%s -c %s server
StateDirectory=garage
DynamicUser=true
User=garage
Group=garage
PermissionsStartOnly=true
ExecStartPre=/usr/bin/install -d -o garage -g garage -m 0750 %s %s
ProtectHome=true
NoNewPrivileges=true
LimitNOFILE=42000

[Install]
WantedBy=multi-user.target
`, cfg.LogLevel, cfg.BinaryPath, cfg.ConfigPath, cfg.MetadataDir, cfg.DataDir)
	writeScript := fmt.Sprintf(
		`set -e
if ! getent group garage >/dev/null 2>&1; then sudo groupadd --system garage; fi
if ! id -u garage >/dev/null 2>&1; then sudo useradd --system --home-dir /var/lib/garage --shell /usr/sbin/nologin --gid garage garage; fi
cat > /tmp/garage.service <<'EOF'
%s
EOF
sudo install -m 0644 /tmp/garage.service /etc/systemd/system/garage.service
rm -f /tmp/garage.service
sudo systemctl daemon-reload
sudo systemctl enable garage`,
		serviceContent,
	)
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(writeScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("install garage systemd service: %w", err), out)
	}
	return nil
}

func (rgi *RemoteGarageInstaller) restartService() error {
	restartScript := `set -e
if sudo systemctl restart garage >/dev/null 2>&1; then exit 0; fi
if sudo systemctl start garage >/dev/null 2>&1; then exit 0; fi
systemctl --no-pager --full status garage || true
journalctl -u garage -n 25 --no-pager || true
echo "Unable to restart Garage service." >&2
exit 1`
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(restartScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("restart garage service: %w", err), out)
	}
	return nil
}

func (rgi *RemoteGarageInstaller) verifyGarageStatus(cfg GarageNodeConfig) error {
	verifyScript := fmt.Sprintf(
		`set -e
sudo systemctl is-active --quiet garage
node_key_file=%s
if [ ! -f "$node_key_file" ]; then exit 0; fi
exec %s`,
		ShellQuote(cfg.MetadataDir+"/node_key"),
		shellJoinWords([]string{cfg.BinaryPath, "-c", cfg.ConfigPath, "status"}),
	)
	out, err := rgi.SshClient.Run("sh -c " + ShellQuote(verifyScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("verify garage status: %w", err), out)
	}
	return nil
}

func buildGarageConfig(cfg GarageNodeConfig) string {
	return fmt.Sprintf(`metadata_dir = %s
data_dir = %s
db_engine = %s

replication_factor = %d
rpc_bind_addr = %s
rpc_public_addr = %s
rpc_secret = %s

[s3_api]
s3_region = %s
api_bind_addr = %s
root_domain = %s

[s3_web]
bind_addr = %s
root_domain = %s
index = %s

[k2v_api]
api_bind_addr = %s

[admin]
api_bind_addr = %s
admin_token = %s
metrics_token = %s
`,
		quoteGarageTOMLString(cfg.MetadataDir),
		quoteGarageTOMLString(cfg.DataDir),
		quoteGarageTOMLString(cfg.DBEngine),
		cfg.ReplicationFactor,
		quoteGarageTOMLString(cfg.RPCBindAddr),
		quoteGarageTOMLString(cfg.RPCPublicAddr),
		quoteGarageTOMLString(cfg.RPCSecret),
		quoteGarageTOMLString(cfg.S3Region),
		quoteGarageTOMLString(cfg.S3APIBindAddr),
		quoteGarageTOMLString(cfg.S3RootDomain),
		quoteGarageTOMLString(cfg.S3WebBindAddr),
		quoteGarageTOMLString(cfg.S3WebRootDomain),
		quoteGarageTOMLString(cfg.S3WebIndex),
		quoteGarageTOMLString(cfg.K2VAPIBindAddr),
		quoteGarageTOMLString(cfg.AdminAPIBindAddr),
		quoteGarageTOMLString(cfg.AdminToken),
		quoteGarageTOMLString(cfg.MetricsToken),
	)
}

func quoteGarageTOMLString(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
