package systemmanage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const maintenanceUnitPrefix = "kejilion-panel-maintenance-"

const (
	maintenanceLaunchGrace   = 15 * time.Second
	maintenanceLaunchTimeout = 2 * time.Minute
)

type maintenanceStep struct {
	stage     string
	progress  int
	command   string
	arguments []string
	operation string
	optional  bool
}

const (
	maintenanceOperationPacmanOrphans = "pacman-orphans"
	maintenanceOperationBBRv3         = "bbrv3"
	maintenanceOperationSystemTuning  = "system-tuning"
)

var pacmanPackagePattern = regexp.MustCompile(`^[a-z0-9@._+][a-z0-9@._+-]{0,127}$`)
var maintenanceIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,96}$`)

func (m *Manager) MaintenanceStatus() contract.SystemMaintenanceSummary {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := m.readMaintenance()
	m.reconcileMaintenanceLaunch(&status)
	limit := time.Hour
	if status.Action == "system-tuning" {
		limit = 2 * time.Hour
	}
	if status.State == "running" && status.StartedAt != nil && m.now().Sub(*status.StartedAt) > limit {
		finishedAt := m.now().UTC()
		status.State = "failed"
		status.Stage = "interrupted"
		status.Progress = 100
		status.Message = "维护任务超过安全时限，需检查软件包管理器状态"
		status.FinishedAt = &finishedAt
		_ = m.writeMaintenance(status)
	}
	return status
}

func (m *Manager) reconcileMaintenanceLaunch(status *contract.SystemMaintenanceSummary) {
	if status.State != "running" || status.StartedAt == nil ||
		(status.Stage != "queued" && status.Stage != "launching") ||
		!maintenanceIDPattern.MatchString(status.ID) {
		return
	}
	elapsed := m.now().Sub(*status.StartedAt)
	if elapsed < maintenanceLaunchGrace {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	output, err := m.runner.Run(
		ctx,
		"systemctl",
		"show",
		maintenanceUnitPrefix+status.ID,
		"--property=LoadState",
		"--property=ActiveState",
		"--property=SubState",
		"--property=Result",
		"--property=ExecMainStatus",
		"--no-pager",
	)
	unit := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok {
			unit[key] = value
		}
	}
	active := unit["ActiveState"]
	if err == nil && (active == "active" || active == "activating") {
		return
	}
	if err != nil && elapsed < maintenanceLaunchTimeout {
		return
	}
	if err == nil && active == "" && strings.TrimSpace(unit["LoadState"]) == "" &&
		elapsed < maintenanceLaunchTimeout {
		return
	}

	// The worker writes the same atomic state file from another process. It may
	// finish after this request read the launch snapshot but before systemctl
	// returns. Never overwrite that newer worker receipt with a reconciliation
	// result derived from the stale snapshot.
	latest := m.readMaintenance()
	if latest.ID != status.ID ||
		latest.State != status.State ||
		latest.Stage != status.Stage ||
		latest.Progress != status.Progress {
		*status = latest
		return
	}

	details := make([]string, 0, 5)
	for _, key := range []string{"LoadState", "ActiveState", "SubState", "Result", "ExecMainStatus"} {
		if value := strings.TrimSpace(unit[key]); value != "" {
			details = append(details, key+"="+value)
		}
	}
	detail := strings.Join(details, " / ")
	if detail == "" && err != nil {
		detail = maintenanceErrorMessage(err)
	}
	if detail == "" {
		detail = "systemd 未返回执行状态"
	}
	finishedAt := m.now().UTC()
	status.State = "failed"
	if err == nil &&
		strings.EqualFold(strings.TrimSpace(unit["Result"]), "success") &&
		strings.TrimSpace(unit["ExecMainStatus"]) == "0" {
		status.Stage = "completion_unverified"
		status.Message = "后台维护进程已退出，但未写入任务完成凭据；不能判定为成功：" + detail
	} else {
		status.Stage = "launch_failed"
		status.Message = "后台维护进程未成功启动：" + detail
	}
	status.Progress = 100
	status.FinishedAt = &finishedAt
	_ = m.writeMaintenance(*status)
}

func (m *Manager) startMaintenance(
	ctx context.Context,
	action string,
	policy string,
) (bool, string, error) {
	mode := ""
	switch action {
	case "update":
		if policy != "full" {
			return false, "", fmt.Errorf("%w: update policy must be full", ErrInvalidInput)
		}
		mode = "update"
	case "cleanup":
		if policy != "cache" && policy != "standard" {
			return false, "", fmt.Errorf("%w: cleanup policy must be cache or standard", ErrInvalidInput)
		}
		mode = "cleanup-" + policy
	case "ssh-defense":
		if policy != "enable" && policy != "disable" && policy != "uninstall" {
			return false, "", fmt.Errorf("%w: SSH defense policy must be enable, disable, or uninstall", ErrInvalidInput)
		}
		mode = "ssh-defense-" + policy
	case "bbrv3":
		if policy != "install" && policy != "update" && policy != "uninstall" {
			return false, "", fmt.Errorf("%w: BBRv3 policy must be install, update, or uninstall", ErrInvalidInput)
		}
		mode = "bbrv3-" + policy
	case "system-tuning":
		if _, _, ok := parseSystemTuningMaintenancePolicy(policy); !ok {
			return false, "", fmt.Errorf("%w: system tuning policy is invalid", ErrInvalidInput)
		}
		mode = "system-tuning-" + policy
	default:
		return false, "", fmt.Errorf("%w: unknown maintenance action", ErrInvalidInput)
	}

	current := m.readMaintenance()
	if current.State == "running" {
		return false, "", fmt.Errorf("%w: another maintenance task is already running", ErrConflict)
	}
	if _, _, _, err := m.maintenanceSteps(mode); err != nil {
		return false, "", err
	}
	executable, err := m.backgroundExecutable()
	if err != nil {
		return false, "", err
	}

	startedAt := m.now().UTC()
	status := contract.SystemMaintenanceSummary{
		ID:    idForMaintenance(startedAt),
		State: "running", Action: action, Policy: policy,
		Stage: "launching", Progress: 2, Message: "正在启动 systemd 后台维护任务",
		StartedAt: &startedAt,
	}
	if err := m.writeMaintenance(status); err != nil {
		return false, "", fmt.Errorf("%w: persist maintenance state: %v", ErrUnsupported, err)
	}

	timeoutStart := "45min"
	if action == "system-tuning" {
		timeoutStart = "90min"
	}
	arguments := []string{
		"--unit=" + maintenanceUnitPrefix + status.ID,
		"--collect",
		"--no-block",
		"--property=Type=oneshot",
		"--property=TimeoutStartSec=" + timeoutStart,
		"--property=TimeoutStopSec=5min",
		"--property=User=root",
		"--property=UMask=0027",
		"--property=PrivateTmp=yes",
		"--property=ProtectHome=read-only",
		"--property=ReadWritePaths=" + m.stateDir,
		"--property=NoNewPrivileges=no",
		"--property=RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6",
		"--property=Nice=10",
		"--property=CPUWeight=20",
		"--property=IOWeight=20",
		"--property=SyslogIdentifier=kpanel-maintenance",
		"--",
		executable,
		"maintenance-run",
		"--state-dir",
		m.stateDir,
		mode,
	}
	if _, err := m.runner.Run(ctx, "systemd-run", arguments...); err != nil {
		finishedAt := m.now().UTC()
		status.State = "failed"
		status.Stage = "launch_failed"
		status.Progress = 100
		status.Message = maintenanceErrorMessage(err)
		status.FinishedAt = &finishedAt
		_ = m.writeMaintenance(status)
		return false, "", fmt.Errorf("%w: start maintenance task: %v", ErrUnsupported, err)
	}
	return true, "系统维护任务已提交，页面将自动刷新进度", nil
}

// RunMaintenance is only called by the Agent's root-only maintenance-run
// subcommand. The mode selects one fixed command list; it never accepts shell
// fragments, package names, paths, or arbitrary arguments from the Web API.
func (m *Manager) RunMaintenance(ctx context.Context, mode string) error {
	action, policy, steps, err := m.maintenanceSteps(mode)
	if err != nil {
		return err
	}
	status := m.readMaintenance()
	if status.ID == "" || status.Action != action || status.Policy != policy {
		startedAt := m.now().UTC()
		status = contract.SystemMaintenanceSummary{
			ID:    idForMaintenance(startedAt),
			State: "running", Action: action, Policy: policy,
			StartedAt: &startedAt,
		}
	}
	status.State = "running"
	status.Stage = "starting"
	status.Progress = 5
	status.Message = "正在准备系统维护任务"
	status.FinishedAt = nil
	if err := m.writeMaintenance(status); err != nil {
		return fmt.Errorf("persist maintenance start: %w", err)
	}

	bbrv3RebootRequired := false
	for _, step := range steps {
		status.Stage = step.stage
		status.Progress = step.progress
		status.Message = maintenanceStageMessage(step.stage)
		if err := m.writeMaintenance(status); err != nil {
			return fmt.Errorf("persist maintenance progress: %w", err)
		}
		var runErr error
		if step.operation == maintenanceOperationPacmanOrphans {
			runErr = m.removePacmanOrphans(ctx, step.command)
		} else if step.operation == maintenanceOperationBBRv3 {
			var output []byte
			output, runErr = m.runner.Run(ctx, step.command, step.arguments...)
			if runErr == nil {
				bbrv3RebootRequired, runErr = verifyBBRv3ActionOutput(output, policy)
			}
		} else if step.operation == maintenanceOperationSystemTuning {
			_, _, ok := parseSystemTuningMaintenancePolicy(policy)
			if !ok {
				runErr = errors.New("system tuning policy became invalid")
			} else {
				expected := strings.TrimPrefix(step.stage, "system_tuning_")
				var output []byte
				output, runErr = m.runSystemTuning(ctx, "apply-item", expected)
				var receiptStatus, selected string
				_, receiptStatus, selected, parseErr := parseSystemTuningOutput(output)
				if parseErr != nil {
					if runErr != nil {
						runErr = fmt.Errorf("%v; invalid system tuning receipt: %w", runErr, parseErr)
					} else {
						runErr = parseErr
					}
				} else if selected != expected {
					runErr = errors.New("system tuning completion receipt did not match the selected item")
				} else if receiptStatus != "applied" && receiptStatus != "unchanged" {
					if runErr != nil {
						runErr = fmt.Errorf("kejilion.sh returned system tuning status %q: %w", receiptStatus, runErr)
					} else {
						runErr = fmt.Errorf("kejilion.sh returned system tuning status %q", receiptStatus)
					}
				}
			}
		} else {
			_, runErr = m.runner.Run(ctx, step.command, step.arguments...)
		}
		if runErr != nil {
			finishedAt := m.now().UTC()
			status.State = "failed"
			status.Progress = 100
			status.Message = maintenanceErrorMessage(runErr)
			status.FinishedAt = &finishedAt
			_ = m.writeMaintenance(status)
			return fmt.Errorf("%s: %w", step.stage, runErr)
		}
	}

	finishedAt := m.now().UTC()
	status.State = "succeeded"
	status.Stage = "completed"
	status.Progress = 100
	status.RebootRequired = bbrv3RebootRequired ||
		regularFile(filepath.Join(m.runRoot, "reboot-required"))
	status.Message = maintenanceCompletionMessage(
		action,
		policy,
		len(steps),
		finishedAt.Sub(*status.StartedAt),
		status.RebootRequired,
	)
	status.FinishedAt = &finishedAt
	if err := m.writeMaintenance(status); err != nil {
		return fmt.Errorf("persist maintenance result: %w", err)
	}
	return nil
}

func (m *Manager) maintenanceSteps(
	mode string,
) (string, string, []maintenanceStep, error) {
	if strings.HasPrefix(mode, "system-tuning-") {
		policy := strings.TrimPrefix(mode, "system-tuning-")
		items, _, ok := parseSystemTuningMaintenancePolicy(policy)
		if !ok {
			return "", "", nil, fmt.Errorf("%w: unknown system tuning policy", ErrInvalidInput)
		}
		script, err := m.systemTuningScriptPath()
		if err != nil {
			return "", "", nil, fmt.Errorf("%w: update kejilion.sh to enable one-click system tuning", ErrUnsupported)
		}
		for _, command := range []string{"env", "bash"} {
			if _, err := m.runner.LookPath(command); err != nil {
				return "", "", nil, fmt.Errorf("%w: system tuning command %s is unavailable", ErrUnsupported, command)
			}
		}
		steps := make([]maintenanceStep, 0, len(items))
		for index, item := range items {
			steps = append(steps, maintenanceStep{
				stage: "system_tuning_" + item, progress: 8 + index*86/len(items), command: "env", operation: maintenanceOperationSystemTuning,
				arguments: []string{
					"KJ_SYSTEM_TUNING_NONINTERACTIVE=1", "LC_ALL=C.UTF-8", "LANG=C.UTF-8", "bash", script,
					"kpanel", "system-tuning", "apply-item", item,
				},
			})
		}
		return "system-tuning", policy, steps, nil
	}
	if mode == "ssh-defense-enable" || mode == "ssh-defense-disable" || mode == "ssh-defense-uninstall" {
		script, err := m.sshDefenseManagerScriptPath()
		if err != nil {
			return "", "", nil, fmt.Errorf(
				"%w: update kejilion.sh to enable the SSH defense protocol",
				ErrUnsupported,
			)
		}
		for _, command := range []string{"env", "bash"} {
			if _, err := m.runner.LookPath(command); err != nil {
				return "", "", nil, fmt.Errorf(
					"%w: SSH defense command %s is unavailable",
					ErrUnsupported,
					command,
				)
			}
		}
		policy := strings.TrimPrefix(mode, "ssh-defense-")
		return "ssh-defense", policy, []maintenanceStep{{
			stage:    "ssh_defense_" + policy,
			progress: 45,
			command:  "env",
			arguments: []string{
				"KJ_F2B_NONINTERACTIVE=1",
				"LC_ALL=C.UTF-8",
				"LANG=C.UTF-8",
				"bash",
				script,
				"f2b",
				"manager",
				policy,
			},
		}}, nil
	}
	if strings.HasPrefix(mode, "bbrv3-") {
		script, err := m.bbrv3Script()
		if err != nil {
			return "", "", nil, fmt.Errorf(
				"%w: update kejilion.sh to enable the BBRv3 protocol",
				ErrUnsupported,
			)
		}
		for _, command := range []string{"env", "bash"} {
			if _, err := m.runner.LookPath(command); err != nil {
				return "", "", nil, fmt.Errorf(
					"%w: BBRv3 command %s is unavailable",
					ErrUnsupported,
					command,
				)
			}
		}
		policy := strings.TrimPrefix(mode, "bbrv3-")
		switch policy {
		case "install", "update", "uninstall":
		default:
			return "", "", nil, fmt.Errorf("%w: unknown BBRv3 policy", ErrInvalidInput)
		}
		return "bbrv3", policy, []maintenanceStep{{
			stage:     "bbrv3_" + policy,
			progress:  35,
			command:   "env",
			operation: maintenanceOperationBBRv3,
			arguments: []string{
				"KJ_BBRV3_NONINTERACTIVE=1",
				"LC_ALL=C.UTF-8",
				"LANG=C.UTF-8",
				"bash",
				script,
				"bbrv3",
				policy,
			},
		}}, nil
	}
	support := m.detectPackageManager()
	if !support.available() {
		reason := support.reason
		if reason == "" {
			reason = "当前发行版没有可用的系统维护适配器"
		}
		return "", "", nil, fmt.Errorf("%w: %s", ErrUnsupported, reason)
	}
	aptOptions := []string{"-o", "Dpkg::Lock::Timeout=120"}
	var action, policy string
	var steps []maintenanceStep
	switch support.kind {
	case packageManagerAPT:
		action, policy, steps = aptMaintenanceSteps(mode, aptOptions)
	case packageManagerDNF:
		action, policy, steps = rpmMaintenanceSteps(mode, support.command)
	case packageManagerYUM:
		action, policy, steps = rpmMaintenanceSteps(mode, support.command)
	case packageManagerAPK:
		action, policy, steps = apkMaintenanceSteps(mode, support.command)
	case packageManagerPacman:
		action, policy, steps = pacmanMaintenanceSteps(mode)
	case packageManagerZypper:
		action, policy, steps = zypperMaintenanceSteps(mode)
	}
	if action == "" {
		return "", "", nil, fmt.Errorf("%w: unknown maintenance mode", ErrInvalidInput)
	}
	availableSteps := make([]maintenanceStep, 0, len(steps))
	for _, step := range steps {
		if _, err := m.runner.LookPath(step.command); err != nil {
			if step.optional {
				continue
			}
			return "", "", nil, fmt.Errorf(
				"%w: %s command %s is unavailable",
				ErrUnsupported,
				support.displayName(),
				step.command,
			)
		}
		availableSteps = append(availableSteps, step)
	}
	return action, policy, availableSteps, nil
}

func aptMaintenanceSteps(mode string, aptOptions []string) (string, string, []maintenanceStep) {
	switch mode {
	case "update":
		return "update", "full", []maintenanceStep{
			{stage: "dpkg_configure", progress: 10, command: "dpkg", arguments: []string{"--force-confold", "--configure", "-a"}},
			{stage: "package_index", progress: 35, command: "apt-get", arguments: append(slicesClone(aptOptions), "update")},
			{
				stage: "full_upgrade", progress: 60, command: "apt-get",
				arguments: append(slicesClone(aptOptions), "-y", "-o", "Dpkg::Options::=--force-confold", "full-upgrade"),
			},
		}
	case "cleanup-cache":
		return "cleanup", "cache", []maintenanceStep{
			{stage: "package_cache", progress: 35, command: "apt-get", arguments: append(slicesClone(aptOptions), "clean")},
			{stage: "obsolete_cache", progress: 70, command: "apt-get", arguments: append(slicesClone(aptOptions), "autoclean")},
		}
	case "cleanup-standard":
		return "cleanup", "standard", append([]maintenanceStep{
			{
				stage: "unused_packages", progress: 15, command: "apt-get",
				arguments: append(slicesClone(aptOptions), "-y", "autoremove", "--purge"),
			},
			{stage: "package_cache", progress: 40, command: "apt-get", arguments: append(slicesClone(aptOptions), "clean")},
			{stage: "obsolete_cache", progress: 55, command: "apt-get", arguments: append(slicesClone(aptOptions), "autoclean")},
		}, journalCleanupSteps()...)
	default:
		return "", "", nil
	}
}

func rpmMaintenanceSteps(mode, command string) (string, string, []maintenanceStep) {
	switch mode {
	case "update":
		return "update", "full", []maintenanceStep{
			{stage: "full_upgrade", progress: 40, command: command, arguments: []string{"-y", "update"}},
		}
	case "cleanup-cache":
		return "cleanup", "cache", []maintenanceStep{
			{stage: "package_cache", progress: 50, command: command, arguments: []string{"clean", "all"}},
		}
	case "cleanup-standard":
		return "cleanup", "standard", append([]maintenanceStep{
			{stage: "unused_packages", progress: 15, command: command, arguments: []string{"-y", "autoremove"}},
			{stage: "package_cache", progress: 40, command: command, arguments: []string{"clean", "all"}},
			{stage: "package_index", progress: 60, command: command, arguments: []string{"-y", "makecache"}},
		}, journalCleanupSteps()...)
	default:
		return "", "", nil
	}
}

func apkMaintenanceSteps(mode, command string) (string, string, []maintenanceStep) {
	switch mode {
	case "update":
		return "update", "full", []maintenanceStep{
			{stage: "package_index", progress: 30, command: command, arguments: []string{"update"}},
			{stage: "full_upgrade", progress: 60, command: command, arguments: []string{"upgrade"}},
		}
	case "cleanup-cache":
		return "cleanup", "cache", []maintenanceStep{
			{stage: "package_cache", progress: 50, command: command, arguments: []string{"cache", "clean"}},
		}
	case "cleanup-standard":
		return "cleanup", "standard", []maintenanceStep{
			{stage: "package_cache", progress: 50, command: command, arguments: []string{"cache", "clean"}},
		}
	default:
		return "", "", nil
	}
}

func pacmanMaintenanceSteps(mode string) (string, string, []maintenanceStep) {
	switch mode {
	case "update":
		return "update", "full", []maintenanceStep{
			{stage: "full_upgrade", progress: 40, command: "pacman", arguments: []string{"-Syu", "--noconfirm"}},
		}
	case "cleanup-cache":
		return "cleanup", "cache", []maintenanceStep{
			{stage: "package_cache", progress: 50, command: "pacman", arguments: []string{"-Scc", "--noconfirm"}},
		}
	case "cleanup-standard":
		return "cleanup", "standard", append([]maintenanceStep{
			{
				stage: "unused_packages", progress: 15, command: "pacman",
				operation: maintenanceOperationPacmanOrphans,
			},
			{stage: "package_cache", progress: 50, command: "pacman", arguments: []string{"-Scc", "--noconfirm"}},
		}, journalCleanupSteps()...)
	default:
		return "", "", nil
	}
}

func (m *Manager) removePacmanOrphans(ctx context.Context, command string) error {
	output, err := m.runner.Run(ctx, command, "-Qdtq")
	if err != nil {
		return err
	}
	packages := strings.Fields(string(output))
	if len(packages) == 0 {
		return nil
	}
	if len(packages) > 4096 {
		return errors.New("Pacman orphan list exceeds the safe limit")
	}
	seen := make(map[string]bool, len(packages))
	arguments := []string{"-Rns", "--noconfirm", "--"}
	for _, name := range packages {
		if !pacmanPackagePattern.MatchString(name) || seen[name] {
			return fmt.Errorf("Pacman returned an invalid or duplicated package name %q", name)
		}
		seen[name] = true
		arguments = append(arguments, name)
	}
	_, err = m.runner.Run(ctx, command, arguments...)
	return err
}

func zypperMaintenanceSteps(mode string) (string, string, []maintenanceStep) {
	switch mode {
	case "update":
		return "update", "full", []maintenanceStep{
			{stage: "package_index", progress: 30, command: "zypper", arguments: []string{"--non-interactive", "refresh"}},
			{stage: "full_upgrade", progress: 60, command: "zypper", arguments: []string{"--non-interactive", "update"}},
		}
	case "cleanup-cache":
		return "cleanup", "cache", []maintenanceStep{
			{stage: "package_cache", progress: 50, command: "zypper", arguments: []string{"--non-interactive", "clean", "--all"}},
		}
	case "cleanup-standard":
		return "cleanup", "standard", append([]maintenanceStep{
			{stage: "package_cache", progress: 45, command: "zypper", arguments: []string{"--non-interactive", "clean", "--all"}},
			{stage: "package_index", progress: 60, command: "zypper", arguments: []string{"--non-interactive", "refresh"}},
		}, journalCleanupSteps()...)
	default:
		return "", "", nil
	}
}

func journalCleanupSteps() []maintenanceStep {
	return []maintenanceStep{
		{stage: "journal_rotate", progress: 70, command: "journalctl", arguments: []string{"--rotate"}, optional: true},
		{stage: "journal_time", progress: 80, command: "journalctl", arguments: []string{"--vacuum-time=7d"}, optional: true},
		{stage: "journal_size", progress: 90, command: "journalctl", arguments: []string{"--vacuum-size=500M"}, optional: true},
	}
}

func (m *Manager) maintenanceStatePath() string {
	return filepath.Join(m.stateDir, "maintenance-state.json")
}

func (m *Manager) readMaintenance() contract.SystemMaintenanceSummary {
	status := contract.SystemMaintenanceSummary{State: "idle"}
	data, err := os.ReadFile(m.maintenanceStatePath())
	if err != nil || len(data) > 64<<10 || json.Unmarshal(data, &status) != nil {
		return contract.SystemMaintenanceSummary{State: "idle"}
	}
	switch status.State {
	case "idle", "running", "succeeded", "failed":
	default:
		return contract.SystemMaintenanceSummary{State: "idle"}
	}
	return status
}

func (m *Manager) writeMaintenance(status contract.SystemMaintenanceSummary) error {
	if err := os.MkdirAll(m.stateDir, 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeAtomic(m.maintenanceStatePath(), data, 0o600)
}

func idForMaintenance(now time.Time) string {
	var nonce [4]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return now.Format("20060102T150405.000000000Z")
	}
	return now.Format("20060102T150405Z") + "-" + hex.EncodeToString(nonce[:])
}

func maintenanceStageMessage(stage string) string {
	if strings.HasPrefix(stage, "system_tuning_") {
		item := strings.TrimPrefix(stage, "system_tuning_")
		labels := map[string]string{
			"system-update": "优化更新源并更新系统", "system-cleanup": "清理系统垃圾",
			"swap-1g": "设置 1 GB 虚拟内存", "ssh-port-5522": "设置 SSH 端口为 5522",
			"ssh-defense": "开启 SSH 防御", "firewall-open-all": "开放所有端口",
			"bbr": "开启 BBR 加速", "timezone-shanghai": "设置上海时区",
			"dns-auto": "自动优化 DNS", "ipv4-preferred": "设置 IPv4 优先",
			"basic-tools": "安装基础工具", "kernel-auto": "自动网络参数优化",
		}
		if label := labels[item]; label != "" {
			return "正在执行：" + label
		}
		return "正在执行一条龙系统调优"
	}
	switch stage {
	case "dpkg_configure":
		return "正在完成未结束的软件包配置"
	case "package_index":
		return "正在刷新软件包索引"
	case "full_upgrade":
		return "正在更新系统软件包"
	case "unused_packages":
		return "正在移除不再使用的依赖"
	case "package_cache":
		return "正在清理软件包缓存"
	case "obsolete_cache":
		return "正在清理过期软件包缓存"
	case "journal_rotate":
		return "正在轮转 systemd journal"
	case "journal_time":
		return "正在保留最近 7 天 journal"
	case "journal_size":
		return "正在限制 journal 最大 500 MiB"
	case "ssh_defense_enable":
		return "正在安装并启用 Fail2Ban SSH 防御"
	case "ssh_defense_disable":
		return "正在停用 Fail2Ban SSH 防御并保留配置"
	case "ssh_defense_uninstall":
		return "正在卸载 Fail2Ban SSH 防御"
	case "bbrv3_install":
		return "正在安装 XanMod BBRv3 内核"
	case "bbrv3_update":
		return "正在更新 XanMod BBRv3 内核"
	case "bbrv3_uninstall":
		return "正在卸载 XanMod BBRv3 内核"
	default:
		return "正在执行系统维护"
	}
}

func maintenanceSuccessMessage(action, policy string, rebootRequired bool) string {
	if action == "ssh-defense" {
		if policy == "enable" {
			return "SSH 防御已启用，Fail2Ban SSH jail 已验证"
		}
		if policy == "uninstall" {
			return "SSH 防御已卸载"
		}
		return "SSH 防御已停用，Fail2Ban 配置仍保留"
	}
	if action == "bbrv3" {
		if !rebootRequired {
			return "BBRv3 操作已完成并通过内核状态复核，当前无需重启切换"
		}
		switch policy {
		case "install":
			return "BBRv3 内核已安装；请安排重启后复核生效状态"
		case "update":
			return "BBRv3 内核已更新；请安排重启后复核生效状态"
		default:
			return "BBRv3 内核已卸载；请安排重启切换回发行版内核"
		}
	}
	if action == "system-tuning" {
		return "所选一条龙系统调优项目已全部完成"
	}
	if action == "update" {
		return "系统更新已完成；如内核或核心组件变化，请按提示安排重启"
	}
	if policy == "cache" {
		return "软件包缓存清理已完成"
	}
	return "系统支持的无用依赖、软件包缓存和旧 journal 已安全清理"
}

func maintenanceCompletionMessage(
	action string,
	policy string,
	completedSteps int,
	elapsed time.Duration,
	rebootRequired bool,
) string {
	if elapsed < 0 {
		elapsed = 0
	}
	duration := "<1 秒"
	if elapsed >= time.Second {
		seconds := int(elapsed.Round(time.Second) / time.Second)
		duration = fmt.Sprintf("%d 秒", seconds)
		if seconds >= 60 {
			duration = fmt.Sprintf("%d 分 %d 秒", seconds/60, seconds%60)
		}
	}
	return fmt.Sprintf(
		"%s；已执行 %d 个固定维护步骤，耗时 %s",
		maintenanceSuccessMessage(action, policy, rebootRequired),
		completedSteps,
		duration,
	)
}

func maintenanceErrorMessage(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 240 {
		message = message[:240]
	}
	if message == "" {
		return "系统维护任务失败，请检查 Agent 日志"
	}
	return "任务失败：" + message
}

func slicesClone(values []string) []string {
	return append([]string(nil), values...)
}
