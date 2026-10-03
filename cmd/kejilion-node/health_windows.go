//go:build windows

package main

import (
	"context"
	"errors"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/version"
	"github.com/kejilion/kejilion-panel/internal/windowsnode"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"time"
)

func platformHealth(context.Context) *contract.LightNodeHealth {
	now := time.Now().UTC()
	health := &contract.LightNodeHealth{ObservedAt: now, RuntimeVersion: version.Version, Update: readUpdateHealth(updateHealthPath, now)}
	health.Services = contract.LightNodeServicesHealth{Timer: unknownServiceHealth(), Telemetry: windowsServiceHealth(windowsnode.TelemetryService), Terminal: windowsServiceHealth(windowsnode.TerminalService), File: windowsServiceHealth(windowsnode.FileService), SSHLogin: windowsServiceHealth(windowsnode.LoginService)}
	// Task status is a protected, bounded snapshot produced by its own SYSTEM
	// execution. An absent/stale snapshot is unknown, never a guessed success.
	if data, err := readTrustedRuntimeFile(windowsnode.RuntimePath("task-health.json"), 4096); err == nil {
		if snapshot := decodeProcdHealthSnapshot(data, now); snapshot != nil {
			health.Services.Timer = snapshot.Services.Timer
		}
	}
	return health
}
func windowsServiceHealth(name string) contract.LightNodeServiceHealth {
	unknown := unknownServiceHealth()
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return unknown
	}
	defer windows.CloseServiceHandle(scm)
	ptr, _ := windows.UTF16PtrFromString(name)
	h, err := windows.OpenService(scm, ptr, windows.SERVICE_QUERY_STATUS|windows.SERVICE_QUERY_CONFIG)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		unknown.LoadState = "not-found"
		return unknown
	}
	if err != nil {
		return unknown
	}
	defer windows.CloseServiceHandle(h)
	service := mgr.Service{Name: name, Handle: h}
	status, err := service.Query()
	if err != nil {
		return unknown
	}
	cfg, err := service.Config()
	if err != nil {
		return unknown
	}
	return mapWindowsServiceHealth(status.State, status.Win32ExitCode, cfg.StartType)
}
func mapWindowsServiceHealth(state svc.State, exit, start uint32) contract.LightNodeServiceHealth {
	result := unknownServiceHealth()
	result.LoadState = "loaded"
	switch state {
	case svc.Running:
		result.ActiveState = "active"
		result.SubState = "running"
	case svc.StartPending:
		result.ActiveState = "activating"
		result.SubState = "start"
	case svc.StopPending:
		result.ActiveState = "deactivating"
		result.SubState = "stop"
	case svc.Stopped:
		result.ActiveState = "inactive"
		result.SubState = "dead"
		if exit != 0 {
			result.ActiveState = "failed"
			result.SubState = "failed"
		}
	}
	switch start {
	case mgr.StartAutomatic:
		result.UnitFileState = "enabled"
	case mgr.StartDisabled:
		result.UnitFileState = "disabled"
	case mgr.StartManual:
		result.UnitFileState = "static"
	}
	return result
}
