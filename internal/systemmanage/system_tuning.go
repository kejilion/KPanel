package systemmanage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	systemTuningOutputLimit = 1 << 20
	systemTuningLogLimit    = 1 << 20
)

type systemTuningCommandRunner interface {
	RunSystemTuning(context.Context, int, int, string, ...string) ([]byte, []byte, error)
}

func (runner commandRunner) RunSystemTuning(
	ctx context.Context,
	outputLimit int,
	logLimit int,
	name string,
	arguments ...string,
) ([]byte, []byte, error) {
	command := exec.CommandContext(ctx, name, arguments...)
	command.Env = append(
		os.Environ(),
		"LC_ALL=C",
		"LANG=C",
		"DEBIAN_FRONTEND=noninteractive",
		"NEEDRESTART_MODE=a",
		"APT_LISTCHANGES_FRONTEND=none",
	)
	stdout := &boundedResourceBuffer{limit: outputLimit}
	stderr := &boundedResourceBuffer{limit: logLimit}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	if stdout.overflow {
		return stdout.buffer.Bytes(), stderr.buffer.Bytes(), errResourceOutputTooLarge
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.buffer.String())
		if len(detail) > 4096 {
			detail = detail[len(detail)-4096:]
		}
		if detail != "" {
			return stdout.buffer.Bytes(), stderr.buffer.Bytes(), fmt.Errorf("%s: %w", detail, err)
		}
	}
	return stdout.buffer.Bytes(), stderr.buffer.Bytes(), err
}

var systemTuningProtocolV1Pattern = regexp.MustCompile(`(?m)^KPANEL_SYSTEM_TUNING_PROTOCOL_VERSION="1"\r?$`)

func (m *Manager) SystemTuningCapabilities() []contract.Capability {
	readErr := m.systemTuningAvailability(false)
	writeErr := m.systemTuningAvailability(true)
	capability := func(id, method string, err error) contract.Capability {
		if err != nil {
			return contract.Capability{ID: id, Enabled: false, Reason: resourceCapabilityReason(err)}
		}
		return contract.Capability{ID: id, Enabled: true, Methods: []string{method}}
	}
	return []contract.Capability{
		capability("system.tuning.read", "GET", readErr),
		capability("system.tuning.write", "POST", writeErr),
	}
}

func (m *Manager) systemTuningAvailability(write bool) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: one-click system tuning is only available on Linux", ErrUnsupported)
	}
	if m.effectiveUID() != 0 {
		return fmt.Errorf("%w: Agent must run as root for one-click system tuning", ErrUnsupported)
	}
	commands := []string{"env", "bash", "awk", "grep", "mktemp", "rm", "sha256sum"}
	if write {
		if !m.enabled {
			return fmt.Errorf("%w: host system writes are disabled", ErrDisabled)
		}
		commands = append(commands, "flock", "systemd-run")
		if _, err := m.backgroundExecutable(); err != nil {
			return fmt.Errorf("%w: Agent background executor is unavailable", ErrUnsupported)
		}
	}
	for _, command := range commands {
		if _, err := m.runner.LookPath(command); err != nil {
			return fmt.Errorf("%w: %s is unavailable", ErrUnsupported, command)
		}
	}
	_, err := m.systemTuningScriptPath()
	return err
}

func (m *Manager) systemTuningScriptPath() (string, error) {
	path, err := m.systemResourceScriptPath()
	if err != nil {
		return "", err
	}
	content, err := readResourceFile(path, resourceScriptMaxBytes)
	if err != nil || !trustedKejilionSystemTuningContent(content) {
		return "", fmt.Errorf("%w: trusted kejilion.sh system-tuning protocol v1 was not found", ErrUnsupported)
	}
	return path, nil
}

func trustedKejilionSystemTuningContent(content []byte) bool {
	value := string(content)
	return trustedKejilionSystemResourceContent(content) &&
		systemTuningProtocolV1Pattern.Match(content) &&
		strings.Contains(value, "KJ_SYSTEM_TUNING_NONINTERACTIVE") &&
		strings.Contains(value, "kpanel_system_tuning_dispatch") &&
		strings.Contains(value, "KPANEL_SYSTEM_TUNING_MIRROR_SHA256") &&
		strings.Contains(value, "KPANEL_SYSTEM_TUNING_NETWORK_SHA256")
}

func (m *Manager) runSystemTuning(ctx context.Context, arguments ...string) ([]byte, error) {
	script, err := m.systemTuningScriptPath()
	if err != nil {
		return nil, err
	}
	commandArguments := []string{
		"KJ_SYSTEM_TUNING_NONINTERACTIVE=1", "LC_ALL=C.UTF-8", "LANG=C.UTF-8", "bash", script,
		"kpanel", "system-tuning",
	}
	commandArguments = append(commandArguments, arguments...)
	if runner, ok := m.runner.(systemTuningCommandRunner); ok {
		output, _, runErr := runner.RunSystemTuning(
			ctx, systemTuningOutputLimit, systemTuningLogLimit, "env", commandArguments...,
		)
		return output, runErr
	}
	output, _, runErr := m.runResourceCommandInput(ctx, systemTuningOutputLimit, nil, "env", commandArguments...)
	return output, runErr
}

func (m *Manager) SystemTuningSnapshot(ctx context.Context) (contract.SystemTuningSnapshot, error) {
	snapshot, err := m.readSystemTuningSnapshot(ctx)
	if err != nil {
		return contract.SystemTuningSnapshot{}, err
	}
	snapshot.Maintenance = m.MaintenanceStatus()
	return snapshot, nil
}

