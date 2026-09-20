package systemmanage

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type systemPackageDefinition struct {
	id         string
	category   string
	launchable bool
}

var systemPackageCatalog = []systemPackageDefinition{
	{id: "curl", category: "network"}, {id: "wget", category: "network"},
	{id: "sudo", category: "system"}, {id: "socat", category: "network"},
	{id: "htop", category: "monitor", launchable: true}, {id: "iftop", category: "monitor", launchable: true},
	{id: "unzip", category: "archive"}, {id: "tar", category: "archive"},
	{id: "tmux", category: "terminal", launchable: true}, {id: "ffmpeg", category: "media"},
	{id: "btop", category: "monitor", launchable: true}, {id: "ranger", category: "terminal", launchable: true},
	{id: "ncdu", category: "monitor", launchable: true}, {id: "fzf", category: "terminal", launchable: true},
	{id: "vim", category: "editor"}, {id: "nano", category: "editor"}, {id: "git", category: "developer"},
}

func (m *Manager) SystemPackagesCapabilities() []contract.Capability {
	readErr := m.systemPackagesAvailability(false)
	writeErr := m.systemPackagesAvailability(true)
	capability := func(id, method string, err error) contract.Capability {
		if err != nil {
			return contract.Capability{ID: id, Reason: resourceCapabilityReason(err)}
		}
		return contract.Capability{ID: id, Enabled: true, Methods: []string{method}}
	}
	return []contract.Capability{
		capability("system.packages.read", "GET", readErr),
		capability("system.packages.write", "POST", writeErr),
	}
}

func (m *Manager) systemPackagesAvailability(write bool) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("%w: system package management is only available on Linux", ErrUnsupported)
	}
	if m.effectiveUID() != 0 {
		return fmt.Errorf("%w: Agent must run as root for system package management", ErrUnsupported)
	}
	support := m.detectPackageManager()
	if !support.available() {
		reason := support.reason
		if reason == "" {
			reason = "no supported package manager was detected"
		}
		return fmt.Errorf("%w: %s", ErrUnsupported, reason)
	}
	if !write {
		return nil
	}
	if !m.enabled {
		return fmt.Errorf("%w: host system writes are disabled", ErrDisabled)
	}
	if _, err := m.backgroundExecutable(); err != nil {
		return fmt.Errorf("%w: Agent background executor is unavailable", ErrUnsupported)
	}
	if err := m.backgroundJobsAvailable(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return nil
}

func systemPackageInstalled(runner Runner, command string) bool {
	_, err := runner.LookPath(command)
	return err == nil
}

func (m *Manager) readSystemPackagesSnapshot() (contract.SystemPackagesSnapshot, error) {
	if err := m.systemPackagesAvailability(false); err != nil {
		return contract.SystemPackagesSnapshot{}, err
	}
	support := m.detectPackageManager()
	items := make([]contract.SystemPackage, 0, len(systemPackageCatalog))
	canonical := []string{string(support.kind)}
	for _, definition := range systemPackageCatalog {
		installed := systemPackageInstalled(m.runner, definition.id)
		items = append(items, contract.SystemPackage{
			ID: definition.id, Category: definition.category, Installed: installed,
			Launchable: definition.launchable,
		})
		canonical = append(canonical, fmt.Sprintf("%s=%t", definition.id, installed))
	}
	return contract.SystemPackagesSnapshot{
		Manager: support.displayName(), Items: items,
		ResourceVersion: resourceHash([]byte(strings.Join(canonical, "\n"))), ObservedAt: m.now().UTC(),
	}, nil
}

func (m *Manager) SystemPackagesSnapshot(context.Context) (contract.SystemPackagesSnapshot, error) {
	snapshot, err := m.readSystemPackagesSnapshot()
	if err != nil {
		return contract.SystemPackagesSnapshot{}, err
	}
	snapshot.Maintenance = m.MaintenanceStatus()
	return snapshot, nil
}

func systemPackagesPolicy(request contract.SystemPackagesActionRequest) string {
	return request.Action + "." + request.ExpectedResourceVersion + "." + strings.Join(request.Items, ",")
}

