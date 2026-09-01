package sites

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/hostpty"
)

const (
	maxRecipeJobBytes        = 128 << 10
	maxRecipeTerminalInput   = 16 << 10
	maxRecipeTerminalChunk   = 64 << 10
	maxRecipeTerminalLogSize = 8 << 20
	recipeJobLaunchGrace     = 5 * time.Second
	recipeJobUnitPrefix      = "kpanel-site-"
)

var (
	recipeJobIDPattern  = regexp.MustCompile(`^[a-f0-9]{32}$`)
	recipeProgressLine  = regexp.MustCompile(`^KPANEL_PROGRESS ([0-9]{1,3}) (.+)$`)
	recipeScriptLicense = regexp.MustCompile(`(?m)^permission_granted="true"\r?$`)
	recipeSelectors     = map[string]string{
		"discuz":    "3",
		"kodbox":    "4",
		"maccms":    "5",
		"dujiaoka":  "6",
		"flarum":    "7",
		"typecho":   "8",
		"linkstack": "9",
		"ai-prompt": "27",
		"bitwarden": "25",
		"halo":      "26",
	}
	recipeCommands = map[string]string{
		"discuz":    "discuz",
		"kodbox":    "kodbox",
		"maccms":    "maccms",
		"dujiaoka":  "dujiaoka",
		"flarum":    "flarum",
		"typecho":   "typecho",
		"linkstack": "linkstack",
		"ai-prompt": "ai-prompt",
		"bitwarden": "bitwarden-site",
		"halo":      "halo-site",
	}
	scriptTemplateDefinitions = map[string]scriptTemplateDefinition{
		"static": {
			recipe: "static-site", selector: "30", command: "static-site", kind: contract.SiteStatic,
		},
		"php": {
			recipe: "php-site", selector: "20", command: "php-site", kind: contract.SitePHP,
		},
		"proxy_domain": {
			recipe: "domain-proxy", selector: "24", command: "domain-proxy", kind: contract.SiteDomainProxy,
		},
		"load_balance": {
			recipe: "loadbalance-site", selector: "28", command: "loadbalance-site", kind: contract.SiteLoadBalance,
		},
		"redirect": {
			recipe: "redirect-site", selector: "22", command: "redirect-site", kind: contract.SiteRedirect,
		},
	}
)

type scriptTemplateDefinition struct {
	recipe   string
	selector string
	command  string
	kind     contract.SiteKind
}

type RecipeJob struct {
	ID                string                `json:"id"`
	Domain            string                `json:"domain"`
	Recipe            string                `json:"recipe"`
	ProxyHost         string                `json:"proxyHost,omitempty"`
	ProxyPort         string                `json:"proxyPort,omitempty"`
	Status            string                `json:"status"`
	Stage             string                `json:"stage"`
	Progress          int                   `json:"progress"`
	Message           string                `json:"message,omitempty"`
	Events            []RecipeJobEvent      `json:"events,omitempty"`
	Interactive       bool                  `json:"interactive,omitempty"`
	CustomCertificate bool                  `json:"customCertificate,omitempty"`
	InputOpen         bool                  `json:"inputOpen,omitempty"`
	Site              *contract.SiteSummary `json:"site,omitempty"`
	CreatedAt         time.Time             `json:"createdAt"`
	StartedAt         *time.Time            `json:"startedAt,omitempty"`
	FinishedAt        *time.Time            `json:"finishedAt,omitempty"`
}

