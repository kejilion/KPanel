package contract

import (
	"regexp"
	"strings"
	"time"
)

const (
	NetworkOperationMaxPorts       = 512
	TrafficShutdownMaxThresholdGiB = 8_388_607
)

var networkOperationVersionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// PortUsageEntry is one bounded ss record returned by the kejilion.sh truth source.
type PortUsageEntry struct {
	Protocol     string `json:"protocol"`
	State        string `json:"state"`
	LocalAddress string `json:"localAddress"`
	LocalPort    string `json:"localPort"`
	PeerAddress  string `json:"peerAddress"`
	PeerPort     string `json:"peerPort"`
	Process      string `json:"process,omitempty"`
	PID          int    `json:"pid,omitempty"`
	Raw          string `json:"raw"`
}

type PortUsageSnapshot struct {
	ResourceVersion string           `json:"resourceVersion"`
	Entries         []PortUsageEntry `json:"entries"`
	Total           int              `json:"total"`
	Truncated       bool             `json:"truncated"`
	ObservedAt      time.Time        `json:"observedAt"`
}

type TrafficShutdownSnapshot struct {
	ResourceVersion string    `json:"resourceVersion"`
	Enabled         bool      `json:"enabled"`
	Health          string    `json:"health"`
	RXBytes         uint64    `json:"rxBytes"`
	TXBytes         uint64    `json:"txBytes"`
	RXThresholdGiB  uint64    `json:"rxThresholdGiB"`
	TXThresholdGiB  uint64    `json:"txThresholdGiB"`
	ResetDay        int       `json:"resetDay"`
	ObservedAt      time.Time `json:"observedAt"`
}

type TrafficShutdownActionRequest struct {
	Action                  string  `json:"action"`
	ExpectedResourceVersion string  `json:"expectedResourceVersion"`
	RXThresholdGiB          *uint64 `json:"rxThresholdGiB,omitempty"`
	TXThresholdGiB          *uint64 `json:"txThresholdGiB,omitempty"`
	ResetDay                *int    `json:"resetDay,omitempty"`
}

func ValidateTrafficShutdownAction(request *TrafficShutdownActionRequest) (string, string) {
	if request == nil {
		return "request", "request is required"
	}
	request.Action = strings.TrimSpace(request.Action)
	if request.Action != "enable" && request.Action != "disable" {
		return "action", "action must be enable or disable"
	}
	if !networkOperationVersionPattern.MatchString(request.ExpectedResourceVersion) {
		return "expectedResourceVersion", "expectedResourceVersion must be 64 lowercase hexadecimal characters"
	}
	if request.Action == "disable" {
		if request.RXThresholdGiB != nil || request.TXThresholdGiB != nil || request.ResetDay != nil {
			return "action", "thresholds and resetDay are not allowed for disable"
		}
		return "", ""
	}
	if request.RXThresholdGiB == nil {
		return "rxThresholdGiB", "rxThresholdGiB is required"
	}
	if *request.RXThresholdGiB == 0 || *request.RXThresholdGiB > TrafficShutdownMaxThresholdGiB {
		return "rxThresholdGiB", "rxThresholdGiB is outside the supported range"
	}
	if request.TXThresholdGiB == nil {
		return "txThresholdGiB", "txThresholdGiB is required"
	}
	if *request.TXThresholdGiB == 0 || *request.TXThresholdGiB > TrafficShutdownMaxThresholdGiB {
		return "txThresholdGiB", "txThresholdGiB is outside the supported range"
	}
	if request.ResetDay == nil {
		return "resetDay", "resetDay is required"
	}
	if *request.ResetDay < 1 || *request.ResetDay > 31 {
		return "resetDay", "resetDay must be between 1 and 31"
	}
	return "", ""
}

type TrafficShutdownActionResult struct {
	Action          string    `json:"action"`
	Status          string    `json:"status"`
	Changed         bool      `json:"changed"`
	Message         string    `json:"message"`
	BackupPath      string    `json:"backupPath,omitempty"`
	ResourceVersion string    `json:"resourceVersion"`
	AppliedAt       time.Time `json:"appliedAt"`
}
