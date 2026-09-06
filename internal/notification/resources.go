package notification

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const MaxNotificationResources = 128
const MaxSelectedContainers = 64
const resourceStatePrefix = "resource:"
const resourceFreshness = 90 * time.Second
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

type ResourceSource interface {
	Resources(context.Context) ResourceSnapshot
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

func paused(until *time.Time, now time.Time) bool { return until != nil && now.Before(*until) }

func unknownResources() ResourceSnapshot {
	return ResourceSnapshot{Certificates: []CertificateResource{}, Containers: []ContainerResource{}, CertificateStatus: "unknown", ContainerStatus: "unknown"}
}

// This cache is bounded, in memory only, and shared with the existing 30 second
// evaluator. Opening multiple dialogs cannot multiply Agent discovery work.
func (s *Service) resourceSnapshot(ctx context.Context) ResourceSnapshot {
	s.resourceMu.Lock()
	defer s.resourceMu.Unlock()
	now := s.now()
	if s.resources == nil {
		return unknownResources()
	}
	if s.resourceFetchedAt.IsZero() || now.Sub(s.resourceFetchedAt) >= defaultEvaluationInterval || now.Before(s.resourceFetchedAt) {
		fetchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		s.resourceCache = sanitizeResources(s.resources.Resources(fetchCtx), now)
		cancel()
		s.resourceFetchedAt = now
	}
	result := s.resourceCache
	s.mu.Lock()
	result.StateCapacityReached = len(s.alerts) >= MaxAlertStates
	s.mu.Unlock()
	return result
}

func sanitizeResources(value ResourceSnapshot, now time.Time) ResourceSnapshot {
	if value.ObservedAt.IsZero() || value.ObservedAt.After(now.Add(5*time.Second)) || now.Sub(value.ObservedAt) > resourceFreshness {
		return unknownResources()
	}
	if len(value.Certificates) > MaxNotificationResources {
		value.Certificates = nil
		value.CertificateStatus = "limited"
	}
	if len(value.Containers) > MaxNotificationResources {
		value.Containers = nil
		value.ContainerStatus = "limited"
	}
	if value.CertificateStatus != "ready" {
		value.Certificates = nil
		if value.CertificateStatus != "limited" {
			value.CertificateStatus = "unknown"
		}
	}
	if value.ContainerStatus != "ready" {
		value.Containers = nil
		if value.ContainerStatus != "limited" {
			value.ContainerStatus = "unknown"
		}
	}
	seen := map[string]bool{}
	for i := range value.Certificates {
		r := &value.Certificates[i]
		if !validHexString(r.ID, 64) || seen[r.ID] || !validDisplayText(r.Name, 253) {
			value.CertificateStatus = "unknown"
			value.Certificates = nil
			break
		}
		seen[r.ID] = true
		if !validHexString(r.Fingerprint, 64) || r.ExpiresAt == nil || r.ExpiresAt.IsZero() {
			r.Known = false
		}
		if r.Maintenance != "custom" && r.Maintenance != "automatic" {
			r.Maintenance = "unknown"
		}
	}
	seen = map[string]bool{}
	for i := range value.Containers {
		r := &value.Containers[i]
		if !validHexString(r.ID, 64) || seen[r.ID] || !validDisplayText(r.Name, 253) {
			value.ContainerStatus = "unknown"
			value.Containers = nil
			break
		}
		seen[r.ID] = true
		if !validResourceVersion(r.ResourceVersion) || r.RestartCount == nil || *r.RestartCount < 0 {
			r.Known = false
		}
		switch r.State {
		case "running", "paused", "restarting", "exited", "dead", "created", "removing":
		default:
			r.Known = false
		}
		switch r.Health {
		case "", "healthy", "unhealthy", "starting":
		default:
			r.Known = false
		}
	}
	if value.Certificates == nil {
		value.Certificates = []CertificateResource{}
	}
	if value.Containers == nil {
		value.Containers = []ContainerResource{}
	}
	return value
}

func certificateStage(expires, now time.Time) string {
	delta := expires.Sub(now)
	switch {
	case delta <= 0:
		return "expired"
	case delta <= 24*time.Hour:
		return "1"
	case delta <= 7*24*time.Hour:
		return "7"
	case delta <= 30*24*time.Hour:
		return "30"
	default:
		return ""
	}
}

// Reserve before attempting delivery. A full state store must never cause
// untracked messages to be sent again at every evaluation.
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

func (s *Service) evaluateResources(ctx context.Context, rules *ResourceRules, now time.Time, locale string, send func(string) (bool, bool)) {
	if rules == nil {
		return
	}
	anyEnabled := rules.CertificatesEnabled
	for _, r := range rules.Containers {
		anyEnabled = anyEnabled || r.Enabled
	}
	if !anyEnabled {
		return
	}
	snapshot := s.resourceSnapshot(ctx)
	known := map[string]bool{}
	for _, c := range snapshot.Certificates {
		known[resourceStatePrefix+"certificate:"+c.ID] = true
	}
	for _, c := range snapshot.Containers {
		known[resourceStatePrefix+"container:"+c.ID] = true
	}
	selected := map[string]ContainerRule{}
	for _, c := range rules.Containers {
		selected[resourceStatePrefix+"container:"+c.ID] = c
	}
	s.mu.Lock()
	for key := range s.alerts {
		if strings.HasPrefix(key, resourceStatePrefix+"certificate:") && (!rules.CertificatesEnabled || (snapshot.CertificateStatus == "ready" && !known[key])) {
			delete(s.alerts, key)
		}
		if strings.HasPrefix(key, resourceStatePrefix+"container:") {
			r, ok := selected[key]
			if !ok || !r.Enabled || (snapshot.ContainerStatus == "ready" && !known[key]) {
				delete(s.alerts, key)
			}
		}
	}
	s.mu.Unlock()
	if paused(rules.PausedUntil, now) {
		s.resetResourceSamples()
		return
	}
	if rules.CertificatesEnabled && snapshot.CertificateStatus == "ready" {
		for _, c := range snapshot.Certificates {
			key := resourceStatePrefix + "certificate:" + c.ID
			if !c.Known {
				continue
			}
			state, ok := s.reserveAlertState(key)
			if !ok {
				continue
			}
			stage := certificateStage(*c.ExpiresAt, now)
			event := c.Fingerprint + ":" + stage
			if stage == "" {
				if state.Active && canRecoveryAttempt(state, now) {
					delivered, attempted := send(resourceMessage("certificate-recovered", c.Name, c.Maintenance, *c.ExpiresAt, now, locale))
					if attempted {
						state.LastAttemptAt = now
					}
					if delivered {
						state.Active = false
						state.LastEventID = ""
						state.LastNotifiedAt = now
					}
				}
			} else if state.LastEventID != event && canAlertAttempt(state, now) {
				delivered, attempted := send(resourceMessage("certificate-"+stage, c.Name, c.Maintenance, *c.ExpiresAt, now, locale))
				if attempted {
					state.LastAttemptAt = now
				}
				if delivered {
					state.Active = true
					state.LastEventID = event
					state.LastNotifiedAt = now
				}
			}
			s.setAlertState(key, state)
		}
	}
	if snapshot.ContainerStatus != "ready" {
		s.resetContainerSamples()
		return
	}
	for _, c := range snapshot.Containers {
		key := resourceStatePrefix + "container:" + c.ID
		rule, ok := selected[key]
		if !ok || !rule.Enabled {
			continue
		}
		state, ok := s.reserveAlertState(key)
		if !ok {
			continue
		}
		if !c.Known || paused(rule.PausedUntil, now) {
			state.Consecutive = 0
			state.RestartWindowAt = time.Time{}
			s.setAlertState(key, state)
			continue
		}
		if !state.LastSampleAt.IsZero() && !snapshot.ObservedAt.After(state.LastSampleAt) {
			continue
		}
		if !state.LastSampleAt.IsZero() && snapshot.ObservedAt.Sub(state.LastSampleAt) > resourceFreshness {
			state.Consecutive = 0
			state.RestartWindowAt = time.Time{}
		}
		state.LastSampleAt = snapshot.ObservedAt
		count := *c.RestartCount
		if state.RestartWindowAt.IsZero() || count < state.RestartCount || now.Sub(state.RestartWindowAt) > restartWindow {
			state.RestartWindowAt = now
			state.RestartBaseline = count
		}
		state.RestartCount = count
		reason := ""
		switch {
		case c.State == "restarting":
			reason = "restarting"
		case c.State != "running":
			reason = "not-running"
		case c.Health == "unhealthy":
			reason = "unhealthy"
		case count-state.RestartBaseline >= 3:
			reason = "repeated-restarts"
		}
		// A health check still starting cannot prove recovery from unhealthy.
		if reason == "" && c.Health == "starting" {
			state.Consecutive = 0
			s.setAlertState(key, state)
			continue
		}
		if reason == "" {
			state.Consecutive = 0
			state.PendingEventID = ""
			if state.Active && canRecoveryAttempt(state, now) {
				delivered, attempted := send(resourceMessage("container-recovered", c.Name, "", time.Time{}, now, locale))
				if attempted {
					state.LastAttemptAt = now
				}
				if delivered {
					state.Active = false
					state.LastNotifiedAt = now
					state.LastEventID = ""
				}
			}
		} else {
			if state.PendingEventID != reason {
				state.Consecutive = 0
				state.PendingEventID = reason
			}
			state.Consecutive = minInt(state.Consecutive+1, s.sustain)
			if state.Consecutive >= s.sustain && (!state.Active || state.LastEventID != reason || now.Sub(state.LastNotifiedAt) >= s.repeat) && canAlertAttempt(state, now) {
				delivered, attempted := send(resourceMessage("container-"+reason, c.Name, "", time.Time{}, now, locale))
				if attempted {
					state.LastAttemptAt = now
				}
				if delivered {
					state.Active = true
					state.LastEventID = reason
					state.LastNotifiedAt = now
				}
			}
		}
		s.setAlertState(key, state)
	}
}

func (s *Service) resetResourceSamples() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.alerts {
		if strings.HasPrefix(k, resourceStatePrefix) {
			v.Consecutive = 0
			v.RestartWindowAt = time.Time{}
			s.alerts[k] = v
		}
	}
}
func (s *Service) resetContainerSamples() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range s.alerts {
		if strings.HasPrefix(k, resourceStatePrefix+"container:") {
			v.Consecutive = 0
			v.RestartWindowAt = time.Time{}
			s.alerts[k] = v
		}
	}
}

