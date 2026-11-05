package sites

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// SiteAccessPasswordRule is a projection of the actual Nginx directives and
// password file. It never contains the password or its hash.
type SiteAccessPasswordRule struct {
	ID               string   `json:"id"`
	Path             string   `json:"path"`
	Modifier         string   `json:"modifier,omitempty"`
	Scope            string   `json:"scope"`
	Realm            string   `json:"realm"`
	Usernames        []string `json:"usernames"`
	AccountsReadable bool     `json:"accountsReadable"`
	ResourceVersion  string   `json:"resourceVersion"`
}

const (
	maxAccessPasswordRules        = 128
	maxAccessPasswordReadBytes    = 8 << 20
	maxAccessPasswordReadAccounts = 4096
)

// AccessPasswordRules reads the active on-disk site config each time. Unknown
// or temporarily unreadable password files do not make their rule disappear.
func (m *Manager) AccessPasswordRules(siteID string) ([]SiteAccessPasswordRule, error) {
	sites, err := m.discoverer.Discover()
	if err != nil {
		return nil, fmt.Errorf("%w: discover sites: %v", ErrUnavailable, err)
	}
	for _, site := range sites {
		if site.ID != siteID {
			continue
		}
		_, _, config, err := m.verifiedDeleteConfig(site)
		if err != nil || config == nil {
			return nil, fmt.Errorf("%w: site configuration unavailable", ErrUnavailable)
		}
		directives, err := parseAccessPasswordDirectives(string(config))
		if err != nil {
			return nil, fmt.Errorf("%w: unable to parse site configuration", ErrUnprocessable)
		}
		if len(directives) > maxAccessPasswordRules {
			return nil, fmt.Errorf("%w: too many access password rules", ErrUnprocessable)
		}
		result := make([]SiteAccessPasswordRule, 0, len(directives))
		readBytes, readAccounts := 0, 0
		for _, directive := range directives {
			identity := []string{siteID, strconv.Itoa(directive.ServerIndex), directive.Scope, directive.Modifier, directive.Context, directive.UserFile}
			rule := SiteAccessPasswordRule{
				ID: stableID(identity...), Path: directive.Context, Modifier: directive.Modifier,
				Scope: directive.Scope, Realm: directive.Realm, Usernames: []string{},
			}
			version := []byte(hashBytes(config))
			version = append(version, []byte("\x00"+strings.Join(identity, "\x00"))...)
			data, readErr := m.readAccessPasswordFile(directive.UserFile)
			if readErr == nil {
				readBytes += len(data)
				if readBytes > maxAccessPasswordReadBytes {
					return nil, fmt.Errorf("%w: access password files exceed total size limit", ErrUnprocessable)
				}
				version = append(version, 0)
				version = append(version, data...)
				if users, parseErr := accessPasswordUsernames(data); parseErr == nil {
					readAccounts += len(users)
					if readAccounts > maxAccessPasswordReadAccounts {
						return nil, fmt.Errorf("%w: too many access password accounts", ErrUnprocessable)
					}
					rule.Usernames, rule.AccountsReadable = users, true
				}
			} else {
				version = append(version, []byte("\x00unreadable")...)
			}
			rule.ResourceVersion = hashBytes(version)
			result = append(result, rule)
		}
		return result, nil
	}
	return nil, fmt.Errorf("%w: site does not exist", ErrConflict)
}

// Translate documented Nginx container mounts only. os.Root confines opens to
// the mount, including when an untrusted path contains a symbolic link.
func (m *Manager) readAccessPasswordFile(nginxPath string) ([]byte, error) {
	if strings.ContainsAny(nginxPath, "$\x00\\") || path.Clean(nginxPath) != nginxPath || nginxPath == ".." || strings.HasPrefix(nginxPath, "../") {
		return nil, fmt.Errorf("invalid password file path")
	}
	if !strings.HasPrefix(nginxPath, "/") {
		nginxPath = "/etc/nginx/" + nginxPath
	}
	for prefix, directory := range map[string]string{
		"/etc/nginx/conf.d/": "conf.d",
		"/etc/nginx/certs/":  "certs",
		"/var/www/html/":     "html",
	} {
		if !strings.HasPrefix(nginxPath, prefix) {
			continue
		}
		relative := strings.TrimPrefix(nginxPath, prefix)
		if relative == "" {
			return nil, fmt.Errorf("invalid password file path")
		}
		rootPath := filepath.Join(m.webRoot, directory)
		rootInfo, err := os.Lstat(rootPath)
		if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("password file mount unavailable")
		}
		root, err := os.OpenRoot(rootPath)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		openedRoot, err := root.Stat(".")
		if err != nil || !os.SameFile(rootInfo, openedRoot) {
			return nil, fmt.Errorf("password file mount changed")
		}
		name := filepath.FromSlash(relative)
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxAccessPasswordFileBytes {
			return nil, fmt.Errorf("password file unavailable")
		}
		file, err := root.Open(name)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		opened, err := file.Stat()
		if err != nil || !os.SameFile(info, opened) {
			return nil, fmt.Errorf("password file changed")
		}
		data, err := io.ReadAll(io.LimitReader(file, maxAccessPasswordFileBytes+1))
		if err != nil || len(data) > maxAccessPasswordFileBytes {
			return nil, fmt.Errorf("password file unavailable")
		}
		return data, nil
	}
	return nil, fmt.Errorf("password file mount is unavailable")
}
