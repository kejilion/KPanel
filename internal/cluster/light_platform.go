package cluster

import (
	"bytes"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
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
	if err := validateLightPlatformChoice(input.Platform); err != nil {
		return err
	}
	if !telemetryIdentifiesLinux(input.Telemetry) {
		return ErrLightPlatformUnsupported
	}
	return nil
}

func validateLightPlatformChoice(platform string) error {
	if platform != "" && platform != "linux" {
		return ErrLightPlatformUnsupported
	}
	return nil
}

// The stable light-node protocol predates platform metadata and supported only
// Linux. Unknown persisted platforms stay visible as offline records, but do
// not regain file, terminal, or history access.
func lightNodePlatformSupported(record lightHostRecord) bool {
	if record.Platform != "" {
		return record.Platform == "linux"
	}
	if record.LastSnapshot == nil {
		return false
	}
	return telemetryIdentifiesLinux(record.LastSnapshot.Telemetry)
}

func telemetryIdentifiesLinux(telemetry contract.HostTelemetry) bool {
	return strings.EqualFold(strings.TrimSpace(telemetry.OSID), "linux") ||
		strings.Contains(strings.ToLower(telemetry.OS), "linux") || len(telemetry.OSLike) > 0
}

func (s *Service) lightControlAllowed(hostID, _ string) bool {
	record, err := s.light.Host(hostID)
	if err == nil {
		return lightNodePlatformSupported(record)
	}
	// Non-light terminals belong to the v2 store. A deleted/missing lightweight
	// record is not permission to continue using an existing stream.
	_, err = s.storeV2.Host(hostID)
	return err == nil
}

func applyLightPlatform(host *Host, record lightHostRecord) {
	if lightNodePlatformSupported(record) {
		host.Platform = "linux"
		host.TerminalShell = "posix"
		return
	}
	host.Platform = "unknown"
	host.TerminalShell = ""
	host.TerminalAvailable = false
	host.FileManagementAvailable = false
	host.LightHealth = nil
	host.LastSnapshot = nil
	host.State = HostOffline
	host.Scope = SummaryScope
}
