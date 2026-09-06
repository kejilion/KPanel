package contract

import "time"

// LightNodeHealth is a credential-free observation, never an authorization or
// proof of broker connectivity. Only the reporting process supplies its version.
type LightNodeHealth struct {
	ObservedAt     time.Time               `json:"observedAt"`
	RuntimeVersion string                  `json:"runtimeVersion"`
	Update         LightNodeUpdateHealth   `json:"update"`
	Services       LightNodeServicesHealth `json:"services"`
}

type LightNodeUpdateHealth struct {
	State      string `json:"state"`
	CheckedAt  int64  `json:"checkedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
	ErrorCode  string `json:"errorCode,omitempty"`
}

type LightNodeServiceHealth struct {
	LoadState     string `json:"loadState"`
	ActiveState   string `json:"activeState"`
	SubState      string `json:"subState"`
	UnitFileState string `json:"unitFileState"`
}

// Fixed fields prevent arbitrary unit names and unbounded service inventories.
type LightNodeServicesHealth struct {
	Timer     LightNodeServiceHealth `json:"timer"`
	Telemetry LightNodeServiceHealth `json:"telemetry"`
	Terminal  LightNodeServiceHealth `json:"terminal"`
	File      LightNodeServiceHealth `json:"file"`
	SSHLogin  LightNodeServiceHealth `json:"sshLogin"`
}

func lightHealthEnum(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func ValidLightNodeUpdateHealth(v LightNodeUpdateHealth, now time.Time) bool {
	if !lightHealthEnum(v.State, "missing", "invalid", "unavailable", "running", "interrupted", "current", "updated", "degraded", "failed", "rolled_back") ||
		!lightHealthEnum(v.ErrorCode, "", "internal", "release_check", "manifest", "download", "checksum", "protocol", "config", "restart", "rollback", "optional_service", "interrupted") {
		return false
	}
	if v.CheckedAt < 0 || v.FinishedAt < 0 || v.CheckedAt > now.Add(time.Minute).Unix() || v.FinishedAt > now.Add(time.Minute).Unix() {
		return false
	}
	if lightHealthEnum(v.State, "missing", "invalid", "unavailable") {
		return v.CheckedAt == 0 && v.FinishedAt == 0 && v.ErrorCode == ""
	}
	if v.CheckedAt == 0 || (v.FinishedAt != 0 && v.FinishedAt < v.CheckedAt) {
		return false
	}
	if v.State == "running" {
		return v.FinishedAt == 0 && v.ErrorCode == ""
	}
	if v.State == "interrupted" {
		return v.ErrorCode == "interrupted"
	}
	if v.FinishedAt == 0 {
		return false
	}
	if lightHealthEnum(v.State, "current", "updated") {
		return v.ErrorCode == ""
	}
	return v.ErrorCode != ""
}

func ValidLightNodeHealth(v LightNodeHealth, now time.Time) bool {
	if v.ObservedAt.IsZero() || v.ObservedAt.After(now.Add(time.Minute)) || v.ObservedAt.Before(now.Add(-5*time.Minute)) || len(v.RuntimeVersion) == 0 || len(v.RuntimeVersion) > 64 {
		return false
	}
	for _, r := range v.RuntimeVersion {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+') {
			return false
		}
	}
	if !ValidLightNodeUpdateHealth(v.Update, now) {
		return false
	}
	for _, s := range []LightNodeServiceHealth{v.Services.Timer, v.Services.Telemetry, v.Services.Terminal, v.Services.File, v.Services.SSHLogin} {
		if !lightHealthEnum(s.LoadState, "unknown", "loaded", "not-found", "masked", "error", "bad-setting") ||
			!lightHealthEnum(s.ActiveState, "unknown", "active", "inactive", "failed", "activating", "deactivating", "reloading") ||
			!lightHealthEnum(s.SubState, "unknown", "running", "waiting", "dead", "failed", "exited", "start", "stop", "auto-restart", "elapsed") ||
			!lightHealthEnum(s.UnitFileState, "unknown", "enabled", "enabled-runtime", "disabled", "masked", "masked-runtime", "static", "indirect", "generated", "transient", "alias", "linked", "linked-runtime") {
			return false
		}
	}
	return true
}
