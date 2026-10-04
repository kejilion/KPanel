package notification

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

const panelLoginTestCredential = "https://open.feishu.cn/open-apis/bot/v2/hook/12345678-abcd"

func newPanelLoginTestService(t *testing.T, robots *notificationTestRobot, host cluster.Host) (*Service, *notificationTestClock, string) {
	t.Helper()
	clock := &notificationTestClock{now: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)}
	dataDir := t.TempDir()
	service, err := NewService(Config{
		DataDir: dataDir, Hosts: &notificationHostSource{host: host}, Telegram: &notificationTestTelegram{}, Robots: robots, Now: clock.Now,
		Timezone:           func(context.Context) *time.Location { return time.FixedZone("CST", 8*60*60) },
		EvaluationInterval: time.Minute, SustainSamples: 1, RepeatInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service, clock, dataDir
}

func panelLoginTestHost() cluster.Host {
	return cluster.Host{ID: cluster.LocalHostID, Name: "主控面板", IsLocal: true, State: cluster.HostOnline}
}

func configurePanelLogin(t *testing.T, service *Service, enabled, push bool) {
	t.Helper()
	rules := DefaultRules()
	rules.CPUEnabled, rules.MemoryEnabled, rules.DiskEnabled = false, false, false
	rules.SSHLoginEnabled, rules.HostOfflineEnabled = false, false
	rules.PanelLoginEnabled = enabled
	if _, err := service.Configure(context.Background(), UpdateInput{
		Enabled: push, Locale: "zh-CN", Rules: rules, Provider: ProviderFeishu, ChannelCredential: panelLoginTestCredential,
		ExpectedResourceVersion: service.Snapshot().ResourceVersion,
	}); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
}

func recordQueuedPanelLogins(t *testing.T, service *Service) {
	t.Helper()
	select {
	case login := <-service.panelLogins:
		if err := service.recordPanelLogins(context.Background(), service.drainPanelLogins(login)); err != nil {
			t.Fatalf("recordPanelLogins() error = %v", err)
		}
	default:
		t.Fatal("no panel login was queued")
	}
}

func TestPanelLoginIsDeliveredImmediatelyWithoutCredentialsOrSessionData(t *testing.T) {
	robots := &notificationTestRobot{}
	service, _, _ := newPanelLoginTestService(t, robots, panelLoginTestHost())
	configurePanelLogin(t, service, true, true)

	if !service.RecordPanelLogin(PanelLogin{Username: "admin", RemoteAddress: "203.0.113.9", Method: PanelLoginPasswordTOTP}) {
		t.Fatal("RecordPanelLogin() rejected a valid sign-in")
	}
	recordQueuedPanelLogins(t, service)

	_, _, messages := robots.calls()
	// The first message is the channel validation sent while configuring.
	if len(messages) != 2 {
		t.Fatalf("robot messages = %#v, want validation + login", messages)
	}
	want := "🔐 [KPanel 集群通知]\n\n主机：主控面板\n面板登录：2026-10-04 20:00:00 (UTC+08:00)\n用户：admin\n来源：203.0.113.9\n方式：密码 + 两步验证\n\n发送时间：2026-10-04 20:00:00 (UTC+08:00)"
	if messages[1] != want {
		t.Fatalf("login message = %q, want %q", messages[1], want)
	}
	page, err := service.History(HistoryQuery{Rule: panelLoginRuleKey})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("History() = %#v, %v", page, err)
	}
	event := page.Items[0]
	if event.HostID != cluster.LocalHostID || !event.IsLocal || event.Kind != "info" || event.Delivery != "sent" || event.HostName != "主控面板" {
		t.Fatalf("login event = %#v", event)
	}
}

