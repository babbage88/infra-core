package proxmox

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type ConsoleProxyResponse struct {
	Port   int    `json:"port"`
	Ticket string `json:"ticket"`
	User   string `json:"user,omitempty"`
}

func (c *Client) CreateQemuVNCProxy(ctx context.Context, node string, vmid int) (*ConsoleProxyResponse, error) {
	form := url.Values{}
	form.Set("websocket", "1")

	var result ConsoleProxyResponse
	if err := c.do(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/%s/qemu/%d/vncproxy", apiNodesPath, url.PathEscape(node), vmid),
		strings.NewReader(form.Encode()),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		true,
		&result,
	); err != nil {
		return nil, err
	}

	return &result, nil
}

func (c *Client) CreateLXCTermProxy(ctx context.Context, node string, vmid int) (*ConsoleProxyResponse, error) {
	form := url.Values{}

	var raw map[string]any
	if err := c.do(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/%s/lxc/%d/termproxy", apiNodesPath, url.PathEscape(node), vmid),
		strings.NewReader(form.Encode()),
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		true,
		&raw,
	); err != nil {
		return nil, err
	}

	result := &ConsoleProxyResponse{}
	if port, err := consolePortFromValue(raw["port"]); err == nil {
		result.Port = port
	}
	if ticket, ok := raw["ticket"].(string); ok {
		result.Ticket = ticket
	}
	if user, ok := raw["user"].(string); ok {
		result.User = user
	}
	if result.Port <= 0 {
		return nil, fmt.Errorf("termproxy response missing port")
	}
	if strings.TrimSpace(result.Ticket) == "" {
		return nil, fmt.Errorf("termproxy response missing ticket")
	}
	return result, nil
}

func consolePortFromValue(value any) (int, error) {
	switch typed := value.(type) {
	case float64:
		return int(typed), nil
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(typed))
	default:
		return 0, fmt.Errorf("unsupported port type %T", value)
	}
}
