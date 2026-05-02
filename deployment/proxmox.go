package deployment

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type proxmoxClient struct {
	baseURL    *url.URL
	httpClient *http.Client
	auth       proxmoxAuthConfig
}

type proxmoxAuthConfig struct {
	TokenID  string
	Secret   string
	Username string
	Password string
	UseToken bool
}

type proxmoxAPIError struct {
	Status int
	Body   string
}

func (e *proxmoxAPIError) Error() string {
	return fmt.Sprintf("proxmox api error: status=%d body=%s", e.Status, e.Body)
}

type proxmoxQemuConfig struct {
	Name        string
	MemoryMB    int
	Sockets     int
	Cores       int
	Description string
	Raw         map[string]string
}

type proxmoxCloneRequest struct {
	NewID       int
	Name        string
	Storage     string
	Description string
	FullClone   bool
}

type proxmoxVMConfigView struct {
	Raw map[string]string
}

func CreateProxmoxLXC(req ProxmoxLXCRequest) (ProxmoxLXCResult, error) {
	if strings.TrimSpace(req.Node) == "" {
		return ProxmoxLXCResult{}, fmt.Errorf("node is required")
	}
	if req.VMID <= 0 {
		return ProxmoxLXCResult{}, fmt.Errorf("vmid must be greater than zero")
	}
	if strings.TrimSpace(req.Hostname) == "" {
		return ProxmoxLXCResult{}, fmt.Errorf("hostname is required")
	}
	if strings.TrimSpace(req.OSTemplate) == "" {
		return ProxmoxLXCResult{}, fmt.Errorf("ostemplate is required")
	}

	client, err := newProxmoxClient(req.Auth)
	if err != nil {
		return ProxmoxLXCResult{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.CreateLXC(ctx, req.Node, req); err != nil {
		return ProxmoxLXCResult{}, fmt.Errorf("create proxmox LXC: %w", err)
	}
	return ProxmoxLXCResult{
		Node:     req.Node,
		VMID:     req.VMID,
		Hostname: req.Hostname,
		Status:   "created",
		Started:  boolValue(req.Start),
		Console:  boolValue(req.Console),
	}, nil
}

func CreateProxmoxVM(req ProxmoxVMCreateRequest) (ProxmoxVMCreateResult, error) {
	if strings.TrimSpace(req.Node) == "" {
		return ProxmoxVMCreateResult{}, fmt.Errorf("node is required")
	}
	if req.VMID <= 0 {
		return ProxmoxVMCreateResult{}, fmt.Errorf("vmid must be greater than zero")
	}
	if req.TemplateVMID <= 0 {
		return ProxmoxVMCreateResult{}, fmt.Errorf("template_vmid must be greater than zero")
	}
	if strings.TrimSpace(req.Name) == "" {
		return ProxmoxVMCreateResult{}, fmt.Errorf("name is required")
	}

	client, err := newProxmoxClient(req.Auth)
	if err != nil {
		return ProxmoxVMCreateResult{}, err
	}
	ctx := context.Background()

	cloudInitConfig, err := buildProxmoxVMCloudInitConfig(req)
	if err != nil {
		return ProxmoxVMCreateResult{}, err
	}
	if strings.TrimSpace(req.CICustomScript) != "" {
		sshReq, err := mergeProxmoxSSHRequest(req.Auth, req.SSH)
		if err != nil {
			return ProxmoxVMCreateResult{}, err
		}
		volid, err := uploadVMCloudInitCustomScript(sshReq, req.Node, req.VMID, req.CISnippetsStorage, req.CICustomScript)
		if err != nil {
			return ProxmoxVMCreateResult{}, err
		}
		if cloudInitConfig.Raw == nil {
			cloudInitConfig.Raw = make(map[string]string)
		}
		cloudInitConfig.Raw["cicustom"] = "user=" + volid
	}

	if err := client.CloneVM(ctx, req.Node, req.TemplateVMID, proxmoxCloneRequest{
		NewID:       req.VMID,
		Name:        req.Name,
		Storage:     req.Storage,
		Description: req.Description,
		FullClone:   boolValue(req.FullClone),
	}); err != nil {
		return ProxmoxVMCreateResult{}, fmt.Errorf("clone proxmox VM template %d: %w", req.TemplateVMID, err)
	}
	if err := waitForVMUnlocked(ctx, client, req.Node, req.VMID, 5*time.Minute); err != nil {
		return ProxmoxVMCreateResult{}, err
	}
	if hasProxmoxVMConfigValues(cloudInitConfig) {
		if err := client.UpdateVMConfig(ctx, req.Node, req.VMID, cloudInitConfig); err != nil {
			return ProxmoxVMCreateResult{}, fmt.Errorf("apply VM config: %w", err)
		}
	}
	if boolValue(req.Start) {
		if _, err := client.StartVM(ctx, req.Node, req.VMID); err != nil {
			return ProxmoxVMCreateResult{}, fmt.Errorf("start proxmox VM: %w", err)
		}
	}

	return ProxmoxVMCreateResult{
		Node:         req.Node,
		VMID:         req.VMID,
		TemplateVMID: req.TemplateVMID,
		Name:         req.Name,
		Status:       "created",
		Started:      boolValue(req.Start),
	}, nil
}

func CreateProxmoxVMTemplate(req ProxmoxVMTemplateRequest) (ProxmoxVMTemplateResult, error) {
	if strings.TrimSpace(req.Node) == "" {
		return ProxmoxVMTemplateResult{}, fmt.Errorf("node is required")
	}
	if req.VMID <= 0 {
		return ProxmoxVMTemplateResult{}, fmt.Errorf("vmid must be greater than zero")
	}
	if strings.TrimSpace(req.Name) == "" {
		return ProxmoxVMTemplateResult{}, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(req.ImageURL) == "" {
		return ProxmoxVMTemplateResult{}, fmt.Errorf("image_url is required")
	}
	if strings.TrimSpace(req.Storage) == "" {
		return ProxmoxVMTemplateResult{}, fmt.Errorf("storage is required")
	}
	if strings.TrimSpace(req.CloudInitStorage) == "" {
		req.CloudInitStorage = req.Storage
	}
	if strings.TrimSpace(req.DiskBus) == "" {
		req.DiskBus = "scsi0"
	}
	if strings.TrimSpace(req.SCSIHW) == "" {
		req.SCSIHW = "virtio-scsi-pci"
	}
	if strings.TrimSpace(req.BootOrder) == "" {
		req.BootOrder = req.DiskBus
	}

	client, err := newProxmoxClient(req.Auth)
	if err != nil {
		return ProxmoxVMTemplateResult{}, err
	}
	ctx := context.Background()

	if err := client.CreateVM(ctx, req.Node, req.VMID, buildProxmoxVMCloudImageBaseConfig(req)); err != nil {
		return ProxmoxVMTemplateResult{}, fmt.Errorf("create VM shell: %w", err)
	}
	if err := waitForVMUnlocked(ctx, client, req.Node, req.VMID, 3*time.Minute); err != nil {
		return ProxmoxVMTemplateResult{}, err
	}

	sshReq, err := mergeProxmoxSSHRequest(req.Auth, req.SSH)
	if err != nil {
		return ProxmoxVMTemplateResult{}, err
	}
	importedVolID, err := importCloudImageDiskOverSSH(sshReq, req)
	if err != nil {
		return ProxmoxVMTemplateResult{}, err
	}

	if err := client.UpdateVMConfig(ctx, req.Node, req.VMID, buildProxmoxVMCloudImageDiskConfig(req, importedVolID)); err != nil {
		return ProxmoxVMTemplateResult{}, fmt.Errorf("attach imported disk: %w", err)
	}
	if err := waitForVMUnlocked(ctx, client, req.Node, req.VMID, 3*time.Minute); err != nil {
		return ProxmoxVMTemplateResult{}, err
	}
	if err := client.TemplateVM(ctx, req.Node, req.VMID); err != nil {
		return ProxmoxVMTemplateResult{}, fmt.Errorf("convert VM to template: %w", err)
	}

	return ProxmoxVMTemplateResult{
		Node:             req.Node,
		VMID:             req.VMID,
		Name:             req.Name,
		ImportedVolumeID: importedVolID,
		Template:         true,
	}, nil
}

func mergeProxmoxSSHRequest(auth ProxmoxAuthOptions, sshReq SSHOptions) (SSHOptions, error) {
	if strings.TrimSpace(sshReq.Host) != "" && strings.TrimSpace(sshReq.User) != "" {
		if sshReq.Port == 0 {
			sshReq.Port = 22
		}
		return sshReq, nil
	}
	if strings.TrimSpace(auth.HostURL) == "" {
		return sshReq, fmt.Errorf("ssh.host and auth.host_url are both empty")
	}
	parsed, err := url.Parse(strings.TrimSpace(auth.HostURL))
	if err != nil {
		return sshReq, fmt.Errorf("parse auth.host_url for SSH defaults: %w", err)
	}
	if strings.TrimSpace(sshReq.Host) == "" {
		sshReq.Host = parsed.Hostname()
	}
	if sshReq.Port == 0 {
		if parsed.Port() != "" {
			if port, convErr := strconv.Atoi(parsed.Port()); convErr == nil && port > 0 {
				sshReq.Port = uint(port)
			}
		}
		if sshReq.Port == 0 {
			sshReq.Port = 22
		}
	}
	if strings.TrimSpace(sshReq.User) == "" {
		sshReq.User = "root"
	}
	return sshReq, nil
}

func buildProxmoxVMCloudInitConfig(req ProxmoxVMCreateRequest) (*proxmoxQemuConfig, error) {
	cfg := &proxmoxQemuConfig{
		Name:        req.Name,
		MemoryMB:    req.MemoryMB,
		Sockets:     req.Sockets,
		Cores:       req.Cores,
		Description: req.Description,
		Raw:         make(map[string]string),
	}
	if strings.TrimSpace(req.CIUser) != "" {
		cfg.Raw["ciuser"] = req.CIUser
	}
	if strings.TrimSpace(req.CIPassword) != "" {
		cfg.Raw["cipassword"] = req.CIPassword
	}
	if len(req.SshPublicKeys) > 0 {
		cfg.Raw["sshkeys"] = strings.Join(req.SshPublicKeys, "\n")
	}
	if req.BallooningDevice != nil {
		if !*req.BallooningDevice {
			cfg.Raw["balloon"] = "0"
		} else if req.MinimumMemoryMB > 0 {
			cfg.Raw["balloon"] = strconv.Itoa(req.MinimumMemoryMB)
		}
	} else if req.MinimumMemoryMB > 0 {
		cfg.Raw["balloon"] = strconv.Itoa(req.MinimumMemoryMB)
	}
	if req.Shares > 0 {
		cfg.Raw["shares"] = strconv.Itoa(req.Shares)
	}
	if strings.TrimSpace(req.Bios) != "" {
		cfg.Raw["bios"] = req.Bios
	}
	if strings.TrimSpace(req.Machine) != "" {
		cfg.Raw["machine"] = req.Machine
	}
	if strings.TrimSpace(req.SCSIHW) != "" {
		cfg.Raw["scsihw"] = req.SCSIHW
	}
	if strings.TrimSpace(req.Boot) != "" {
		cfg.Raw["boot"] = req.Boot
	}
	if req.AgentEnabled != nil {
		if *req.AgentEnabled {
			cfg.Raw["agent"] = "enabled=1"
		} else {
			cfg.Raw["agent"] = "0"
		}
	}
	if strings.TrimSpace(req.Net0) != "" {
		cfg.Raw["net0"] = req.Net0
	}
	if strings.TrimSpace(req.IPConfig0) != "" {
		cfg.Raw["ipconfig0"] = req.IPConfig0
	}
	if strings.TrimSpace(req.Nameserver) != "" {
		cfg.Raw["nameserver"] = req.Nameserver
	}
	if strings.TrimSpace(req.SearchDomain) != "" {
		cfg.Raw["searchdomain"] = req.SearchDomain
	}
	if len(cfg.Raw) == 0 {
		cfg.Raw = nil
	}
	return cfg, nil
}

func buildProxmoxVMCloudImageBaseConfig(req ProxmoxVMTemplateRequest) *proxmoxQemuConfig {
	cfg := &proxmoxQemuConfig{
		Name:        req.Name,
		MemoryMB:    req.MemoryMB,
		Sockets:     req.Sockets,
		Cores:       req.Cores,
		Description: req.Description,
		Raw: map[string]string{
			"scsihw": req.SCSIHW,
			"net0":   req.Net0,
		},
	}
	if boolValue(req.Agent) {
		cfg.Raw["agent"] = "enabled=1"
	}
	if boolValue(req.SerialConsole) {
		cfg.Raw["serial0"] = "socket"
		cfg.Raw["vga"] = "serial0"
	}
	return cfg
}

func buildProxmoxVMCloudImageDiskConfig(req ProxmoxVMTemplateRequest, importedVolID string) *proxmoxQemuConfig {
	return &proxmoxQemuConfig{Raw: map[string]string{
		req.DiskBus: importedVolID,
		"ide2":      fmt.Sprintf("%s:cloudinit", req.CloudInitStorage),
		"boot":      "order=" + req.BootOrder,
	}}
}

func uploadVMCloudInitCustomScript(sshReq SSHOptions, node string, vmid int, storage string, script string) (string, error) {
	if strings.TrimSpace(storage) == "" {
		storage = "local"
	}
	filename := fmt.Sprintf("infractl-vm-%d-user-data.yaml", vmid)
	volid := fmt.Sprintf("%s:snippets/%s", storage, filename)
	cloudConfig := vmCloudInitUserDataFromScript(script)

	sshOpts, cleanupSSHKey, err := PrepareSSHOptions(sshReq)
	if err != nil {
		return "", err
	}
	defer cleanupSSHKey()
	sshClient, err := InitializeSshClient(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return "", fmt.Errorf("initialize SSH to upload cloud-init snippet: %w", err)
	}
	defer sshClient.Close()

	encoded := base64.StdEncoding.EncodeToString([]byte(cloudConfig))
	uploadScript := `set -eu
volid=` + ShellQuote(volid) + `
path="$(pvesm path "$volid" 2>/dev/null || true)"
if [ -z "$path" ] && [ "${volid%%:*}" = "local" ]; then
  path="/var/lib/vz/${volid#*:}"
fi
if [ -z "$path" ]; then
  echo "Unable to resolve snippet path for $volid. Ensure the storage supports snippets." >&2
  exit 1
fi
mkdir -p "$(dirname "$path")"
printf %s ` + ShellQuote(encoded) + ` | base64 -d > "$path"
chmod 0644 "$path"`
	out, err := sshClient.Run("sh -c " + ShellQuote(uploadScript))
	if err != nil {
		return "", formatRemoteCommandError(fmt.Errorf("upload cloud-init snippet on node %s: %w", node, err), out)
	}
	return volid, nil
}

func vmCloudInitUserDataFromScript(script string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(script))
	return `#cloud-config
write_files:
  - path: /var/lib/infractl/custom-init.sh
    permissions: '0700'
    encoding: b64
    content: ` + encoded + `
runcmd:
  - [ /bin/sh, /var/lib/infractl/custom-init.sh ]
`
}

func importCloudImageDiskOverSSH(sshReq SSHOptions, req ProxmoxVMTemplateRequest) (string, error) {
	sshOpts, cleanupSSHKey, err := PrepareSSHOptions(sshReq)
	if err != nil {
		return "", err
	}
	defer cleanupSSHKey()
	sshClient, err := InitializeSshClient(sshOpts.Host, sshOpts.User, sshOpts.KeyPath, sshOpts.Passphrase, sshOpts.UseAgent, sshOpts.Port)
	if err != nil {
		return "", fmt.Errorf("initialize SSH for cloud image import: %w", err)
	}
	defer sshClient.Close()
	out, err := sshClient.Run("sh -c " + ShellQuote(renderVMCloudImageImportScript(req)))
	if err != nil {
		return "", FormatExecError(err, out)
	}
	volid := parseCloudImageImportVolID(string(out))
	if volid == "" {
		return "", fmt.Errorf("cloud image import completed but no imported volume ID was detected in output: %s", strings.TrimSpace(string(out)))
	}
	return volid, nil
}

func renderVMCloudImageImportScript(req ProxmoxVMTemplateRequest) string {
	cleanup := "false"
	if boolValue(req.CleanupImage) {
		cleanup = "true"
	}
	return `set -eu
vmid=` + ShellQuote(strconv.Itoa(req.VMID)) + `
image_url=` + ShellQuote(req.ImageURL) + `
storage=` + ShellQuote(req.Storage) + `
filename=` + ShellQuote(cloudImageFilename(req.ImageURL, req.VMID)) + `
cleanup_image=` + cleanup + `
workdir="$(mktemp -d /var/tmp/infractl-cloud-image.XXXXXX)"
cleanup() {
  if [ "$cleanup_image" = "true" ]; then
    rm -rf "$workdir"
  else
    echo "IMAGE_PATH=$workdir/$filename"
  fi
}
trap cleanup EXIT
image_path="$workdir/$filename"
if command -v curl >/dev/null 2>&1; then
  curl -fL --retry 3 --connect-timeout 20 -o "$image_path" "$image_url"
elif command -v wget >/dev/null 2>&1; then
  wget -O "$image_path" "$image_url"
else
  echo "curl or wget is required on the Proxmox node to download cloud images" >&2
  exit 1
fi
before="$(qm config "$vmid" | sed -n 's/^\(unused[0-9][0-9]*\):.*/\1/p' | sort | tr '\n' ' ')"
qm importdisk "$vmid" "$image_path" "$storage"
after="$(qm config "$vmid")"
imported="$(printf '%s\n' "$after" | awk -v before="$before" '
  BEGIN { split(before, existing, " "); for (i in existing) seen[existing[i]]=1 }
  /^unused[0-9]+:/ {
    key=$1
    sub(/:$/, "", key)
    if (!seen[key]) {
      sub(/^[^:]+:[[:space:]]*/, "", $0)
      print $0
      exit
    }
  }
')"
if [ -z "$imported" ]; then
  imported="$(printf '%s\n' "$after" | awk '/^unused[0-9]+:/ { sub(/^[^:]+:[[:space:]]*/, "", $0); value=$0 } END { print value }')"
fi
if [ -z "$imported" ]; then
  echo "Unable to determine imported disk volume from qm config output" >&2
  exit 1
fi
echo "IMPORTED_VOLID=$imported"`
}

func cloudImageFilename(rawURL string, vmid int) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err == nil && strings.TrimSpace(parsed.Path) != "" {
		filename := path.Base(parsed.Path)
		if filename != "." && filename != "/" && filename != "" {
			return filename
		}
	}
	return fmt.Sprintf("cloud-image-%d.img", vmid)
}

func parseCloudImageImportVolID(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "IMPORTED_VOLID=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "IMPORTED_VOLID="))
		}
	}
	return ""
}

