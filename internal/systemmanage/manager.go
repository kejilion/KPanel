package systemmanage

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

var (
	ErrDisabled       = errors.New("system write executor is disabled")
	ErrInvalidInput   = errors.New("invalid system action input")
	ErrUnsupported    = errors.New("system action is unsupported on this host")
	ErrConflict       = errors.New("system configuration changed or conflicts")
	ErrRolledBack     = errors.New("system action failed and was rolled back")
	ErrNeedsAttention = errors.New("system action needs manual attention")
	dnsScriptLicense  = regexp.MustCompile(`(?m)^permission_granted="true"\r?$`)
)

const (
	kpanelIPPreferenceMarker = "# KPanel managed IPv4 precedence"
	kpanelKernelMarker       = "# KPanel managed kernel profile"
	kpanelSwapMarker         = "# KPanel managed swap"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
	LookPath(string) (string, error)
}

type CountryResolver func(context.Context) (string, error)
type KejilionScriptFinder func() (string, error)

type commandRunner struct{}

func (commandRunner) Run(ctx context.Context, name string, arguments ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Env = append(
		os.Environ(),
		"LC_ALL=C",
		"LANG=C",
		"DEBIAN_FRONTEND=noninteractive",
		"NEEDRESTART_MODE=a",
		"APT_LISTCHANGES_FRONTEND=none",
	)
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

func (commandRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

type Config struct {
	Enabled      bool
	EtcRoot      string
	ProcRoot     string
	SysRoot      string
	RunRoot      string
	StateDir     string
	SwapPath     string
	Executable   string
	Now          func() time.Time
	Runner       Runner
	Country      CountryResolver
	EffectiveUID func() int
	DNSScript    KejilionScriptFinder
	F2BScript    KejilionScriptFinder
	BBRv3Script  KejilionScriptFinder
}

type Manager struct {
	enabled         bool
	etcRoot         string
	procRoot        string
	sysRoot         string
	runRoot         string
	stateDir        string
	swapPath        string
	executable      string
	now             func() time.Time
	runner          Runner
	country         CountryResolver
	effectiveUID    func() int
	dnsScript       KejilionScriptFinder
	f2bScript       KejilionScriptFinder
	bbrv3Script     KejilionScriptFinder
	rebootScheduled bool
	mu              sync.Mutex
}

func NewManager(config Config) *Manager {
	if config.EtcRoot == "" {
		config.EtcRoot = "/etc"
	}
	if config.ProcRoot == "" {
		config.ProcRoot = "/proc"
	}
	if config.SysRoot == "" {
		config.SysRoot = "/sys"
	}
	if config.RunRoot == "" {
		config.RunRoot = "/var/run"
	}
	if config.StateDir == "" {
		config.StateDir = "/var/lib/kejilion-panel/system"
	}
	if config.SwapPath == "" {
		config.SwapPath = "/swapfile"
	}
	if config.Executable == "" {
		config.Executable = "/usr/local/libexec/kejilion-agent"
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Runner == nil {
		config.Runner = commandRunner{}
	}
	if config.Country == nil {
		config.Country = resolvePublicCountry
	}
	if config.EffectiveUID == nil {
		config.EffectiveUID = os.Geteuid
	}
	if config.DNSScript == nil {
		config.DNSScript = findKejilionDNSScript
	}
	if config.F2BScript == nil {
		config.F2BScript = findKejilionF2BScript
	}
	if config.BBRv3Script == nil {
		config.BBRv3Script = findKejilionBBRv3Script
	}
	return &Manager{
		enabled: config.Enabled, etcRoot: filepath.Clean(config.EtcRoot),
		procRoot: filepath.Clean(config.ProcRoot), sysRoot: filepath.Clean(config.SysRoot),
		runRoot:  filepath.Clean(config.RunRoot),
		stateDir: filepath.Clean(config.StateDir), swapPath: filepath.Clean(config.SwapPath),
		executable: filepath.Clean(config.Executable),
		now:        config.Now, runner: config.Runner, country: config.Country,
		effectiveUID: config.EffectiveUID, dnsScript: config.DNSScript,
		f2bScript: config.F2BScript, bbrv3Script: config.BBRv3Script,
	}
}

func (m *Manager) backgroundExecutable() (string, error) {
	path := filepath.Clean(m.executable)
	if path == "." || !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: Agent executable path is unavailable", ErrUnsupported)
	}
	if _, err := m.runner.LookPath(path); err != nil {
		return "", fmt.Errorf(
			"%w: Agent executable does not exist or is not executable at %s",
			ErrUnsupported,
			path,
		)
	}
	return path, nil
}

func capabilityReason(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	return strings.TrimPrefix(value, ErrUnsupported.Error()+": ")
}

func (m *Manager) Capabilities() []contract.Capability {
	disabledReason := ""
	if !m.enabled {
		disabledReason = "宿主机系统写入开关未启用"
	} else if runtime.GOOS != "linux" {
		disabledReason = "系统写入仅支持 Linux"
	} else if m.effectiveUID() != 0 {
		disabledReason = "Agent 必须以受限 root 服务运行"
	}
	capability := func(id string, supported bool, reason string) contract.Capability {
		if disabledReason != "" {
			return contract.Capability{ID: id, Enabled: false, Reason: disabledReason}
		}
		if !supported {
			return contract.Capability{ID: id, Enabled: false, Reason: reason}
		}
		return contract.Capability{ID: id, Enabled: true, Methods: []string{"POST"}}
	}
	_, hostnamectlErr := m.runner.LookPath("hostnamectl")
	_, timedatectlErr := m.runner.LookPath("timedatectl")
	_, systemctlErr := m.runner.LookPath("systemctl")
	_, mkswapErr := m.runner.LookPath("mkswap")
	_, swaponErr := m.runner.LookPath("swapon")
	_, swapoffErr := m.runner.LookPath("swapoff")
	_, fallocateErr := m.runner.LookPath("fallocate")
	_, sysctlErr := m.runner.LookPath("sysctl")
	_, systemdRunErr := m.runner.LookPath("systemd-run")
	_, modprobeErr := m.runner.LookPath("modprobe")
	_, helperErr := m.backgroundExecutable()
	_, envErr := m.runner.LookPath("env")
	_, bashErr := m.runner.LookPath("bash")
	_, chattrErr := m.runner.LookPath("chattr")
	_, dnsScriptErr := m.dnsScript()
	sshScriptErr := dnsScriptErr
	if sshScriptErr == nil {
		script, _ := m.dnsScript()
		content, err := os.ReadFile(script)
		if err != nil || !trustedKejilionSSHPortContent(content) {
			sshScriptErr = errors.New("trusted kejilion.sh SSH port protocol was not found")
		}
	}
	_, f2bScriptErr := m.f2bScript()
	_, bbrv3ScriptErr := m.bbrv3Script()

	sshConfig := regularFile(filepath.Join(m.etcRoot, "ssh", "sshd_config"))
	packageManager := m.detectPackageManager()
	packageSources := len(m.packageSourceFiles(packageManager.kind)) > 0
	_, _, _, updatePlanErr := m.maintenanceSteps("update")
	_, _, _, cacheCleanupPlanErr := m.maintenanceSteps("cleanup-cache")
	_, _, _, standardCleanupPlanErr := m.maintenanceSteps("cleanup-standard")
	maintenanceExecutorAvailable := systemdRunErr == nil && helperErr == nil
	updateSupported := maintenanceExecutorAvailable && updatePlanErr == nil
	cleanupSupported := maintenanceExecutorAvailable &&
		(cacheCleanupPlanErr == nil || standardCleanupPlanErr == nil)
	executorReason := ""
	if systemdRunErr != nil {
		executorReason = "systemd 后台任务执行器不可用"
	} else if helperErr != nil {
		executorReason = "Agent 后台执行程序不可用，请更新或重新安装 KPanel"
	}
	updateReason := executorReason
	if updateReason == "" {
		updateReason = capabilityReason(updatePlanErr)
	}
	cleanupReason := executorReason
	if cleanupReason == "" && cacheCleanupPlanErr != nil && standardCleanupPlanErr != nil {
		cleanupReason = capabilityReason(cacheCleanupPlanErr)
	}
	if updateReason == "" {
		updateReason = "当前版本尚未实现此发行版的系统维护适配器"
	}
	if cleanupReason == "" {
		cleanupReason = "当前版本尚未实现此发行版的系统清理适配器"
	}
	aptMirrorSupported := packageManager.kind == packageManagerAPT &&
		(packageManager.osID == "debian" || packageManager.osID == "ubuntu") &&
		packageSources
	mirrorReason := "当前版本尚未实现此发行版的软件源写入适配器"
	if (packageManager.osID == "debian" || packageManager.osID == "ubuntu") && !packageSources {
		mirrorReason = "未发现可由当前适配器修改的 APT 软件源"
	}
	dnsBackendErr := chattrErr
	dnsBackendReason := "静态 resolv.conf 写入所需的 chattr 不可用"
	if m.usesSystemdResolved() {
		dnsBackendErr = systemctlErr
		dnsBackendReason = "systemd-resolved 写入所需的 systemctl 不可用"
	}
	dnsSupported := envErr == nil && bashErr == nil && dnsScriptErr == nil && dnsBackendErr == nil
	dnsReason := "请更新本机 kejilion.sh 以启用 KPanel DNS 非交互协议"
	if dnsScriptErr == nil && (envErr != nil || bashErr != nil) {
		dnsReason = "执行 kejilion.sh DNS 协议所需的 env 或 bash 不可用"
	} else if dnsScriptErr == nil && dnsBackendErr != nil {
		dnsReason = dnsBackendReason
	}
	return []contract.Capability{
		capability("system.hostname.write", hostnamectlErr == nil, "hostnamectl 不可用"),
		capability("system.ssh-port.write", envErr == nil && bashErr == nil && sshScriptErr == nil && sshConfig, "请更新本机 kejilion.sh 以启用 KPanel SSH 端口协议"),
		capability(
			"system.ssh-defense.write",
			systemdRunErr == nil && helperErr == nil && envErr == nil && bashErr == nil && f2bScriptErr == nil,
			"请更新本机 kejilion.sh 以启用 SSH 防御固定协议",
		),
		capability("system.dns.write", dnsSupported, dnsReason),
		capability("system.timezone.write", timedatectlErr == nil, "timedatectl 不可用"),
		capability("system.swap.write", mkswapErr == nil && swaponErr == nil && swapoffErr == nil && fallocateErr == nil && systemdRunErr == nil && helperErr == nil, "Swap 工具、Agent 后台执行程序或 systemd 事务执行器不完整"),
		capability("system.mirror.write", aptMirrorSupported, mirrorReason),
		capability("system.ip-preference.write", true, ""),
		capability("system.kernel-tuning.write", sysctlErr == nil, "sysctl 不可用"),
		capability("system.bbr.write", sysctlErr == nil && modprobeErr == nil, "内核调优工具不完整"),
		capability(
			"system.bbrv3.write",
			systemdRunErr == nil && helperErr == nil && envErr == nil && bashErr == nil && bbrv3ScriptErr == nil,
			"请更新本机 kejilion.sh 以启用 BBRv3 固定协议",
		),
		capability("system.update.write", updateSupported, updateReason),
		capability("system.cleanup.write", cleanupSupported, cleanupReason),
		capability("system.reboot.write", systemctlErr == nil && systemdRunErr == nil, "systemctl 或 systemd-run 不可用"),
		{ID: "system.reinstall", Enabled: false, Reason: "尚未实现 kejilion.sh 重装流程的非交互参数与任务恢复协议"},
	}
}

func (m *Manager) Execute(ctx context.Context, input contract.SystemActionRequest) (contract.SystemActionResult, error) {
	if !m.enabled {
		return contract.SystemActionResult{}, ErrDisabled
	}
	if runtime.GOOS != "linux" || m.effectiveUID() != 0 {
		return contract.SystemActionResult{}, ErrUnsupported
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	result := contract.SystemActionResult{
		Action: input.Action, Status: "succeeded", AppliedAt: m.now().UTC(),
	}
	var err error
	switch input.Action {
	case "hostname":
		result.Changed, result.BackupPath, result.Message, err = m.setHostname(ctx, input.Hostname)
	case "ssh-port":
		result.Changed, result.BackupPath, result.Message, err = m.addSSHPort(ctx, input.Port)
	case "ssh-defense":
		if input.Enabled == nil {
			err = fmt.Errorf("%w: enabled is required", ErrInvalidInput)
			break
		}
		policy := "disable"
		if *input.Enabled {
			policy = "enable"
		}
		result.Changed, result.Message, err = m.startMaintenance(ctx, input.Action, policy)
		if err == nil {
			result.Status = "accepted"
		}
	case "dns":
		result.Changed, result.BackupPath, result.Message, err = m.setDNS(ctx, input.Servers)
	case "timezone":
		result.Changed, result.Message, err = m.setTimezone(ctx, input.Timezone)
	case "swap":
		result.Changed, result.BackupPath, result.Message, err = m.runSwapViaSystemd(ctx, input.SwapSizeMiB)
	case "mirror":
		result.Changed, result.BackupPath, result.Message, err = m.setMirror(ctx, input.MirrorPreset)
	case "ip-preference":
		result.Changed, result.BackupPath, result.Message, err = m.setIPPreference(input.Preference)
	case "kernel-tuning":
		result.Changed, result.BackupPath, result.Message, err = m.setKernelTuning(ctx, input.Profile)
	case "bbr":
		if input.Enabled == nil {
			err = fmt.Errorf("%w: enabled is required", ErrInvalidInput)
			break
		}
		result.Changed, result.BackupPath, result.Message, err = m.setBBR(ctx, *input.Enabled)
	case "bbrv3":
		result.Changed, result.Message, err = m.startMaintenance(
			ctx,
			input.Action,
			input.MaintenancePolicy,
		)
		if err == nil {
			result.Status = "accepted"
		}
	case "update":
		result.Changed, result.Message, err = m.startMaintenance(ctx, input.Action, input.MaintenancePolicy)
		if err == nil {
			result.Status = "accepted"
		}
	case "cleanup":
		result.Changed, result.Message, err = m.startMaintenance(ctx, input.Action, input.MaintenancePolicy)
		if err == nil {
			result.Status = "accepted"
		}
	case "reboot":
		result.Changed, result.Message, err = m.scheduleReboot(ctx, input)
		if err == nil {
			result.Status = "accepted"
		}
	default:
		err = fmt.Errorf("%w: unknown action", ErrInvalidInput)
	}
	if err != nil {
		result.Status = "failed"
		return result, err
	}
	return result, nil
}

func (m *Manager) setHostname(ctx context.Context, value string) (bool, string, string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if !validHostname(value) {
		return false, "", "", fmt.Errorf("%w: hostname must be a valid DNS hostname", ErrInvalidInput)
	}
	hostnamePath := filepath.Join(m.etcRoot, "hostname")
	hostsPath := filepath.Join(m.etcRoot, "hosts")
	oldHostname := strings.TrimSpace(readLimited(hostnamePath))
	if oldHostname == value {
		return false, "", "主机名已经是 " + value, nil
	}
	backup, err := m.createBackup("hostname", hostnamePath, hostsPath)
	if err != nil {
		return false, "", "", err
	}
	oldHosts, err := os.ReadFile(hostsPath)
	if err != nil {
		return false, backup, "", fmt.Errorf("%w: read hosts: %v", ErrUnsupported, err)
	}
	newHosts := updateHosts(oldHosts, oldHostname, value)
	if err := writeAtomic(hostnamePath, []byte(value+"\n"), 0o644); err != nil {
		return false, backup, "", err
	}
	if err := writeAtomic(hostsPath, newHosts, 0o644); err != nil {
		_ = writeAtomic(hostnamePath, []byte(oldHostname+"\n"), 0o644)
		return false, backup, "", fmt.Errorf("%w: %v", ErrRolledBack, err)
	}
	if _, err := m.runner.Run(ctx, "hostnamectl", "set-hostname", value); err != nil {
		_ = writeAtomic(hostnamePath, []byte(oldHostname+"\n"), 0o644)
		_ = writeAtomic(hostsPath, oldHosts, 0o644)
		_, _ = m.runner.Run(ctx, "hostnamectl", "set-hostname", oldHostname)
		return false, backup, "", fmt.Errorf("%w: hostnamectl: %v", ErrRolledBack, err)
	}
	output, err := m.runner.Run(ctx, "hostname")
	if err != nil || strings.TrimSpace(string(output)) != value {
		_ = writeAtomic(hostnamePath, []byte(oldHostname+"\n"), 0o644)
		_ = writeAtomic(hostsPath, oldHosts, 0o644)
		_, rollbackErr := m.runner.Run(ctx, "hostnamectl", "set-hostname", oldHostname)
		if rollbackErr != nil {
			return false, backup, "", fmt.Errorf("%w: hostname verification failed and rollback command failed", ErrNeedsAttention)
		}
		return false, backup, "", fmt.Errorf("%w: hostname verification failed", ErrRolledBack)
	}
	return true, backup, "主机名已更新并回读验证", nil
}

func (m *Manager) addSSHPort(ctx context.Context, port uint16) (bool, string, string, error) {
	if port == 0 {
		return false, "", "", fmt.Errorf("%w: port must be between 1 and 65535", ErrInvalidInput)
	}
	current := m.configuredSSHPorts()
	if len(current) == 1 && current[0] == port {
		return false, "", "SSH 已使用该端口", nil
	}
	script, err := m.dnsScript()
	if err != nil {
		return false, "", "", fmt.Errorf("%w: update kejilion.sh to enable the KPanel SSH port protocol", ErrUnsupported)
	}
	content, err := os.ReadFile(script)
	if err != nil || !trustedKejilionSSHPortContent(content) {
		return false, "", "", fmt.Errorf("%w: update kejilion.sh to enable the KPanel SSH port protocol", ErrUnsupported)
	}
	for _, command := range []string{"env", "bash"} {
		if _, err := m.runner.LookPath(command); err != nil {
			return false, "", "", fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}

	configPath := filepath.Join(m.etcRoot, "ssh", "sshd_config")
	fragments, err := filepath.Glob(filepath.Join(m.etcRoot, "ssh", "sshd_config.d", "*.conf"))
	if err != nil {
		return false, "", "", fmt.Errorf("%w: enumerate sshd configuration fragments: %v", ErrUnsupported, err)
	}
	slices.Sort(fragments)
	paths := append([]string{configPath}, fragments...)
	backup, err := m.createBackup("ssh-port", paths...)
	if err != nil {
		return false, "", "", err
	}
	output, err := m.runner.Run(ctx, "env",
		"KJ_SSH_PORT_NONINTERACTIVE=1", "LC_ALL=C.UTF-8", "LANG=C.UTF-8",
		"bash", script, "ssh-port", strconv.Itoa(int(port)),
	)
	if err != nil {
		return false, backup, "", fmt.Errorf("%w: kejilion.sh SSH port transaction: %v", ErrNeedsAttention, err)
	}
	if strings.Contains(string(output), "KPANEL_SSH_RESULT unchanged") {
		return false, backup, "SSH 已使用该端口", nil
	}
	if !strings.Contains(string(output), "KPANEL_SSH_RESULT applied") {
		return false, backup, "", fmt.Errorf(
			"%w: kejilion.sh SSH port transaction did not return a result marker",
			ErrNeedsAttention,
		)
	}
	current = m.configuredSSHPorts()
	if len(current) != 1 || current[0] != port {
		return false, backup, "", fmt.Errorf(
			"%w: kejilion.sh SSH port result did not match the host configuration",
			ErrNeedsAttention,
		)
	}
	return true, backup, fmt.Sprintf("SSH 端口已由 kejilion.sh 修改为 %d 并回读验证", port), nil
}

func (m *Manager) setDNS(ctx context.Context, servers []string) (bool, string, string, error) {
	if len(servers) < 1 || len(servers) > 4 {
		return false, "", "", fmt.Errorf("%w: between one and four DNS servers are required", ErrInvalidInput)
	}
	normalized := make([]string, 0, len(servers))
	seen := make(map[string]bool)
	ipv4Count := 0
	ipv6Count := 0
	for _, raw := range servers {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() {
			return false, "", "", fmt.Errorf("%w: invalid DNS address %q", ErrInvalidInput, raw)
		}
		value := ip.String()
		if !seen[value] {
			seen[value] = true
			normalized = append(normalized, value)
			if ip.To4() != nil {
				ipv4Count++
			} else {
				ipv6Count++
			}
		}
	}
	if ipv4Count > 2 || ipv6Count > 2 {
		return false, "", "", fmt.Errorf(
			"%w: kejilion.sh accepts at most two IPv4 and two IPv6 DNS servers",
			ErrInvalidInput,
		)
	}
	script, err := m.dnsScript()
	if err != nil {
		return false, "", "", fmt.Errorf(
			"%w: update kejilion.sh to enable the KPanel DNS protocol",
			ErrUnsupported,
		)
	}
	for _, command := range []string{"env", "bash"} {
		if _, err := m.runner.LookPath(command); err != nil {
			return false, "", "", fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}
	backendCommand := "chattr"
	if m.usesSystemdResolved() {
		backendCommand = "systemctl"
	}
	if _, err := m.runner.LookPath(backendCommand); err != nil {
		return false, "", "", fmt.Errorf("%w: %s is unavailable", ErrUnsupported, backendCommand)
	}
	backupPath, err := m.dnsBackupPath()
	if err != nil {
		return false, "", "", err
	}
	backup, err := m.createBackup("dns", backupPath)
	if err != nil {
		return false, "", "", err
	}
	arguments := []string{
		"KJ_DNS_NONINTERACTIVE=1",
		"LC_ALL=C.UTF-8",
		"LANG=C.UTF-8",
		"bash",
		script,
		"dns",
	}
	arguments = append(arguments, normalized...)
	output, err := m.runner.Run(ctx, "env", arguments...)
	if err != nil {
		if strings.Contains(err.Error(), "需要人工检查") {
			return false, backup, "", fmt.Errorf("%w: kejilion.sh DNS transaction: %v", ErrNeedsAttention, err)
		}
		return false, backup, "", fmt.Errorf("%w: kejilion.sh DNS transaction: %v", ErrRolledBack, err)
	}
	if strings.Contains(string(output), "KPANEL_DNS_RESULT unchanged") {
		return false, backup, "DNS 配置没有变化", nil
	}
	if strings.Contains(string(output), "KPANEL_DNS_RESULT applied") {
		return true, backup, "DNS 已由 kejilion.sh 原生事务更新并回读验证", nil
	}
	return false, backup, "", fmt.Errorf(
		"%w: kejilion.sh DNS transaction did not return a result marker",
		ErrNeedsAttention,
	)
}

func (m *Manager) setTimezone(ctx context.Context, zone string) (bool, string, error) {
	zone = strings.TrimSpace(zone)
	if zone == "" || strings.HasPrefix(zone, "/") || strings.Contains(zone, "..") || strings.ContainsAny(zone, "\x00\r\n") {
		return false, "", fmt.Errorf("%w: invalid timezone", ErrInvalidInput)
	}
	zoneRoot := filepath.Join(m.etcRoot, "..", "usr", "share", "zoneinfo")
	if m.etcRoot == "/etc" {
		zoneRoot = "/usr/share/zoneinfo"
	}
	candidate := filepath.Clean(filepath.Join(zoneRoot, filepath.FromSlash(zone)))
	relative, err := filepath.Rel(filepath.Clean(zoneRoot), candidate)
	if err != nil || strings.HasPrefix(relative, "..") || !regularFileFollow(candidate) {
		return false, "", fmt.Errorf("%w: timezone is not present in the IANA database", ErrInvalidInput)
	}
	oldOutput, err := m.runner.Run(ctx, "timedatectl", "show", "--property=Timezone", "--value")
	if err != nil {
		return false, "", fmt.Errorf("%w: read current timezone: %v", ErrUnsupported, err)
	}
	old := strings.TrimSpace(string(oldOutput))
	if old == zone {
		return false, "系统时区没有变化", nil
	}
	if _, err := m.runner.Run(ctx, "timedatectl", "set-timezone", zone); err != nil {
		return false, "", fmt.Errorf("%w: set timezone: %v", ErrRolledBack, err)
	}
	currentOutput, err := m.runner.Run(ctx, "timedatectl", "show", "--property=Timezone", "--value")
	if err != nil || strings.TrimSpace(string(currentOutput)) != zone {
		_, rollbackErr := m.runner.Run(ctx, "timedatectl", "set-timezone", old)
		if rollbackErr != nil {
			return false, "", fmt.Errorf("%w: timezone verification failed and rollback failed", ErrNeedsAttention)
		}
		return false, "", fmt.Errorf("%w: timezone verification failed", ErrRolledBack)
	}
	return true, "系统时区已更新并回读验证", nil
}

func (m *Manager) setIPPreference(preference string) (bool, string, string, error) {
	if preference != "ipv4" && preference != "system_default" {
		return false, "", "", fmt.Errorf("%w: preference must be ipv4 or system_default", ErrInvalidInput)
	}
	path := filepath.Join(m.etcRoot, "gai.conf")
	old, existed, mode, err := snapshotFile(path)
	if err != nil {
		return false, "", "", err
	}
	newData := updateIPPreference(old, preference)
	if bytes.Equal(old, newData) {
		return false, "", "IP 优先级配置没有变化", nil
	}
	backup, err := m.createBackup("ip-preference", path)
	if err != nil {
		return false, "", "", err
	}
	if mode == 0 {
		mode = 0o644
	}
	if err := writeAtomic(path, newData, mode); err != nil {
		_ = restoreFile(path, old, existed, mode)
		return false, backup, "", err
	}
	if preference == "ipv4" {
		return true, backup, "已设置 IPv4 优先；kejilion.sh 可识别同一规则", nil
	}
	return true, backup, "已恢复系统默认地址优先级", nil
}

func (m *Manager) setSwap(ctx context.Context, sizeMiB int) (bool, string, string, error) {
	return m.applySwap(ctx, sizeMiB)
}

func (m *Manager) setBBR(ctx context.Context, enabled bool) (bool, string, string, error) {
	path := filepath.Join(m.etcRoot, "sysctl.d", "99-kejilion-bbr.conf")
	old, existed, mode, err := snapshotFile(path)
	if err != nil {
		return false, "", "", err
	}
	targetCC, targetQDisc := "cubic", "fq_codel"
	if enabled {
		if _, err := m.runner.Run(ctx, "modprobe", "tcp_bbr"); err != nil {
			return false, "", "", fmt.Errorf("%w: load tcp_bbr: %v", ErrUnsupported, err)
		}
		available := strings.Fields(m.procValue("sys/net/ipv4/tcp_available_congestion_control"))
		if !slices.Contains(available, "bbr") {
			return false, "", "", fmt.Errorf("%w: the running kernel does not expose BBR", ErrUnsupported)
		}
		targetCC, targetQDisc = "bbr", "fq"
	}
	config := []byte("# Managed by KPanel; compatible with kejilion.sh bbr_on\n" +
		"net.core.default_qdisc=" + targetQDisc + "\n" +
		"net.ipv4.tcp_congestion_control=" + targetCC + "\n")
	currentCC := strings.TrimSpace(m.procValue("sys/net/ipv4/tcp_congestion_control"))
	currentQDisc := strings.TrimSpace(m.procValue("sys/net/core/default_qdisc"))
	if currentCC == targetCC && currentQDisc == targetQDisc && bytes.Equal(bytes.TrimSpace(old), bytes.TrimSpace(config)) {
		return false, "", "BBR 配置没有变化", nil
	}
	backup, err := m.createBackup("bbr", path)
	if err != nil {
		return false, "", "", err
	}
	if err := writeAtomic(path, config, 0o644); err != nil {
		return false, backup, "", err
	}
	if _, err := m.runner.Run(ctx, "sysctl", "-p", path); err != nil {
		_ = restoreFile(path, old, existed, mode)
		if existed {
			_, _ = m.runner.Run(ctx, "sysctl", "-p", path)
		} else {
			_, _ = m.runner.Run(ctx, "sysctl", "--system")
		}
		return false, backup, "", fmt.Errorf("%w: apply BBR configuration: %v", ErrRolledBack, err)
	}
	if strings.TrimSpace(m.procValue("sys/net/ipv4/tcp_congestion_control")) != targetCC {
		_ = restoreFile(path, old, existed, mode)
		_, _ = m.runner.Run(ctx, "sysctl", "--system")
		return false, backup, "", fmt.Errorf("%w: congestion control verification failed", ErrRolledBack)
	}
	if enabled {
		return true, backup, "BBR 已启用并回读验证", nil
	}
	return true, backup, "BBR 已停用，已恢复 cubic/fq_codel", nil
}

func (m *Manager) createBackup(action string, paths ...string) (string, error) {
	var nonce [4]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("generate backup identifier: %w", err)
	}
	name := m.now().UTC().Format("20060102T150405Z") + "-" + safeName(action) + "-" + hex.EncodeToString(nonce[:])
	root := filepath.Join(m.stateDir, "backups", name)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create system backup: %w", err)
	}
	var manifest strings.Builder
	for index, path := range paths {
		fmt.Fprintf(&manifest, "%d\t%s\n", index, path)
		data, existed, mode, err := snapshotFile(path)
		if err != nil {
			return root, err
		}
		if !existed {
			if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("%02d.absent", index)), nil, 0o600); err != nil {
				return root, err
			}
			continue
		}
		backupPath := filepath.Join(root, fmt.Sprintf("%02d-%s", index, safeName(filepath.Base(path))))
		if err := os.WriteFile(backupPath, data, fileModeOr(mode, 0o600)); err != nil {
			return root, err
		}
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.tsv"), []byte(manifest.String()), 0o600); err != nil {
		return root, err
	}
	return root, nil
}

func (m *Manager) configuredSSHPorts() []uint16 {
	files := []string{filepath.Join(m.etcRoot, "ssh", "sshd_config")}
	fragments, _ := filepath.Glob(filepath.Join(m.etcRoot, "ssh", "sshd_config.d", "*.conf"))
	slices.Sort(fragments)
	files = append(files, fragments...)
	var ports []uint16
	for _, path := range files {
		scanner := bufio.NewScanner(strings.NewReader(readLimited(path)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) != 2 || !strings.EqualFold(fields[0], "Port") {
				continue
			}
			value, err := strconv.ParseUint(fields[1], 10, 16)
			if err == nil && value > 0 {
				ports = append(ports, uint16(value))
			}
		}
	}
	if len(ports) == 0 {
		return []uint16{22}
	}
	slices.Sort(ports)
	return slices.Compact(ports)
}

func (m *Manager) dnsBackupPath() (string, error) {
	path := filepath.Join(m.etcRoot, "resolv.conf")
	if m.usesSystemdResolved() {
		return filepath.Join(m.etcRoot, "systemd", "resolved.conf.d", "90-kpanel.conf"), nil
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("%w: %s is not a regular file", ErrConflict, path)
		}
		return path, nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("%w: resolve DNS configuration target: %v", ErrConflict, err)
	}
	if !regularFile(target) {
		return "", fmt.Errorf("%w: DNS configuration target is not a regular file", ErrConflict)
	}
	return target, nil
}

