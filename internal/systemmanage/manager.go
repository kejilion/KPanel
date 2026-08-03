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
	cronFieldPattern  = regexp.MustCompile(`^[0-9*/?,\-]+$`)
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
	_, sshdErr := m.runner.LookPath("sshd")
	_, ssErr := m.runner.LookPath("ss")
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
	hostsScriptErr, cronScriptErr := dnsScriptErr, dnsScriptErr
	networkScriptErr, firewallScriptErr := dnsScriptErr, dnsScriptErr
	if dnsScriptErr == nil {
		script, _ := m.dnsScript()
		content, err := os.ReadFile(script)
		if err != nil || !trustedKejilionHostsContent(content) {
			hostsScriptErr = errors.New("trusted kejilion.sh hosts protocol was not found")
		}
		if err != nil || !trustedKejilionCronContent(content) {
			cronScriptErr = errors.New("trusted kejilion.sh cron protocol was not found")
		}
		if err != nil || !trustedKejilionNetworkContent(content) {
			networkScriptErr = errors.New("trusted kejilion.sh network protocol was not found")
		}
		if err != nil || !trustedKejilionFirewallContent(content) {
			firewallScriptErr = errors.New("trusted kejilion.sh firewall protocol was not found")
		}
	}
	_, crontabErr := m.runner.LookPath("crontab")
	_, ipErr := m.runner.LookPath("ip")
	_, iptablesErr := m.runner.LookPath("iptables")
	_, iptablesSaveErr := m.runner.LookPath("iptables-save")
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
		capability("system.ssh-port.write", sshdErr == nil && ssErr == nil && systemctlErr == nil && sshConfig, "OpenSSH 服务或配置不可用"),
		capability(
			"system.ssh-defense.write",
			systemdRunErr == nil && helperErr == nil && envErr == nil && bashErr == nil && f2bScriptErr == nil,
			"请更新本机 kejilion.sh 以启用 SSH 防御固定协议",
		),
		capability("system.dns.write", dnsSupported, dnsReason),
		capability("system.hosts.write", envErr == nil && bashErr == nil && hostsScriptErr == nil, "请更新本机 kejilion.sh 以启用 KPanel hosts 非交互协议"),
		capability("system.cron.write", envErr == nil && bashErr == nil && crontabErr == nil && cronScriptErr == nil, "请安装 crontab 并更新本机 kejilion.sh 以启用 KPanel cron 非交互协议"),
		capability("system.network-interface.write", envErr == nil && bashErr == nil && ipErr == nil && networkScriptErr == nil, "请安装 iproute2 并更新本机 kejilion.sh 以启用 KPanel 网卡协议"),
		capability("system.firewall.write", envErr == nil && bashErr == nil && iptablesErr == nil && iptablesSaveErr == nil && firewallScriptErr == nil, "请安装 iptables 并更新本机 kejilion.sh 以启用 KPanel 防火墙协议"),
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
	case "hosts":
		result.Changed, result.BackupPath, result.Message, err = m.setHosts(ctx, input.HostsOperation, input.HostsEntry)
	case "cron":
		result.Changed, result.Message, err = m.setCron(ctx, input.CronOperation, input.CronEntry)
	case "network-interface":
		result.Changed, result.Message, err = m.setNetworkInterface(ctx, input.NetworkOperation, input.InterfaceName)
	case "firewall":
		result.Changed, result.Message, err = m.setFirewall(ctx, input)
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
	type sshConfigSnapshot struct {
		path    string
		data    []byte
		existed bool
		mode    os.FileMode
	}
	snapshots := make([]sshConfigSnapshot, 0, len(paths))
	for _, path := range paths {
		data, existed, mode, snapshotErr := snapshotFile(path)
		if snapshotErr != nil {
			return false, backup, "", snapshotErr
		}
		snapshots = append(snapshots, sshConfigSnapshot{
			path: path, data: data, existed: existed, mode: mode,
		})
	}
	rollback := func() error {
		var rollbackErrors []error
		for index := len(snapshots) - 1; index >= 0; index-- {
			snapshot := snapshots[index]
			if restoreErr := restoreFile(
				snapshot.path,
				snapshot.data,
				snapshot.existed,
				snapshot.mode,
			); restoreErr != nil {
				rollbackErrors = append(rollbackErrors, restoreErr)
			}
		}
		if len(rollbackErrors) > 0 {
			return errors.Join(rollbackErrors...)
		}
		_, reloadErr := m.reloadSSH(ctx)
		return reloadErr
	}

	for index, snapshot := range snapshots {
		updated := removeSSHPortDirectives(snapshot.data)
		if index == 0 {
			updated = append(
				[]byte(fmt.Sprintf("Port %d\n", port)),
				updated...,
			)
		}
		if err := writeAtomic(snapshot.path, updated, fileModeOr(snapshot.mode, 0o640)); err != nil {
			_ = rollback()
			return false, backup, "", fmt.Errorf("%w: update SSH port configuration: %v", ErrRolledBack, err)
		}
	}
	if _, err := m.runner.Run(ctx, "sshd", "-t", "-f", filepath.Join(m.etcRoot, "ssh", "sshd_config")); err != nil {
		_ = rollback()
		return false, backup, "", fmt.Errorf("%w: sshd configuration test: %v", ErrRolledBack, err)
	}
	firewallRollback, err := m.openFirewallPort(ctx, port)
	if err != nil {
		_ = rollback()
		return false, backup, "", fmt.Errorf("%w: firewall: %v", ErrRolledBack, err)
	}
	if _, err := m.reloadSSH(ctx); err != nil {
		_ = firewallRollback()
		_ = rollback()
		return false, backup, "", fmt.Errorf("%w: reload SSH: %v", ErrRolledBack, err)
	}
	listening := false
	for attempt := 0; attempt < 10; attempt++ {
		if listening, _ = m.portListening(ctx, port); listening {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !listening {
		_ = firewallRollback()
		if err := rollback(); err != nil {
			return false, backup, "", fmt.Errorf("%w: new SSH port did not listen and rollback failed: %v", ErrNeedsAttention, err)
		}
		return false, backup, "", fmt.Errorf("%w: new SSH port did not listen", ErrRolledBack)
	}
	return true, backup, fmt.Sprintf("SSH 端口已修改为 %d，与 kejilion.sh 的单端口配置语义一致", port), nil
}

func removeSSHPortDirectives(data []byte) []byte {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		candidate := strings.TrimSpace(line)
		candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "#"))
		fields := strings.Fields(candidate)
		if len(fields) >= 1 && strings.EqualFold(fields[0], "Port") {
			continue
		}
		kept = append(kept, line)
	}
	result := strings.TrimLeft(strings.TrimRight(strings.Join(kept, "\n"), "\n"), "\n")
	if result == "" {
		return nil
	}
	return []byte(result + "\n")
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

