package deployment

import (
	"bufio"
	"bytes"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/babbage88/goph/v2"
)

type RemoteValkeyInstaller struct {
	RemoteHostname string
	SshClient      *goph.Client
	RemoteSshUser  string
	OsInfo         map[string]string
}

func NewRemoteValkeyInstallerWithSsh(hostname, sshUser, sshKey, sshPassphrase string, useSshAgent bool, port uint) (*RemoteValkeyInstaller, error) {
	client, err := InitializeSshClient(hostname, sshUser, sshKey, sshPassphrase, useSshAgent, port)
	if err != nil {
		return nil, err
	}

	return &RemoteValkeyInstaller{
		SshClient:      client,
		RemoteHostname: hostname,
		RemoteSshUser:  sshUser,
	}, nil
}

func (rvi *RemoteValkeyInstaller) EnsureInstalledAndConfigured(username, password, bind string, port int, aclFile string) error {
	if strings.TrimSpace(username) == "" {
		return fmt.Errorf("valkey username is required")
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("valkey password is required")
	}
	if err := rvi.loadOsInfo(); err != nil {
		return err
	}
	if err := rvi.ensureValkeyInstalled(); err != nil {
		return err
	}
	configPath, err := rvi.detectConfigAndService()
	if err != nil {
		return err
	}
	if strings.TrimSpace(aclFile) == "" {
		aclFile = defaultValkeyACLFile(configPath)
	}
	if err := rvi.configureRemoteAccess(configPath, bind, port, aclFile); err != nil {
		return err
	}
	if err := rvi.restartService(); err != nil {
		return err
	}
	return rvi.createOrUpdateACLUser(username, password, port)
}

func (rvi *RemoteValkeyInstaller) loadOsInfo() error {
	if rvi.OsInfo != nil {
		return nil
	}
	out, err := rvi.SshClient.Run("cat /etc/os-release")
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
	rvi.OsInfo = osInfoMap
	return nil
}

func (rvi *RemoteValkeyInstaller) ensureValkeyInstalled() error {
	checkScript := `command -v valkey-server >/dev/null 2>&1 && command -v valkey-cli >/dev/null 2>&1`
	if out, err := rvi.SshClient.Run("sh -c " + ShellQuote(checkScript)); err == nil {
		_ = out
		return nil
	}
	osID := rvi.OsInfo["ID"]
	osLike := rvi.OsInfo["ID_LIKE"]
	combinedID := strings.TrimSpace(osID + " " + osLike)
	var installCmd string
	switch {
	case strings.Contains(combinedID, "ubuntu"), strings.Contains(combinedID, "debian"):
		installCmd = "sudo apt-get update -y && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y valkey"
	case strings.Contains(combinedID, "rhel"), strings.Contains(combinedID, "fedora"), strings.Contains(combinedID, "centos"), strings.Contains(combinedID, "rocky"), strings.Contains(combinedID, "alma"), strings.Contains(combinedID, "amzn"):
		installCmd = `if command -v dnf >/dev/null 2>&1; then sudo dnf install -y valkey; elif command -v yum >/dev/null 2>&1; then sudo yum install -y valkey; else echo "No supported package manager found for Valkey install" >&2; exit 1; fi`
	case strings.Contains(combinedID, "arch"):
		installCmd = "sudo pacman -Sy --noconfirm valkey"
	case strings.Contains(combinedID, "suse"), strings.Contains(combinedID, "opensuse"):
		installCmd = "sudo zypper --non-interactive install valkey"
	case strings.Contains(combinedID, "alpine"):
		installCmd = "sudo apk add --no-cache valkey"
	default:
		return fmt.Errorf("unsupported OS for automatic Valkey installation: id=%q id_like=%q", osID, osLike)
	}
	out, err := rvi.SshClient.Run("sh -c " + ShellQuote(installCmd))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("install valkey via distro package manager: %w", err), out)
	}
	verifyScript := `command -v valkey-server >/dev/null 2>&1 && command -v valkey-cli >/dev/null 2>&1`
	out, err = rvi.SshClient.Run("sh -c " + ShellQuote(verifyScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("verify valkey installation: %w", err), out)
	}
	return nil
}

func (rvi *RemoteValkeyInstaller) detectConfigAndService() (string, error) {
	findConfigScript := `for file in /etc/valkey/valkey.conf /etc/valkey.conf /etc/redis/redis.conf /etc/redis.conf /etc/valkey/default.conf /etc/redis/default.conf; do
  if [ -f "$file" ]; then
    printf '%s\n' "$file"
    exit 0
  fi
done
find /etc -maxdepth 5 -type f \( -name 'valkey.conf' -o -name 'redis.conf' -o -name '*.conf' \) 2>/dev/null | grep -E '/(valkey|redis)/|/(valkey|redis)\.conf$' | head -n 1 && exit 0`
	out, err := rvi.SshClient.Run("sudo sh -c " + ShellQuote(findConfigScript))
	if err != nil {
		return "", formatRemoteCommandError(fmt.Errorf("locate valkey config: %w", err), out)
	}
	configPath := strings.TrimSpace(string(out))
	if configPath == "" {
		return "", fmt.Errorf("could not locate a Valkey configuration file on remote host")
	}
	return configPath, nil
}

