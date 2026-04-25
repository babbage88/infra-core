package proxmox

import (
	"fmt"
	"net"
	"strings"
	"time"

	infraSSH "github.com/babbage88/infra-core/ssh"
)

type LxcSSHForceOptions struct {
	AddAdminUser bool
	AdminUser    string
	AdminUID     int
}

type LxcLogSink func(LxcLogEntry)

type LxcLogKind string

const (
	LxcLogStatus  LxcLogKind = "status"
	LxcLogCommand LxcLogKind = "command"

	lxcPctExecProbeTimeout      = 15 * time.Second
	lxcPctExecNetworkingTimeout = 45 * time.Second
)

type LxcLogEntry struct {
	Kind  LxcLogKind
	Label string
	Body  string
}

func ForceLxcSSHReadiness(sshClient infraSSH.Client, req *LxcContainer, options LxcSSHForceOptions) (string, error) {
	return ForceLxcSSHReadinessWithLog(sshClient, req, options, nil)
}

func ForceLxcSSHReadinessWithLog(sshClient infraSSH.Client, req *LxcContainer, options LxcSSHForceOptions, log LxcLogSink) (string, error) {
	if req == nil {
		return "", fmt.Errorf("LXC request is required")
	}
	if req.VmId <= 0 {
		return "", fmt.Errorf("container VM ID is required for --ssh-force")
	}
	if strings.TrimSpace(req.Node) == "" {
		return "", fmt.Errorf("Proxmox node is required for --ssh-force")
	}
	if len(req.SshPublicKeys) == 0 {
		return "", fmt.Errorf("--ssh-force requires at least one SSH public key; pass --ssh-public-keys or accept the SSH key prompt")
	}

	if req.Start != "1" {
		lxcStatusf(log, "Starting container %d so SSH can be prepared...", req.VmId)
		if _, err := RunRemoteQuotedCommandWithLog(sshClient, log, "pct", "start", fmt.Sprintf("%d", req.VmId)); err != nil {
			return "", fmt.Errorf("start container %d for --ssh-force: %w", req.VmId, err)
		}
	}

	if err := WaitForLxcRunningOverSSHWithLog(sshClient, req.VmId, 2*time.Minute, log); err != nil {
		return "", err
	}

	osInfo, err := DetectLxcOSTypeOverSSHWithLog(sshClient, req.VmId, log)
	if err != nil {
		return "", err
	}
	lxcStatusf(log, "Container %d OS: %s.", req.VmId, osInfo)

	if err := EnsureLxcNetworkingStartedOverSSHWithLog(sshClient, req.VmId, log); err != nil {
		return "", err
	}

	ipAddr, err := WaitForLxcIPv4OverSSHWithLog(sshClient, req.VmId, 3*time.Minute, log)
	if err != nil {
		return "", err
	}
	lxcStatusf(log, "Container %d reported IPv4 address %s.", req.VmId, ipAddr)

	if err := EnsureLxcSSHServerAndUsersOverSSHWithLog(sshClient, req.VmId, req.SshPublicKeys, options, log); err != nil {
		return "", err
	}
	lxcStatusf(log, "Container %d SSH server is installed, enabled, and has root authorized_keys.", req.VmId)
	if options.AddAdminUser {
		lxcStatusf(log, "Container %d admin user %s is ready.", req.VmId, options.AdminUser)
	}

	return ipAddr, nil
}

func DetectLxcOSTypeOverSSHWithLog(sshClient infraSSH.Client, vmid int, log LxcLogSink) (string, error) {
	script := `if [ -r /etc/os-release ]; then . /etc/os-release; printf '%s' "${PRETTY_NAME:-${ID:-unknown}}"; else uname -s; fi`
	out, err := RunPctExecShellScriptWithLog(sshClient, vmid, script, log)
	if err != nil {
		return "", fmt.Errorf("detect container OS for %d: %w", vmid, err)
	}

	osInfo := strings.TrimSpace(string(out))
	if osInfo == "" {
		return "unknown", nil
	}
	return osInfo, nil
}

