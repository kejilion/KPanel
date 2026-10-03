//go:build windows

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster/sshlogin"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/terminal"
	"github.com/kejilion/kejilion-panel/internal/windowsnode"
)

var defaultConfigPath = windowsnode.ConfigPath()
var defaultTerminalConfigPath = windowsnode.TerminalConfigPath()
var updateHealthPath = filepath.Join(windowsnode.DataDir(), "update-status.json")
var nodeCheckStatusPath = windowsnode.RuntimePath("check-status.json")
var windowsServiceContext context.Context
var windowsDesktopBroker func(context.Context, nodeConfig)
var windowsDesktopReason func() string

func nodeSignalContext() (context.Context, context.CancelFunc) {
	if windowsServiceContext != nil {
		return context.WithCancel(windowsServiceContext)
	}
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
func requireEnrollmentIdentity() error {
	if !windowsnode.IsSystem() {
		return errors.New("Windows enrollment requires the SYSTEM bootstrap service; use the signed installer")
	}
	return nil
}
func requireBrokerIdentity() error {
	if !windowsnode.IsSystem() {
		return errors.New("Windows privileged broker requires LocalSystem")
	}
	return nil
}
func requireEnrollmentCenter(headers http.Header) error {
	if !hasResponseCapability(headers, "windows-node-v1") {
		return errors.New("center does not support Windows nodes; upgrade the center before enrollment")
	}
	return nil
}
func enrollmentCapabilities(value string) ([]string, error) {
	if value == "" {
		joined, err := windowsnode.DomainJoined()
		if err != nil {
			return nil, err
		}
		if joined {
			return []string{"monitoring"}, nil
		}
		if terminal.Available() != nil {
			return []string{"monitoring", "files", "login"}, nil
		}
		return []string{"monitoring", "terminal", "files", "login"}, nil
	}
	result := []string{"monitoring"}
	for _, c := range strings.Split(value, ",") {
		c = strings.TrimSpace(c)
		if !slices.Contains([]string{"monitoring", "terminal", "files", "login", "desktop"}, c) {
			return nil, errors.New("unknown Windows node capability")
		}
		if !slices.Contains(result, c) {
			result = append(result, c)
		}
	}
	if slices.Contains(result, "terminal") && terminal.Available() != nil {
		return nil, errors.New("this Windows version does not support ConPTY")
	}
	if slices.Contains(result, "desktop") && windowsDesktopBroker == nil {
		return nil, errors.New("remote desktop is unavailable in this build")
	}
	return result, nil
}

func platformTerminalEnabled(config nodeConfig) bool {
	return slices.Contains(config.Capabilities, "terminal")
}

func installationCapabilities(request windowsInstallRequest) ([]string, error) {
	caps, err := enrollmentCapabilities(request.Capabilities)
	if err != nil {
		return nil, err
	}
	if request.EnableDesktop && !slices.Contains(caps, "desktop") {
		if windowsDesktopBroker == nil {
			return nil, errors.New("remote desktop is unavailable in this build")
		}
		caps = append(caps, "desktop")
	}
	return caps, nil
}
func requirePlatformBrokerCapabilities(ctx context.Context, config nodeConfig) error {
	if platformTerminalEnabled(config) {
		return requireWindowsCapability(ctx, config, "terminal")
	}
	return requireWindowsCapability(ctx, config, "desktop")
}
func startPlatformDesktop(ctx context.Context, config nodeConfig) func() {
	if !slices.Contains(config.Capabilities, "desktop") || windowsDesktopBroker == nil {
		return func() {}
	}
	child, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); windowsDesktopBroker(child, config) }()
	return func() { cancel(); <-done }
}

func requireWindowsCapability(parent context.Context, config nodeConfig, capability string) error {
	if !slices.Contains(config.Capabilities, capability) {
		return errors.New("Windows node capability was not enabled at installation")
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	secret, err := base64.RawURLEncoding.DecodeString(config.ReportingKey)
	if err != nil || len(secret) != 32 {
		return errors.New("invalid node reporting key")
	}
	defer clear(secret)
	const path = "/api/v3/federation/light/capabilities"
	body := []byte("{}")
	headers, err := signedLightNodeHeaders(config, path, body, secret, nil)
	if err != nil {
		return err
	}
	var response struct {
		Capabilities []string `json:"capabilities"`
	}
	if _, _, err = postRawJSONWithStatusAndHeaders(ctx, config.Origin+path, body, headers, &response); err != nil {
		return err
	}
	if !slices.Contains(response.Capabilities, "windows-node-v1") {
		return errors.New("center does not support Windows nodes")
	}
	if capability == "desktop" && !slices.Contains(response.Capabilities, "desktop-v1") {
		return errors.New("center does not support remote desktop")
	}
	return nil
}

func applyPlatformReport(payload *reportRequest, config nodeConfig) {
	payload.Platform = "windows"
	payload.UnavailableMetrics = []string{"load"}
	payload.Capabilities = append([]string(nil), config.Capabilities...)
	if slices.Contains(config.Capabilities, "desktop") {
		payload.DesktopUnavailableReason = "desktop_broker_unavailable"
		if windowsDesktopReason != nil {
			payload.DesktopUnavailableReason = windowsDesktopReason()
		}
	}
}
func validatePlatformConfig(file *os.File, secret bool) error {
	access := windowsnode.TelemetryRead
	if secret {
		access = windowsnode.SystemOnly
	}
	return windowsnode.ValidateFile(file, access)
}
func writePlatformConfig(path string, value any, secret bool) (bool, error) {
	if !windowsnode.IsSystem() {
		return true, errors.New("Windows configuration writes require SYSTEM")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return true, err
	}
	access := windowsnode.TelemetryRead
	if secret {
		access = windowsnode.SystemOnly
	}
	return true, windowsnode.WriteAtomic(path, append(data, '\n'), access)
}
func repairLegacyNodeConfigAccess() error { return nil }
func preserveNodeConfigAccess(*os.File, os.FileInfo) error {
	return errors.New("Windows atomic writer must use an explicit DACL")
}
func rootOwned(os.FileInfo) bool     { return false }
func processOwned(os.FileInfo) bool  { return false }
func platformStateDirectory() string { return filepath.Join(windowsnode.DataDir(), "state") }
func readUpdateHealthFile(path string) ([]byte, error) {
	return windowsnode.ReadFile(path, maxUpdateHealthBytes, windowsnode.TelemetryRead)
}
func readTrustedRuntimeFile(path string, limit int64) ([]byte, error) {
	return windowsnode.ReadFile(path, limit, windowsnode.TelemetryRead)
}
func publishNodeCheckStatus(summary contract.ServiceCheckSummary) error {
	if !windowsnode.IsSystem() {
		return os.ErrPermission
	}
	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	return windowsnode.WriteAtomic(nodeCheckStatusPath, data, windowsnode.TelemetryRead)
}
func publishProcdHealthSnapshot(contract.LightNodeHealth) error {
	return errors.New("procd is unavailable on Windows")
}

func latestPlatformLogin(context.Context, *sshlogin.Reader) (*contract.SSHLoginEvent, error) {
	return readWindowsLoginEvent()
}
