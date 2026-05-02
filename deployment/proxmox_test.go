package deployment

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestCreateProxmoxVMAutoAllocatesVMIDWhenOmitted(t *testing.T) {
	t.Helper()

	var cloneNewID string
	var configVMID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/cluster/nextid":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": 4321})
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve1/qemu/9000/clone":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse clone form: %v", err)
			}
			cloneNewID = r.Form.Get("newid")
			if cloneNewID != "4321" {
				t.Fatalf("expected clone newid 4321, got %q", cloneNewID)
			}
			if r.Form.Get("name") != "app-vm" {
				t.Fatalf("expected clone name app-vm, got %q", r.Form.Get("name"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve1/qemu/4321/config":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"name": "app-vm"}})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/pve1/qemu/4321/config":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse config form: %v", err)
			}
			configVMID = strings.TrimPrefix(r.URL.Path, "/api2/json/nodes/pve1/qemu/")
			configVMID = strings.TrimSuffix(configVMID, "/config")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	result, err := CreateProxmoxVM(ProxmoxVMCreateRequest{
		Auth: ProxmoxAuthOptions{
			HostURL:    server.URL,
			APITokenID: "user@pve!token",
			APISecret:  "secret",
		},
		Node:         "pve1",
		TemplateVMID: 9000,
		Name:         "app-vm",
	})
	if err != nil {
		t.Fatalf("CreateProxmoxVM returned error: %v", err)
	}
	if result.VMID != 4321 {
		t.Fatalf("expected result VMID 4321, got %d", result.VMID)
	}
	if cloneNewID != "4321" {
		t.Fatalf("expected clone call to receive allocated VMID, got %q", cloneNewID)
	}
	if configVMID != strconv.Itoa(result.VMID) {
		t.Fatalf("expected config update for VMID %d, got path VMID %q", result.VMID, configVMID)
	}
}

func TestProxmoxEscapeCloudInitSSHKeys(t *testing.T) {
	keys := strings.Join([]string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIL1AWCZrkN1LHIg8BbTw23v44Hf59pZOTn4d/5nZnQkt jtrahan@Justins-MacBook-Air.local",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAkSxvL74Se111NzT1hsFcFk7R2YTRTOVmhbOjJVaYuz justin.m.trahan@outlook.com",
	}, "\n")

	got := proxmoxEscapeCloudInitSSHKeys(keys)

	wantContains := []string{
		"ssh-ed25519%20AAAAC3NzaC1lZDI1NTE5",
		"%2F5nZnQkt%20jtrahan%40Justins-MacBook-Air.local",
		"%0A",
		"%20justin.m.trahan%40outlook.com",
	}

	for _, want := range wantContains {
		if !strings.Contains(got, want) {
			t.Fatalf("expected escaped sshkeys to contain %q\ngot: %s", want, got)
		}
	}

	form := url.Values{}
	form.Set("sshkeys", got)

	encodedForm := form.Encode()
	if !strings.Contains(encodedForm, "%2520") {
		t.Fatalf("expected outer form encoding to double-escape %%20 as %%2520, got: %s", encodedForm)
	}
	if !strings.Contains(encodedForm, "%250A") {
		t.Fatalf("expected outer form encoding to double-escape newline as %%250A, got: %s", encodedForm)
	}
}

func TestCreateProxmoxVMEncodesSSHKeysForCloudInit(t *testing.T) {
	t.Helper()

	var sshkeysValue string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve1/qemu/9000/clone":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve1/qemu/4321/config":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"name": "app-vm"}})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/pve1/qemu/4321/config":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse config form: %v", err)
			}
			sshkeysValue = r.Form.Get("sshkeys")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	_, err := CreateProxmoxVM(ProxmoxVMCreateRequest{
		Auth: ProxmoxAuthOptions{
			HostURL:    server.URL,
			APITokenID: "user@pve!token",
			APISecret:  "secret",
		},
		Node:         "pve1",
		VMID:         4321,
		TemplateVMID: 9000,
		Name:         "app-vm",
		SshPublicKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyOne user@example.com",
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyTwo user2@example.com",
		},
	})
	if err != nil {
		t.Fatalf("CreateProxmoxVM returned error: %v", err)
	}

	wantEncodedKeys := strings.ReplaceAll(url.QueryEscape(strings.Join([]string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyOne user@example.com",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyTwo user2@example.com",
	}, "\n")), "+", "%20")
	if sshkeysValue != wantEncodedKeys {
		t.Fatalf("expected encoded sshkeys value %q, got %q", wantEncodedKeys, sshkeysValue)
	}
}

func TestCreateProxmoxVMDeletesCloneWhenConfigFails(t *testing.T) {
	t.Helper()

	var deletedVMID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api2/json/nodes/pve1/qemu/9000/clone":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": nil})
		case r.Method == http.MethodGet && r.URL.Path == "/api2/json/nodes/pve1/qemu/4321/config":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"name": "app-vm"}})
		case r.Method == http.MethodPut && r.URL.Path == "/api2/json/nodes/pve1/qemu/4321/config":
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errors": map[string]string{"sshkeys": "invalid format"},
				"data":   nil,
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api2/json/nodes/pve1/qemu/4321":
			deletedVMID = "4321"
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:pve1:delete"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	_, err := CreateProxmoxVM(ProxmoxVMCreateRequest{
		Auth: ProxmoxAuthOptions{
			HostURL:    server.URL,
			APITokenID: "user@pve!token",
			APISecret:  "secret",
		},
		Node:         "pve1",
		VMID:         4321,
		TemplateVMID: 9000,
		Name:         "app-vm",
		SshPublicKeys: []string{
			"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyOne user@example.com",
		},
	})
	if err == nil {
		t.Fatalf("expected config failure error")
	}
	if deletedVMID != "4321" {
		t.Fatalf("expected failed clone cleanup to delete VM 4321, got %q", deletedVMID)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback error message, got %q", err.Error())
	}
}