func (m *Manager) usesSystemdResolved() bool {
	target, err := os.Readlink(filepath.Join(m.etcRoot, "resolv.conf"))
	return err == nil && strings.Contains(strings.ToLower(target), "systemd/resolve")
}

func findKejilionDNSScript() (string, error) {
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
			info.Mode().Perm()&0o022 != 0 || !dnsScriptOwnerTrusted(info) {
			continue
		}
		content, err := os.ReadFile(resolved)
		if err != nil {
			continue
		}
		if trustedKejilionDNSContent(content) {
			return resolved, nil
		}
	}
	return "", errors.New("a trusted kejilion.sh DNS command was not found")
}

func trustedKejilionDNSContent(content []byte) bool {
	value := string(content)
	return dnsScriptLicense.Match(content) &&
		strings.Contains(value, "KJ_DNS_NONINTERACTIVE") &&
		strings.Contains(value, "kpanel_protocol_active") &&
		strings.Contains(value, "kpanel_set_dns_noninteractive")
}

func trustedKejilionSSHPortContent(content []byte) bool {
	value := string(content)
	return dnsScriptLicense.Match(content) &&
		strings.Contains(value, "KJ_SSH_PORT_NONINTERACTIVE") &&
		strings.Contains(value, "kpanel_protocol_active") &&
		strings.Contains(value, "kpanel_ssh_port_noninteractive") &&
		strings.Contains(value, "new_ssh_port \"$new_port\"") &&
		strings.Contains(value, "KPANEL_SSH_RESULT applied")
}

