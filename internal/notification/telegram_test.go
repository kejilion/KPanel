package notification

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testBotToken = "123456:aaaaaaaaaaaaaaaaaaaa"

func TestTelegramClientValidatesAndDiscoversPrivateStartChat(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/getMe"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":42,"is_bot":true,"first_name":"KPanel","username":"kpanel_bot"}}`))
		case strings.HasSuffix(r.URL.Path, "/getUpdates"):
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":10,"message":{"text":"/start","chat":{"id":-8,"type":"group"}}},{"update_id":12,"message":{"text":"/help","chat":{"id":91,"type":"private"}}},{"update_id":13,"message":{"text":"/start@kpanel_bot extra","chat":{"id":91,"type":"private","username":"admin"}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := newTelegramClientForTest(server.URL, server.Client())
	bot, chatID, err := client.ValidateAndDiscover(context.Background(), testBotToken)
	if err != nil {
		t.Fatalf("ValidateAndDiscover() error = %v", err)
	}
	if bot.ID != 42 || bot.Username != "kpanel_bot" || chatID != 91 {
		t.Fatalf("ValidateAndDiscover() = %#v, %d", bot, chatID)
	}
	if len(methods) != 2 || !strings.HasSuffix(methods[0], "/getMe") || !strings.HasSuffix(methods[1], "/getUpdates") {
		t.Fatalf("Telegram API methods = %#v", methods)
	}
}

func TestTelegramClientSendsPlainTextWithoutLeakingResponseDetails(t *testing.T) {
	var received struct {
		ChatID int64  `json:"chat_id"`
		Text   string `json:"text"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Fatalf("request path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode sendMessage = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true}}`))
	}))
	defer server.Close()

	client := newTelegramClientForTest(server.URL, server.Client())
	if err := client.SendMessage(context.Background(), testBotToken, 91, "hello KPanel"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if received.ChatID != 91 || received.Text != "hello KPanel" {
		t.Fatalf("sendMessage payload = %#v", received)
	}
	if err := client.SendMessage(context.Background(), "not-a-token", 91, "secret"); err == nil || !strings.Contains(err.Error(), "invalid_token") || strings.Contains(err.Error(), "not-a-token") {
		t.Fatalf("invalid token error = %v", err)
	}
}

func TestTelegramClientMapsWebhookAndMissingChatErrors(t *testing.T) {
	for _, test := range []struct {
		name     string
		updates  string
		status   int
		want     error
		wantCode string
	}{
		{
			name: "missing chat", updates: `{"ok":true,"result":[]}`, status: http.StatusOK,
			want: ErrChatNotFound, wantCode: "chat_not_found",
		},
		{
			name: "webhook", updates: `{"ok":false,"error_code":409,"description":"Conflict: webhook active"}`, status: http.StatusConflict,
			want: ErrTelegramWebhookActive, wantCode: "webhook_active",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/getMe") {
					_, _ = w.Write([]byte(`{"ok":true,"result":{"id":42,"is_bot":true}}`))
					return
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.updates))
			}))
			defer server.Close()

			client := newTelegramClientForTest(server.URL, server.Client())
			_, _, err := client.ValidateAndDiscover(context.Background(), testBotToken)
			if err == nil || !strings.Contains(err.Error(), test.wantCode) || !errors.Is(err, test.want) {
				t.Fatalf("ValidateAndDiscover() error = %v, want %s/%v", err, test.wantCode, test.want)
			}
		})
	}
}

func TestValidBotTokenRejectsWhitespaceAndMalformedValues(t *testing.T) {
	for _, value := range []string{"", " " + testBotToken, testBotToken + " ", "123:short", "123456:aaaaaaaaaaaaaaaaaaa\n"} {
		if ValidBotToken(value) {
			t.Fatalf("ValidBotToken(%q) = true", value)
		}
	}
}
