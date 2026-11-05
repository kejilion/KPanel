package sites

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type AccessPasswordCreateInput struct {
	Scope    string `json:"scope"`
	Path     string `json:"path,omitempty"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (m *Manager) verifyCreatedAccess(job RecipeJob, site contract.SiteSummary) error {
	if !job.AccessPassword {
		return nil
	}
	path, err := accessPasswordStagePath(m.recipeJobs.stateDir, job.ID)
	if err != nil {
		return err
	}
	data, err := readRegularFile(path, 16384)
	if err != nil {
		return errors.New("access password input unavailable")
	}
	var staged struct {
		Operation  string `json:"operation"`
		ConfigName string `json:"configName"`
		Scope      string `json:"scope"`
		Path       string `json:"path"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}
	if json.Unmarshal(data, &staged) != nil || staged.Operation != "create-stage" || staged.ConfigName != job.Domain+".conf" ||
		validateAccessPasswordCreate(&AccessPasswordCreateInput{Scope: staged.Scope, Path: staged.Path, Username: staged.Username, Password: staged.Password}) != nil {
		return errors.New("access password input invalid")
	}
	_, _, config, err := m.verifiedDeleteConfig(site)
	if err != nil || config == nil {
		return errors.New("created site configuration unavailable")
	}
	expected := 0
	redirectOnly, err := accessRedirectOnlyServers(string(config))
	if err != nil {
		return errors.New("created site server context unavailable")
	}
	for index, server := range nginxServers(string(config)) {
		if redirectOnly[index] {
			continue
		}
		for _, domain := range validDomains(server.names) {
			if containsString(site.Domains, domain) {
				expected++
				break
			}
		}
	}
	if expected == 0 {
		return errors.New("created site server context unavailable")
	}
	rules, err := m.AccessPasswordRules(site.ID)
	if err != nil {
		return err
	}
	matched := 0
	for _, rule := range rules {
		if rule.Scope == staged.Scope && (staged.Scope == "server" || rule.Path == staged.Path && rule.Modifier == "") && rule.AccountsReadable && containsString(rule.Usernames, staged.Username) {
			matched++
		}
	}
	if matched != expected {
		return errors.New("created access rules could not be reconciled")
	}
	return nil
}

func (m *Manager) prepareAccessPassword(input *AccessPasswordCreateInput) error {
	if err := validateAccessPasswordCreate(input); err != nil {
		return err
	}
	if input != nil {
		if err := m.AccessPasswordWritable(); err != nil {
			return fmt.Errorf("%w: access password adapter unavailable", ErrUnavailable)
		}
	}
	return nil
}

func validateAccessPasswordCreate(input *AccessPasswordCreateInput) error {
	if input == nil {
		return nil
	}
	if !validAccessAccountName(input.Username) || input.Password == "" || len(input.Password) > 256 || !utf8.ValidString(input.Password) || strings.ContainsAny(input.Password, "\x00\r\n") {
		return fmt.Errorf("%w: invalid access credentials", ErrInvalidInput)
	}
	switch input.Scope {
	case "server":
		if input.Path != "" {
			return fmt.Errorf("%w: invalid access scope", ErrInvalidInput)
		}
	case "location":
		if !strings.HasPrefix(input.Path, "/") || len(input.Path) > 1024 || !utf8.ValidString(input.Path) || strings.ContainsAny(input.Path, "\x00\r\n") {
			return fmt.Errorf("%w: invalid access path", ErrInvalidInput)
		}
	default:
		return fmt.Errorf("%w: invalid access scope", ErrInvalidInput)
	}
	return nil
}

func accessPasswordStagePath(stateDir, id string) (string, error) {
	if !recipeJobIDPattern.MatchString(id) {
		return "", errors.New("invalid website job identity")
	}
	stateDir = filepath.Clean(stateDir)
	if !filepath.IsAbs(stateDir) || stateDir == string(filepath.Separator) {
		return "", errors.New("invalid website job directory")
	}
	return filepath.Join(stateDir, id+".access-password.json"), nil
}

func stageAccessPassword(stateDir, id, domain string, input *AccessPasswordCreateInput) error {
	if input == nil {
		return nil
	}
	path, err := accessPasswordStagePath(stateDir, id)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		Operation  string `json:"operation"`
		ConfigName string `json:"configName"`
		Scope      string `json:"scope"`
		Path       string `json:"path"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}{"create-stage", domain + ".conf", input.Scope, input.Path, input.Username, input.Password})
	if err != nil {
		return err
	}
	return writePrivateCertificateFile(path, string(payload))
}

func withAccessPasswordInvocation(invocation scriptSiteInvocation, stateDir, id string, required bool) (scriptSiteInvocation, error) {
	path, err := accessPasswordStagePath(stateDir, id)
	if err != nil {
		return scriptSiteInvocation{}, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && !required {
		return invocation, nil
	}
	if err != nil || !required || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() <= 0 || info.Size() > 16384 {
		return scriptSiteInvocation{}, errors.New("access password staging unavailable")
	}
	invocation.environment = append(append([]string(nil), invocation.environment...), "KJ_WEB_ACCESS_PASSWORD_FILE="+path)
	invocation.required = append(append([]string(nil), invocation.required...), `KPANEL_WEB_ACCESS_PASSWORD_PROTOCOL_VERSION="1"`, "kpanel_web_access_password()")
	return invocation, nil
}

func cleanupAccessPassword(stateDir, id string) {
	path, err := accessPasswordStagePath(stateDir, id)
	if err == nil {
		_ = os.Remove(path)
	}
}

func cleanupSiteCreationSecrets(stateDir, id string) {
	cleanupCustomCertificateFiles(stateDir, id)
	cleanupAccessPassword(stateDir, id)
}

func cleanupOrphanAccessPasswordFiles(registry *recipeJobRegistry) {
	if registry == nil {
		return
	}
	registry.mu.Lock()
	active := make(map[string]bool, len(registry.jobs))
	for id, job := range registry.jobs {
		active[id] = job.Status == "queued" || job.Status == "running"
	}
	registry.mu.Unlock()
	entries, err := os.ReadDir(registry.stateDir)
	if err != nil {
		return
	}
	const suffix = ".access-password.json"
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), suffix)
		if !recipeJobIDPattern.MatchString(id) || active[id] {
			continue
		}
		path := filepath.Join(registry.stateDir, entry.Name())
		info, err := os.Lstat(path)
		if err == nil && info.Mode().IsRegular() {
			_ = os.Remove(path)
		}
	}
}
