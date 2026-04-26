package proxmox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const apiRootPath string = "/api2/json"
const apiClusterResourcesPath string = "/api2/json/cluster/resources"
const apiNodesPath string = "/api2/json/nodes"
const apiVmStartSubPath string = "/status/start"
const apiVmStopSubPath string = "/status/stop"

// APIError represents an error returned by the Proxmox API.
type APIError struct {
	Status int
	Errors map[string]interface{}
	Body   string
}

func (e *APIError) Error() string {
	if strings.TrimSpace(e.Body) != "" {
		return fmt.Sprintf("proxmox api error: status=%d errors=%v body=%s", e.Status, e.Errors, e.Body)
	}
	return fmt.Sprintf("proxmox api error: status=%d errors=%v", e.Status, e.Errors)
}

type AuthMethod int

const (
	AuthPassword AuthMethod = iota
	AuthToken
)

// Client is a reusable, thread-safe Proxmox VE API client.
type Client struct {
	baseURL     *url.URL
	authMethod  AuthMethod
	username    string // for AuthPassword: user@realm; for AuthToken: user@realm!tokenid
	password    string // for AuthPassword: password; for AuthToken: token secret
	httpClient  *http.Client
	loginMu     sync.Mutex
	authMu      sync.RWMutex
	authTicket  string
	csrfToken   string
	authCookie  *http.Cookie
	lastLogin   time.Time
	loginExpiry time.Duration
}

// TLSConfig holds optional TLS settings for the client.
type TLSConfig struct {
	IgnoreCertErrors bool   // true to skip verification
	CACertPath       string // optional path to CA cert to trust
}

// NewClientPassword creates a client using username/password auth.
func NewClient(base, username, password string, tlsCfg bool, useToken bool) (*Client, error) {
	authMethod := AuthPassword
	if useToken {
		authMethod = AuthToken
	}
	return newClient(base, username, password, authMethod, true)
}

// NewClientPassword creates a client using username/password auth.
func NewClientPassword(base, username, password string, tlsCfg bool) (*Client, error) {
	return newClient(base, username, password, AuthPassword, true)
}

// NewClientToken creates a client using API token authentication.
func NewClientToken(base, tokenID, secret string, tlsCfg bool) (*Client, error) {
	// tokenID format: user@realm!tokenname
	return newClient(base, tokenID, secret, AuthToken, true)
}

func newClient(base, username, password string, method AuthMethod, ignoreTlsError bool) (*Client, error) {
	if base == "" {
		return nil, errors.New("base URL required")
	}
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Custom transport to optionally skip TLS verification
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: ignoreTlsError,
		},
	}

	return &Client{
		baseURL:    u,
		authMethod: method,
		username:   username,
		password:   password,
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
		},
		loginExpiry: 1 * time.Hour,
	}, nil
}

// Login authenticates if using password mode.
func (c *Client) Login(ctx context.Context) error {
	if c.authMethod == AuthToken {
		// token mode doesn't require login
		return nil
	}

	c.loginMu.Lock()
	defer c.loginMu.Unlock()

	// Skip if still valid
	if time.Since(c.lastLogin) < c.loginExpiry && c.authTicket != "" {
		return nil
	}

	loginURL := *c.baseURL
	loginURL.Path = "/api2/json/access/ticket"

	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("login failed: status=%d body=%s", resp.StatusCode, body)
	}

	var v struct {
		Data struct {
			Ticket              string `json:"ticket"`
			CSRFPreventionToken string `json:"CSRFPreventionToken"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return fmt.Errorf("decoding login response: %w", err)
	}

	var authCookie *http.Cookie
	for _, ck := range resp.Cookies() {
		if ck.Name == "PVEAuthCookie" {
			authCookie = ck
			break
		}
	}

	c.authMu.Lock()
	c.authTicket = v.Data.Ticket
	c.csrfToken = v.Data.CSRFPreventionToken
	c.authCookie = authCookie
	c.lastLogin = time.Now()
	c.authMu.Unlock()

	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, headers map[string]string, csrf bool, out any) error {
	full := *c.baseURL
	full.Path = strings.TrimRight(c.baseURL.Path, "/") + path

	req, err := http.NewRequestWithContext(ctx, method, full.String(), body)
	if err != nil {
		return err
	}

	// Authentication
	if c.authMethod == AuthToken {
		req.Header.Set("Authorization", "PVEAPIToken="+c.username+"="+c.password)
	} else {
		if csrf {
			c.authMu.RLock()
			if c.csrfToken != "" {
				req.Header.Set("CSRFPreventionToken", c.csrfToken)
			}
			c.authMu.RUnlock()
		}
		c.authMu.RLock()
		if c.authCookie != nil {
			req.AddCookie(c.authCookie)
		} else if c.authTicket != "" {
			req.Header.Set("Cookie", "PVEAuthCookie="+c.authTicket)
		}
		c.authMu.RUnlock()
	}

	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read entire body first
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Try to parse Proxmox JSON error format, but ignore parsing errors
		var wrapper struct {
			Errors map[string]interface{} `json:"errors"`
		}
		_ = json.Unmarshal(bodyBytes, &wrapper)
		return &APIError{Status: resp.StatusCode, Errors: wrapper.Errors, Body: string(bodyBytes)}
	}

	if out != nil && len(bodyBytes) > 0 {
		var wrapper struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(bodyBytes, &wrapper); err != nil {
			return fmt.Errorf("invalid JSON response: %w", err)
		}
		if len(wrapper.Data) > 0 {
			if err := json.Unmarshal(wrapper.Data, out); err != nil {
				return fmt.Errorf("decoding data: %w", err)
			}
		}
	}

	return nil
}

func (c *Client) AuthHeaders(ctx context.Context, csrf bool) (http.Header, error) {
	if c.authMethod != AuthToken {
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
	}

	headers := http.Header{}
	if c.authMethod == AuthToken {
		headers.Set("Authorization", "PVEAPIToken="+c.username+"="+c.password)
		return headers, nil
	}

	c.authMu.RLock()
	defer c.authMu.RUnlock()
	if csrf && c.csrfToken != "" {
		headers.Set("CSRFPreventionToken", c.csrfToken)
	}
	if c.authCookie != nil {
		headers.Set("Cookie", c.authCookie.String())
	} else if c.authTicket != "" {
		headers.Set("Cookie", "PVEAuthCookie="+c.authTicket)
	}
	return headers, nil
}

func (c *Client) WebsocketDialer() *websocket.Dialer {
	dialer := websocket.Dialer{}
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok && transport.TLSClientConfig != nil {
		dialer.TLSClientConfig = transport.TLSClientConfig.Clone()
	}
	return &dialer
}
