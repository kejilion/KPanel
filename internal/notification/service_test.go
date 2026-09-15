package notification

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

type notificationTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *notificationTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *notificationTestClock) Advance(value time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(value)
	c.mu.Unlock()
}

type notificationHostSource struct {
	mu   sync.RWMutex
	host cluster.Host
}

func (s *notificationHostSource) Hosts(context.Context) cluster.HostList {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cluster.HostList{Items: []cluster.Host{s.host}, Total: 1, MaxHosts: cluster.MaxHosts}
}

func (s *notificationHostSource) setTelemetry(telemetry contract.HostTelemetry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.host.LastSnapshot == nil {
		s.host.LastSnapshot = &cluster.HostSnapshot{}
	}
	s.host.LastSnapshot.Telemetry = telemetry
}

func (s *notificationHostSource) setState(state cluster.HostState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.host.State = state
}

type notificationTestTelegram struct {
	mu          sync.Mutex
	validateErr error
	sendErr     error
	messages    []string
}

type notificationTestRobot struct {
	mu          sync.Mutex
	sendErr     error
	providers   []Provider
	credentials []string
	messages    []string
}

func (r *notificationTestRobot) SendMessage(_ context.Context, provider Provider, credential, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sendErr != nil {
		return r.sendErr
	}
	r.providers = append(r.providers, provider)
	r.credentials = append(r.credentials, credential)
	r.messages = append(r.messages, message)
	return nil
}

func (r *notificationTestRobot) calls() (providers []Provider, credentials, messages []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Provider(nil), r.providers...), append([]string(nil), r.credentials...), append([]string(nil), r.messages...)
}

func (t *notificationTestTelegram) ValidateAndDiscover(context.Context, string) (BotInfo, int64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.validateErr != nil {
		return BotInfo{}, 0, t.validateErr
	}
	return BotInfo{ID: 42, Username: "kpanel_bot"}, 91, nil
}

func (t *notificationTestTelegram) SendMessage(_ context.Context, _ string, _ int64, message string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sendErr != nil {
		return t.sendErr
	}
	t.messages = append(t.messages, message)
	return nil
}

func (t *notificationTestTelegram) messageCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.messages)
}

func (t *notificationTestTelegram) messagesSnapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.messages...)
}

func newNotificationTestHost(now time.Time) *notificationHostSource {
	return &notificationHostSource{host: cluster.Host{
		ID: "host-1", Name: "测试主机", State: cluster.HostOnline,
		LastSnapshot: &cluster.HostSnapshot{Telemetry: contract.HostTelemetry{
			CPU:         contract.CPUSummary{Cores: 4, UsagePercent: 20},
			Memory:      contract.MemorySummary{TotalBytes: 8 << 30, UsedBytes: 2 << 30, AvailableBytes: 6 << 30, UsagePercent: 25},
			Disk:        contract.DiskCapacitySummary{TotalBytes: 80 << 30, UsedBytes: 20 << 30, UsagePercent: 25},
			Network:     contract.NetworkSummary{ReceivedBytes: 1000, SentBytes: 1000},
			CollectedAt: now,
		}, ReceivedAt: now},
	}}
}

