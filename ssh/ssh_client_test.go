package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	skknownhosts "github.com/skeema/knownhosts"
	cryptossh "golang.org/x/crypto/ssh"
)

func TestBuildSSHAuthMethodsUsesExplicitKeyWithoutAgent(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing-agent.sock"))
	keyPath := writeTestPrivateKey(t)
	auth, err := buildSSHAuthMethods(keyPath, "", false)
	if err != nil {
		t.Fatalf("expected key auth to succeed without agent, got error: %v", err)
	}
	if len(auth) != 1 {
		t.Fatalf("expected exactly one auth method from explicit key, got %d", len(auth))
	}
}

func TestBuildSSHAuthMethodsFallsBackToKeyWhenAgentRequestedButUnavailable(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing-agent.sock"))
	keyPath := writeTestPrivateKey(t)
	auth, err := buildSSHAuthMethods(keyPath, "", true)
	if err != nil {
		t.Fatalf("expected key auth fallback to succeed, got error: %v", err)
	}
	if len(auth) != 1 {
		t.Fatalf("expected key fallback auth method, got %d methods", len(auth))
	}
}

func TestBuildSSHAuthMethodsErrorsWithoutKeyOrAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "")
	_, err := buildSSHAuthMethods("", "", false)
	if err == nil {
		t.Fatal("expected an error when no key or agent is configured")
	}
}

func TestBuildSSHAuthMethodsUsesAutoDiscoveredDefaultKey(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("SSH_AUTH_SOCK", "")
	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatalf("create .ssh dir: %v", err)
	}
	keyPath := filepath.Join(sshDir, "id_rsa")
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if err := os.WriteFile(keyPath, privateKeyPEM, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	auth, err := buildSSHAuthMethods("", "", false)
	if err != nil {
		t.Fatalf("expected auto-discovered key auth to succeed, got error: %v", err)
	}
	if len(auth) != 1 {
		t.Fatalf("expected exactly one auth method from auto-discovered key, got %d", len(auth))
	}
}

func TestKnownHostTargetsDefaultSSHPortPrefersPlainHost(t *testing.T) {
	remote := &net.TCPAddr{IP: net.ParseIP("10.2.10.248"), Port: 22}
	got := knownHostTargets("10.2.10.248", remote)
	want := []string{"10.2.10.248:22"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("knownHostTargets() = %v, want %v", got, want)
	}
}

func TestCheckKnownHostMatchesPlainHostEntryOnPort22(t *testing.T) {
	hostKey, err := generateHostPublicKey()
	if err != nil {
		t.Fatalf("generate host public key: %v", err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	file, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open known_hosts file: %v", err)
	}
	defer file.Close()
	remote := &net.TCPAddr{IP: net.ParseIP("10.2.10.248"), Port: 22}
	if err := skknownhosts.WriteKnownHost(file, "10.2.10.248", remote, hostKey); err != nil {
		t.Fatalf("write known_hosts entry: %v", err)
	}
	matched, err := checkKnownHost("10.2.10.248", remote, hostKey, []string{knownHostsPath})
	if err != nil {
		t.Fatalf("checkKnownHost returned error: %v", err)
	}
	if !matched {
		t.Fatal("expected plain host known_hosts entry to match port 22 target")
	}
}

func TestSplitSSHConfigValues(t *testing.T) {
	got := splitSSHConfigValues("~/.ssh/known_hosts ~/.ssh/known_hosts2")
	want := []string{"~/.ssh/known_hosts", "~/.ssh/known_hosts2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitSSHConfigValues() = %v, want %v", got, want)
	}
}

func TestDetermineSSHAuthSource(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("SSH_AUTH_SOCK", "")
	defaultKeyPath := writePrivateKeyAt(t, filepath.Join(homeDir, ".ssh", "id_ed25519"))

	tests := []struct {
		name                string
		explicitKeyProvided bool
		sshKeyPath          string
		agentRequested      bool
		want                string
	}{
		{name: "explicit key", explicitKeyProvided: true, sshKeyPath: "/tmp/custom", want: "explicit-key"},
		{name: "ssh config identity", sshKeyPath: "/tmp/config_identity", want: "ssh-config-identityfile"},
		{name: "default key", sshKeyPath: defaultKeyPath, want: "default-key"},
		{name: "default key plus agent", sshKeyPath: defaultKeyPath, agentRequested: true, want: "default-key+agent"},
		{name: "agent only", agentRequested: true, want: "agent-only"},
		{name: "none", want: "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineSSHAuthSource(tt.explicitKeyProvided, tt.sshKeyPath, tt.agentRequested)
			if got != tt.want {
				t.Fatalf("determineSSHAuthSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWithDefaultTERMUsesFallbackWithoutOverwritingExistingTERM(t *testing.T) {
	command := WithDefaultTERM("'pveum' 'user' 'token' 'list'")

	want := `TERM="${TERM:-dumb}" 'pveum' 'user' 'token' 'list'`
	if command != want {
		t.Fatalf("unexpected command:\nwant: %s\ngot:  %s", want, command)
	}
}

func TestWithDefaultTERMHandlesEmptyCommand(t *testing.T) {
	command := WithDefaultTERM("   ")

	want := `TERM="${TERM:-dumb}"`
	if command != want {
		t.Fatalf("unexpected command:\nwant: %s\ngot:  %s", want, command)
	}
}

func TestFormatExecErrorStripsBenignTERMNoise(t *testing.T) {
	err := errors.New("process exited with status 255")
	out := []byte("tput: No value for $TERM and no -T specified\n400 Parameter verification failed.\ntokenid: Token already exists.\n")

	got := FormatExecError(err, out).Error()
	want := "SSH execution failed: process exited with status 255: 400 Parameter verification failed.\ntokenid: Token already exists."
	if got != want {
		t.Fatalf("unexpected formatted error:\nwant: %s\ngot:  %s", want, got)
	}
}

func writeTestPrivateKey(t *testing.T) string {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	keyPath := filepath.Join(t.TempDir(), "id_rsa")
	if err := os.WriteFile(keyPath, privateKeyPEM, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	return keyPath
}

func writePrivateKeyAt(t *testing.T, keyPath string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatalf("create ssh dir: %v", err)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	if err := os.WriteFile(keyPath, privateKeyPEM, 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}
	return keyPath
}

func generateHostPublicKey() (cryptossh.PublicKey, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	signer, err := cryptossh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, err
	}
	return signer.PublicKey(), nil
}
