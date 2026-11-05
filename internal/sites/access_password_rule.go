package sites

import (
	"context"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type AccessPasswordRuleInput struct {
	Action          string `json:"action"`
	ResourceVersion string `json:"resourceVersion"`
	RuleID          string `json:"ruleId,omitempty"`
	Scope           string `json:"scope,omitempty"`
	Path            string `json:"path,omitempty"`
	Modifier        string `json:"modifier,omitempty"`
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
}

type accessRuleAddRequest struct {
	Operation     string `json:"operation"`
	ConfigName    string `json:"configName"`
	ConfigVersion string `json:"configVersion"`
	ServerIndices []int  `json:"serverIndices"`
	Scope         string `json:"scope"`
	Path          string `json:"path"`
	Modifier      string `json:"modifier"`
	Username      string `json:"username"`
	Password      string `json:"password"`
}

type accessRuleDeleteRequest struct {
	Operation     string `json:"operation"`
	ConfigName    string `json:"configName"`
	ConfigVersion string `json:"configVersion"`
	FileVersion   string `json:"fileVersion"`
	ServerIndex   int    `json:"serverIndex"`
	Scope         string `json:"scope"`
	Path          string `json:"path"`
	Modifier      string `json:"modifier"`
}

func validateAccessRuleInput(input AccessPasswordRuleInput) error {
	_, versionErr := hex.DecodeString(strings.TrimPrefix(input.ResourceVersion, "sha256:"))
	if !strings.HasPrefix(input.ResourceVersion, "sha256:") || len(input.ResourceVersion) != 71 || versionErr != nil {
		return fmt.Errorf("%w: invalid resource version", ErrInvalidInput)
	}
	switch input.Action {
	case "add":
		if input.RuleID != "" || !validAccessAccountName(input.Username) || input.Password == "" || len(input.Password) > 1024 || !utf8.ValidString(input.Password) || strings.ContainsAny(input.Password, "\x00\r\n") {
			return fmt.Errorf("%w: invalid access rule", ErrInvalidInput)
		}
		if input.Scope == "server" {
			if input.Path != "" || input.Modifier != "" {
				return fmt.Errorf("%w: invalid server scope", ErrInvalidInput)
			}
		} else if input.Scope == "location" {
			if input.Path == "" || len(input.Path) > 1024 || !utf8.ValidString(input.Path) || strings.ContainsAny(input.Path, "\x00\r\n") ||
				(input.Modifier != "" && input.Modifier != "=" && input.Modifier != "^~" && input.Modifier != "~" && input.Modifier != "~*") {
				return fmt.Errorf("%w: invalid path scope", ErrInvalidInput)
			}
		} else {
			return fmt.Errorf("%w: invalid access scope", ErrInvalidInput)
		}
	case "delete":
		_, idErr := hex.DecodeString(input.RuleID)
		if len(input.RuleID) != 32 || idErr != nil || strings.ToLower(input.RuleID) != input.RuleID || input.Scope != "" || input.Path != "" || input.Modifier != "" || input.Username != "" || input.Password != "" {
			return fmt.Errorf("%w: invalid deletion input", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: invalid access rule action", ErrInvalidInput)
	}
	return nil
}

// EditAccessPasswordRule resolves the target against current Nginx artifacts,
// including rules that were written or modified outside the panel.
func (m *Manager) EditAccessPasswordRule(ctx context.Context, siteID string, input AccessPasswordRuleInput) ([]SiteAccessPasswordRule, error) {
	if err := validateAccessRuleInput(input); err != nil {
		return nil, err
	}
	siteWriteMutex.Lock()
	defer siteWriteMutex.Unlock()
	items, err := m.discoverer.Discover()
	if err != nil {
		return nil, fmt.Errorf("%w: site discovery failed", ErrUnavailable)
	}
	for _, site := range items {
		if site.ID != siteID {
			continue
		}
		configPath, _, config, err := m.verifiedDeleteConfig(site)
		if err != nil || config == nil {
			return nil, fmt.Errorf("%w: site configuration changed", ErrConflict)
		}
		var request any
		if input.Action == "add" {
			if site.ResourceVersion != input.ResourceVersion {
				return nil, fmt.Errorf("%w: site changed", ErrConflict)
			}
			indices := make([]int, 0)
			for index, server := range nginxServers(string(config)) {
				for _, domain := range validDomains(server.names) {
					if containsString(site.Domains, domain) {
						indices = append(indices, index)
						break
					}
				}
			}
			if len(indices) == 0 || len(indices) > 64 {
				return nil, fmt.Errorf("%w: matching server contexts unavailable", ErrUnprocessable)
			}
			request = accessRuleAddRequest{Operation: "rule-add", ConfigName: filepath.Base(configPath), ConfigVersion: strings.TrimPrefix(hashBytes(config), "sha256:"), ServerIndices: indices, Scope: input.Scope, Path: input.Path, Modifier: input.Modifier, Username: input.Username, Password: input.Password}
		} else {
			directives, parseErr := parseAccessPasswordDirectives(string(config))
			if parseErr != nil {
				return nil, fmt.Errorf("%w: site configuration unavailable", ErrUnprocessable)
			}
			for _, directive := range directives {
				identity := []string{siteID, strconv.Itoa(directive.ServerIndex), directive.Scope, directive.Modifier, directive.Context, directive.UserFile}
				if stableID(identity...) != input.RuleID {
					continue
				}
				file, readErr := m.readAccessPasswordFile(directive.UserFile)
				if readErr != nil {
					return nil, fmt.Errorf("%w: password file unavailable", ErrUnprocessable)
				}
				version := append([]byte(hashBytes(config)), []byte("\x00"+strings.Join(identity, "\x00"))...)
				version = append(append(version, 0), file...)
				if hashBytes(version) != input.ResourceVersion {
					return nil, fmt.Errorf("%w: access rule changed", ErrConflict)
				}
				path := directive.Context
				if directive.Scope == "server" {
					path = ""
				}
				request = accessRuleDeleteRequest{Operation: "rule-delete", ConfigName: filepath.Base(configPath), ConfigVersion: strings.TrimPrefix(hashBytes(config), "sha256:"), FileVersion: strings.TrimPrefix(hashBytes(file), "sha256:"), ServerIndex: directive.ServerIndex, Scope: directive.Scope, Path: path, Modifier: directive.Modifier}
				break
			}
			if request == nil {
				return nil, fmt.Errorf("%w: access rule changed", ErrConflict)
			}
		}
		if m.accessAccountEditor == nil {
			return nil, fmt.Errorf("%w: password adapter unavailable", ErrUnavailable)
		}
		if err := m.accessAccountEditor.Edit(ctx, m.webRoot, filepath.Base(configPath), request); err != nil {
			return nil, err
		}
		return m.AccessPasswordRules(siteID)
	}
	return nil, fmt.Errorf("%w: site does not exist", ErrConflict)
}
