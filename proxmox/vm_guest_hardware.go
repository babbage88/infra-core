package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type GuestNetworkIPAddress struct {
	IPAddress     string  `json:"ip-address"`
	IPAddressType string  `json:"ip-address-type,omitempty"`
	Prefix        JsonInt `json:"prefix,omitempty"`
}

type GuestNetworkInterface struct {
	Name         string                  `json:"name,omitempty"`
	HardwareAddr string                  `json:"hardware-address,omitempty"`
	IPAddresses  []GuestNetworkIPAddress `json:"ip-addresses,omitempty"`
	Raw          map[string]any          `json:"raw,omitempty"`
}

type NodeNetworkInterface struct {
	Iface       string `json:"iface"`
	Type        string `json:"type,omitempty"`
	Active      int    `json:"active,omitempty"`
	Autostart   int    `json:"autostart,omitempty"`
	BridgePorts string `json:"bridge_ports,omitempty"`
	CIDR        string `json:"cidr,omitempty"`
	Gateway     string `json:"gateway,omitempty"`
	Method      string `json:"method,omitempty"`
	Method6     string `json:"method6,omitempty"`
}

type qemuAgentNetworkGetInterfacesResponse struct {
	Result []GuestNetworkInterface `json:"result"`
}

func (c *Client) GetQemuAgentNetworkInterfaces(ctx context.Context, node string, vmid int) ([]GuestNetworkInterface, error) {
	path := fmt.Sprintf("%s/%s/qemu/%d/agent/network-get-interfaces", apiNodesPath, url.PathEscape(node), vmid)
	var resp qemuAgentNetworkGetInterfacesResponse
	if err := c.do(ctx, http.MethodGet, path, nil, nil, false, &resp); err != nil {
		return nil, err
	}

	return resp.Result, nil
}

func (c *Client) GetLXCInterfaces(ctx context.Context, node string, vmid int) ([]GuestNetworkInterface, error) {
	path := fmt.Sprintf("%s/%s/lxc/%d/interfaces", apiNodesPath, url.PathEscape(node), vmid)
	var interfaces []GuestNetworkInterface
	if err := c.do(ctx, http.MethodGet, path, nil, nil, false, &interfaces); err != nil {
		return nil, err
	}
	return interfaces, nil
}

func (c *Client) ListNodeNetwork(ctx context.Context, node string) ([]NodeNetworkInterface, error) {
	path := fmt.Sprintf("%s/%s/network", apiNodesPath, url.PathEscape(node))
	var interfaces []NodeNetworkInterface
	if err := c.do(ctx, http.MethodGet, path, nil, nil, false, &interfaces); err != nil {
		return nil, err
	}
	return interfaces, nil
}

func (c *Client) ListNodeBridges(ctx context.Context, node string) ([]NodeNetworkInterface, error) {
	interfaces, err := c.ListNodeNetwork(ctx, node)
	if err != nil {
		return nil, err
	}
	bridges := make([]NodeNetworkInterface, 0)
	for _, item := range interfaces {
		if strings.EqualFold(item.Type, "bridge") || strings.HasPrefix(item.Iface, "vmbr") {
			bridges = append(bridges, item)
		}
	}
	return bridges, nil
}

func (c *Client) UpdateVMConfigRaw(ctx context.Context, node string, vmid int, params url.Values) error {
	if params == nil {
		return fmt.Errorf("params are required")
	}
	path := fmt.Sprintf("%s/%s/qemu/%d/config", apiNodesPath, url.PathEscape(node), vmid)
	headers := map[string]string{"Content-Type": "application/x-www-form-urlencoded"}
	return c.do(ctx, http.MethodPut, path, strings.NewReader(params.Encode()), headers, true, nil)
}