func resourceMessage(kind, name, maintenance string, expires, now time.Time, locale string) string {
	labels := map[string][3]string{
		"certificate-30":              {"证书将在 30 天内到期", "憑證將於 30 天內到期", "Certificate expires within 30 days"},
		"certificate-7":               {"证书将在 7 天内到期", "憑證將於 7 天內到期", "Certificate expires within 7 days"},
		"certificate-1":               {"证书将在 1 天内到期", "憑證將於 1 天內到期", "Certificate expires within 1 day"},
		"certificate-expired":         {"证书已过期", "憑證已過期", "Certificate expired"},
		"certificate-recovered":       {"证书到期提醒已解除", "憑證到期提醒已解除", "Certificate expiry alert resolved"},
		"container-unhealthy":         {"容器持续不健康", "容器持續不健康", "Container remains unhealthy"},
		"container-not-running":       {"容器持续未运行", "容器持續未執行", "Container remains not running"},
		"container-restarting":        {"容器持续重启中", "容器持續重新啟動中", "Container remains restarting"},
		"container-repeated-restarts": {"容器 5 分钟内重启至少 3 次", "容器 5 分鐘內重新啟動至少 3 次", "Container restarted at least 3 times within 5 minutes"},
		"container-recovered":         {"容器状态提醒已解除", "容器狀態提醒已解除", "Container state alert resolved"},
	}
	index := 0
	if locale == "zh-TW" {
		index = 1
	}
	if locale == "en-US" {
		index = 2
	}
	message := fmt.Sprintf("KPanel · %s\n%s\n%s", labels[kind][index], safeMessageText(name), formatNotificationTime(now))
	if strings.HasPrefix(kind, "certificate-") {
		m := map[string][3]string{"custom": {"自有证书，请自行换证", "自有憑證，請自行換證", "Custom certificate; replace it manually"}, "automatic": {"具备脚本自动续期资格；不代表续期成功", "具備指令碼自動續期資格；不代表續期成功", "Eligible for script renewal; renewal success is unconfirmed"}, "unknown": {"维护方式未知", "維護方式未知", "Maintenance mode unknown"}}
		message += fmt.Sprintf("\n%s\n%s", formatNotificationTime(expires.In(now.Location())), m[maintenance][index])
	}
	return message
}

