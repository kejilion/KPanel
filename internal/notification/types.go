package notification

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	StateSchemaVersion              = 1
	DefaultCPUThresholdPercent      = 90
	DefaultMemoryThresholdPercent   = 90
	DefaultDiskThresholdPercent     = 90
	DefaultTrafficThresholdMiB      = 100
	DefaultTrafficTotalThresholdGiB = 100
	DefaultSustainSamples           = 3
	MaxTrafficThresholdMiB          = 1_048_576
	MaxTrafficTotalThresholdGiB     = 1_048_576
	MaxAlertStates                  = 1_024
	MaxTelegramTokenBytes           = 256
	DefaultNotificationLocale       = "zh-CN"
)

var (
	ErrConflict              = errors.New("notification settings changed")
	ErrTokenRequired         = errors.New("telegram bot token is required")
	ErrNotConfigured         = errors.New("telegram notifications are not configured")
	ErrNotReady              = errors.New("telegram notifications are not ready")
	ErrChatNotFound          = errors.New("telegram private chat was not found")
	ErrInvalidSettings       = errors.New("notification settings are invalid")
	ErrTelegramUnavailable   = errors.New("telegram bot api is unavailable")
	ErrTelegramInvalidToken  = errors.New("telegram bot token is invalid")
	ErrTelegramWebhookActive = errors.New("telegram bot webhook is active")
)

type Error struct {
	Code      string
	Retryable bool
	Cause     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error { return ErrInvalidSettings }

type Rules struct {
	CPUEnabled                   bool `json:"cpuEnabled"`
	CPUThresholdPercent          int  `json:"cpuThresholdPercent"`
	MemoryEnabled                bool `json:"memoryEnabled"`
	MemoryThresholdPercent       int  `json:"memoryThresholdPercent"`
	DiskEnabled                  bool `json:"diskEnabled"`
	DiskThresholdPercent         int  `json:"diskThresholdPercent"`
	TrafficEnabled               bool `json:"trafficEnabled"`
	TrafficThresholdMiBPerSecond int  `json:"trafficThresholdMiBPerSecond"`
	TrafficTotalEnabled          bool `json:"trafficTotalEnabled"`
	TrafficTotalThresholdGiB     int  `json:"trafficTotalThresholdGiB"`
	SSHLoginEnabled              bool `json:"sshLoginEnabled"`
	HostOfflineEnabled           bool `json:"hostOfflineEnabled"`
}

func DefaultRules() Rules {
	return Rules{
		CPUEnabled: true, CPUThresholdPercent: DefaultCPUThresholdPercent,
		MemoryEnabled: true, MemoryThresholdPercent: DefaultMemoryThresholdPercent,
		DiskEnabled: true, DiskThresholdPercent: DefaultDiskThresholdPercent,
		TrafficEnabled: false, TrafficThresholdMiBPerSecond: DefaultTrafficThresholdMiB,
		TrafficTotalEnabled: false, TrafficTotalThresholdGiB: DefaultTrafficTotalThresholdGiB,
		SSHLoginEnabled: true, HostOfflineEnabled: true,
	}
}

func (r Rules) Validate() error {
	checks := []struct {
		field string
		value int
	}{
		{"rules.cpuThresholdPercent", r.CPUThresholdPercent},
		{"rules.memoryThresholdPercent", r.MemoryThresholdPercent},
		{"rules.diskThresholdPercent", r.DiskThresholdPercent},
	}
	for _, check := range checks {
		if check.value < 1 || check.value > 100 {
			return &ValidationError{Field: check.field, Message: "阈值必须在 1 到 100 之间"}
		}
	}
	if r.TrafficThresholdMiBPerSecond < 1 || r.TrafficThresholdMiBPerSecond > MaxTrafficThresholdMiB {
		return &ValidationError{Field: "rules.trafficThresholdMiBPerSecond", Message: "阈值超出允许范围"}
	}
	if r.TrafficTotalThresholdGiB < 1 || r.TrafficTotalThresholdGiB > MaxTrafficTotalThresholdGiB {
		return &ValidationError{Field: "rules.trafficTotalThresholdGiB", Message: "累计流量阈值超出允许范围"}
	}
	return nil
}

type Settings struct {
	Enabled bool   `json:"enabled"`
	Locale  string `json:"locale"`
	Rules   Rules  `json:"rules"`
}

type UpdateInput struct {
	Enabled                 bool   `json:"enabled"`
	Locale                  string `json:"locale,omitempty"`
	Rules                   Rules  `json:"rules"`
	TelegramBotToken        string `json:"telegramBotToken,omitempty"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

type TelegramStatus string

const (
	TelegramNotConfigured TelegramStatus = "not_configured"
	TelegramWaitingChat   TelegramStatus = "waiting_for_chat"
	TelegramReady         TelegramStatus = "ready"
	TelegramError         TelegramStatus = "error"
)

type TelegramSnapshot struct {
	Configured    bool           `json:"configured"`
	Ready         bool           `json:"ready"`
	Status        TelegramStatus `json:"status"`
	BotUsername   string         `json:"botUsername,omitempty"`
	LastCheckedAt *time.Time     `json:"lastCheckedAt,omitempty"`
	LastSuccessAt *time.Time     `json:"lastSuccessAt,omitempty"`
	LastErrorCode string         `json:"lastErrorCode,omitempty"`
}

type Snapshot struct {
	Enabled         bool             `json:"enabled"`
	Locale          string           `json:"locale"`
	Timezone        string           `json:"timezone"`
	Rules           Rules            `json:"rules"`
	Telegram        TelegramSnapshot `json:"telegram"`
	ResourceVersion string           `json:"resourceVersion"`
	UpdatedAt       time.Time        `json:"updatedAt"`
}

type BotInfo struct {
	ID        int64
	FirstName string
	Username  string
}

func normalizeRules(value Rules) Rules {
	if value.TrafficTotalThresholdGiB == 0 {
		value.TrafficTotalThresholdGiB = DefaultTrafficTotalThresholdGiB
	}
	return value
}

func normalizeNotificationLocale(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultNotificationLocale
	}
	return value
}

func validNotificationLocale(value string) bool {
	switch value {
	case "zh-CN", "zh-TW", "en-US":
		return true
	default:
		return false
	}
}
