package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/notification"
)

func TestClusterNotificationsAPIUsesSessionCSRFAndHidesChannelSecrets(t *testing.T) {
	server, tokenPath := newTestServer(t)
	if response := performRequest(server, http.MethodGet, clusterNotificationsPath, nil, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated notification settings status = %d", response.Code)
	}

	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	settings := authenticatedRequest(
		server, http.MethodGet, clusterNotificationsPath, nil,
		sessionCookie, csrfCookie, nil,
	)
	if settings.Code != http.StatusOK {
		t.Fatalf("notification settings status = %d; body=%s", settings.Code, settings.Body.String())
	}
	if strings.Contains(settings.Body.String(), "chatId") || strings.Contains(settings.Body.String(), "telegramBotToken") {
		t.Fatalf("notification settings exposed channel secrets: %s", settings.Body.String())
	}
	var snapshot notification.Snapshot
	if err := json.Unmarshal(settings.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}

	rules := notification.DefaultRules()
	body, err := json.Marshal(notification.UpdateInput{
		Enabled: false, Rules: rules, ExpectedResourceVersion: snapshot.ResourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	missingCSRF := authenticatedRequest(
		server, http.MethodPut, clusterNotificationsPath, body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test",
		},
	)
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("notification update without CSRF = %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}

	updated := authenticatedRequest(
		server, http.MethodPut, clusterNotificationsPath, body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if updated.Code != http.StatusOK {
		t.Fatalf("notification update status = %d; body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Enabled || snapshot.Telegram.Configured {
		t.Fatalf("unexpected notification snapshot = %#v", snapshot)
	}
	if snapshot.Locale != notification.DefaultNotificationLocale || snapshot.Timezone == "" {
		t.Fatalf("notification locale/timezone = %q/%q", snapshot.Locale, snapshot.Timezone)
	}
}