func hasProxmoxVMConfigValues(cfg *proxmoxQemuConfig) bool {
	return cfg != nil &&
		(cfg.Name != "" ||
			cfg.MemoryMB > 0 ||
			cfg.Sockets > 0 ||
			cfg.Cores > 0 ||
			cfg.Description != "" ||
			len(cfg.Raw) > 0)
}

func waitForVMUnlocked(ctx context.Context, client *proxmoxClient, node string, vmid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		cfg, err := client.GetVMConfig(ctx, node, vmid)
		if err == nil {
			if cfg.Raw == nil || strings.TrimSpace(cfg.Raw["lock"]) == "" {
				return nil
			}
			lastErr = fmt.Errorf("VM %d is locked: %s", vmid, cfg.Raw["lock"])
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("timed out waiting for VM %d to unlock: %w", vmid, lastErr)
			}
			return fmt.Errorf("timed out waiting for VM %d to unlock", vmid)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func newProxmoxClient(auth ProxmoxAuthOptions) (*proxmoxClient, error) {
	hostURL := strings.TrimSpace(auth.HostURL)
	if hostURL == "" {
		return nil, fmt.Errorf("auth.host_url is required")
	}
	parsedURL, err := url.Parse(strings.TrimRight(hostURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid auth.host_url: %w", err)
	}
	useToken := true
	if auth.UseToken != nil {
		useToken = *auth.UseToken
	}
	cfg := proxmoxAuthConfig{UseToken: useToken}
	if useToken {
		cfg.TokenID = strings.TrimSpace(auth.APITokenID)
		cfg.Secret = strings.TrimSpace(auth.APISecret)
		if strings.TrimSpace(auth.APIToken) != "" {
			tokenID, secret, err := parseAPIToken(strings.TrimSpace(auth.APIToken))
			if err != nil {
				return nil, err
			}
			cfg.TokenID = tokenID
			cfg.Secret = secret
		}
		if cfg.TokenID == "" {
			return nil, fmt.Errorf("auth.api_token_id is required")
		}
		if cfg.Secret == "" {
			return nil, fmt.Errorf("auth.api_secret is required")
		}
	} else {
		return nil, fmt.Errorf("password authentication is not supported yet")
	}
	return &proxmoxClient{
		baseURL: parsedURL,
		auth:    cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: insecureTLSConfig(auth),
			},
		},
	}, nil
}

