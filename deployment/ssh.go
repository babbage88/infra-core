package deployment

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/babbage88/goph/v2"
	coressh "github.com/babbage88/infra-core/ssh"
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
	return coressh.InitializeSshClient(hostname, username, sshKey, sshPassphrase, useAgent, port)
}

func ExpandPath(path string) string {
	return coressh.ExpandPath(path)
}

func ShellQuote(s string) string {
	return coressh.ShellQuote(s)
}

func FormatExecError(err error, out []byte) error {
	return coressh.FormatExecError(err, out)
}
