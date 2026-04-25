package deployment

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/babbage88/goph/v2"
)

type WebProxyInstallConfig struct {
	Name            string
	PackageName     string
	BinaryName      string
	ServiceName     string
	ConfigPath      string
	LocalConfigPath string
}

type RemoteWebProxyInstaller struct {
	RemoteHostname string
	SshClient      *goph.Client
	RemoteSshUser  string
	OsInfo         map[string]string
}

func NewRemoteWebProxyInstallerWithSsh(hostname, sshUser, sshKey, sshPassphrase string, useSshAgent bool, port uint) (*RemoteWebProxyInstaller, error) {
	client, err := InitializeSshClient(hostname, sshUser, sshKey, sshPassphrase, useSshAgent, port)
	if err != nil {
		return nil, err
	}

	return &RemoteWebProxyInstaller{
		SshClient:      client,
		RemoteHostname: hostname,
		RemoteSshUser:  sshUser,
	}, nil
}

func (rwpi *RemoteWebProxyInstaller) EnsureInstalledAndConfigured(cfg WebProxyInstallConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("proxy name is required")
	}
	if strings.TrimSpace(cfg.PackageName) == "" {
		return fmt.Errorf("package name is required for %s", cfg.Name)
	}
	if strings.TrimSpace(cfg.BinaryName) == "" {
		return fmt.Errorf("binary name is required for %s", cfg.Name)
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return fmt.Errorf("service name is required for %s", cfg.Name)
	}
	if strings.TrimSpace(cfg.ConfigPath) == "" {
		return fmt.Errorf("config path is required for %s", cfg.Name)
	}

	if err := rwpi.loadOsInfo(); err != nil {
		return err
	}
	if err := rwpi.ensurePackageInstalled(cfg); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.LocalConfigPath) != "" {
		if err := rwpi.deployConfig(cfg); err != nil {
			return err
		}
		if err := rwpi.restartService(cfg.ServiceName); err != nil {
			return err
		}
	} else {
		if err := rwpi.enableAndStartService(cfg.ServiceName); err != nil {
			return err
		}
	}
	if err := rwpi.verifyService(cfg.ServiceName); err != nil {
		return err
	}

	return nil
}

func (rwpi *RemoteWebProxyInstaller) loadOsInfo() error {
	if rwpi.OsInfo != nil {
		return nil
	}

	out, err := rwpi.SshClient.Run("cat /etc/os-release")
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

	rwpi.OsInfo = osInfoMap
	return nil
}

func (rwpi *RemoteWebProxyInstaller) ensurePackageInstalled(cfg WebProxyInstallConfig) error {
	checkScript := fmt.Sprintf("command -v %s >/dev/null 2>&1", ShellQuote(cfg.BinaryName))
	if out, err := rwpi.SshClient.Run("sh -c " + ShellQuote(checkScript)); err == nil {
		_ = out
		return nil
	}

	osID := rwpi.OsInfo["ID"]
	osLike := rwpi.OsInfo["ID_LIKE"]
	combinedID := strings.TrimSpace(osID + " " + osLike)

	var installCmd string
	switch {
	case strings.Contains(combinedID, "ubuntu"), strings.Contains(combinedID, "debian"):
		installCmd = fmt.Sprintf("sudo apt-get update -y && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y %s", ShellQuote(cfg.PackageName))
	case strings.Contains(combinedID, "rhel"), strings.Contains(combinedID, "fedora"), strings.Contains(combinedID, "centos"), strings.Contains(combinedID, "rocky"), strings.Contains(combinedID, "alma"), strings.Contains(combinedID, "amzn"):
		installCmd = fmt.Sprintf(`if command -v dnf >/dev/null 2>&1; then sudo dnf install -y %s; elif command -v yum >/dev/null 2>&1; then sudo yum install -y %s; else echo "No supported package manager found for %s install" >&2; exit 1; fi`, ShellQuote(cfg.PackageName), ShellQuote(cfg.PackageName), cfg.Name)
	case strings.Contains(combinedID, "arch"):
		installCmd = fmt.Sprintf("sudo pacman -Sy --noconfirm %s", ShellQuote(cfg.PackageName))
	case strings.Contains(combinedID, "suse"), strings.Contains(combinedID, "opensuse"):
		installCmd = fmt.Sprintf("sudo zypper --non-interactive install %s", ShellQuote(cfg.PackageName))
	case strings.Contains(combinedID, "alpine"):
		installCmd = fmt.Sprintf("sudo apk add --no-cache %s", ShellQuote(cfg.PackageName))
	default:
		return fmt.Errorf("unsupported OS for automatic %s installation: id=%q id_like=%q", cfg.Name, osID, osLike)
	}

	out, err := rwpi.SshClient.Run("sh -c " + ShellQuote(installCmd))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("install %s via distro package manager: %w", cfg.Name, err), out)
	}

	verifyScript := fmt.Sprintf("command -v %s >/dev/null 2>&1", ShellQuote(cfg.BinaryName))
	out, err = rwpi.SshClient.Run("sh -c " + ShellQuote(verifyScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("verify %s installation: %w", cfg.Name, err), out)
	}

	return nil
}