func (rvi *RemoteValkeyInstaller) configureRemoteAccess(configPath, bind string, port int, aclFile string) error {
	configScript := fmt.Sprintf(
		`set -e
config_file=%s
acl_file=%s
acl_dir=$(dirname "$acl_file")
mkdir -p "$acl_dir"
touch "$acl_file"
if id -u valkey >/dev/null 2>&1; then
  chown valkey:valkey "$acl_file"
elif id -u redis >/dev/null 2>&1; then
  chown redis:redis "$acl_file"
fi
chmod 640 "$acl_file"
set_config() {
  key="$1"
  value="$2"
  if grep -Eq "^[#[:space:]]*${key}[[:space:]]+" "$config_file"; then
    sed -i "s|^[#[:space:]]*${key}[[:space:]].*|${key} ${value}|" "$config_file"
  else
    printf "\n%%s %%s\n" "$key" "$value" >> "$config_file"
  fi
}
set_config bind %s
set_config protected-mode no
set_config port %s
set_config aclfile "$acl_file"`,
		ShellQuote(configPath),
		ShellQuote(aclFile),
		ShellQuote(bind),
		ShellQuote(strconv.Itoa(port)),
	)
	out, err := rvi.SshClient.Run("sudo sh -c " + ShellQuote(configScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("configure valkey remote access: %w", err), out)
	}
	return nil
}

func (rvi *RemoteValkeyInstaller) restartService() error {
	restartScript := fmt.Sprintf(
		`set -e
for svc in %s $(systemctl list-unit-files 'valkey*' 'redis*' --type=service --no-legend 2>/dev/null | awk '{print $1}') $(systemctl list-units --all 'valkey*' 'redis*' --type=service --no-legend 2>/dev/null | awk '{print $1}'); do
  [ -n "$svc" ] || continue
  svc=${svc%%.service}
  if sudo systemctl restart "$svc" >/dev/null 2>&1; then
    exit 0
  fi
  if sudo systemctl start "$svc" >/dev/null 2>&1; then
    exit 0
  fi
  if command -v service >/dev/null 2>&1 && sudo service "$svc" restart >/dev/null 2>&1; then
    exit 0
  fi
done
for svc in valkey valkey-server redis redis-server; do
  if systemctl status "$svc" >/dev/null 2>&1; then
    systemctl --no-pager --full status "$svc" || true
    journalctl -u "$svc" -n 25 --no-pager || true
    break
  fi
done
echo "Unable to restart Valkey service. Tried common Valkey/Redis unit names and detected services." >&2
exit 1`,
		shellJoinWords([]string{"valkey", "valkey.service", "valkey-server", "valkey-server.service", "redis", "redis.service", "redis-server", "redis-server.service"}),
	)
	out, err := rvi.SshClient.Run("sh -c " + ShellQuote(restartScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("restart valkey service: %w", err), out)
	}
	return nil
}

func (rvi *RemoteValkeyInstaller) createOrUpdateACLUser(username, password string, port int) error {
	cliBin, err := rvi.detectValkeyCLI()
	if err != nil {
		return err
	}
	setUserCmd := shellJoinWords([]string{"sudo", cliBin, "-h", "127.0.0.1", "-p", strconv.Itoa(port), "ACL", "SETUSER", username, "on", "resetpass", ">" + password, "~*", "&*", "+@all"})
	if out, err := rvi.SshClient.Run("sh -c " + ShellQuote(setUserCmd)); err != nil {
		return formatRemoteCommandError(fmt.Errorf("create/update valkey ACL user: %w", err), out)
	}
	lockDefaultCmd := shellJoinWords([]string{"sudo", cliBin, "-h", "127.0.0.1", "-p", strconv.Itoa(port), "ACL", "SETUSER", "default", "off", "resetpass", "resetkeys", "resetchannels", "-@all"})
	if out, err := rvi.SshClient.Run("sh -c " + ShellQuote(lockDefaultCmd)); err != nil {
		return formatRemoteCommandError(fmt.Errorf("disable default valkey user: %w", err), out)
	}
	saveACLCommand := shellJoinWords([]string{"sudo", cliBin, "--user", username, "--pass", password, "-h", "127.0.0.1", "-p", strconv.Itoa(port), "ACL", "SAVE"})
	if out, err := rvi.SshClient.Run("sh -c " + ShellQuote(saveACLCommand)); err != nil {
		return formatRemoteCommandError(fmt.Errorf("save valkey ACLs: %w", err), out)
	}
	return nil
}

func (rvi *RemoteValkeyInstaller) detectValkeyCLI() (string, error) {
	checkScript := `if command -v valkey-cli >/dev/null 2>&1; then printf '%s\n' valkey-cli; exit 0; fi
if command -v redis-cli >/dev/null 2>&1; then printf '%s\n' redis-cli; exit 0; fi
echo "No valkey-cli or redis-cli found" >&2
exit 1`
	out, err := rvi.SshClient.Run("sh -c " + ShellQuote(checkScript))
	if err != nil {
		return "", formatRemoteCommandError(fmt.Errorf("locate valkey CLI: %w", err), out)
	}
	return strings.TrimSpace(string(out)), nil
}

func defaultValkeyACLFile(configPath string) string {
	switch {
	case strings.Contains(configPath, "/etc/valkey/"):
		return "/etc/valkey/users.acl"
	case strings.Contains(configPath, "/etc/redis/"):
		return "/etc/redis/users.acl"
	default:
		return path.Join(path.Dir(configPath), "users.acl")
	}
}
