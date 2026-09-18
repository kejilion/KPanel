package notification

import "time"

const MaxNotificationResources = 128
const MaxSelectedContainers = 64
const resourceStatePrefix = "resource:"
const restartWindow = 5 * time.Minute

type ResourceRules struct {
	CertificatesEnabled bool            `json:"certificatesEnabled"`
	PausedUntil         *time.Time      `json:"pausedUntil,omitempty"`
	Containers          []ContainerRule `json:"containers"`
}

type ContainerRule struct {
	ID          string     `json:"id"`
	Enabled     bool       `json:"enabled"`
	PausedUntil *time.Time `json:"pausedUntil,omitempty"`
}

type CertificateResource struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	Maintenance string     `json:"maintenance"`
	Known       bool       `json:"known"`
}

type ContainerResource struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	State           string `json:"state"`
	Health          string `json:"health,omitempty"`
	RestartCount    *int64 `json:"restartCount,omitempty"`
	ResourceVersion string `json:"resourceVersion"`
	Known           bool   `json:"known"`
}

// Source status is independent per collection. Only a complete, fresh result
// can prove absence; unknown/limited results cannot resolve an existing alert.
type ResourceSnapshot struct {
	Certificates         []CertificateResource `json:"certificates"`
	Containers           []ContainerResource   `json:"containers"`
	CertificateStatus    string                `json:"certificateStatus"`
	ContainerStatus      string                `json:"containerStatus"`
	ObservedAt           time.Time             `json:"observedAt"`
	StateCapacityReached bool                  `json:"stateCapacityReached"`
}

func cloneResourceRules(value *ResourceRules) *ResourceRules {
	if value == nil {
		return nil
	}
	copy := *value
	copy.PausedUntil = cloneTime(value.PausedUntil)
	copy.Containers = append([]ContainerRule{}, value.Containers...)
	for i := range copy.Containers {
		copy.Containers[i].PausedUntil = cloneTime(copy.Containers[i].PausedUntil)
	}
	return &copy
}

func validateResourceRules(rules *ResourceRules, now time.Time) error {
	if rules == nil {
		return nil
	}
	if len(rules.Containers) > MaxSelectedContainers {
		return &ValidationError{Field: "rules.resourceAlerts.containers", Message: "最多选择 64 个容器"}
	}
	validPause := func(value *time.Time) bool {
		return value == nil || (!value.IsZero() && (now.IsZero() || !value.After(now.Add(24*time.Hour))))
	}
	if !validPause(rules.PausedUntil) {
		return &ValidationError{Field: "rules.resourceAlerts.pausedUntil", Message: "维护暂停最多 24 小时"}
	}
	seen := map[string]bool{}
	for _, rule := range rules.Containers {
		if !validHexString(rule.ID, 64) || seen[rule.ID] || !validPause(rule.PausedUntil) {
			return &ValidationError{Field: "rules.resourceAlerts.containers", Message: "容器选择或维护暂停无效"}
		}
		seen[rule.ID] = true
	}
	return nil
}

func unknownResources() ResourceSnapshot {
	return ResourceSnapshot{Certificates: []CertificateResource{}, Containers: []ContainerResource{}, CertificateStatus: "unknown", ContainerStatus: "unknown"}
}

// This cache is bounded, in memory only, and shared with the existing 30 second
// evaluator. Opening multiple dialogs cannot multiply Agent discovery work.
func (s *Service) reserveAlertState(key string) (alertState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, ok := s.alerts[key]; ok {
		return state, true
	}
	if len(s.alerts) >= MaxAlertStates {
		return alertState{}, false
	}
	s.alerts[key] = alertState{}
	return alertState{}, true
}
