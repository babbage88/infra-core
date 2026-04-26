package proxmox

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

func CreateLXCContainer(host, token, node string, params map[string]string) error {
	client, err := NewClientTokenString(host, token, true)
	if err != nil {
		return err
	}

	cfg := &LxcContainer{Node: node}
	for k, values := range params {
		switch k {
		case "vmid":
			fmt.Sscanf(values, "%d", &cfg.VmId)
		case "hostname":
			cfg.Hostname = values
		case "password":
			cfg.Password = values
		case "ostemplate":
			cfg.OsTemplate = values
		case "storage":
			cfg.Storage = values
		case "memory":
			fmt.Sscanf(values, "%d", &cfg.Memory)
		case "swap":
			fmt.Sscanf(values, "%d", &cfg.Swap)
		case "cores":
			fmt.Sscanf(values, "%d", &cfg.Cores)
		case "cpulimit":
			fmt.Sscanf(values, "%d", &cfg.CpuLimit)
		case "cpuunits":
			fmt.Sscanf(values, "%d", &cfg.CpuUnits)
		case "net0":
			cfg.Net0 = values
		case "arch":
			cfg.Arch = values
		case "cmode":
			cfg.Cmode = values
		case "start":
			cfg.Start = values
		case "console":
			cfg.Console = values
		case "unprivileged":
			cfg.Unprivileged = values
		case "features":
			cfg.Features = values
		case "ssh-public-keys":
			cfg.SshPublicKeys = strings.Split(values, "\n")
		case "rootfs":
			if idx := strings.Index(values, ":"); idx >= 0 && idx < len(values)-1 {
				cfg.RootFsSize = values[idx+1:]
			}
		}
	}

	return client.CreateLXCContainer(context.Background(), node, cfg)
}

func NewClientTokenString(base, apiToken string, tlsCfg bool) (*Client, error) {
	tokenID, secret, err := ParseAPIToken(apiToken)
	if err != nil {
		return nil, err
	}

	return NewClientToken(base, tokenID, secret, tlsCfg)
}

func ParseAPIToken(apiToken string) (string, string, error) {
	tokenID, secret, ok := strings.Cut(strings.TrimSpace(apiToken), "=")
	if !ok || tokenID == "" || secret == "" {
		return "", "", fmt.Errorf("invalid Proxmox API token format; expected USER@REALM!TOKENID=SECRET")
	}

	return tokenID, secret, nil
}

func (c *Client) CreateLXCContainer(ctx context.Context, node string, cfg *LxcContainer) error {
	if cfg == nil {
		return fmt.Errorf("LXC config cannot be nil")
	}
	if strings.TrimSpace(node) == "" {
		return fmt.Errorf("node is required")
	}

	form := url.Values{}
	for k, v := range cfg.ToFormParams() {
		form.Set(k, v)
	}

	path := fmt.Sprintf("%s/%s/lxc", apiNodesPath, url.PathEscape(node))
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}

	if err := c.do(ctx, http.MethodPost, path, strings.NewReader(form.Encode()), headers, true, nil); err != nil {
		return err
	}

	log.Println("Container created successfully")
	return nil
}

func (c *Client) ListLxcContainers(ctx context.Context, node string) ([]LxcContainer, error) {
	path := fmt.Sprintf("%s/%s/lxc", apiNodesPath, url.PathEscape(node))

	var containers []LxcContainer
	if err := c.do(ctx, http.MethodGet, path, nil, nil, false, &containers); err != nil {
		return nil, err
	}

	slices.SortFunc(containers, func(a, b LxcContainer) int {
		switch {
		case a.VmId < b.VmId:
			return -1
		case a.VmId > b.VmId:
			return 1
		default:
			return 0
		}
	})

	return containers, nil
}

func (c *Client) StartLXCContainer(ctx context.Context, node string, vmid int) (string, error) {
	path := fmt.Sprintf("%s/%s/lxc/%d%s", apiNodesPath, url.PathEscape(node), vmid, apiVmStartSubPath)

	var upid string
	if err := c.do(ctx, http.MethodPost, path, nil, nil, false, &upid); err != nil {
		return "", err
	}

	return upid, nil
}

func (c *Client) StopLXCContainer(ctx context.Context, node string, vmid int) (string, error) {
	path := fmt.Sprintf("%s/%s/lxc/%d%s", apiNodesPath, url.PathEscape(node), vmid, apiVmStopSubPath)

	var upid string
	if err := c.do(ctx, http.MethodPost, path, nil, nil, false, &upid); err != nil {
		return "", err
	}

	return upid, nil
}

func (c *Client) DeleteLXCContainer(ctx context.Context, node string, vmid int) (string, error) {
	path := fmt.Sprintf("%s/%s/lxc/%d", apiNodesPath, url.PathEscape(node), vmid)

	var upid string
	if err := c.do(ctx, http.MethodDelete, path, nil, nil, true, &upid); err != nil {
		return "", err
	}

	return upid, nil
}

func (c *Client) GetLXCConfig(ctx context.Context, node string, vmid int) (map[string]string, error) {
	path := fmt.Sprintf("%s/%s/lxc/%d/config", apiNodesPath, url.PathEscape(node), vmid)

	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, path, nil, nil, false, &raw); err != nil {
		return nil, err
	}

	cfg := make(map[string]string, len(raw))
	for k, v := range raw {
		cfg[k] = fmt.Sprintf("%v", v)
	}
	return cfg, nil
}

func (c *Client) UpdateLXCConfig(ctx context.Context, node string, vmid int, params map[string]string) error {
	form := url.Values{}
	for key, value := range params {
		if strings.TrimSpace(value) != "" {
			form.Set(key, value)
		}
	}

	path := fmt.Sprintf("%s/%s/lxc/%d/config", apiNodesPath, url.PathEscape(node), vmid)
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	}

	return c.do(ctx, http.MethodPut, path, strings.NewReader(form.Encode()), headers, true, nil)
}

func ParseLXCConfig(raw map[string]string, vmid int) *LxcContainer {
	cfg := &LxcContainer{
		VmId: vmid,
	}

	for k, v := range raw {
		switch k {
		case "hostname":
			cfg.Hostname = v
		case "memory":
			if parsed, err := strconv.Atoi(v); err == nil {
				cfg.Memory = parsed
			}
		case "swap":
			if parsed, err := strconv.Atoi(v); err == nil {
				cfg.Swap = parsed
			}
		case "cores":
			if parsed, err := strconv.Atoi(v); err == nil {
				cfg.Cores = parsed
			}
		case "net0":
			cfg.Net0 = v
		case "rootfs":
			cfg.RootFsSize = v
			if idx := strings.Index(v, ":"); idx >= 0 {
				cfg.Storage = v[:idx]
			}
		}
	}

	return cfg
}

func (l *LxcContainer) ParseSshPublicKeySlice() (string, error) {
	var sshKeysParam strings.Builder

	if len(l.SshPublicKeys) < 1 {
		return "", fmt.Errorf("empty slice: no ssh keys provided")
	}

	if len(l.SshPublicKeys) == 1 {
		return l.SshPublicKeys[0], nil
	}

	lastSshKey := len(l.SshPublicKeys) - 1
	lastSshKeyItem := l.SshPublicKeys[lastSshKey]
	allButLastKey := l.SshPublicKeys[:lastSshKey]

	for _, value := range allButLastKey {
		sshKeysParam.WriteString(value)
		sshKeysParam.WriteString("\n")
	}
	sshKeysParam.WriteString(lastSshKeyItem)
	return sshKeysParam.String(), nil
}
