package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	telegramAPIBaseURL     = "https://api.telegram.org"
	telegramRequestTimeout = 6 * time.Second
	telegramResponseBytes  = int64(64 << 10)
	telegramMessageRunes   = 4096
)

var telegramBotTokenPattern = regexp.MustCompile(`^[0-9]{5,16}:[A-Za-z0-9_-]{20,256}$`)

type TelegramAPI interface {
	ValidateAndDiscover(context.Context, string) (BotInfo, int64, error)
	SendMessage(context.Context, string, int64, string) error
}

type TelegramClient struct {
	baseURL string
	client  *http.Client
}

func NewTelegramClient() *TelegramClient {
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		MaxIdleConns:          4,
		MaxConnsPerHost:       2,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return &TelegramClient{
		baseURL: telegramAPIBaseURL,
		client:  &http.Client{Transport: transport, Timeout: telegramRequestTimeout},
	}
}

func newTelegramClientForTest(baseURL string, client *http.Client) *TelegramClient {
	if client == nil {
		client = &http.Client{Timeout: telegramRequestTimeout}
	}
	return &TelegramClient{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

func ValidBotToken(value string) bool {
	return len(value) <= MaxTelegramTokenBytes && telegramBotTokenPattern.MatchString(value)
}

type telegramResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
}

type telegramBot struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type telegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID        int64  `json:"id"`
			Type      string `json:"type"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		} `json:"chat"`
	} `json:"message,omitempty"`
}

func (c *TelegramClient) ValidateAndDiscover(ctx context.Context, token string) (BotInfo, int64, error) {
	if !ValidBotToken(token) {
		return BotInfo{}, 0, &Error{Code: "invalid_token", Cause: ErrTelegramInvalidToken}
	}
	var me telegramBot
	if err := c.call(ctx, token, "getMe", nil, &me); err != nil {
		return BotInfo{}, 0, err
	}
	if me.ID <= 0 || !me.IsBot {
		return BotInfo{}, 0, &Error{Code: "invalid_token", Cause: ErrTelegramInvalidToken}
	}
	var updates []telegramUpdate
	if err := c.call(ctx, token, "getUpdates", map[string]any{
		"limit":   100,
		"timeout": 0,
	}, &updates); err != nil {
		return BotInfo{}, 0, err
	}
	var chatID int64
	var newest int64
	for _, update := range updates {
		if update.UpdateID < newest || update.Message == nil ||
			update.Message.Chat.Type != "private" || update.Message.Chat.ID == 0 ||
			!isStartCommand(update.Message.Text) {
			continue
		}
		chatID = update.Message.Chat.ID
		newest = update.UpdateID
	}
	if chatID == 0 {
		return BotInfo{ID: me.ID, FirstName: cleanBotText(me.FirstName), Username: cleanBotText(me.Username)}, 0,
			&Error{Code: "chat_not_found", Cause: ErrChatNotFound}
	}
	return BotInfo{ID: me.ID, FirstName: cleanBotText(me.FirstName), Username: cleanBotText(me.Username)}, chatID, nil
}

func (c *TelegramClient) SendMessage(ctx context.Context, token string, chatID int64, message string) error {
	if !ValidBotToken(token) {
		return &Error{Code: "invalid_token", Cause: ErrTelegramInvalidToken}
	}
	if chatID == 0 || strings.TrimSpace(message) == "" || utf8.RuneCountInString(message) > telegramMessageRunes {
		return &Error{Code: "invalid_message", Cause: ErrNotReady}
	}
	var result telegramBot
	return c.call(ctx, token, "sendMessage", map[string]any{
		"chat_id":                  chatID,
		"text":                     message,
		"disable_web_page_preview": true,
	}, &result)
}

func (c *TelegramClient) call(ctx context.Context, token, method string, payload any, result any) error {
	if c == nil || c.client == nil || strings.TrimSpace(c.baseURL) == "" {
		return &Error{Code: "unavailable", Retryable: true, Cause: ErrTelegramUnavailable}
	}
	var body io.Reader = http.NoBody
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return &Error{Code: "unavailable", Retryable: true, Cause: ErrTelegramUnavailable}
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/bot"+token+"/"+method, body,
	)
	if err != nil {
		return &Error{Code: "unavailable", Retryable: true, Cause: ErrTelegramUnavailable}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "KPanel-Telegram-Notifications")
	response, err := c.client.Do(request)
	if err != nil {
		return &Error{Code: "unavailable", Retryable: true, Cause: ErrTelegramUnavailable}
	}
	defer response.Body.Close()
	content, readErr := io.ReadAll(io.LimitReader(response.Body, telegramResponseBytes+1))
	if readErr != nil || int64(len(content)) > telegramResponseBytes {
		return &Error{Code: "invalid_response", Retryable: true, Cause: ErrTelegramUnavailable}
	}
	var envelope telegramResponse[json.RawMessage]
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(&envelope); err != nil {
		return &Error{Code: "invalid_response", Retryable: true, Cause: ErrTelegramUnavailable}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return &Error{Code: "invalid_response", Retryable: true, Cause: ErrTelegramUnavailable}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || !envelope.OK {
		return telegramResponseError(response.StatusCode, envelope.ErrorCode)
	}
	if result == nil || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return &Error{Code: "invalid_response", Retryable: true, Cause: ErrTelegramUnavailable}
	}
	return nil
}

func telegramResponseError(statusCode, apiCode int) error {
	switch {
	case statusCode == http.StatusUnauthorized || apiCode == http.StatusUnauthorized:
		return &Error{Code: "invalid_token", Cause: ErrTelegramInvalidToken}
	case statusCode == http.StatusConflict || apiCode == http.StatusConflict:
		return &Error{Code: "webhook_active", Cause: ErrTelegramWebhookActive}
	case statusCode == http.StatusTooManyRequests || apiCode == http.StatusTooManyRequests:
		return &Error{Code: "rate_limited", Retryable: true, Cause: ErrTelegramUnavailable}
	case statusCode >= http.StatusInternalServerError:
		return &Error{Code: "unavailable", Retryable: true, Cause: ErrTelegramUnavailable}
	default:
		return &Error{Code: "api_error", Retryable: true, Cause: ErrTelegramUnavailable}
	}
}

func isStartCommand(value string) bool {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return false
	}
	command := strings.ToLower(fields[0])
	return command == "/start" || strings.HasPrefix(command, "/start@")
}

func cleanBotText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 64 || strings.ContainsAny(value, "\r\n\t\x00") {
		return ""
	}
	return value
}

var _ TelegramAPI = (*TelegramClient)(nil)
