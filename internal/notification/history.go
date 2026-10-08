package notification

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	historyFileName  = "notification-history.json"
	MaxHistoryEvents = 2000
	MaxHistoryBytes  = 4 << 20
	HistoryRetention = 30 * 24 * time.Hour
)

// Event is an occurrence, independent of whether an external channel delivered it.
type Event struct {
	ID             string     `json:"id"`
	CreatedAt      time.Time  `json:"createdAt"`
	HostID         string     `json:"hostId"`
	HostName       string     `json:"hostName"`
	IsLocal        bool       `json:"isLocal"`
	Rule           string     `json:"rule"`
	Kind           string     `json:"kind"`
	Message        string     `json:"message"`
	RelatedEventID string     `json:"relatedEventId,omitempty"`
	Delivery       string     `json:"delivery"`
	Provider       Provider   `json:"provider,omitempty"`
	Attempts       int        `json:"attempts"`
	LastAttemptAt  *time.Time `json:"lastAttemptAt,omitempty"`
	LastErrorCode  string     `json:"lastErrorCode,omitempty"`
}

type storedEvent struct {
	Event
	TrafficThresholdGiB int    `json:"trafficThresholdGiB,omitempty"`
	TrafficCycle        string `json:"trafficCycle,omitempty"`
	// Binds delayed delivery to the expiry date that generated this reminder.
	ExpiryDate string `json:"expiryDate,omitempty"`
	// Never included in an API response. Bind retries to the original channel.
	ChannelFingerprint string `json:"channelFingerprint,omitempty"`
}

type historyState struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Sequence      uint64                `json:"sequence"`
	Rules         Rules                 `json:"rules"`
	Alerts        map[string]alertState `json:"alerts"`
	Events        []storedEvent         `json:"events"`
}

type historyStore struct {
	mu         sync.RWMutex
	path       string
	state      historyState
	err        error
	loadFailed bool
}

func openHistory(directory string, legacy persistedState) *historyStore {
	h := &historyStore{path: filepath.Join(directory, historyFileName)}
	h.state = historyState{SchemaVersion: 1, Rules: legacy.Settings.Rules, Alerts: clonePersistedState(legacy).AlertStates, Events: []storedEvent{}}
	content, err := readRegularFile(h.path, MaxHistoryBytes, false)
	if errors.Is(err, os.ErrNotExist) {
		h.err = h.commit(h.state)
		return h
	}
	if err == nil {
		var state historyState
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&state)
		if err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = errors.New("multiple history values")
			}
		}
		if err == nil {
			err = validateHistory(state)
		}
		if err == nil {
			err = os.Chmod(h.path, 0o600)
		}
		if err == nil {
			h.state = state
		}
	}
	h.err = err
	h.loadFailed = err != nil
	return h
}

func validateHistory(state historyState) error {
	if state.SchemaVersion != 1 || len(state.Events) > MaxHistoryEvents {
		return errors.New("invalid notification history")
	}
	if err := state.Rules.Validate(); err != nil {
		return err
	}
	if err := validateAlertStates(state.Alerts); err != nil {
		return err
	}
	var previous uint64
	for _, event := range state.Events {
		if event.TrafficThresholdGiB < 0 || event.TrafficThresholdGiB > MaxTrafficTotalThresholdGiB {
			return errors.New("invalid traffic threshold")
		}
		if !validTrafficCycle(event.TrafficCycle) {
			return errors.New("invalid traffic cycle")
		}
		if (event.Rule == serverExpiryRuleKey && !validExpiryDate(event.ExpiryDate)) ||
			(event.Rule != serverExpiryRuleKey && event.ExpiryDate != "") {
			return errors.New("invalid notification expiry event")
		}
		id, err := strconv.ParseUint(event.ID, 10, 64)
		if err != nil || id <= previous || id > state.Sequence || event.CreatedAt.IsZero() ||
			!validDisplayText(event.HostID, 256) || !validDisplayText(event.HostName, 1024) ||
			!validEventRule(event.Rule) || !validEventKind(event.Kind) || !validDelivery(event.Delivery) ||
			len(event.Message) > 4096 || event.Attempts < 0 || event.Attempts > 300 ||
			(event.Provider != "" && !validProvider(event.Provider)) || len(event.LastErrorCode) > 80 ||
			(event.ChannelFingerprint != "" && !validHexString(event.ChannelFingerprint, 64)) {
			return errors.New("invalid notification history event")
		}
		previous = id
	}
	return nil
}

func validEventRule(value string) bool {
	switch value {
	case "cpu", "memory", "disk", "traffic", cumulativeTrafficReceivedRuleKey, cumulativeTrafficSentRuleKey, "availability", "ssh", serverExpiryRuleKey, panelLoginRuleKey, backupRuleKey:
		return true
	}
	return false
}
func validEventKind(value string) bool {
	return value == "alert" || value == "recovery" || value == "info"
}
func validDelivery(value string) bool {
	switch value {
	case "local_only", "pending", "sent", "failed", "cancelled":
		return true
	}
	return false
}

func cloneHistory(state historyState) historyState {
	state.Alerts = clonePersistedState(persistedState{AlertStates: state.Alerts}).AlertStates
	state.Events = append([]storedEvent{}, state.Events...)
	return state
}

