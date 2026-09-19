package contract

import (
	"regexp"
	"strings"
	"time"
)

var systemTuningVersionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

var SystemTuningItemIDs = []string{
	"system-update", "system-cleanup", "swap-1g", "ssh-port-5522",
	"ssh-defense", "firewall-open-all", "bbr", "timezone-shanghai",
	"dns-auto", "ipv4-preferred", "basic-tools", "kernel-auto",
}

type SystemTuningItem struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

type SystemTuningSnapshot struct {
	ResourceVersion string                   `json:"resourceVersion"`
	Items           []SystemTuningItem       `json:"items"`
	Maintenance     SystemMaintenanceSummary `json:"maintenance"`
	ObservedAt      time.Time                `json:"observedAt"`
}

type SystemTuningActionRequest struct {
	Action                  string   `json:"action"`
	Items                   []string `json:"items"`
	ExpectedResourceVersion string   `json:"expectedResourceVersion"`
}

func IsSystemTuningItem(value string) bool {
	for _, item := range SystemTuningItemIDs {
		if value == item {
			return true
		}
	}
	return false
}

func ValidateSystemTuningAction(request *SystemTuningActionRequest) (string, string) {
	if request == nil {
		return "request", "request is required"
	}
	request.Action = strings.TrimSpace(request.Action)
	if request.Action != "apply" {
		return "action", "action must be apply"
	}
	if len(request.Items) < 1 || len(request.Items) > len(SystemTuningItemIDs) {
		return "items", "items must contain 1 to 12 entries"
	}
	seen := make(map[string]struct{}, len(request.Items))
	for _, item := range request.Items {
		if !IsSystemTuningItem(item) {
			return "items", "items contains an unsupported entry"
		}
		if _, exists := seen[item]; exists {
			return "items", "items must not contain duplicates"
		}
		seen[item] = struct{}{}
	}
	if !systemTuningVersionPattern.MatchString(request.ExpectedResourceVersion) {
		return "expectedResourceVersion", "expectedResourceVersion must be 64 lowercase hexadecimal characters"
	}
	return "", ""
}

type SystemTuningActionResult struct {
	TaskID          string    `json:"taskId,omitempty"`
	Action          string    `json:"action"`
	Items           []string  `json:"items"`
	Status          string    `json:"status"`
	Changed         bool      `json:"changed"`
	Message         string    `json:"message"`
	ResourceVersion string    `json:"resourceVersion"`
	AcceptedAt      time.Time `json:"acceptedAt"`
}