func sortedResourceRules(rules *ResourceRules) *ResourceRules {
	r := cloneResourceRules(rules)
	if r != nil {
		sort.Slice(r.Containers, func(i, j int) bool { return r.Containers[i].ID < r.Containers[j].ID })
	}
	return r
}

func (s *Service) validateNewContainerSelections(ctx context.Context, previous, next *ResourceRules) error {
	if next == nil {
		return nil
	}
	existing := map[string]bool{}
	if previous != nil {
		for _, r := range previous.Containers {
			existing[r.ID] = r.Enabled
		}
	}
	var inventory *ResourceSnapshot
	for _, r := range next.Containers {
		if !r.Enabled || existing[r.ID] {
			continue
		}
		if inventory == nil {
			value := s.resourceSnapshot(ctx)
			inventory = &value
		}
		found := false
		if inventory.ContainerStatus == "ready" {
			for _, c := range inventory.Containers {
				if c.ID == r.ID {
					found = true
					break
				}
			}
		}
		if !found {
			return &ValidationError{Field: "rules.resourceAlerts.containers", Message: "请重新读取本机容器后再选择提醒"}
		}
	}
	return nil
}

func reconcileResourceAlertStates(states map[string]alertState, previous, next *ResourceRules) {
	selected := map[string]ContainerRule{}
	if next != nil {
		for _, r := range next.Containers {
			selected[resourceStatePrefix+"container:"+r.ID] = r
		}
	}
	for key, value := range states {
		if !strings.HasPrefix(key, resourceStatePrefix) {
			continue
		}
		if strings.HasPrefix(key, resourceStatePrefix+"certificate:") && (next == nil || !next.CertificatesEnabled) {
			delete(states, key)
			continue
		}
		if strings.HasPrefix(key, resourceStatePrefix+"container:") {
			r, ok := selected[key]
			if !ok || !r.Enabled {
				delete(states, key)
				continue
			}
		}
		// Any explicit configuration save starts a new continuous observation
		// interval, while retaining delivered-event deduplication and recovery.
		value.Consecutive = 0
		value.RestartWindowAt = time.Time{}
		states[key] = value
	}
}
