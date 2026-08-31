package systemmanage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/redact"
)

const (
	systemLogFileTailBytes int64 = 1 << 20
	systemLogUsageTimeout        = 2 * time.Second
	sshLoginCacheTTL             = 15 * time.Second
)

var (
	journalDiskUsagePattern = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([kmgtpe]?)(?:i?b)?`)
	sshAcceptedPattern      = regexp.MustCompile(`(?i)\bAccepted\s+([^\s]+)\s+for\s+([^\s]+)\s+from\s+([^\s]+)`)
	sshLoginTokenPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+:/-]{0,127}$`)
	sshLoginAddressPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.:%_-]{0,252}$`)
)

// SystemLogCapabilities keeps read access independent from the host-write
// switch. Cleanup still uses the existing durable maintenance executor.
func (m *Manager) SystemLogCapabilities() []contract.Capability {
	readCapability := contract.Capability{ID: "system.logs.read"}
	writeCapability := contract.Capability{ID: "system.logs.write"}
	if runtime.GOOS != "linux" {
		readCapability.Reason = "系统日志仅支持 Linux"
		writeCapability.Reason = "系统日志清理仅支持 Linux"
		return []contract.Capability{readCapability, writeCapability}
	}
	if m.effectiveUID() != 0 {
		readCapability.Reason = "Agent 必须以受限 root 服务运行"
		writeCapability.Reason = readCapability.Reason
		return []contract.Capability{readCapability, writeCapability}
	}

	_, journalErr := m.runner.LookPath("journalctl")
	_, lastErr := m.runner.LookPath("last")
	_, duErr := m.runner.LookPath("du")
	_, authPath, _ := m.fixedAuthLog()
	if journalErr == nil || lastErr == nil || duErr == nil || authPath != "" {
		readCapability.Enabled = true
		readCapability.Methods = []string{"GET"}
	} else {
		readCapability.Reason = "journalctl、last 与系统日志文件均不可用"
	}

	if !m.enabled {
		writeCapability.Reason = "宿主机系统写入开关未启用"
		return []contract.Capability{readCapability, writeCapability}
	}
	if _, err := m.runner.LookPath("systemd-run"); err != nil {
		writeCapability.Reason = "systemd 后台任务执行器不可用"
		return []contract.Capability{readCapability, writeCapability}
	}
	if journalErr != nil {
		writeCapability.Reason = "journalctl 不可用"
		return []contract.Capability{readCapability, writeCapability}
	}
	if _, err := m.backgroundExecutable(); err != nil {
		writeCapability.Reason = "Agent 后台执行程序不可用，请更新或重新安装 KPanel"
		return []contract.Capability{readCapability, writeCapability}
	}
	writeCapability.Enabled = true
	writeCapability.Methods = []string{"POST"}
	return []contract.Capability{readCapability, writeCapability}
}

func (m *Manager) SystemLogSummary(ctx context.Context) (contract.SystemLogSummary, error) {
	result := contract.SystemLogSummary{
		ObservedAt: m.now().UTC(),
	}
	if runtime.GOOS != "linux" {
		return result, fmt.Errorf("%w: system logs are only available on Linux", ErrUnsupported)
	}
	if m.effectiveUID() != 0 {
		return result, fmt.Errorf("%w: Agent must run as root to read system logs", ErrUnsupported)
	}

	result.Maintenance = m.MaintenanceStatus()
	varLogContext, cancelVarLog := context.WithTimeout(ctx, systemLogUsageTimeout)
	result.VarLog = m.varLogUsage(varLogContext)
	cancelVarLog()
	journalContext, cancelJournal := context.WithTimeout(ctx, systemLogUsageTimeout)
	result.Journal = m.journalUsage(journalContext)
	cancelJournal()
	if _, err := m.runner.LookPath("journalctl"); err == nil {
		result.Sources.Journal.Available = true
	} else {
		result.Sources.Journal.Reason = "journalctl 不可用"
	}

	if _, err := m.runner.LookPath("last"); err == nil {
		result.Sources.Login.Available = true
	} else {
		result.Sources.Login.Reason = "last 命令不可用"
	}

	_, fixedAuthPath, _ := m.fixedAuthLog()
	if result.Sources.Journal.Available {
		result.Sources.Security.Available = true
		result.AuthSource = "journal"
	} else if fixedAuthPath != "" {
		result.Sources.Security.Available = true
		result.AuthSource = fixedAuthPath
	} else {
		result.Sources.Security.Reason = "journal 与固定认证日志文件均不可用"
	}

	return result, nil
}

func (m *Manager) varLogUsage(ctx context.Context) contract.SystemLogUsage {
	if _, err := m.runner.LookPath("du"); err != nil {
		return contract.SystemLogUsage{Reason: "du 命令不可用"}
	}
	output, _, err := m.runResourceCommand(ctx, contract.SystemLogMaxOutputBytes, "du", "-skx", "--", m.logRoot)
	if err != nil {
		return contract.SystemLogUsage{Reason: "无法统计 /var/log 用量"}
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return contract.SystemLogUsage{Reason: "du 未返回日志用量"}
	}
	valueKiB, parseErr := strconv.ParseUint(fields[0], 10, 64)
	if parseErr != nil || valueKiB > math.MaxUint64/1024 {
		return contract.SystemLogUsage{Reason: "du 返回的日志用量无效"}
	}
	return contract.SystemLogUsage{Available: true, Bytes: valueKiB * 1024}
}

func (m *Manager) journalUsage(ctx context.Context) contract.SystemLogUsage {
	if _, err := m.runner.LookPath("journalctl"); err != nil {
		return contract.SystemLogUsage{Reason: "journalctl 不可用"}
	}
	output, _, err := m.runResourceCommand(
		ctx,
		contract.SystemLogMaxOutputBytes,
		"journalctl",
		"--disk-usage",
		"--no-pager",
	)
	if err != nil {
		return contract.SystemLogUsage{Reason: "无法读取 journal 用量"}
	}
	value, ok := parseJournalDiskUsage(string(output))
	if !ok {
		return contract.SystemLogUsage{Reason: "journalctl 未返回可识别的用量"}
	}
	return contract.SystemLogUsage{Available: true, Bytes: value}
}

func parseJournalDiskUsage(value string) (uint64, bool) {
	matches := journalDiskUsagePattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return 0, false
	}
	match := matches[len(matches)-1]
	number, err := strconv.ParseFloat(match[1], 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return 0, false
	}
	multiplier := float64(1)
	switch strings.ToUpper(match[2]) {
	case "K":
		multiplier = 1 << 10
	case "M":
		multiplier = 1 << 20
	case "G":
		multiplier = 1 << 30
	case "T":
		multiplier = 1 << 40
	case "P":
		multiplier = 1 << 50
	case "E":
		multiplier = 1 << 60
	}
	bytes := number * multiplier
	if bytes > math.MaxUint64 {
		return 0, false
	}
	return uint64(math.Round(bytes)), true
}

func (m *Manager) SystemLogs(ctx context.Context, query contract.SystemLogQuery) (contract.SystemLogSnapshot, error) {
	result := contract.SystemLogSnapshot{
		Source:  query.Source,
		Entries: make([]contract.SystemLogEntry, 0), ObservedAt: m.now().UTC(),
	}
	if !contract.ValidSystemLogQuery(query) {
		return result, fmt.Errorf("%w: invalid system log query", ErrInvalidInput)
	}
	if runtime.GOOS != "linux" || m.effectiveUID() != 0 {
		return result, fmt.Errorf("%w: system logs are unavailable on this host", ErrUnsupported)
	}
	switch query.Source {
	case "system", "service":
		entries, truncated, err := m.readJournalLogs(ctx, query, false)
		result.Entries, result.Truncated = entries, truncated
		return result, err
	case "security":
		entries, truncated, journalErr := m.readJournalLogs(ctx, query, true)
		if journalErr == nil && len(entries) > 0 {
			result.Entries, result.Truncated, result.AuthSource = entries, truncated, "journal"
			return result, nil
		}
		if journalErr != nil && ctx.Err() != nil {
			return result, journalErr
		}
		path, fallbackEntries, fallbackTruncated, fileErr := m.readFixedAuthLog(query.Limit)
		if fileErr == nil {
			result.Entries, result.Truncated, result.AuthSource = fallbackEntries, fallbackTruncated, path
			return result, nil
		}
		if journalErr == nil {
			result.AuthSource = "journal"
			return result, nil
		}
		return result, fmt.Errorf("%w: journal and fixed authentication logs are unavailable: %w", ErrUnsupported, journalErr)
	case "login":
		entries, truncated, err := m.readLoginLogs(ctx, query.Limit)
		result.Entries, result.Truncated = entries, truncated
		return result, err
	default:
		return result, fmt.Errorf("%w: unknown system log source", ErrInvalidInput)
	}
}

// LatestSSHLogin returns only the newest accepted SSH authentication event.
// The full authentication log remains behind the dedicated system-log API;
// telemetry carries this bounded event so the cluster controller can detect a
// new login without receiving host configuration or arbitrary log text.
func (m *Manager) LatestSSHLogin(ctx context.Context) (*contract.SSHLoginEvent, error) {
	if runtime.GOOS != "linux" || m.effectiveUID() != 0 {
		return nil, fmt.Errorf("%w: SSH login events are unavailable on this host", ErrUnsupported)
	}
	m.sshLoginMu.Lock()
	defer m.sshLoginMu.Unlock()
	now := m.now().UTC()
	if !m.sshLoginCheckedAt.IsZero() && now.Sub(m.sshLoginCheckedAt) >= 0 &&
		now.Sub(m.sshLoginCheckedAt) < sshLoginCacheTTL {
		return cloneSSHLoginEvent(m.sshLoginCache), nil
	}
	entries, _, journalErr := m.readJournalLogs(ctx, contract.SystemLogQuery{
		Source: "security", Limit: 50, Priority: "all",
	}, true)
	if journalErr == nil {
		m.sshLoginCache = latestSSHLogin(entries, now)
		m.sshLoginCheckedAt = now
		return cloneSSHLoginEvent(m.sshLoginCache), nil
	}
	_, entries, _, fileErr := m.readFixedAuthLog(50)
	if fileErr != nil {
		return nil, errors.Join(journalErr, fileErr)
	}
	m.sshLoginCache = latestSSHLogin(entries, now)
	m.sshLoginCheckedAt = now
	return cloneSSHLoginEvent(m.sshLoginCache), nil
}

func cloneSSHLoginEvent(event *contract.SSHLoginEvent) *contract.SSHLoginEvent {
	if event == nil {
		return nil
	}
	clone := *event
	return &clone
}

func latestSSHLogin(entries []contract.SystemLogEntry, observedAt time.Time) *contract.SSHLoginEvent {
	for index := len(entries) - 1; index >= 0; index-- {
		if event, ok := parseSSHLoginEntry(entries[index], observedAt); ok {
			return &event
		}
	}
	return nil
}

func parseSSHLoginEntry(entry contract.SystemLogEntry, observedAt time.Time) (contract.SSHLoginEvent, bool) {
	identity := strings.ToLower(entry.Identifier + " " + entry.Unit)
	if !strings.Contains(identity, "sshd") && !strings.Contains(strings.ToLower(entry.Message), "sshd") {
		return contract.SSHLoginEvent{}, false
	}
	match := sshAcceptedPattern.FindStringSubmatch(entry.Message)
	if len(match) != 4 || !sshLoginTokenPattern.MatchString(match[1]) ||
		!sshLoginTokenPattern.MatchString(match[2]) || !sshLoginAddressPattern.MatchString(match[3]) {
		return contract.SSHLoginEvent{}, false
	}
	occurredAt := observedAt.UTC()
	if entry.Timestamp != nil {
		occurredAt = entry.Timestamp.UTC()
	}
	id := strings.TrimSpace(entry.Cursor)
	if id == "" {
		hash := sha256.Sum256([]byte(strings.Join([]string{
			occurredAt.Format(time.RFC3339Nano), match[1], match[2], match[3], entry.Message,
		}, "\x00")))
		id = "sha256:" + hex.EncodeToString(hash[:])
	}
	event := contract.SSHLoginEvent{
		ID: id, OccurredAt: occurredAt, Method: match[1],
		Username: match[2], RemoteAddress: match[3],
	}
	if !contract.ValidSSHLoginEvent(event) {
		return contract.SSHLoginEvent{}, false
	}
	return event, true
}

func (m *Manager) readJournalLogs(
	ctx context.Context,
	query contract.SystemLogQuery,
	authFacilities bool,
) ([]contract.SystemLogEntry, bool, error) {
	if _, err := m.runner.LookPath("journalctl"); err != nil {
		return nil, false, fmt.Errorf("%w: journalctl is unavailable", ErrUnsupported)
	}
	arguments := []string{
		"--no-pager",
		"--quiet",
		"--reverse",
		"--lines=" + strconv.Itoa(query.Limit+1),
		"--output=json",
		"--output-fields=__CURSOR,__REALTIME_TIMESTAMP,MESSAGE,PRIORITY,_SYSTEMD_UNIT,UNIT,OBJECT_SYSTEMD_UNIT,COREDUMP_UNIT,SYSLOG_IDENTIFIER,_COMM,_PID,SYSLOG_PID",
	}
	if query.Source == "service" {
		arguments = append(arguments, "--unit=*.service")
	}
	switch query.Priority {
	case "warning":
		arguments = append(arguments, "--priority=0..4")
	case "error":
		arguments = append(arguments, "--priority=0..3")
	case "all":
	default:
		return nil, false, fmt.Errorf("%w: invalid journal priority", ErrInvalidInput)
	}
	if authFacilities {
		arguments = append(arguments, "SYSLOG_FACILITY=4", "SYSLOG_FACILITY=10")
	}
	output, _, err := m.runResourceCommand(ctx, contract.SystemLogMaxOutputBytes, "journalctl", arguments...)
	outputTruncated := errors.Is(err, errResourceOutputTooLarge)
	if err != nil && !outputTruncated {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, false, ctxErr
		}
		return nil, false, fmt.Errorf("%w: read journal: %w", ErrUnsupported, err)
	}
	entries, invalid := parseJournalEntries(output)
	if invalid && len(entries) == 0 && len(strings.TrimSpace(string(output))) > 0 {
		return nil, false, fmt.Errorf("%w: journalctl returned invalid JSON", ErrUnsupported)
	}
	truncated := outputTruncated || invalid || len(entries) > query.Limit
	reverseSystemLogEntries(entries)
	redactSystemLogEntries(entries)
	if len(entries) > query.Limit {
		entries = entries[len(entries)-query.Limit:]
	}
	return entries, truncated, nil
}

func parseJournalEntries(output []byte) ([]contract.SystemLogEntry, bool) {
	lines := strings.Split(string(output), "\n")
	entries := make([]contract.SystemLogEntry, 0, len(lines))
	invalid := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			invalid = true
			continue
		}
		entry := contract.SystemLogEntry{
			Cursor:     sanitizeLogText(journalString(fields["__CURSOR"]), 4096),
			Priority:   journalPriority(journalString(fields["PRIORITY"])),
			Unit:       sanitizeLogText(journalString(fields["_SYSTEMD_UNIT"]), 255),
			Identifier: sanitizeLogText(journalString(fields["SYSLOG_IDENTIFIER"]), 256),
			Message:    normalizeLogText(journalString(fields["MESSAGE"]), contract.SystemLogMaxMessageBytes),
		}
		if entry.Unit == "" {
			for _, field := range []string{"UNIT", "OBJECT_SYSTEMD_UNIT", "COREDUMP_UNIT"} {
				if entry.Unit = sanitizeLogText(journalString(fields[field]), 255); entry.Unit != "" {
					break
				}
			}
		}
		if entry.Identifier == "" {
			entry.Identifier = sanitizeLogText(journalString(fields["_COMM"]), 256)
		}
		pidValue := journalString(fields["_PID"])
		if pidValue == "" {
			pidValue = journalString(fields["SYSLOG_PID"])
		}
		if pid, err := strconv.Atoi(pidValue); err == nil && pid > 0 {
			entry.PID = pid
		}
		if micros, err := strconv.ParseInt(journalString(fields["__REALTIME_TIMESTAMP"]), 10, 64); err == nil &&
			micros >= 0 && micros <= math.MaxInt64/int64(time.Microsecond) {
			timestamp := time.Unix(0, micros*int64(time.Microsecond)).UTC()
			entry.Timestamp = &timestamp
		}
		entries = append(entries, entry)
	}
	return entries, invalid
}

func journalString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	var values []string
	if json.Unmarshal(raw, &values) == nil && len(values) > 0 {
		return values[0]
	}
	var bytes []byte
	if json.Unmarshal(raw, &bytes) == nil {
		return string(bytes)
	}
	return ""
}

func journalPriority(value string) string {
	priorities := map[string]string{
		"0": "emergency", "1": "alert", "2": "critical", "3": "error",
		"4": "warning", "5": "notice", "6": "info", "7": "debug",
	}
	return priorities[value]
}

func (m *Manager) readLoginLogs(ctx context.Context, limit int) ([]contract.SystemLogEntry, bool, error) {
	if _, err := m.runner.LookPath("last"); err != nil {
		return nil, false, fmt.Errorf("%w: last is unavailable", ErrUnsupported)
	}
	output, _, err := m.runResourceCommand(
		ctx,
		contract.SystemLogMaxOutputBytes,
		"last",
		"-n",
		strconv.Itoa(limit+1),
	)
	outputTruncated := errors.Is(err, errResourceOutputTooLarge)
	if err != nil && !outputTruncated {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, false, ctxErr
		}
		return nil, false, fmt.Errorf("%w: read login history: %w", ErrUnsupported, err)
	}
	entries := make([]contract.SystemLogEntry, 0, limit+1)
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "wtmp begins") || strings.HasPrefix(line, "btmp begins") {
			continue
		}
		entries = append(entries, contract.SystemLogEntry{
			Identifier: "last",
			Message:    normalizeLogText(line, contract.SystemLogMaxMessageBytes),
		})
	}
	truncated := outputTruncated || len(entries) > limit
	reverseSystemLogEntries(entries)
	redactSystemLogEntries(entries)
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, truncated, nil
}

func (m *Manager) fixedAuthLog() (string, string, error) {
	for _, name := range []string{"secure", "auth.log"} {
		path := filepath.Join(m.logRoot, name)
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return name, path, nil
		}
	}
	return "", "", os.ErrNotExist
}

func (m *Manager) readFixedAuthLog(limit int) (string, []contract.SystemLogEntry, bool, error) {
	_, path, err := m.fixedAuthLog()
	if err != nil {
		return "", nil, false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", nil, false, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return "", nil, false, os.ErrInvalid
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, current) || current.Mode()&os.ModeSymlink != 0 {
		return "", nil, false, os.ErrInvalid
	}
	start := int64(0)
	truncatedByBytes := false
	if opened.Size() > systemLogFileTailBytes {
		start = opened.Size() - systemLogFileTailBytes
		truncatedByBytes = true
	}
	if _, err := file.Seek(start, 0); err != nil {
		return "", nil, false, err
	}
	buffer, err := io.ReadAll(io.LimitReader(file, systemLogFileTailBytes))
	if err != nil {
		return "", nil, false, err
	}
	lines := strings.Split(string(buffer), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
	}
	values := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			values = append(values, line)
		}
	}
	truncated := truncatedByBytes || len(values) > limit
	entries := make([]contract.SystemLogEntry, 0, len(values))
	for _, value := range values {
		hash := sha256.Sum256([]byte(strings.Join([]string{
			path, value,
		}, "\x00")))
		entries = append(entries, contract.SystemLogEntry{
			Cursor:     "sha256:" + hex.EncodeToString(hash[:]),
			Identifier: filepath.Base(path),
			Message:    normalizeLogText(value, contract.SystemLogMaxMessageBytes),
		})
	}
	redactSystemLogEntries(entries)
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return path, entries, truncated, nil
}

func sanitizeLogText(value string, limit int) string {
	return normalizeLogText(redact.Text(value), limit)
}

func normalizeLogText(value string, limit int) string {
	value = strings.ToValidUTF8(value, "�")
	value = strings.Map(func(character rune) rune {
		if character == '\n' || character == '\t' || character >= ' ' {
			return character
		}
		return -1
	}, value)
	return truncateUTF8(value, limit)
}

func redactSystemLogEntries(entries []contract.SystemLogEntry) {
	messages := make([]string, len(entries))
	for index := range entries {
		messages[index] = entries[index].Message
	}
	redacted := redact.Records(messages)
	for index := range entries {
		entries[index].Message = normalizeLogText(
			redacted[index],
			contract.SystemLogMaxMessageBytes,
		)
	}
}

func reverseSystemLogEntries(entries []contract.SystemLogEntry) {
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
}

func truncateUTF8(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value
}

func validSystemLogCleanupRequest(input contract.SystemActionRequest) bool {
	return input.Action == "log-cleanup" &&
		(input.MaintenancePolicy == "retain-7d" || input.MaintenancePolicy == "retain-3d" || input.MaintenancePolicy == "max-500m") &&
		input.Hostname == "" && input.Port == 0 && len(input.Servers) == 0 && input.Timezone == "" &&
		input.SwapSizeMiB == 0 && input.MirrorPreset == "" && input.Preference == "" && input.Profile == "" &&
		len(input.Confirmation) == 0 && input.Enabled == nil && input.PID == 0 && input.StartTimeTicks == 0 && input.Signal == ""
}
