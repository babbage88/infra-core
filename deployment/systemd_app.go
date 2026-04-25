package deployment

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/babbage88/goph/v2"
)

func DefaultSystemdAppDeployRequest() SystemdAppDeployRequest {
	return SystemdAppDeployRequest{
		ServiceUID: 8888,
		SystemdDir: "/etc/systemd/system",
		SourceDir:  ".",
	}
}

func MergeSystemdAppDeployDefaults(req, defaults SystemdAppDeployRequest) SystemdAppDeployRequest {
	if strings.TrimSpace(req.AppName) == "" {
		req.AppName = defaults.AppName
	}
	if req.EnvVars == nil {
		req.EnvVars = defaults.EnvVars
	}
	if strings.TrimSpace(req.ServiceUser) == "" {
		req.ServiceUser = defaults.ServiceUser
	}
	if req.ServiceUID == 0 {
		req.ServiceUID = defaults.ServiceUID
	}
	if strings.TrimSpace(req.DestinationBinary) == "" {
		req.DestinationBinary = defaults.DestinationBinary
	}
	if strings.TrimSpace(req.InstallDir) == "" {
		req.InstallDir = defaults.InstallDir
	}
	if strings.TrimSpace(req.SystemdDir) == "" {
		req.SystemdDir = defaults.SystemdDir
	}
	if strings.TrimSpace(req.SourceDir) == "" {
		req.SourceDir = defaults.SourceDir
	}
	if strings.TrimSpace(req.SourcePackage) == "" {
		req.SourcePackage = defaults.SourcePackage
	}
	req.SSH = MergeSSHDefaults(req.SSH, defaults.SSH)
	return req
}

func DeploySystemdApp(req SystemdAppDeployRequest) (SystemdAppDeployResult, error) {
	if strings.TrimSpace(req.SSH.Host) == "" {
		return SystemdAppDeployResult{}, fmt.Errorf("ssh.host is required")
	}
	if strings.TrimSpace(req.SSH.User) == "" {
		return SystemdAppDeployResult{}, fmt.Errorf("ssh.user is required")
	}
	if strings.TrimSpace(req.AppName) == "" {
		return SystemdAppDeployResult{}, fmt.Errorf("app_name is required")
	}
	if req.ServiceUID <= 0 {
		return SystemdAppDeployResult{}, fmt.Errorf("service_uid must be greater than zero")
	}
	if strings.TrimSpace(req.ServiceUser) == "" {
		req.ServiceUser = req.AppName
	}
	if strings.TrimSpace(req.DestinationBinary) == "" {
		req.DestinationBinary = req.AppName
	}
	if strings.TrimSpace(req.InstallDir) == "" {
		req.InstallDir = filepath.ToSlash(filepath.Join("/opt", req.AppName))
	}
	if strings.TrimSpace(req.SystemdDir) == "" {
		req.SystemdDir = "/etc/systemd/system"
	}
	if strings.TrimSpace(req.SourcePackage) == "" {
		req.SourcePackage = "."
	}

	sshOpts, cleanupSSHKey, err := PrepareSSHOptions(req.SSH)
	if err != nil {
		return SystemdAppDeployResult{}, err
	}
	defer cleanupSSHKey()

	sshClient, err := InitializeSshClient(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return SystemdAppDeployResult{}, fmt.Errorf("initialize SSH client: %w", err)
	}
	defer sshClient.Close()

	binaryPath, sourceDesc, cleanupBinary, err := prepareSystemdSourceBinary(sshClient, req)
	if err != nil {
		return SystemdAppDeployResult{}, err
	}
	if cleanupBinary != nil {
		defer cleanupBinary()
	}

	if err := ensureRemoteSystemdServiceUser(sshClient, req.ServiceUser, req.ServiceUID, req.InstallDir); err != nil {
		return SystemdAppDeployResult{}, err
	}
	if err := ensureRemoteDirectories(sshClient, req.InstallDir, req.SystemdDir); err != nil {
		return SystemdAppDeployResult{}, err
	}

	remoteBinaryPath := filepath.ToSlash(filepath.Join(req.InstallDir, req.DestinationBinary))
	if err := uploadRemoteFileWithInstall(sshClient, binaryPath, remoteBinaryPath, "0755"); err != nil {
		return SystemdAppDeployResult{}, err
	}
	if len(req.EnvVars) > 0 {
		envFile, cleanup, err := writeTempContentFile(req.AppName+"-env", renderEnvFile(req.EnvVars))
		if err != nil {
			return SystemdAppDeployResult{}, err
		}
		defer cleanup()
		if err := uploadRemoteFileWithInstall(sshClient, envFile, fmt.Sprintf("/etc/%s.env", req.AppName), "0600"); err != nil {
			return SystemdAppDeployResult{}, err
		}
	}

	unitFile, cleanup, err := writeTempContentFile(req.AppName+".service", renderSystemdUnitFile(req))
	if err != nil {
		return SystemdAppDeployResult{}, err
	}
	defer cleanup()
	unitPath := filepath.ToSlash(filepath.Join(req.SystemdDir, req.AppName+".service"))
	if err := uploadRemoteFileWithInstall(sshClient, unitFile, unitPath, "0644"); err != nil {
		return SystemdAppDeployResult{}, err
	}

	restartScript := fmt.Sprintf(
		`set -e
sudo chown -R %s:%s %s
sudo systemctl daemon-reload
sudo systemctl enable %s
sudo systemctl restart %s
sudo systemctl is-active --quiet %s`,
		ShellQuote(req.ServiceUser),
		ShellQuote(req.ServiceUser),
		ShellQuote(req.InstallDir),
		ShellQuote(req.AppName),
		ShellQuote(req.AppName),
		ShellQuote(req.AppName),
	)
	if out, err := sshClient.Run("sh -c " + ShellQuote(restartScript)); err != nil {
		return SystemdAppDeployResult{}, formatRemoteCommandError(fmt.Errorf("restart systemd service: %w", err), out)
	}

	return SystemdAppDeployResult{
		Host:         sshOpts.Host,
		AppName:      req.AppName,
		ServiceName:  req.AppName,
		ServiceUser:  req.ServiceUser,
		InstallDir:   req.InstallDir,
		BinaryPath:   remoteBinaryPath,
		SystemdUnit:  unitPath,
		SourceBinary: sourceDesc,
	}, nil
}