type RecipeJobEvent struct {
	Stage    string    `json:"stage"`
	Progress int       `json:"progress"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
}

type recipeJobRegistry struct {
	mu       sync.Mutex
	stateDir string
	jobs     map[string]RecipeJob
}

type scriptSiteInvocation struct {
	arguments   []string
	environment []string
	required    []string
	timeout     time.Duration
}

type recipeJobCommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type systemRecipeJobRunner struct{}

func (systemRecipeJobRunner) Run(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, arguments...).CombinedOutput()
}

func newRecipeJobRegistry(stateDir string) *recipeJobRegistry {
	return &recipeJobRegistry{stateDir: stateDir, jobs: make(map[string]RecipeJob)}
}

func (m *Manager) ConfigureRecipeJobState(stateDir, executable string) error {
	return m.configureRecipeJobState(stateDir, executable, systemRecipeJobRunner{})
}

func (m *Manager) configureRecipeJobState(
	stateDir string,
	executable string,
	runner recipeJobCommandRunner,
) error {
	stateDir = filepath.Clean(stateDir)
	executable = filepath.Clean(executable)
	if !filepath.IsAbs(stateDir) || stateDir == string(filepath.Separator) ||
		!filepath.IsAbs(executable) || runner == nil {
		return errors.New("site recipe jobs require dedicated absolute paths")
	}
	if err := ensureRecipeJobDirectory(stateDir); err != nil {
		return err
	}
	registry := newRecipeJobRegistry(stateDir)
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		job, readErr := registry.read(id)
		if readErr != nil {
			continue
		}
		registry.jobs[id] = job
	}
	m.recipeJobs = registry
	m.jobExecutable = executable
	m.jobRunner = runner
	m.recoverInterruptedRecipeJobs()
	cleanupOrphanCustomCertificateFiles(registry)
	return nil
}

func ensureRecipeJobDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o750); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("site recipe job directory is unavailable or unsafe")
	}
	return nil
}

func (m *Manager) RecipeWritable() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: kejilion.sh recipes require Linux", ErrUnavailable)
	}
	if m.recipeJobs == nil || m.recipeJobs.stateDir == "" {
		return fmt.Errorf("%w: recipe background jobs are unavailable", ErrUnavailable)
	}
	if m.jobRunner == nil || m.jobExecutable == "" {
		return fmt.Errorf("%w: recipe background worker is unavailable", ErrUnavailable)
	}
	if _, err := findSystemdRun(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	_, err := findRecipeScript()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func (m *Manager) WordPressWritable(_ context.Context) error {
	return m.wordPressWritable(false)
}

func (m *Manager) wordPressWritable(useCustomCertificate bool) error {
	return m.directSiteWritableWithCustomCertificate(useCustomCertificate,
		"KJ_WEB_NONINTERACTIVE",
		"KJ_WEB_INTERACTIVE",
		"KJ_WEB_RECIPE",
		"KJ_WEB_DOMAIN",
		`ldnmp_wp "${KJ_WEB_DOMAIN:-}"`,
	)
}

func (m *Manager) ProxyWritable() error {
	return m.proxyWritable(false)
}

func (m *Manager) proxyWritable(useCustomCertificate bool) error {
	return m.directSiteWritableWithCustomCertificate(useCustomCertificate,
		"KJ_WEB_NONINTERACTIVE",
		"KJ_WEB_INTERACTIVE",
		"KJ_WEB_RECIPE",
		"KJ_WEB_DOMAIN",
		"KJ_WEB_PROXY_HOST",
		"KJ_WEB_PROXY_PORT",
		`ldnmp_Proxy "${KJ_WEB_DOMAIN:-}" "${KJ_WEB_PROXY_HOST:-}" "${KJ_WEB_PROXY_PORT:-}"`,
	)
}

func (m *Manager) TemplateWritable() error {
	return m.templateWritable(false)
}

func (m *Manager) templateWritable(useCustomCertificate bool) error {
	return m.directSiteWritableWithCustomCertificate(useCustomCertificate,
		"KJ_WEB_NONINTERACTIVE",
		"KJ_WEB_INTERACTIVE",
		"KJ_WEB_RECIPE",
		"KJ_WEB_DOMAIN",
		"kpanel_run_web_recipe_cli()",
		"php-site)",
		"redirect-site)",
		"domain-proxy)",
		"loadbalance-site)",
		"static-site)",
	)
}

func (m *Manager) CustomCertificateWritable() error {
	return m.directSiteWritableWithCustomCertificate(
		true,
		"KJ_WEB_NONINTERACTIVE",
		"KJ_WEB_INTERACTIVE",
		"KJ_WEB_DOMAIN",
	)
}

func (m *Manager) directSiteWritableWithCustomCertificate(useCustomCertificate bool, required ...string) error {
	if useCustomCertificate {
		required = append(required, customCertificateProtocolRequirements...)
	}
	return m.directSiteWritable(required...)
}

func (m *Manager) directSiteWritable(required ...string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: kejilion.sh website commands require Linux", ErrUnavailable)
	}
	if m.recipeJobs == nil || m.recipeJobs.stateDir == "" {
		return fmt.Errorf("%w: website background jobs are unavailable", ErrUnavailable)
	}
	if m.jobRunner == nil || m.jobExecutable == "" {
		return fmt.Errorf("%w: website background worker is unavailable", ErrUnavailable)
	}
	if _, err := findSystemdRun(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if _, err := findTrustedKejilionScript(required...); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func (m *Manager) StartRecipe(_ context.Context, input ScriptSiteInput) (RecipeJob, error) {
	domain, _, err := normalizeRecipeInput(input.SiteInput)
	if err != nil {
		return RecipeJob{}, err
	}
	customCertificate, err := normalizeCustomCertificateInput(input.Certificate, input.PrivateKey, domain)
	if err != nil {
		return RecipeJob{}, err
	}
	if customCertificate.present() {
		if err := m.CustomCertificateWritable(); err != nil {
			return RecipeJob{}, err
		}
	}
	if err := m.RecipeWritable(); err != nil {
		return RecipeJob{}, err
	}
	siteWriteMutex.Lock()
	defer siteWriteMutex.Unlock()
	if err := m.checkCollisions(managedSpec{Primary: domain, Kind: contract.SitePHP}, ""); err != nil {
		return RecipeJob{}, err
	}
	if m.recipeJobs.hasActive() {
		return RecipeJob{}, fmt.Errorf("%w: another one-click website task is running", ErrConflict)
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return RecipeJob{}, err
	}
	job := RecipeJob{
		ID: hex.EncodeToString(identity[:]), Domain: domain, Recipe: input.Recipe,
		Status: "queued", Stage: "queued", Progress: 0,
		Message: "一键建站任务已进入后台队列", CreatedAt: time.Now().UTC(),
		Interactive:       true,
		CustomCertificate: customCertificate.present(),
	}
	appendRecipeEvent(&job, job.Stage, job.Progress, job.Message)
	if err := stageCustomCertificateFiles(m.recipeJobs.stateDir, job.ID, customCertificate); err != nil {
		return RecipeJob{}, fmt.Errorf("%w: stage custom certificate: %v", ErrUnavailable, err)
	}
	if err := hostpty.CreateInput(m.recipeJobs.inputPath(job.ID)); err != nil {
		cleanupCustomCertificateFiles(m.recipeJobs.stateDir, job.ID)
		return RecipeJob{}, fmt.Errorf("%w: prepare interactive website terminal: %v", ErrUnavailable, err)
	}
	if err := m.recipeJobs.put(job); err != nil {
		_ = hostpty.RemoveInput(m.recipeJobs.inputPath(job.ID))
		cleanupCustomCertificateFiles(m.recipeJobs.stateDir, job.ID)
		return RecipeJob{}, fmt.Errorf("%w: persist recipe job: %v", ErrNeedsAttention, err)
	}
	if err := m.launchRecipeJob(context.Background(), job); err != nil {
		m.failRecipeJob(job, "start_failed", err)
		_ = hostpty.RemoveInput(m.recipeJobs.inputPath(job.ID))
		cleanupCustomCertificateFiles(m.recipeJobs.stateDir, job.ID)
		return RecipeJob{}, fmt.Errorf("%w: launch website job: %v", ErrUnavailable, err)
	}
	return job, nil
}

func (m *Manager) StartWordPress(ctx context.Context, input ScriptSiteInput) (RecipeJob, error) {
	spec, err := normalizeWordPressInput(input.SiteInput)
	if err != nil {
		return RecipeJob{}, err
	}
	customCertificate, err := normalizeCustomCertificateInput(input.Certificate, input.PrivateKey, spec.Primary)
	if err != nil {
		return RecipeJob{}, err
	}
	if err := m.wordPressWritable(customCertificate.present()); err != nil {
		return RecipeJob{}, err
	}
	return m.startDirectSiteJob(
		spec,
		"wordpress",
		wordPressInvocation(spec.Primary),
		customCertificate,
	)
}

func (m *Manager) StartProxy(input ScriptSiteInput) (RecipeJob, error) {
	spec, host, port, err := normalizeDirectProxyInput(input.SiteInput)
	if err != nil {
		return RecipeJob{}, err
	}
	customCertificate, err := normalizeCustomCertificateInput(input.Certificate, input.PrivateKey, spec.Primary)
	if err != nil {
		return RecipeJob{}, err
	}
	if err := m.proxyWritable(customCertificate.present()); err != nil {
		return RecipeJob{}, err
	}
	return m.startDirectSiteJob(
		spec,
		"reverse-proxy",
		proxyInvocation(spec.Primary, host, port),
		customCertificate,
	)
}

func (m *Manager) StartTemplate(input ScriptSiteInput) (RecipeJob, error) {
	domain, definition, err := normalizeTemplateInput(input.SiteInput)
	if err != nil {
		return RecipeJob{}, err
	}
	customCertificate, err := normalizeCustomCertificateInput(input.Certificate, input.PrivateKey, domain)
	if err != nil {
		return RecipeJob{}, err
	}
	if err := m.templateWritable(customCertificate.present()); err != nil {
		return RecipeJob{}, err
	}
	return m.startDirectSiteJob(
		managedSpec{Primary: domain, Kind: definition.kind},
		definition.recipe,
		templateInvocation(domain, definition),
		customCertificate,
	)
}

func wordPressInvocation(domain string) scriptSiteInvocation {
	return scriptSiteInvocation{
		arguments:   []string{"wp", domain},
		environment: []string{"KJ_WEB_NONINTERACTIVE=1", "KJ_WEB_INTERACTIVE=1", "KJ_WEB_RECIPE=2", "KJ_WEB_DOMAIN=" + domain},
		required: []string{
			"KJ_WEB_NONINTERACTIVE",
			"KJ_WEB_INTERACTIVE",
			"KJ_WEB_RECIPE",
			"KJ_WEB_DOMAIN",
			"wp|wordpress)",
			`ldnmp_wp "$@"`,
		},
		timeout: 60 * time.Minute,
	}
}

func proxyInvocation(domain, host, port string) scriptSiteInvocation {
	return scriptSiteInvocation{
		arguments: []string{"fd", domain, host, port},
		environment: []string{
			"KJ_WEB_NONINTERACTIVE=1",
			"KJ_WEB_INTERACTIVE=1",
			"KJ_WEB_RECIPE=23",
			"KJ_WEB_DOMAIN=" + domain,
			"KJ_WEB_PROXY_HOST=" + host,
			"KJ_WEB_PROXY_PORT=" + port,
		},
		required: []string{
			"KJ_WEB_NONINTERACTIVE",
			"KJ_WEB_INTERACTIVE",
			"KJ_WEB_RECIPE",
			"KJ_WEB_DOMAIN",
			"KJ_WEB_PROXY_HOST",
			"KJ_WEB_PROXY_PORT",
			"fd|rp|反代)",
			`ldnmp_Proxy "$@"`,
		},
		timeout: 45 * time.Minute,
	}
}

func templateInvocation(domain string, definition scriptTemplateDefinition) scriptSiteInvocation {
	return scriptSiteInvocation{
		arguments: []string{definition.command, domain},
		environment: []string{
			"KJ_WEB_NONINTERACTIVE=1",
			"KJ_WEB_INTERACTIVE=1",
			"KJ_WEB_RECIPE=" + definition.selector,
			"KJ_WEB_DOMAIN=" + domain,
		},
		required: []string{
			"KJ_WEB_NONINTERACTIVE",
			"KJ_WEB_INTERACTIVE",
			"KJ_WEB_RECIPE",
			"KJ_WEB_DOMAIN",
			"kpanel_run_web_recipe_cli()",
			definition.command + ")",
		},
		timeout: 60 * time.Minute,
	}
}

func (m *Manager) startDirectSiteJob(
	spec managedSpec,
	recipe string,
	invocation scriptSiteInvocation,
	customCertificate normalizedCustomCertificate,
) (RecipeJob, error) {
	siteWriteMutex.Lock()
	defer siteWriteMutex.Unlock()
	if err := m.checkCollisions(spec, ""); err != nil {
		return RecipeJob{}, err
	}
	if m.recipeJobs.hasActive() {
		return RecipeJob{}, fmt.Errorf("%w: another kejilion.sh website task is running", ErrConflict)
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return RecipeJob{}, err
	}
	job := RecipeJob{
		ID: hex.EncodeToString(identity[:]), Domain: spec.Primary, Recipe: recipe,
		Status: "queued", Stage: "queued", Progress: 0,
		Message: "kejilion.sh 原生建站任务已进入后台队列", CreatedAt: time.Now().UTC(),
		Interactive:       true,
		CustomCertificate: customCertificate.present(),
	}
	if recipe == "reverse-proxy" {
		job.ProxyHost = invocation.arguments[2]
		job.ProxyPort = invocation.arguments[3]
	}
	appendRecipeEvent(&job, job.Stage, job.Progress, job.Message)
	if err := stageCustomCertificateFiles(m.recipeJobs.stateDir, job.ID, customCertificate); err != nil {
		return RecipeJob{}, fmt.Errorf("%w: stage custom certificate: %v", ErrUnavailable, err)
	}
	if err := hostpty.CreateInput(m.recipeJobs.inputPath(job.ID)); err != nil {
		cleanupCustomCertificateFiles(m.recipeJobs.stateDir, job.ID)
		return RecipeJob{}, fmt.Errorf("%w: prepare interactive website terminal: %v", ErrUnavailable, err)
	}
	if err := m.recipeJobs.put(job); err != nil {
		_ = hostpty.RemoveInput(m.recipeJobs.inputPath(job.ID))
		cleanupCustomCertificateFiles(m.recipeJobs.stateDir, job.ID)
		return RecipeJob{}, fmt.Errorf("%w: persist website job: %v", ErrNeedsAttention, err)
	}
	if err := m.launchRecipeJob(context.Background(), job); err != nil {
		m.failRecipeJob(job, "start_failed", err)
		_ = hostpty.RemoveInput(m.recipeJobs.inputPath(job.ID))
		cleanupCustomCertificateFiles(m.recipeJobs.stateDir, job.ID)
		return RecipeJob{}, fmt.Errorf("%w: launch website job: %v", ErrUnavailable, err)
	}
	return job, nil
}

func normalizeRecipeInput(input SiteInput) (string, string, error) {
	if input.Type != "recipe" || input.Recipe == "" {
		return "", "", fmt.Errorf("%w: a one-click recipe is required", ErrInvalidInput)
	}
	selector, ok := recipeSelectors[input.Recipe]
	if !ok {
		return "", "", fmt.Errorf("%w: unsupported one-click recipe", ErrUnprocessable)
	}
	domain, err := normalizeFQDN(input.PrimaryDomain)
	if err != nil {
		return "", "", err
	}
	if len(input.Aliases) > 0 || input.Upstream != "" || len(input.Upstreams) > 0 ||
		input.RedirectTarget != "" || input.RedirectCode != 0 || input.PHPVersion != "" ||
		input.ExpectedResourceVersion != "" || (input.Enabled != nil && !*input.Enabled) {
		return "", "", fmt.Errorf("%w: recipe does not accept generic site settings", ErrUnprocessable)
	}
	return domain, selector, nil
}

func normalizeTemplateInput(input SiteInput) (string, scriptTemplateDefinition, error) {
	definition, ok := scriptTemplateDefinitions[input.Type]
	if !ok {
		return "", scriptTemplateDefinition{}, fmt.Errorf("%w: unsupported scripted website template", ErrUnprocessable)
	}
	domain, err := normalizeFQDN(input.PrimaryDomain)
	if err != nil {
		return "", scriptTemplateDefinition{}, err
	}
	if input.Enabled != nil && !*input.Enabled {
		return "", scriptTemplateDefinition{}, fmt.Errorf("%w: disabling sites is not supported", ErrUnprocessable)
	}
	if len(input.Aliases) > 0 || input.Recipe != "" || input.Upstream != "" ||
		len(input.Upstreams) > 0 || input.RedirectTarget != "" ||
		input.RedirectCode != 0 || input.PHPVersion != "" ||
		input.ExpectedResourceVersion != "" {
		return "", scriptTemplateDefinition{}, fmt.Errorf(
			"%w: scripted website details must be entered in the interactive terminal",
			ErrInvalidInput,
		)
	}
	return domain, definition, nil
}

func scriptTemplateByRecipe(recipe string) (scriptTemplateDefinition, bool) {
	for _, definition := range scriptTemplateDefinitions {
		if definition.recipe == recipe {
			return definition, true
		}
	}
	return scriptTemplateDefinition{}, false
}

func normalizeWordPressInput(input SiteInput) (managedSpec, error) {
	primary, err := normalizeFQDN(input.PrimaryDomain)
	if err != nil {
		return managedSpec{}, err
	}
	if input.Type != "wordpress" {
		return managedSpec{}, fmt.Errorf("%w: WordPress type is required", ErrInvalidInput)
	}
	if input.Enabled != nil && !*input.Enabled {
		return managedSpec{}, fmt.Errorf("%w: disabling WordPress is not supported", ErrUnprocessable)
	}
	if input.ExpectedResourceVersion != "" {
		return managedSpec{}, fmt.Errorf("%w: expectedResourceVersion is not valid for create", ErrInvalidInput)
	}
	if len(input.Aliases) != 0 || strings.TrimSpace(input.Upstream) != "" ||
		len(input.Upstreams) != 0 || strings.TrimSpace(input.RedirectTarget) != "" ||
		input.RedirectCode != 0 || strings.TrimSpace(input.PHPVersion) != "" ||
		input.Recipe != "" {
		return managedSpec{}, fmt.Errorf(
			"%w: kejilion.sh WordPress accepts only one primary domain",
			ErrUnprocessable,
		)
	}
	return managedSpec{Primary: primary, Kind: contract.SiteWordPress}, nil
}

func normalizeDirectProxyInput(input SiteInput) (managedSpec, string, string, error) {
	spec, err := normalizeSiteInput(input)
	if err != nil {
		return managedSpec{}, "", "", err
	}
	if input.Type != "proxy" || spec.Kind != contract.SiteReverseProxy {
		return managedSpec{}, "", "", fmt.Errorf("%w: IP and port proxy type is required", ErrInvalidInput)
	}
	if input.ExpectedResourceVersion != "" {
		return managedSpec{}, "", "", fmt.Errorf(
			"%w: expectedResourceVersion is not valid for create",
			ErrInvalidInput,
		)
	}
	if len(spec.Aliases) != 0 {
		return managedSpec{}, "", "", fmt.Errorf(
			"%w: kejilion.sh IP and port proxy accepts one primary domain",
			ErrUnprocessable,
		)
	}
	parsed, err := url.Parse(spec.Upstream)
	if err != nil || parsed.Scheme != "http" {
		return managedSpec{}, "", "", fmt.Errorf(
			"%w: kejilion.sh IP and port proxy requires an http upstream",
			ErrUnprocessable,
		)
	}
	host, port := parsed.Hostname(), parsed.Port()
	if host == "" || port == "" || strings.Contains(host, ":") {
		return managedSpec{}, "", "", fmt.Errorf(
			"%w: kejilion.sh IP and port proxy requires an IPv4 or hostname with an explicit port",
			ErrUnprocessable,
		)
	}
	return spec, host, port, nil
}

func (m *Manager) launchRecipeJob(ctx context.Context, job RecipeJob) error {
	if m.jobRunner == nil || m.jobExecutable == "" {
		return errors.New("website background worker is unavailable")
	}
	systemdRun, err := findSystemdRun()
	if err != nil {
		return err
	}
	arguments := siteWorkerSystemdArguments(
		job,
		m.jobExecutable,
		m.recipeJobs.stateDir,
		m.webRoot,
	)
	output, err := m.jobRunner.Run(ctx, systemdRun, arguments...)
	if err == nil {
		return nil
	}
	detail := strings.TrimSpace(string(output))
	if len(detail) > 300 {
		detail = detail[:300]
	}
	if detail != "" {
		return fmt.Errorf("%s: %w", detail, err)
	}
	return err
}

func siteWorkerSystemdArguments(
	job RecipeJob,
	executable string,
	stateDir string,
	webRoot string,
) []string {
	return []string{
		"--unit=" + recipeJobUnitPrefix + job.ID,
		"--collect",
		"--no-block",
		"--property=Type=oneshot",
		"--property=TimeoutStartSec=60min",
		"--property=TimeoutStopSec=10min",
		"--property=User=root",
		"--property=UMask=0027",
		"--property=PrivateTmp=no",
		"--property=NoNewPrivileges=no",
		"--property=ProtectSystem=no",
		"--property=ProtectHome=no",
		"--property=PrivateDevices=no",
		"--property=RestrictNamespaces=no",
		"--property=RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK",
		"--property=SyslogIdentifier=kpanel-site",
		"--",
		executable,
		"site-pty-run",
		"--state-dir",
		stateDir,
		"--web-root",
		webRoot,
		"--id",
		job.ID,
	}
}

func (m *Manager) recipeUnitState(id string) (running bool, known bool) {
	if m.jobRunner == nil {
		return false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := m.jobRunner.Run(
		ctx,
		"systemctl",
		"is-active",
		recipeJobUnitPrefix+id+".service",
	)
	state := strings.TrimSpace(string(output))
	switch state {
	case "active", "activating", "reloading":
		return true, true
	case "inactive", "failed", "dead", "deactivating":
		return false, true
	default:
		return false, err == nil && state != ""
	}
}

func (m *Manager) recoverInterruptedRecipeJobs() {
	if m.recipeJobs == nil {
		return
	}
	for _, job := range m.recipeJobs.list() {
		if job.Status != "queued" && job.Status != "running" {
			continue
		}
		running, known := m.recipeUnitState(job.ID)
		if known && !running {
			m.markRecipeJobInterrupted(job)
		}
	}
}

func (m *Manager) reconcileInactiveRecipeJobs() {
	if m.recipeJobs == nil {
		return
	}
	for _, job := range m.recipeJobs.list() {
		if (job.Status != "queued" && job.Status != "running") ||
			time.Since(job.CreatedAt) < recipeJobLaunchGrace {
			continue
		}
		running, known := m.recipeUnitState(job.ID)
		if !known || running {
			continue
		}
		latest, err := m.recipeJobs.read(job.ID)
		if err != nil || (latest.Status != "queued" && latest.Status != "running") {
			continue
		}
		m.markRecipeJobInterrupted(latest)
	}
}

func (m *Manager) markRecipeJobInterrupted(job RecipeJob) {
	finished := time.Now().UTC()
	job.Status = "failed"
	job.Stage = "interrupted"
	job.Progress = 100
	job.Message = "后台建站单元已结束但未回写结果，请核对实际站点产物后重试"
	job.InputOpen = false
	job.FinishedAt = &finished
	appendRecipeEvent(&job, job.Stage, job.Progress, job.Message)
	_ = m.recipeJobs.put(job)
	_ = hostpty.RemoveInput(m.recipeJobs.inputPath(job.ID))
	cleanupCustomCertificateFiles(m.recipeJobs.stateDir, job.ID)
}

func (m *Manager) RecipeJob(id string) (RecipeJob, error) {
	if m.recipeJobs == nil {
		return RecipeJob{}, ErrConflict
	}
	m.reconcileInactiveRecipeJobs()
	return m.recipeJobs.read(id)
}

func (m *Manager) InstallationJob(id string) (any, error) {
	if job, err := m.RecipeJob(id); err == nil {
		return job, nil
	}
	return nil, ErrConflict
}

func (m *Manager) InstallationJobs() []RecipeJob {
	if m.recipeJobs == nil {
		return []RecipeJob{}
	}
	m.reconcileInactiveRecipeJobs()
	return m.recipeJobs.list()
}

type SiteTerminalChunk struct {
	DataBase64 string `json:"dataBase64"`
	NextOffset int64  `json:"nextOffset"`
	InputOpen  bool   `json:"inputOpen"`
	Finished   bool   `json:"finished"`
}

func (m *Manager) InstallationTerminal(id string, offset int64) (SiteTerminalChunk, error) {
	if m.recipeJobs == nil || !recipeJobIDPattern.MatchString(id) || offset < 0 {
		return SiteTerminalChunk{}, ErrConflict
	}
	job, err := m.recipeJobs.read(id)
	if err != nil || !job.Interactive {
		return SiteTerminalChunk{}, ErrConflict
	}
	path := m.recipeJobs.logPath(id)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return SiteTerminalChunk{
			InputOpen: job.InputOpen,
			Finished:  job.Status == "succeeded" || job.Status == "failed",
		}, nil
	}
	if err != nil {
		return SiteTerminalChunk{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return SiteTerminalChunk{}, err
	}
	if offset > info.Size() {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return SiteTerminalChunk{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxRecipeTerminalChunk))
	if err != nil {
		return SiteTerminalChunk{}, err
	}
	nextOffset := offset + int64(len(data))
	return SiteTerminalChunk{
		DataBase64: base64.StdEncoding.EncodeToString(data),
		NextOffset: nextOffset,
		InputOpen:  job.InputOpen,
		Finished: (job.Status == "succeeded" || job.Status == "failed") &&
			nextOffset >= info.Size(),
	}, nil
}

func (m *Manager) WriteInstallationInput(id, value string) error {
	if m.recipeJobs == nil || !recipeJobIDPattern.MatchString(id) {
		return ErrConflict
	}
	data := []byte(value)
	if len(data) == 0 || len(data) > maxRecipeTerminalInput || bytes.IndexByte(data, 0) >= 0 {
		return fmt.Errorf("%w: interactive website input is invalid", ErrInvalidInput)
	}
	job, err := m.recipeJobs.read(id)
	if err != nil {
		return ErrConflict
	}
	if !job.Interactive || !job.InputOpen ||
		(job.Status != "queued" && job.Status != "running") {
		return fmt.Errorf("%w: interactive website input is not open", ErrConflict)
	}
	if err := hostpty.WriteInput(m.recipeJobs.inputPath(id), data); err != nil {
		return fmt.Errorf("%w: interactive website input is unavailable: %v", ErrConflict, err)
	}
	return nil
}

func RunRecipeJob(ctx context.Context, stateDir, webRoot, id string) error {
	if os.Geteuid() != 0 {
		return errors.New("site-pty-run requires root")
	}
	if !recipeJobIDPattern.MatchString(id) {
		return errors.New("invalid website job identity")
	}
	stateDir = filepath.Clean(stateDir)
	webRoot = filepath.Clean(webRoot)
	if !filepath.IsAbs(stateDir) || stateDir == string(filepath.Separator) ||
		!filepath.IsAbs(webRoot) || webRoot == string(filepath.Separator) {
		return errors.New("website job requires dedicated absolute paths")
	}
	registry := newRecipeJobRegistry(stateDir)
	if err := ensureRecipeJobDirectory(stateDir); err != nil {
		return err
	}
	job, err := registry.read(id)
	if err != nil {
		return err
	}
	invocation, err := invocationForRecipeJob(job)
	if err != nil {
		finished := time.Now().UTC()
		job.Status = "failed"
		job.Stage = "invalid_job"
		job.Message = "后台建站任务参数无效"
		job.FinishedAt = &finished
		_ = registry.put(job)
		cleanupCustomCertificateFiles(stateDir, id)
		return err
	}
	invocation, err = withCustomCertificateInvocation(invocation, stateDir, id, job.CustomCertificate)
	if err != nil {
		finished := time.Now().UTC()
		job.Status = "failed"
		job.Stage = "invalid_job"
		job.Message = "后台建站任务证书材料无效"
		job.FinishedAt = &finished
		_ = registry.put(job)
		cleanupCustomCertificateFiles(stateDir, id)
		return err
	}
	manager := NewManager(webRoot, NewDiscoverer(webRoot), nil)
	manager.recipeJobs = registry
	manager.runRecipeJob(ctx, id, invocation)
	completed, err := registry.read(id)
	if err != nil {
		return err
	}
	if completed.Status == "failed" {
		return errors.New(completed.Message)
	}
	return nil
}

func invocationForRecipeJob(job RecipeJob) (scriptSiteInvocation, error) {
	switch job.Recipe {
	case "wordpress":
		if _, err := normalizeWordPressInput(SiteInput{
			PrimaryDomain: job.Domain,
			Type:          "wordpress",
		}); err != nil {
			return scriptSiteInvocation{}, err
		}
		return wordPressInvocation(job.Domain), nil
	case "reverse-proxy":
		_, host, port, err := normalizeDirectProxyInput(SiteInput{
			PrimaryDomain: job.Domain,
			Type:          "proxy",
			Upstream:      "http://" + job.ProxyHost + ":" + job.ProxyPort,
		})
		if err != nil {
			return scriptSiteInvocation{}, err
		}
		return proxyInvocation(job.Domain, host, port), nil
	default:
		if definition, ok := scriptTemplateByRecipe(job.Recipe); ok {
			domain, err := normalizeFQDN(job.Domain)
			if err != nil {
				return scriptSiteInvocation{}, err
			}
			return templateInvocation(domain, definition), nil
		}
		domain, selector, err := normalizeRecipeInput(SiteInput{
			PrimaryDomain: job.Domain,
			Type:          "recipe",
			Recipe:        job.Recipe,
		})
		if err != nil {
			return scriptSiteInvocation{}, err
		}
		return scriptSiteInvocation{
			arguments: []string{recipeCommands[job.Recipe], domain},
			environment: []string{
				"KJ_WEB_NONINTERACTIVE=1",
				"KJ_WEB_INTERACTIVE=1",
				"KJ_WEB_RECIPE=" + selector,
				"KJ_WEB_DOMAIN=" + domain,
			},
			required: []string{
				"KJ_WEB_NONINTERACTIVE",
				"KJ_WEB_INTERACTIVE",
				"KJ_WEB_RECIPE",
				"KJ_WEB_DOMAIN",
				"kpanel_run_web_recipe_cli()",
				recipeCommands[job.Recipe] + ")",
			},
			timeout: 60 * time.Minute,
		}, nil
	}
}

func (m *Manager) runRecipeJob(ctx context.Context, id string, invocation scriptSiteInvocation) {
	job, err := m.recipeJobs.read(id)
	if err != nil {
		return
	}
	defer func() {
		_ = hostpty.RemoveInput(m.recipeJobs.inputPath(id))
		cleanupCustomCertificateFiles(m.recipeJobs.stateDir, id)
	}()
	script, err := findTrustedKejilionScript(invocation.required...)
	if err != nil {
		m.failRecipeJob(job, "script_unavailable", err)
		return
	}
	started := time.Now().UTC()
	job.Status = "running"
	job.Stage = "preflight"
	job.Progress = 1
	job.Message = "正在启动 kejilion.sh 原生一键建站流程"
	job.StartedAt = &started
	appendRecipeEvent(&job, job.Stage, job.Progress, job.Message)
	if m.recipeJobs.put(job) != nil {
		return
	}

	commandArguments := append([]string{script}, invocation.arguments...)
	command := exec.CommandContext(ctx, "/bin/bash", commandArguments...)
	command.Env = siteCommandEnvironment(
		append(invocation.environment, "LC_ALL=C.UTF-8", "LANG=C.UTF-8", "TERM=xterm-256color"),
	)
	input, err := hostpty.OpenInput(m.recipeJobs.inputPath(id))
	if err != nil {
		m.failRecipeJob(job, "terminal_unavailable", err)
		return
	}
	defer input.Close()
	logFile, err := os.OpenFile(
		m.recipeJobs.logPath(id),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0o600,
	)
	if err != nil {
		m.failRecipeJob(job, "terminal_unavailable", err)
		return
	}
	terminal, err := hostpty.Start(command, 36, 120)
	if err != nil {
		_ = logFile.Close()
		m.failRecipeJob(job, "terminal_unavailable", err)
		return
	}
	defer terminal.Close()
	job.InputOpen = true
	if m.recipeJobs.put(job) != nil {
		_ = terminal.Kill()
		_ = terminal.Wait()
		_ = logFile.Close()
		return
	}

	inputDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(terminal, input)
		close(inputDone)
	}()
	readErr := m.copyRecipeTerminalOutput(&job, logFile, terminal)
	if readErr != nil && !hostpty.IsEnd(readErr) {
		_ = terminal.Kill()
	}
	_ = logFile.Sync()
	_ = logFile.Close()
	waitErr := terminal.Wait()
	_ = input.Close()
	select {
	case <-inputDone:
	case <-time.After(250 * time.Millisecond):
	}
	job.InputOpen = false
	if readErr != nil && !hostpty.IsEnd(readErr) && waitErr == nil {
		waitErr = readErr
	}
	if waitErr != nil {
		m.failRecipeJob(job, "failed", waitErr)
		return
	}
	items, err := m.discoverer.Discover()
	if err != nil {
		m.failRecipeJob(job, "reconcile_failed", err)
		return
	}
	for index := range items {
		if items[index].PrimaryDomain == job.Domain {
			job.Site = &items[index]
			break
		}
	}
	if job.Site == nil {
		m.failRecipeJob(job, "reconcile_failed", errors.New("site artifacts were not discovered"))
		return
	}
	finished := time.Now().UTC()
	job.Status = "succeeded"
	job.Stage = "completed"
	job.Progress = 100
	job.Message = "kejilion.sh 原生" + scriptSiteLabel(job.Recipe) + "产物已完成对账"
	job.InputOpen = false
	job.FinishedAt = &finished
	appendRecipeEvent(&job, job.Stage, job.Progress, job.Message)
	_ = m.recipeJobs.put(job)
}

func (m *Manager) copyRecipeTerminalOutput(
	job *RecipeJob,
	logFile *os.File,
	terminal hostpty.Process,
) error {
	buffer := make([]byte, 4096)
	pending := ""
	written := int64(0)
	outputLines := 0
	for {
		count, err := terminal.Read(buffer)
		if count > 0 {
			chunk := buffer[:count]
			if written < maxRecipeTerminalLogSize {
				remaining := int64(maxRecipeTerminalLogSize) - written
				if int64(len(chunk)) > remaining {
					chunk = chunk[:remaining]
				}
				size, writeErr := logFile.Write(chunk)
				written += int64(size)
				if writeErr != nil {
					return writeErr
				}
			}
			pending += stripSiteTerminalControls(string(buffer[:count]))
			lines := strings.FieldsFunc(pending, func(value rune) bool {
				return value == '\n' || value == '\r'
			})
			if strings.HasSuffix(pending, "\n") || strings.HasSuffix(pending, "\r") {
				pending = ""
			} else if len(lines) > 0 {
				pending = lines[len(lines)-1]
				lines = lines[:len(lines)-1]
			}
			for _, line := range lines {
				line = strings.TrimSpace(line)
				matches := recipeProgressLine.FindStringSubmatch(line)
				if len(matches) == 3 {
					progress, _ := strconv.Atoi(matches[1])
					job.Progress = min(max(progress, 0), 100)
					job.Stage = recipeJobStage(job.Progress)
					job.Message = safeRecipeMessage(matches[2])
					appendRecipeEvent(job, job.Stage, job.Progress, job.Message)
					_ = m.recipeJobs.put(*job)
					continue
				}
				outputLines++
				if outputLines%8 == 0 && job.Progress < 88 {
					job.Progress = min(job.Progress+3, 88)
					job.Stage = "installing"
					job.Message = fmt.Sprintf(
						"kejilion.sh 原生%s流程正在执行（终端已输出 %d 行）",
						scriptSiteLabel(job.Recipe),
						outputLines,
					)
					_ = m.recipeJobs.put(*job)
				}
			}
			if len(pending) > 4096 {
				pending = pending[len(pending)-4096:]
			}
		}
		if err != nil {
			return err
		}
	}
}

func scriptSiteLabel(recipe string) string {
	switch recipe {
	case "wordpress":
		return " WordPress 建站"
	case "reverse-proxy":
		return " IP+端口反向代理"
	case "static-site":
		return "静态站点"
	case "php-site":
		return "PHP 动态站点"
	case "domain-proxy":
		return "域名反向代理"
	case "loadbalance-site":
		return "负载均衡站点"
	case "redirect-site":
		return "站点重定向"
	default:
		return "一键建站"
	}
}

func (m *Manager) failRecipeJob(job RecipeJob, stage string, cause error) {
	finished := time.Now().UTC()
	job.Status = "failed"
	job.Stage = stage
	job.InputOpen = false
	job.Message = recipeFailureMessage(job, stage, cause)
	job.FinishedAt = &finished
	appendRecipeEvent(&job, job.Stage, job.Progress, job.Message)
	_ = m.recipeJobs.put(job)
}

func recipeFailureMessage(job RecipeJob, stage string, cause error) string {
	switch {
	case errors.Is(cause, context.DeadlineExceeded):
		return "一键建站执行超时并已终止，请核对当前服务器网络和实际站点产物"
	case stage == "script_unavailable":
		return "未找到已授权且支持建站协议的 kejilion.sh，请先更新脚本后重试"
	case stage == "runner_unavailable":
		return "无法启动后台建站任务，请检查 systemd-run 和 Host Agent 状态"
	case stage == "start_failed":
		return "kejilion.sh 建站任务启动失败，请检查 Host Agent 的 systemd 权限"
	case stage == "terminal_unavailable":
		return "建站交互终端启动失败，请检查 Host Agent 的 PTY、状态目录和 systemd 权限"
	case stage == "reconcile_failed":
		return "脚本已结束，但 KPanel 未发现完整站点产物，请检查 Nginx 配置、站点目录和证书"
	}

	message := strings.TrimSpace(job.Message)
	if message == "" || message == "正在启动 kejilion.sh 原生一键建站流程" {
		message = "kejilion.sh 未返回可识别的失败阶段"
	}
	var exitErr *exec.ExitError
	if errors.As(cause, &exitErr) {
		return fmt.Sprintf("建站失败：%s（脚本退出码 %d）", message, exitErr.ExitCode())
	}
	return "建站失败：" + message
}

func findRecipeScript() (string, error) {
	return findTrustedKejilionScript(
		"KJ_WEB_NONINTERACTIVE",
		"KJ_WEB_INTERACTIVE",
		"KJ_WEB_RECIPE",
		"KJ_WEB_DOMAIN",
		"kpanel_run_web_recipe_cli()",
	)
}

func findTrustedKejilionScript(required ...string) (string, error) {
	candidates := []string{
		"/home/docker/kpanel/bin/kejilion.sh",
		"/usr/local/bin/k",
		"/usr/bin/k",
		"/root/kejilion.sh",
	}
	if path, err := exec.LookPath("k"); err == nil {
		candidates = append(candidates, path)
	}
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		resolved = filepath.Clean(resolved)
		if seen[resolved] {
			continue
		}
		seen[resolved] = true
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 1024 || info.Size() > 4<<20 ||
			info.Mode().Perm()&0o022 != 0 || !recipeScriptOwnerTrusted(info) {
			continue
		}
		content, err := os.ReadFile(resolved)
		if err != nil {
			continue
		}
		value := string(content)
		if recipeScriptLicense.Match(content) && containsAll(value, required) {
			return resolved, nil
		}
	}
	return "", errors.New("a trusted kejilion.sh website command was not found")
}

func findSystemdRun() (string, error) {
	for _, candidate := range []string{"/usr/bin/systemd-run", "/bin/systemd-run"} {
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0 &&
			recipeScriptOwnerTrusted(info) {
			return candidate, nil
		}
	}
	return "", errors.New("trusted systemd-run is unavailable")
}

func containsAll(value string, required []string) bool {
	for _, token := range required {
		if token == "" || !strings.Contains(value, token) {
			return false
		}
	}
	return true
}

func recipeJobStage(progress int) string {
	switch {
	case progress < 10:
		return "preflight"
	case progress < 90:
		return "installing"
	case progress < 100:
		return "reconciling"
	default:
		return "completed"
	}
}

func safeRecipeMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 300 {
		value = value[:300]
	}
	return value
}

func stripSiteTerminalControls(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	escape := false
	csi := false
	for _, character := range value {
		if escape {
			if character == '[' {
				csi = true
				escape = false
				continue
			}
			escape = false
			continue
		}
		if csi {
			if character >= 0x40 && character <= 0x7e {
				csi = false
			}
			continue
		}
		if character == 0x1b {
			escape = true
			continue
		}
		if character == '\t' || character == '\n' || character == '\r' ||
			character >= 0x20 {
			result.WriteRune(character)
		}
	}
	return result.String()
}

func appendRecipeEvent(job *RecipeJob, stage string, progress int, message string) {
	message = safeRecipeMessage(message)
	if message == "" {
		return
	}
	if count := len(job.Events); count > 0 {
		last := job.Events[count-1]
		if last.Stage == stage && last.Progress == progress && last.Message == message {
			return
		}
	}
	if len(job.Events) >= 48 {
		job.Events = append([]RecipeJobEvent(nil), job.Events[len(job.Events)-47:]...)
	}
	job.Events = append(job.Events, RecipeJobEvent{
		Stage:    stage,
		Progress: min(max(progress, 0), 100),
		Message:  message,
		At:       time.Now().UTC(),
	})
}

func (registry *recipeJobRegistry) hasActive() bool {
	for _, job := range registry.list() {
		if job.Status == "queued" || job.Status == "running" {
			return true
		}
	}
	return false
}

func (registry *recipeJobRegistry) put(job RecipeJob) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if !recipeJobIDPattern.MatchString(job.ID) {
		return errors.New("invalid recipe job identity")
	}
	data, err := json.MarshalIndent(job, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxRecipeJobBytes {
		return errors.New("recipe job state exceeds the safety limit")
	}
	temp, err := os.CreateTemp(registry.stateDir, "."+job.ID+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	target := registry.path(job.ID)
	if runtime.GOOS == "windows" {
		_ = os.Remove(target)
	}
	if err := os.Rename(tempPath, target); err != nil {
		return err
	}
	registry.jobs[job.ID] = job
	registry.pruneLocked()
	return nil
}

func (registry *recipeJobRegistry) read(id string) (RecipeJob, error) {
	if !recipeJobIDPattern.MatchString(id) {
		return RecipeJob{}, ErrConflict
	}
	info, err := os.Lstat(registry.path(id))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() <= 0 || info.Size() > maxRecipeJobBytes {
		return RecipeJob{}, ErrConflict
	}
	data, err := os.ReadFile(registry.path(id))
	if err != nil {
		return RecipeJob{}, err
	}
	var job RecipeJob
	if json.Unmarshal(data, &job) != nil || job.ID != id {
		return RecipeJob{}, ErrConflict
	}
	return job, nil
}

func (registry *recipeJobRegistry) list() []RecipeJob {
	registry.mu.Lock()
	ids := make([]string, 0, len(registry.jobs))
	for id := range registry.jobs {
		ids = append(ids, id)
	}
	registry.mu.Unlock()
	jobs := make([]RecipeJob, 0, len(ids))
	for _, id := range ids {
		if job, err := registry.read(id); err == nil {
			jobs = append(jobs, job)
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	return jobs
}

func (registry *recipeJobRegistry) path(id string) string {
	return filepath.Join(registry.stateDir, id+".json")
}

func (registry *recipeJobRegistry) logPath(id string) string {
	return filepath.Join(registry.stateDir, id+".terminal.log")
}

func (registry *recipeJobRegistry) inputPath(id string) string {
	return filepath.Join(registry.stateDir, id+".terminal.input")
}

func (registry *recipeJobRegistry) pruneLocked() {
	jobs := make([]RecipeJob, 0, len(registry.jobs))
	for _, job := range registry.jobs {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})
	if len(jobs) <= 100 {
		return
	}
	for _, job := range jobs[100:] {
		if job.Status == "queued" || job.Status == "running" {
			continue
		}
		delete(registry.jobs, job.ID)
		_ = os.Remove(registry.path(job.ID))
		_ = os.Remove(registry.logPath(job.ID))
		_ = hostpty.RemoveInput(registry.inputPath(job.ID))
		cleanupCustomCertificateFiles(registry.stateDir, job.ID)
	}
}