func EnsureLxcNetworkingStartedOverSSHWithLog(sshClient infraSSH.Client, vmid int, log LxcLogSink) error {
	lxcStatusf(log, "Ensuring container %d networking is started...", vmid)
	if _, err := runPctExecShellScriptWithTimeoutAndLog(sshClient, vmid, lxcNetworkingBootstrapScript(), lxcPctExecNetworkingTimeout, log); err != nil {
		return fmt.Errorf("ensure networking is started in container %d: %w", vmid, err)
	}
	return nil
}

func lxcNetworkingBootstrapScript() string {
	return `log_step() {
	printf 'infractl: %s\n' "$*"
}
has_ipv4() {
	if command -v ip >/dev/null 2>&1; then
		ip -4 -o addr show scope global 2>/dev/null | awk '{split($4, a, "/"); if (a[1] !~ /^127\./) {found=1}} END {exit found ? 0 : 1}'
	elif command -v ifconfig >/dev/null 2>&1; then
		ifconfig 2>/dev/null | awk '/inet / && $0 !~ /127\.0\.0\.1/ {found=1} END {exit found ? 0 : 1}'
	else
		return 1
	fi
}
non_loopback_interfaces() {
	for iface in /sys/class/net/*; do
		[ -e "$iface" ] || continue
		iface=${iface##*/}
		[ "$iface" = "lo" ] && continue
		printf '%s\n' "$iface"
	done
}
if has_ipv4; then
	log_step "network already has IPv4"
	exit 0
fi
if command -v ip >/dev/null 2>&1; then
	for iface in $(non_loopback_interfaces); do
		ip link set "$iface" up >/dev/null 2>&1 || true
	done
fi
if command -v ifup >/dev/null 2>&1; then
	log_step "starting configured interfaces with ifup"
	ifup -a >/dev/null 2>&1 || for iface in $(non_loopback_interfaces); do ifup "$iface" >/dev/null 2>&1 || true; done
fi
if ! has_ipv4 && command -v rc-service >/dev/null 2>&1; then
	log_step "starting OpenRC networking"
	rc-service networking start >/dev/null 2>&1 || rc-service networking restart >/dev/null 2>&1 || true
fi
if ! has_ipv4 && command -v udhcpc >/dev/null 2>&1; then
	for iface in $(non_loopback_interfaces); do
		log_step "requesting DHCP lease on $iface"
		udhcpc -q -n -i "$iface" -t 5 >/dev/null 2>&1 || true
		has_ipv4 && break
	done
fi
has_ipv4 || true`
}

func EnsureLxcSSHServerAndUsersOverSSH(sshClient infraSSH.Client, vmid int, publicKeys []string, options LxcSSHForceOptions) error {
	return EnsureLxcSSHServerAndUsersOverSSHWithLog(sshClient, vmid, publicKeys, options, nil)
}

func EnsureLxcSSHServerAndUsersOverSSHWithLog(sshClient infraSSH.Client, vmid int, publicKeys []string, options LxcSSHForceOptions, log LxcLogSink) error {
	keys := sanitizeSSHPublicKeys(publicKeys)
	if len(keys) == 0 {
		return fmt.Errorf("no usable SSH public keys were provided")
	}
	if options.AddAdminUser {
		options.AdminUser = strings.TrimSpace(options.AdminUser)
		if options.AdminUser == "" {
			return fmt.Errorf("--admin-username is required when --add-admin-user is set")
		}
		if strings.ContainsAny(options.AdminUser, "\r\n:/'\"`$\\ ") {
			return fmt.Errorf("admin username %q contains unsupported characters", options.AdminUser)
		}
		if options.AdminUID <= 0 {
			return fmt.Errorf("--admin-uid must be greater than zero when --add-admin-user is set")
		}
	}

	lxcStatusf(log, "Ensuring SSH server, authorized_keys, and requested users inside container %d...", vmid)

	script := lxcSSHForceBootstrapScript(keys, options)
	if _, err := RunPctExecShellScriptWithLog(sshClient, vmid, script, log); err != nil {
		return fmt.Errorf("ensure SSH server, users, and authorized_keys in container %d: %w", vmid, err)
	}
	return nil
}