func (h *historyStore) snapshot() (historyState, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return cloneHistory(h.state), h.err
}

// Events and evaluator cursors are replaced together. A failed write cannot
// advance the evaluator or authorize external delivery of an unrecorded event.
func (h *historyStore) commit(state historyState) error {
	if err := validateHistory(state); err != nil {
		return err
	}
	content, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(content) > MaxHistoryBytes {
		remaining, remove := len(content), 0
		for remaining > MaxHistoryBytes && remove < len(state.Events) {
			encoded, err := json.Marshal(state.Events[remove])
			if err != nil {
				return err
			}
			remaining -= len(encoded) + 1
			remove++
		}
		state.Events = state.Events[remove:]
		content, err = json.Marshal(state)
		if err != nil {
			return err
		}
	}
	if len(content) > MaxHistoryBytes {
		return errors.New("notification history exceeds limit")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := atomicWrite(h.path, content, 0o600); err != nil {
		h.err = err
		return err
	}
	h.state = cloneHistory(state)
	h.err = nil
	return nil
}

func pruneHistory(state *historyState, now time.Time) {
	cutoff := now.Add(-HistoryRetention)
	kept := state.Events[:0]
	for _, event := range state.Events {
		if !event.CreatedAt.Before(cutoff) {
			kept = append(kept, event)
		}
	}
	if len(kept) > MaxHistoryEvents {
		kept = kept[len(kept)-MaxHistoryEvents:]
	}
	state.Events = kept
}

type HistoryQuery struct {
	HostID   string
	Rule     string
	Kind     string
	Delivery string
	Search   string
	Since    time.Time
	Until    time.Time
	Before   uint64
	Limit    int
}

type HistoryHost struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsLocal bool   `json:"isLocal"`
}
type HistoryPage struct {
	Items         []Event       `json:"items"`
	Hosts         []HistoryHost `json:"hosts"`
	NextCursor    string        `json:"nextCursor,omitempty"`
	RetentionDays int           `json:"retentionDays"`
	MaxEvents     int           `json:"maxEvents"`
	MaxBytes      int           `json:"maxBytes"`
}

func (q HistoryQuery) Validate() error {
	if (q.HostID != "" && !validDisplayText(q.HostID, 256)) ||
		(q.Rule != "" && !validEventRule(q.Rule)) || (q.Kind != "" && !validEventKind(q.Kind)) ||
		(q.Delivery != "" && !validDelivery(q.Delivery)) || !utf8.ValidString(q.Search) || len(q.Search) > 800 ||
		// Match the browser input's maxlength, which counts UTF-16 code units.
		len(utf16.Encode([]rune(q.Search))) > 200 ||
		(!q.Until.IsZero() && !q.Since.IsZero() && q.Since.After(q.Until)) || q.Limit < 0 || q.Limit > 100 {
		return &ValidationError{Field: "filters", Message: "通知记录筛选条件无效"}
	}
	return nil
}

func (s *Service) History(query HistoryQuery) (HistoryPage, error) {
	if err := query.Validate(); err != nil {
		return HistoryPage{}, err
	}
	state, err := s.history.snapshot()
	if err != nil {
		return HistoryPage{}, &Error{Code: "history_store_unavailable", Cause: err}
	}
	if query.Limit == 0 {
		query.Limit = 50
	}
	page := HistoryPage{Items: []Event{}, Hosts: []HistoryHost{}, RetentionDays: 30, MaxEvents: MaxHistoryEvents, MaxBytes: MaxHistoryBytes}
	seenHosts := map[string]bool{}
	search := strings.ToLower(strings.TrimSpace(query.Search))
	cutoff := s.now().Add(-HistoryRetention)
	for i := len(state.Events) - 1; i >= 0; i-- {
		event := state.Events[i].Event
		if event.CreatedAt.Before(cutoff) {
			continue
		}
		if !seenHosts[event.HostID] {
			page.Hosts = append(page.Hosts, HistoryHost{event.HostID, event.HostName, event.IsLocal})
			seenHosts[event.HostID] = true
		}
		if page.NextCursor != "" {
			continue
		}
		id, _ := strconv.ParseUint(event.ID, 10, 64)
		if (query.Before > 0 && id >= query.Before) || (query.HostID != "" && event.HostID != query.HostID) ||
			(query.Rule != "" && event.Rule != query.Rule) || (query.Kind != "" && event.Kind != query.Kind) ||
			(query.Delivery != "" && event.Delivery != query.Delivery) ||
			(!query.Since.IsZero() && event.CreatedAt.Before(query.Since)) || (!query.Until.IsZero() && event.CreatedAt.After(query.Until)) ||
			(search != "" && !strings.Contains(strings.ToLower(event.HostName+" "+event.Message), search)) {
			continue
		}
		if len(page.Items) == query.Limit {
			page.NextCursor = page.Items[len(page.Items)-1].ID
			continue
		}
		page.Items = append(page.Items, event)
	}
	return page, nil
}

func historyError(err error) error { return fmt.Errorf("notification history unavailable: %w", err) }
