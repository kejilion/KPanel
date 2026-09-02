package notification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	defaultEvaluationInterval            = 30 * time.Second
	alertRetryInterval                   = 5 * time.Minute
	alertRepeatInterval                  = 6 * time.Hour
	maxMessagesPerEvaluation             = 8
	telegramSendTimeout                  = 6 * time.Second
	cumulativeTrafficReceivedRuleKey     = "traffic-total-received"
	cumulativeTrafficSentRuleKey         = "traffic-total-sent"
	cumulativeTrafficReceivedAlertSuffix = ":" + cumulativeTrafficReceivedRuleKey
	cumulativeTrafficSentAlertSuffix     = ":" + cumulativeTrafficSentRuleKey
	legacyCumulativeTrafficAlertSuffix   = ":traffic-total"
	bytesPerGigabyte                     = uint64(1024 * 1024 * 1024)
)

var cumulativeTrafficAlertSuffixes = []string{
	cumulativeTrafficReceivedAlertSuffix,
	cumulativeTrafficSentAlertSuffix,
	legacyCumulativeTrafficAlertSuffix,
}

type HostSource interface {
	Hosts(context.Context) cluster.HostList
}

// TimezoneSource returns the timezone used by the notification center host.
// The source may fall back to the process timezone when the host configuration
// cannot be read; notification timestamps are still persisted as UTC.
type TimezoneSource func(context.Context) *time.Location

type Config struct {
	DataDir            string
	Hosts              HostSource
	Telegram           TelegramAPI
	Timezone           TimezoneSource
	Now                func() time.Time
	EvaluationInterval time.Duration
	SustainSamples     int
	RepeatInterval     time.Duration
}

type trafficSample struct {
	received uint64
	sent     uint64
	at       time.Time
}