func lxcSSHForceBootstrapScript(keys []string, options LxcSSHForceOptions) string {
	return `set -eu
if [ -r /etc/os-release ]; then . /etc/os-release; fi
export LC_ALL=C
export LANG=C
export LANGUAGE=C
log_step() {
	printf 'infractl: %s\n' "$*"
}
has_sshd() {
	command -v sshd >/dev/null 2>&1 || [ -x /usr/sbin/sshd ] || [ -x /usr/local/sbin/sshd ]
}
install_pkg() {
	log_step "installing packages: $*"
	if command -v dnf >/dev/null 2>&1; then
		dnf -y install "$@"
	elif command -v yum >/dev/null 2>&1; then
		yum -y install "$@"
	elif command -v apt-get >/dev/null 2>&1; then
		apt-get update
		DEBIAN_FRONTEND=noninteractive APT_LISTCHANGES_FRONTEND=none apt-get install -y "$@"
	elif command -v zypper >/dev/null 2>&1; then
		zypper --non-interactive install "$@"
	elif command -v apk >/dev/null 2>&1; then
		apk add --no-cache "$@"
	elif command -v pacman >/dev/null 2>&1; then
		pacman -Sy --noconfirm "$@"
	else
		echo "no supported package manager found" >&2
		exit 1
	fi
}
if ! has_sshd; then
	log_step "SSH server is missing; installing it"
	if command -v dnf >/dev/null 2>&1; then
		install_pkg openssh-server
	elif command -v yum >/dev/null 2>&1; then
		install_pkg openssh-server
	elif command -v apt-get >/dev/null 2>&1; then
		install_pkg openssh-server
	elif command -v zypper >/dev/null 2>&1; then
		install_pkg openssh
	elif command -v apk >/dev/null 2>&1; then
		install_pkg openssh
	elif command -v pacman >/dev/null 2>&1; then
		install_pkg openssh
	else
		echo "no supported package manager found to install openssh-server" >&2
		exit 1
	fi
else
	log_step "SSH server is already installed"
fi
ensure_authorized_keys() {
	user_name="$1"
	user_home="$2"
	log_step "ensuring authorized_keys for $user_name"
	install -d -m 0700 "$user_home/.ssh"
	touch "$user_home/.ssh/authorized_keys"
	chmod 0600 "$user_home/.ssh/authorized_keys"
	while IFS= read -r key; do
		[ -n "$key" ] || continue
		grep -qxF "$key" "$user_home/.ssh/authorized_keys" || printf '%s\n' "$key" >> "$user_home/.ssh/authorized_keys"
	done <<'INFRACTL_SSH_KEYS'
` + strings.Join(keys, "\n") + `
INFRACTL_SSH_KEYS
	if [ "$user_name" != "root" ]; then
		chown "$user_name:$user_name" "$user_home" 2>/dev/null || chown "$user_name" "$user_home" 2>/dev/null || true
		chmod 0755 "$user_home" 2>/dev/null || true
		chown -R "$user_name:$user_name" "$user_home/.ssh" 2>/dev/null || chown -R "$user_name" "$user_home/.ssh"
	fi
}
ensure_authorized_keys root /root
` + adminUserBootstrapScript(options) + `
if command -v ssh-keygen >/dev/null 2>&1; then
	log_step "generating SSH host keys"
	ssh-keygen -A
fi
if command -v systemctl >/dev/null 2>&1; then
	log_step "enabling SSH service"
	systemctl enable --now sshd >/dev/null 2>&1 || systemctl enable --now ssh >/dev/null 2>&1 || true
fi
if ! pgrep -x sshd >/dev/null 2>&1; then
	if command -v service >/dev/null 2>&1; then
		log_step "starting SSH service"
		service sshd start >/dev/null 2>&1 || service ssh start >/dev/null 2>&1 || true
	fi
fi
if ! pgrep -x sshd >/dev/null 2>&1; then
	log_step "starting sshd directly"
	if [ -x /usr/sbin/sshd ]; then
		/usr/sbin/sshd
	elif [ -x /usr/local/sbin/sshd ]; then
		/usr/local/sbin/sshd
	else
		sshd_path=$(command -v sshd 2>/dev/null || true)
		if [ -n "$sshd_path" ]; then
			case "$sshd_path" in
				/*) "$sshd_path" ;;
				*) echo "sshd executable was found at non-absolute path $sshd_path" >&2; exit 1 ;;
			esac
		fi
	fi
fi
log_step "verifying SSH daemon"
pgrep -x sshd >/dev/null 2>&1`
}

