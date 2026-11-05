package sites

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxAccessPasswordFileBytes = 1 << 20
	maxAccessPasswordUsers     = 4096
)

// accessPasswordUsernames projects only account names. Password hashes remain
// in the Nginx password file and must never be returned in an API response.
func accessPasswordUsernames(content []byte) ([]string, error) {
	if len(content) > maxAccessPasswordFileBytes {
		return nil, fmt.Errorf("password file exceeds size limit")
	}
	var names []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, hash, ok := strings.Cut(line, ":")
		if !ok || name == "" || hash == "" || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00\r\n") {
			return nil, fmt.Errorf("invalid password file entry")
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate password file account")
		}
		seen[name] = true
		names = append(names, name)
		if len(names) > maxAccessPasswordUsers {
			return nil, fmt.Errorf("password file exceeds account limit")
		}
	}
	return names, nil
}
