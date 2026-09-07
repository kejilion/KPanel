package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
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

// Seed the on-disk shape produced by the released implementation. Configure
// intentionally cannot enable or edit these rules once the feature is withdrawn.
func seedDormantResources(t *testing.T, s *Service, clock *notificationTestClock) (*ResourceRules, map[string]alertState) {
	t.Helper()
	until := clock.Now().Add(time.Hour)
	rules := &ResourceRules{CertificatesEnabled: true, PausedUntil: &until, Containers: []ContainerRule{{ID: testContainer().ID, Enabled: true, PausedUntil: &until}}}
	states := map[string]alertState{
		resourceStatePrefix + "certificate:" + strings.Repeat("a", 64): {Active: true, LastEventID: strings.Repeat("b", 64) + ":7", LastAttemptAt: clock.Now().Add(-time.Hour), LastNotifiedAt: clock.Now().Add(-24 * time.Hour)},
		resourceStatePrefix + "container:" + testContainer().ID:        {Active: true, LastEventID: "unhealthy", PendingEventID: "restarting", Consecutive: 3, LastAttemptAt: clock.Now().Add(-time.Hour), RestartWindowAt: clock.Now(), RestartBaseline: 1, RestartCount: 4},
	}
	state := s.store.stateSnapshot()
	state.Settings.Rules.ResourceAlerts = cloneResourceRules(rules)
	state.AlertStates = states
	state.ResourceVersion = configResourceVersion(state.Settings, state.Telegram)
	if err := s.store.commitState(state); err != nil {
		t.Fatal(err)
	}
	s.alerts = s.store.stateSnapshot().AlertStates
	return rules, states
}

func TestWithdrawnResourcesRemainDormantAcrossRestartAndPauseExpiry(t *testing.T) {
	for _, healthy := range []bool{false, true} {
		t.Run(fmt.Sprintf("healthy=%v", healthy), func(t *testing.T) {
			s, source, tg, clock := resourceService(t)
			rules, states := seedDormantResources(t, s, clock)
			cert, container := testCertificate(clock), testContainer()
			if healthy {
				expiry := clock.Now().Add(90 * 24 * time.Hour)
				cert.ExpiresAt = &expiry
			} else {
				container.State = "restarting"
			}
			source.value.Certificates = []CertificateResource{cert}
			source.value.Containers = []ContainerResource{container}
			for restart := range 2 {
				if restart == 1 {
					next, err := NewService(Config{DataDir: filepath.Dir(s.store.directory), Hosts: s.hosts, Telegram: tg, Resources: source, Now: clock.Now})
					if err != nil {
						t.Fatal(err)
					}
					defer next.Close()
					s = next
				}
				clock.Advance(25 * time.Hour)
				for range 5 {
					snapshot := s.Snapshot()
					if !reflect.DeepEqual(snapshot.Rules.ResourceAlerts, rules) {
						t.Fatal("compatibility read changed rules")
					}
					if !reflect.DeepEqual(snapshot.Resources, unknownResources()) {
						t.Fatal("snapshot exposed inventory")
					}
					resourceTick(t, s, clock)
				}
				if source.calls != 0 || tg.messageCount() != 0 {
					t.Fatalf("withdrawn resources collected/sent: %d/%d", source.calls, tg.messageCount())
				}
				for key, want := range states {
					if !reflect.DeepEqual(s.alertStateSnapshot()[key], want) || !reflect.DeepEqual(s.store.stateSnapshot().AlertStates[key], want) {
						t.Fatalf("dormant retry/recovery state changed: %s", key)
					}
				}
			}
		})
	}
}

func TestWithdrawnResourceWritesPreserveConfigAndState(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%v", existing), func(t *testing.T) {
			s, source, tg, clock := resourceService(t)
			var rules *ResourceRules
			states := s.alertStateSnapshot()
			if existing {
				rules, states = seedDormantResources(t, s, clock)
			}
			for _, incoming := range []*ResourceRules{nil, {}, {CertificatesEnabled: true, Containers: []ContainerRule{{ID: strings.Repeat("e", 64), Enabled: true}}}, {Containers: []ContainerRule{{ID: "invalid", Enabled: true}}}} {
				snapshot := s.Snapshot()
				input := snapshot.Rules
				input.CPUThresholdPercent = 85
				input.ResourceAlerts = incoming
				got, err := s.Configure(context.Background(), UpdateInput{Enabled: true, Rules: input, ExpectedResourceVersion: snapshot.ResourceVersion})
				if err != nil {
					t.Fatal(err)
				}
				if got.Rules.CPUThresholdPercent != 85 || !reflect.DeepEqual(got.Rules.ResourceAlerts, rules) {
					t.Fatal("save lost ordinary settings or modified dormant rules")
				}
				if !reflect.DeepEqual(s.store.stateSnapshot().AlertStates, states) {
					t.Fatal("save changed dormant state")
				}
			}
			if source.calls != 0 {
				t.Fatal("API write discovered resources")
			}
			// Original host alerts and Telegram test/discovery remain functional with
			// dormant resource states sharing the same store and delivery budget.
			hostSource := s.hosts.(*notificationHostSource)
			telemetry := hostSource.host.LastSnapshot.Telemetry
			telemetry.CPU.UsagePercent = 95
			hostSource.setTelemetry(telemetry)
			for range 3 {
				resourceTick(t, s, clock)
			}
			if tg.messageCount() != 1 || !strings.Contains(tg.messagesSnapshot()[0], "CPU") {
				t.Fatal("host CPU alert stopped")
			}
			if _, err := s.Discover(context.Background(), s.Snapshot().ResourceVersion); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Test(context.Background()); err != nil {
				t.Fatal(err)
			}
			if tg.messageCount() != 2 {
				t.Fatal("Telegram test stopped")
			}
			if source.calls != 0 {
				t.Fatal("Telegram operation discovered resources")
			}
		})
	}
}
