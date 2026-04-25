package deployment

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/babbage88/goph/v2"
	cryptossh "golang.org/x/crypto/ssh"
)

func PrepareSSHOptions(opts SSHOptions) (SSHOptions, func(), error) {
	hasPEM := strings.TrimSpace(opts.PrivateKeyPEM) != ""
	hasBase64 := strings.TrimSpace(opts.PrivateKeyBase64) != ""
	hasPath := strings.TrimSpace(opts.KeyPath) != ""

	if !hasPEM && !hasBase64 {
		return opts, func() {}, nil
	}
	if hasPath {
		return opts, func() {}, fmt.Errorf("provide only one SSH key source: key_path, private_key_pem, or private_key_base64")
	}
	if hasPEM && hasBase64 {
		return opts, func() {}, fmt.Errorf("provide only one SSH key source: private_key_pem or private_key_base64")
	}

	keyBytes := []byte(opts.PrivateKeyPEM)
	if hasBase64 {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(opts.PrivateKeyBase64))
		if err != nil {
			return opts, func() {}, fmt.Errorf("decode private_key_base64: %w", err)
		}
		keyBytes = decoded
	}
	if len(strings.TrimSpace(string(keyBytes))) == 0 {
		return opts, func() {}, fmt.Errorf("SSH private key content is empty")
	}

	keyFile, err := os.CreateTemp("", "infractl-ssh-key-*")
	if err != nil {
		return opts, func() {}, fmt.Errorf("create temporary SSH private key file: %w", err)
	}
	keyPath := keyFile.Name()
	cleanup := func() {
		_ = os.Remove(keyPath)
	}

	if err := keyFile.Chmod(0o600); err != nil {
		_ = keyFile.Close()
		cleanup()
		return opts, func() {}, fmt.Errorf("secure temporary SSH private key file: %w", err)
	}
	if _, err := keyFile.Write(keyBytes); err != nil {
		_ = keyFile.Close()
		cleanup()
		return opts, func() {}, fmt.Errorf("write temporary SSH private key file: %w", err)
	}
	if err := keyFile.Close(); err != nil {
		cleanup()
		return opts, func() {}, fmt.Errorf("close temporary SSH private key file: %w", err)
	}

	opts.KeyPath = keyPath
	opts.PrivateKeyPEM = ""
	opts.PrivateKeyBase64 = ""
	return opts, cleanup, nil
}

func InitializeSshClient(hostname, username, sshKey, sshPassphrase string, useAgent bool, port uint) (*goph.Client, error) {
	if strings.TrimSpace(hostname) == "" {
		return nil, fmt.Errorf("ssh host is required")
	}
	if strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf("ssh user is required")
	}
	if port == 0 {
		port = 22
	}

	authMethods := make(goph.Auth, 0, 2)
	if strings.TrimSpace(sshKey) != "" {
		keyAuth, err := goph.Key(ExpandPath(sshKey), sshPassphrase)
		if err != nil {
			return nil, fmt.Errorf("load SSH key %q: %w", sshKey, err)
		}
		authMethods = append(authMethods, keyAuth...)
	}

	if useAgent || goph.HasAgent() {
		agentAuth, err := goph.UseAgent()
		if err == nil {
			authMethods = append(authMethods, agentAuth...)
		} else if useAgent && len(authMethods) == 0 {
			return nil, fmt.Errorf("use ssh agent: %w", err)
		}
	}

	if len(authMethods) == 0 {
		for _, candidate := range []string{"~/.ssh/id_ed25519", "~/.ssh/id_rsa"} {
			expanded := ExpandPath(candidate)
			if info, err := os.Stat(expanded); err == nil && !info.IsDir() {
				keyAuth, loadErr := goph.Key(expanded, sshPassphrase)
				if loadErr != nil {
					return nil, fmt.Errorf("load SSH key %q: %w", expanded, loadErr)
				}
				authMethods = append(authMethods, keyAuth...)
				break
			}
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH authentication method configured; provide an ssh key or enable ssh-agent")
	}

	client, err := goph.NewConn(&goph.Config{
		User:     username,
		Addr:     hostname,
		Port:     port,
		Auth:     authMethods,
		Timeout:  goph.DefaultTimeout,
		Callback: cryptossh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return nil, fmt.Errorf("initialize ssh client: %w", err)
	}
	return client, nil
}

func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return path
}

func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func FormatExecError(err error, out []byte) error {
	output := strings.TrimSpace(string(out))
	if output == "" {
		return fmt.Errorf("SSH execution failed: %w", err)
	}
	return fmt.Errorf("SSH execution failed: %w: %s", err, output)
}
