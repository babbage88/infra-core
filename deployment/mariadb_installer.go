package deployment

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/babbage88/goph/v2"
	infraSSH "github.com/babbage88/infra-core/ssh"
)

type MariaDBInstallLogKind string

const (
	MariaDBInstallLogStatus  MariaDBInstallLogKind = "status"
	MariaDBInstallLogCommand MariaDBInstallLogKind = "command"
	MariaDBInstallLogStdout  MariaDBInstallLogKind = "stdout"
	MariaDBInstallLogStderr  MariaDBInstallLogKind = "stderr"
)

type MariaDBInstallLogEntry struct {
	Kind  MariaDBInstallLogKind
	Label string
	Body  string
}

type MariaDBInstallLogSink func(MariaDBInstallLogEntry)

type RemoteMariaDBInstaller struct {
	RemoteHostname string
	SshClient      *goph.Client
	RemoteSshUser  string
	OsInfo         map[string]string
	Log            MariaDBInstallLogSink
}

func NewRemoteMariaDBInstallerWithSsh(hostname, sshUser, sshKey, sshPassphrase string, useSshAgent bool, port uint) (*RemoteMariaDBInstaller, error) {
	client, err := InitializeSshClient(hostname, sshUser, sshKey, sshPassphrase, useSshAgent, port)
	if err != nil {
		return nil, err
	}

	return &RemoteMariaDBInstaller{
		SshClient:      client,
		RemoteHostname: hostname,
		RemoteSshUser:  sshUser,
	}, nil
}

func (rmi *RemoteMariaDBInstaller) EnsureInstalledAndConfigured(dbName, username, password, bind string, port int) error {
	if strings.TrimSpace(dbName) == "" {
		return fmt.Errorf("database name is required")
	}
	if strings.TrimSpace(username) == "" {
		return fmt.Errorf("database user is required")
	}
	if strings.TrimSpace(password) == "" {
		return fmt.Errorf("database password is required")
	}

	mariaDBInstallStatusf(rmi.Log, "Connecting to %s and inspecting the host...", rmi.RemoteHostname)
	if err := rmi.loadOsInfo(); err != nil {
		return err
	}
	mariaDBInstallStatusf(rmi.Log, "Ensuring MariaDB packages are installed...")
	if err := rmi.ensureMariaDBInstalled(); err != nil {
		return err
	}
	mariaDBInstallStatusf(rmi.Log, "Locating the MariaDB configuration file...")
	configPath, err := rmi.detectConfigPath()
	if err != nil {
		return err
	}
	mariaDBInstallStatusf(rmi.Log, "Updating MariaDB remote access settings in %s...", configPath)
	if err := rmi.configureRemoteAccess(configPath, bind, port); err != nil {
		return err
	}
	mariaDBInstallStatusf(rmi.Log, "Restarting the MariaDB service...")
	if err := rmi.restartService(); err != nil {
		return err
	}
	mariaDBInstallStatusf(rmi.Log, "Creating database %s and user %s...", dbName, username)
	return rmi.createDatabaseAndUser(dbName, username, password)
}

func (rmi *RemoteMariaDBInstaller) loadOsInfo() error {
	if rmi.OsInfo != nil {
		return nil
	}
	out, err := rmi.runRemoteCommand("read /etc/os-release", "cat /etc/os-release", "cat /etc/os-release")
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("read /etc/os-release: %w", err), out)
	}
	osInfoMap := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		osInfoMap[key] = value
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	rmi.OsInfo = osInfoMap
	return nil
}

