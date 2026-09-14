package systemmanage

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const sshDefenseManagerOutputLimit = 1 << 20

var (
	sshDefenseManagerProtocolV1Pattern = regexp.MustCompile(`(?m)^KPANEL_F2B_MANAGER_PROTOCOL_VERSION="1"\r?$`)
	sshDefenseManagerJailPattern       = regexp.MustCompile(`^(sshd|alpine-sshd)$`)
	sshDefenseManagerRecoveryPattern   = regexp.MustCompile(`^/var/lib/kejilion-panel/system/recovery/system-resource/[0-9]{8}T[0-9]{6}Z-ssh-defense\.[A-Za-z0-9]{6}$`)
	sshDefenseManagerEventPattern      = regexp.MustCompile(`^([0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2},[0-9]{3}).*\[(?:sshd|alpine-sshd)\].*\b(Found|Ban|Unban)[[:space:]]+([^[:space:]]+)`)
)

func (m *Manager) SSHDefenseManagementCapabilities() []contract.Capability {
	readErr := m.sshDefenseManagerAvailability(false)
	writeErr := m.sshDefenseManagerAvailability(true)
	capability := func(id, method string, err error) contract.Capability {
		if err != nil {
			return contract.Capability{ID: id, Enabled: false, Reason: resourceCapabilityReason(err)}
		}
		return contract.Capability{ID: id, Enabled: true, Methods: []string{method}}
	}
	return []contract.Capability{
		capability("system.ssh-defense.read", "GET", readErr),
		capability("system.ssh-defense.write", "POST", writeErr),
	}
}

func (m *Manager) sshDefenseManagerAvailability(write bool) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: SSH defense is only available on Linux", ErrUnsupported)
	}
	if m.effectiveUID() != 0 {
		return fmt.Errorf("%w: Agent must run as root for SSH defense", ErrUnsupported)
	}
	commands := []string{"env", "bash", "awk", "grep", "tail", "tr", "mktemp", "rm", "wc", "sha256sum", "sed", "sort", "od", "python3"}
	if write {
		if !m.enabled {
			return fmt.Errorf("%w: host system writes are disabled", ErrDisabled)
		}
		commands = append(commands, "flock", "cp", "mv", "chmod", "chown")
		if _, err := m.backgroundExecutable(); err != nil {
			return fmt.Errorf("%w: Agent background executor is unavailable", ErrUnsupported)
		}
		if err := m.backgroundJobsAvailable(); err != nil {
			return fmt.Errorf("%w: %v", ErrUnsupported, err)
		}
	}
	for _, command := range commands {
		if _, err := m.runner.LookPath(command); err != nil {
			return fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}
	_, err := m.sshDefenseManagerScriptPath()
	return err
}

func (m *Manager) sshDefenseManagerScriptPath() (string, error) {
	path, err := m.systemResourceScriptPath()
	if err != nil {
		return "", err
	}
	content, err := readResourceFile(path, resourceScriptMaxBytes)
	if err != nil || !trustedKejilionSSHDefenseManagerContent(content) {
		return "", fmt.Errorf("%w: trusted kejilion.sh SSH defense manager protocol v1 was not found", ErrUnsupported)
	}
	return path, nil
}

func trustedKejilionSSHDefenseManagerContent(content []byte) bool {
	value := string(content)
	return trustedKejilionSystemResourceContent(content) &&
		sshDefenseManagerProtocolV1Pattern.Match(content) &&
		strings.Contains(value, "KJ_F2B_NONINTERACTIVE") &&
		strings.Contains(value, "kpanel_f2b_manager_dispatch") &&
		strings.Contains(value, "KPANEL_F2B_MANAGER_STATUS")
}

func (m *Manager) runSSHDefenseManager(ctx context.Context, arguments ...string) ([]byte, error) {
	script, err := m.sshDefenseManagerScriptPath()
	if err != nil {
		return nil, err
	}
	commandArguments := []string{
		"KJ_F2B_NONINTERACTIVE=1", "LC_ALL=C.UTF-8", "LANG=C.UTF-8", "bash", script,
		"f2b", "manager",
	}
	commandArguments = append(commandArguments, arguments...)
	output, _, runErr := m.runResourceCommandInput(ctx, sshDefenseManagerOutputLimit, nil, "env", commandArguments...)
	return output, runErr
}

