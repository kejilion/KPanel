package cluster

import (
	"bytes"
	"slices"
	"strings"
	"time"
)

const (
	LightCapabilitiesPath = "/api/v3/federation/light/capabilities"
)

type LightCapabilitiesResponse struct {
	Capabilities []string `json:"capabilities"`
}

func LightCenterCapabilities() []string {
	return []string{SSHLoginCapability, LightHealthCapability, ServiceChecksCapability}
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
	if input.Platform == "windows" || strings.EqualFold(input.Telemetry.OSID, "windows") {
		return ErrLightPlatformUnsupported
	}
	if input.Platform != "" && input.Platform != "linux" {
		return ErrProtocolMismatch
	}
	if input.DesktopUnavailableReason != "" {
		return ErrProtocolMismatch
	}
	for _, list := range []struct{ values, allowed []string }{
		{input.UnavailableMetrics, []string{"load", "swap", "diskIO", "networkConnections"}},
		{input.Capabilities, []string{"monitoring", "terminal", "files", "login"}},
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

func validateLightPlatformChoice(platform string, desktop bool) error {
	if desktop || platform == "windows" {
		return ErrLightPlatformUnsupported
	}
	if platform != "" && platform != "linux" {
		return ErrProtocolMismatch
	}
	return nil
}

func lightHostIsWindows(record lightHostRecord) bool {
	return record.Platform == "windows" || record.LastSnapshot != nil && (record.LastSnapshot.Platform == "windows" ||
		strings.EqualFold(record.LastSnapshot.Telemetry.OSID, "windows"))
}

// Windows light-node support is withdrawn from the next preview. Keep existing
// records recognizable for safe display, but never authorize their controls.
func lightPlatformAllows(record lightHostRecord, _ string, _ time.Time) bool {
	if lightHostIsWindows(record) {
		return false
	}
	// Current Linux enrollments persist their platform before the first report,
	// so relay setup can proceed during startup. Older records without either a
	// persisted platform or a snapshot remain unknown and fail closed.
	return record.Platform == "linux" || record.LastSnapshot != nil
}

func (s *Service) lightControlAllowed(hostID, capability string) bool {
	record, err := s.light.Host(hostID)
	if err == nil {
		return lightPlatformAllows(record, capability, s.now().UTC())
	}
	// Non-light terminals belong to the v2 store. A deleted/missing lightweight
	// record is not permission to continue using an existing stream.
	_, err = s.storeV2.Host(hostID)
	return err == nil
}

func applyLightPlatform(host *Host, record lightHostRecord) {
	if !lightHostIsWindows(record) {
		if record.LastSnapshot == nil && record.Platform != "linux" {
			host.Platform = "unknown"
			host.TerminalShell, host.PathStyle = "", ""
			host.TerminalAvailable = false
			host.FileManagementAvailable = false
			host.LightHealth = nil
			host.LastSnapshot = nil
			host.State = HostOffline
			host.Scope = SummaryScope
			return
		}
		host.Platform, host.TerminalShell, host.PathStyle = "linux", "posix", "posix"
		return
	}
	host.Platform, host.TerminalShell, host.PathStyle = "windows", "", ""
	host.TerminalAvailable = false
	host.FileManagementAvailable = false
	host.LightHealth = nil
	host.LastSnapshot = nil
	host.State = HostOffline
	host.Scope = SummaryScope
}