func (m *Manager) aptSourceFiles() []string {
	var files []string
	primary := filepath.Join(m.etcRoot, "apt", "sources.list")
	if regularFile(primary) {
		files = append(files, primary)
	}
	for _, pattern := range []string{
		filepath.Join(m.etcRoot, "apt", "sources.list.d", "*.list"),
		filepath.Join(m.etcRoot, "apt", "sources.list.d", "*.sources"),
	} {
		matches, _ := filepath.Glob(pattern)
		slices.Sort(matches)
		for _, path := range matches {
			if regularFile(path) {
				files = append(files, path)
			}
		}
	}
	return files
}

func (m *Manager) swapActive(path string) bool {
	info, statErr := os.Lstat(path)
	for _, line := range strings.Split(m.procValue("swaps"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		exactPath := fields[0] == path
		legacyAlias := path == filepath.Join(m.stateDir, "swapfile") &&
			fields[0] == m.swapPath
		if !exactPath && !legacyAlias {
			continue
		}
		if statErr != nil || !info.Mode().IsRegular() {
			return exactPath
		}
		sizeKiB, err := strconv.ParseUint(fields[2], 10, 64)
		if err == nil && swapSizeMatches(info.Size(), sizeKiB*1024) {
			return true
		}
	}
	return false
}

func (m *Manager) procValue(relative string) string {
	data, _ := os.ReadFile(filepath.Join(m.procRoot, filepath.FromSlash(relative)))
	return string(data)
}

func validHostname(value string) bool {
	if len(value) < 1 || len(value) > 253 || strings.HasSuffix(value, ".") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func updateHosts(data []byte, oldHostname, newHostname string) []byte {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	found := false
	for index, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] != "127.0.0.1" && fields[0] != "127.0.1.1" && fields[0] != "::1" {
			continue
		}
		aliases := make([]string, 0, len(fields))
		if fields[0] == "127.0.1.1" {
			aliases = append(aliases, newHostname)
			found = true
		}
		for _, alias := range fields[1:] {
			if alias != oldHostname && alias != newHostname {
				aliases = append(aliases, alias)
			}
		}
		lines[index] = fields[0]
		if len(aliases) > 0 {
			lines[index] += "\t" + strings.Join(aliases, " ")
		}
	}
	if !found {
		lines = append(lines, "127.0.1.1\t"+newHostname)
	}
	return []byte(strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n")
}

func updateIPPreference(data []byte, preference string) []byte {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == kpanelIPPreferenceMarker ||
			regexp.MustCompile(`^precedence\s+::ffff:0:0/96\s+100(?:\s*#.*)?$`).MatchString(trimmed) {
			continue
		}
		kept = append(kept, line)
	}
	result := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if preference == "ipv4" {
		if result != "" {
			result += "\n\n"
		}
		result += kpanelIPPreferenceMarker + "\nprecedence ::ffff:0:0/96  100"
	}
	if result == "" {
		return nil
	}
	return []byte(result + "\n")
}