func (m *Manager) SSHDefenseSnapshot(ctx context.Context) (contract.SSHDefenseSnapshot, error) {
	snapshot, err := m.readSSHDefenseSnapshot(ctx)
	if err != nil {
		return contract.SSHDefenseSnapshot{}, err
	}
	snapshot.Maintenance = m.MaintenanceStatus()
	return snapshot, nil
}

// readSSHDefenseSnapshot reads only Fail2Ban state. Callers that already hold
// m.mu must use this helper because MaintenanceStatus takes the same lock.
func (m *Manager) readSSHDefenseSnapshot(ctx context.Context) (contract.SSHDefenseSnapshot, error) {
	if err := m.sshDefenseManagerAvailability(false); err != nil {
		return contract.SSHDefenseSnapshot{}, err
	}
	readContext, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	output, runErr := m.runSSHDefenseManager(readContext, "status")
	snapshot, parseErr := parseSSHDefenseSnapshot(output)
	if parseErr != nil {
		return contract.SSHDefenseSnapshot{}, fmt.Errorf("%w: invalid SSH defense protocol output: %v", ErrNeedsAttention, parseErr)
	}
	if runErr != nil {
		return contract.SSHDefenseSnapshot{}, fmt.Errorf("%w: kejilion.sh could not read SSH defense: %v", ErrUnsupported, runErr)
	}
	snapshot.ObservedAt = m.now().UTC()
	return snapshot, nil
}