func (rmi *RemoteMariaDBInstaller) ensureMariaDBInstalled() error {
	checkScript := `if command -v mariadbd >/dev/null 2>&1 || command -v mysqld >/dev/null 2>&1; then
  if command -v mariadb >/dev/null 2>&1 || command -v mysql >/dev/null 2>&1; then
    exit 0
  fi
fi
exit 1`
	if out, err := rmi.runRemoteCommand("check MariaDB installation", "sh -c "+ShellQuote(checkScript), "sh -c "+ShellQuote(checkScript)); err == nil {
		_ = out
		mariaDBInstallStatusf(rmi.Log, "MariaDB is already installed.")
		return nil
	}
	osID := rmi.OsInfo["ID"]
	osLike := rmi.OsInfo["ID_LIKE"]
	combinedID := strings.TrimSpace(osID + " " + osLike)

	var installCmd string
	switch {
	case strings.Contains(combinedID, "ubuntu"), strings.Contains(combinedID, "debian"):
		installCmd = shellCommandWithOptionalSudo(rmi.RemoteSshUser, "apt-get update -y") + " && " + shellCommandWithOptionalSudo(rmi.RemoteSshUser, "DEBIAN_FRONTEND=noninteractive apt-get install -y mariadb-server mariadb-client")
	case strings.Contains(combinedID, "rhel"), strings.Contains(combinedID, "fedora"), strings.Contains(combinedID, "centos"), strings.Contains(combinedID, "rocky"), strings.Contains(combinedID, "alma"), strings.Contains(combinedID, "amzn"):
		installCmd = fmt.Sprintf(`if command -v dnf >/dev/null 2>&1; then %s; elif command -v yum >/dev/null 2>&1; then %s; else echo "No supported package manager found for MariaDB install" >&2; exit 1; fi`,
			shellCommandWithOptionalSudo(rmi.RemoteSshUser, "dnf install -y mariadb-server mariadb"),
			shellCommandWithOptionalSudo(rmi.RemoteSshUser, "yum install -y mariadb-server mariadb"),
		)
	case strings.Contains(combinedID, "arch"):
		installCmd = shellCommandWithOptionalSudo(rmi.RemoteSshUser, "pacman -Sy --noconfirm mariadb")
	case strings.Contains(combinedID, "suse"), strings.Contains(combinedID, "opensuse"):
		installCmd = shellCommandWithOptionalSudo(rmi.RemoteSshUser, "zypper --non-interactive install mariadb mariadb-client")
	case strings.Contains(combinedID, "alpine"):
		installCmd = shellCommandWithOptionalSudo(rmi.RemoteSshUser, "apk add --no-cache mariadb mariadb-client")
	default:
		return fmt.Errorf("unsupported OS for automatic MariaDB installation: id=%q id_like=%q", osID, osLike)
	}

	out, err := rmi.runRemoteCommand("install MariaDB packages", "sh -c "+ShellQuote(installCmd), "sh -c "+ShellQuote(installCmd))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("install mariadb via distro package manager: %w", err), out)
	}
	verifyScript := `if command -v mariadbd >/dev/null 2>&1 || command -v mysqld >/dev/null 2>&1; then
  if command -v mariadb >/dev/null 2>&1 || command -v mysql >/dev/null 2>&1; then
    exit 0
  fi
fi
exit 1`
	out, err = rmi.runRemoteCommand("verify MariaDB installation", "sh -c "+ShellQuote(verifyScript), "sh -c "+ShellQuote(verifyScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("verify mariadb installation: %w", err), out)
	}
	mariaDBInstallStatusf(rmi.Log, "MariaDB packages are installed.")
	return nil
}

func (rmi *RemoteMariaDBInstaller) detectConfigPath() (string, error) {
	findConfigScript := `for file in /etc/mysql/mariadb.conf.d/50-server.cnf /etc/my.cnf.d/mariadb-server.cnf /etc/my.cnf /etc/mysql/my.cnf /etc/my.cnf.d/server.cnf; do
  if [ -f "$file" ]; then
    printf '%s\n' "$file"
    exit 0
  fi
done
find /etc -maxdepth 4 -type f \( -name 'my.cnf' -o -name '*.cnf' \) 2>/dev/null | grep -E 'maria|mysql' | head -n 1`
	out, err := rmi.runRemoteCommand("locate MariaDB config", "sh -c "+ShellQuote(findConfigScript), "sh -c "+ShellQuote(findConfigScript))
	if err != nil {
		return "", formatRemoteCommandError(fmt.Errorf("locate mariadb config: %w", err), out)
	}
	configPath := strings.TrimSpace(string(out))
	if configPath == "" {
		return "", fmt.Errorf("could not locate a MariaDB configuration file on remote host")
	}
	return configPath, nil
}