func TestPanelLoginRuleIsOffByDefaultAndOmittedFromState(t *testing.T) {
	robots := &notificationTestRobot{}
	service, _, dataDir := newPanelLoginTestService(t, robots, panelLoginTestHost())
	if service.Snapshot().Rules.PanelLoginEnabled {
		t.Fatal("panel login notices must be off by default")
	}
	configurePanelLogin(t, service, false, true)
	service.RecordPanelLogin(PanelLogin{Username: "admin", RemoteAddress: "203.0.113.9", Method: PanelLoginPassword})
	recordQueuedPanelLogins(t, service)
	if page, err := service.History(HistoryQuery{}); err != nil || len(page.Items) != 0 {
		t.Fatalf("disabled rule recorded %#v, %v", page.Items, err)
	}
	// Binaries predating the rule decode state strictly; an untouched rule
	// must leave no field for them to reject.
	for _, name := range []string{stateFileName, historyFileName} {
		content, err := os.ReadFile(filepath.Join(dataDir, "notifications", name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(content), "panelLoginEnabled") {
			t.Fatalf("%s carries the disabled rule: %s", name, content)
		}
	}
	configurePanelLogin(t, service, true, true)
	content, err := os.ReadFile(filepath.Join(dataDir, "notifications", stateFileName))
	if err != nil || !regexp.MustCompile(`"panelLoginEnabled":\s*true`).Match(content) {
		t.Fatalf("enabled rule was not persisted: %s, %v", content, err)
	}
}

func TestPanelLoginIsRecordedLocallyWhenPushIsOff(t *testing.T) {
	robots := &notificationTestRobot{}
	service, _, _ := newPanelLoginTestService(t, robots, panelLoginTestHost())
	configurePanelLogin(t, service, true, false)
	service.RecordPanelLogin(PanelLogin{Username: "admin", RemoteAddress: "2001:db8::7", Method: PanelLoginPasskey})
	recordQueuedPanelLogins(t, service)

	if _, _, messages := robots.calls(); len(messages) != 1 {
		t.Fatalf("push disabled but robot received %#v", messages)
	}
	page, err := service.History(HistoryQuery{Rule: panelLoginRuleKey})
	if err != nil || len(page.Items) != 1 || page.Items[0].Delivery != "local_only" ||
		!strings.Contains(page.Items[0].Message, "方式：通行密钥\n") || !strings.Contains(page.Items[0].Message, "来源：2001:db8::7\n") {
		t.Fatalf("History() = %#v, %v", page.Items, err)
	}
}

func TestPanelLoginStaysPendingWhenLocalHostIsNotListed(t *testing.T) {
	robots := &notificationTestRobot{}
	remote := cluster.Host{ID: "host-1", Name: "远程主机", State: cluster.HostOnline}
	service, _, _ := newPanelLoginTestService(t, robots, remote)
	configurePanelLogin(t, service, true, true)
	service.RecordPanelLogin(PanelLogin{Username: "admin", RemoteAddress: "203.0.113.9", Method: PanelLoginPassword})
	recordQueuedPanelLogins(t, service)

	if _, _, messages := robots.calls(); len(messages) != 1 {
		t.Fatalf("unlisted local host was delivered: %#v", messages)
	}
	page, err := service.History(HistoryQuery{Rule: panelLoginRuleKey})
	if err != nil || len(page.Items) != 1 || page.Items[0].Delivery != "pending" || page.Items[0].HostID != cluster.LocalHostID {
		t.Fatalf("History() = %#v, %v", page.Items, err)
	}
}

func TestPanelLoginPendingDeliveryStopsWhenRuleIsDisabled(t *testing.T) {
	robots := &notificationTestRobot{}
	service, clock, _ := newPanelLoginTestService(t, robots, panelLoginTestHost())
	configurePanelLogin(t, service, true, true)
	robots.mu.Lock()
	robots.sendErr = &Error{Code: "api_error", Cause: ErrChannelUnavailable}
	robots.mu.Unlock()
	service.RecordPanelLogin(PanelLogin{Username: "admin", RemoteAddress: "203.0.113.9", Method: PanelLoginPassword})
	recordQueuedPanelLogins(t, service)
	page, _ := service.History(HistoryQuery{Rule: panelLoginRuleKey})
	if len(page.Items) != 1 || page.Items[0].Delivery != "failed" {
		t.Fatalf("failed delivery = %#v", page.Items)
	}

	robots.mu.Lock()
	robots.sendErr = nil
	robots.mu.Unlock()
	configurePanelLogin(t, service, false, true)
	clock.Advance(alertRetryInterval)
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatalf("evaluate() error = %v", err)
	}
	page, _ = service.History(HistoryQuery{Rule: panelLoginRuleKey})
	if len(page.Items) != 1 || page.Items[0].Delivery != "cancelled" || page.Items[0].LastErrorCode != "rule_disabled" {
		t.Fatalf("disabled rule retry = %#v", page.Items)
	}
}

