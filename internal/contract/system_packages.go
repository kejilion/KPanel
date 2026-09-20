package contract

import (
	"regexp"
	"strings"
	"time"
)

var SystemPackageIDs = []string{
	"curl", "wget", "sudo", "socat", "htop", "iftop", "unzip", "tar", "tmux", "ffmpeg",
	"btop", "ranger", "ncdu", "fzf", "vim", "nano", "git",
}

var systemPackageVersionPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type SystemPackage struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	Installed  bool   `json:"installed"`
	Launchable bool   `json:"launchable"`
}

type SystemPackagesSnapshot struct {
	Manager         string                   `json:"manager"`
	Items           []SystemPackage          `json:"items"`
	Maintenance     SystemMaintenanceSummary `json:"maintenance"`
	ResourceVersion string                   `json:"resourceVersion"`
	ObservedAt      time.Time                `json:"observedAt"`
}

type SystemPackagesActionRequest struct {
	Action                  string   `json:"action"`
	Items                   []string `json:"items"`
	ExpectedResourceVersion string   `json:"expectedResourceVersion"`
}

type SystemPackagesActionResult struct {
	TaskID          string    `json:"taskId"`
	Action          string    `json:"action"`
	Items           []string  `json:"items"`
	Status          string    `json:"status"`
	Changed         bool      `json:"changed"`
	Message         string    `json:"message"`
	ResourceVersion string    `json:"resourceVersion"`
	AcceptedAt      time.Time `json:"acceptedAt"`
}

func IsSystemPackage(value string) bool {
	for _, item := range SystemPackageIDs {
		if value == item {
			return true
		}
	}
	return false
}

func ValidateSystemPackagesAction(request *SystemPackagesActionRequest) (string, string) {
	if request == nil {
		return "request", "request is required"
	}
	request.Action = strings.TrimSpace(request.Action)
	if request.Action != "install" && request.Action != "remove" {
		return "action", "action must be install or remove"
	}
	if len(request.Items) < 1 || len(request.Items) > len(SystemPackageIDs) {
		return "items", "items must contain one to 17 entries"
	}
	seen := make(map[string]struct{}, len(request.Items))
	for _, item := range request.Items {
		if !IsSystemPackage(item) {
			return "items", "items contains an unsupported entry"
		}
		if _, exists := seen[item]; exists {
			return "items", "items must not contain duplicates"
		}
		seen[item] = struct{}{}
	}
	if !systemPackageVersionPattern.MatchString(request.ExpectedResourceVersion) {
		return "expectedResourceVersion", "expectedResourceVersion must be 64 lowercase hexadecimal characters"
	}
	return "", ""
}