func updateFstabSwap(data []byte, path, legacyPath string, enabled bool) []byte {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		managedPath := len(fields) >= 3 &&
			(fields[0] == path || (legacyPath != "" && fields[0] == legacyPath)) &&
			fields[2] == "swap"
		if trimmed == kpanelSwapMarker || managedPath {
			continue
		}
		lines = append(lines, line)
	}
	result := strings.TrimRight(strings.Join(lines, "\n"), "\n")
	if enabled {
		if result != "" {
			result += "\n"
		}
		result += path + " swap swap defaults 0 0"
	}
	if result == "" {
		return nil
	}
	return []byte(result + "\n")
}

func osReleaseValue(path, key string) string {
	for _, line := range strings.Split(readLimited(path), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok && name == key {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return ""
}

func snapshotFile(path string) ([]byte, bool, os.FileMode, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, 0, nil
	}
	if err != nil {
		return nil, false, 0, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false, 0, fmt.Errorf("%w: %s is not a regular file", ErrConflict, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, 0, err
	}
	if len(data) > 8<<20 {
		return nil, false, 0, fmt.Errorf("%w: configuration file %s exceeds 8 MiB", ErrConflict, path)
	}
	return data, true, info.Mode().Perm(), nil
}

func restoreFile(path string, data []byte, existed bool, mode os.FileMode) error {
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	return writeAtomic(path, data, fileModeOr(mode, 0o600))
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: refusing to replace non-regular file %s", ErrConflict, path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(parent, "."+filepath.Base(path)+".kpanel-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode.Perm()); err != nil {
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
	return renameReplace(tempPath, path)
}

func readLimited(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, _ := io.ReadAll(io.LimitReader(file, 8<<20))
	return string(data)
}

func regularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func regularFileFollow(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func fileModeOr(value, fallback os.FileMode) os.FileMode {
	if value == 0 {
		return fallback
	}
	return value.Perm()
}

func sizeWithinMiB(size int64, wanted int) bool {
	return size >= int64(wanted)*1024*1024 && size < int64(wanted+1)*1024*1024
}

func swapSizeMatches(fileSize int64, activeSizeBytes uint64) bool {
	if fileSize < 0 || uint64(fileSize) < activeSizeBytes {
		return false
	}
	const swapMetadataAllowance = 8 * 1024 * 1024
	return uint64(fileSize)-activeSizeBytes <= swapMetadataAllowance
}

func safeName(value string) string {
	value = strings.ToLower(value)
	var out strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			out.WriteRune(char)
		} else {
			out.WriteByte('_')
		}
	}
	if out.Len() == 0 {
		return "item"
	}
	return out.String()
}