func (rwpi *RemoteWebProxyInstaller) deployConfig(cfg WebProxyInstallConfig) error {
	localConfigPath := expandLocalPath(cfg.LocalConfigPath)
	info, err := os.Stat(localConfigPath)
	if err != nil {
		return fmt.Errorf("stat local %s config %q: %w", cfg.Name, localConfigPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("local %s config %q is a directory", cfg.Name, localConfigPath)
	}

	tmpRemotePath := filepath.ToSlash(filepath.Join("/tmp", fmt.Sprintf("%s-config-%d", cfg.Name, os.Getpid())))
	if err := rwpi.SshClient.Upload(localConfigPath, tmpRemotePath); err != nil {
		return fmt.Errorf("upload %s config to remote host: %w", cfg.Name, err)
	}

	configDir := filepath.ToSlash(filepath.Dir(cfg.ConfigPath))
	installScript := fmt.Sprintf(
		`set -e
sudo mkdir -p %s
sudo install -m 0644 %s %s
sudo rm -f %s`,
		ShellQuote(configDir),
		ShellQuote(tmpRemotePath),
		ShellQuote(cfg.ConfigPath),
		ShellQuote(tmpRemotePath),
	)

	out, err := rwpi.SshClient.Run("sh -c " + ShellQuote(installScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("deploy %s config: %w", cfg.Name, err), out)
	}

	return nil
}

func (rwpi *RemoteWebProxyInstaller) enableAndStartService(serviceName string) error {
	serviceScript := fmt.Sprintf(
		`set -e
sudo systemctl daemon-reload >/dev/null 2>&1 || true
sudo systemctl enable %s >/dev/null 2>&1 || true
if sudo systemctl start %s >/dev/null 2>&1; then
  exit 0
fi
if sudo systemctl restart %s >/dev/null 2>&1; then
  exit 0
fi
systemctl --no-pager --full status %s || true
journalctl -u %s -n 25 --no-pager || true
exit 1`,
		ShellQuote(serviceName),
		ShellQuote(serviceName),
		ShellQuote(serviceName),
		ShellQuote(serviceName),
		ShellQuote(serviceName),
	)

	out, err := rwpi.SshClient.Run("sh -c " + ShellQuote(serviceScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("enable/start %s service: %w", serviceName, err), out)
	}

	return nil
}

func (rwpi *RemoteWebProxyInstaller) restartService(serviceName string) error {
	restartScript := fmt.Sprintf(
		`set -e
sudo systemctl daemon-reload >/dev/null 2>&1 || true
sudo systemctl enable %s >/dev/null 2>&1 || true
if sudo systemctl restart %s >/dev/null 2>&1; then
  exit 0
fi
if sudo systemctl start %s >/dev/null 2>&1; then
  exit 0
fi
systemctl --no-pager --full status %s || true
journalctl -u %s -n 25 --no-pager || true
exit 1`,
		ShellQuote(serviceName),
		ShellQuote(serviceName),
		ShellQuote(serviceName),
		ShellQuote(serviceName),
		ShellQuote(serviceName),
	)

	out, err := rwpi.SshClient.Run("sh -c " + ShellQuote(restartScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("restart %s service: %w", serviceName, err), out)
	}

	return nil
}

func (rwpi *RemoteWebProxyInstaller) verifyService(serviceName string) error {
	verifyScript := fmt.Sprintf(`set -e
sudo systemctl is-active --quiet %s`, ShellQuote(serviceName))
	out, err := rwpi.SshClient.Run("sh -c " + ShellQuote(verifyScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("verify %s service status: %w", serviceName, err), out)
	}
	return nil
}
