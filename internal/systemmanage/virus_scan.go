package systemmanage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	virusScanOutputLimit  = 64 << 10
	virusScanReportLimit  = 256 << 10
	virusScanFindingLimit = 200
)

var virusScanProtocolV1Pattern = regexp.MustCompile(`(?m)^KPANEL_VIRUS_SCAN_PROTOCOL_VERSION="1"\r?$`)

type virusScanReceipt struct {
	Status        string
	Mode          string
	ScannedFiles  uint64
	InfectedFiles uint64
	Errors        uint64
}

type virusScanResultState struct {
	Mode             string    `json:"mode"`
	Paths            []string  `json:"paths,omitempty"`
	Status           string    `json:"status"`
	ReportModifiedAt time.Time `json:"reportModifiedAt"`
	CompletedAt      time.Time `json:"completedAt"`
}

func (m *Manager) VirusScanCapabilities() []contract.Capability {
	readErr := m.virusScanAvailability(false)
	writeErr := m.virusScanAvailability(true)
	capability := func(id, method string, err error) contract.Capability {
		if err != nil {
			return contract.Capability{ID: id, Enabled: false, Reason: resourceCapabilityReason(err)}
		}
		return contract.Capability{ID: id, Enabled: true, Methods: []string{method}}
	}
	return []contract.Capability{
		capability("system.virus-scan.read", "GET", readErr),
		capability("system.virus-scan.write", "POST", writeErr),
	}
}

