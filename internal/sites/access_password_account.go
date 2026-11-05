package sites

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/jobcontrol"
)

type AccessPasswordAccountInput struct {
	RuleID           string `json:"ruleId"`
	ResourceVersion  string `json:"resourceVersion"`
	Action           string `json:"action"`
	Username         string `json:"username"`
	PreviousUsername string `json:"previousUsername,omitempty"`
	Password         string `json:"password,omitempty"`
}

type accessAccountRequest struct {
	Operation        string `json:"operation"`
	ConfigName       string `json:"configName"`
	ConfigVersion    string `json:"configVersion"`
	FileVersion      string `json:"fileVersion"`
	ServerIndex      int    `json:"serverIndex"`
	Scope            string `json:"scope"`
	Path             string `json:"path"`
	Modifier         string `json:"modifier"`
	Username         string `json:"username"`
	PreviousUsername string `json:"previousUsername,omitempty"`
	Password         string `json:"password,omitempty"`
}

type siteAccessAccountEditor interface {
	Available(string) error
	Edit(context.Context, string, string, any) error
}

type scriptSiteAccessAccountEditor struct{}

func (scriptSiteAccessAccountEditor) Available(webRoot string) error {
	if runtime.GOOS != "linux" || filepath.Clean(webRoot) != "/home/web" {
		return fmt.Errorf("password adapter requires the script Web root")
	}
	if _, err := findTrustedKejilionScript(`KPANEL_WEB_ACCESS_PASSWORD_PROTOCOL_VERSION="1"`, "kpanel_web_access_password()"); err != nil {
		return err
	}
	return jobcontrol.Available(systemRecipeJobRunner{})
}

func (m *Manager) AccessPasswordWritable() error {
	if m.accessAccountEditor == nil {
		return fmt.Errorf("password adapter unavailable")
	}
	return m.accessAccountEditor.Available(m.webRoot)
}

func validAccessAccountName(name string) bool {
	if name == "" || len(name) > 256 || !utf8.ValidString(name) || strings.HasPrefix(name, "#") {
		return false
	}
	return !strings.ContainsAny(name, ":\r\n\x00") && strings.IndexFunc(name, unicode.IsControl) < 0
}

