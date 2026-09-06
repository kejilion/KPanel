package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResourceAlertRulesRoundTrip(t *testing.T) {
	rules := DefaultRules()
	if err := json.Unmarshal([]byte(`{"resourceAlerts":{"certificatesEnabled":true,"containers":[]}}`), &rules); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"certificatesEnabled":true`) {
		t.Fatalf("resource alert settings lost on round trip: %s", data)
	}
}

type resourceTestSource struct {
	value ResourceSnapshot
	clock *notificationTestClock
	calls int
	stale bool
}

func (r *resourceTestSource) Resources(context.Context) ResourceSnapshot {
	r.calls++
	v := r.value
	v.Certificates = append([]CertificateResource{}, v.Certificates...)
	v.Containers = append([]ContainerResource{}, v.Containers...)
	v.ObservedAt = r.clock.Now()
	if r.stale {
		v.ObservedAt = v.ObservedAt.Add(-10 * time.Minute)
	}
	return v
}

func resourceService(t *testing.T) (*Service, *resourceTestSource, *notificationTestTelegram, *notificationTestClock) {
	t.Helper()
	clock := &notificationTestClock{now: time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC)}
	telegram := &notificationTestTelegram{}
	s := configureNotificationTestService(t, t.TempDir(), newNotificationTestHost(clock.Now()), telegram, clock)
	r := &resourceTestSource{clock: clock, value: ResourceSnapshot{CertificateStatus: "ready", ContainerStatus: "ready"}}
	s.resources = r
	t.Cleanup(func() { _ = s.Close() })
	return s, r, telegram, clock
}

func saveResourceRules(t *testing.T, s *Service, rules *ResourceRules) {
	t.Helper()
	snapshot := s.Snapshot()
	r := snapshot.Rules
	r.ResourceAlerts = rules
	_, err := s.Configure(context.Background(), UpdateInput{Enabled: true, Locale: "zh-CN", Rules: r, ExpectedResourceVersion: snapshot.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
}
func resourceTick(t *testing.T, s *Service, clock *notificationTestClock) {
	t.Helper()
	clock.Advance(30 * time.Second)
	if err := s.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func testCertificate(clock *notificationTestClock) CertificateResource {
	expiry := clock.Now().Add(7 * 24 * time.Hour)
	return CertificateResource{ID: strings.Repeat("a", 64), Name: "example.com", Fingerprint: strings.Repeat("b", 64), ExpiresAt: &expiry, Maintenance: "unknown", Known: true}
}
func testContainer() ContainerResource {
	count := int64(0)
	return ContainerResource{ID: strings.Repeat("c", 64), Name: "important", State: "running", Health: "healthy", RestartCount: &count, ResourceVersion: "sha256:" + strings.Repeat("d", 64), Known: true}
}

func TestCertificateStageExactBoundaries(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		delta time.Duration
		want  string
	}{{30*24*time.Hour + 1, ""}, {30 * 24 * time.Hour, "30"}, {7*24*time.Hour + 1, "30"}, {7 * 24 * time.Hour, "7"}, {24*time.Hour + 1, "7"}, {24 * time.Hour, "1"}, {1, "1"}, {0, "expired"}, {-1, "expired"}} {
		if got := certificateStage(now.Add(tc.delta), now); got != tc.want {
			t.Fatalf("delta=%v got=%q want=%q", tc.delta, got, tc.want)
		}
	}
}

func TestResourceCertificatesDeduplicateRenewUnknownDeleteAndRestart(t *testing.T) {
	s, source, tg, clock := resourceService(t)
	source.value.Certificates = []CertificateResource{testCertificate(clock)}
	saveResourceRules(t, s, &ResourceRules{CertificatesEnabled: true})
	resourceTick(t, s, clock)
	resourceTick(t, s, clock)
	if tg.messageCount() != 1 {
		t.Fatal(tg.messagesSnapshot())
	}
	source.value.Certificates[0].Name = "renamed.example.com"
	resourceTick(t, s, clock)
	if tg.messageCount() != 1 {
		t.Fatal("rename repeated alert")
	}
	source.stale = true
	resourceTick(t, s, clock)
	if tg.messageCount() != 1 {
		t.Fatal("stale fabricated recovery")
	}
	source.stale = false
	source.value.Certificates[0].Known = false
	resourceTick(t, s, clock)
	if tg.messageCount() != 1 {
		t.Fatal("unknown fabricated recovery")
	}
	source.value.Certificates[0].Known = true
	// Persisted delivered state must remain deduplicated after process restart.
	next, err := NewService(Config{DataDir: filepath.Dir(s.store.directory), Hosts: s.hosts, Telegram: tg, Resources: source, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	resourceTick(t, next, clock)
	if tg.messageCount() != 1 {
		t.Fatal("restart repeated alert")
	}
	expiry := clock.Now().Add(90 * 24 * time.Hour)
	source.value.Certificates[0].ExpiresAt = &expiry
	source.value.Certificates[0].Fingerprint = strings.Repeat("e", 64)
	resourceTick(t, next, clock)
	if tg.messageCount() != 2 || !strings.Contains(tg.messagesSnapshot()[1], "已解除") {
		t.Fatal(tg.messagesSnapshot())
	}
	source.value.Certificates = nil
	resourceTick(t, next, clock)
	for key := range next.alertStateSnapshot() {
		if strings.HasPrefix(key, resourceStatePrefix) {
			t.Fatalf("deleted state retained %s", key)
		}
	}
}

func TestResourceContainersSustainUnknownPauseAndRecovery(t *testing.T) {
	s, source, tg, clock := resourceService(t)
	c := testContainer()
	c.Health = "unhealthy"
	source.value.Containers = []ContainerResource{c}
	saveResourceRules(t, s, &ResourceRules{Containers: []ContainerRule{{ID: c.ID, Enabled: true}}})
	resourceTick(t, s, clock)
	resourceTick(t, s, clock)
	if tg.messageCount() != 0 {
		t.Fatal("early alert")
	}
	source.value.ContainerStatus = "unknown"
	resourceTick(t, s, clock)
	source.value.ContainerStatus = "ready"
	resourceTick(t, s, clock)
	resourceTick(t, s, clock)
	if tg.messageCount() != 0 {
		t.Fatal("unknown bridged continuous samples")
	}
	resourceTick(t, s, clock)
	if tg.messageCount() != 1 {
		t.Fatal(tg.messagesSnapshot())
	}
	source.value.Containers[0].Health = "starting"
	resourceTick(t, s, clock)
	if tg.messageCount() != 1 {
		t.Fatal("starting fabricated recovery")
	}
	source.value.Containers[0].Name = "renamed"
	source.value.Containers[0].ResourceVersion = "sha256:" + strings.Repeat("e", 64)
	source.value.Containers[0].Health = "healthy"
	resourceTick(t, s, clock)
	if tg.messageCount() != 2 {
		t.Fatal(tg.messagesSnapshot())
	}
	pause := clock.Now().Add(time.Minute)
	saveResourceRules(t, s, &ResourceRules{Containers: []ContainerRule{{ID: c.ID, Enabled: true, PausedUntil: &pause}}})
	source.value.Containers[0].State = "exited"
	resourceTick(t, s, clock)
	if tg.messageCount() != 2 {
		t.Fatal("maintenance sent")
	}
	clock.Advance(5 * time.Minute)
	for range 3 {
		resourceTick(t, s, clock)
	}
	if tg.messageCount() != 3 || !strings.Contains(tg.messagesSnapshot()[2], "未运行") {
		t.Fatal(tg.messagesSnapshot())
	}
	saveResourceRules(t, s, &ResourceRules{})
	for range 4 {
		resourceTick(t, s, clock)
	}
	if tg.messageCount() != 3 {
		t.Fatal("disabled sent")
	}
}

func TestResourceRestartCounterRequiresObservedIncrements(t *testing.T) {
	s, source, tg, clock := resourceService(t)
	c := testContainer()
	count := int64(900)
	c.RestartCount = &count
	source.value.Containers = []ContainerResource{c}
	saveResourceRules(t, s, &ResourceRules{Containers: []ContainerRule{{ID: c.ID, Enabled: true}}})
	for range 3 {
		resourceTick(t, s, clock)
	}
	if tg.messageCount() != 0 {
		t.Fatal("historical count alerted")
	}
	count = 903
	for range 3 {
		resourceTick(t, s, clock)
	}
	if tg.messageCount() != 1 || !strings.Contains(tg.messagesSnapshot()[0], "至少 3 次") {
		t.Fatal(tg.messagesSnapshot())
	}
	count = 0
	resourceTick(t, s, clock)
	if tg.messageCount() != 2 {
		t.Fatal("counter reset did not resolve observed condition")
	}
}

func TestResourceSendingBudgetCapacityAndTelegramRetry(t *testing.T) {
	s, source, tg, clock := resourceService(t)
	for i := range 12 {
		c := testCertificate(clock)
		c.ID = fmt.Sprintf("%064x", i+1)
		source.value.Certificates = append(source.value.Certificates, c)
	}
	saveResourceRules(t, s, &ResourceRules{CertificatesEnabled: true})
	resourceTick(t, s, clock)
	if tg.messageCount() != 8 {
		t.Fatalf("budget %d", tg.messageCount())
	}
	resourceTick(t, s, clock)
	if tg.messageCount() != 12 {
		t.Fatalf("pending %d", tg.messageCount())
	}
	s.mu.Lock()
	s.alerts = map[string]alertState{}
	for i := range MaxAlertStates {
		s.alerts[fmt.Sprintf("host-1:full-%d", i)] = alertState{Active: true}
	}
	s.mu.Unlock()
	resourceTick(t, s, clock)
	if tg.messageCount() != 12 || len(s.alertStateSnapshot()) != MaxAlertStates {
		t.Fatal("capacity exceeded or untracked notification sent")
	}
	s.mu.Lock()
	s.alerts = map[string]alertState{}
	s.mu.Unlock()
	source.value.Certificates = source.value.Certificates[:1]
	tg.sendErr = errors.New("test Telegram failure")
	resourceTick(t, s, clock)
	state := s.getAlertState(resourceStatePrefix + "certificate:" + source.value.Certificates[0].ID)
	first := state.LastAttemptAt
	tg.sendErr = nil
	resourceTick(t, s, clock)
	if !s.getAlertState(resourceStatePrefix+"certificate:"+source.value.Certificates[0].ID).LastAttemptAt.Equal(first) || tg.messageCount() != 12 {
		t.Fatal("retried before five minutes")
	}
	clock.Advance(5 * time.Minute)
	resourceTick(t, s, clock)
	if tg.messageCount() != 13 {
		t.Fatal("failed notification not retried")
	}
}

func TestResourceRulesOldClientPreservesAndLimits(t *testing.T) {
	s, source, _, clock := resourceService(t)
	c := testContainer()
	source.value.Containers = []ContainerResource{c}
	saveResourceRules(t, s, &ResourceRules{CertificatesEnabled: true, Containers: []ContainerRule{{ID: c.ID, Enabled: true}}})
	old := s.Snapshot()
	old.Rules.ResourceAlerts = nil
	got, err := s.Configure(context.Background(), UpdateInput{Enabled: true, Rules: old.Rules, ExpectedResourceVersion: old.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if got.Rules.ResourceAlerts == nil || !got.Rules.ResourceAlerts.CertificatesEnabled || len(got.Rules.ResourceAlerts.Containers) != 1 {
		t.Fatal("old client erased rules")
	}
	tooLong := clock.Now().Add(25 * time.Hour)
	if validateResourceRules(&ResourceRules{PausedUntil: &tooLong}, clock.Now()) == nil {
		t.Fatal("unbounded pause")
	}
	if validateResourceRules(&ResourceRules{Containers: []ContainerRule{{ID: c.ID}, {ID: c.ID}}}, clock.Now()) == nil {
		t.Fatal("duplicate selection")
	}
	source.value.Containers = nil
	clock.Advance(time.Minute)
	got.Resources = s.resourceSnapshot(context.Background())
	saveResourceRules(t, s, &ResourceRules{})
	next := s.Snapshot()
	next.Rules.ResourceAlerts = &ResourceRules{Containers: []ContainerRule{{ID: c.ID, Enabled: true}}}
	_, err = s.Configure(context.Background(), UpdateInput{Enabled: true, Rules: next.Rules, ExpectedResourceVersion: next.ResourceVersion})
	if err == nil {
		t.Fatal("deleted ID silently selected")
	}
}

func TestResourceReadCacheAndDisabledBudget(t *testing.T) {
	s, source, _, clock := resourceService(t)
	for range 20 {
		s.Snapshot()
	}
	if source.calls != 1 {
		t.Fatalf("uncached source calls %d", source.calls)
	}
	for range 3 {
		resourceTick(t, s, clock)
	}
	if source.calls != 1 {
		t.Fatal("disabled resources polled")
	}
}

func TestResourceContainerSamplesSurviveRestartAndPauseDeadline(t *testing.T) {
	s, source, tg, clock := resourceService(t)
	c := testContainer()
	c.State = "exited"
	source.value.Containers = []ContainerResource{c}
	saveResourceRules(t, s, &ResourceRules{Containers: []ContainerRule{{ID: c.ID, Enabled: true}}})
	resourceTick(t, s, clock)
	resourceTick(t, s, clock)
	next, err := NewService(Config{DataDir: filepath.Dir(s.store.directory), Hosts: s.hosts, Telegram: tg, Resources: source, Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	resourceTick(t, next, clock)
	if tg.messageCount() != 1 {
		t.Fatal("restart lost continuous samples")
	}
	until := clock.Now().Add(time.Minute)
	saveResourceRules(t, next, &ResourceRules{PausedUntil: &until, Containers: []ContainerRule{{ID: c.ID, Enabled: true}}})
	source.value.Containers[0].State = "running"
	resourceTick(t, next, clock)
	if tg.messageCount() != 1 {
		t.Fatal("global pause sent recovery")
	}
	resourceTick(t, next, clock)
	if tg.messageCount() != 2 {
		t.Fatal("pause did not expire at exact deadline")
	}
}

func TestResourceSnapshotRejectsStaleFutureDuplicateAndOversize(t *testing.T) {
	now := time.Now()
	base := ResourceSnapshot{CertificateStatus: "ready", ContainerStatus: "ready", ObservedAt: now}
	for _, delta := range []time.Duration{-91 * time.Second, 6 * time.Second} {
		v := base
		v.ObservedAt = now.Add(delta)
		got := sanitizeResources(v, now)
		if got.ContainerStatus != "unknown" || got.CertificateStatus != "unknown" {
			t.Fatal("invalid observed time accepted")
		}
	}
	v := base
	v.Containers = []ContainerResource{testContainer(), testContainer()}
	if sanitizeResources(v, now).ContainerStatus != "unknown" {
		t.Fatal("duplicate IDs accepted")
	}
	v = base
	v.Certificates = make([]CertificateResource, 129)
	if sanitizeResources(v, now).CertificateStatus != "limited" {
		t.Fatal("oversize certificate list accepted")
	}
}

func TestResourceFullStoreAlsoPreventsUntrackedHostDelivery(t *testing.T) {
	s, _, tg, clock := resourceService(t)
	s.mu.Lock()
	for i := range MaxAlertStates {
		s.alerts[fmt.Sprintf("host-1:capacity-%d", i)] = alertState{}
	}
	s.mu.Unlock()
	host := s.hosts.Hosts(context.Background()).Items[0]
	s.handleCumulativeThreshold(host, "traffic-total-received", 100<<30, 1, clock.Now(), "zh-CN", func(message string) (bool, bool) {
		_ = tg.SendMessage(context.Background(), "", 0, message)
		return true, true
	})
	if tg.messageCount() != 0 {
		t.Fatal("full shared state store sent untracked host alert")
	}
}