func TestRecordPanelLoginNeverBlocksTheSignIn(t *testing.T) {
	service, _, _ := newPanelLoginTestService(t, &notificationTestRobot{}, panelLoginTestHost())
	if service.RecordPanelLogin(PanelLogin{Username: "admin", Method: "sms"}) {
		t.Fatal("an unknown method was accepted")
	}
	for index := 0; index < panelLoginQueueSize; index++ {
		if !service.RecordPanelLogin(PanelLogin{Username: "admin", Method: PanelLoginPassword}) {
			t.Fatalf("queue rejected sign-in %d before it was full", index)
		}
	}
	done := make(chan bool, 1)
	go func() { done <- service.RecordPanelLogin(PanelLogin{Username: "admin", Method: PanelLoginPassword}) }()
	select {
	case accepted := <-done:
		if accepted {
			t.Fatal("a full queue accepted another sign-in")
		}
	case <-time.After(time.Second):
		t.Fatal("RecordPanelLogin() blocked on a full queue")
	}
	if logins := service.drainPanelLogins(<-service.panelLogins); len(logins) != panelLoginQueueSize {
		t.Fatalf("drained %d sign-ins, want %d", len(logins), panelLoginQueueSize)
	}
	var unset *Service
	if unset.RecordPanelLogin(PanelLogin{Username: "admin", Method: PanelLoginPassword}) {
		t.Fatal("a nil service accepted a sign-in")
	}
}

func TestPanelLoginRunLoopDeliversQueuedSignIn(t *testing.T) {
	robots := &notificationTestRobot{}
	service, _, _ := newPanelLoginTestService(t, robots, panelLoginTestHost())
	configurePanelLogin(t, service, true, true)
	service.Start(context.Background())
	service.RecordPanelLogin(PanelLogin{Username: "admin", RemoteAddress: "203.0.113.9", Method: PanelLoginPasskeyTOTP})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, messages := robots.calls(); len(messages) == 2 {
			if !strings.Contains(messages[1], "方式：通行密钥 + 两步验证\n") {
				t.Fatalf("login message = %q", messages[1])
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the run loop did not deliver the queued sign-in")
}

func TestPanelLoginMessagesAreLocalizedAndSanitized(t *testing.T) {
	now := time.Date(2026, 10, 4, 20, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	login := PanelLogin{Username: "ad\nmin", RemoteAddress: "", Method: PanelLoginPasskey, occurredAt: now}
	host := cluster.Host{ID: cluster.LocalHostID, Name: "Main\tpanel"}
	cases := map[string][]string{
		"en-US": {"🔐 [KPanel Cluster Notice]", "Host: Mainpanel", "Panel login: 2026-10-04 20:00:00 (UTC+08:00)", "User: admin", "Source: -", "Method: Passkey", "Sent: "},
		"zh-TW": {"🔐 [KPanel 叢集通知]", "主機：Mainpanel", "面板登入：", "使用者：admin", "來源：-", "方式：通行金鑰", "傳送時間："},
		"zh-CN": {"🔐 [KPanel 集群通知]", "主机：Mainpanel", "面板登录：", "用户：admin", "来源：-", "方式：通行密钥", "发送时间："},
	}
	if got := panelLoginMethodLabel(PanelLoginPasswordTOTP, "zh-TW"); got != "密碼 + 兩步驗證" {
		t.Fatalf("zh-TW two-factor label = %q, want the catalog term 兩步驗證", got)
	}
	for locale, want := range cases {
		message := panelLoginMessage(host, login, now, locale)
		for _, fragment := range want {
			if !strings.Contains(message, fragment) {
				t.Fatalf("%s message %q lacks %q", locale, message, fragment)
			}
		}
	}
	history := func(rule string) historyState {
		return historyState{SchemaVersion: 1, Sequence: 1, Rules: DefaultRules(), Events: []storedEvent{{Event: Event{
			ID: "1", CreatedAt: now, HostID: cluster.LocalHostID, HostName: "x", Rule: rule, Kind: "info", Delivery: "local_only",
		}}}}
	}
	if err := validateHistory(history(panelLoginRuleKey)); err != nil {
		t.Fatalf("history rejected a panel login event: %v", err)
	}
	if err := validateHistory(history("panel-logins")); err == nil {
		t.Fatal("history accepted an unknown rule")
	}
}