func (m *Manager) setHosts(ctx context.Context, operation, entry string) (bool, string, string, error) {
	if operation != "add" && operation != "delete" {
		return false, "", "", fmt.Errorf("%w: hosts operation must be add or delete", ErrInvalidInput)
	}
	fields := strings.Fields(entry)
	if len(fields) < 2 || len(fields) > 16 || net.ParseIP(fields[0]) == nil {
		return false, "", "", fmt.Errorf("%w: invalid hosts entry", ErrInvalidInput)
	}
	for _, hostname := range fields[1:] {
		if len(hostname) > 253 || !validHostname(strings.ToLower(hostname)) {
			return false, "", "", fmt.Errorf("%w: invalid hosts hostname", ErrInvalidInput)
		}
	}
	entry = strings.Join(fields, " ")
	script, err := m.dnsScript()
	if err != nil {
		return false, "", "", fmt.Errorf("%w: update kejilion.sh to enable the KPanel hosts protocol", ErrUnsupported)
	}
	content, err := os.ReadFile(script)
	if err != nil || !trustedKejilionHostsContent(content) {
		return false, "", "", fmt.Errorf("%w: update kejilion.sh to enable the KPanel hosts protocol", ErrUnsupported)
	}
	for _, command := range []string{"env", "bash"} {
		if _, err := m.runner.LookPath(command); err != nil {
			return false, "", "", fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}
	backup, err := m.createBackup("hosts", filepath.Join(m.etcRoot, "hosts"))
	if err != nil {
		return false, "", "", err
	}
	output, err := m.runner.Run(ctx, "env", "KJ_HOSTS_NONINTERACTIVE=1", "LC_ALL=C.UTF-8", "LANG=C.UTF-8", "bash", script, "hosts", operation, entry)
	if err != nil {
		return false, backup, "", fmt.Errorf("%w: kejilion.sh hosts transaction: %v", ErrRolledBack, err)
	}
	if strings.Contains(string(output), "KPANEL_HOSTS_RESULT unchanged") {
		return false, backup, "本地 hosts 记录没有变化", nil
	}
	if strings.Contains(string(output), "KPANEL_HOSTS_RESULT applied") {
		return true, backup, "本地 hosts 已由 kejilion.sh 更新并回读验证", nil
	}
	return false, backup, "", fmt.Errorf("%w: kejilion.sh hosts transaction did not return a result marker", ErrNeedsAttention)
}

func (m *Manager) setCron(ctx context.Context, operation, entry string) (bool, string, error) {
	if operation != "add" && operation != "delete" {
		return false, "", fmt.Errorf("%w: cron operation must be add or delete", ErrInvalidInput)
	}
	entry = strings.TrimSpace(entry)
	if entry == "" || len(entry) > 4096 || strings.ContainsAny(entry, "\x00\r\n") {
		return false, "", fmt.Errorf("%w: invalid cron entry", ErrInvalidInput)
	}
	if operation == "add" {
		fields := strings.Fields(entry)
		if len(fields) < 6 {
			return false, "", fmt.Errorf("%w: cron entry requires five schedule fields and a command", ErrInvalidInput)
		}
		for _, field := range fields[:5] {
			if !cronFieldPattern.MatchString(field) {
				return false, "", fmt.Errorf("%w: invalid cron schedule field", ErrInvalidInput)
			}
		}
	}
	script, err := m.dnsScript()
	if err != nil {
		return false, "", fmt.Errorf("%w: update kejilion.sh to enable the KPanel cron protocol", ErrUnsupported)
	}
	content, err := os.ReadFile(script)
	if err != nil || !trustedKejilionCronContent(content) {
		return false, "", fmt.Errorf("%w: update kejilion.sh to enable the KPanel cron protocol", ErrUnsupported)
	}
	for _, command := range []string{"env", "bash", "crontab"} {
		if _, err := m.runner.LookPath(command); err != nil {
			return false, "", fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}
	output, err := m.runner.Run(ctx, "env", "KJ_CRON_NONINTERACTIVE=1", "LC_ALL=C.UTF-8", "LANG=C.UTF-8", "bash", script, "cron", operation, entry)
	if err != nil {
		return false, "", fmt.Errorf("%w: kejilion.sh cron transaction: %v", ErrRolledBack, err)
	}
	if strings.Contains(string(output), "KPANEL_CRON_RESULT unchanged") {
		return false, "定时任务没有变化", nil
	}
	if strings.Contains(string(output), "KPANEL_CRON_RESULT applied") {
		return true, "定时任务已由 kejilion.sh 更新并验证", nil
	}
	return false, "", fmt.Errorf("%w: kejilion.sh cron transaction did not return a result marker", ErrNeedsAttention)
}

func (m *Manager) NetworkInterfaces(ctx context.Context) []contract.NetworkInterfaceSummary {
	directory := filepath.Join(m.sysRoot, "class", "net")
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	items := make([]contract.NetworkInterfaceSummary, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		item := contract.NetworkInterfaceSummary{Name: name, State: strings.TrimSpace(readLimited(filepath.Join(directory, name, "operstate"))), MAC: strings.TrimSpace(readLimited(filepath.Join(directory, name, "address"))), Addresses: []string{}}
		if output, runErr := m.runner.Run(ctx, "ip", "-o", "addr", "show", "dev", name); runErr == nil {
			for _, line := range strings.Split(string(output), "\n") {
				fields := strings.Fields(line)
				for index, field := range fields {
					if (field == "inet" || field == "inet6") && index+1 < len(fields) {
						item.Addresses = append(item.Addresses, fields[index+1])
						break
					}
				}
			}
		}
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b contract.NetworkInterfaceSummary) int { return strings.Compare(a.Name, b.Name) })
	return items
}

func (m *Manager) FirewallSummary(ctx context.Context) contract.FirewallSummary {
	output, err := m.runner.Run(ctx, "iptables", "-S", "INPUT")
	if err != nil {
		return contract.FirewallSummary{Rules: []string{}}
	}
	summary := contract.FirewallSummary{Available: true, Rules: []string{}}
	var pingBlocked, ddosTCPDrop, ddosUDPDrop bool
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "-P INPUT ") {
			summary.InputPolicy = strings.TrimSpace(strings.TrimPrefix(line, "-P INPUT "))
			continue
		}
		if strings.Contains(line, "--icmp-type echo-request") && strings.HasSuffix(line, "-j ACCEPT") {
			summary.PingAllowed = true
		}
		if strings.Contains(line, "--icmp-type echo-request") && strings.HasSuffix(line, "-j DROP") {
			pingBlocked = true
		}
		if strings.Contains(line, "-p tcp") && strings.Contains(line, "--tcp-flags") && strings.HasSuffix(line, "-j DROP") {
			ddosTCPDrop = true
		}
		if strings.Contains(line, "-p udp") && strings.HasSuffix(line, "-j DROP") {
			ddosUDPDrop = true
		}
		if strings.Contains(line, "connlimit") || strings.Contains(line, "hashlimit") {
			summary.DDOSDefense = true
		}
		summary.Rules = append(summary.Rules, line)
		if len(summary.Rules) == 100 {
			break
		}
	}
	if !summary.PingAllowed && !pingBlocked && summary.InputPolicy == "ACCEPT" {
		summary.PingAllowed = true
	}
	summary.DDOSDefense = summary.DDOSDefense || (ddosTCPDrop && ddosUDPDrop)
	return summary
}

