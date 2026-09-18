package webenv

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	"github.com/kejilion/kejilion-panel/internal/hostpty"
	"github.com/kejilion/kejilion-panel/internal/jobcontrol"
)

var (
	ErrUnavailable = errors.New("web environment unavailable")
	ErrInvalid     = errors.New("invalid web environment request")
	ErrConflict    = errors.New("web environment task conflict")
	ErrNotFound    = errors.New("web environment resource not found")
	jobIDPattern   = regexp.MustCompile(`^[a-f0-9]{32}$`)
	backupPattern  = regexp.MustCompile(`^web_[0-9]{14}\.tar\.gz$`)
	versionPattern = regexp.MustCompile(`^(latest|[0-9]+\.[0-9]+)$`)
	sha256Pattern  = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

const (
	maxJobBytes      = 128 << 10
	maxTerminalChunk = 64 << 10
	maxProgressBytes = 256 << 10
	maxTerminalInput = 16 << 10
	maxLogBytes      = 32 << 20
)

type Component struct {
	Name         string `json:"name"`
	Required     bool   `json:"required"`
	Exists       bool   `json:"exists"`
	Running      bool   `json:"running"`
	State        string `json:"state"`
	Image        string `json:"image"`
	Version      string `json:"version"`
	RepoDigest   string `json:"repoDigest"`
	UpdateStatus string `json:"updateStatus"`
	UpdateReason string `json:"updateReason"`
}

type Protection struct {
	Fail2Ban   bool `json:"fail2ban"`
	WAF        bool `json:"waf"`
	Cloudflare bool `json:"cloudflare"`
	DDoS       bool `json:"ddos"`
}

type Optimization struct {
	Mode   string `json:"mode"`
	Gzip   bool   `json:"gzip"`
	Brotli bool   `json:"brotli"`
	Zstd   bool   `json:"zstd"`
}

type Summary struct {
	ProtocolVersion  string       `json:"protocolVersion"`
	State            string       `json:"state"`
	Profile          string       `json:"profile"`
	Health           string       `json:"health"`
	WebRoot          string       `json:"webRoot"`
	DiskBytes        int64        `json:"diskBytes"`
	SiteCount        int          `json:"siteCount"`
	DatabaseCount    int          `json:"databaseCount"`
	CertificateCount int          `json:"certificateCount"`
	ComposeValid     bool         `json:"composeValid"`
	NginxValid       bool         `json:"nginxValid"`
	ResourceVersion  string       `json:"resourceVersion"`
	ScriptVersion    string       `json:"scriptVersion"`
	LatestBackup     string       `json:"latestBackup"`
	PortConflicts    []string     `json:"portConflicts"`
	Components       []Component  `json:"components"`
	Protection       Protection   `json:"protection"`
	Optimization     Optimization `json:"optimization"`
	ObservedAt       time.Time    `json:"observedAt"`
}

type Catalog struct {
	ProtocolVersion     string              `json:"protocolVersion"`
	InstallProfiles     []CatalogChoice     `json:"installProfiles"`
	ProtectionActions   []string            `json:"protectionActions"`
	OptimizationActions []string            `json:"optimizationActions"`
	UpdateComponents    []CatalogUpdateItem `json:"updateComponents"`
}

type CatalogChoice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type CatalogUpdateItem struct {
	ID       string   `json:"id"`
	Versions []string `json:"versions"`
}

type Backup struct {
	ID        string    `json:"id"`
	SizeBytes int64     `json:"sizeBytes"`
	CreatedAt time.Time `json:"createdAt"`
	Verified  bool      `json:"verified"`
	Format    string    `json:"format"`
}

type ActionRequest struct {
	Action                  string `json:"action"`
	Profile                 string `json:"profile,omitempty"`
	Operation               string `json:"operation,omitempty"`
	Component               string `json:"component,omitempty"`
	Version                 string `json:"version,omitempty"`
	BackupID                string `json:"backupId,omitempty"`
	BackupBeforeChange      bool   `json:"backupBeforeChange,omitempty"`
	ExpectedResourceVersion string `json:"expectedResourceVersion,omitempty"`
	CloudflareAccount       string `json:"cloudflareAccount,omitempty"`
	CloudflareToken         string `json:"cloudflareToken,omitempty"`
	CloudflareZoneID        string `json:"cloudflareZoneId,omitempty"`
}

type Job struct {
	ID         string     `json:"id"`
	Action     string     `json:"action"`
	Target     string     `json:"target,omitempty"`
	Status     string     `json:"status"`
	Stage      string     `json:"stage"`
	Progress   int        `json:"progress"`
	Message    string     `json:"message"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type TerminalChunk struct {
	DataBase64 string `json:"dataBase64"`
	NextOffset int64  `json:"nextOffset"`
	InputOpen  bool   `json:"inputOpen"`
	Finished   bool   `json:"finished"`
}

type Service struct {
	mu       sync.Mutex
	stateDir string
	now      func() time.Time
}

type backgroundRunner struct{}

func (runner backgroundRunner) Run(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	if !filepath.IsAbs(name) {
		resolved, err := runner.LookPath(name)
		if err != nil {
			return nil, err
		}
		name = resolved
	}
	return exec.CommandContext(ctx, name, arguments...).CombinedOutput()
}

func (backgroundRunner) LookPath(name string) (string, error) {
	var candidates []string
	switch name {
	case "systemd-run", "systemctl":
		candidates = []string{filepath.Join("/usr/bin", name), filepath.Join("/bin", name)}
	case "start-stop-daemon", "rc-service":
		candidates = []string{filepath.Join("/sbin", name), filepath.Join("/usr/sbin", name)}
	default:
		return "", errors.New("unsupported background control executable")
	}
	for _, candidate := range candidates {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0 && ownerTrusted(info) {
			return resolved, nil
		}
	}
	return "", errors.New("trusted background control executable was not found")
}

func New(stateDir string) (*Service, error) {
	stateDir = filepath.Clean(stateDir)
	if !filepath.IsAbs(stateDir) || stateDir == string(filepath.Separator) {
		return nil, errors.New("web environment state directory must be absolute")
	}
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		return nil, err
	}
	info, err := os.Lstat(stateDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("web environment state directory is unsafe")
	}
	return &Service{stateDir: stateDir, now: time.Now}, nil
}

func (s *Service) Readable() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: Linux is required", ErrUnavailable)
	}
	if _, err := trustedScript(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func (s *Service) Available() error {
	if err := s.Readable(); err != nil {
		return err
	}
	if err := jobcontrol.Available(backgroundRunner{}); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func (s *Service) Summary(ctx context.Context) (Summary, error) {
	var result Summary
	if err := runStructured(ctx, "status", &result); err != nil {
		return result, err
	}
	if result.ProtocolVersion != "1" {
		return result, fmt.Errorf("%w: unsupported script protocol", ErrUnavailable)
	}
	if result.Components == nil {
		result.Components = []Component{}
	}
	if result.PortConflicts == nil {
		result.PortConflicts = []string{}
	}
	return result, nil
}

func (s *Service) Catalog(ctx context.Context) (Catalog, error) {
	var result Catalog
	if err := runStructured(ctx, "catalog", &result); err != nil {
		return result, err
	}
	if result.InstallProfiles == nil {
		result.InstallProfiles = []CatalogChoice{}
	}
	if result.ProtectionActions == nil {
		result.ProtectionActions = []string{}
	}
	if result.OptimizationActions == nil {
		result.OptimizationActions = []string{}
	}
	if result.UpdateComponents == nil {
		result.UpdateComponents = []CatalogUpdateItem{}
	}
	return result, nil
}

const (
	maxStructuredOutputBytes = 1 << 20
	maxStructuredErrorDetail = 2 << 10
)

// cappedOutput keeps at most limit bytes of a status command's output. It
// keeps accepting writes so the script is not killed by a broken pipe; the
// 20 second deadline still bounds how long it may run.
type cappedOutput struct {
	limit    int
	data     []byte
	overflow bool
}

func (c *cappedOutput) Write(chunk []byte) (int, error) {
	room := c.limit - len(c.data)
	if len(chunk) > room {
		c.data = append(c.data, chunk[:max(room, 0)]...)
		c.overflow = true
		return len(chunk), nil
	}
	c.data = append(c.data, chunk...)
	return len(chunk), nil
}

func runStructured(ctx context.Context, command string, target any) error {
	script, err := trustedScript()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, script, "web", "env", command)
	cmd.Env = append(os.Environ(), "KJ_LDNMP_NONINTERACTIVE=1", "KJ_LDNMP_PROTOCOL=1")
	captured := &cappedOutput{limit: maxStructuredOutputBytes}
	cmd.Stdout = captured
	cmd.Stderr = captured
	err = cmd.Run()
	output := captured.data
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > maxStructuredErrorDetail {
			detail = detail[:maxStructuredErrorDetail]
		}
		return fmt.Errorf("%w: %s", ErrUnavailable, detail)
	}
	if captured.overflow {
		return fmt.Errorf("%w: script response exceeds %d bytes", ErrUnavailable, maxStructuredOutputBytes)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "{") {
			if err := json.Unmarshal([]byte(lines[index]), target); err != nil {
				return fmt.Errorf("%w: invalid script response", ErrUnavailable)
			}
			return nil
		}
	}
	return fmt.Errorf("%w: script response is missing", ErrUnavailable)
}

func (s *Service) Start(ctx context.Context, input ActionRequest) (Job, error) {
	if err := s.Available(); err != nil {
		return Job{}, err
	}
	summary, err := s.Summary(ctx)
	if err != nil {
		return Job{}, err
	}
	args, target, err := validateAction(input, summary)
	if err != nil {
		return Job{}, err
	}
	secret, err := actionSecret(input)
	if err != nil {
		return Job{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, job := range s.jobsLocked() {
		job = s.refreshLocked(job)
		if job.Status == "queued" || job.Status == "running" {
			return Job{}, ErrConflict
		}
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return Job{}, err
	}
	id := hex.EncodeToString(identity[:])
	now := s.now().UTC()
	job := Job{ID: id, Action: input.Action, Target: target, Status: "queued", Stage: "queued",
		Progress: 0, Message: "LDNMP 环境任务已进入后台队列", CreatedAt: now}
	if err := s.writeJob(job); err != nil {
		return Job{}, err
	}
	if err := s.writeArguments(id, args); err != nil {
		_ = os.Remove(s.jobPath(id))
		return Job{}, err
	}
	if err := hostpty.CreateInput(s.inputPath(id)); err != nil {
		_ = os.Remove(s.jobPath(id))
		_ = os.Remove(s.argumentsPath(id))
		return Job{}, fmt.Errorf("%w: prepare environment terminal input: %v", ErrUnavailable, err)
	}
	if len(secret) > 0 {
		if err := os.WriteFile(s.secretPath(id), secret, 0o600); err != nil {
			_ = os.Remove(s.jobPath(id))
			_ = os.Remove(s.argumentsPath(id))
			_ = hostpty.RemoveInput(s.inputPath(id))
			return Job{}, err
		}
	}
	executable, err := os.Executable()
	if err != nil {
		_ = os.Remove(s.secretPath(id))
		_ = hostpty.RemoveInput(s.inputPath(id))
		return Job{}, fmt.Errorf("%w: resolve Agent executable", ErrUnavailable)
	}
	if err := jobcontrol.Launch(ctx, backgroundRunner{}, s.backgroundSpec(id, executable)); err != nil {
		_ = os.Remove(s.secretPath(id))
		_ = hostpty.RemoveInput(s.inputPath(id))
		job.Status, job.Stage, job.Progress = "failed", "start_failed", 100
		job.Message = "无法启动 LDNMP 后台任务: " + safeJobError(err)
		finished := s.now().UTC()
		job.FinishedAt = &finished
		_ = s.writeJob(job)
		return Job{}, fmt.Errorf("%w: %s", ErrUnavailable, job.Message)
	}
	job.Status, job.Stage, job.Progress = "running", "running", 1
	job.Message = "LDNMP 环境任务正在后台执行"
	job.StartedAt = &now
	_ = s.writeJob(job)
	return job, nil
}

func validateAction(input ActionRequest, summary Summary) ([]string, string, error) {
	if input.ExpectedResourceVersion == "" {
		return nil, "", ErrInvalid
	}
	if input.ExpectedResourceVersion != summary.ResourceVersion {
		return nil, "", ErrConflict
	}
	switch input.Action {
	case "install":
		if input.Profile != "full" && input.Profile != "nginx" {
			return nil, "", ErrInvalid
		}
		return []string{"install", input.Profile}, input.Profile, nil
	case "protection.configure":
		allowed := map[string]bool{"fail2ban-install": true, "fail2ban-uninstall": true, "unban-all": true,
			"waf-on": true, "waf-off": true, "ddos-on": true, "ddos-off": true,
			"cloudflare-fail2ban": true, "cloudflare-shield": true}
		if !allowed[input.Operation] {
			return nil, "", ErrInvalid
		}
		return []string{"protect", input.Operation}, input.Operation, nil
	case "optimization.apply":
		allowed := map[string]bool{"standard": true, "high": true, "gzip-on": true, "gzip-off": true,
			"brotli-on": true, "brotli-off": true,
			"zstd-on": true, "zstd-off": true}
		if !allowed[input.Operation] {
			return nil, "", ErrInvalid
		}
		return []string{"optimize", input.Operation}, input.Operation, nil
	case "update.component", "update.all":
		component := input.Component
		if input.Action == "update.all" {
			component = "all"
		}
		if !map[string]bool{"nginx": true, "mysql": true, "php": true, "redis": true, "all": true}[component] ||
			(input.Version != "" && !versionPattern.MatchString(input.Version)) {
			return nil, "", ErrInvalid
		}
		return []string{"update", component, defaultValue(input.Version, "latest"),
			strconv.FormatBool(input.BackupBeforeChange)}, component, nil
	case "backup.create":
		return []string{"backup"}, "web", nil
	case "backup.delete":
		if !backupPattern.MatchString(input.BackupID) {
			return nil, "", ErrInvalid
		}
		return []string{"backup", "delete", input.BackupID}, input.BackupID, nil
	case "restore":
		if !backupPattern.MatchString(input.BackupID) {
			return nil, "", ErrInvalid
		}
		return []string{"restore", input.BackupID}, input.BackupID, nil
	case "uninstall":
		return []string{"uninstall", strconv.FormatBool(input.BackupBeforeChange)}, "web", nil
	default:
		return nil, "", ErrInvalid
	}
}

func actionSecret(input ActionRequest) ([]byte, error) {
	cloudflareAction := input.Action == "protection.configure" &&
		(input.Operation == "cloudflare-fail2ban" || input.Operation == "cloudflare-shield")
	if !cloudflareAction {
		if input.CloudflareAccount != "" || input.CloudflareToken != "" || input.CloudflareZoneID != "" {
			return nil, ErrInvalid
		}
		return nil, nil
	}
	if !validSecretPart(input.CloudflareAccount, 254) || !validSecretPart(input.CloudflareToken, 512) {
		return nil, ErrInvalid
	}
	if input.Operation == "cloudflare-shield" && !validSecretPart(input.CloudflareZoneID, 128) {
		return nil, ErrInvalid
	}
	if input.Operation == "cloudflare-fail2ban" && input.CloudflareZoneID != "" {
		return nil, ErrInvalid
	}
	return []byte(input.CloudflareAccount + "\n" + input.CloudflareToken + "\n" + input.CloudflareZoneID + "\n"), nil
}

func validSecretPart(value string, maximum int) bool {
	return value != "" && len(value) <= maximum &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (s *Service) Jobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	jobs := s.jobsLocked()
	for index := range jobs {
		jobs[index] = s.refreshLocked(jobs[index])
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })
	return jobs
}

func (s *Service) Job(id string) (Job, error) {
	if !jobIDPattern.MatchString(id) {
		return Job{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	job, err := s.readJob(id)
	if err != nil {
		return Job{}, ErrNotFound
	}
	return s.refreshLocked(job), nil
}

func (s *Service) refreshLocked(job Job) Job {
	if job.Status != "queued" && job.Status != "running" {
		return job
	}
	job = s.refreshProgressLocked(job)
	data, err := os.ReadFile(s.receiptPath(job.ID))
	if err != nil {
		if job.StartedAt != nil && s.now().Sub(*job.StartedAt) > 3*time.Second {
			active, statusErr := s.environmentUnitActive(job.ID)
			if statusErr == nil && !active {
				job.Status, job.Stage, job.Progress = "needs_attention", "receipt_missing", 100
				job.Message = "后台任务已经退出，但未写入可信完成凭据；请查看终端输出并人工复核环境状态"
				finished := s.now().UTC()
				job.FinishedAt = &finished
				_ = os.Remove(s.secretPath(job.ID))
				_ = s.writeJob(job)
			}
		}
		return job
	}
	var receipt struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &receipt) != nil {
		return job
	}
	job.Status, job.Stage, job.Progress = receipt.Status, "complete", 100
	switch receipt.Status {
	case "succeeded":
	case "needs_attention":
		job.Stage = "attention_required"
	default:
		job.Status, job.Stage = "failed", "failed"
	}
	job.Message = receipt.Message
	finished := s.now().UTC()
	job.FinishedAt = &finished
	_ = os.Remove(s.secretPath(job.ID))
	_ = s.writeJob(job)
	return job
}

func (s *Service) refreshProgressLocked(job Job) Job {
	data, err := readTail(s.logPath(job.ID), maxProgressBytes)
	if err != nil {
		return job
	}
	for _, line := range strings.Split(string(data), "\n") {
		const marker = "KPANEL_LDNMP_EVENT "
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, marker) {
			continue
		}
		var event struct {
			Stage    string `json:"stage"`
			Progress int    `json:"progress"`
			Message  string `json:"message"`
		}
		if json.Unmarshal([]byte(strings.TrimPrefix(line, marker)), &event) == nil &&
			event.Progress >= job.Progress && event.Progress >= 0 && event.Progress <= 100 {
			job.Stage, job.Progress, job.Message = event.Stage, event.Progress, event.Message
		}
	}
	_ = s.writeJob(job)
	return job
}

func readTail(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	offset := info.Size() - limit
	if offset < 0 {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(file, limit))
}

func (s *Service) environmentUnitActive(id string) (bool, error) {
	executable, err := os.Executable()
	if err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	state, err := jobcontrol.Inspect(ctx, backgroundRunner{}, s.backgroundSpec(id, executable))
	if err != nil {
		return false, err
	}
	return state.Running, nil
}

func (s *Service) backgroundSpec(id string, executable string) jobcontrol.Spec {
	return jobcontrol.Spec{
		Unit:       "kpanel-env-" + id,
		Executable: filepath.Clean(executable),
		Arguments:  []string{"environment-run", "--state-dir", s.stateDir, "--id", id},
		StateDir:   s.stateDir,
		SystemdProperties: []string{
			"Type=oneshot", "TimeoutStartSec=90min", "TimeoutStopSec=10min",
			"User=root", "UMask=0027", "PrivateTmp=yes", "NoNewPrivileges=no",
			"SyslogIdentifier=kpanel-web-environment",
		},
		UMask:       "0027",
		Nice:        5,
		IOClass:     "2:6",
		StopTimeout: 10 * time.Minute,
	}
}

func safeJobError(err error) string {
	value := strings.TrimSpace(err.Error())
	if len(value) > 300 {
		value = value[:300]
	}
	return value
}

func (s *Service) Terminal(id string, offset int64) (TerminalChunk, error) {
	job, err := s.Job(id)
	if err != nil {
		return TerminalChunk{}, err
	}
	if offset < 0 {
		return TerminalChunk{}, ErrInvalid
	}
	file, err := os.Open(s.logPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return TerminalChunk{
			NextOffset: offset,
			InputOpen:  s.inputAvailable(id) && !jobFinished(job.Status),
			Finished:   jobFinished(job.Status),
		}, nil
	}
	if err != nil {
		return TerminalChunk{}, err
	}
	defer file.Close()
	info, _ := file.Stat()
	if offset > info.Size() {
		offset = 0
	}
	_, _ = file.Seek(offset, io.SeekStart)
	data, err := io.ReadAll(io.LimitReader(file, maxTerminalChunk))
	if err != nil {
		return TerminalChunk{}, err
	}
	next := offset + int64(len(data))
	return TerminalChunk{DataBase64: base64.StdEncoding.EncodeToString(data), NextOffset: next,
		InputOpen: s.inputAvailable(id) && !jobFinished(job.Status),
		Finished:  jobFinished(job.Status) && next >= info.Size()}, nil
}

func (s *Service) WriteInput(id, value string) error {
	data := []byte(value)
	if !jobIDPattern.MatchString(id) || len(data) == 0 || len(data) > maxTerminalInput ||
		strings.IndexByte(value, 0) >= 0 {
		return ErrInvalid
	}
	job, err := s.Job(id)
	if err != nil || jobFinished(job.Status) || (job.Status != "queued" && job.Status != "running") {
		return ErrConflict
	}
	if err := hostpty.WriteInput(s.inputPath(id), data); err != nil {
		return fmt.Errorf("%w: environment terminal input is unavailable", ErrConflict)
	}
	return nil
}

func (s *Service) inputAvailable(id string) bool {
	info, err := os.Lstat(s.inputPath(id))
	return err == nil && info.Mode()&os.ModeNamedPipe != 0
}

func jobFinished(status string) bool {
	return status == "succeeded" || status == "failed" || status == "needs_attention"
}

func (s *Service) Backups() ([]Backup, error) {
	return backupsIn("/home")
}

func backupsIn(root string) ([]Backup, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	result := []Backup{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !backupPattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		archivePath := filepath.Join(root, entry.Name())
		_, sidecarErr := os.Stat(archivePath + ".kpanel.json")
		format := "legacy"
		verified := false
		if sidecarErr == nil {
			format = "kejilion-ldnmp-v1"
			verified = verifyBackupSidecar(archivePath)
		}
		result = append(result, Backup{ID: entry.Name(), SizeBytes: info.Size(),
			CreatedAt: info.ModTime().UTC(), Verified: verified, Format: format})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func verifyBackupSidecar(archive string) bool {
	data, err := os.ReadFile(archive + ".kpanel.json")
	if err != nil || len(data) > 4096 {
		return false
	}
	var sidecar struct {
		File   string `json:"file"`
		SHA256 string `json:"sha256"`
	}
	if json.Unmarshal(data, &sidecar) != nil || filepath.Base(archive) != sidecar.File ||
		!sha256Pattern.MatchString(sidecar.SHA256) {
		return false
	}
	file, err := os.Open(archive)
	if err != nil {
		return false
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return false
	}
	return hex.EncodeToString(digest.Sum(nil)) == sidecar.SHA256
}

// OpenBackup opens a backup archive and confirms the opened file is the
// regular file that was checked, so a path swapped for a symlink between the
// check and the open is refused instead of followed.
func (s *Service) OpenBackup(id string) (*os.File, os.FileInfo, error) {
	path, checked, err := backupFileInfo(id)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(checked, opened) {
		file.Close()
		return nil, nil, ErrNotFound
	}
	return file, opened, nil
}

func backupFileInfo(id string) (string, os.FileInfo, error) {
	if !backupPattern.MatchString(id) {
		return "", nil, ErrNotFound
	}
	path := filepath.Join("/home", id)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, ErrNotFound
	}
	return path, info, nil
}

func (s *Service) jobsLocked() []Job {
	entries, _ := os.ReadDir(s.stateDir)
	result := []Job{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		job, err := s.readJob(strings.TrimSuffix(entry.Name(), ".json"))
		if err == nil {
			result = append(result, job)
		}
	}
	return result
}

func (s *Service) writeJob(job Job) error {
	data, err := json.Marshal(job)
	if err != nil || len(data) > maxJobBytes {
		return errors.New("invalid web environment job")
	}
	tmp := s.jobPath(job.ID) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.jobPath(job.ID))
}

func (s *Service) readJob(id string) (Job, error) {
	var job Job
	data, err := os.ReadFile(s.jobPath(id))
	if err != nil || len(data) > maxJobBytes || json.Unmarshal(data, &job) != nil || job.ID != id {
		return job, ErrNotFound
	}
	return job, nil
}

func (s *Service) jobPath(id string) string     { return filepath.Join(s.stateDir, id+".json") }
func (s *Service) logPath(id string) string     { return filepath.Join(s.stateDir, id+".log") }
func (s *Service) receiptPath(id string) string { return filepath.Join(s.stateDir, id+".receipt") }
func (s *Service) secretPath(id string) string  { return filepath.Join(s.stateDir, id+".secret") }
func (s *Service) inputPath(id string) string   { return filepath.Join(s.stateDir, id+".terminal.input") }
func (s *Service) argumentsPath(id string) string {
	return filepath.Join(s.stateDir, id+".arguments.json")
}

func (s *Service) writeArguments(id string, args []string) error {
	if !jobIDPattern.MatchString(id) || len(args) == 0 || len(args) > 8 {
		return ErrInvalid
	}
	data, err := json.Marshal(args)
	if err != nil || len(data) > 2048 {
		return ErrInvalid
	}
	return os.WriteFile(s.argumentsPath(id), data, 0o600)
}

func (s *Service) readArguments(id string) ([]string, error) {
	data, err := os.ReadFile(s.argumentsPath(id))
	if err != nil || len(data) > 2048 {
		return nil, ErrInvalid
	}
	var args []string
	if json.Unmarshal(data, &args) != nil || len(args) == 0 || len(args) > 8 {
		return nil, ErrInvalid
	}
	return args, nil
}

func RunJob(ctx context.Context, stateDir, id string) error {
	if currentEUID() != 0 {
		return errors.New("environment-run requires root")
	}
	cleanStateDir := filepath.Clean(strings.TrimSpace(stateDir))
	if !jobIDPattern.MatchString(id) || !filepath.IsAbs(cleanStateDir) ||
		cleanStateDir == string(filepath.Separator) {
		return errors.New("invalid environment job")
	}
	service := &Service{stateDir: cleanStateDir, now: time.Now}
	job, err := service.readJob(id)
	if err != nil {
		return errors.New("environment job is unavailable")
	}
	script, err := trustedScript()
	if err != nil {
		return err
	}
	args, err := service.readArguments(id)
	if err != nil || !storedArgumentsAllowed(job.Action, args) {
		return errors.New("environment job arguments are invalid")
	}
	logFile, err := os.OpenFile(service.logPath(id), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	input, err := hostpty.OpenInput(service.inputPath(id))
	if err != nil {
		return err
	}
	defer input.Close()
	defer hostpty.RemoveInput(service.inputPath(id))
	defer os.Remove(service.secretPath(id))

	command := exec.CommandContext(ctx, script, append([]string{"web", "env"}, args...)...)
	command.Env = append(
		os.Environ(),
		"KJ_LDNMP_NONINTERACTIVE=1",
		"KJ_LDNMP_PROTOCOL=1",
		"KJ_LDNMP_RECEIPT="+service.receiptPath(id),
		"KJ_LDNMP_SECRET_FILE="+service.secretPath(id),
		"LC_ALL=C.UTF-8",
		"LANG=C.UTF-8",
		"TERM=xterm-256color",
	)
	terminal, err := hostpty.Start(command, 40, 140)
	if err != nil {
		return err
	}
	inputDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(terminal, input)
		close(inputDone)
	}()
	writer := &environmentLogWriter{target: logFile, remaining: maxLogBytes}
	_, readErr := io.Copy(writer, terminal)
	if readErr != nil && !hostpty.IsEnd(readErr) {
		_ = terminal.Kill()
	}
	runErr := terminal.Wait()
	_ = input.Close()
	select {
	case <-inputDone:
	case <-time.After(250 * time.Millisecond):
	}
	_ = terminal.Close()
	if runErr == nil && readErr != nil && !hostpty.IsEnd(readErr) {
		runErr = readErr
	}
	_ = logFile.Sync()
	return runErr
}

func storedArgumentsAllowed(action string, args []string) bool {
	switch action {
	case "install":
		return len(args) == 2 && args[0] == "install" && (args[1] == "full" || args[1] == "nginx")
	case "protection.configure":
		return len(args) == 2 && args[0] == "protect" && map[string]bool{
			"fail2ban-install": true, "fail2ban-uninstall": true, "unban-all": true,
			"waf-on": true, "waf-off": true, "ddos-on": true, "ddos-off": true,
			"cloudflare-fail2ban": true, "cloudflare-shield": true,
		}[args[1]]
	case "optimization.apply":
		return len(args) == 2 && args[0] == "optimize" && map[string]bool{
			"standard": true, "high": true, "gzip-on": true, "gzip-off": true,
			"brotli-on": true, "brotli-off": true, "zstd-on": true, "zstd-off": true,
		}[args[1]]
	case "update.component", "update.all":
		return len(args) == 4 && args[0] == "update" &&
			map[string]bool{"nginx": true, "mysql": true, "php": true, "redis": true, "all": true}[args[1]] &&
			versionPattern.MatchString(args[2]) && (args[3] == "true" || args[3] == "false")
	case "backup.create":
		return len(args) == 1 && args[0] == "backup"
	case "backup.delete":
		return len(args) == 3 && args[0] == "backup" && args[1] == "delete" && backupPattern.MatchString(args[2])
	case "restore":
		return len(args) == 2 && args[0] == "restore" && backupPattern.MatchString(args[1])
	case "uninstall":
		return len(args) == 2 && args[0] == "uninstall" && (args[1] == "true" || args[1] == "false")
	default:
		return false
	}
}

type environmentLogWriter struct {
	target    io.Writer
	remaining int
}

func (writer *environmentLogWriter) Write(data []byte) (int, error) {
	original := len(data)
	if writer.remaining <= 0 {
		return original, nil
	}
	chunk := data
	if len(chunk) > writer.remaining {
		chunk = chunk[:writer.remaining]
	}
	if _, err := writer.target.Write(chunk); err != nil {
		return 0, err
	}
	writer.remaining -= len(chunk)
	return original, nil
}

func trustedScript() (string, error) {
	required := []string{"KJ_LDNMP_NONINTERACTIVE", "kpanel_ldnmp_dispatch()", "KPANEL_LDNMP_RESULT"}
	for _, candidate := range []string{"/home/docker/kpanel/bin/kejilion.sh", "/usr/local/bin/k", "/usr/bin/k", "/root/kejilion.sh"} {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 1024 || info.Size() > 4<<20 ||
			info.Mode().Perm()&0o022 != 0 || !ownerTrusted(info) {
			continue
		}
		content, err := os.ReadFile(resolved)
		if err != nil {
			continue
		}
		value, ok := string(content), true
		for _, token := range required {
			ok = ok && strings.Contains(value, token)
		}
		if ok {
			return resolved, nil
		}
	}
	return "", errors.New("trusted kejilion.sh LDNMP protocol was not found")
}
