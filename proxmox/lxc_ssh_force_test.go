package proxmox

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeLxcSSHClient struct {
	commands []string
	run      func(cmd string) ([]byte, error)
}

func (c *fakeLxcSSHClient) Run(cmd string) ([]byte, error) {
	c.commands = append(c.commands, cmd)
	if c.run != nil {
		return c.run(cmd)
	}
	return nil, nil
}

func (c *fakeLxcSSHClient) Upload(_, _ string) error {
	return nil
}

func (c *fakeLxcSSHClient) Close() error {
	return nil
}

func TestLxcSSHForceBootstrapScriptUsesStableLocaleForApt(t *testing.T) {
	script := lxcSSHForceBootstrapScript([]string{"ssh-ed25519 AAAATEST user@example"}, LxcSSHForceOptions{})

	for _, want := range []string{
		"export LC_ALL=C",
		"export LANG=C",
		"export LANGUAGE=C",
		"DEBIAN_FRONTEND=noninteractive APT_LISTCHANGES_FRONTEND=none apt-get install -y \"$@\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("bootstrap script missing %q\nscript:\n%s", want, script)
		}
	}
}

func TestLxcSSHForceBootstrapScriptIncludesAdminSetup(t *testing.T) {
	script := lxcSSHForceBootstrapScript([]string{"ssh-ed25519 AAAATEST user@example"}, LxcSSHForceOptions{
		AddAdminUser: true,
		AdminUser:    "deploy",
		AdminUID:     1001,
	})

	for _, want := range []string{
		"install_pkg sudo",
		"admin_user='deploy'",
		"admin_uid='1001'",
		"ensure_account_allows_ssh_public_key_login \"$admin_user\"",
		"printf '%s:%s\\n' \"$user_name\" \"$(random_account_password)\" | chpasswd",
		"printf '%s ALL=(ALL) NOPASSWD:ALL\\n' \"$admin_user\" > '/etc/sudoers.d/90-infractl-deploy'",
		"ensure_authorized_keys \"$admin_user\" \"$admin_home\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("bootstrap script missing %q\nscript:\n%s", want, script)
		}
	}
}

func TestLxcSSHForceBootstrapScriptStartsSSHDWithAbsolutePath(t *testing.T) {
	script := lxcSSHForceBootstrapScript([]string{"ssh-ed25519 AAAATEST user@example"}, LxcSSHForceOptions{})

	for _, want := range []string{
		"if [ -x /usr/sbin/sshd ]; then",
		"/usr/sbin/sshd",
		"sshd_path=$(command -v sshd 2>/dev/null || true)",
		"/*) \"$sshd_path\" ;;",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("bootstrap script missing %q\nscript:\n%s", want, script)
		}
	}
	if strings.Contains(script, "\n\t\tsshd\n") {
		t.Fatalf("bootstrap script still invokes sshd without an absolute path\nscript:\n%s", script)
	}
}

func TestRunPctExecShellScriptUsesRemoteTimeout(t *testing.T) {
	client := &fakeLxcSSHClient{}

	if _, err := runPctExecShellScriptWithTimeoutAndLog(client, 252, "true", lxcPctExecProbeTimeout, nil); err != nil {
		t.Fatalf("run pct exec script: %v", err)
	}

	if len(client.commands) != 1 {
		t.Fatalf("expected one command, got %d", len(client.commands))
	}
	for _, want := range []string{"TERM=\"${TERM:-dumb}\"", "command -v timeout", "timeout", "15s", "pct exec", "252", "env TERM=dumb sh -c", "true"} {
		if !strings.Contains(client.commands[0], want) {
			t.Fatalf("pct exec command missing %q: %s", want, client.commands[0])
		}
	}
}

func TestRunPctExecShellScriptDoesNotTimeoutLongBootstrapByDefault(t *testing.T) {
	client := &fakeLxcSSHClient{}

	if _, err := RunPctExecShellScript(client, 252, "apk add --no-cache openssh"); err != nil {
		t.Fatalf("run pct exec script: %v", err)
	}

	if len(client.commands) != 1 {
		t.Fatalf("expected one command, got %d", len(client.commands))
	}
	if strings.Contains(client.commands[0], "timeout") {
		t.Fatalf("default pct exec command unexpectedly includes timeout: %s", client.commands[0])
	}
	for _, want := range []string{"TERM=\"${TERM:-dumb}\"", "env TERM=dumb sh -c"} {
		if !strings.Contains(client.commands[0], want) {
			t.Fatalf("pct exec command missing %q: %s", want, client.commands[0])
		}
	}
}

func TestRunPctExecShellScriptStripsBenignTERMNoiseFromSuccessfulOutput(t *testing.T) {
	client := &fakeLxcSSHClient{run: func(cmd string) ([]byte, error) {
		return []byte("tput: No value for $TERM and no -T specified\nDebian GNU/Linux 13 (trixie).\n"), nil
	}}

	out, err := RunPctExecShellScript(client, 252, "printf test")
	if err != nil {
		t.Fatalf("run pct exec script: %v", err)
	}

	got := string(out)
	if strings.Contains(got, "tput: No value for $TERM and no -T specified") {
		t.Fatalf("expected TERM noise to be stripped from output: %q", got)
	}
	if !strings.Contains(got, "Debian GNU/Linux 13 (trixie).") {
		t.Fatalf("expected real output to remain: %q", got)
	}
}

func TestLxcPrimaryIPv4ScriptPrefersIPAddrWithFallbacks(t *testing.T) {
	script := lxcPrimaryIPv4Script()

	for _, want := range []string{
		"ip -4 -o addr show scope global",
		"hostname -I",
		"ifconfig",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("IPv4 script missing %q\nscript:\n%s", want, script)
		}
	}
}

func TestWaitForLxcIPv4ReturnsWhenContainerStops(t *testing.T) {
	client := &fakeLxcSSHClient{run: func(cmd string) ([]byte, error) {
		if strings.Contains(cmd, "status") {
			return []byte("status: stopped\n"), nil
		}
		return nil, errors.New("pct exec failed")
	}}

	_, err := WaitForLxcIPv4OverSSH(client, 252, time.Second)
	if err == nil {
		t.Fatal("expected wait to fail when container stops")
	}
	if !strings.Contains(err.Error(), "stopped while waiting for IPv4") {
		t.Fatalf("expected stopped error, got: %v", err)
	}
}

func TestLxcNetworkingBootstrapScriptSupportsAlpineDHCP(t *testing.T) {
	script := lxcNetworkingBootstrapScript()

	for _, want := range []string{
		"rc-service networking start",
		"udhcpc -q -n -i \"$iface\" -t 5",
		"ifup -a",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("networking bootstrap script missing %q\nscript:\n%s", want, script)
		}
	}
}