func (m *Manager) readSystemTuningSnapshot(ctx context.Context) (contract.SystemTuningSnapshot, error) {
	if err := m.systemTuningAvailability(false); err != nil {
		return contract.SystemTuningSnapshot{}, err
	}
	readContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, runErr := m.runSystemTuning(readContext, "status")
	snapshot, status, _, parseErr := parseSystemTuningOutput(output)
	if parseErr != nil {
		return contract.SystemTuningSnapshot{}, fmt.Errorf("%w: invalid system tuning protocol output: %v", ErrNeedsAttention, parseErr)
	}
	if runErr != nil || status != "ok" {
		return contract.SystemTuningSnapshot{}, fmt.Errorf("%w: kejilion.sh could not read system tuning state: %v", ErrUnsupported, runErr)
	}
	snapshot.ObservedAt = m.now().UTC()
	return snapshot, nil
}

func parseSystemTuningOutput(output []byte) (contract.SystemTuningSnapshot, string, string, error) {
	var snapshot contract.SystemTuningSnapshot
	status, version, selected := "", "", ""
	seen := make(map[string]struct{}, len(contract.SystemTuningItemIDs))
	for _, line := range resourceLines(output) {
		if line == "" || line == "KPANEL_SYSTEM_TUNING_PROTOCOL 1" {
			continue
		}
		if value, ok := receiptValue(line, "KPANEL_SYSTEM_TUNING_STATUS"); ok {
			if status != "" {
				return snapshot, "", "", errors.New("duplicate status")
			}
			status = value
			continue
		}
		if value, ok := receiptValue(line, "KPANEL_SYSTEM_TUNING_VERSION"); ok {
			if version != "" {
				return snapshot, "", "", errors.New("duplicate version")
			}
			version = value
			continue
		}
		if value, ok := receiptValue(line, "KPANEL_SYSTEM_TUNING_SELECTED"); ok {
			if selected != "" || !contract.IsSystemTuningItem(value) {
				return snapshot, "", "", errors.New("selected item is invalid")
			}
			selected = value
			continue
		}
		if value, ok := receiptValue(line, "KPANEL_SYSTEM_TUNING_ITEM"); ok {
			id, state, found := strings.Cut(value, ":")
			if !found || !contract.IsSystemTuningItem(id) || (state != "ready" && state != "pending") {
				return snapshot, "", "", errors.New("item receipt is invalid")
			}
			if _, exists := seen[id]; exists {
				return snapshot, "", "", errors.New("duplicate item")
			}
			seen[id] = struct{}{}
			snapshot.Items = append(snapshot.Items, contract.SystemTuningItem{ID: id, State: state})
			continue
		}
		return snapshot, "", "", errors.New("unexpected protocol output")
	}
	switch status {
	case "ok", "applied", "unchanged", "conflict", "failed", "needs-attention":
	default:
		return snapshot, "", "", errors.New("status is invalid")
	}
	if !resourceVersionPattern.MatchString(version) {
		return snapshot, "", "", errors.New("resource version is invalid")
	}
	if len(snapshot.Items) != len(contract.SystemTuningItemIDs) {
		return snapshot, "", "", errors.New("exactly 12 items are required")
	}
	for index, id := range contract.SystemTuningItemIDs {
		if snapshot.Items[index].ID != id {
			return snapshot, "", "", errors.New("item order is invalid")
		}
	}
	snapshot.ResourceVersion = version
	return snapshot, status, selected, nil
}

func systemTuningMaintenancePolicy(items []string, version string) string {
	return version + "." + strings.Join(items, ",")
}

func parseSystemTuningMaintenancePolicy(policy string) ([]string, string, bool) {
	version, itemList, ok := strings.Cut(policy, ".")
	if !ok {
		return nil, "", false
	}
	request := contract.SystemTuningActionRequest{Action: "apply", Items: strings.Split(itemList, ","), ExpectedResourceVersion: version}
	field, _ := contract.ValidateSystemTuningAction(&request)
	return request.Items, version, field == ""
}

func (m *Manager) ExecuteSystemTuningAction(ctx context.Context, request contract.SystemTuningActionRequest) (contract.SystemTuningActionResult, error) {
	if field, detail := contract.ValidateSystemTuningAction(&request); field != "" {
		return contract.SystemTuningActionResult{}, fmt.Errorf("%w: %s: %s", ErrInvalidInput, field, detail)
	}
	if err := m.systemTuningAvailability(true); err != nil {
		return contract.SystemTuningActionResult{}, err
	}
	lockContext, cancelLock := context.WithTimeout(ctx, resourceWriterLockTimeout)
	if !lockSystemResource(lockContext, &m.mu) {
		cancelLock()
		return contract.SystemTuningActionResult{}, fmt.Errorf("%w: timed out waiting for the system tuning writer", ErrConflict)
	}
	cancelLock()
	defer m.mu.Unlock()
	transactionContext, cancelTransaction := context.WithTimeout(context.WithoutCancel(ctx), resourceActionTimeout)
	defer cancelTransaction()
	current, err := m.readSystemTuningSnapshot(transactionContext)
	if err != nil {
		return contract.SystemTuningActionResult{}, err
	}
	if current.ResourceVersion != request.ExpectedResourceVersion {
		return contract.SystemTuningActionResult{}, fmt.Errorf("%w: expected resource version is stale", ErrConflict)
	}
	policy := systemTuningMaintenancePolicy(request.Items, request.ExpectedResourceVersion)
	changed, message, err := m.startMaintenance(transactionContext, "system-tuning", policy)
	if err != nil {
		return contract.SystemTuningActionResult{}, err
	}
	return contract.SystemTuningActionResult{
		Action: request.Action, Items: append([]string(nil), request.Items...), Status: "accepted", Changed: changed,
		Message: message, ResourceVersion: current.ResourceVersion, AcceptedAt: m.now().UTC(),
	}, nil
}