func insecureTLSConfig(auth ProxmoxAuthOptions) *tls.Config {
	skipTLS := true
	if auth.SkipTLS != nil {
		skipTLS = *auth.SkipTLS
	}
	return &tls.Config{InsecureSkipVerify: skipTLS}
}

func (c *proxmoxClient) do(ctx context.Context, method string, path string, body url.Values, out interface{}) error {
	fullURL := *c.baseURL
	fullURL.Path = strings.TrimRight(c.baseURL.Path, "/") + path

	var reader io.Reader
	if body != nil {
		reader = bytes.NewBufferString(body.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", "PVEAPIToken="+c.auth.TokenID+"="+c.auth.Secret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read proxmox response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return &proxmoxAPIError{Status: resp.StatusCode, Body: string(bodyBytes)}
	}
	if out == nil || len(bodyBytes) == 0 {
		return nil
	}

	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &wrapper); err != nil {
		return fmt.Errorf("decode proxmox response: %w", err)
	}
	if len(wrapper.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(wrapper.Data, out); err != nil {
		return fmt.Errorf("decode proxmox data: %w", err)
	}
	return nil
}

func (c *proxmoxClient) CreateLXC(ctx context.Context, node string, req ProxmoxLXCRequest) error {
	params := url.Values{}
	params.Set("vmid", strconv.Itoa(req.VMID))
	params.Set("hostname", req.Hostname)
	if strings.TrimSpace(req.Password) != "" {
		params.Set("password", req.Password)
	}
	if strings.TrimSpace(req.OSTemplate) != "" {
		params.Set("ostemplate", req.OSTemplate)
	}
	if len(req.SshPublicKeys) > 0 {
		params.Set("ssh-public-keys", strings.Join(req.SshPublicKeys, "\n"))
	}
	if strings.TrimSpace(req.Storage) != "" {
		params.Set("storage", req.Storage)
	}
	if strings.TrimSpace(req.RootFSSize) != "" && strings.TrimSpace(req.Storage) != "" {
		params.Set("rootfs", fmt.Sprintf("%s:%s", req.Storage, req.RootFSSize))
	}
	if req.Memory > 0 {
		params.Set("memory", strconv.Itoa(req.Memory))
	}
	if req.Swap > 0 {
		params.Set("swap", strconv.Itoa(req.Swap))
	}
	if req.Cores > 0 {
		params.Set("cores", strconv.Itoa(req.Cores))
	}
	if req.CPULimit > 0 {
		params.Set("cpulimit", strconv.Itoa(req.CPULimit))
	}
	if req.CPUUnits > 0 {
		params.Set("cpuunits", strconv.Itoa(req.CPUUnits))
	}
	if strings.TrimSpace(req.Net0) != "" {
		params.Set("net0", req.Net0)
	}
	if strings.TrimSpace(req.Arch) != "" {
		params.Set("arch", req.Arch)
	}
	if strings.TrimSpace(req.Cmode) != "" {
		params.Set("cmode", req.Cmode)
	}
	if strings.TrimSpace(req.Features) != "" {
		params.Set("features", req.Features)
	}
	if strings.TrimSpace(req.Nameserver) != "" {
		params.Set("nameserver", req.Nameserver)
	}
	if strings.TrimSpace(req.SearchDomain) != "" {
		params.Set("searchdomain", req.SearchDomain)
	}
	if strings.TrimSpace(req.Description) != "" {
		params.Set("description", req.Description)
	}
	if req.Unprivileged != nil {
		params.Set("unprivileged", proxmoxBoolFlag(*req.Unprivileged))
	}
	if req.Start != nil {
		params.Set("start", proxmoxBoolFlag(*req.Start))
	}
	if req.Console != nil {
		params.Set("console", proxmoxBoolFlag(*req.Console))
	}
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/lxc", url.PathEscape(node)), params, nil)
}

func (c *proxmoxClient) CloneVM(ctx context.Context, node string, templateVMID int, req proxmoxCloneRequest) error {
	params := url.Values{}
	params.Set("newid", strconv.Itoa(req.NewID))
	if strings.TrimSpace(req.Name) != "" {
		params.Set("name", req.Name)
	}
	if strings.TrimSpace(req.Storage) != "" {
		params.Set("storage", req.Storage)
	}
	if strings.TrimSpace(req.Description) != "" {
		params.Set("description", req.Description)
	}
	params.Set("full", proxmoxBoolFlag(req.FullClone))
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/clone", url.PathEscape(node), templateVMID), params, nil)
}

func (c *proxmoxClient) CreateVM(ctx context.Context, node string, vmid int, cfg *proxmoxQemuConfig) error {
	params := cfg.toValues()
	params.Set("vmid", strconv.Itoa(vmid))
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/qemu", url.PathEscape(node)), params, nil)
}

func (c *proxmoxClient) UpdateVMConfig(ctx context.Context, node string, vmid int, cfg *proxmoxQemuConfig) error {
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", url.PathEscape(node), vmid), cfg.toValues(), nil)
}

func (c *proxmoxClient) GetVMConfig(ctx context.Context, node string, vmid int) (*proxmoxVMConfigView, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/config", url.PathEscape(node), vmid), nil, &raw); err != nil {
		return nil, err
	}
	view := &proxmoxVMConfigView{Raw: make(map[string]string, len(raw))}
	for k, v := range raw {
		view.Raw[k] = fmt.Sprintf("%v", v)
	}
	return view, nil
}

