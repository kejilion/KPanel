package contract

import (
	"net/url"
	"time"
)

const (
	SystemLogMaxMessageBytes = 16 << 10
	SystemLogMaxOutputBytes  = 4 << 20
)

type SystemLogUsage struct {
	Available bool   `json:"available"`
	Bytes     uint64 `json:"bytes"`
	Reason    string `json:"reason,omitempty"`
}

type SystemLogSourceStatus struct {
	Available        bool   `json:"available"`
	Reason           string `json:"reason,omitempty"`
	SupportsPriority bool   `json:"supportsPriority,omitempty"`
}

type SystemLogSources struct {
	System   SystemLogSourceStatus `json:"system"`
	Journal  SystemLogSourceStatus `json:"journal"`
	Login    SystemLogSourceStatus `json:"login"`
	Security SystemLogSourceStatus `json:"security"`
}

type SystemLogSummary struct {
	ObservedAt  time.Time                `json:"observedAt"`
	VarLog      SystemLogUsage           `json:"varLog"`
	Journal     SystemLogUsage           `json:"journal"`
	Sources     SystemLogSources         `json:"sources"`
	AuthSource  string                   `json:"authSource,omitempty"`
	Maintenance SystemMaintenanceSummary `json:"maintenance"`
}

type SystemLogQuery struct {
	Source   string
	Limit    int
	Priority string
}

type SystemLogEntry struct {
	Timestamp  *time.Time `json:"timestamp,omitempty"`
	Cursor     string     `json:"cursor,omitempty"`
	Priority   string     `json:"priority,omitempty"`
	Unit       string     `json:"unit,omitempty"`
	Identifier string     `json:"identifier,omitempty"`
	PID        int        `json:"pid,omitempty"`
	Message    string     `json:"message"`
}

type SystemLogSnapshot struct {
	Source     string           `json:"source"`
	AuthSource string           `json:"authSource,omitempty"`
	Entries    []SystemLogEntry `json:"entries"`
	Truncated  bool             `json:"truncated"`
	ObservedAt time.Time        `json:"observedAt"`
}

func ValidSystemLogQuery(query SystemLogQuery) bool {
	if query.Limit != 50 && query.Limit != 100 && query.Limit != 200 {
		return false
	}
	switch query.Priority {
	case "all", "warning", "error":
	default:
		return false
	}
	switch query.Source {
	case "system", "service":
		return true
	case "security", "login":
		return query.Priority == "all"
	default:
		return false
	}
}

// ParseSystemLogQuery accepts only the small, documented filter surface. It
// deliberately rejects duplicate and unknown query keys before any value can
// reach journalctl or last.
func ParseSystemLogQuery(values url.Values) (SystemLogQuery, string, string) {
	for key, entries := range values {
		if len(entries) != 1 {
			return SystemLogQuery{}, key, "query parameters must not be repeated"
		}
		switch key {
		case "source", "limit", "priority":
		default:
			return SystemLogQuery{}, key, "query parameter is not supported"
		}
	}

	query := SystemLogQuery{
		Source: values.Get("source"), Limit: 100, Priority: "all",
	}
	switch query.Source {
	case "system", "service", "security", "login":
	default:
		return SystemLogQuery{}, "source", "source must be system, service, security, or login"
	}

	if raw := values.Get("limit"); raw != "" {
		switch raw {
		case "50":
			query.Limit = 50
		case "100":
			query.Limit = 100
		case "200":
			query.Limit = 200
		default:
			return SystemLogQuery{}, "limit", "limit must be 50, 100, or 200"
		}
	}

	if raw, present := values["priority"]; present {
		query.Priority = raw[0]
		if query.Source != "system" && query.Source != "service" {
			return SystemLogQuery{}, "priority", "priority is only allowed for system or service logs"
		}
	}
	switch query.Priority {
	case "all", "warning", "error":
	default:
		return SystemLogQuery{}, "priority", "priority must be all, warning, or error"
	}
	if !ValidSystemLogQuery(query) {
		return SystemLogQuery{}, "query", "system log query is invalid"
	}
	return query, "", ""
}
