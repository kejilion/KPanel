package contract

import (
	"path"
	"strings"
	"time"
)

const (
	VirusScanModeFull      = "full"
	VirusScanModeImportant = "important"
	VirusScanModeCustom    = "custom"
	VirusScanMaxPaths      = 8
	VirusScanMaxPathBytes  = 512
)

type VirusScanActionRequest struct {
	Mode  string   `json:"mode"`
	Paths []string `json:"paths,omitempty"`
}

type VirusScanActionResult struct {
	Status    string    `json:"status"`
	TaskID    string    `json:"taskId"`
	Mode      string    `json:"mode"`
	Paths     []string  `json:"paths,omitempty"`
	Message   string    `json:"message"`
	AppliedAt time.Time `json:"appliedAt"`
}

type VirusScanSnapshot struct {
	ReportAvailable bool                     `json:"reportAvailable"`
	ReportPath      string                   `json:"reportPath"`
	Source          string                   `json:"source,omitempty"`
	Status          string                   `json:"status"`
	Mode            string                   `json:"mode,omitempty"`
	Paths           []string                 `json:"paths,omitempty"`
	ScannedFiles    uint64                   `json:"scannedFiles"`
	InfectedFiles   uint64                   `json:"infectedFiles"`
	Errors          uint64                   `json:"errors"`
	Findings        []string                 `json:"findings"`
	Truncated       bool                     `json:"truncated"`
	CompletedAt     *time.Time               `json:"completedAt,omitempty"`
	ObservedAt      time.Time                `json:"observedAt"`
	Maintenance     SystemMaintenanceSummary `json:"maintenance"`
}

func ValidateVirusScanAction(input *VirusScanActionRequest) (string, string) {
	if input == nil {
		return "request", "request is required"
	}
	input.Mode = strings.TrimSpace(input.Mode)
	switch input.Mode {
	case VirusScanModeFull, VirusScanModeImportant:
		if len(input.Paths) != 0 {
			return "paths", "paths are only allowed for custom scans"
		}
		return "", ""
	case VirusScanModeCustom:
	default:
		return "mode", "mode must be full, important, or custom"
	}
	if len(input.Paths) < 1 || len(input.Paths) > VirusScanMaxPaths {
		return "paths", "custom scans require one to eight paths"
	}
	seen := make(map[string]struct{}, len(input.Paths))
	for index, value := range input.Paths {
		value = strings.TrimSpace(value)
		if len(value) < 1 || len(value) > VirusScanMaxPathBytes || !path.IsAbs(value) || path.Clean(value) != value ||
			strings.Contains(value, ",") || strings.ContainsAny(value, "\x00\r\n") {
			return "paths", "every path must be a canonical absolute Linux directory path without commas or control characters"
		}
		if _, ok := seen[value]; ok {
			return "paths", "paths must not contain duplicates"
		}
		seen[value] = struct{}{}
		input.Paths[index] = value
	}
	return "", ""
}
