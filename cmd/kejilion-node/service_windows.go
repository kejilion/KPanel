//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/windowsnode"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type windowsInstallRequest struct {
	EnableDesktop bool                    `json:"enableDesktop,omitempty"`
	Token         string                  `json:"token"`
	Name          string                  `json:"name,omitempty"`
	Capabilities  string                  `json:"capabilities,omitempty"`
	Trust         windowsnode.TrustPolicy `json:"trust"`
}

func runPlatformCommand(arguments []string) (bool, error) {
	if len(arguments) == 0 {
		return false, nil
	}
	switch arguments[0] {
	case "install":
		return true, installWindowsNode(arguments[1:])
	case "status":
		return true, json.NewEncoder(os.Stdout).Encode(collectLightHealth(context.Background()))
	case "update":
		return true, runWindowsUpdate()
	case "uninstall":
		return true, uninstallWindowsNode()
	case "login-broker", "ssh-login-broker":
		return true, runWindowsLoginBroker()
	case "service":
		if len(arguments) == 2 && arguments[1] == "bootstrap" {
			return true, windowsnode.RunService(windowsnode.BootstrapService, func(ctx context.Context) error { windowsServiceContext = ctx; return bootstrapWindowsNode() })
		}
		if len(arguments) == 2 && arguments[1] == "remove" {
			return true, windowsnode.RunService(windowsnode.BootstrapService, func(context.Context) (resultErr error) {
				if !windowsnode.IsSystem() {
					return errors.New("uninstall bootstrap requires SYSTEM")
				}
				release, err := windowsnode.AcquireLifecycle()
				if err != nil {
					return err
				}
				defer func() { resultErr = errors.Join(resultErr, release()) }()
				if err := windowsnode.RemoveServices(); err != nil {
					return err
				}
				return windowsnode.RemoveInstallation()
			})
		}
		if len(arguments) != 3 || arguments[1] != "run" {
			return true, errors.New("expected service run <fixed service name>")
		}
		for _, spec := range windowsnode.Services {
			if spec.Name == arguments[2] {
				return true, windowsnode.RunService(spec.Name, func(ctx context.Context) error { windowsServiceContext = ctx; return run([]string{spec.Command}) })
			}
		}
		return true, errors.New("unknown Windows node service")
	}
	return false, nil
}
func installWindowsNode(arguments []string) (resultErr error) {
	if len(arguments) != 1 || arguments[0] != "--stdin" {
		return errors.New("expected install --stdin; enrollment credentials are never service arguments")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("run installer as Administrator")
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 8193))
	if err != nil || len(data) > 8192 {
		return errors.New("invalid installation request")
	}
	defer clear(data)
	var request windowsInstallRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		return errors.New("invalid installation request")
	}
	if _, err := enrollmentTargetFromToken(request.Token); err != nil {
		return err
	}
	if !validEnrollmentName(request.Name) {
		return errors.New("invalid node name")
	}
	if err := request.Trust.Validate(); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	expected := filepath.Join(windowsnode.InstallDir(), "kejilion-node.exe")
	if !strings.EqualFold(filepath.Clean(executable), expected) {
		return errors.New("installer executable must be in protected Program Files directory")
	}
	if err := windowsnode.VerifySignature(executable, request.Trust); err != nil {
		return err
	}
	release, err := windowsnode.AcquireLifecycle()
	if err != nil {
		return err
	}
	defer func() {
		if release != nil {
			resultErr = errors.Join(resultErr, release())
		}
	}()
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.CreateService(windowsnode.BootstrapService, executable, mgr.Config{StartType: mgr.StartManual, DisplayName: windowsnode.BootstrapService}, "service", "bootstrap")
	if err != nil {
		return err
	}
	defer service.Close()
	defer service.Delete()
	requestPath := filepath.Join(windowsnode.DataDir(), "enrollment-request.json")
	if err := windowsnode.WriteAtomic(requestPath, data, windowsnode.SystemOnly); err != nil {
		return err
	}
	if err := release(); err != nil {
		return err
	}
	release = nil
	if err := service.Start(); err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Stopped {
			if status.Win32ExitCode != 0 || status.ServiceSpecificExitCode != 0 {
				return errors.New("SYSTEM enrollment bootstrap failed; review Windows service status; batch attempt identity is retained")
			}
			fmt.Println("KPanel Windows node installed; reporting credentials and relay identity are protected by SYSTEM DACLs")
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("Windows enrollment bootstrap timed out; check service status before retrying")
}
func bootstrapWindowsNode() (resultErr error) {
	if !windowsnode.IsSystem() {
		return errors.New("bootstrap requires SYSTEM")
	}
	release, err := windowsnode.AcquireLifecycle()
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, release()) }()
	path := filepath.Join(windowsnode.DataDir(), "enrollment-request.json")
	data, err := windowsnode.ReadFile(path, 8192, windowsnode.SystemOnly)
	if err != nil {
		return err
	}
	defer clear(data)
	// Delete the token-bearing handoff as soon as its SYSTEM reader has opened it.
	if err := os.Remove(path); err != nil {
		return err
	}
	var request windowsInstallRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		return errors.New("invalid bootstrap request")
	}
	if err := windowsnode.InitializeDirectories(); err != nil {
		return err
	}
	if err := request.Trust.Validate(); err != nil {
		return err
	}
	for _, name := range []string{"kejilion-node.exe", "kejilion-node-bootstrap.exe"} {
		path := filepath.Join(windowsnode.InstallDir(), name)
		if err := windowsnode.VerifySignature(path, request.Trust); err != nil {
			return err
		}
		if err := windowsnode.SecureProgramFile(path); err != nil {
			return err
		}
	}
	policy, err := json.Marshal(request.Trust)
	if err != nil {
		return err
	}
	if err := windowsnode.WriteAtomic(filepath.Join(windowsnode.DataDir(), "trust.json"), policy, windowsnode.SystemOnly); err != nil {
		return err
	}
	capabilities, err := installationCapabilities(request)
	if err != nil {
		return err
	}
	if err := ensureWindowsEnrollment(request, capabilities); err != nil {
		return err
	}
	request.Token = ""
	return windowsnode.InstallServices(capabilities)
}