func parseSystemPackagesPolicy(policy string) (contract.SystemPackagesActionRequest, bool) {
	parts := strings.SplitN(policy, ".", 3)
	if len(parts) != 3 {
		return contract.SystemPackagesActionRequest{}, false
	}
	request := contract.SystemPackagesActionRequest{
		Action: parts[0], ExpectedResourceVersion: parts[1], Items: strings.Split(parts[2], ","),
	}
	field, _ := contract.ValidateSystemPackagesAction(&request)
	return request, field == ""
}

func (m *Manager) ExecuteSystemPackagesAction(ctx context.Context, request contract.SystemPackagesActionRequest) (contract.SystemPackagesActionResult, error) {
	if field, detail := contract.ValidateSystemPackagesAction(&request); field != "" {
		return contract.SystemPackagesActionResult{}, fmt.Errorf("%w: %s: %s", ErrInvalidInput, field, detail)
	}
	if err := m.systemPackagesAvailability(true); err != nil {
		return contract.SystemPackagesActionResult{}, err
	}
	lockContext, cancelLock := context.WithTimeout(ctx, resourceWriterLockTimeout)
	if !lockSystemResource(lockContext, &m.mu) {
		cancelLock()
		return contract.SystemPackagesActionResult{}, fmt.Errorf("%w: timed out waiting for the package manager", ErrConflict)
	}
	cancelLock()
	defer m.mu.Unlock()
	transactionContext, cancelTransaction := context.WithTimeout(context.WithoutCancel(ctx), resourceActionTimeout)
	defer cancelTransaction()
	current, err := m.readSystemPackagesSnapshot()
	if err != nil {
		return contract.SystemPackagesActionResult{}, err
	}
	if current.ResourceVersion != request.ExpectedResourceVersion {
		return contract.SystemPackagesActionResult{}, fmt.Errorf("%w: expected resource version is stale", ErrConflict)
	}
	changed, message, taskID, err := m.startMaintenanceTask(transactionContext, "packages", systemPackagesPolicy(request))
	if err != nil {
		return contract.SystemPackagesActionResult{}, err
	}
	return contract.SystemPackagesActionResult{
		TaskID: taskID, Action: request.Action, Items: append([]string(nil), request.Items...),
		Status: "accepted", Changed: changed, Message: message,
		ResourceVersion: current.ResourceVersion, AcceptedAt: m.now().UTC(),
	}, nil
}

func (m *Manager) systemPackageSteps(policy string) ([]maintenanceStep, error) {
	request, ok := parseSystemPackagesPolicy(policy)
	if !ok {
		return nil, fmt.Errorf("%w: package action policy is invalid", ErrInvalidInput)
	}
	support := m.detectPackageManager()
	if !support.available() {
		return nil, fmt.Errorf("%w: %s", ErrUnsupported, support.reason)
	}
	steps := make([]maintenanceStep, 0, len(request.Items))
	for index, item := range request.Items {
		arguments := systemPackageArguments(support.kind, request.Action, item)
		if len(arguments) == 0 {
			return nil, fmt.Errorf("%w: unsupported package manager action", ErrUnsupported)
		}
		steps = append(steps, maintenanceStep{
			stage:    "packages_" + request.Action + "_" + item,
			progress: 8 + index*86/len(request.Items), command: support.command, arguments: arguments,
		})
	}
	return steps, nil
}

func systemPackageArguments(kind packageManagerKind, action, item string) []string {
	switch kind {
	case packageManagerAPT:
		verb := "install"
		if action == "remove" {
			verb = "purge"
		}
		return []string{"-o", "Dpkg::Lock::Timeout=120", "-y", verb, "--", item}
	case packageManagerDNF, packageManagerYUM:
		return []string{"-y", action, item}
	case packageManagerAPK:
		verb := "add"
		if action == "remove" {
			verb = "del"
		}
		return []string{"--no-progress", verb, item}
	case packageManagerPacman:
		if action == "install" {
			return []string{"-S", "--noconfirm", "--needed", "--", item}
		}
		return []string{"-Rns", "--noconfirm", "--", item}
	case packageManagerZypper:
		return []string{"--non-interactive", action, "--", item}
	default:
		return nil
	}
}