func (m *Manager) setNetworkInterface(ctx context.Context, operation, name string) (bool, string, error) {
	if (operation != "up" && operation != "down") || !regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`).MatchString(name) {
		return false, "", fmt.Errorf("%w: invalid network interface action", ErrInvalidInput)
	}
	if _, err := os.Stat(filepath.Join(m.sysRoot, "class", "net", name)); err != nil {
		return false, "", fmt.Errorf("%w: network interface does not exist", ErrInvalidInput)
	}
	return m.runTrustedNetworkScript(ctx, "network", operation, name, "KPANEL_NETWORK_RESULT", "网卡状态")
}

func (m *Manager) setFirewall(ctx context.Context, input contract.SystemActionRequest) (bool, string, error) {
	allowed := map[string]bool{"port-open": true, "port-close": true, "all-open": true, "all-close": true, "ip-allow": true, "ip-block": true, "ip-remove": true, "ping-allow": true, "ping-block": true, "ddos-enable": true, "ddos-disable": true, "country-block": true, "country-allow": true, "country-unblock": true}
	if !allowed[input.FirewallOperation] {
		return false, "", fmt.Errorf("%w: invalid firewall operation", ErrInvalidInput)
	}
	arguments := []string{"firewall", input.FirewallOperation}
	if strings.HasPrefix(input.FirewallOperation, "port-") {
		if input.FirewallPort == 0 {
			return false, "", fmt.Errorf("%w: firewall port is required", ErrInvalidInput)
		}
		arguments = append(arguments, strconv.Itoa(int(input.FirewallPort)))
	} else if strings.HasPrefix(input.FirewallOperation, "ip-") {
		if _, _, err := net.ParseCIDR(input.FirewallAddress); err != nil && net.ParseIP(input.FirewallAddress) == nil {
			return false, "", fmt.Errorf("%w: firewall address is invalid", ErrInvalidInput)
		}
		arguments = append(arguments, input.FirewallAddress)
	} else if strings.HasPrefix(input.FirewallOperation, "country-") {
		if len(input.CountryCodes) < 1 || len(input.CountryCodes) > 20 {
			return false, "", fmt.Errorf("%w: one to twenty country codes are required", ErrInvalidInput)
		}
		for _, code := range input.CountryCodes {
			code = strings.ToUpper(strings.TrimSpace(code))
			if len(code) != 2 || code[0] < 'A' || code[0] > 'Z' || code[1] < 'A' || code[1] > 'Z' {
				return false, "", fmt.Errorf("%w: invalid country code", ErrInvalidInput)
			}
			arguments = append(arguments, code)
		}
	}
	return m.runTrustedNetworkScript(ctx, arguments[0], arguments[1], strings.Join(arguments[2:], " "), "KPANEL_FIREWALL_RESULT", "防火墙")
}

func (m *Manager) runTrustedNetworkScript(ctx context.Context, command, operation, value, marker, label string) (bool, string, error) {
	script, err := m.dnsScript()
	if err != nil {
		return false, "", fmt.Errorf("%w: update kejilion.sh to enable the KPanel protocol", ErrUnsupported)
	}
	content, err := os.ReadFile(script)
	if err != nil || (command == "network" && !trustedKejilionNetworkContent(content)) || (command == "firewall" && !trustedKejilionFirewallContent(content)) {
		return false, "", fmt.Errorf("%w: update kejilion.sh to enable the KPanel protocol", ErrUnsupported)
	}
	environment := "KJ_NETWORK_NONINTERACTIVE=1"
	if command == "firewall" {
		environment = "KJ_FIREWALL_NONINTERACTIVE=1"
	}
	arguments := []string{environment, "LC_ALL=C.UTF-8", "LANG=C.UTF-8", "bash", script, command, operation}
	if value != "" {
		arguments = append(arguments, strings.Fields(value)...)
	}
	output, err := m.runner.Run(ctx, "env", arguments...)
	if err != nil {
		return false, "", fmt.Errorf("%w: kejilion.sh %s transaction: %v", ErrRolledBack, command, err)
	}
	if strings.Contains(string(output), marker+" unchanged") {
		return false, label + "没有变化", nil
	}
	if strings.Contains(string(output), marker+" applied") {
		return true, label + "已由 kejilion.sh 更新并验证", nil
	}
	return false, "", fmt.Errorf("%w: kejilion.sh transaction did not return a result marker", ErrNeedsAttention)
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

func (m *Manager) reloadSSH(ctx context.Context) ([]byte, error) {
	if output, err := m.runner.Run(ctx, "systemctl", "reload", "ssh.service"); err == nil {
		return output, nil
	}
	return m.runner.Run(ctx, "systemctl", "reload", "sshd.service")
}

func (m *Manager) portListening(ctx context.Context, port uint16) (bool, error) {
	output, err := m.runner.Run(ctx, "ss", "-H", "-ltn")
	if err != nil {
		return false, err
	}
	suffix := ":" + strconv.Itoa(int(port))
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.HasSuffix(fields[3], suffix) {
			return true, nil
		}
	}
	return false, nil
}

func (m *Manager) openFirewallPort(ctx context.Context, port uint16) (func() error, error) {
	rule := strconv.Itoa(int(port)) + "/tcp"
	if _, err := m.runner.LookPath("ufw"); err == nil {
		output, statusErr := m.runner.Run(ctx, "ufw", "status")
		if statusErr == nil && strings.Contains(strings.ToLower(string(output)), "status: active") {
			if _, err := m.runner.Run(ctx, "ufw", "allow", rule); err != nil {
				return func() error { return nil }, err
			}
			return func() error {
				_, err := m.runner.Run(ctx, "ufw", "--force", "delete", "allow", rule)
				return err
			}, nil
		}
	}
	if _, err := m.runner.LookPath("firewall-cmd"); err == nil {
		if _, stateErr := m.runner.Run(ctx, "firewall-cmd", "--state"); stateErr == nil {
			if _, err := m.runner.Run(ctx, "firewall-cmd", "--add-port="+rule); err != nil {
				return func() error { return nil }, err
			}
			if _, err := m.runner.Run(ctx, "firewall-cmd", "--permanent", "--add-port="+rule); err != nil {
				_, _ = m.runner.Run(ctx, "firewall-cmd", "--remove-port="+rule)
				return func() error { return nil }, err
			}
			return func() error {
				_, first := m.runner.Run(ctx, "firewall-cmd", "--remove-port="+rule)
				_, second := m.runner.Run(ctx, "firewall-cmd", "--permanent", "--remove-port="+rule)
				return errors.Join(first, second)
			}, nil
		}
	}
	if _, err := m.runner.LookPath("iptables"); err == nil {
		output, statusErr := m.runner.Run(ctx, "iptables", "-S", "INPUT")
		if statusErr == nil && strings.Contains(string(output), "-P INPUT DROP") {
			if _, allowedErr := m.runner.Run(
				ctx, "iptables", "-C", "INPUT", "-p", "tcp",
				"--dport", strconv.Itoa(int(port)), "-j", "ACCEPT",
			); allowedErr == nil {
				return func() error { return nil }, nil
			}
			arguments := []string{
				"-I", "INPUT", "-p", "tcp",
				"--dport", strconv.Itoa(int(port)), "-j", "ACCEPT",
			}
			if _, insertErr := m.runner.Run(ctx, "iptables", arguments...); insertErr != nil {
				return func() error { return nil }, insertErr
			}
			if _, saveErr := m.runner.LookPath("netfilter-persistent"); saveErr == nil {
				if _, saveErr = m.runner.Run(ctx, "netfilter-persistent", "save"); saveErr != nil {
					_, _ = m.runner.Run(
						ctx, "iptables", "-D", "INPUT", "-p", "tcp",
						"--dport", strconv.Itoa(int(port)), "-j", "ACCEPT",
					)
					return func() error { return nil }, saveErr
				}
			}
			return func() error {
				_, err := m.runner.Run(
					ctx, "iptables", "-D", "INPUT", "-p", "tcp",
					"--dport", strconv.Itoa(int(port)), "-j", "ACCEPT",
				)
				if _, saveErr := m.runner.LookPath("netfilter-persistent"); saveErr == nil {
					_, persistErr := m.runner.Run(ctx, "netfilter-persistent", "save")
					return errors.Join(err, persistErr)
				}
				return err
			}, nil
		}
	}
	return func() error { return nil }, nil
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

func trustedKejilionHostsContent(content []byte) bool {
	value := string(content)
	return dnsScriptLicense.Match(content) && strings.Contains(value, "KJ_HOSTS_NONINTERACTIVE") && strings.Contains(value, "kpanel_hosts_noninteractive")
}
func trustedKejilionCronContent(content []byte) bool {
	value := string(content)
	return dnsScriptLicense.Match(content) && strings.Contains(value, "KJ_CRON_NONINTERACTIVE") && strings.Contains(value, "kpanel_cron_noninteractive")
}
func trustedKejilionNetworkContent(content []byte) bool {
	value := string(content)
	return dnsScriptLicense.Match(content) && strings.Contains(value, "KJ_NETWORK_NONINTERACTIVE") && strings.Contains(value, "kpanel_network_noninteractive")
}
func trustedKejilionFirewallContent(content []byte) bool {
	value := string(content)
	return dnsScriptLicense.Match(content) && strings.Contains(value, "KJ_FIREWALL_NONINTERACTIVE") && strings.Contains(value, "kpanel_firewall_noninteractive")
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