func validateAccessAccountInput(input AccessPasswordAccountInput) error {
	_, idErr := hex.DecodeString(input.RuleID)
	_, versionErr := hex.DecodeString(strings.TrimPrefix(input.ResourceVersion, "sha256:"))
	if len(input.RuleID) != 32 || idErr != nil || strings.ToLower(input.RuleID) != input.RuleID ||
		!strings.HasPrefix(input.ResourceVersion, "sha256:") || len(input.ResourceVersion) != 71 || versionErr != nil ||
		!validAccessAccountName(input.Username) {
		return fmt.Errorf("%w: invalid access account input", ErrInvalidInput)
	}
	switch input.Action {
	case "add":
		if input.PreviousUsername != "" {
			return fmt.Errorf("%w: unexpected previous account", ErrInvalidInput)
		}
	case "update":
		if !validAccessAccountName(input.PreviousUsername) {
			return fmt.Errorf("%w: previous account required", ErrInvalidInput)
		}
	case "delete":
		if input.PreviousUsername != "" || input.Password != "" {
			return fmt.Errorf("%w: unexpected deletion input", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: invalid account action", ErrInvalidInput)
	}
	if input.Action != "delete" && (input.Password == "" || len(input.Password) > 256 || !utf8.ValidString(input.Password) || strings.ContainsAny(input.Password, "\x00\r\n")) {
		return fmt.Errorf("%w: invalid access password", ErrInvalidInput)
	}
	return nil
}

// EditAccessPasswordAccount resolves the rule again from the current config;
// callers cannot choose an arbitrary privileged password file.
func (m *Manager) EditAccessPasswordAccount(ctx context.Context, siteID string, input AccessPasswordAccountInput) ([]SiteAccessPasswordRule, error) {
	if err := validateAccessAccountInput(input); err != nil {
		return nil, err
	}
	siteWriteMutex.Lock()
	defer siteWriteMutex.Unlock()
	rules, err := m.AccessPasswordRules(siteID)
	if err != nil {
		return nil, err
	}
	var matched *SiteAccessPasswordRule
	for i := range rules {
		if rules[i].ID == input.RuleID {
			matched = &rules[i]
			break
		}
	}
	if matched == nil || matched.ResourceVersion != input.ResourceVersion {
		return nil, fmt.Errorf("%w: access password rule changed", ErrConflict)
	}
	if !matched.AccountsReadable {
		return nil, fmt.Errorf("%w: password file unavailable", ErrUnprocessable)
	}
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
		directives, err := parseAccessPasswordDirectives(string(config))
		if err != nil {
			return nil, fmt.Errorf("%w: site configuration unavailable", ErrUnprocessable)
		}
		for _, directive := range directives {
			identity := []string{siteID, strconv.Itoa(directive.ServerIndex), directive.Scope, directive.Modifier, directive.Context, directive.UserFile}
			if stableID(identity...) != input.RuleID {
				continue
			}
			file, err := m.readAccessPasswordFile(directive.UserFile)
			if err != nil {
				return nil, fmt.Errorf("%w: password file unavailable", ErrUnprocessable)
			}
			version := append([]byte(hashBytes(config)), []byte("\x00"+strings.Join(identity, "\x00"))...)
			version = append(append(version, 0), file...)
			if hashBytes(version) != input.ResourceVersion {
				return nil, fmt.Errorf("%w: access password rule changed", ErrConflict)
			}
			path := directive.Context
			if directive.Scope == "server" {
				path = ""
			}
			request := accessAccountRequest{
				Operation: input.Action, ConfigName: filepath.Base(configPath),
				ConfigVersion: strings.TrimPrefix(hashBytes(config), "sha256:"), FileVersion: strings.TrimPrefix(hashBytes(file), "sha256:"),
				ServerIndex: directive.ServerIndex, Scope: directive.Scope, Path: path, Modifier: directive.Modifier,
				Username: input.Username, PreviousUsername: input.PreviousUsername, Password: input.Password,
			}
			if m.accessAccountEditor == nil {
				return nil, fmt.Errorf("%w: password adapter unavailable", ErrUnavailable)
			}
			if err := m.accessAccountEditor.Edit(ctx, m.webRoot, request.ConfigName, request); err != nil {
				return nil, err
			}
			return m.AccessPasswordRules(siteID)
		}
	}
	return nil, fmt.Errorf("%w: access password rule changed", ErrConflict)
}

func (scriptSiteAccessAccountEditor) Edit(ctx context.Context, webRoot, configName string, input any) error {
	if runtime.GOOS != "linux" || filepath.Clean(webRoot) != "/home/web" {
		return fmt.Errorf("%w: password adapter requires the script Web root", ErrUnavailable)
	}
	script, err := findTrustedKejilionScript(`KPANEL_WEB_ACCESS_PASSWORD_PROTOCOL_VERSION="1"`, "kpanel_web_access_password()")
	if err != nil {
		return fmt.Errorf("%w: password adapter unavailable", ErrUnavailable)
	}
	runner := systemRecipeJobRunner{}
	if err := jobcontrol.Available(runner); err != nil {
		return fmt.Errorf("%w: password worker unavailable", ErrUnavailable)
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("%w: invalid account input", ErrInvalidInput)
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 210*time.Second)
	defer cancel()
	spec := jobcontrol.Spec{
		Unit:       "kpanel-site-access-" + stableID(configName, time.Now().UTC().String())[:24],
		Executable: "/bin/bash", Arguments: []string{script, "web", "access-password"},
		Environment: []string{"KJ_WEB_NONINTERACTIVE=1"}, StateDir: "/var/lib/kejilion-panel/wordpress-jobs",
		SystemdProperties: []string{"Type=exec", "RuntimeMaxSec=180s", "TimeoutStopSec=15s", "User=root", "UMask=0077"}, UMask: "0077",
	}
	name, args, _, err := jobcontrol.ForegroundInvocation(runner, spec)
	if err != nil {
		return fmt.Errorf("%w: password worker unavailable", ErrUnavailable)
	}
	command := exec.CommandContext(ctx, name, args...)
	command.Env = siteCommandEnvironment(spec.Environment)
	command.Stdin = bytes.NewReader(payload)
	var receipt certificateReceipt
	command.Stdout = &receipt
	err = command.Run()
	var result struct {
		OK          bool   `json:"ok"`
		Error       string `json:"error"`
		FileVersion string `json:"fileVersion"`
	}
	if json.Unmarshal(receipt.value, &result) != nil {
		return fmt.Errorf("%w: password adapter receipt missing", ErrNeedsAttention)
	}
	if err != nil || !result.OK {
		switch result.Error {
		case "resource_conflict":
			return fmt.Errorf("%w: password resource changed", ErrConflict)
		case "invalid_request":
			return fmt.Errorf("%w: password operation invalid", ErrUnprocessable)
		case "needs_attention":
			return fmt.Errorf("%w: password operation needs attention", ErrNeedsAttention)
		default:
			return fmt.Errorf("%w: password operation failed", ErrUnavailable)
		}
	}
	if len(result.FileVersion) != 64 {
		return fmt.Errorf("%w: invalid password adapter receipt", ErrNeedsAttention)
	}
	return nil
}