func adminUserBootstrapScript(options LxcSSHForceOptions) string {
	if !options.AddAdminUser {
		return ""
	}

	adminUser := infraSSH.ShellQuote(options.AdminUser)
	adminUID := infraSSH.ShellQuote(fmt.Sprintf("%d", options.AdminUID))
	sudoersPath := infraSSH.ShellQuote("/etc/sudoers.d/90-infractl-" + options.AdminUser)

	return `
if ! command -v sudo >/dev/null 2>&1; then
	log_step "sudo is missing; installing it"
	install_pkg sudo
else
	log_step "sudo is already installed"
fi
admin_user=` + adminUser + `
admin_uid=` + adminUID + `
user_home_from_passwd() {
	awk -F: -v user="$1" '$1 == user {print $6; exit}' /etc/passwd
}
account_password_field() {
	awk -F: -v user="$1" '$1 == user {print $2; exit}' /etc/shadow 2>/dev/null || true
}
random_account_password() {
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -base64 24
	elif command -v base64 >/dev/null 2>&1; then
		dd if=/dev/urandom bs=24 count=1 2>/dev/null | base64
	else
		dd if=/dev/urandom bs=24 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n'
	fi
}
ensure_account_allows_ssh_public_key_login() {
	user_name="$1"
	password_field=$(account_password_field "$user_name")
	case "$password_field" in
		""|"!"|"!!"|"*"|!* )
			log_step "setting random locked-account password field for $user_name"
			if command -v chpasswd >/dev/null 2>&1; then
				printf '%s:%s\n' "$user_name" "$(random_account_password)" | chpasswd
			elif command -v passwd >/dev/null 2>&1; then
				passwd -u "$user_name" >/dev/null 2>&1 || true
			elif command -v usermod >/dev/null 2>&1; then
				usermod -U "$user_name" >/dev/null 2>&1 || true
			else
				echo "no supported command found to make $user_name eligible for SSH public-key login" >&2
				exit 1
			fi
			;;
	esac
}
if id "$admin_user" >/dev/null 2>&1; then
	log_step "admin user $admin_user already exists"
	admin_home=$(user_home_from_passwd "$admin_user")
else
	log_step "creating admin user $admin_user"
	admin_shell=/bin/sh
	[ -x /bin/bash ] && admin_shell=/bin/bash
	if command -v useradd >/dev/null 2>&1; then
		useradd -m -u "$admin_uid" -s "$admin_shell" "$admin_user"
	elif command -v adduser >/dev/null 2>&1; then
		adduser -D -u "$admin_uid" -s "$admin_shell" "$admin_user"
	else
		echo "no supported user creation command found" >&2
		exit 1
	fi
	admin_home=$(user_home_from_passwd "$admin_user")
fi
[ -n "$admin_home" ] || admin_home="/home/$admin_user"
ensure_account_allows_ssh_public_key_login "$admin_user"
log_step "writing sudoers rule for $admin_user"
install -d -m 0755 /etc/sudoers.d
printf '%s ALL=(ALL) NOPASSWD:ALL\n' "$admin_user" > ` + sudoersPath + `
chmod 0440 ` + sudoersPath + `
ensure_authorized_keys "$admin_user" "$admin_home"
`
}

