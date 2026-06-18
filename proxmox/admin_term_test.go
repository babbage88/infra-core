package proxmox

import (
	"testing"

	coressh "github.com/babbage88/infra-core/ssh"
)

type fakeAdminRunner struct {
	command string
}

func (f *fakeAdminRunner) Run(cmd string) ([]byte, error) {
	f.command = cmd
	return []byte("ok"), nil
}

func TestRunRemoteQuotedCommandSetsDefaultTERM(t *testing.T) {
	runner := &fakeAdminRunner{}

	if _, err := runRemoteQuotedCommand(runner, "pveum", "user", "token", "permissions", "root@pam!infractl-cli"); err != nil {
		t.Fatalf("run remote quoted command: %v", err)
	}

	want := coressh.WithDefaultTERM("'pveum' 'user' 'token' 'permissions' 'root@pam!infractl-cli'")
	if runner.command != want {
		t.Fatalf("unexpected command:\nwant: %s\ngot:  %s", want, runner.command)
	}
}