func (m *Manager) virusScanAvailability(write bool) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: virus scanning is only available on Linux", ErrUnsupported)
	}
	if m.effectiveUID() != 0 {
		return fmt.Errorf("%w: Agent must run as root for virus scanning", ErrUnsupported)
	}
	if !write {
		return nil
	}
	if _, err := m.virusScanScriptPath(); err != nil {
		return err
	}
	if !m.enabled {
		return fmt.Errorf("%w: host system writes are disabled", ErrDisabled)
	}
	for _, command := range []string{"env", "bash", "docker", "flock", "install"} {
		if _, err := m.runner.LookPath(command); err != nil {
			return fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}
	if _, err := m.backgroundExecutable(); err != nil {
		return fmt.Errorf("%w: Agent background executor is unavailable", ErrUnsupported)
	}
	if err := m.backgroundJobsAvailable(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return nil
}

func (m *Manager) virusScanScriptPath() (string, error) {
	path, err := m.systemResourceScriptPath()
	if err != nil {
		return "", err
	}
	content, err := readResourceFile(path, resourceScriptMaxBytes)
	if err != nil || !trustedKejilionVirusScanContent(content) {
		return "", fmt.Errorf("%w: trusted kejilion.sh virus-scan protocol v1 was not found", ErrUnsupported)
	}
	return path, nil
}

func trustedKejilionVirusScanContent(content []byte) bool {
	value := string(content)
	return trustedKejilionSystemResourceContent(content) &&
		virusScanProtocolV1Pattern.Match(content) &&
		strings.Contains(value, "KJ_VIRUS_SCAN_NONINTERACTIVE") &&
		strings.Contains(value, "kpanel_virus_scan_dispatch")
}

func encodeVirusScanPolicy(input contract.VirusScanActionRequest) (string, error) {
	if input.Mode != contract.VirusScanModeCustom {
		return input.Mode, nil
	}
	data, err := json.Marshal(input.Paths)
	if err != nil {
		return "", err
	}
	return "custom." + base64.RawURLEncoding.EncodeToString(data), nil
}

func parseVirusScanPolicy(policy string) (string, []string, bool) {
	if policy == contract.VirusScanModeFull || policy == contract.VirusScanModeImportant {
		return policy, nil, true
	}
	encoded, ok := strings.CutPrefix(policy, "custom.")
	if !ok || encoded == "" || len(encoded) > 8*contract.VirusScanMaxPathBytes*2 {
		return "", nil, false
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", nil, false
	}
	input := contract.VirusScanActionRequest{Mode: contract.VirusScanModeCustom}
	if json.Unmarshal(data, &input.Paths) != nil {
		return "", nil, false
	}
	if field, _ := contract.ValidateVirusScanAction(&input); field != "" {
		return "", nil, false
	}
	return input.Mode, input.Paths, true
}

func virusScanHostPaths(mode string, paths []string) []string {
	switch mode {
	case contract.VirusScanModeFull:
		return []string{"/"}
	case contract.VirusScanModeImportant:
		return []string{"/etc", "/var", "/usr", "/home", "/root"}
	default:
		return append([]string(nil), paths...)
	}
}

func (m *Manager) ExecuteVirusScanAction(ctx context.Context, input contract.VirusScanActionRequest) (contract.VirusScanActionResult, error) {
	if field, detail := contract.ValidateVirusScanAction(&input); field != "" {
		return contract.VirusScanActionResult{}, fmt.Errorf("%w: %s: %s", ErrInvalidInput, field, detail)
	}
	if err := m.virusScanAvailability(true); err != nil {
		return contract.VirusScanActionResult{}, err
	}
	for _, path := range input.Paths {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return contract.VirusScanActionResult{}, fmt.Errorf("%w: scan path is not an available directory: %s", ErrInvalidInput, path)
		}
	}
	policy, err := encodeVirusScanPolicy(input)
	if err != nil {
		return contract.VirusScanActionResult{}, fmt.Errorf("%w: encode scan request", ErrInvalidInput)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, message, taskID, err := m.startMaintenanceTask(ctx, "virus-scan", policy)
	if err != nil {
		return contract.VirusScanActionResult{}, err
	}
	return contract.VirusScanActionResult{
		Status: "accepted", TaskID: taskID, Mode: input.Mode,
		Paths: append([]string(nil), input.Paths...), Message: message, AppliedAt: m.now().UTC(),
	}, nil
}

func (m *Manager) runVirusScan(ctx context.Context, arguments ...string) (virusScanReceipt, error) {
	script, err := m.virusScanScriptPath()
	if err != nil {
		return virusScanReceipt{}, err
	}
	commandArguments := []string{
		"KJ_VIRUS_SCAN_NONINTERACTIVE=1", "LC_ALL=C.UTF-8", "LANG=C.UTF-8", "bash", script,
		"kpanel", "virus-scan",
	}
	commandArguments = append(commandArguments, arguments...)
	output, _, runErr := m.runResourceCommandInput(ctx, virusScanOutputLimit, nil, "env", commandArguments...)
	receipt, parseErr := parseVirusScanReceipt(output)
	if parseErr != nil {
		if runErr != nil {
			return receipt, fmt.Errorf("%v; invalid virus scan receipt: %w", runErr, parseErr)
		}
		return receipt, parseErr
	}
	if runErr != nil {
		return receipt, runErr
	}
	if len(arguments) >= 2 && arguments[0] == "scan" && receipt.Mode != arguments[1] {
		return receipt, fmt.Errorf("virus scan completion receipt mode %q does not match request %q", receipt.Mode, arguments[1])
	}
	return receipt, nil
}

func parseVirusScanReceipt(output []byte) (virusScanReceipt, error) {
	result := virusScanReceipt{}
	seenProtocol := false
	values := make(map[string]string)
	for _, line := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "KPANEL_VIRUS_SCAN_PROTOCOL 1" {
			seenProtocol = true
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.HasPrefix(key, "KPANEL_VIRUS_SCAN_") {
			values[key] = value
		}
	}
	if !seenProtocol {
		return result, errors.New("virus scan protocol header is missing")
	}
	result.Status = values["KPANEL_VIRUS_SCAN_STATUS"]
	result.Mode = values["KPANEL_VIRUS_SCAN_MODE"]
	if result.Status == "" {
		return result, errors.New("virus scan status is missing")
	}
	var err error
	if result.ScannedFiles, err = strconv.ParseUint(defaultZero(values["KPANEL_VIRUS_SCAN_SCANNED"]), 10, 64); err != nil {
		return result, errors.New("virus scan scanned count is invalid")
	}
	if result.InfectedFiles, err = strconv.ParseUint(defaultZero(values["KPANEL_VIRUS_SCAN_INFECTED"]), 10, 64); err != nil {
		return result, errors.New("virus scan infected count is invalid")
	}
	if result.Errors, err = strconv.ParseUint(defaultZero(values["KPANEL_VIRUS_SCAN_ERRORS"]), 10, 64); err != nil {
		return result, errors.New("virus scan error count is invalid")
	}
	return result, nil
}

func defaultZero(value string) string {
	if value == "" {
		return "0"
	}
	return value
}

func (m *Manager) virusScanResultPath() string {
	return filepath.Join(m.stateDir, "virus-scan-result.json")
}

func (m *Manager) writeVirusScanResult(mode string, paths []string, status string) error {
	info, err := os.Lstat(m.virusScanReport)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("virus scan report is unavailable after completion")
	}
	state := virusScanResultState{
		Mode: mode, Paths: virusScanHostPaths(mode, paths), Status: status,
		ReportModifiedAt: info.ModTime().UTC(), CompletedAt: m.now().UTC(),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeAtomic(m.virusScanResultPath(), data, 0o600)
}

func (m *Manager) readVirusScanResult() virusScanResultState {
	var state virusScanResultState
	data, err := os.ReadFile(m.virusScanResultPath())
	if err != nil || len(data) > 16<<10 || json.Unmarshal(data, &state) != nil {
		return virusScanResultState{}
	}
	return state
}

func (m *Manager) VirusScanSnapshot(ctx context.Context) (contract.VirusScanSnapshot, error) {
	if err := m.virusScanAvailability(false); err != nil {
		return contract.VirusScanSnapshot{}, err
	}
	snapshot := contract.VirusScanSnapshot{
		ReportPath: m.virusScanReport, Status: "never", Paths: []string{}, Findings: []string{},
		ObservedAt: m.now().UTC(), Maintenance: m.MaintenanceStatus(),
	}
	if err := ctx.Err(); err != nil {
		return contract.VirusScanSnapshot{}, err
	}
	pathInfo, err := os.Lstat(m.virusScanReport)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return contract.VirusScanSnapshot{}, fmt.Errorf("%w: inspect virus scan report: %v", ErrUnsupported, err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return contract.VirusScanSnapshot{}, fmt.Errorf("%w: virus scan report is not a regular file", ErrNeedsAttention)
	}
	file, err := os.Open(m.virusScanReport)
	if err != nil {
		return contract.VirusScanSnapshot{}, fmt.Errorf("%w: open virus scan report: %v", ErrUnsupported, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return contract.VirusScanSnapshot{}, fmt.Errorf("%w: virus scan report is not a regular file", ErrNeedsAttention)
	}
	start := int64(0)
	if info.Size() > virusScanReportLimit {
		start = info.Size() - virusScanReportLimit
		snapshot.Truncated = true
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return contract.VirusScanSnapshot{}, fmt.Errorf("%w: read virus scan report: %v", ErrUnsupported, err)
	}
	data, err := io.ReadAll(io.LimitReader(file, virusScanReportLimit+1))
	if err != nil {
		return contract.VirusScanSnapshot{}, fmt.Errorf("%w: read virus scan report: %v", ErrUnsupported, err)
	}
	if len(data) > virusScanReportLimit {
		data = data[:virusScanReportLimit]
		snapshot.Truncated = true
	}
	if start > 0 {
		if newline := strings.IndexByte(string(data), '\n'); newline >= 0 {
			data = data[newline+1:]
		}
	}
	snapshot.ReportAvailable = true
	completedAt := info.ModTime().UTC()
	snapshot.CompletedAt = &completedAt
	state := m.readVirusScanResult()
	if !state.ReportModifiedAt.IsZero() && state.ReportModifiedAt.Equal(completedAt) {
		snapshot.Source = "kpanel"
		snapshot.Mode = state.Mode
		snapshot.Paths = append([]string(nil), state.Paths...)
	} else {
		snapshot.Source = "script"
	}
	parseVirusScanReport(&snapshot, string(data))
	if snapshot.Maintenance.State == "running" && snapshot.Maintenance.Action == "virus-scan" {
		snapshot.Status = "running"
	}
	return snapshot, nil
}

func parseVirusScanReport(snapshot *contract.VirusScanSnapshot, report string) {
	sawSummary := false
	for _, raw := range strings.Split(strings.ReplaceAll(report, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "Scanned files:") {
			snapshot.ScannedFiles = parseReportCount(line)
			sawSummary = true
		} else if strings.HasPrefix(line, "Infected files:") {
			snapshot.InfectedFiles = parseReportCount(line)
			sawSummary = true
		} else if strings.HasPrefix(line, "Total errors:") {
			snapshot.Errors = parseReportCount(line)
			sawSummary = true
		}
		if strings.HasSuffix(line, " FOUND") {
			if len(snapshot.Findings) >= virusScanFindingLimit {
				snapshot.Truncated = true
				continue
			}
			if len(line) > 2048 {
				line = line[:2048]
				snapshot.Truncated = true
			}
			snapshot.Findings = append(snapshot.Findings, mapVirusScanFinding(line, snapshot.Paths))
		}
	}
	if sawSummary {
		if snapshot.InfectedFiles > 0 {
			snapshot.Status = "infected"
		} else if snapshot.Errors > 0 {
			snapshot.Status = "completed-with-errors"
		} else {
			snapshot.Status = "clean"
		}
	} else {
		snapshot.Status = "unknown"
	}
}

func parseReportCount(line string) uint64 {
	_, value, ok := strings.Cut(line, ":")
	if !ok {
		return 0
	}
	count, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return count
}

func mapVirusScanFinding(line string, paths []string) string {
	findingPath, detail, hasDetail := strings.Cut(line, ": ")
	for index, hostPath := range paths {
		containerPath := fmt.Sprintf("/mnt/scan/%d", index)
		if strings.HasPrefix(findingPath, containerPath+"/") || findingPath == containerPath {
			suffix := strings.TrimPrefix(findingPath, containerPath)
			mapped := strings.TrimSuffix(hostPath, "/") + suffix
			if mapped == "" {
				mapped = "/"
			}
			if hasDetail {
				return mapped + ": " + detail
			}
			return mapped
		}
	}
	return line
}