func parseSSHDefenseSnapshot(output []byte) (contract.SSHDefenseSnapshot, error) {
	snapshot := contract.SSHDefenseSnapshot{
		BannedIPs:        []string{},
		TrustedAddresses: []string{},
		RecentEvents:     []contract.SSHDefenseEvent{},
	}
	values := make(map[string]string)
	bannedSeen := make(map[string]bool)
	trustedSeen := make(map[string]bool)
	for _, line := range resourceLines(output) {
		if line == "" || line == "KPANEL_F2B_PROTOCOL 1" || line == "KPANEL_F2B_MANAGER_PROTOCOL 1" {
			continue
		}
		if value, ok := receiptValue(line, "KPANEL_F2B_MANAGER_BAN"); ok {
			if len(snapshot.BannedIPs) >= contract.SSHDefenseMaxBannedIPs || bannedSeen[value] {
				return snapshot, errors.New("banned IP list is invalid")
			}
			if _, err := netip.ParseAddr(value); err != nil {
				return snapshot, errors.New("banned IP is invalid")
			}
			bannedSeen[value] = true
			snapshot.BannedIPs = append(snapshot.BannedIPs, value)
			continue
		}
		if value, ok := receiptValue(line, "KPANEL_F2B_MANAGER_TRUSTED"); ok {
			if len(snapshot.TrustedAddresses) >= contract.SSHDefenseMaxTrustedAddresses || trustedSeen[value] || !validSSHDefenseAddress(value) {
				return snapshot, errors.New("trusted address list is invalid")
			}
			trustedSeen[value] = true
			snapshot.TrustedAddresses = append(snapshot.TrustedAddresses, value)
			continue
		}
		if value, ok := receiptValue(line, "KPANEL_F2B_MANAGER_EVENT_HEX"); ok {
			if len(snapshot.RecentEvents) >= contract.SSHDefenseMaxRecentEvents {
				return snapshot, errors.New("recent event count exceeds protocol limit")
			}
			event, err := parseSSHDefenseEvent(value)
			if err != nil {
				return snapshot, err
			}
			snapshot.RecentEvents = append(snapshot.RecentEvents, event)
			continue
		}
		matched := false
		for _, key := range []string{
			"KPANEL_F2B_MANAGER_STATUS", "KPANEL_F2B_MANAGER_VERSION", "KPANEL_F2B_MANAGER_INSTALLED",
			"KPANEL_F2B_MANAGER_RUNNING", "KPANEL_F2B_MANAGER_ENABLED", "KPANEL_F2B_MANAGER_AUTOSTART",
			"KPANEL_F2B_MANAGER_JAIL", "KPANEL_F2B_MANAGER_CURRENT_FAILED", "KPANEL_F2B_MANAGER_TOTAL_FAILED",
			"KPANEL_F2B_MANAGER_CURRENT_BANNED", "KPANEL_F2B_MANAGER_TOTAL_BANNED", "KPANEL_F2B_MANAGER_BANTIME",
			"KPANEL_F2B_MANAGER_FINDTIME", "KPANEL_F2B_MANAGER_MAXRETRY", "KPANEL_F2B_MANAGER_PROFILE",
			"KPANEL_F2B_MANAGER_BANS_TRUNCATED",
		} {
			if value, ok := receiptValue(line, key); ok {
				if _, exists := values[key]; exists {
					return snapshot, fmt.Errorf("duplicate %s", key)
				}
				values[key], matched = value, true
				break
			}
		}
		if !matched {
			return snapshot, errors.New("unexpected protocol output")
		}
	}
	if values["KPANEL_F2B_MANAGER_STATUS"] != "ok" && values["KPANEL_F2B_MANAGER_STATUS"] != "applied" && values["KPANEL_F2B_MANAGER_STATUS"] != "unchanged" {
		return snapshot, errors.New("snapshot status is invalid")
	}
	if !resourceVersionPattern.MatchString(values["KPANEL_F2B_MANAGER_VERSION"]) {
		return snapshot, errors.New("snapshot version is invalid")
	}
	parseBool := func(key string) (bool, error) {
		switch values[key] {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return false, fmt.Errorf("%s is invalid", key)
		}
	}
	parseNumber := func(key string, maximum int) (int, error) {
		value, err := strconv.Atoi(values[key])
		if err != nil || value < 0 || value > maximum {
			return 0, fmt.Errorf("%s is invalid", key)
		}
		return value, nil
	}
	var err error
	snapshot.ResourceVersion = values["KPANEL_F2B_MANAGER_VERSION"]
	if snapshot.Installed, err = parseBool("KPANEL_F2B_MANAGER_INSTALLED"); err != nil {
		return snapshot, err
	}
	if snapshot.Running, err = parseBool("KPANEL_F2B_MANAGER_RUNNING"); err != nil {
		return snapshot, err
	}
	if snapshot.Enabled, err = parseBool("KPANEL_F2B_MANAGER_ENABLED"); err != nil {
		return snapshot, err
	}
	if snapshot.Autostart, err = parseBool("KPANEL_F2B_MANAGER_AUTOSTART"); err != nil {
		return snapshot, err
	}
	if snapshot.BansTruncated, err = parseBool("KPANEL_F2B_MANAGER_BANS_TRUNCATED"); err != nil {
		return snapshot, err
	}
	snapshot.Jail = values["KPANEL_F2B_MANAGER_JAIL"]
	if !sshDefenseManagerJailPattern.MatchString(snapshot.Jail) {
		return snapshot, errors.New("jail is invalid")
	}
	snapshot.Profile = values["KPANEL_F2B_MANAGER_PROFILE"]
	if snapshot.Profile != "mild" && snapshot.Profile != "standard" && snapshot.Profile != "strict" && snapshot.Profile != "custom" {
		return snapshot, errors.New("profile is invalid")
	}
	if snapshot.CurrentFailed, err = parseNumber("KPANEL_F2B_MANAGER_CURRENT_FAILED", 1_000_000_000); err != nil {
		return snapshot, err
	}
	if snapshot.TotalFailed, err = parseNumber("KPANEL_F2B_MANAGER_TOTAL_FAILED", 1_000_000_000); err != nil {
		return snapshot, err
	}
	if snapshot.CurrentBanned, err = parseNumber("KPANEL_F2B_MANAGER_CURRENT_BANNED", 1_000_000_000); err != nil {
		return snapshot, err
	}
	if snapshot.TotalBanned, err = parseNumber("KPANEL_F2B_MANAGER_TOTAL_BANNED", 1_000_000_000); err != nil {
		return snapshot, err
	}
	if snapshot.BanTimeSeconds, err = parseNumber("KPANEL_F2B_MANAGER_BANTIME", 315_360_000); err != nil {
		return snapshot, err
	}
	if snapshot.FindTimeSeconds, err = parseNumber("KPANEL_F2B_MANAGER_FINDTIME", 315_360_000); err != nil {
		return snapshot, err
	}
	if snapshot.MaxRetry, err = parseNumber("KPANEL_F2B_MANAGER_MAXRETRY", 1_000); err != nil || snapshot.MaxRetry == 0 {
		return snapshot, errors.New("maximum retry is invalid")
	}
	if !snapshot.BansTruncated && snapshot.CurrentBanned != len(snapshot.BannedIPs) {
		return snapshot, errors.New("banned IP count does not match list")
	}
	if snapshot.BansTruncated && snapshot.CurrentBanned <= len(snapshot.BannedIPs) {
		return snapshot, errors.New("truncated banned IP count is inconsistent")
	}
	if !snapshot.Installed && (snapshot.Running || snapshot.Enabled || snapshot.Autostart) {
		return snapshot, errors.New("service state is inconsistent")
	}
	return snapshot, nil
}

