package deployment

import (
	"fmt"
	"path/filepath"
	"strings"
)

func shellJoinWords(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		quoted = append(quoted, ShellQuote(value))
	}
	return strings.Join(quoted, " ")
}

func formatRemoteCommandError(err error, out []byte) error {
	output := strings.TrimSpace(string(out))
	if output == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, output)
}

func shellCommandWithOptionalSudo(remoteUser, cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(remoteUser), "root") {
		return cmd
	}
	return "sudo " + cmd
}

func shellWordsWithOptionalSudo(remoteUser string, values []string) []string {
	if strings.EqualFold(strings.TrimSpace(remoteUser), "root") {
		return values
	}
	return append([]string{"sudo"}, values...)
}

func expandLocalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	return filepath.Clean(ExpandPath(path))
}