func sanitizeSSHPublicKeys(keys []string) []string {
	sanitized := make([]string, 0, len(keys))
	seen := make(map[string]struct{})
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsAny(key, "\r\n") {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		sanitized = append(sanitized, key)
	}
	return sanitized
}

func RunPctExecShellScript(sshClient infraSSH.Client, vmid int, script string) ([]byte, error) {
	return RunPctExecShellScriptWithLog(sshClient, vmid, script, nil)
}

func RunPctExecShellScriptWithLog(sshClient infraSSH.Client, vmid int, script string, log LxcLogSink) ([]byte, error) {
	return runPctExecShellScriptWithTimeoutAndLog(sshClient, vmid, script, 0, log)
}

func runPctExecShellScriptWithTimeoutAndLog(sshClient infraSSH.Client, vmid int, script string, timeout time.Duration, log LxcLogSink) ([]byte, error) {
	command := `pct exec ` + infraSSH.ShellQuote(fmt.Sprintf("%d", vmid)) + ` -- sh -lc ` + infraSSH.ShellQuote(script)
	if timeout > 0 {
		command = `if command -v timeout >/dev/null 2>&1; then timeout ` + infraSSH.ShellQuote(fmt.Sprintf("%.0fs", timeout.Seconds())) + ` ` + command + `; else ` + command + `; fi`
	}
	out, err := sshClient.Run("sh -c " + infraSSH.ShellQuote(command))
	lxcCommandOutput(log, fmt.Sprintf("pct exec %d", vmid), out)
	if err != nil {
		return nil, infraSSH.FormatExecError(err, out)
	}
	return out, nil
}

func RunRemoteQuotedCommandWithLog(sshClient infraSSH.Client, log LxcLogSink, args ...string) ([]byte, error) {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, infraSSH.ShellQuote(arg))
	}

	command := strings.Join(quoted, " ")
	out, err := sshClient.Run(command)
	lxcCommandOutput(log, strings.Join(args, " "), out)
	if err != nil {
		return nil, infraSSH.FormatExecError(err, out)
	}

	return out, nil
}

func lxcStatusf(log LxcLogSink, format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if log == nil {
		fmt.Println(line)
		return
	}
	log(LxcLogEntry{Kind: LxcLogStatus, Body: line})
}

func lxcCommandOutput(log LxcLogSink, label string, out []byte) {
	if log == nil {
		return
	}
	if strings.HasPrefix(label, "pct status ") {
		return
	}
	output := strings.TrimSpace(string(out))
	if output == "" {
		return
	}
	log(LxcLogEntry{Kind: LxcLogCommand, Label: label, Body: output})
}

func WaitForLxcRunningOverSSH(sshClient infraSSH.Client, vmid int, timeout time.Duration) error {
	return WaitForLxcRunningOverSSHWithLog(sshClient, vmid, timeout, nil)
}

func WaitForLxcRunningOverSSHWithLog(sshClient infraSSH.Client, vmid int, timeout time.Duration, log LxcLogSink) error {
	deadline := time.Now().Add(timeout)
	lxcStatusf(log, "Waiting for container %d to report running status...", vmid)
	for {
		out, err := RunRemoteQuotedCommandWithLog(sshClient, log, "pct", "status", fmt.Sprintf("%d", vmid))
		if err == nil && strings.Contains(strings.ToLower(string(out)), "status: running") {
			lxcStatusf(log, "Container %d is running.", vmid)
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("timed out waiting for container %d to start: %w", vmid, err)
			}
			return fmt.Errorf("timed out waiting for container %d to start; latest status was %q", vmid, strings.TrimSpace(string(out)))
		}
		time.Sleep(3 * time.Second)
	}
}

func WaitForLxcIPv4OverSSH(sshClient infraSSH.Client, vmid int, timeout time.Duration) (string, error) {
	return WaitForLxcIPv4OverSSHWithLog(sshClient, vmid, timeout, nil)
}