type Service struct {
	store      *Store
	hosts      HostSource
	telegram   TelegramAPI
	timezone   TimezoneSource
	now        func() time.Time
	evaluation time.Duration
	sustain    int
	repeat     time.Duration

	opMu    sync.Mutex
	mu      sync.Mutex
	alerts  map[string]alertState
	traffic map[string]trafficSample

	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewService(config Config) (*Service, error) {
	if strings.TrimSpace(config.DataDir) == "" || !filepath.IsAbs(config.DataDir) {
		return nil, errors.New("notification data directory must be absolute")
	}
	if config.Hosts == nil {
		return nil, errors.New("notification host source is required")
	}
	if config.Telegram == nil {
		config.Telegram = NewTelegramClient()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.EvaluationInterval == 0 {
		config.EvaluationInterval = defaultEvaluationInterval
	}
	if config.EvaluationInterval < 100*time.Millisecond || config.EvaluationInterval > 10*time.Minute {
		return nil, errors.New("notification evaluation interval is invalid")
	}
	if config.SustainSamples == 0 {
		config.SustainSamples = DefaultSustainSamples
	}
	if config.SustainSamples < 1 || config.SustainSamples > 5 {
		return nil, errors.New("notification sustain sample count is invalid")
	}
	if config.RepeatInterval == 0 {
		config.RepeatInterval = alertRepeatInterval
	}
	if config.RepeatInterval < time.Minute || config.RepeatInterval > 7*24*time.Hour {
		return nil, errors.New("notification repeat interval is invalid")
	}
	store, err := Open(config.DataDir)
	if err != nil {
		return nil, err
	}
	state := store.stateSnapshot()
	service := &Service{
		store: store, hosts: config.Hosts, telegram: config.Telegram, timezone: config.Timezone, now: config.Now,
		evaluation: config.EvaluationInterval, sustain: config.SustainSamples,
		repeat: config.RepeatInterval, alerts: make(map[string]alertState),
		traffic: make(map[string]trafficSample),
	}
	for key, value := range state.AlertStates {
		service.alerts[key] = value
	}
	if state.ResourceVersion == "" {
		state.ResourceVersion = configResourceVersion(state.Settings, state.Telegram)
		if err := store.commitState(state); err != nil {
			return nil, err
		}
	}
	return service, nil
}

func (s *Service) Start(parent context.Context) {
	if parent == nil {
		parent = context.Background()
	}
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if s.started {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.started = true
	s.wg.Add(1)
	go s.run(ctx)
}

func (s *Service) run(ctx context.Context) {
	defer s.wg.Done()
	_ = s.evaluate(ctx)
	ticker := time.NewTicker(s.evaluation)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.evaluate(ctx)
		}
	}
}

func (s *Service) Close() error {
	s.opMu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.started = false
	s.opMu.Unlock()
	s.wg.Wait()
	return nil
}

func (s *Service) Snapshot() Snapshot {
	return s.snapshot(context.Background())
}

func (s *Service) snapshot(ctx context.Context) Snapshot {
	state := s.store.stateSnapshot()
	now := s.now()
	meta := state.Telegram
	token, configured, tokenErr := s.store.token()
	if tokenErr != nil {
		configured = true
		meta.Status = TelegramError
		meta.LastErrorCode = "token_file_unavailable"
	}
	if !configured {
		meta = telegramState{Status: TelegramNotConfigured}
	} else if tokenErr == nil && meta.TokenFingerprint != tokenFingerprint(token) {
		meta.Status = TelegramError
		meta.LastErrorCode = "token_changed"
		meta.HasChat = false
	} else if !meta.HasChat {
		meta.Status = TelegramWaitingChat
	}
	return snapshotFromState(state, meta, configured && tokenErr == nil, notificationTimezone(s.displayTime(ctx, now)))
}

func (s *Service) Configure(ctx context.Context, input UpdateInput) (Snapshot, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	state := s.store.stateSnapshot()
	if input.ExpectedResourceVersion == "" || input.ExpectedResourceVersion != state.ResourceVersion {
		return Snapshot{}, ErrConflict
	}
	rules := normalizeRules(input.Rules)
	locale := normalizeNotificationLocale(input.Locale)
	if !validNotificationLocale(locale) {
		return Snapshot{}, &ValidationError{Field: "locale", Message: "通知语言不受支持"}
	}
	if err := rules.Validate(); err != nil {
		return Snapshot{}, err
	}
	newToken := strings.TrimSpace(input.TelegramBotToken)
	if newToken != "" && !ValidBotToken(newToken) {
		return Snapshot{}, &Error{Code: "invalid_token", Cause: ErrTelegramInvalidToken}
	}
	oldToken, oldPresent, err := s.store.token()
	if err != nil && !errors.Is(err, ErrTelegramInvalidToken) {
		return Snapshot{}, &Error{Code: "token_file_unavailable", Retryable: true, Cause: ErrNotConfigured}
	}
	if err := s.ensureEnabledCanRun(input.Enabled, newToken, oldToken, oldPresent, state.Telegram); err != nil {
		return Snapshot{}, err
	}
	next := state
	next.Settings = Settings{Enabled: input.Enabled, Locale: locale, Rules: rules}
	next.AlertStates = s.alertStateSnapshot()
	if cumulativeTrafficRulesChanged(state.Settings.Rules, rules) {
		removeCumulativeTrafficAlertStates(next.AlertStates)
	}
	// The aggregate state is not meaningful for the directional rules, even
	// when the migrated settings happen to retain the same threshold value.
	removeAlertStates(next.AlertStates, legacyCumulativeTrafficAlertSuffix)
	next.UpdatedAt = s.now().UTC()
	tokenChanged := false
	if newToken != "" {
		bot, chatID, discoverErr := s.telegram.ValidateAndDiscover(ctx, newToken)
		if discoverErr != nil {
			return Snapshot{}, discoverErr
		}
		next.Telegram = readyTelegramState(newToken, bot, chatID, s.now().UTC())
		tokenChanged = !oldPresent || oldToken != newToken
	} else if !oldPresent {
		next.Telegram = telegramState{Status: TelegramNotConfigured}
	}
	next.ResourceVersion = configResourceVersion(next.Settings, next.Telegram)
	if tokenChanged {
		if err := s.store.replaceToken(newToken); err != nil {
			return Snapshot{}, &Error{Code: "token_store_unavailable", Retryable: true, Cause: err}
		}
	}
	if err := s.store.commitState(next); err != nil {
		if tokenChanged {
			_ = s.store.restoreToken(oldToken, oldPresent)
		}
		return Snapshot{}, &Error{Code: "state_store_unavailable", Retryable: true, Cause: err}
	}
	s.replaceAlertStates(next.AlertStates)
	return s.snapshot(ctx), nil
}

func (s *Service) Discover(ctx context.Context, expectedResourceVersion string) (Snapshot, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	state := s.store.stateSnapshot()
	if expectedResourceVersion == "" || expectedResourceVersion != state.ResourceVersion {
		return Snapshot{}, ErrConflict
	}
	token, configured, err := s.store.token()
	if err != nil {
		if errors.Is(err, ErrTelegramInvalidToken) {
			return Snapshot{}, &Error{Code: "invalid_token", Cause: err}
		}
		return Snapshot{}, &Error{Code: "token_file_unavailable", Retryable: true, Cause: err}
	}
	if !configured {
		return Snapshot{}, ErrNotConfigured
	}
	bot, chatID, discoverErr := s.telegram.ValidateAndDiscover(ctx, token)
	now := s.now().UTC()
	if discoverErr != nil {
		state.Telegram.TokenFingerprint = tokenFingerprint(token)
		state.Telegram.HasChat = false
		state.Telegram.Status = TelegramError
		state.Telegram.LastCheckedAt = timePtr(now)
		state.Telegram.LastErrorCode = telegramCode(discoverErr)
		if errors.Is(discoverErr, ErrChatNotFound) {
			state.Telegram.Status = TelegramWaitingChat
		}
		state.UpdatedAt = now
		_ = s.store.commitState(state)
		return Snapshot{}, discoverErr
	}
	state.Telegram = readyTelegramState(token, bot, chatID, now)
	state.ResourceVersion = configResourceVersion(state.Settings, state.Telegram)
	state.UpdatedAt = now
	if err := s.store.commitState(state); err != nil {
		return Snapshot{}, &Error{Code: "state_store_unavailable", Retryable: true, Cause: err}
	}
	return s.snapshot(ctx), nil
}

func (s *Service) Test(ctx context.Context) (Snapshot, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	state := s.store.stateSnapshot()
	token, configured, err := s.store.token()
	if err != nil {
		if errors.Is(err, ErrTelegramInvalidToken) {
			return Snapshot{}, &Error{Code: "invalid_token", Cause: err}
		}
		return Snapshot{}, &Error{Code: "token_file_unavailable", Retryable: true, Cause: err}
	}
	if !configured {
		return Snapshot{}, ErrNotConfigured
	}
	if !state.Telegram.HasChat || state.Telegram.TokenFingerprint != tokenFingerprint(token) {
		return Snapshot{}, ErrNotReady
	}
	displayNow := s.displayTime(ctx, s.now())
	err = s.telegram.SendMessage(ctx, token, state.Telegram.ChatID, testMessage(displayNow, state.Settings.Locale))
	now := displayNow.UTC()
	if err != nil {
		state.Telegram.Status = TelegramError
		state.Telegram.LastErrorCode = telegramCode(err)
		state.Telegram.LastCheckedAt = timePtr(now)
		state.UpdatedAt = now
		_ = s.store.commitState(state)
		return Snapshot{}, err
	}
	state.Telegram.Status = TelegramReady
	state.Telegram.LastCheckedAt = timePtr(now)
	state.Telegram.LastSuccessAt = timePtr(now)
	state.Telegram.LastErrorCode = ""
	state.UpdatedAt = now
	if err := s.store.commitState(state); err != nil {
		return Snapshot{}, &Error{Code: "state_store_unavailable", Retryable: true, Cause: err}
	}
	return s.snapshot(ctx), nil
}

func (s *Service) ensureEnabledCanRun(enabled bool, newToken, oldToken string, oldPresent bool, meta telegramState) error {
	if !enabled {
		return nil
	}
	if newToken != "" {
		return nil
	}
	if !oldPresent || oldToken == "" {
		return ErrTokenRequired
	}
	if meta.Status != TelegramReady || !meta.HasChat || meta.TokenFingerprint != tokenFingerprint(oldToken) {
		return ErrNotReady
	}
	return nil
}

func (s *Service) evaluate(parent context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if parent == nil {
		parent = context.Background()
	}
	state := s.store.stateSnapshot()
	if !state.Settings.Enabled {
		return nil
	}
	token, configured, tokenErr := s.store.token()
	if tokenErr != nil || !configured || !state.Telegram.HasChat || state.Telegram.TokenFingerprint != tokenFingerprint(token) {
		return nil
	}
	fetchCtx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	hosts := s.hosts.Hosts(fetchCtx)
	now := s.displayTime(parent, s.now())
	locale := normalizeNotificationLocale(state.Settings.Locale)
	telegram := state.Telegram
	sent := 0
	stateChanged := s.pruneHostState(hosts.Items)
	trySend := func(message string) (bool, bool) {
		if sent >= maxMessagesPerEvaluation {
			return false, false
		}
		sent++
		sendCtx, cancel := context.WithTimeout(parent, telegramSendTimeout)
		err := s.telegram.SendMessage(sendCtx, token, telegram.ChatID, message)
		cancel()
		telegram.LastCheckedAt = timePtr(now)
		if err != nil {
			telegram.Status = TelegramError
			telegram.LastErrorCode = telegramCode(err)
			return false, true
		}
		telegram.Status = TelegramReady
		telegram.LastSuccessAt = timePtr(now)
		telegram.LastErrorCode = ""
		return true, true
	}
	for _, host := range hosts.Items {
		if host.LastSnapshot == nil {
			if state.Settings.Rules.HostOfflineEnabled {
				stateChanged = s.handleAvailability(host, now, locale, trySend) || stateChanged
			}
			continue
		}
		rate, rateAvailable := s.trafficRate(host, now)
		if state.Settings.Rules.HostOfflineEnabled {
			stateChanged = s.handleAvailability(host, now, locale, trySend) || stateChanged
		}
		if state.Settings.Rules.SSHLoginEnabled && host.LastSnapshot.Telemetry.SSHLogin != nil {
			stateChanged = s.handleSSHLogin(host, *host.LastSnapshot.Telemetry.SSHLogin, now, locale, trySend) || stateChanged
		}
		if host.State != cluster.HostOnline && host.State != cluster.HostDegraded {
			continue
		}
		rules := state.Settings.Rules
		if rules.CPUEnabled && cpuMetricAvailable(host.LastSnapshot.Telemetry) {
			stateChanged = s.handleThreshold(host, "cpu", host.LastSnapshot.Telemetry.CPU.UsagePercent, float64(rules.CPUThresholdPercent), "%", now, locale, trySend) || stateChanged
		}
		if rules.MemoryEnabled && memoryMetricAvailable(host.LastSnapshot.Telemetry) {
			stateChanged = s.handleThreshold(host, "memory", host.LastSnapshot.Telemetry.Memory.UsagePercent, float64(rules.MemoryThresholdPercent), "%", now, locale, trySend) || stateChanged
		}
		if rules.DiskEnabled && diskMetricAvailable(host.LastSnapshot.Telemetry) {
			stateChanged = s.handleThreshold(host, "disk", host.LastSnapshot.Telemetry.Disk.UsagePercent, float64(rules.DiskThresholdPercent), "%", now, locale, trySend) || stateChanged
		}
		if rules.TrafficEnabled && rateAvailable {
			stateChanged = s.handleThreshold(host, "traffic", rate, float64(rules.TrafficThresholdMiBPerSecond), "MiB/s", now, locale, trySend) || stateChanged
		}
		if rules.TrafficTotalReceivedEnabled {
			stateChanged = s.handleCumulativeThreshold(host, cumulativeTrafficReceivedRuleKey, host.LastSnapshot.Telemetry.Network.ReceivedBytes, rules.TrafficTotalReceivedThresholdGiB, now, locale, trySend) || stateChanged
		}
		if rules.TrafficTotalSentEnabled {
			stateChanged = s.handleCumulativeThreshold(host, cumulativeTrafficSentRuleKey, host.LastSnapshot.Telemetry.Network.SentBytes, rules.TrafficTotalSentThresholdGiB, now, locale, trySend) || stateChanged
		}
	}
	alertStates := s.alertStateSnapshot()
	if !stateChanged && reflect.DeepEqual(telegram, state.Telegram) && reflect.DeepEqual(alertStates, state.AlertStates) {
		return nil
	}
	state.Telegram = telegram
	state.AlertStates = alertStates
	return s.store.commitState(state)
}

func (s *Service) handleThreshold(
	host cluster.Host,
	ruleKey string,
	value, threshold float64,
	unit string,
	now time.Time,
	locale string,
	send func(string) (bool, bool),
) bool {
	if !finiteMetric(value) || (unit == "%" && (value < 0 || value > 100)) {
		return false
	}
	if value < threshold {
		return s.handleThresholdRecovery(host, ruleKey, value, unit, locale, now, send)
	}
	key := host.ID + ":" + ruleKey
	state := s.getAlertState(key)
	state.Consecutive = minInt(state.Consecutive+1, s.sustain)
	if !state.Active && state.Consecutive >= s.sustain && canAlertAttempt(state, now) {
		state.LastAttemptAt = now
		s.setAlertState(key, state)
		success, attempted := send(resourceAlertMessage(host, ruleKey, value, threshold, unit, now, locale, false))
		if !attempted {
			state.LastAttemptAt = time.Time{}
			s.setAlertState(key, state)
			return false
		}
		if success {
			state.Active = true
			state.LastNotifiedAt = now
			state.LastValue = value
			s.setAlertState(key, state)
			return true
		}
		s.setAlertState(key, state)
		return false
	}
	if state.Active && now.Sub(state.LastNotifiedAt) >= s.repeat && canAlertAttempt(state, now) {
		state.LastAttemptAt = now
		s.setAlertState(key, state)
		success, attempted := send(resourceAlertMessage(host, ruleKey, value, threshold, unit, now, locale, false))
		if !attempted {
			state.LastAttemptAt = time.Time{}
			s.setAlertState(key, state)
			return false
		}
		if success {
			state.LastNotifiedAt = now
			state.LastValue = value
			s.setAlertState(key, state)
			return true
		}
	}
	s.setAlertState(key, state)
	return false
}

func (s *Service) handleThresholdRecovery(host cluster.Host, ruleKey string, value float64, unit, locale string, now time.Time, send func(string) (bool, bool)) bool {
	key := host.ID + ":" + ruleKey
	state := s.getAlertState(key)
	state.Consecutive = 0
	if !state.Active || !canRecoveryAttempt(state, now) {
		s.setAlertState(key, state)
		return false
	}
	state.LastAttemptAt = now
	s.setAlertState(key, state)
	success, attempted := send(resourceAlertMessage(host, ruleKey, value, 0, unit, now, locale, true))
	if !attempted {
		state.LastAttemptAt = time.Time{}
		s.setAlertState(key, state)
		return false
	}
	if success {
		state.Active = false
		state.LastNotifiedAt = now
		state.LastValue = value
		s.setAlertState(key, state)
		return true
	}
	s.setAlertState(key, state)
	return false
}

func (s *Service) handleCumulativeThreshold(host cluster.Host, ruleKey string, value uint64, thresholdGiB int, now time.Time, locale string, send func(string) (bool, bool)) bool {
	if thresholdGiB <= 0 || thresholdGiB > MaxTrafficTotalThresholdGiB {
		return false
	}
	thresholdBytes := uint64(thresholdGiB) * bytesPerGigabyte
	key := host.ID + ":" + ruleKey
	state := s.getAlertState(key)
	if state.LastNetworkBytes > 0 && value < state.LastNetworkBytes {
		// Network counters are monotonic until an interface, host or agent
		// restarts. A rollback starts a new accumulation cycle and must not
		// generate a misleading recovery message.
		state = alertState{}
	}
	state.LastNetworkBytes = value
	state.Consecutive = 0
	if value < thresholdBytes {
		state.Active = false
		state.LastAttemptAt = time.Time{}
		s.setAlertState(key, state)
		return false
	}
	if state.Active || !canAlertAttempt(state, now) {
		s.setAlertState(key, state)
		return false
	}
	state.LastAttemptAt = now
	s.setAlertState(key, state)
	success, attempted := send(cumulativeTrafficAlertMessage(host, ruleKey, value, thresholdBytes, now, locale))
	if !attempted {
		state.LastAttemptAt = time.Time{}
		s.setAlertState(key, state)
		return false
	}
	if success {
		state.Active = true
		state.LastNotifiedAt = now
		s.setAlertState(key, state)
		return true
	}
	s.setAlertState(key, state)
	return false
}

func (s *Service) handleAvailability(host cluster.Host, now time.Time, locale string, send func(string) (bool, bool)) bool {
	key := host.ID + ":availability"
	state := s.getAlertState(key)
	unavailable := host.State == cluster.HostStale || host.State == cluster.HostOffline ||
		host.State == cluster.HostAuthFailed || host.State == cluster.HostTLSFailed || host.State == cluster.HostIncompatible
	if !unavailable && host.State != cluster.HostOnline && host.State != cluster.HostDegraded {
		s.setAlertState(key, state)
		return false
	}
	if !unavailable {
		if !state.Active || !canRecoveryAttempt(state, now) {
			s.setAlertState(key, state)
			return false
		}
		state.LastAttemptAt = now
		s.setAlertState(key, state)
		success, attempted := send(availabilityAlertMessage(host, now, locale, true))
		if !attempted {
			state.LastAttemptAt = time.Time{}
			s.setAlertState(key, state)
			return false
		}
		if success {
			state.Active = false
			state.LastNotifiedAt = now
			s.setAlertState(key, state)
			return true
		}
		s.setAlertState(key, state)
		return false
	}
	if state.Active || !canAlertAttempt(state, now) {
		s.setAlertState(key, state)
		return false
	}
	state.LastAttemptAt = now
	s.setAlertState(key, state)
	success, attempted := send(availabilityAlertMessage(host, now, locale, false))
	if !attempted {
		state.LastAttemptAt = time.Time{}
		s.setAlertState(key, state)
		return false
	}
	if success {
		state.Active = true
		state.LastNotifiedAt = now
		s.setAlertState(key, state)
		return true
	}
	return false
}

func (s *Service) handleSSHLogin(host cluster.Host, event contract.SSHLoginEvent, now time.Time, locale string, send func(string) (bool, bool)) bool {
	if !contract.ValidSSHLoginEvent(event) {
		return false
	}
	key := host.ID + ":ssh"
	state := s.getAlertState(key)
	if state.LastEventID == event.ID {
		return false
	}
	if state.LastEventID == "" && state.PendingEventID == "" {
		// The first observation establishes a baseline. Otherwise enabling the
		// feature or restarting a fresh controller would report an old login as
		// a new event.
		state.LastEventID = event.ID
		state.LastNotifiedAt = time.Time{}
		s.setAlertState(key, state)
		return true
	}
	if state.PendingEventID == event.ID && !canAlertAttempt(state, now) {
		return false
	}
	state.PendingEventID = event.ID
	state.LastAttemptAt = now
	s.setAlertState(key, state)
	success, attempted := send(sshLoginMessage(host, event, now, locale))
	if !attempted {
		state.LastAttemptAt = time.Time{}
		state.PendingEventID = ""
		s.setAlertState(key, state)
		return false
	}
	if success {
		state.LastEventID = event.ID
		state.PendingEventID = ""
		state.LastNotifiedAt = now
		s.setAlertState(key, state)
		return true
	}
	return false
}

func (s *Service) getAlertState(key string) alertState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.alerts[key]
}