func ensureWindowsEnrollment(request windowsInstallRequest, capabilities []string) error {
	if _, err := os.Lstat(defaultConfigPath); err == nil {
		config, _, err := readConfig(defaultConfigPath)
		if err != nil {
			return err
		}
		digest := sha256.Sum256([]byte(strings.TrimSpace(request.Token)))
		if config.EnrollmentTokenHash != hex.EncodeToString(digest[:]) || !slices.Equal(config.Capabilities, capabilities) {
			return errors.New("existing enrollment belongs to a different token or capability set")
		}
		if config.TargetNodeID == "" {
			return nil
		}
		if _, _, err := readTerminalConfig(defaultTerminalConfigPath); err == nil {
			return nil
		}
		if !strings.HasPrefix(request.Token, lightBatchTokenPrefix) {
			return errors.New("enrollment was interrupted before relay credentials were saved; generate a new token after removing this incomplete installation")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return runEnroll([]string{"--token", request.Token, "--name", request.Name, "--capabilities", strings.Join(capabilities, ",")})
}
func runWindowsLoginBroker() error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if !windowsnode.IsSystem() && user.User.Sid.String() != windowsnode.ServiceSID(windowsnode.LoginService) {
		return errors.New("login broker requires its dedicated Windows service identity")
	}
	ctx, cancel := nodeSignalContext()
	defer cancel()
	path := windowsnode.RuntimePath(filepath.Join("login", "event.json"))
	last := ""
	for {
		event, err := windowsnode.LatestLogin(ctx)
		if err == nil && event != nil && event.ID != last {
			data, marshalErr := json.Marshal(event)
			if marshalErr != nil {
				return marshalErr
			}
			if err := windowsnode.WriteAtomic(path, data, windowsnode.LoginSnapshot); err != nil {
				return err
			}
			last = event.ID
		}
		if !waitContext(ctx, 5*time.Second) {
			return nil
		}
	}
}
func readWindowsLoginEvent() (*contract.SSHLoginEvent, error) {
	data, err := windowsnode.ReadFile(windowsnode.RuntimePath(filepath.Join("login", "event.json")), 4096, windowsnode.LoginSnapshot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var event contract.SSHLoginEvent
	if json.Unmarshal(data, &event) != nil || !contract.ValidSSHLoginEvent(event) || event.OccurredAt.After(time.Now().Add(time.Minute)) || event.OccurredAt.Before(time.Now().Add(-24*time.Hour)) {
		return nil, errors.New("invalid Windows login snapshot")
	}
	return &event, nil
}
func readWindowsTrust() (windowsnode.TrustPolicy, error) {
	data, err := windowsnode.ReadFile(filepath.Join(windowsnode.DataDir(), "trust.json"), 2048, windowsnode.SystemOnly)
	if err != nil {
		return windowsnode.TrustPolicy{}, err
	}
	var policy windowsnode.TrustPolicy
	if json.Unmarshal(data, &policy) != nil {
		return policy, errors.New("invalid publisher trust policy")
	}
	return policy, policy.Validate()
}
func windowsServicesHealthy(ctx context.Context) error {
	config, _, err := readConfig(defaultConfigPath)
	if err != nil {
		return err
	}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	healthySince := time.Time{}
	for {
		healthy := true
		for _, spec := range windowsnode.Services {
			if slices.Contains(config.Capabilities, spec.Capability) || (spec.Name == windowsnode.TerminalService && slices.Contains(config.Capabilities, "desktop")) {
				if windowsServiceHealth(spec.Name).ActiveState != "active" {
					healthy = false
				}
			}
		}
		if healthy {
			if healthySince.IsZero() {
				healthySince = time.Now()
			}
			if time.Since(healthySince) >= 3*time.Second {
				return nil
			}
		} else {
			healthySince = time.Time{}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("Windows node services did not become healthy")
		case <-ticker.C:
		}
	}
}
func runWindowsUpdate() (resultErr error) {
	if !windowsnode.IsSystem() {
		if !windows.GetCurrentProcessToken().IsElevated() {
			return errors.New("update requires Administrator")
		}
		return windowsnode.RequestUpdate()
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(executable), filepath.Join(windowsnode.InstallDir(), "kejilion-node-bootstrap.exe")) {
		return errors.New("run the protected bootstrap executable to update; the live service executable cannot replace itself")
	}
	release, err := windowsnode.AcquireLifecycle()
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, release()) }()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	policy, err := readWindowsTrust()
	if err != nil {
		return err
	}
	updater := windowsnode.NewUpdater(windowsServicesHealthy)
	updater.Policy = policy
	if err := updater.Recover(ctx); err != nil {
		return err
	}
	checked := time.Now().Unix()
	writeStatus := func(state, code string) {
		value := contract.LightNodeUpdateHealth{State: state, CheckedAt: checked, ErrorCode: code}
		if state != "running" {
			value.FinishedAt = time.Now().Unix()
		}
		data, _ := json.Marshal(value)
		_ = windowsnode.WriteAtomic(updateHealthPath, data, windowsnode.TelemetryRead)
	}
	writeStatus("running", "")
	live := filepath.Join(windowsnode.InstallDir(), "kejilion-node.exe")
	if err := windowsnode.VerifySignature(live, policy); err != nil {
		writeStatus("failed", "checksum")
		return err
	}
	command := exec.CommandContext(ctx, live, "version")
	output := &healthOutput{}
	command.Stdout = output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		writeStatus("failed", "protocol")
		return err
	}
	fields := strings.Fields(string(output.Bytes()))
	if len(fields) != 2 || fields[1] != lightProtocol {
		writeStatus("failed", "protocol")
		return errors.New("installed node version probe failed")
	}
	digest, tag, err := windowsnode.DownloadUpdate(ctx, fields[0])
	if err != nil {
		writeStatus("failed", "download")
		return err
	}
	if digest == "" {
		writeStatus("current", "")
		return nil
	}
	next := filepath.Join(windowsnode.InstallDir(), "staging", "kejilion-node.exe.next")
	if err := windowsnode.VerifySignature(next, policy); err != nil {
		writeStatus("failed", "checksum")
		return err
	}
	probe := exec.CommandContext(ctx, next, "version")
	probeOutput := &healthOutput{}
	probe.Stdout = probeOutput
	probe.Stderr = io.Discard
	if err := probe.Run(); err != nil {
		writeStatus("failed", "protocol")
		return err
	}
	probeFields := strings.Fields(string(probeOutput.Bytes()))
	if len(probeFields) != 2 || strings.TrimPrefix(probeFields[0], "v") != strings.TrimPrefix(tag, "v") || probeFields[1] != lightProtocol {
		writeStatus("failed", "protocol")
		return errors.New("downloaded node version or protocol does not match release")
	}
	if err := updater.Apply(ctx, digest); err != nil {
		writeStatus("failed", "rollback")
		return err
	}
	writeStatus("updated", "")
	return nil
}
func uninstallWindowsNode() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return errors.New("uninstall requires Administrator")
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.CreateService(windowsnode.BootstrapService, filepath.Join(windowsnode.InstallDir(), "kejilion-node-bootstrap.exe"), mgr.Config{StartType: mgr.StartManual, DisplayName: windowsnode.BootstrapService}, "service", "remove")
	if err != nil {
		return err
	}
	defer service.Close()
	defer service.Delete()
	if err := service.Start(); err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == svc.Stopped {
			if status.Win32ExitCode != 0 || status.ServiceSpecificExitCode != 0 {
				return errors.New("SYSTEM uninstall failed; protected data retained where removal was unsafe")
			}
			fmt.Println("Node services, scheduled update and protected data removed. Restart Windows to delete any in-use executables.")
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("SYSTEM uninstall is still running; check bootstrap service status")
}