func prepareSystemdSourceBinary(sshClient *goph.Client, req SystemdAppDeployRequest) (string, string, func(), error) {
	remoteGOOS, remoteGOARCH, err := detectRemotePlatform(sshClient)
	if err != nil {
		return "", "", nil, fmt.Errorf("detect remote platform: %w", err)
	}
	switch {
	case strings.TrimSpace(req.SourceBin) != "":
		path := resolveSystemdSourceBinPath(req.SourceDir, req.SourceBin)
		return path, path, nil, nil
	case strings.TrimSpace(req.SourceGoModule) != "":
		path, cleanup, err := buildGoBinaryInDir(expandLocalPath(req.SourceGoModule), req.SourcePackage, req.AppName, remoteGOOS, remoteGOARCH)
		return path, path, cleanup, err
	case strings.TrimSpace(req.SourceRepo) != "":
		repoDir, cleanupRepo, err := cloneSourceRepo(req.SourceRepo, req.SourceRef)
		if err != nil {
			return "", "", nil, err
		}
		path, cleanupBinary, err := buildGoBinaryInDir(repoDir, req.SourcePackage, req.AppName, remoteGOOS, remoteGOARCH)
		if err != nil {
			cleanupRepo()
			return "", "", nil, err
		}
		return path, path, func() {
			if cleanupBinary != nil {
				cleanupBinary()
			}
			cleanupRepo()
		}, nil
	default:
		return "", "", nil, fmt.Errorf("one of source_bin, source_go_module, or source_repo is required")
	}
}

func detectRemotePlatform(sshClient *goph.Client) (string, string, error) {
	out, err := sshClient.Run(`sh -c 'printf "%s\n%s\n" "$(uname -s)" "$(uname -m)"'`)
	if err != nil {
		return "", "", FormatExecError(err, out)
	}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	values := make([]string, 0, 2)
	for scanner.Scan() {
		values = append(values, strings.TrimSpace(scanner.Text()))
	}
	if len(values) < 2 {
		return "", "", fmt.Errorf("unexpected uname output: %q", strings.TrimSpace(string(out)))
	}
	goos := strings.ToLower(values[0])
	goarch := normalizeGoArch(values[1])
	return goos, goarch, nil
}

func normalizeGoArch(arch string) string {
	switch strings.ToLower(strings.TrimSpace(arch)) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(arch))
	}
}

func resolveSystemdSourceBinPath(sourceDir, sourceBin string) string {
	sourceBin = expandLocalPath(sourceBin)
	if filepath.IsAbs(sourceBin) {
		return sourceBin
	}
	baseDir := expandLocalPath(sourceDir)
	if strings.TrimSpace(baseDir) == "" {
		baseDir = "."
	}
	return filepath.Clean(filepath.Join(baseDir, sourceBin))
}