func (s *Service) setAlertState(key string, value alertState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.alerts[key]; !exists && len(s.alerts) >= MaxAlertStates {
		return
	}
	s.alerts[key] = value
}

func (s *Service) replaceAlertStates(states map[string]alertState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alerts = make(map[string]alertState, len(states))
	for key, value := range states {
		s.alerts[key] = value
	}
}

func removeAlertStates(states map[string]alertState, suffix string) {
	for key := range states {
		if strings.HasSuffix(key, suffix) {
			delete(states, key)
		}
	}
}

func removeCumulativeTrafficAlertStates(states map[string]alertState) {
	for _, suffix := range cumulativeTrafficAlertSuffixes {
		removeAlertStates(states, suffix)
	}
}

func cumulativeTrafficRulesChanged(previous, next Rules) bool {
	previous = normalizeRules(previous)
	next = normalizeRules(next)
	return previous.TrafficTotalReceivedEnabled != next.TrafficTotalReceivedEnabled ||
		previous.TrafficTotalReceivedThresholdGiB != next.TrafficTotalReceivedThresholdGiB ||
		previous.TrafficTotalSentEnabled != next.TrafficTotalSentEnabled ||
		previous.TrafficTotalSentThresholdGiB != next.TrafficTotalSentThresholdGiB
}