func (c *proxmoxClient) StartVM(ctx context.Context, node string, vmid int) (string, error) {
	var upid string
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/status/start", url.PathEscape(node), vmid), nil, &upid); err != nil {
		return "", err
	}
	return upid, nil
}

func (c *proxmoxClient) TemplateVM(ctx context.Context, node string, vmid int) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/api2/json/nodes/%s/qemu/%d/template", url.PathEscape(node), vmid), nil, nil)
}

func (cfg *proxmoxQemuConfig) toValues() url.Values {
	params := url.Values{}
	if cfg == nil {
		return params
	}
	if strings.TrimSpace(cfg.Name) != "" {
		params.Set("name", cfg.Name)
	}
	if cfg.MemoryMB > 0 {
		params.Set("memory", strconv.Itoa(cfg.MemoryMB))
	}
	if cfg.Sockets > 0 {
		params.Set("sockets", strconv.Itoa(cfg.Sockets))
	}
	if cfg.Cores > 0 {
		params.Set("cores", strconv.Itoa(cfg.Cores))
	}
	if strings.TrimSpace(cfg.Description) != "" {
		params.Set("description", cfg.Description)
	}
	for k, v := range cfg.Raw {
		if strings.TrimSpace(v) != "" {
			params.Set(k, v)
		}
	}
	return params
}

func proxmoxBoolFlag(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func parseAPIToken(apiToken string) (string, string, error) {
	tokenID, secret, ok := strings.Cut(strings.TrimSpace(apiToken), "=")
	if !ok || tokenID == "" || secret == "" {
		return "", "", fmt.Errorf("invalid Proxmox API token format; expected USER@REALM!TOKENID=SECRET")
	}
	return tokenID, secret, nil
}