func WaitForLxcIPv4OverSSHWithLog(sshClient infraSSH.Client, vmid int, timeout time.Duration, log LxcLogSink) (string, error) {
	deadline := time.Now().Add(timeout)
	lxcStatusf(log, "Waiting for container %d to report an IPv4 address...", vmid)
	for {
		ipAddr, err := GetLxcPrimaryIPv4OverSSHWithLog(sshClient, vmid, log)
		if err == nil && ipAddr != "" {
			return ipAddr, nil
		}
		statusOut, statusErr := RunRemoteQuotedCommandWithLog(sshClient, log, "pct", "status", fmt.Sprintf("%d", vmid))
		if statusErr == nil && !strings.Contains(strings.ToLower(string(statusOut)), "status: running") {
			return "", fmt.Errorf("container %d stopped while waiting for IPv4; latest status was %q", vmid, strings.TrimSpace(string(statusOut)))
		}
		if time.Now().After(deadline) {
			if err != nil {
				return "", fmt.Errorf("timed out waiting for IPv4 on container %d: %w", vmid, err)
			}
			return "", fmt.Errorf("timed out waiting for IPv4 on container %d", vmid)
		}
		time.Sleep(5 * time.Second)
	}
}

func GetLxcPrimaryIPv4OverSSH(sshClient infraSSH.Client, vmid int) (string, error) {
	return GetLxcPrimaryIPv4OverSSHWithLog(sshClient, vmid, nil)
}

func GetLxcPrimaryIPv4OverSSHWithLog(sshClient infraSSH.Client, vmid int, log LxcLogSink) (string, error) {
	out, err := runPctExecShellScriptWithTimeoutAndLog(sshClient, vmid, lxcPrimaryIPv4Script(), lxcPctExecProbeTimeout, log)
	if err != nil {
		return "", err
	}

	ipAddr := strings.TrimSpace(string(out))
	if ipAddr == "" {
		return "", fmt.Errorf("container has not reported an IPv4 address yet")
	}
	if net.ParseIP(ipAddr) == nil {
		return "", fmt.Errorf("container reported an invalid IPv4 address %q", ipAddr)
	}
	return ipAddr, nil
}

func lxcPrimaryIPv4Script() string {
	return `ip_addr=""
if command -v ip >/dev/null 2>&1; then
	ip_addr=$(ip -4 -o addr show scope global 2>/dev/null | awk '{split($4, a, "/"); if (a[1] !~ /^127\./) {print a[1]; exit}}')
fi
if [ -z "$ip_addr" ] && command -v hostname >/dev/null 2>&1; then
	ip_addr=$(hostname -I 2>/dev/null | tr ' ' '\n' | awk '/^[0-9]+\./ && $1 !~ /^127\./ {print $1; exit}')
fi
if [ -z "$ip_addr" ] && command -v ifconfig >/dev/null 2>&1; then
	ip_addr=$(ifconfig 2>/dev/null | awk '/inet / {for (i = 1; i <= NF; i++) {if ($i == "inet") ip=$(i+1); else if ($i ~ /^addr:/) {ip=$i; sub(/^addr:/, "", ip)}; if (ip !~ /^127\./ && ip ~ /^[0-9]+\./) {print ip; exit}}}')
fi
printf '%s' "$ip_addr"`
}

func VerifyLxcSSHFromProxmoxNode(sshClient infraSSH.Client, ipAddr string, sshPort uint) error {
	if sshPort == 0 {
		sshPort = 22
	}
	port := fmt.Sprintf("%d", sshPort)
	script := `if command -v ssh-keyscan >/dev/null 2>&1; then ssh-keyscan -T 5 -p ` + infraSSH.ShellQuote(port) + ` ` + infraSSH.ShellQuote(ipAddr) + ` >/dev/null 2>&1; else nc -z -w 5 ` + infraSSH.ShellQuote(ipAddr) + ` ` + infraSSH.ShellQuote(port) + ` >/dev/null 2>&1; fi`
	out, err := sshClient.Run("sh -c " + infraSSH.ShellQuote(script))
	if err != nil {
		return infraSSH.FormatExecError(err, out)
	}
	return nil
}