func (s *Service) pruneHostState(hosts []cluster.Host) bool {
	if len(hosts) == 0 {
		return false
	}
	known := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		if host.ID != "" {
			known[host.ID] = struct{}{}
		}
	}
	changed := false
	s.mu.Lock()
	for key := range s.alerts {
		hostID, _, found := strings.Cut(key, ":")
		if !found {
			delete(s.alerts, key)
			changed = true
			continue
		}
		if _, ok := known[hostID]; !ok {
			delete(s.alerts, key)
			changed = true
		}
	}
	for hostID := range s.traffic {
		if _, ok := known[hostID]; !ok {
			delete(s.traffic, hostID)
		}
	}
	s.mu.Unlock()
	return changed
}

func (s *Service) alertStateSnapshot() map[string]alertState {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]alertState, len(s.alerts))
	for key, value := range s.alerts {
		result[key] = value
	}
	return result
}

func (s *Service) trafficRate(host cluster.Host, now time.Time) (float64, bool) {
	if host.LastSnapshot == nil {
		return 0, false
	}
	telemetry := host.LastSnapshot.Telemetry
	if telemetry.Network.ReceivedBytes > uint64(math.MaxInt64) || telemetry.Network.SentBytes > uint64(math.MaxInt64) {
		return 0, false
	}
	sampleAt := host.LastSnapshot.ReceivedAt
	if sampleAt.IsZero() || sampleAt.After(now) {
		sampleAt = now
	}
	current := trafficSample{received: telemetry.Network.ReceivedBytes, sent: telemetry.Network.SentBytes, at: sampleAt}
	s.mu.Lock()
	previous, exists := s.traffic[host.ID]
	s.traffic[host.ID] = current
	s.mu.Unlock()
	if !exists {
		rate := host.LastSnapshot.ReceiveBytesPerSecond + host.LastSnapshot.TransmitBytesPerSecond
		if finiteMetric(rate) && rate > 0 {
			return rate / (1024 * 1024), true
		}
		return 0, false
	}
	if !current.at.After(previous.at) || current.received < previous.received || current.sent < previous.sent {
		return 0, false
	}
	elapsed := current.at.Sub(previous.at).Seconds()
	if elapsed <= 0 {
		return 0, false
	}
	rate := (float64(current.received-previous.received) + float64(current.sent-previous.sent)) / elapsed / (1024 * 1024)
	return rate, finiteMetric(rate)
}

