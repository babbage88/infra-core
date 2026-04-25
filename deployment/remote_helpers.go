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

func expandLocalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	return filepath.Clean(ExpandPath(path))
}
