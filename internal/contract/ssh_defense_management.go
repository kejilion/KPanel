package contract

import (
	"net/netip"
	"strings"
	"time"
)

const (
	SSHDefenseMaxBannedIPs        = 256
	SSHDefenseMaxTrustedAddresses = 64
	SSHDefenseMaxRecentEvents     = 20
)

type SSHDefenseEvent struct {
	OccurredAt string `json:"occurredAt"`
	Action     string `json:"action"`
	Address    string `json:"address"`
}

type SSHDefenseSnapshot struct {
	ResourceVersion  string                   `json:"resourceVersion"`
	Installed        bool                     `json:"installed"`
	Running          bool                     `json:"running"`
	Enabled          bool                     `json:"enabled"`
	Autostart        bool                     `json:"autostart"`
	Jail             string                   `json:"jail"`
	Profile          string                   `json:"profile"`
	BanTimeSeconds   int                      `json:"banTimeSeconds"`
	FindTimeSeconds  int                      `json:"findTimeSeconds"`
	MaxRetry         int                      `json:"maxRetry"`
	CurrentFailed    int                      `json:"currentFailed"`
	TotalFailed      int                      `json:"totalFailed"`
	CurrentBanned    int                      `json:"currentBanned"`
	TotalBanned      int                      `json:"totalBanned"`
	BannedIPs        []string                 `json:"bannedIps"`
	BansTruncated    bool                     `json:"bansTruncated"`
	TrustedAddresses []string                 `json:"trustedAddresses"`
	RecentEvents     []SSHDefenseEvent        `json:"recentEvents"`
	Maintenance      SystemMaintenanceSummary `json:"maintenance"`
	ObservedAt       time.Time                `json:"observedAt"`
}

type SSHDefenseActionRequest struct {
	Action                  string `json:"action"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
	Profile                 string `json:"profile,omitempty"`
	Address                 string `json:"address,omitempty"`
}

func ValidateSSHDefenseAction(request *SSHDefenseActionRequest) (string, string) {
	if request == nil {
		return "request", "request is required"
	}
	request.Action = strings.TrimSpace(request.Action)
	request.Profile = strings.TrimSpace(request.Profile)
	request.Address = strings.TrimSpace(request.Address)
	if !accountKeyIDPattern.MatchString(request.ExpectedResourceVersion) {
		return "expectedResourceVersion", "expectedResourceVersion must be 64 lowercase hexadecimal characters"
	}
	noValues := func() (string, string) {
		if request.Profile != "" || request.Address != "" {
			return "action", "profile and address are not allowed for this action"
		}
		return "", ""
	}
	switch request.Action {
	case "enable", "disable", "uninstall", "unban-all":
		return noValues()
	case "set-profile":
		if request.Address != "" {
			return "address", "address is not allowed for set-profile"
		}
		if request.Profile != "mild" && request.Profile != "standard" && request.Profile != "strict" {
			return "profile", "profile must be mild, standard, or strict"
		}
	case "add-trusted", "remove-trusted":
		if request.Profile != "" {
			return "profile", "profile is not allowed for trusted-address actions"
		}
		if len(request.Address) == 0 || len(request.Address) > 80 || strings.ContainsAny(request.Address, "\x00\r\n") {
			return "address", "address must be one IP address or CIDR"
		}
		if _, err := netip.ParseAddr(request.Address); err != nil {
			if _, prefixErr := netip.ParsePrefix(request.Address); prefixErr != nil {
				return "address", "address must be one IP address or CIDR"
			}
		}
	case "unban":
		if request.Profile != "" {
			return "profile", "profile is not allowed for unban"
		}
		if _, err := netip.ParseAddr(request.Address); err != nil {
			return "address", "address must be one IP address"
		}
	default:
		return "action", "unsupported SSH defense action"
	}
	return "", ""
}

type SSHDefenseActionResult struct {
	TaskID          string    `json:"taskId,omitempty"`
	Action          string    `json:"action"`
	Status          string    `json:"status"`
	Changed         bool      `json:"changed"`
	Message         string    `json:"message"`
	BackupPath      string    `json:"backupPath,omitempty"`
	ResourceVersion string    `json:"resourceVersion"`
	AppliedAt       time.Time `json:"appliedAt"`
}
