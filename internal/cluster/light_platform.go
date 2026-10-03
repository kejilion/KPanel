package cluster

import (
	"bytes"
	"slices"
	"strings"
	"time"
)

const (
	LightCapabilitiesPath = "/api/v3/federation/light/capabilities"
	WindowsNodeCapability = "windows-node-v1"
)

type LightCapabilitiesResponse struct {
	Capabilities []string `json:"capabilities"`
}

func LightCenterCapabilities() []string {
	return []string{SSHLoginCapability, LightHealthCapability, ServiceChecksCapability, WindowsNodeCapability}
}

// ProbeLightCapabilities is read-only. It must never provision a privileged
// broker identity as the older file-capability upgrade does.
func (s *Service) ProbeLightCapabilities(auth LightReportAuth, body []byte) (LightCapabilitiesResponse, error) {
	now := s.now().UTC()
	if !s.lightSources.Allow(cleanRateSubject(auth.Source), now) {
		return LightCapabilitiesResponse{}, ErrRateLimited
	}
	if _, _, err := s.authenticateLightRequest(auth, LightCapabilitiesPath, body, 32, now); err != nil {
		return LightCapabilitiesResponse{}, err
	}
	if !bytes.Equal(bytes.TrimSpace(body), []byte("{}")) {
		return LightCapabilitiesResponse{}, ErrProtocolMismatch
	}
	if !s.lightReports.Allow(auth.NodeID, now) {
		return LightCapabilitiesResponse{}, ErrRateLimited
	}
	if err := s.replays.Accept(auth.NodeID, auth.RequestID, now); err != nil {
		return LightCapabilitiesResponse{}, err
	}
	return LightCapabilitiesResponse{Capabilities: LightCenterCapabilities()}, nil
}

func validateLightPlatform(input LightReportRequest) error {
	if input.Platform != "" && input.Platform != "linux" && input.Platform != "windows" {
		return ErrProtocolMismatch
	}
	if input.Platform == "windows" && (!strings.EqualFold(input.Telemetry.OSID, "windows") ||
		!slices.Contains(input.UnavailableMetrics, "load")) {
		return ErrProtocolMismatch
	}
	if input.Platform != "windows" && strings.EqualFold(input.Telemetry.OSID, "windows") {
		return ErrProtocolMismatch
	}
	for _, list := range []struct{ values, allowed []string }{
		{input.UnavailableMetrics, []string{"load", "swap", "diskIO", "networkConnections"}},
		{input.Capabilities, []string{"monitoring", "terminal", "files", "login", "desktop"}},
	} {
		if len(list.values) > len(list.allowed) {
			return ErrProtocolMismatch
		}
		for index, value := range list.values {
			if !slices.Contains(list.allowed, value) || slices.Contains(list.values[:index], value) {
				return ErrProtocolMismatch
			}
		}
	}
	return nil
}

func lightHostIsWindows(record lightHostRecord) bool {
	return record.LastSnapshot != nil && (record.LastSnapshot.Platform == "windows" ||
		strings.EqualFold(record.LastSnapshot.Telemetry.OSID, "windows"))
}

// Platform metadata is deliberately not persisted, so an old center can read
// its state after rollback. Windows must re-advertise before control is allowed.
func lightPlatformAllows(record lightHostRecord, capability string, now time.Time) bool {
	if !lightHostIsWindows(record) {
		return true
	}
	snapshot := record.LastSnapshot
	return snapshot.Platform == "windows" && now.Sub(snapshot.ReceivedAt) <= 90*time.Second &&
		slices.Contains(snapshot.NodeCapabilities, capability)
}

func (s *Service) lightControlAllowed(hostID, capability string) bool {
	record, err := s.light.Host(hostID)
	return err != nil || lightPlatformAllows(record, capability, s.now().UTC())
}

func applyLightPlatform(host *Host, record lightHostRecord, now time.Time) {
	if !lightHostIsWindows(record) {
		host.Platform, host.TerminalShell, host.PathStyle = "linux", "posix", "posix"
		return
	}
	host.Platform, host.TerminalShell, host.PathStyle = "windows", "powershell", "windows-volumes"
	host.UnavailableMetrics = []string{"load", "swap", "diskIO", "networkConnections"}
	if record.LastSnapshot.Platform == "windows" {
		host.UnavailableMetrics = append([]string(nil), record.LastSnapshot.UnavailableMetrics...)
	}
	host.TerminalAvailable = host.TerminalAvailable && lightPlatformAllows(record, "terminal", now)
	host.FileManagementAvailable = host.FileManagementAvailable && lightPlatformAllows(record, "files", now)
	host.Scope = SummaryScope
	if host.FileManagementAvailable {
		host.Scope = SummaryTerminalFilesScope
	} else if host.TerminalAvailable {
		host.Scope = SummaryTerminalScope
	}
	host.DesktopUnavailableReason = "desktop_not_enabled"
}