func TestServiceUsesConfiguredTimezoneSource(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	location := time.FixedZone("CST", 8*60*60)
	service, err := NewService(Config{
		DataDir: t.TempDir(), Hosts: source, Now: clock.Now,
		Timezone: func(context.Context) *time.Location { return location },
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	if got := service.Snapshot().Timezone; got != "UTC+08:00" {
		t.Fatalf("notification timezone = %q, want UTC+08:00", got)
	}
}

func TestServiceConfiguresTestsAndDeliversThroughRobotChannel(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	source.setTelemetry(contract.HostTelemetry{
		CPU:         contract.CPUSummary{Cores: 4, UsagePercent: 99},
		Memory:      contract.MemorySummary{TotalBytes: 8 << 30, UsedBytes: 2 << 30, AvailableBytes: 6 << 30, UsagePercent: 25},
		Disk:        contract.DiskCapacitySummary{TotalBytes: 80 << 30, UsedBytes: 20 << 30, UsagePercent: 25},
		Network:     contract.NetworkSummary{ReceivedBytes: 1000, SentBytes: 1000},
		CollectedAt: clock.Now(),
	})
	robots := &notificationTestRobot{}
	dataDir := t.TempDir()
	service, err := NewService(Config{
		DataDir: dataDir, Hosts: source, Telegram: &notificationTestTelegram{}, Robots: robots, Now: clock.Now,
		EvaluationInterval: time.Minute, SustainSamples: 1, RepeatInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	const credential = "https://open.feishu.cn/open-apis/bot/v2/hook/12345678-abcd"
	rules := DefaultRules()
	rules.MemoryEnabled = false
	rules.DiskEnabled = false
	rules.SSHLoginEnabled = false
	rules.HostOfflineEnabled = false
	snapshot, err := service.Configure(context.Background(), UpdateInput{
		Enabled: true, Locale: "en-US", Rules: rules, Provider: ProviderFeishu, ChannelCredential: credential,
		ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	if snapshot.Provider != ProviderFeishu || snapshot.Channel.Provider != ProviderFeishu || !snapshot.Channel.Ready || !snapshot.Channel.Configured {
		t.Fatalf("configured snapshot = %#v", snapshot)
	}
	if snapshot.Telegram.Configured || snapshot.Telegram.Ready {
		t.Fatalf("legacy Telegram snapshot exposed robot state = %#v", snapshot.Telegram)
	}
	if _, err := service.Discover(context.Background(), snapshot.ResourceVersion); !errors.Is(err, ErrDiscoverUnsupported) {
		t.Fatalf("Discover() error = %v, want ErrDiscoverUnsupported", err)
	}
	if _, err := service.Test(context.Background()); err != nil {
		t.Fatalf("Test() error = %v", err)
	}
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}

	providers, credentials, messages := robots.calls()
	if len(messages) != 3 {
		t.Fatalf("robot message count = %d, want validation + test + alert", len(messages))
	}
	for index := range providers {
		if providers[index] != ProviderFeishu || credentials[index] != credential {
			t.Fatalf("robot call %d = %q / %q", index, providers[index], credentials[index])
		}
	}
	if !strings.Contains(messages[0], "Feishu channel test succeeded") || !strings.Contains(messages[2], "CPU") {
		t.Fatalf("robot messages = %#v", messages)
	}

	stateContent, err := os.ReadFile(filepath.Join(dataDir, "notifications", stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stateContent), credential) || strings.Contains(string(stateContent), "12345678-abcd") {
		t.Fatalf("notification state exposed channel credential: %s", stateContent)
	}

	_, err = service.Configure(context.Background(), UpdateInput{
		Enabled: true, Locale: "en-US", Rules: rules, Provider: ProviderWeCom,
		ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	})
	if !errors.Is(err, ErrCredentialRequired) {
		t.Fatalf("Configure() channel switch error = %v, want ErrCredentialRequired", err)
	}
}

func TestServiceDoesNotStoreRobotCredentialWhenValidationFails(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)}
	dataDir := t.TempDir()
	robots := &notificationTestRobot{sendErr: &Error{Code: "api_error", Cause: ErrChannelUnavailable}}
	service, err := NewService(Config{
		DataDir: dataDir, Hosts: newNotificationTestHost(clock.Now()), Robots: robots, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	_, err = service.Configure(context.Background(), UpdateInput{
		Enabled: false, Rules: DefaultRules(), Provider: ProviderDingTalk,
		ChannelCredential:       "https://oapi.dingtalk.com/robot/send?access_token=abc12345",
		ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	})
	if !errors.Is(err, ErrChannelUnavailable) {
		t.Fatalf("Configure() error = %v, want ErrChannelUnavailable", err)
	}
	if _, statErr := os.Stat(filepath.Join(dataDir, "notifications", telegramTokenName)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("credential file exists after failed validation: %v", statErr)
	}
}

func TestServiceCanRepairInvalidStoredCredential(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)}
	dataDir := t.TempDir()
	robots := &notificationTestRobot{}
	service, err := NewService(Config{
		DataDir: dataDir, Hosts: newNotificationTestHost(clock.Now()), Robots: robots, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	defer service.Close()

	credentialPath := filepath.Join(dataDir, "notifications", telegramTokenName)
	if err := os.WriteFile(credentialPath, []byte("damaged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	const replacement = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc12345"
	snapshot, err := service.Configure(context.Background(), UpdateInput{
		Enabled: false, Rules: DefaultRules(), Provider: ProviderWeCom, ChannelCredential: replacement,
		ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	})
	if err != nil {
		t.Fatalf("Configure() repair error = %v", err)
	}
	if snapshot.Provider != ProviderWeCom || !snapshot.Channel.Ready {
		t.Fatalf("repaired snapshot = %#v", snapshot)
	}
	content, err := os.ReadFile(credentialPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(content)) != replacement {
		t.Fatalf("stored replacement = %q", content)
	}
}

func configureNotificationTestService(t *testing.T, dataDir string, source *notificationHostSource, telegram *notificationTestTelegram, clock *notificationTestClock) *Service {
	t.Helper()
	service, err := NewService(Config{
		DataDir: dataDir, Hosts: source, Telegram: telegram, Now: clock.Now,
		EvaluationInterval: time.Minute, SustainSamples: 3, RepeatInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	rules := DefaultRules()
	rules.MemoryEnabled = false
	rules.DiskEnabled = false
	rules.TrafficEnabled = false
	rules.HostOfflineEnabled = false
	rules.SSHLoginEnabled = false
	snapshot, err := service.Configure(context.Background(), UpdateInput{
		Enabled: true, Rules: rules, TelegramBotToken: testBotToken,
		ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	})
	if err != nil {
		service.Close()
		t.Fatalf("Configure() error = %v", err)
	}
	if !snapshot.Enabled || !snapshot.Telegram.Ready {
		service.Close()
		t.Fatalf("configured snapshot = %#v", snapshot)
	}
	return service
}

func TestNormalizeRulesMigratesLegacyCumulativeTraffic(t *testing.T) {
	legacy := DefaultRules()
	legacy.TrafficTotalEnabled = true
	legacy.TrafficTotalThresholdGiB = 7

	normalized := normalizeRules(legacy)
	if !normalized.TrafficTotalReceivedEnabled || !normalized.TrafficTotalSentEnabled {
		t.Fatalf("legacy cumulative enablement = received:%v sent:%v", normalized.TrafficTotalReceivedEnabled, normalized.TrafficTotalSentEnabled)
	}
	if normalized.TrafficTotalReceivedThresholdGiB != 7 || normalized.TrafficTotalSentThresholdGiB != 7 {
		t.Fatalf("legacy cumulative thresholds = received:%d sent:%d", normalized.TrafficTotalReceivedThresholdGiB, normalized.TrafficTotalSentThresholdGiB)
	}
	if normalized.TrafficTotalEnabled || normalized.TrafficTotalThresholdGiB != 0 {
		t.Fatalf("legacy cumulative fields were not cleared: %#v", normalized)
	}
}

func TestServiceThresholdSustainsRecoversAndPersistsSamples(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	dataDir := t.TempDir()
	service := configureNotificationTestService(t, dataDir, source, telegram, clock)

	telemetry := source.host.LastSnapshot.Telemetry
	telemetry.CPU.UsagePercent = 95
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatalf("first evaluation error = %v", err)
	}
	clock.Advance(time.Minute)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatalf("second evaluation error = %v", err)
	}
	if telegram.messageCount() != 0 {
		t.Fatalf("threshold alerted before sustain count: %d", telegram.messageCount())
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}

	service = configureNotificationTestService(t, dataDir, source, telegram, clock)
	// Configure above intentionally resets the settings but retains the
	// persisted alert sample; use the existing resource version to avoid a
	// second token validation in the persistence test.
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatalf("persisted third evaluation error = %v", err)
	}
	if telegram.messageCount() != 1 {
		t.Fatalf("message count after persisted sustain = %d, want 1", telegram.messageCount())
	}

	clock.Advance(time.Minute)
	telemetry.CPU.UsagePercent = 50
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatalf("recovery evaluation error = %v", err)
	}
	if telegram.messageCount() != 2 {
		t.Fatalf("message count after recovery = %d, want 2", telegram.messageCount())
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServiceSkipsUnavailableResourceMetrics(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 11, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	service := configureNotificationTestService(t, t.TempDir(), source, telegram, clock)
	defer service.Close()

	telemetry := source.host.LastSnapshot.Telemetry
	telemetry.CPU.UsagePercent = 95
	source.setTelemetry(telemetry)
	for index := 0; index < 3; index++ {
		if err := service.evaluate(context.Background()); err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Minute)
	}
	if telegram.messageCount() != 1 {
		t.Fatalf("initial high CPU messages = %d, want 1", telegram.messageCount())
	}
	telemetry.CPU.UsagePercent = math.NaN()
	telemetry.Memory.TotalBytes = 0
	telemetry.Disk.TotalBytes = 0
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 1 {
		t.Fatalf("unavailable metrics caused a message: %d", telegram.messageCount())
	}
}

func TestServiceDeduplicatesSSHEventsButAllowsNewLoginImmediately(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	service := configureNotificationTestService(t, t.TempDir(), source, telegram, clock)
	defer service.Close()

	rules := DefaultRules()
	rules.CPUEnabled = false
	rules.MemoryEnabled = false
	rules.DiskEnabled = false
	rules.TrafficEnabled = false
	rules.HostOfflineEnabled = false
	rules.SSHLoginEnabled = true
	configured := service.Snapshot()
	updated, err := service.Configure(context.Background(), UpdateInput{Enabled: true, Rules: rules, ExpectedResourceVersion: configured.ResourceVersion})
	if err != nil {
		t.Fatalf("Configure() SSH rules error = %v", err)
	}
	if !updated.Telegram.Ready {
		t.Fatalf("updated Telegram snapshot = %#v", updated.Telegram)
	}

	telemetry := source.host.LastSnapshot.Telemetry
	firstAt := clock.Now().Add(-time.Minute)
	telemetry.SSHLogin = &contract.SSHLoginEvent{ID: "event-1", OccurredAt: firstAt, Username: "root", RemoteAddress: "203.0.113.10", Method: "publickey"}
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 0 {
		t.Fatalf("baseline SSH event messages = %d, want 0", telegram.messageCount())
	}
	clock.Advance(time.Second)
	telemetry.SSHLogin = &contract.SSHLoginEvent{ID: "event-2", OccurredAt: clock.Now(), Username: "admin", RemoteAddress: "2001:db8::10", Method: "password"}
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 1 {
		t.Fatalf("new SSH message count = %d, want 1", telegram.messageCount())
	}
}

func TestServiceSSHBaselineSurvivesRestart(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 12, 30, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	dataDir := t.TempDir()
	service := configureNotificationTestService(t, dataDir, source, telegram, clock)

	rules := DefaultRules()
	rules.CPUEnabled = false
	rules.MemoryEnabled = false
	rules.DiskEnabled = false
	rules.TrafficEnabled = false
	rules.HostOfflineEnabled = false
	rules.SSHLoginEnabled = true
	if _, err := service.Configure(context.Background(), UpdateInput{
		Enabled: true, Rules: rules, ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	}); err != nil {
		t.Fatalf("Configure() SSH rules error = %v", err)
	}

	telemetry := source.host.LastSnapshot.Telemetry
	telemetry.SSHLogin = &contract.SSHLoginEvent{
		ID: "historical-event", OccurredAt: clock.Now().Add(-time.Hour),
		Username: "root", RemoteAddress: "203.0.113.10", Method: "publickey",
	}
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 0 {
		t.Fatalf("historical SSH event messages = %d, want 0", telegram.messageCount())
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewService(Config{
		DataDir: dataDir, Hosts: source, Telegram: telegram, Now: clock.Now,
		EvaluationInterval: time.Minute, SustainSamples: 3, RepeatInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("restart NewService() error = %v", err)
	}
	defer restarted.Close()
	if err := restarted.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 0 {
		t.Fatalf("replayed SSH event messages = %d, want 0", telegram.messageCount())
	}

	telemetry.SSHLogin.ID = "new-event"
	telemetry.SSHLogin.OccurredAt = clock.Now()
	source.setTelemetry(telemetry)
	if err := restarted.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 1 {
		t.Fatalf("new SSH event messages = %d, want 1", telegram.messageCount())
	}
}

func TestServiceAvailabilityAlertsAndRecovers(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 13, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	source.host.Kind = cluster.HostKindPanel
	telegram := &notificationTestTelegram{}
	service := configureNotificationTestService(t, t.TempDir(), source, telegram, clock)
	defer service.Close()

	rules := DefaultRules()
	rules.CPUEnabled = false
	rules.MemoryEnabled = false
	rules.DiskEnabled = false
	rules.TrafficEnabled = false
	rules.SSHLoginEnabled = false
	rules.HostOfflineEnabled = true
	configured := service.Snapshot()
	if _, err := service.Configure(context.Background(), UpdateInput{Enabled: true, Rules: rules, ExpectedResourceVersion: configured.ResourceVersion}); err != nil {
		t.Fatalf("Configure() availability rules error = %v", err)
	}
	source.setState(cluster.HostOffline)
	for sample := 1; sample <= 3; sample++ {
		if err := service.evaluate(context.Background()); err != nil {
			t.Fatal(err)
		}
		wantMessages := 0
		if sample == 3 {
			wantMessages = 1
		}
		if telegram.messageCount() != wantMessages {
			t.Fatalf("offline sample %d messages = %d, want %d", sample, telegram.messageCount(), wantMessages)
		}
		clock.Advance(time.Second)
	}
	source.setState(cluster.HostUnknown)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 1 {
		t.Fatalf("unknown transition recovered too early: %d", telegram.messageCount())
	}
	clock.Advance(time.Second)
	source.setState(cluster.HostOnline)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 2 {
		t.Fatalf("offline recovery messages = %d, want 2", telegram.messageCount())
	}
}

func TestServiceAvailabilityIgnoresTransientUnavailableStates(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 9, 15, 6, 47, 30, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	source.host.Kind = cluster.HostKindLightNode
	telegram := &notificationTestTelegram{}
	service := configureNotificationTestService(t, t.TempDir(), source, telegram, clock)
	defer service.Close()

	rules := DefaultRules()
	rules.CPUEnabled = false
	rules.MemoryEnabled = false
	rules.DiskEnabled = false
	rules.TrafficEnabled = false
	rules.SSHLoginEnabled = false
	rules.HostOfflineEnabled = true
	configured := service.Snapshot()
	if _, err := service.Configure(context.Background(), UpdateInput{Enabled: true, Rules: rules, ExpectedResourceVersion: configured.ResourceVersion}); err != nil {
		t.Fatalf("Configure() availability rules error = %v", err)
	}

	for _, state := range []cluster.HostState{cluster.HostOffline, cluster.HostUnknown, cluster.HostOffline, cluster.HostOffline, cluster.HostOnline} {
		source.setState(state)
		if err := service.evaluate(context.Background()); err != nil {
			t.Fatal(err)
		}
		clock.Advance(30 * time.Second)
	}
	if telegram.messageCount() != 0 {
		t.Fatalf("transient availability messages = %d, want 0", telegram.messageCount())
	}
}

func TestServiceTrafficRateRequiresFreshSnapshot(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	service, err := NewService(Config{
		DataDir: t.TempDir(), Hosts: source, Telegram: telegram, Now: clock.Now,
		EvaluationInterval: time.Minute, SustainSamples: 3, RepeatInterval: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	host := source.host
	if _, ok := service.trafficRate(host, clock.Now()); ok {
		t.Fatal("zero-rate first sample should not be available")
	}
	clock.Advance(time.Minute)
	if _, ok := service.trafficRate(host, clock.Now()); ok {
		t.Fatal("duplicate snapshot should not be treated as a fresh zero-rate sample")
	}
	fresh := *host.LastSnapshot
	fresh.ReceivedAt = clock.Now()
	fresh.Telemetry.Network.ReceivedBytes += 60 * 1024 * 1024
	fresh.Telemetry.Network.SentBytes += 60 * 1024 * 1024
	host.LastSnapshot = &fresh
	rate, ok := service.trafficRate(host, clock.Now())
	if !ok || rate < 1.9 || rate > 2.1 {
		t.Fatalf("fresh traffic rate = %v, %v; want about 2 MiB/s", rate, ok)
	}
}

func TestServiceCumulativeTrafficAlertsEachDirectionOncePerCounterCycle(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 23, 0, 0, 0, location)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	service := configureNotificationTestService(t, t.TempDir(), source, telegram, clock)
	defer service.Close()

	rules := DefaultRules()
	rules.CPUEnabled = false
	rules.MemoryEnabled = false
	rules.DiskEnabled = false
	rules.TrafficEnabled = false
	rules.HostOfflineEnabled = false
	rules.SSHLoginEnabled = false
	rules.TrafficTotalReceivedEnabled = true
	rules.TrafficTotalReceivedThresholdGiB = 1
	rules.TrafficTotalSentEnabled = true
	rules.TrafficTotalSentThresholdGiB = 1
	configured, err := service.Configure(context.Background(), UpdateInput{
		Enabled: true, Locale: "en-US", Rules: rules,
		ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	})
	if err != nil {
		t.Fatalf("Configure() cumulative traffic rules error = %v", err)
	}
	if configured.Locale != "en-US" || configured.Timezone != "UTC+08:00" {
		t.Fatalf("configured locale/timezone = %q/%q", configured.Locale, configured.Timezone)
	}

	telemetry := source.host.LastSnapshot.Telemetry
	telemetry.Network = contract.NetworkSummary{ReceivedBytes: 700 << 20, SentBytes: 500 << 20}
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 0 {
		t.Fatalf("directional cumulative threshold messages = %d, want 0", telegram.messageCount())
	}

	telemetry.Network.ReceivedBytes = 1200 << 20
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 1 {
		t.Fatalf("received cumulative threshold messages = %d, want 1", telegram.messageCount())
	}
	if messages := telegram.messagesSnapshot(); !strings.Contains(messages[0], "Cumulative received") || !strings.Contains(messages[0], "1.2 GB") {
		t.Fatalf("unexpected received cumulative message: %q", messages[0])
	}
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 1 {
		t.Fatalf("received cumulative threshold repeated without reset: %d", telegram.messageCount())
	}

	telemetry.Network = contract.NetworkSummary{ReceivedBytes: 1300 << 20, SentBytes: 1200 << 20}
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 2 {
		t.Fatalf("sent cumulative threshold messages = %d, want 2", telegram.messageCount())
	}
	if messages := telegram.messagesSnapshot(); !strings.Contains(messages[1], "Cumulative sent") || !strings.Contains(messages[1], "1.2 GB") {
		t.Fatalf("unexpected sent cumulative message: %q", messages[1])
	}
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 2 {
		t.Fatalf("directional cumulative threshold repeated while active: %d", telegram.messageCount())
	}

	// Reset only the receive counter. The sent rule remains active and must not
	// emit a duplicate notification.
	telemetry.Network = contract.NetworkSummary{SentBytes: 1300 << 20}
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	telemetry.Network.ReceivedBytes = 2 << 30
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 3 {
		t.Fatalf("received cumulative threshold did not re-arm independently: %d", telegram.messageCount())
	}
	if messages := telegram.messagesSnapshot(); !strings.Contains(messages[2], "Cumulative received") || !strings.Contains(messages[2], "2.0 GB") || !strings.Contains(messages[2], "UTC+08:00") {
		t.Fatalf("unexpected re-armed received cumulative message: %q", messages[2])
	}
}

func TestServiceCumulativeTrafficUsesExactCounterRollback(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 15, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	service := configureNotificationTestService(t, t.TempDir(), source, telegram, clock)
	defer service.Close()

	rules := DefaultRules()
	rules.CPUEnabled = false
	rules.MemoryEnabled = false
	rules.DiskEnabled = false
	rules.TrafficEnabled = false
	rules.HostOfflineEnabled = false
	rules.SSHLoginEnabled = false
	rules.TrafficTotalReceivedEnabled = true
	rules.TrafficTotalReceivedThresholdGiB = 1
	if _, err := service.Configure(context.Background(), UpdateInput{
		Enabled: true, Rules: rules,
		ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	}); err != nil {
		t.Fatalf("Configure() exact cumulative traffic rules error = %v", err)
	}

	large := uint64(1) << 54
	telemetry := source.host.LastSnapshot.Telemetry
	telemetry.Network = contract.NetworkSummary{ReceivedBytes: large}
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 1 {
		t.Fatalf("large cumulative threshold messages = %d, want 1", telegram.messageCount())
	}

	telemetry.Network.ReceivedBytes = large - 1
	source.setTelemetry(telemetry)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if telegram.messageCount() != 2 {
		t.Fatalf("one-byte counter rollback did not start a new cycle: %d", telegram.messageCount())
	}
}

func TestServiceUsesConfiguredLocaleForResourceAlerts(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 23, 30, 0, 0, location)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	service := configureNotificationTestService(t, t.TempDir(), source, telegram, clock)
	defer service.Close()

	rules := DefaultRules()
	rules.MemoryEnabled = false
	rules.DiskEnabled = false
	rules.TrafficEnabled = false
	rules.TrafficTotalReceivedEnabled = false
	rules.HostOfflineEnabled = false
	rules.SSHLoginEnabled = false
	if _, err := service.Configure(context.Background(), UpdateInput{
		Enabled: true, Locale: "en-US", Rules: rules,
		ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	}); err != nil {
		t.Fatalf("Configure() locale rules error = %v", err)
	}

	telemetry := source.host.LastSnapshot.Telemetry
	telemetry.CPU.UsagePercent = 95
	source.setTelemetry(telemetry)
	for index := 0; index < 3; index++ {
		if err := service.evaluate(context.Background()); err != nil {
			t.Fatal(err)
		}
		clock.Advance(time.Minute)
	}
	messages := telegram.messagesSnapshot()
	if len(messages) != 1 || !strings.Contains(messages[0], "CPU usage") ||
		!strings.Contains(messages[0], "[KPanel Cluster Alert]") ||
		!strings.Contains(messages[0], "UTC+08:00") {
		t.Fatalf("unexpected localized resource message: %#v", messages)
	}
}

func TestServiceCannotEnableAfterChannelError(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 16, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	service := configureNotificationTestService(t, t.TempDir(), source, telegram, clock)
	defer service.Close()

	telegram.mu.Lock()
	telegram.sendErr = errors.New("telegram send failed")
	telegram.mu.Unlock()
	if _, err := service.Test(context.Background()); err == nil {
		t.Fatal("Test() unexpectedly succeeded with a failed channel")
	}
	state := service.Snapshot()
	if _, err := service.Configure(context.Background(), UpdateInput{
		Enabled: true, Rules: state.Rules, ExpectedResourceVersion: state.ResourceVersion,
	}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Configure() after channel error = %v, want ErrNotReady", err)
	}
}

func TestServiceDoesNotPersistTelegramTokenInSnapshot(t *testing.T) {
	clock := &notificationTestClock{now: time.Date(2026, 8, 31, 14, 0, 0, 0, time.UTC)}
	source := newNotificationTestHost(clock.Now())
	telegram := &notificationTestTelegram{}
	dataDir := t.TempDir()
	service := configureNotificationTestService(t, dataDir, source, telegram, clock)
	defer service.Close()

	value := service.Snapshot()
	if value.Telegram.BotUsername != "kpanel_bot" || value.Telegram.Configured != true || value.Telegram.Ready != true {
		t.Fatalf("unexpected snapshot = %#v", value)
	}
	content, err := os.ReadFile(filepath.Join(dataDir, "notifications", stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), testBotToken) {
		t.Fatalf("notification state leaked Telegram token: %s", content)
	}
	tokenContent, err := os.ReadFile(filepath.Join(dataDir, "notifications", telegramTokenName))
	if err != nil {
		t.Fatal(err)
	}
	if string(tokenContent) != testBotToken+"\n" {
		t.Fatalf("token store content = %q", tokenContent)
	}
}
