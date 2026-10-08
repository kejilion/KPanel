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
// Linux. Unknown persisted platforms stay in protected local state for
// compatibility, but are absent from the product surface and cannot regain
// file, terminal, or history access.
func lightNodePlatformSupported(record lightHostRecord) bool {
	if record.Platform != "" {
		return record.Platform == "linux"
	}
	if record.LastSnapshot == nil {
		return false
	}
	return telemetryIdentifiesLinux(record.LastSnapshot.Telemetry)
}

func supportedLightHostCount(records []lightHostRecord) int {
	count := 0
	for _, record := range records {
		if lightNodePlatformSupported(record) {
			count++
		}
	}
	return count
}

func retiredWindowsLightHost(record lightHostRecord) bool {
	if record.Platform == "windows" {
		return true
	}
	return record.Platform == "" && record.LastSnapshot != nil && telemetryIdentifiesWindows(record.LastSnapshot.Telemetry)
}

func telemetryIdentifiesWindows(telemetry contract.HostTelemetry) bool {
	if strings.EqualFold(strings.TrimSpace(telemetry.OSID), "windows") || strings.Contains(strings.ToLower(telemetry.OS), "windows") {
		return true
	}
	for _, id := range telemetry.OSLike {
		if strings.EqualFold(strings.TrimSpace(id), "windows") {
			return true
		}
	}
	return false
}

func telemetryIdentifiesLinux(telemetry contract.HostTelemetry) bool {
	if telemetryIdentifiesWindows(telemetry) {
		return false
	}
	osID := strings.ToLower(strings.TrimSpace(telemetry.OSID))
	osName := strings.ToLower(strings.TrimSpace(telemetry.OS))
	if osID == "linux" || strings.Contains(osName, "linux") {
		return true
	}
	for _, id := range append([]string{osID}, telemetry.OSLike...) {
		switch strings.ToLower(strings.TrimSpace(id)) {
		case "almalinux", "alpine", "amzn", "amazon", "arch", "centos", "debian", "fedora",
			"gentoo", "kali", "linuxmint", "manjaro", "nixos", "ol", "opensuse", "oracle",
			"rhel", "rocky", "sles", "slackware", "suse", "ubuntu", "void":
			return true
		case "openwrt", "lede", "immortalwrt", "istoreos":
			return true
		}
	}
	return false
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