func buildGoBinaryInDir(moduleDir, sourcePackage, appName, goos, goarch string) (string, func(), error) {
	info, err := os.Stat(moduleDir)
	if err != nil {
		return "", nil, fmt.Errorf("stat go module dir %q: %w", moduleDir, err)
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("source go module %q is not a directory", moduleDir)
	}
	tempDir, err := os.MkdirTemp("", "infractl-systemd-build-*")
	if err != nil {
		return "", nil, fmt.Errorf("create build temp dir: %w", err)
	}
	outputPath := filepath.Join(tempDir, appName)
	args := []string{"build", "-o", outputPath}
	if strings.TrimSpace(sourcePackage) == "" {
		sourcePackage = "."
	}
	args = append(args, sourcePackage)
	cmd := exec.Command("go", args...)
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", nil, fmt.Errorf("go build failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return outputPath, func() { _ = os.RemoveAll(tempDir) }, nil
}

func cloneSourceRepo(repoURL, sourceRef string) (string, func(), error) {
	repoDir, err := os.MkdirTemp("", "infractl-source-repo-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp directory for source repo: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(repoDir) }
	cloneCmd := exec.Command("git", "clone", repoURL, repoDir)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("clone source repo %q failed: %w: %s", repoURL, err, strings.TrimSpace(string(out)))
	}
	if strings.TrimSpace(sourceRef) != "" {
		checkoutCmd := exec.Command("git", "checkout", sourceRef)
		checkoutCmd.Dir = repoDir
		if out, err := checkoutCmd.CombinedOutput(); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("checkout source ref %q failed: %w: %s", sourceRef, err, strings.TrimSpace(string(out)))
		}
	}
	return repoDir, cleanup, nil
}

func ensureRemoteSystemdServiceUser(sshClient *goph.Client, serviceUser string, serviceUID int64, installDir string) error {
	script := fmt.Sprintf(
		`set -e
if ! getent group %s >/dev/null 2>&1; then
  sudo groupadd --system %s
fi
if ! id -u %s >/dev/null 2>&1; then
  sudo useradd --system --uid %d --gid %s --home-dir %s --create-home --shell /usr/sbin/nologin %s
fi`,
		ShellQuote(serviceUser),
		ShellQuote(serviceUser),
		ShellQuote(serviceUser),
		serviceUID,
		ShellQuote(serviceUser),
		ShellQuote(installDir),
		ShellQuote(serviceUser),
	)
	if out, err := sshClient.Run("sh -c " + ShellQuote(script)); err != nil {
		return formatRemoteCommandError(fmt.Errorf("ensure remote systemd service user: %w", err), out)
	}
	return nil
}

func ensureRemoteDirectories(sshClient *goph.Client, installDir, systemdDir string) error {
	script := fmt.Sprintf(`sudo mkdir -p %s %s`, ShellQuote(installDir), ShellQuote(systemdDir))
	if out, err := sshClient.Run("sh -c " + ShellQuote(script)); err != nil {
		return formatRemoteCommandError(fmt.Errorf("create remote directories: %w", err), out)
	}
	return nil
}

func uploadRemoteFileWithInstall(sshClient *goph.Client, localPath, remotePath, mode string) error {
	tmpRemotePath := filepath.ToSlash(filepath.Join("/tmp", fmt.Sprintf("infractl-upload-%d", os.Getpid())))
	if err := sshClient.Upload(localPath, tmpRemotePath); err != nil {
		return fmt.Errorf("upload %q to remote host: %w", localPath, err)
	}
	script := fmt.Sprintf(
		`set -e
sudo mkdir -p %s
sudo install -m %s %s %s
rm -f %s`,
		ShellQuote(filepath.ToSlash(filepath.Dir(remotePath))),
		ShellQuote(mode),
		ShellQuote(tmpRemotePath),
		ShellQuote(remotePath),
		ShellQuote(tmpRemotePath),
	)
	if out, err := sshClient.Run("sh -c " + ShellQuote(script)); err != nil {
		return formatRemoteCommandError(fmt.Errorf("install remote file %q: %w", remotePath, err), out)
	}
	return nil
}

func writeTempContentFile(name, content string) (string, func(), error) {
	file, err := os.CreateTemp("", name+"-*")
	if err != nil {
		return "", nil, err
	}
	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", nil, err
	}
	return file.Name(), func() { _ = os.Remove(file.Name()) }, nil
}

func renderEnvFile(envVars map[string]string) string {
	keys := make([]string, 0, len(envVars))
	for key := range envVars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(envVars[key])
		builder.WriteString("\n")
	}
	return builder.String()
}

func renderSystemdUnitFile(req SystemdAppDeployRequest) string {
	envFile := ""
	if len(req.EnvVars) > 0 {
		envFile = fmt.Sprintf("EnvironmentFile=-/etc/%s.env\n", req.AppName)
	}
	return fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
User=%s
Group=%s
WorkingDirectory=%s
%sExecStart=%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`,
		req.AppName,
		req.ServiceUser,
		req.ServiceUser,
		req.InstallDir,
		envFile,
		filepath.ToSlash(filepath.Join(req.InstallDir, req.DestinationBinary)),
	)
}
