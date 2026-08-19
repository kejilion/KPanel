package diagnostics

import (
	"bufio"
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

	"github.com/kejilion/kejilion-panel/internal/hostpty"
)

const (
	jobUnitPrefix    = "kejilion-panel-diagnostic-"
	maxStateBytes    = 256 << 10
	maxLogBytes      = 8 << 20
	maxCatalogBytes  = 256 << 10
	maxPublicLines   = 400
	maxJobRuntime    = 100 * time.Minute
	maxTerminalInput = 16 << 10
	maxTerminalChunk = 64 << 10
)

var (
	ErrNotFound     = errors.New("diagnostic job not found")
	ErrUnsupported  = errors.New("diagnostic protocol is unavailable")
	ErrConflict     = errors.New("another diagnostic job is active")
	ErrInvalidInput = errors.New("invalid diagnostic request")

	jobIDPattern    = regexp.MustCompile(`^[a-f0-9]{32}$`)
	selectorPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
	licensePattern  = regexp.MustCompile(`(?m)^permission_granted="true"\r?$`)
)

type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Check struct {
	ID               string `json:"id"`
	Category         string `json:"category"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	SourceURL        string `json:"sourceUrl"`
	EstimatedMinutes int    `json:"estimatedMinutes"`
	Impact           string `json:"impact"`
}

type Catalog struct {
	Categories []Category `json:"categories"`
	Items      []Check    `json:"items"`
}

type Job struct {
	ID               string     `json:"id"`
	CheckID          string     `json:"checkId"`
	CheckName        string     `json:"checkName"`
	Category         string     `json:"category"`
	SourceURL        string     `json:"sourceUrl"`
	EstimatedMinutes int        `json:"estimatedMinutes"`
	Impact           string     `json:"impact"`
	Status           string     `json:"status"`
	Stage            string     `json:"stage"`
	Progress         int        `json:"progress"`
	Message          string     `json:"message,omitempty"`
	Logs             []string   `json:"logs"`
	CreatedAt        time.Time  `json:"createdAt"`
	StartedAt        *time.Time `json:"startedAt,omitempty"`
	FinishedAt       *time.Time `json:"finishedAt,omitempty"`
	Interactive      bool       `json:"interactive,omitempty"`
	InputOpen        bool       `json:"inputOpen,omitempty"`
}

type record struct {
	Job
	BootID string `json:"bootId,omitempty"`
}

type commandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
	LookPath(string) (string, error)
}

type systemRunner struct{}

func (systemRunner) Run(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	return command.CombinedOutput()
}

func (systemRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

type Service struct {
	mu           sync.Mutex
	stateDir     string
	executable   string
	runner       commandRunner
	scriptFinder func() (string, error)
	now          func() time.Time
	bootID       func() string
	jobs         map[string]record
}

func New(stateDir, executable string) (*Service, error) {
	service := &Service{
		runner:       systemRunner{},
		scriptFinder: findScript,
		now:          time.Now,
		bootID:       currentBootID,
		jobs:         make(map[string]record),
	}
	if err := service.configure(stateDir, executable, service.runner); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) configure(stateDir, executable string, runner commandRunner) error {
	stateDir = filepath.Clean(strings.TrimSpace(stateDir))
	executable = filepath.Clean(strings.TrimSpace(executable))
	if !filepath.IsAbs(stateDir) || stateDir == string(filepath.Separator) {
		return errors.New("diagnostics require a dedicated absolute state path")
	}
	if !filepath.IsAbs(executable) || executable == string(filepath.Separator) {
		return errors.New("diagnostics require an absolute Agent executable")
	}
	if runner == nil {
		return errors.New("diagnostics require a background command runner")
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.bootID == nil {
		s.bootID = currentBootID
	}
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		return fmt.Errorf("create diagnostic state directory: %w", err)
	}
	s.stateDir = stateDir
	s.executable = executable
	s.runner = runner
	s.load()
	s.reconcileInterruptedLocked()
	return nil
}

func (s *Service) Available() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: diagnostics require Linux", ErrUnsupported)
	}
	for _, command := range []string{"env", "bash", "systemd-run"} {
		if _, err := s.runner.LookPath(command); err != nil {
			return fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}
	if _, err := s.scriptFinder(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return nil
}

func (s *Service) Catalog(ctx context.Context) (Catalog, error) {
	if runtime.GOOS != "linux" {
		return Catalog{}, fmt.Errorf("%w: diagnostics require Linux", ErrUnsupported)
	}
	if _, err := s.runner.LookPath("env"); err != nil {
		return Catalog{}, fmt.Errorf("%w: env is unavailable", ErrUnsupported)
	}
	if _, err := s.runner.LookPath("bash"); err != nil {
		return Catalog{}, fmt.Errorf("%w: bash is unavailable", ErrUnsupported)
	}
	script, err := s.scriptFinder()
	if err != nil {
		return Catalog{}, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	catalogContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := s.runner.Run(
		catalogContext,
		"env",
		"KJ_TEST_NONINTERACTIVE=1",
		"LC_ALL=C.UTF-8",
		"LANG=C.UTF-8",
		"bash",
		script,
		"test",
		"list",
	)
	if err != nil {
		return Catalog{}, fmt.Errorf("%w: read kejilion.sh test catalog: %v", ErrUnsupported, err)
	}
	if len(output) > maxCatalogBytes {
		return Catalog{}, fmt.Errorf("%w: test catalog exceeds the size limit", ErrUnsupported)
	}
	return parseCatalog(output)
}

func (s *Service) Start(ctx context.Context, checkID string) (Job, error) {
	if !selectorPattern.MatchString(checkID) {
		return Job{}, fmt.Errorf("%w: unsupported check selector", ErrInvalidInput)
	}
	if err := s.Available(); err != nil {
		return Job{}, err
	}
	catalog, err := s.Catalog(ctx)
	if err != nil {
		return Job{}, err
	}
	var selected *Check
	for index := range catalog.Items {
		if catalog.Items[index].ID == checkID {
			selected = &catalog.Items[index]
			break
		}
	}
	if selected == nil {
		return Job{}, fmt.Errorf("%w: unsupported check selector", ErrInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hasActiveLocked() {
		return Job{}, ErrConflict
	}
	now := s.now().UTC()
	item := record{Job: Job{
		ID: newJobID(), CheckID: selected.ID, CheckName: selected.Name,
		Category: selected.Category, SourceURL: selected.SourceURL,
		EstimatedMinutes: selected.EstimatedMinutes, Impact: selected.Impact,
		Status: "queued", Stage: "queued", Progress: 0,
		Message: "体检任务已提交，正在等待后台执行", Logs: []string{}, CreatedAt: now,
		Interactive: true,
	}, BootID: s.bootID()}
	if !jobIDPattern.MatchString(item.ID) {
		return Job{}, errors.New("generate diagnostic job identity")
	}
	if err := hostpty.CreateInput(s.inputPath(item.ID)); err != nil {
		return Job{}, fmt.Errorf("%w: prepare diagnostic terminal input: %v", ErrUnsupported, err)
	}
	if err := s.putLocked(item); err != nil {
		_ = hostpty.RemoveInput(s.inputPath(item.ID))
		return Job{}, err
	}
	if err := s.launch(ctx, item); err != nil {
		finished := s.now().UTC()
		item.Status = "failed"
		item.Stage = "launch_failed"
		item.Progress = 100
		item.Message = safeMessage(err)
		item.FinishedAt = &finished
		_ = s.putLocked(item)
		_ = hostpty.RemoveInput(s.inputPath(item.ID))
		return Job{}, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return s.publicLocked(item), nil
}

func (s *Service) launch(ctx context.Context, item record) error {
	arguments := []string{
		"--unit=" + jobUnitPrefix + item.ID,
		"--collect",
		"--no-block",
		"--property=Type=oneshot",
		"--property=TimeoutStartSec=90min",
		"--property=TimeoutStopSec=10min",
		"--property=User=root",
		"--property=UMask=0027",
		"--property=PrivateTmp=yes",
		"--property=NoNewPrivileges=no",
		"--property=RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6",
		"--property=Nice=10",
		"--property=CPUWeight=30",
		"--property=IOWeight=30",
		"--property=SyslogIdentifier=kpanel-diagnostic",
		"--",
		s.executable,
		"diagnostic-run",
		"--state-dir",
		s.stateDir,
		"--id",
		item.ID,
	}
	output, err := s.runner.Run(ctx, "systemd-run", arguments...)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		if detail != "" {
			return fmt.Errorf("%s: %w", detail, err)
		}
		return err
	}
	return nil
}

func (s *Service) Job(id string) (Job, error) {
	if !jobIDPattern.MatchString(id) {
		return Job{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileInterruptedLocked()
	item, err := s.readLocked(id)
	if err != nil {
		return Job{}, ErrNotFound
	}
	return s.publicLocked(item), nil
}

func (s *Service) Jobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcileInterruptedLocked()
	records := s.listLocked()
	result := make([]Job, 0, len(records))
	for _, item := range records {
		result = append(result, s.publicLocked(item))
	}
	return result
}

type TerminalChunk struct {
	DataBase64 string `json:"dataBase64"`
	NextOffset int64  `json:"nextOffset"`
	InputOpen  bool   `json:"inputOpen"`
	Finished   bool   `json:"finished"`
}

func (s *Service) Terminal(id string, offset int64) (TerminalChunk, error) {
	if !jobIDPattern.MatchString(id) || offset < 0 {
		return TerminalChunk{}, ErrNotFound
	}
	s.mu.Lock()
	item, err := s.readLocked(id)
	s.mu.Unlock()
	if err != nil || !item.Interactive {
		return TerminalChunk{}, ErrNotFound
	}
	file, err := os.Open(s.logPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return TerminalChunk{
			InputOpen: item.InputOpen,
			Finished:  item.Status == "succeeded" || item.Status == "failed",
		}, nil
	}
	if err != nil {
		return TerminalChunk{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return TerminalChunk{}, err
	}
	if offset > info.Size() {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return TerminalChunk{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTerminalChunk))
	if err != nil {
		return TerminalChunk{}, err
	}
	nextOffset := offset + int64(len(data))
	return TerminalChunk{
		DataBase64: base64.StdEncoding.EncodeToString(data),
		NextOffset: nextOffset,
		InputOpen:  item.InputOpen,
		Finished: (item.Status == "succeeded" || item.Status == "failed") &&
			nextOffset >= info.Size(),
	}, nil
}

func (s *Service) WriteInput(id, value string) error {
	data := []byte(value)
	if !jobIDPattern.MatchString(id) || len(data) == 0 || len(data) > maxTerminalInput ||
		strings.IndexByte(value, 0) >= 0 {
		return ErrInvalidInput
	}
	s.mu.Lock()
	item, err := s.readLocked(id)
	s.mu.Unlock()
	if err != nil || !item.Interactive || !item.InputOpen ||
		(item.Status != "queued" && item.Status != "running") {
		return ErrConflict
	}
	if err := hostpty.WriteInput(s.inputPath(id), data); err != nil {
		return fmt.Errorf("%w: diagnostic terminal input is unavailable: %v", ErrConflict, err)
	}
	return nil
}

func RunJob(ctx context.Context, stateDir, id string) error {
	if os.Geteuid() != 0 {
		return errors.New("diagnostic-run requires root")
	}
	if !jobIDPattern.MatchString(id) {
		return errors.New("invalid diagnostic job identity")
	}
	cleanStateDir := filepath.Clean(strings.TrimSpace(stateDir))
	if !filepath.IsAbs(cleanStateDir) || cleanStateDir == string(filepath.Separator) {
		return errors.New("diagnostics require a dedicated absolute state path")
	}
	service := &Service{
		stateDir: cleanStateDir, runner: systemRunner{}, scriptFinder: findScript,
		now: time.Now, jobs: make(map[string]record),
	}
	service.mu.Lock()
	item, err := service.readLocked(id)
	service.mu.Unlock()
	if err != nil || !selectorPattern.MatchString(item.CheckID) {
		return errors.New("diagnostic job contains an invalid selector")
	}
	script, err := service.scriptFinder()
	if err != nil {
		return service.fail(item, "script_unavailable", err)
	}

	workspace := filepath.Join(cleanStateDir, id+".work")
	if filepath.Dir(workspace) != cleanStateDir {
		return service.fail(item, "workspace_invalid", errors.New("invalid diagnostic workspace"))
	}
	if err := os.Mkdir(workspace, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
		return service.fail(item, "workspace_unavailable", err)
	}
	defer os.RemoveAll(workspace)

	logFile, err := os.OpenFile(
		service.logPath(id),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0o600,
	)
	if err != nil {
		return service.fail(item, "log_unavailable", err)
	}
	defer logFile.Close()
	input, err := hostpty.OpenInput(service.inputPath(id))
	if err != nil {
		return service.fail(item, "terminal_unavailable", err)
	}
	defer input.Close()
	defer hostpty.RemoveInput(service.inputPath(id))
	writer := &limitedWriter{target: logFile, remaining: maxLogBytes}
	writeTerminalHeader(writer, item.CheckName, item.SourceURL)

	started := service.now().UTC()
	item.Status = "running"
	item.Stage = "running"
	item.Progress = 10
	item.Message = "第三方体检脚本正在运行，结果将持续写入日志"
	item.InputOpen = true
	item.StartedAt = &started
	item.FinishedAt = nil
	service.mu.Lock()
	err = service.putLocked(item)
	service.mu.Unlock()
	if err != nil {
		return err
	}

	command := exec.CommandContext(
		ctx,
		"/usr/bin/env",
		"KJ_TEST_NONINTERACTIVE=1",
		"LC_ALL=C.UTF-8",
		"LANG=C.UTF-8",
		"TERM=xterm-256color",
		"/bin/bash",
		script,
		"test",
		"run",
		item.CheckID,
	)
	command.Dir = workspace
	terminal, err := hostpty.Start(command, 36, 120)
	if err != nil {
		return service.fail(item, "terminal_unavailable", err)
	}
	inputDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(terminal, input)
		close(inputDone)
	}()
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

	finished := service.now().UTC()
	item.Progress = 100
	item.FinishedAt = &finished
	item.InputOpen = false
	if runErr != nil {
		item.Status = "failed"
		item.Stage = "failed"
		item.Message = "体检脚本执行失败，请查看日志确认第三方来源或网络状态"
	} else {
		item.Status = "succeeded"
		item.Stage = "completed"
		item.Message = "体检完成，完整跑分结果已保存在任务日志"
	}
	service.mu.Lock()
	putErr := service.putLocked(item)
	service.mu.Unlock()
	if putErr != nil {
		return putErr
	}
	return runErr
}

func writeTerminalHeader(writer io.Writer, checkName, sourceURL string) {
	_, _ = fmt.Fprintf(writer, "KPanel 体检：%s\r\n来源：%s\r\n\r\n", checkName, sourceURL)
}

type limitedWriter struct {
	target    io.Writer
	remaining int
	truncated bool
}

func (writer *limitedWriter) Write(data []byte) (int, error) {
	original := len(data)
	exceedsLimit := len(data) > writer.remaining
	if writer.remaining > 0 {
		chunk := data
		if len(chunk) > writer.remaining {
			chunk = chunk[:writer.remaining]
		}
		if _, err := writer.target.Write(chunk); err != nil {
			return 0, err
		}
		writer.remaining -= len(chunk)
	}
	if exceedsLimit && !writer.truncated {
		writer.truncated = true
		if _, err := io.WriteString(writer.target, "\n[KPanel] 输出超过 8 MiB，后续内容已截断。\n"); err != nil {
			return 0, err
		}
	}
	return original, nil
}

func parseCatalog(output []byte) (Catalog, error) {
	result := Catalog{Categories: []Category{}, Items: []Check{}}
	categories := make(map[string]bool)
	items := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		fields := strings.Split(line, "\t")
		switch {
		case len(fields) == 3 && fields[0] == "KPANEL_TEST_CATEGORY":
			id, name := fields[1], strings.TrimSpace(fields[2])
			if !selectorPattern.MatchString(id) || name == "" || len(name) > 80 || categories[id] {
				return Catalog{}, fmt.Errorf("%w: malformed or duplicate test category", ErrUnsupported)
			}
			categories[id] = true
			result.Categories = append(result.Categories, Category{ID: id, Name: name})
		case len(fields) == 8 && fields[0] == "KPANEL_TEST_ITEM":
			estimated, estimatedErr := strconv.Atoi(fields[6])
			source, sourceErr := url.ParseRequestURI(fields[5])
			validSource := sourceErr == nil && source.Scheme == "https" && source.Host != "" &&
				source.User == nil && source.Fragment == ""
			if !selectorPattern.MatchString(fields[1]) || !selectorPattern.MatchString(fields[2]) ||
				items[fields[1]] || strings.TrimSpace(fields[3]) == "" ||
				len(fields[3]) > 120 || len(fields[4]) > 300 || !validSource ||
				estimatedErr != nil || estimated < 1 || estimated > 120 ||
				(fields[7] != "light" && fields[7] != "network" && fields[7] != "intensive") {
				return Catalog{}, fmt.Errorf("%w: malformed or duplicate test item", ErrUnsupported)
			}
			items[fields[1]] = true
			result.Items = append(result.Items, Check{
				ID: fields[1], Category: fields[2], Name: strings.TrimSpace(fields[3]),
				Description: strings.TrimSpace(fields[4]), SourceURL: fields[5],
				EstimatedMinutes: estimated, Impact: fields[7],
			})
		case strings.HasPrefix(line, "KPANEL_TEST_CATEGORY") ||
			strings.HasPrefix(line, "KPANEL_TEST_ITEM"):
			return Catalog{}, fmt.Errorf("%w: malformed test catalog record", ErrUnsupported)
		}
	}
	if err := scanner.Err(); err != nil {
		return Catalog{}, fmt.Errorf("%w: read test catalog: %v", ErrUnsupported, err)
	}
	if len(result.Categories) == 0 || len(result.Categories) > 12 ||
		len(result.Items) == 0 || len(result.Items) > 64 {
		return Catalog{}, fmt.Errorf("%w: test catalog is empty or too large", ErrUnsupported)
	}
	for _, item := range result.Items {
		if !categories[item.Category] {
			return Catalog{}, fmt.Errorf("%w: test item uses an unknown category", ErrUnsupported)
		}
	}
	return result, nil
}

func findScript() (string, error) {
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
		if err != nil || !info.Mode().IsRegular() || info.Size() < 1024 || info.Size() > 4<<20 {
			continue
		}
		if runtime.GOOS == "linux" && (!trustedScriptOwner(info) || info.Mode().Perm()&0o022 != 0) {
			continue
		}
		content, err := os.ReadFile(resolved)
		if err != nil || !compatibleScript(content) {
			continue
		}
		return resolved, nil
	}
	return "", errors.New("a trusted kejilion.sh diagnostic protocol was not found")
}

func compatibleScript(content []byte) bool {
	value := string(content)
	return licensePattern.MatchString(value) &&
		strings.Contains(value, "KJ_TEST_NONINTERACTIVE") &&
		strings.Contains(value, "kpanel_test_catalog") &&
		strings.Contains(value, "kpanel_run_test_noninteractive")
}

func (s *Service) fail(item record, stage string, cause error) error {
	finished := s.now().UTC()
	item.Status = "failed"
	item.Stage = stage
	item.Progress = 100
	item.Message = safeMessage(cause)
	item.InputOpen = false
	item.FinishedAt = &finished
	_ = hostpty.RemoveInput(s.inputPath(item.ID))
	s.mu.Lock()
	_ = s.putLocked(item)
	s.mu.Unlock()
	return cause
}

func (s *Service) load() {
	entries, err := os.ReadDir(s.stateDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if item, err := s.readLocked(id); err == nil {
			s.jobs[id] = item
		}
	}
	s.pruneLocked()
}

func (s *Service) hasActiveLocked() bool {
	s.reconcileInterruptedLocked()
	for _, item := range s.listLocked() {
		if item.Status == "queued" || item.Status == "running" {
			return true
		}
	}
	return false
}

func (s *Service) reconcileInterruptedLocked() {
	currentBoot := s.bootID()
	now := s.now().UTC()
	for _, item := range s.jobs {
		if item.Status != "queued" && item.Status != "running" {
			continue
		}
		bootChanged := item.BootID != "" && currentBoot != "" && item.BootID != currentBoot
		timedOut := !item.CreatedAt.IsZero() && now.Sub(item.CreatedAt) > maxJobRuntime
		if !bootChanged && !timedOut {
			continue
		}
		finished := now
		item.Status = "failed"
		item.Stage = "interrupted"
		item.Progress = 100
		item.Message = "体检任务因系统重启或运行超时而中断，请重新执行"
		item.InputOpen = false
		item.FinishedAt = &finished
		_ = s.putLocked(item)
		_ = hostpty.RemoveInput(s.inputPath(item.ID))
	}
}

func (s *Service) putLocked(item record) error {
	if !jobIDPattern.MatchString(item.ID) {
		return errors.New("invalid diagnostic job identity")
	}
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxStateBytes {
		return errors.New("diagnostic job state exceeds the size limit")
	}
	temp, err := os.CreateTemp(s.stateDir, "."+item.ID+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	target := s.statePath(item.ID)
	if runtime.GOOS == "windows" {
		_ = os.Remove(target)
	}
	if err := os.Rename(tempPath, target); err != nil {
		return err
	}
	s.jobs[item.ID] = item
	s.pruneLocked()
	return nil
}

func (s *Service) readLocked(id string) (record, error) {
	if !jobIDPattern.MatchString(id) {
		return record{}, os.ErrNotExist
	}
	data, err := os.ReadFile(s.statePath(id))
	if err != nil || len(data) > maxStateBytes {
		return record{}, os.ErrNotExist
	}
	var item record
	if json.Unmarshal(data, &item) != nil || item.ID != id ||
		!selectorPattern.MatchString(item.CheckID) {
		return record{}, os.ErrNotExist
	}
	s.jobs[id] = item
	return item, nil
}

func (s *Service) listLocked() []record {
	records := make([]record, 0, len(s.jobs))
	for id := range s.jobs {
		if item, err := s.readLocked(id); err == nil {
			records = append(records, item)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records
}

func (s *Service) publicLocked(item record) Job {
	job := item.Job
	job.Logs = s.logTail(item.ID)
	return job
}

func (s *Service) logTail(id string) []string {
	data, err := os.ReadFile(s.logPath(id))
	if err != nil {
		return []string{}
	}
	if len(data) > maxLogBytes {
		data = data[len(data)-maxLogBytes:]
	}
	clean := stripControls(string(data))
	lines := strings.Split(strings.TrimRight(clean, "\r\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}
	}
	if len(lines) > maxPublicLines {
		lines = lines[len(lines)-maxPublicLines:]
	}
	return lines
}

func stripControls(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	escape := false
	csi := false
	for _, character := range value {
		if escape {
			if character == '[' {
				csi = true
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
		if character == '\t' || character == '\n' || character == '\r' || character >= 0x20 {
			result.WriteRune(character)
		}
	}
	return result.String()
}

func (s *Service) pruneLocked() {
	if len(s.jobs) <= 50 {
		return
	}
	terminal := make([]record, 0, len(s.jobs))
	for _, item := range s.jobs {
		if item.Status != "queued" && item.Status != "running" {
			terminal = append(terminal, item)
		}
	}
	sort.Slice(terminal, func(i, j int) bool {
		return terminal[i].CreatedAt.Before(terminal[j].CreatedAt)
	})
	removeCount := len(s.jobs) - 50
	if removeCount > len(terminal) {
		removeCount = len(terminal)
	}
	for _, item := range terminal[:removeCount] {
		delete(s.jobs, item.ID)
		_ = os.Remove(s.statePath(item.ID))
		_ = os.Remove(s.logPath(item.ID))
	}
}

func (s *Service) statePath(id string) string {
	return filepath.Join(s.stateDir, id+".json")
}

func (s *Service) logPath(id string) string {
	return filepath.Join(s.stateDir, id+".log")
}

func (s *Service) inputPath(id string) string {
	return filepath.Join(s.stateDir, id+".input")
}

func newJobID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(value[:])
}

func safeMessage(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 300 {
		value = value[:300]
	}
	if value == "" {
		return "体检任务失败"
	}
	return value
}

func currentBootID() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	value, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	bootID := strings.TrimSpace(string(value))
	if len(bootID) > 128 {
		return ""
	}
	return bootID
}