func (s *Service) displayTime(ctx context.Context, value time.Time) time.Time {
	location := value.Location()
	if s.timezone != nil {
		if configured := s.timezone(ctx); configured != nil {
			location = configured
		}
	}
	return value.In(location)
}

func canAlertAttempt(state alertState, now time.Time) bool {
	return state.LastAttemptAt.IsZero() || now.Sub(state.LastAttemptAt) >= alertRetryInterval
}

func canRecoveryAttempt(state alertState, now time.Time) bool {
	if state.LastAttemptAt.IsZero() || !state.LastAttemptAt.After(state.LastNotifiedAt) {
		return true
	}
	return now.Sub(state.LastAttemptAt) >= alertRetryInterval
}

func finiteMetric(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func cpuMetricAvailable(telemetry contract.HostTelemetry) bool {
	return telemetry.CPU.Cores > 0 && telemetry.CPU.Cores <= 4096 && finiteMetric(telemetry.CPU.UsagePercent) && telemetry.CPU.UsagePercent <= 100
}

func memoryMetricAvailable(telemetry contract.HostTelemetry) bool {
	return telemetry.Memory.TotalBytes > 0 && telemetry.Memory.UsedBytes <= telemetry.Memory.TotalBytes &&
		telemetry.Memory.AvailableBytes <= telemetry.Memory.TotalBytes && finiteMetric(telemetry.Memory.UsagePercent) && telemetry.Memory.UsagePercent <= 100
}

func diskMetricAvailable(telemetry contract.HostTelemetry) bool {
	return telemetry.Disk.TotalBytes > 0 && telemetry.Disk.UsedBytes <= telemetry.Disk.TotalBytes &&
		finiteMetric(telemetry.Disk.UsagePercent) && telemetry.Disk.UsagePercent <= 100
}

func minInt(value, maximum int) int {
	if value > maximum {
		return maximum
	}
	return value
}

func snapshotFromState(state persistedState, telegram telegramState, configured bool, timezone string) Snapshot {
	ready := configured && telegram.HasChat && telegram.TokenFingerprint != "" && telegram.Status == TelegramReady
	return Snapshot{
		Enabled: state.Settings.Enabled, Locale: normalizeNotificationLocale(state.Settings.Locale), Timezone: timezone,
		Rules: normalizeRules(state.Settings.Rules),
		Telegram: TelegramSnapshot{
			Configured: configured, Ready: ready, Status: telegram.Status,
			BotUsername: telegram.BotUsername, LastCheckedAt: cloneTime(telegram.LastCheckedAt),
			LastSuccessAt: cloneTime(telegram.LastSuccessAt), LastErrorCode: telegram.LastErrorCode,
		},
		ResourceVersion: state.ResourceVersion, UpdatedAt: state.UpdatedAt,
	}
}

func readyTelegramState(token string, bot BotInfo, chatID int64, now time.Time) telegramState {
	return telegramState{
		TokenFingerprint: tokenFingerprint(token), BotID: bot.ID, BotUsername: bot.Username,
		ChatID: chatID, HasChat: true, Status: TelegramReady,
		LastCheckedAt: timePtr(now), LastSuccessAt: timePtr(now),
	}
}

func configResourceVersion(settings Settings, telegram telegramState) string {
	material := struct {
		Settings         Settings `json:"settings"`
		TokenFingerprint string   `json:"tokenFingerprint,omitempty"`
		BotID            int64    `json:"botId,omitempty"`
		BotUsername      string   `json:"botUsername,omitempty"`
		ChatID           int64    `json:"chatId,omitempty"`
		HasChat          bool     `json:"hasChat,omitempty"`
	}{
		Settings: settings, TokenFingerprint: telegram.TokenFingerprint,
		BotID: telegram.BotID, BotUsername: telegram.BotUsername,
		ChatID: telegram.ChatID, HasChat: telegram.HasChat,
	}
	content, _ := json.Marshal(material)
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func timePtr(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}

func telegramCode(err error) string {
	var typed *Error
	if errors.As(err, &typed) && typed.Code != "" {
		return typed.Code
	}
	switch {
	case errors.Is(err, ErrChatNotFound):
		return "chat_not_found"
	case errors.Is(err, ErrTelegramInvalidToken):
		return "invalid_token"
	case errors.Is(err, ErrTelegramWebhookActive):
		return "webhook_active"
	case errors.Is(err, ErrTelegramUnavailable):
		return "unavailable"
	default:
		return "notification_failed"
	}
}

func resourceAlertMessage(host cluster.Host, ruleKey string, value, threshold float64, unit string, now time.Time, locale string, recovery bool) string {
	hostName := safeMessageText(host.Name)
	metric := localizedMetricLabel(ruleKey, locale)
	current := formatMetric(value, unit)
	when := formatNotificationTime(now)
	switch locale {
	case "en-US":
		if recovery {
			return fmt.Sprintf("[KPanel Cluster Notice]\nHost: %s\nRecovered: %s, current %s\nTime: %s", hostName, metric, current, when)
		}
		return fmt.Sprintf("[KPanel Cluster Alert]\nHost: %s\n%s reached %s (threshold %s)\nTime: %s", hostName, metric, current, formatMetric(threshold, unit), when)
	case "zh-TW":
		if recovery {
			return fmt.Sprintf("[KPanel 叢集通知]\n主機：%s\n已恢復：%s 目前 %s\n時間：%s", hostName, metric, current, when)
		}
		return fmt.Sprintf("[KPanel 叢集警報]\n主機：%s\n%s達到 %s（閾值 %s）\n時間：%s", hostName, metric, current, formatMetric(threshold, unit), when)
	default:
		if recovery {
			return fmt.Sprintf("[KPanel 集群通知]\n主机：%s\n已恢复：%s 当前 %s\n时间：%s", hostName, metric, current, when)
		}
		return fmt.Sprintf("[KPanel 集群告警]\n主机：%s\n%s达到 %s（阈值 %s）\n时间：%s", hostName, metric, current, formatMetric(threshold, unit), when)
	}
}

func cumulativeTrafficAlertMessage(host cluster.Host, ruleKey string, valueBytes, thresholdBytes uint64, now time.Time, locale string) string {
	hostName := safeMessageText(host.Name)
	metric := localizedMetricLabel(ruleKey, locale)
	current := formatNetworkBytes(valueBytes)
	threshold := formatNetworkBytes(thresholdBytes)
	when := formatNotificationTime(now)
	switch locale {
	case "en-US":
		return fmt.Sprintf("[KPanel Cluster Alert]\nHost: %s\n%s reached %s (threshold %s)\nTime: %s", hostName, metric, current, threshold, when)
	case "zh-TW":
		return fmt.Sprintf("[KPanel 叢集警報]\n主機：%s\n%s達到 %s（閾值 %s）\n時間：%s", hostName, metric, current, threshold, when)
	default:
		return fmt.Sprintf("[KPanel 集群告警]\n主机：%s\n%s达到 %s（阈值 %s）\n时间：%s", hostName, metric, current, threshold, when)
	}
}

func availabilityAlertMessage(host cluster.Host, now time.Time, locale string, recovery bool) string {
	hostName := safeMessageText(host.Name)
	state := localizedHostStateLabel(host.State, locale)
	when := formatNotificationTime(now)
	switch locale {
	case "en-US":
		if recovery {
			return fmt.Sprintf("[KPanel Cluster Notice]\nHost: %s\nConnection recovered, current state: %s\nTime: %s", hostName, state, when)
		}
		return fmt.Sprintf("[KPanel Cluster Alert]\nHost: %s\nHost is temporarily unreachable, current state: %s\nTime: %s", hostName, state, when)
	case "zh-TW":
		if recovery {
			return fmt.Sprintf("[KPanel 叢集通知]\n主機：%s\n連線已恢復，目前狀態：%s\n時間：%s", hostName, state, when)
		}
		return fmt.Sprintf("[KPanel 叢集警報]\n主機：%s\n主機暫時失聯，目前狀態：%s\n時間：%s", hostName, state, when)
	default:
		if recovery {
			return fmt.Sprintf("[KPanel 集群通知]\n主机：%s\n连接已恢复，当前状态：%s\n时间：%s", hostName, state, when)
		}
		return fmt.Sprintf("[KPanel 集群告警]\n主机：%s\n主机暂时失联，当前状态：%s\n时间：%s", hostName, state, when)
	}
}

func sshLoginMessage(host cluster.Host, event contract.SSHLoginEvent, now time.Time, locale string) string {
	hostName := safeMessageText(host.Name)
	eventTime := formatNotificationTime(event.OccurredAt.In(now.Location()))
	sentAt := formatNotificationTime(now)
	username := safeMessageText(event.Username)
	remoteAddress := safeMessageText(event.RemoteAddress)
	method := safeMessageText(event.Method)
	switch locale {
	case "en-US":
		return fmt.Sprintf("[KPanel Cluster Notice]\nHost: %s\nSSH login: %s\nUser: %s\nSource: %s\nMethod: %s\nSent: %s", hostName, eventTime, username, remoteAddress, method, sentAt)
	case "zh-TW":
		return fmt.Sprintf("[KPanel 叢集通知]\n主機：%s\nSSH 登入：%s\n使用者：%s\n來源：%s\n方式：%s\n傳送時間：%s", hostName, eventTime, username, remoteAddress, method, sentAt)
	default:
		return fmt.Sprintf("[KPanel 集群通知]\n主机：%s\nSSH 登录：%s\n用户：%s\n来源：%s\n方式：%s\n发送时间：%s", hostName, eventTime, username, remoteAddress, method, sentAt)
	}
}

func testMessage(now time.Time, locale string) string {
	when := formatNotificationTime(now)
	switch locale {
	case "en-US":
		return fmt.Sprintf("[KPanel Cluster Notice]\nTelegram channel test succeeded.\nTime: %s", when)
	case "zh-TW":
		return fmt.Sprintf("[KPanel 叢集通知]\nTelegram 頻道測試成功。\n時間：%s", when)
	default:
		return fmt.Sprintf("[KPanel 集群通知]\nTelegram 通道测试成功。\n时间：%s", when)
	}
}

func formatMetric(value float64, unit string) string {
	if unit == "%" {
		return fmt.Sprintf("%.1f%%", value)
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}

// formatNetworkBytes mirrors the desktop widget's binary formatter: 1024
// bytes per step and one decimal place for non-byte units.
func formatNetworkBytes(value uint64) string {
	if value == 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	scaled := float64(value)
	index := 0
	for scaled >= 1024 && index < len(units)-1 {
		scaled /= 1024
		index++
	}
	if index == 0 {
		return fmt.Sprintf("%.0f B", scaled)
	}
	return fmt.Sprintf("%.1f %s", scaled, units[index])
}

func hostStateLabel(value cluster.HostState) string {
	switch value {
	case cluster.HostStale:
		return "数据过期"
	case cluster.HostOffline:
		return "离线"
	case cluster.HostAuthFailed:
		return "授权失败"
	case cluster.HostTLSFailed:
		return "TLS 错误"
	case cluster.HostIncompatible:
		return "协议不兼容"
	case cluster.HostDegraded:
		return "降级"
	case cluster.HostOnline:
		return "在线"
	default:
		return string(value)
	}
}

func localizedMetricLabel(ruleKey, locale string) string {
	labels := map[string]string{
		"cpu": "CPU 使用率", "memory": "内存使用率", "disk": "磁盘使用率",
		"traffic": "网络吞吐", cumulativeTrafficReceivedRuleKey: "累计接收", cumulativeTrafficSentRuleKey: "累计传送",
	}
	if locale == "en-US" {
		return map[string]string{
			"cpu": "CPU usage", "memory": "Memory usage", "disk": "Disk usage",
			"traffic": "Network throughput", cumulativeTrafficReceivedRuleKey: "Cumulative received", cumulativeTrafficSentRuleKey: "Cumulative sent",
		}[ruleKey]
	}
	if locale == "zh-TW" {
		return map[string]string{
			"cpu": "CPU 使用率", "memory": "記憶體使用率", "disk": "磁碟使用率",
			"traffic": "網路吞吐量", cumulativeTrafficReceivedRuleKey: "累計接收", cumulativeTrafficSentRuleKey: "累計傳送",
		}[ruleKey]
	}
	return labels[ruleKey]
}

func localizedHostStateLabel(value cluster.HostState, locale string) string {
	if locale == "en-US" {
		if label := map[cluster.HostState]string{
			cluster.HostStale: "stale data", cluster.HostOffline: "offline",
			cluster.HostAuthFailed: "authorization failed", cluster.HostTLSFailed: "TLS error",
			cluster.HostIncompatible: "protocol incompatible", cluster.HostDegraded: "degraded",
			cluster.HostOnline: "online",
		}[value]; label != "" {
			return label
		}
	}
	if locale == "zh-TW" {
		if label := map[cluster.HostState]string{
			cluster.HostStale: "資料過期", cluster.HostOffline: "離線",
			cluster.HostAuthFailed: "授權失敗", cluster.HostTLSFailed: "TLS 錯誤",
			cluster.HostIncompatible: "協定不相容", cluster.HostDegraded: "降級",
			cluster.HostOnline: "線上",
		}[value]; label != "" {
			return label
		}
	}
	return hostStateLabel(value)
}

func formatNotificationTime(value time.Time) string {
	return value.Format("2006-01-02 15:04:05") + " (" + notificationTimezone(value) + ")"
}

func notificationTimezone(value time.Time) string {
	_, offset := value.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, (offset%3600)/60)
}

func safeMessageText(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(character rune) rune {
		if character < ' ' || character == '\u007f' {
			return -1
		}
		return character
	}, value)
	characters := []rune(value)
	if len(characters) > 240 {
		return string(characters[:240])
	}
	return value
}
