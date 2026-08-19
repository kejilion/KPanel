package appmarket

import (
	"bufio"
	"context"
	"crypto/rand"
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
)

const (
	appJobUnitPrefix  = "kejilion-panel-app-"
	maxAppJobBytes    = 256 << 10
	maxAppJobLog      = 1 << 20
	appJobLaunchGrace = 15 * time.Second
)

var (
	appJobIDPattern    = regexp.MustCompile(`^[a-f0-9]{32}$`)
	appSelectorPattern = regexp.MustCompile(`^(?:[1-9][0-9]{0,2}|[A-Za-z0-9][A-Za-z0-9_-]{0,63})$`)
	containerIDPattern = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
	appProgressPattern = regexp.MustCompile(`^KPANEL_PROGRESS ([0-9]{1,3}) (.+)$`)
	appLicensePattern  = regexp.MustCompile(`(?m)^permission_granted="true"\r?$`)
)

type AppJob struct {
	ID          string     `json:"id"`
	AppID       string     `json:"appId"`
	AppName     string     `json:"appName"`
	Action      string     `json:"action"`
	Interactive bool       `json:"interactive,omitempty"`
	InputOpen   bool       `json:"inputOpen,omitempty"`
	Status      string     `json:"status"`
	Stage       string     `json:"stage"`
	Progress    int        `json:"progress"`
	Message     string     `json:"message,omitempty"`
	Logs        []string   `json:"logs"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
}

type appJobRecord struct {
	AppJob
	Selector            string `json:"selector"`
	HostPort            uint16 `json:"hostPort,omitempty"`
	AccessMode          string `json:"accessMode,omitempty"`
	Adapter             string `json:"adapter"`
	ExpectedContainerID string `json:"expectedContainerId,omitempty"`
}

type appJobRegistry struct {
	mu       sync.Mutex
	stateDir string
	jobs     map[string]appJobRecord
}

type jobCommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
	LookPath(string) (string, error)
}

type systemJobRunner struct{}

func (systemJobRunner) Run(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		if detail != "" {
			return nil, fmt.Errorf("%s: %w", detail, err)
		}
		return nil, err
	}
	return output, nil
}

func (systemJobRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (s *Service) ConfigureJobs(stateDir, executable string) error {
	return s.configureJobs(stateDir, executable, systemJobRunner{})
}

func (s *Service) configureJobs(stateDir, executable string, runner jobCommandRunner) error {
	stateDir = filepath.Clean(stateDir)
	executable = filepath.Clean(executable)
	if !filepath.IsAbs(stateDir) || stateDir == string(filepath.Separator) ||
		!filepath.IsAbs(executable) || runner == nil {
		return errors.New("application jobs require dedicated absolute paths")
	}
	if err := ensureAppJobDirectory(stateDir); err != nil {
		return err
	}
	registry := &appJobRegistry{stateDir: stateDir, jobs: make(map[string]appJobRecord)}
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 ||
			!strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !appJobIDPattern.MatchString(id) {
			continue
		}
		record, readErr := registry.read(id)
		if readErr == nil {
			registry.jobs[id] = record
		}
	}
	s.jobs = registry
	s.jobExecutable = executable
	s.jobRunner = runner
	s.recoverInterruptedJobs()
	return nil
}

func (s *Service) recoverInterruptedJobs() {
	for _, record := range s.jobs.list() {
		if record.Status != "queued" && record.Status != "running" {
			continue
		}
		if record.Adapter == "kejilion" {
			running, known := s.scriptJobUnitState(record.ID)
			if running || !known {
				continue
			}
		}
		if s.jobs.cancelRequested(record.ID) {
			s.finishCancelledJob(record)
			continue
		}
		finished := s.now().UTC()
		record.Status = "failed"
		record.Stage = "interrupted"
		record.Progress = 100
		record.Message = "后台应用任务已被 Agent 或服务器重启中断，请核对应用状态后重试"
		record.InputOpen = false
		record.FinishedAt = &finished
		_ = s.jobs.put(record)
		_ = removeTerminalInput(s.jobs.inputPath(record.ID))
	}
}

func (s *Service) reconcileInactiveScriptJobs() {
	if s.jobs == nil || s.jobRunner == nil {
		return
	}
	for _, record := range s.jobs.list() {
		if record.Adapter != "kejilion" ||
			(record.Status != "queued" && record.Status != "running") ||
			(record.Stage != "cancelling" && s.now().Sub(record.CreatedAt) < appJobLaunchGrace) {
			continue
		}
		running, known := s.scriptJobUnitState(record.ID)
		if running || !known {
			continue
		}
		latest, readErr := s.jobs.read(record.ID)
		if readErr != nil || (latest.Status != "queued" && latest.Status != "running") {
			continue
		}
		if s.jobs.cancelRequested(latest.ID) {
			s.finishCancelledJob(latest)
			continue
		}
		finished := s.now().UTC()
		latest.Status = "failed"
		latest.Stage = "interrupted"
		latest.Progress = 100
		latest.Message = "上一个应用任务已结束但状态未回写，已自动释放任务锁；请核对应用实际状态后重试"
		latest.InputOpen = false
		latest.FinishedAt = &finished
		_ = s.jobs.put(latest)
		_ = removeTerminalInput(s.jobs.inputPath(latest.ID))
	}
}

func (s *Service) finishCancelledJob(record appJobRecord) {
	finished := s.now().UTC()
	record.Status = "cancelled"
	record.Stage = "cancelled"
	record.Progress = 100
	record.Message = "交互任务已由管理员手动结束，应用状态将按宿主机实际产物重新读取"
	record.InputOpen = false
	record.FinishedAt = &finished
	if err := s.jobs.put(record); err != nil {
		return
	}
	_ = removeTerminalInput(s.jobs.inputPath(record.ID))
	_ = os.Remove(s.jobs.cancelPath(record.ID))
}

func (s *Service) scriptJobUnitState(id string) (running bool, known bool) {
	if s.jobRunner == nil || !appJobIDPattern.MatchString(id) {
		return false, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := s.jobRunner.Run(
		ctx,
		"systemctl",
		"show",
		"--property=ActiveState",
		"--value",
		appJobUnitPrefix+id+".service",
	)
	if err != nil {
		return false, false
	}
	switch strings.TrimSpace(string(output)) {
	case "active", "activating", "reloading", "deactivating":
		return true, true
	case "inactive", "failed", "dead":
		return false, true
	default:
		return false, false
	}
}

func ensureAppJobDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o750); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("application job state directory is unavailable or unsafe")
	}
	return nil
}

func (s *Service) StartInstall(
	ctx context.Context,
	id string,
	input InstallInput,
) (AppJob, error) {
	s.actions.Lock()
	defer s.actions.Unlock()
	if s.jobs == nil {
		return AppJob{}, fmt.Errorf("%w: background application jobs are unavailable", ErrUnsupported)
	}
	item, err := s.Find(ctx, id)
	if err != nil {
		return AppJob{}, err
	}
	if !item.Capabilities["install"].Enabled {
		return AppJob{}, fmt.Errorf("%w: %s", ErrForbidden, item.Capabilities["install"].Reason)
	}
	if item.DefaultPort < 0 || item.DefaultPort > 65535 {
		return AppJob{}, fmt.Errorf("%w: application default port is invalid", ErrUnsupported)
	}
	if item.InstallPortConfigurable {
		if input.HostPort == 0 {
			input.HostPort = uint16(item.DefaultPort)
		}
		portStatus, portErr := s.inspectInstallPort(ctx, input.HostPort)
		if portErr != nil {
			return AppJob{}, fmt.Errorf(
				"%w: host port validation failed: %v",
				ErrNeedsAttention,
				portErr,
			)
		}
		if !portStatus.Available {
			return AppJob{}, fmt.Errorf(
				"%w: host port %d is already bound by another listener or container",
				ErrPortConflict,
				input.HostPort,
			)
		}
	} else if input.HostPort != 0 {
		return AppJob{}, fmt.Errorf(
			"%w: this application does not expose a single configurable install port",
			ErrUnsupported,
		)
	}
	if input.AccessMode == "" {
		input.AccessMode = "direct"
	}
	s.reconcileInactiveScriptJobs()
	if s.jobs.hasActive() {
		return AppJob{}, ErrTaskConflict
	}

	selector, scriptBacked := s.scriptSelector(item)
	adapter := "declarative"
	if scriptBacked {
		adapter = "kejilion"
	}
	input.Interactive = scriptBacked
	if scriptBacked {
		if s.scriptInteractiveFinder == nil {
			return AppJob{}, fmt.Errorf("%w: interactive kejilion.sh protocol is unavailable", ErrUnsupported)
		}
		if _, err := s.scriptInteractiveFinder(); err != nil {
			return AppJob{}, fmt.Errorf(
				"%w: the installed kejilion.sh does not support KPanel interactive jobs",
				ErrUnsupported,
			)
		}
	}
	record, err := newAppJobRecord(item, selector, adapter, "install", input, "")
	if err != nil {
		return AppJob{}, err
	}
	if err := s.jobs.put(record); err != nil {
		return AppJob{}, fmt.Errorf("%w: persist application job: %v", ErrNeedsAttention, err)
	}

	if scriptBacked {
		if err := s.launchScriptJob(ctx, record); err != nil {
			finished := s.now().UTC()
			record.Status = "failed"
			record.Stage = "launch_failed"
			record.Progress = 100
			record.Message = safeAppJobMessage(err)
			record.FinishedAt = &finished
			_ = s.jobs.put(record)
			return AppJob{}, fmt.Errorf("%w: launch background application task: %v", ErrNeedsAttention, err)
		}
		return s.jobs.public(record), nil
	}

	go s.runDeclarativeInstall(record, input)
	return s.jobs.public(record), nil
}

func newAppJobRecord(
	item Summary,
	selector, adapter, action string,
	input InstallInput,
	expectedContainerID string,
) (appJobRecord, error) {
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return appJobRecord{}, err
	}
	now := time.Now().UTC()
	return appJobRecord{
		AppJob: AppJob{
			ID: hex.EncodeToString(idBytes[:]), AppID: item.ID, AppName: item.NameZH,
			Action: action, Status: "queued", Stage: "queued", Progress: 0,
			Message: appActionLabel(action) + "任务已进入后台队列", Logs: []string{}, CreatedAt: now,
			Interactive: input.Interactive,
		},
		Selector: selector, HostPort: input.HostPort,
		AccessMode: input.AccessMode, Adapter: adapter,
		ExpectedContainerID: expectedContainerID,
	}, nil
}

func (s *Service) StartScriptMutation(
	ctx context.Context,
	id, action string,
	input MutationInput,
) (AppJob, bool, error) {
	if action != "update" && action != "uninstall" &&
		action != "direct_access" && action != "manage" {
		return AppJob{}, false, ErrUnsupported
	}
	s.actions.Lock()
	defer s.actions.Unlock()
	item, err := s.Find(ctx, id)
	if err != nil {
		return AppJob{}, false, err
	}
	if _, declarative := declarativeSpecs[item.Token]; declarative {
		return AppJob{}, false, nil
	}
	selector, scriptBacked := s.scriptSelectorFor(item)
	if !scriptBacked {
		return AppJob{}, false, nil
	}
	if s.jobs == nil {
		return AppJob{}, true, fmt.Errorf("%w: background application jobs are unavailable", ErrUnsupported)
	}
	scriptFinder := s.scriptInteractiveFinder
	if action == "manage" {
		scriptFinder = s.scriptInteractiveManageFinder
	}
	if scriptFinder == nil {
		return AppJob{}, true, fmt.Errorf("%w: interactive kejilion.sh protocol is unavailable", ErrUnsupported)
	}
	if _, err := scriptFinder(); err != nil {
		return AppJob{}, true, fmt.Errorf(
			"%w: the installed kejilion.sh does not support KPanel interactive jobs",
			ErrUnsupported,
		)
	}
	if !item.Capabilities[action].Enabled {
		return AppJob{}, true, fmt.Errorf("%w: %s", ErrForbidden, item.Capabilities[action].Reason)
	}
	if input.ResourceVersion == "" || input.ResourceVersion != item.Runtime.ResourceVersion {
		return AppJob{}, true, ErrConflict
	}
	expectedContainerID := item.Runtime.ContainerID
	if action == "manage" {
		if expectedContainerID != "" {
			return AppJob{}, true, fmt.Errorf(
				"%w: script recovery is available only when the application container is missing",
				ErrConflict,
			)
		}
	} else if !containerIDPattern.MatchString(expectedContainerID) {
		return AppJob{}, true, ErrConflict
	}
	if action == "direct_access" && input.AccessMode != "direct" && input.AccessMode != "domain_only" {
		return AppJob{}, true, fmt.Errorf("%w: invalid access mode", ErrForbidden)
	}
	s.reconcileInactiveScriptJobs()
	if s.jobs.hasActive() {
		return AppJob{}, true, ErrTaskConflict
	}
	record, err := newAppJobRecord(
		item,
		selector,
		"kejilion",
		action,
		InstallInput{AccessMode: input.AccessMode},
		expectedContainerID,
	)
	if err != nil {
		return AppJob{}, true, err
	}
	record.Interactive = true
	if err := s.jobs.put(record); err != nil {
		return AppJob{}, true, fmt.Errorf("%w: persist application job: %v", ErrNeedsAttention, err)
	}
	if err := s.launchScriptJob(ctx, record); err != nil {
		finished := s.now().UTC()
		record.Status = "failed"
		record.Stage = "launch_failed"
		record.Progress = 100
		record.Message = safeAppJobMessage(err)
		record.FinishedAt = &finished
		_ = s.jobs.put(record)
		return AppJob{}, true, fmt.Errorf("%w: launch background application task: %v", ErrNeedsAttention, err)
	}
	return s.jobs.public(record), true, nil
}

func (s *Service) runDeclarativeInstall(record appJobRecord, input InstallInput) {
	started := s.now().UTC()
	record.Status = "running"
	record.Stage = "preparing"
	record.Progress = 10
	record.Message = "正在校验端口与 Docker 环境"
	record.StartedAt = &started
	if s.jobs.put(record) != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	record.Stage = "installing"
	record.Progress = 35
	record.Message = "正在拉取镜像并创建应用容器"
	_ = s.jobs.put(record)
	result, err := s.Install(ctx, record.AppID, input)
	finished := s.now().UTC()
	record.FinishedAt = &finished
	record.Progress = 100
	if err != nil {
		record.Status = "failed"
		record.Stage = "failed"
		record.Message = safeAppJobMessage(err)
	} else {
		record.Status = "succeeded"
		record.Stage = "completed"
		record.Message = "应用安装完成，Docker 与 kejilion.sh 兼容状态已对账"
		record.Logs = []string{
			fmt.Sprintf("container=%s", result.ContainerID),
			fmt.Sprintf("resourceVersion=%s", result.ResourceVersion),
		}
	}
	_ = s.jobs.put(record)
}

func (s *Service) launchScriptJob(ctx context.Context, record appJobRecord) error {
	if s.jobRunner == nil || s.jobExecutable == "" {
		return errors.New("application background runner is unavailable")
	}
	if _, err := s.jobRunner.LookPath("systemd-run"); err != nil {
		return errors.New("systemd background task runner is unavailable")
	}
	subcommand := "app-run"
	if record.Interactive {
		if err := createTerminalInput(s.jobs.inputPath(record.ID)); err != nil {
			return fmt.Errorf("prepare interactive input: %w", err)
		}
		subcommand = "app-pty-run"
	}
	arguments := []string{
		"--unit=" + appJobUnitPrefix + record.ID,
		"--collect",
		"--no-block",
		"--property=Type=oneshot",
		"--property=TimeoutStartSec=45min",
		"--property=TimeoutStopSec=10min",
		"--property=User=root",
		"--property=UMask=0027",
		"--property=PrivateTmp=yes",
		"--property=NoNewPrivileges=no",
		"--property=RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK",
		"--property=Nice=5",
		"--property=CPUWeight=40",
		"--property=IOWeight=40",
		"--property=SyslogIdentifier=kpanel-app",
		"--",
		s.jobExecutable,
		subcommand,
		"--state-dir",
		s.jobs.stateDir,
		"--id",
		record.ID,
	}
	_, err := s.jobRunner.Run(ctx, "systemd-run", arguments...)
	if err != nil && record.Interactive {
		_ = removeTerminalInput(s.jobs.inputPath(record.ID))
	}
	return err
}

func (s *Service) AppJob(id string) (AppJob, error) {
	if s.jobs == nil || !appJobIDPattern.MatchString(id) {
		return AppJob{}, ErrNotFound
	}
	s.reconcileInactiveScriptJobs()
	record, err := s.jobs.read(id)
	if err != nil {
		return AppJob{}, ErrNotFound
	}
	return s.jobs.public(record), nil
}

func (s *Service) AppJobs() []AppJob {
	if s.jobs == nil {
		return []AppJob{}
	}
	s.reconcileInactiveScriptJobs()
	records := s.jobs.list()
	jobs := make([]AppJob, 0, len(records))
	for _, record := range records {
		jobs = append(jobs, s.jobs.public(record))
	}
	return jobs
}

func (s *Service) CancelAppJob(id string) (AppJob, error) {
	s.actions.Lock()
	defer s.actions.Unlock()
	if s.jobs == nil || s.jobRunner == nil || !appJobIDPattern.MatchString(id) {
		return AppJob{}, ErrNotFound
	}
	s.reconcileInactiveScriptJobs()
	record, err := s.jobs.read(id)
	if err != nil {
		return AppJob{}, ErrNotFound
	}
	if !record.Interactive || record.Adapter != "kejilion" {
		return AppJob{}, fmt.Errorf(
			"%w: only active interactive kejilion.sh tasks can be ended manually",
			ErrForbidden,
		)
	}
	if record.Status == "cancelled" {
		return s.jobs.public(record), nil
	}
	if record.Status != "queued" && record.Status != "running" {
		return AppJob{}, fmt.Errorf("%w: application task is no longer active", ErrConflict)
	}
	if record.Stage == "cancelling" && s.jobs.cancelRequested(id) {
		return s.jobs.public(record), nil
	}

	original := record
	if err := s.jobs.requestCancel(id); err != nil {
		return AppJob{}, fmt.Errorf("%w: persist cancellation request: %v", ErrNeedsAttention, err)
	}
	record.Stage = "cancelling"
	record.Message = "正在结束 kejilion.sh 交互任务，请等待后台进程安全退出"
	record.InputOpen = false
	if err := s.jobs.put(record); err != nil {
		_ = os.Remove(s.jobs.cancelPath(id))
		return AppJob{}, fmt.Errorf("%w: persist cancellation state: %v", ErrNeedsAttention, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := s.jobRunner.Run(
		ctx,
		"systemctl",
		"stop",
		"--no-block",
		appJobUnitPrefix+id+".service",
	); err != nil {
		_ = os.Remove(s.jobs.cancelPath(id))
		_ = s.jobs.put(original)
		return AppJob{}, fmt.Errorf("%w: stop interactive application task: %v", ErrNeedsAttention, err)
	}
	_ = removeTerminalInput(s.jobs.inputPath(id))
	return s.jobs.public(record), nil
}

func (s *Service) scriptSelector(item Summary) (string, bool) {
	if !s.scriptInstallAvailable() {
		return "", false
	}
	return s.scriptSelectorFor(item)
}

func (s *Service) scriptSelectorFor(item Summary) (string, bool) {
	if _, declarative := declarativeSpecs[item.Token]; declarative {
		return "", false
	}
	if item.Source == "thirdparty" {
		return item.Token, true
	}
	if item.Source == "builtin" && item.Num > 0 && item.Num <= maxRemoteCatalogApps {
		return strconv.Itoa(item.Num), true
	}
	return "", false
}

func (s *Service) scriptInstallAvailable() bool {
	if s.jobs == nil || s.jobRunner == nil || s.jobExecutable == "" ||
		s.scriptInteractiveFinder == nil {
		return false
	}
	if _, err := s.jobRunner.LookPath("systemd-run"); err != nil {
		return false
	}
	_, err := s.scriptInteractiveFinder()
	return err == nil
}

func (s *Service) scriptManageAvailable() bool {
	if s.jobs == nil || s.jobRunner == nil || s.jobExecutable == "" ||
		s.scriptInteractiveFinder == nil || s.scriptManageFinder == nil {
		return false
	}
	if _, err := s.jobRunner.LookPath("systemd-run"); err != nil {
		return false
	}
	if _, err := s.scriptInteractiveFinder(); err != nil {
		return false
	}
	_, err := s.scriptManageFinder()
	return err == nil
}

func (s *Service) scriptInteractiveManageAvailable() bool {
	if s.jobs == nil || s.jobRunner == nil || s.jobExecutable == "" ||
		s.scriptInteractiveManageFinder == nil {
		return false
	}
	if _, err := s.jobRunner.LookPath("systemd-run"); err != nil {
		return false
	}
	_, err := s.scriptInteractiveManageFinder()
	return err == nil
}

func findKejilionScript() (string, error) {
	return findKejilionScriptMatching(isKPanelCompatibleScript)
}

func findKejilionManageScript() (string, error) {
	return findKejilionScriptMatching(isKPanelManageCompatibleScript)
}

func findKejilionInteractiveScript() (string, error) {
	return findKejilionScriptMatching(isKPanelInteractiveCompatibleScript)
}

func findKejilionInteractiveManageScript() (string, error) {
	return findKejilionScriptMatching(isKPanelInteractiveManageCompatibleScript)
}

func findKejilionScriptMatching(compatible func([]byte) bool) (string, error) {
	candidates := preferredKejilionScriptCandidates()
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
		if runtime.GOOS == "linux" &&
			(!trustedFileOwner(info) || info.Mode().Perm()&0o022 != 0) {
			continue
		}
		content, err := os.ReadFile(resolved)
		if err != nil || !compatible(content) {
			continue
		}
		return resolved, nil
	}
	return "", errors.New("a KPanel-compatible kejilion.sh was not found")
}

func preferredKejilionScriptCandidates() []string {
	return []string{
		"/home/docker/kpanel/bin/kejilion.sh",
		"/usr/local/bin/k",
		"/usr/bin/k",
		"/root/kejilion.sh",
	}
}

func appScriptCompatible(
	compatible func([]byte) bool,
	appID, selector string,
) func([]byte) bool {
	return func(content []byte) bool {
		if !compatible(content) {
			return false
		}
		if !strings.HasPrefix(appID, "builtin-") {
			return true
		}
		if appID != "builtin-"+selector {
			return false
		}
		return scriptSupportsBuiltinSelector(content, selector)
	}
}

func scriptSupportsBuiltinSelector(content []byte, selector string) bool {
	if selector == "" {
		return false
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == selector+")" ||
			(strings.HasPrefix(line, selector+"|") && strings.HasSuffix(line, ")")) {
			return true
		}
	}
	return false
}

func isKPanelCompatibleScript(content []byte) bool {
	value := string(content)
	return strings.Contains(value, "KJ_APP_NONINTERACTIVE") &&
		strings.Contains(value, "kpanel_run_docker_app_install") &&
		appLicensePattern.MatchString(value)
}

func isKPanelManageCompatibleScript(content []byte) bool {
	value := string(content)
	return isKPanelCompatibleScript(content) &&
		strings.Contains(value, "kpanel_run_docker_app_action") &&
		strings.Contains(value, "KJ_APP_EXPECTED_CONTAINER_ID") &&
		strings.Contains(value, "KJ_APP_RECONCILE_MARKER")
}

func isKPanelInteractiveCompatibleScript(content []byte) bool {
	value := string(content)
	return isKPanelCompatibleScript(content) &&
		strings.Contains(value, "KJ_APP_INTERACTIVE") &&
		strings.Contains(value, "kpanel_app_interactive_choice")
}

func isKPanelInteractiveManageCompatibleScript(content []byte) bool {
	value := string(content)
	return isKPanelInteractiveCompatibleScript(content) &&
		strings.Contains(value, "kpanel_app_interactive_manage_choice") &&
		strings.Contains(value, "KJ_APP_MARKER_RECOVERY")
}

func RunAppJob(ctx context.Context, stateDir, id string) error {
	if os.Geteuid() != 0 {
		return errors.New("app-run requires root")
	}
	if !appJobIDPattern.MatchString(id) {
		return errors.New("invalid application job identity")
	}
	registry := &appJobRegistry{stateDir: filepath.Clean(stateDir), jobs: make(map[string]appJobRecord)}
	if err := ensureAppJobDirectory(registry.stateDir); err != nil {
		return err
	}
	record, err := registry.read(id)
	if err != nil {
		return err
	}
	if record.Adapter != "kejilion" || record.Action == "manage" ||
		!supportedScriptJobAction(record.Action) ||
		!appSelectorPattern.MatchString(record.Selector) {
		return errors.New("application job contains an unsupported adapter request")
	}
	if record.Action != "install" && !containerIDPattern.MatchString(record.ExpectedContainerID) {
		return errors.New("application job contains an invalid expected container")
	}
	if record.Action == "direct_access" &&
		record.AccessMode != "direct" && record.AccessMode != "domain_only" {
		return errors.New("application job contains an invalid access policy")
	}
	scriptCompatible := isKPanelCompatibleScript
	if record.Action != "install" {
		scriptCompatible = isKPanelManageCompatibleScript
	}
	script, err := findKejilionScriptMatching(
		appScriptCompatible(scriptCompatible, record.AppID, record.Selector),
	)
	if err != nil {
		return registry.fail(record, "script_unavailable", err)
	}

	started := time.Now().UTC()
	record.Status = "running"
	record.Stage = "starting"
	record.Progress = 1
	record.Message = "正在启动 kejilion.sh 原生" + appActionLabel(record.Action) + "流程"
	record.StartedAt = &started
	record.FinishedAt = nil
	if err := registry.put(record); err != nil {
		return err
	}

	command := exec.CommandContext(ctx, "/bin/bash", script, "app", record.Selector)
	command.Env = append(
		os.Environ(),
		"KJ_APP_NONINTERACTIVE=1",
		"KJ_APP_ACTION="+record.Action,
		"LC_ALL=C.UTF-8",
		"LANG=C.UTF-8",
	)
	if record.HostPort != 0 {
		command.Env = append(command.Env, "KJ_APP_PORT="+strconv.Itoa(int(record.HostPort)))
	}
	if record.AccessMode != "" {
		command.Env = append(command.Env, "KJ_APP_ACCESS_MODE="+record.AccessMode)
	}
	if record.ExpectedContainerID != "" {
		command.Env = append(command.Env, "KJ_APP_EXPECTED_CONTAINER_ID="+record.ExpectedContainerID)
	}
	reader, writer := io.Pipe()
	command.Stdout = writer
	command.Stderr = writer
	if err := command.Start(); err != nil {
		_ = writer.Close()
		_ = reader.Close()
		return registry.fail(record, "start_failed", err)
	}
	wait := make(chan error, 1)
	go func() {
		wait <- command.Wait()
		_ = writer.Close()
	}()

	logFile, logErr := os.OpenFile(
		registry.logPath(id),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC,
		0o600,
	)
	if logErr != nil {
		_ = command.Process.Kill()
		_ = reader.Close()
		<-wait
		return registry.fail(record, "log_unavailable", logErr)
	}
	written := int64(0)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if written < maxAppJobLog {
			payload := []byte(line + "\n")
			remaining := int64(maxAppJobLog) - written
			if int64(len(payload)) > remaining {
				payload = payload[:remaining]
			}
			count, _ := logFile.Write(payload)
			written += int64(count)
		}
		if matches := appProgressPattern.FindStringSubmatch(line); len(matches) == 3 {
			progress, _ := strconv.Atoi(matches[1])
			if progress < 0 {
				progress = 0
			}
			if progress > 100 {
				progress = 100
			}
			record.Progress = progress
			record.Stage = appJobStage(progress)
			record.Message = safeAppJobMessage(errors.New(matches[2]))
			_ = registry.put(record)
		}
	}
	_ = reader.Close()
	_ = logFile.Sync()
	_ = logFile.Close()
	commandErr := <-wait
	if scanErr := scanner.Err(); scanErr != nil && commandErr == nil {
		commandErr = scanErr
	}
	finished := time.Now().UTC()
	record.Progress = 100
	record.FinishedAt = &finished
	if commandErr != nil {
		record.Status = "failed"
		record.Stage = "failed"
		record.Message = appActionLabel(record.Action) + "失败，请查看任务日志后修复并重试"
		_ = registry.put(record)
		return commandErr
	}
	record.Status = "succeeded"
	record.Stage = "completed"
	record.Message = "应用" + appActionLabel(record.Action) + "完成，产物已由 kejilion.sh 原生业务函数对账"
	return registry.put(record)
}

func supportedScriptJobAction(action string) bool {
	return action == "install" || action == "update" ||
		action == "uninstall" || action == "direct_access" ||
		action == "manage"
}

func appActionLabel(action string) string {
	switch action {
	case "install":
		return "安装"
	case "update":
		return "更新"
	case "uninstall":
		return "卸载"
	case "direct_access":
		return "访问策略变更"
	case "manage":
		return "脚本管理"
	default:
		return "应用操作"
	}
}

func appJobStage(progress int) string {
	switch {
	case progress < 15:
		return "preflight"
	case progress < 30:
		return "runtime"
	case progress < 90:
		return "executing"
	case progress < 100:
		return "reconciling"
	default:
		return "completed"
	}
}

func safeAppJobMessage(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 400 {
		value = value[:400]
	}
	if value == "" {
		return "应用后台任务失败"
	}
	return value
}

func (registry *appJobRegistry) fail(record appJobRecord, stage string, cause error) error {
	finished := time.Now().UTC()
	record.Status = "failed"
	record.Stage = stage
	record.Progress = 100
	record.Message = safeAppJobMessage(cause)
	record.FinishedAt = &finished
	_ = registry.put(record)
	return cause
}

func (registry *appJobRegistry) put(record appJobRecord) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if !appJobIDPattern.MatchString(record.ID) {
		return errors.New("invalid application job identity")
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxAppJobBytes {
		return errors.New("application job state exceeds the safety limit")
	}
	temp, err := os.CreateTemp(registry.stateDir, "."+record.ID+".*.tmp")
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
	targetPath := registry.statePath(record.ID)
	// KPanel runs on Linux, where rename atomically replaces the old state file.
	// Windows does not allow that replacement, so remove only the test/dev copy.
	if runtime.GOOS == "windows" {
		_ = os.Remove(targetPath)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return err
	}
	registry.jobs[record.ID] = record
	registry.pruneLocked()
	return nil
}

func (registry *appJobRegistry) read(id string) (appJobRecord, error) {
	if !appJobIDPattern.MatchString(id) {
		return appJobRecord{}, os.ErrNotExist
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	data, err := os.ReadFile(registry.statePath(id))
	if err != nil || len(data) > maxAppJobBytes {
		return appJobRecord{}, os.ErrNotExist
	}
	var record appJobRecord
	if json.Unmarshal(data, &record) != nil || record.ID != id {
		return appJobRecord{}, os.ErrNotExist
	}
	registry.jobs[id] = record
	return record, nil
}

func (registry *appJobRegistry) list() []appJobRecord {
	registry.mu.Lock()
	ids := make([]string, 0, len(registry.jobs))
	for id := range registry.jobs {
		ids = append(ids, id)
	}
	registry.mu.Unlock()
	records := make([]appJobRecord, 0, len(ids))
	for _, id := range ids {
		if record, err := registry.read(id); err == nil {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records
}

func (registry *appJobRegistry) hasActive() bool {
	for _, record := range registry.list() {
		if record.Status == "queued" || record.Status == "running" {
			return true
		}
	}
	return false
}

func (registry *appJobRegistry) public(record appJobRecord) AppJob {
	job := record.AppJob
	job.Logs = registry.logTail(record.ID, 120)
	return job
}

func (registry *appJobRegistry) logTail(id string, maxLines int) []string {
	data, err := os.ReadFile(registry.logPath(id))
	if err != nil {
		return []string{}
	}
	if len(data) > maxAppJobLog {
		data = data[len(data)-maxAppJobLog:]
	}
	lines := strings.Split(strings.TrimRight(string(data), "\r\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return []string{}
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}

func (registry *appJobRegistry) pruneLocked() {
	if len(registry.jobs) <= 100 {
		return
	}
	terminal := make([]appJobRecord, 0, len(registry.jobs))
	for _, record := range registry.jobs {
		if record.Status != "queued" && record.Status != "running" {
			terminal = append(terminal, record)
		}
	}
	sort.Slice(terminal, func(i, j int) bool {
		return terminal[i].CreatedAt.Before(terminal[j].CreatedAt)
	})
	removeCount := len(registry.jobs) - 100
	if removeCount > len(terminal) {
		removeCount = len(terminal)
	}
	for _, record := range terminal[:removeCount] {
		delete(registry.jobs, record.ID)
		_ = os.Remove(registry.statePath(record.ID))
		_ = os.Remove(registry.logPath(record.ID))
		_ = removeTerminalInput(registry.inputPath(record.ID))
		_ = os.Remove(registry.cancelPath(record.ID))
	}
}

func (registry *appJobRegistry) statePath(id string) string {
	return filepath.Join(registry.stateDir, id+".json")
}

func (registry *appJobRegistry) logPath(id string) string {
	return filepath.Join(registry.stateDir, id+".log")
}

func (registry *appJobRegistry) cancelPath(id string) string {
	return filepath.Join(registry.stateDir, id+".cancel")
}

func (registry *appJobRegistry) requestCancel(id string) error {
	path := registry.cancelPath(id)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		if registry.cancelRequested(id) {
			return nil
		}
		return errors.New("unsafe application cancellation marker")
	}
	if err != nil {
		return err
	}
	if _, err := file.WriteString("cancel\n"); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

func (registry *appJobRegistry) cancelRequested(id string) bool {
	info, err := os.Lstat(registry.cancelPath(id))
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}