func validSSHDefenseAddress(value string) bool {
	if _, err := netip.ParseAddr(value); err == nil {
		return true
	}
	_, err := netip.ParsePrefix(value)
	return err == nil
}

func parseSSHDefenseEvent(value string) (contract.SSHDefenseEvent, error) {
	if value == "" || len(value) > 4096 || len(value)%2 != 0 {
		return contract.SSHDefenseEvent{}, errors.New("event encoding size is invalid")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || strings.ContainsAny(string(decoded), "\x00\r\n") {
		return contract.SSHDefenseEvent{}, errors.New("event encoding is invalid")
	}
	match := sshDefenseManagerEventPattern.FindStringSubmatch(string(decoded))
	if len(match) != 4 {
		return contract.SSHDefenseEvent{}, errors.New("event record is invalid")
	}
	address := strings.TrimRight(match[3], ",.;")
	if _, err := netip.ParseAddr(address); err != nil {
		return contract.SSHDefenseEvent{}, errors.New("event address is invalid")
	}
	return contract.SSHDefenseEvent{OccurredAt: match[1], Action: strings.ToLower(match[2]), Address: address}, nil
}

type sshDefenseManagerReceipt struct {
	Status  string
	Version string
	Backup  string
}

func parseSSHDefenseManagerReceipt(output []byte) (sshDefenseManagerReceipt, error) {
	var receipt sshDefenseManagerReceipt
	seen := make(map[string]bool)
	for _, line := range resourceLines(output) {
		if line == "" || line == "KPANEL_F2B_PROTOCOL 1" || line == "KPANEL_F2B_MANAGER_PROTOCOL 1" {
			continue
		}
		matched := false
		for key, target := range map[string]*string{
			"KPANEL_F2B_MANAGER_STATUS":  &receipt.Status,
			"KPANEL_F2B_MANAGER_VERSION": &receipt.Version,
			"KPANEL_F2B_MANAGER_BACKUP":  &receipt.Backup,
		} {
			if value, ok := receiptValue(line, key); ok {
				if seen[key] {
					return receipt, fmt.Errorf("duplicate %s", key)
				}
				seen[key], matched, *target = true, true, value
				break
			}
		}
		if !matched && !isSSHDefenseManagerSnapshotReceiptLine(line) {
			return receipt, errors.New("unexpected receipt output")
		}
	}
	if receipt.Status == "" || !resourceVersionPattern.MatchString(receipt.Version) {
		return receipt, errors.New("receipt status or version is invalid")
	}
	if receipt.Backup != "" && !sshDefenseManagerRecoveryPattern.MatchString(receipt.Backup) {
		return receipt, errors.New("receipt recovery path is invalid")
	}
	return receipt, nil
}

func isSSHDefenseManagerSnapshotReceiptLine(line string) bool {
	for _, key := range []string{
		"KPANEL_F2B_MANAGER_INSTALLED", "KPANEL_F2B_MANAGER_RUNNING", "KPANEL_F2B_MANAGER_ENABLED",
		"KPANEL_F2B_MANAGER_AUTOSTART", "KPANEL_F2B_MANAGER_JAIL", "KPANEL_F2B_MANAGER_CURRENT_FAILED",
		"KPANEL_F2B_MANAGER_TOTAL_FAILED", "KPANEL_F2B_MANAGER_CURRENT_BANNED", "KPANEL_F2B_MANAGER_TOTAL_BANNED",
		"KPANEL_F2B_MANAGER_BANTIME", "KPANEL_F2B_MANAGER_FINDTIME", "KPANEL_F2B_MANAGER_MAXRETRY",
		"KPANEL_F2B_MANAGER_PROFILE", "KPANEL_F2B_MANAGER_BANS_TRUNCATED", "KPANEL_F2B_MANAGER_BAN",
		"KPANEL_F2B_MANAGER_TRUSTED", "KPANEL_F2B_MANAGER_EVENT_HEX",
	} {
		if _, ok := receiptValue(line, key); ok {
			return true
		}
	}
	return false
}

func sshDefenseManagerInvocation(request contract.SSHDefenseActionRequest) []string {
	arguments := []string{request.Action, request.ExpectedResourceVersion}
	switch request.Action {
	case "set-profile":
		arguments = append(arguments, request.Profile)
	case "add-trusted", "remove-trusted", "unban":
		arguments = append(arguments, request.Address)
	}
	return arguments
}

func (m *Manager) ExecuteSSHDefenseAction(ctx context.Context, request contract.SSHDefenseActionRequest) (contract.SSHDefenseActionResult, error) {
	if field, detail := contract.ValidateSSHDefenseAction(&request); field != "" {
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: %s: %s", ErrInvalidInput, field, detail)
	}
	if err := m.sshDefenseManagerAvailability(true); err != nil {
		return contract.SSHDefenseActionResult{}, err
	}
	lockContext, cancelLock := context.WithTimeout(ctx, resourceWriterLockTimeout)
	if !lockSystemResource(lockContext, &m.mu) {
		cancelLock()
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: timed out waiting for the SSH defense writer", ErrConflict)
	}
	cancelLock()
	defer m.mu.Unlock()
	transactionContext, cancelTransaction := context.WithTimeout(context.WithoutCancel(ctx), resourceActionTimeout)
	defer cancelTransaction()
	current, err := m.readSSHDefenseSnapshot(transactionContext)
	if err != nil {
		return contract.SSHDefenseActionResult{}, err
	}
	if current.ResourceVersion != request.ExpectedResourceVersion {
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: expected resource version is stale", ErrConflict)
	}
	if request.Action == "enable" || request.Action == "disable" || request.Action == "uninstall" {
		changed, message, err := m.startMaintenance(transactionContext, "ssh-defense", request.Action)
		if err != nil {
			return contract.SSHDefenseActionResult{}, err
		}
		return contract.SSHDefenseActionResult{Action: request.Action, Status: "accepted", Changed: changed, Message: message, ResourceVersion: current.ResourceVersion, AppliedAt: m.now().UTC()}, nil
	}
	output, runErr := m.runSSHDefenseManager(transactionContext, sshDefenseManagerInvocation(request)...)
	receipt, parseErr := parseSSHDefenseManagerReceipt(output)
	if parseErr != nil {
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: SSH defense receipt is invalid: %v", ErrNeedsAttention, parseErr)
	}
	switch receipt.Status {
	case "conflict":
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: script detected an SSH defense resource conflict", ErrConflict)
	case "failed":
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: kejilion.sh reported a completed SSH defense rollback", ErrRolledBack)
	case "rollback-failed":
		detail := "kejilion.sh reported SSH defense rollback failure"
		if receipt.Backup != "" {
			detail += "; recovery backup: " + receipt.Backup
		}
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: %s", ErrRollbackFailed, detail)
	case "needs-attention":
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: kejilion.sh could not verify the SSH defense action", ErrNeedsAttention)
	case "applied", "unchanged":
		if runErr != nil {
			return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: script exited after a success receipt: %v", ErrNeedsAttention, runErr)
		}
	default:
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: unsupported SSH defense receipt status", ErrNeedsAttention)
	}
	actual, err := m.readSSHDefenseSnapshot(transactionContext)
	if err != nil {
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: SSH defense post-write readback failed: %v", ErrNeedsAttention, err)
	}
	if actual.ResourceVersion != receipt.Version {
		return contract.SSHDefenseActionResult{}, fmt.Errorf("%w: SSH defense resource version does not match the script receipt", ErrNeedsAttention)
	}
	changed := receipt.Status == "applied"
	status, message := "succeeded", "SSH defense action applied by kejilion.sh"
	if !changed {
		status, message = "unchanged", "SSH defense already matched the requested state"
	}
	return contract.SSHDefenseActionResult{Action: request.Action, Status: status, Changed: changed, Message: message, BackupPath: receipt.Backup, ResourceVersion: actual.ResourceVersion, AppliedAt: m.now().UTC()}, nil
}