func (rmi *RemoteMariaDBInstaller) configureRemoteAccess(configPath, bind string, port int) error {
	configScript := fmt.Sprintf(
		`set -e
config_file=%s
ensure_mysqld_section() {
  grep -Eq '^[[:space:]]*\[mysqld\][[:space:]]*$' "$config_file" || printf "\n[mysqld]\n" >> "$config_file"
}
if grep -Eq '^[#[:space:]]*bind-address[[:space:]]*=' "$config_file"; then
  sed -i "s/^[#[:space:]]*bind-address[[:space:]]*=.*/bind-address = %s/" "$config_file"
else
  ensure_mysqld_section
  printf "bind-address = %s\n" >> "$config_file"
fi
if grep -Eq '^[#[:space:]]*port[[:space:]]*=' "$config_file"; then
  sed -i "s/^[#[:space:]]*port[[:space:]]*=.*/port = %s/" "$config_file"
else
  ensure_mysqld_section
  printf "port = %s\n" >> "$config_file"
fi
sed -i 's/^[[:space:]]*skip-networking/# skip-networking/' "$config_file" || true`,
		ShellQuote(configPath),
		bind,
		bind,
		strconv.Itoa(port),
		strconv.Itoa(port),
	)
	out, err := rmi.runRemoteCommand("configure MariaDB remote access", shellCommandWithOptionalSudo(rmi.RemoteSshUser, "sh -c "+ShellQuote(configScript)), shellCommandWithOptionalSudo(rmi.RemoteSshUser, "sh -c "+ShellQuote(configScript)))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("configure mariadb remote access: %w", err), out)
	}
	return nil
}

func (rmi *RemoteMariaDBInstaller) restartService() error {
	sudoPrefix := ""
	if !strings.EqualFold(strings.TrimSpace(rmi.RemoteSshUser), "root") {
		sudoPrefix = "sudo "
	}
	restartScript := fmt.Sprintf(
		`set -e
for svc in %s $(systemctl list-unit-files 'mariadb*' 'mysql*' --type=service --no-legend 2>/dev/null | awk '{print $1}') $(systemctl list-units --all 'mariadb*' 'mysql*' --type=service --no-legend 2>/dev/null | awk '{print $1}'); do
  [ -n "$svc" ] || continue
  svc=${svc%%.service}
  if %ssystemctl restart "$svc" >/dev/null 2>&1; then
    exit 0
  fi
  if %ssystemctl start "$svc" >/dev/null 2>&1; then
    exit 0
  fi
  if command -v service >/dev/null 2>&1 && %sservice "$svc" restart >/dev/null 2>&1; then
    exit 0
  fi
done
if command -v mariadb-install-db >/dev/null 2>&1 && [ ! -d /var/lib/mysql/mysql ]; then
  %smariadb-install-db --user=mysql --datadir=/var/lib/mysql >/dev/null 2>&1 || true
  for svc in mariadb mysql mysqld; do
    if %ssystemctl start "$svc" >/dev/null 2>&1; then
      exit 0
    fi
  done
fi
for svc in mariadb mysql mysqld; do
  if systemctl status "$svc" >/dev/null 2>&1; then
    systemctl --no-pager --full status "$svc" || true
    journalctl -u "$svc" -n 25 --no-pager || true
    break
  fi
done
echo "Unable to restart MariaDB service. Tried common MariaDB/MySQL unit names and detected services." >&2
exit 1`,
		shellJoinWords([]string{"mariadb", "mariadb.service", "mysql", "mysql.service", "mysqld", "mysqld.service"}),
		sudoPrefix,
		sudoPrefix,
		sudoPrefix,
		sudoPrefix,
		sudoPrefix,
	)
	out, err := rmi.runRemoteCommand("restart MariaDB service", "sh -c "+ShellQuote(restartScript), "sh -c "+ShellQuote(restartScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("restart mariadb service: %w", err), out)
	}
	return nil
}

