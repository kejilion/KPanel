package panel

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/notification"
)

func enablePanelLoginNotices(t *testing.T, server *Server) {
	t.Helper()
	rules := notification.DefaultRules()
	rules.CPUEnabled, rules.MemoryEnabled, rules.DiskEnabled = false, false, false
	rules.SSHLoginEnabled, rules.HostOfflineEnabled = false, false
	rules.PanelLoginEnabled = true
	if _, err := server.notifications.Configure(context.Background(), notification.UpdateInput{
		Rules: rules, ExpectedResourceVersion: server.notifications.Snapshot().ResourceVersion,
	}); err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
}

func waitForPanelLoginEvents(t *testing.T, server *Server, want int) []notification.Event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		page, err := server.notifications.History(notification.HistoryQuery{Rule: "panel-login"})
		if err != nil {
			t.Fatalf("History() error = %v", err)
		}
		if len(page.Items) >= want || time.Now().After(deadline) {
			return page.Items
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSuccessfulPasswordLoginIsRecordedForNotification(t *testing.T) {
	server, tokenPath := newTestServer(t)
	bootstrapCookies(t, server, tokenPath)
	enablePanelLoginNotices(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server.notifications.Start(ctx)

	if response := loginRequest(server, "wrong-password-123"); response.Code != http.StatusUnauthorized {
		t.Fatalf("failed login status = %d", response.Code)
	}
	if response := loginRequest(server, "a-strong-password-1"); response.Code != http.StatusOK {
		t.Fatalf("login status = %d %s", response.Code, response.Body.String())
	}
	events := waitForPanelLoginEvents(t, server, 1)
	if len(events) != 1 {
		t.Fatalf("panel login events = %#v, want exactly the successful login", events)
	}
	message := events[0].Message
	for _, fragment := range []string{"面板登录：", "用户：admin\n", "来源：192.0.2.1\n", "方式：密码\n"} {
		if !strings.Contains(message, fragment) {
			t.Fatalf("login notice %q lacks %q", message, fragment)
		}
	}
	for _, secret := range []string{"a-strong-password-1", "wrong-password-123", "kejilion_session"} {
		if strings.Contains(message, secret) {
			t.Fatalf("login notice leaked %q: %q", secret, message)
		}
	}
	if events[0].Delivery != "local_only" {
		t.Fatalf("push is off but delivery = %q", events[0].Delivery)
	}
}

func TestPanelLoginNoticeMethodReflectsVerifiedFactors(t *testing.T) {
	server, _ := newTestServer(t)
	cases := []struct {
		user    auth.PublicUser
		passkey bool
		want    string
	}{
		{auth.PublicUser{Username: "admin"}, false, "方式：密码\n"},
		{auth.PublicUser{Username: "admin", TOTPEnabled: true}, false, "方式：密码 + 两步验证\n"},
		{auth.PublicUser{Username: "admin"}, true, "方式：通行密钥\n"},
		{auth.PublicUser{Username: "admin", TOTPEnabled: true}, true, "方式：通行密钥 + 两步验证\n"},
	}
	enablePanelLoginNotices(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server.notifications.Start(ctx)
	request, _ := http.NewRequest(http.MethodPost, "http://panel.test/api/v1/auth/login", nil)
	request.RemoteAddr = "198.51.100.4:4000"
	for index, item := range cases {
		server.notifyPanelLogin(request, item.user, item.passkey)
		events := waitForPanelLoginEvents(t, server, index+1)
		if len(events) != index+1 || !strings.Contains(events[0].Message, item.want) || !strings.Contains(events[0].Message, "来源：198.51.100.4\n") {
			t.Fatalf("case %d events = %#v", index, events)
		}
	}
	var disabled Server
	disabled.notifyPanelLogin(request, auth.PublicUser{Username: "admin"}, false)
}
