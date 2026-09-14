package notification

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	robotRequestTimeout = 6 * time.Second
	robotResponseBytes  = int64(64 << 10)
	channelMessageRunes = 4096
)

type RobotAPI interface {
	SendMessage(context.Context, Provider, string, string) error
}

type RobotClient struct {
	client *http.Client
}

func NewRobotClient() *RobotClient {
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
	return &RobotClient{client: &http.Client{
		Transport: transport,
		Timeout:   robotRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func newRobotClientForTest(client *http.Client) *RobotClient {
	if client == nil {
		client = &http.Client{Timeout: robotRequestTimeout}
	}
	return &RobotClient{client: client}
}

func (c *RobotClient) SendMessage(ctx context.Context, provider Provider, credential, message string) error {
	if !validProvider(provider) || provider == ProviderTelegram {
		return &Error{Code: "unsupported_provider", Cause: ErrUnsupportedProvider}
	}
	if !ValidChannelCredential(provider, credential) {
		return &Error{Code: "invalid_credential", Cause: ErrInvalidCredential}
	}
	if strings.TrimSpace(message) == "" || !utf8.ValidString(message) || utf8.RuneCountInString(message) > channelMessageRunes {
		return &Error{Code: "invalid_message", Cause: ErrNotReady}
	}
	payload := map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": message},
	}
	if provider == ProviderFeishu {
		payload = map[string]any{
			"msg_type": "text",
			"content":  map[string]string{"text": message},
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return &Error{Code: "unavailable", Retryable: true, Cause: ErrChannelUnavailable}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, credential, bytes.NewReader(encoded))
	if err != nil {
		return &Error{Code: "invalid_credential", Cause: ErrInvalidCredential}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "KPanel-Cluster-Notifications")
	if c == nil || c.client == nil {
		return &Error{Code: "unavailable", Retryable: true, Cause: ErrChannelUnavailable}
	}
	response, err := c.client.Do(request)
	if err != nil {
		return &Error{Code: "unavailable", Retryable: true, Cause: ErrChannelUnavailable}
	}
	defer response.Body.Close()
	content, readErr := io.ReadAll(io.LimitReader(response.Body, robotResponseBytes+1))
	if readErr != nil || int64(len(content)) > robotResponseBytes {
		return &Error{Code: "invalid_response", Retryable: true, Cause: ErrChannelUnavailable}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return robotResponseError(response.StatusCode)
	}
	if !robotResponseSucceeded(provider, content) {
		return &Error{Code: "api_error", Cause: ErrChannelUnavailable}
	}
	return nil
}

type robotResponse struct {
	Code       *int `json:"code"`
	ErrCode    *int `json:"errcode"`
	StatusCode *int `json:"StatusCode"`
}

func robotResponseSucceeded(provider Provider, content []byte) bool {
	var envelope robotResponse
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(&envelope); err != nil {
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return false
	}
	if provider == ProviderFeishu {
		return (envelope.Code != nil && *envelope.Code == 0) ||
			(envelope.StatusCode != nil && *envelope.StatusCode == 0)
	}
	return envelope.ErrCode != nil && *envelope.ErrCode == 0
}

func robotResponseError(statusCode int) error {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return &Error{Code: "invalid_credential", Cause: ErrInvalidCredential}
	case statusCode == http.StatusTooManyRequests:
		return &Error{Code: "rate_limited", Retryable: true, Cause: ErrChannelUnavailable}
	case statusCode >= http.StatusInternalServerError:
		return &Error{Code: "unavailable", Retryable: true, Cause: ErrChannelUnavailable}
	default:
		return &Error{Code: "api_error", Cause: ErrChannelUnavailable}
	}
}

var _ RobotAPI = (*RobotClient)(nil)