func (rmi *RemoteMariaDBInstaller) createDatabaseAndUser(dbName, username, password string) error {
	sqlStatements := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", quoteMariaDBIdentifier(dbName)),
		fmt.Sprintf("CREATE USER IF NOT EXISTS %s@'%%' IDENTIFIED BY %s", quoteMariaDBString(username), quoteMariaDBString(password)),
		fmt.Sprintf("ALTER USER %s@'%%' IDENTIFIED BY %s", quoteMariaDBString(username), quoteMariaDBString(password)),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON %s.* TO %s@'%%'", quoteMariaDBIdentifier(dbName), quoteMariaDBString(username)),
		"FLUSH PRIVILEGES",
	}
	sql := strings.Join(sqlStatements, "; ") + ";"
	mariaDBCmd := shellJoinWords(shellWordsWithOptionalSudo(rmi.RemoteSshUser, []string{"mariadb", "-e", sql}))
	mysqlCmd := shellJoinWords(shellWordsWithOptionalSudo(rmi.RemoteSshUser, []string{"mysql", "-e", sql}))
	sqlScript := fmt.Sprintf(
		`if command -v mariadb >/dev/null 2>&1; then
  exec %s
elif command -v mysql >/dev/null 2>&1; then
  exec %s
else
  echo "No mariadb/mysql client found" >&2
  exit 1
fi`,
		mariaDBCmd,
		mysqlCmd,
	)
	redactedSQL := strings.Join([]string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", quoteMariaDBIdentifier(dbName)),
		fmt.Sprintf("CREATE USER IF NOT EXISTS %s@'%%' IDENTIFIED BY '********'", quoteMariaDBString(username)),
		fmt.Sprintf("ALTER USER %s@'%%' IDENTIFIED BY '********'", quoteMariaDBString(username)),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON %s.* TO %s@'%%'", quoteMariaDBIdentifier(dbName), quoteMariaDBString(username)),
		"FLUSH PRIVILEGES",
	}, "; ") + ";"
	redactedMariaDBCmd := shellJoinWords(shellWordsWithOptionalSudo(rmi.RemoteSshUser, []string{"mariadb", "-e", redactedSQL}))
	redactedMySQLCmd := shellJoinWords(shellWordsWithOptionalSudo(rmi.RemoteSshUser, []string{"mysql", "-e", redactedSQL}))
	redactedSQLScript := fmt.Sprintf(
		`if command -v mariadb >/dev/null 2>&1; then
  exec %s
elif command -v mysql >/dev/null 2>&1; then
  exec %s
else
  echo "No mariadb/mysql client found" >&2
  exit 1
fi`,
		redactedMariaDBCmd,
		redactedMySQLCmd,
	)
	out, err := rmi.runRemoteCommand("create MariaDB database and user", "sh -c "+ShellQuote(sqlScript), "sh -c "+ShellQuote(redactedSQLScript))
	if err != nil {
		return formatRemoteCommandError(fmt.Errorf("create mariadb database/user: %w", err), out)
	}
	return nil
}

func (rmi *RemoteMariaDBInstaller) runRemoteCommand(label, command, displayCommand string) ([]byte, error) {
	if rmi.Log == nil {
		return rmi.SshClient.Run(command)
	}

	cmd, err := rmi.SshClient.Command("sh", "-c", command)
	if err != nil {
		return nil, err
	}
	defer cmd.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	rmi.Log(MariaDBInstallLogEntry{Kind: MariaDBInstallLogCommand, Label: label, Body: displayCommand})

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)
	go streamMariaDBInstallOutput(stdout, &stdoutBuf, label, MariaDBInstallLogStdout, rmi.Log, &wg)
	go streamMariaDBInstallOutput(stderr, &stderrBuf, label, MariaDBInstallLogStderr, rmi.Log, &wg)

	waitErr := cmd.Wait()
	wg.Wait()

	combined := append([]byte{}, stdoutBuf.Bytes()...)
	combined = append(combined, stderrBuf.Bytes()...)
	if waitErr != nil {
		return combined, infraSSH.FormatExecError(waitErr, combined)
	}
	return combined, nil
}

func streamMariaDBInstallOutput(reader io.Reader, capture *bytes.Buffer, label string, kind MariaDBInstallLogKind, log MariaDBInstallLogSink, wg *sync.WaitGroup) {
	defer wg.Done()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		capture.WriteString(line)
		capture.WriteByte('\n')
		if log != nil {
			log(MariaDBInstallLogEntry{Kind: kind, Label: label, Body: line})
		}
	}
	if err := scanner.Err(); err != nil && log != nil {
		log(MariaDBInstallLogEntry{Kind: MariaDBInstallLogStderr, Label: label, Body: fmt.Sprintf("stream read error: %v", err)})
	}
}

func mariaDBInstallStatusf(log MariaDBInstallLogSink, format string, args ...any) {
	if log == nil {
		return
	}
	log(MariaDBInstallLogEntry{Kind: MariaDBInstallLogStatus, Body: fmt.Sprintf(format, args...)})
}

func quoteMariaDBIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func quoteMariaDBString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
