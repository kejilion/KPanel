package mcpaccess

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Reserve enough space for every accepted operation's maximum receipt, even
// when all operations finish at once. 64 * (96 + 96) KiB plus bounded metadata
// stays below the private store's 16 MiB plaintext ceiling.
const MaxOperations = 64
const MaxOperationArguments = 96 << 10
const MaxOperationResult = 96 << 10

type Operation struct {
	ID           string          `json:"id"`
	ClientID     string          `json:"clientId"`
	Key          string          `json:"key"`
	HostID       string          `json:"hostId"`
	HostIdentity string          `json:"hostIdentity"`
	Tool         string          `json:"tool"`
	Arguments    json.RawMessage `json:"arguments"`
	Digest       string          `json:"digest"`
	State        string          `json:"state"`
	CreatedAt    time.Time       `json:"createdAt"`
	ExpiresAt    time.Time       `json:"expiresAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
	ApprovedBy   string          `json:"approvedBy,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
	Error        string          `json:"error,omitempty"`
}

type operationState struct {
	Version int         `json:"version"`
	Items   []Operation `json:"items"`
}

type Operations struct {
	mu        sync.Mutex
	path      string
	state     operationState
	available bool
	now       func() time.Time
	write     func(string, any) error
}

func OpenOperations(dataDir string) *Operations {
	s := &Operations{path: filepath.Join(dataDir, "mcp-access", "operations.json"), state: operationState{Version: 1}, available: true, now: time.Now, write: writePrivateJSON}
	b, err := readPrivateJSON(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s
	}
	if err != nil || json.Unmarshal(b, &s.state) != nil || s.state.Version != 1 || len(s.state.Items) > MaxOperations {
		s.available = false
		return s
	}
	seen := map[string]bool{}
	changed := false
	for i := range s.state.Items {
		o := &s.state.Items[i]
		if !validHex(o.ID, 32) || !validHex(o.ClientID, 32) || !validHex(o.Digest, 64) || seen[o.ID] || !validOperationState(o.State) || len(o.Arguments) > MaxOperationArguments || len(o.Result) > MaxOperationResult || !json.Valid(o.Arguments) || operationDigest(*o) != o.Digest {
			s.available = false
			return s
		}
		seen[o.ID] = true
		// A crash between submission and durable receipt is not proof of failure.
		// Never automatically retry a host mutation after this point.
		if o.State == "executing" {
			o.State = "unknown"
			o.Error = "execution_interrupted_verify_host_state"
			o.UpdatedAt = s.now().UTC()
			changed = true
		}
	}
	if changed {
		_ = s.save(s.state)
	}
	return s
}

func validOperationState(state string) bool {
	return slices.Contains([]string{"pending", "approved", "rejected", "executing", "submitted", "succeeded", "failed", "unknown", "expired"}, state)
}

func operationDigest(o Operation) string {
	b, _ := json.Marshal([]any{o.ClientID, o.HostID, o.HostIdentity, o.Tool, o.Arguments})
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

func cloneOperation(o Operation) Operation {
	o.Arguments = slices.Clone(o.Arguments)
	o.Result = slices.Clone(o.Result)
	return o
}

func (s *Operations) save(next operationState) error {
	if err := s.write(s.path, next); err != nil {
		s.available = false
		return ErrUnavailable
	}
	s.state = next
	return nil
}

func (s *Operations) Plan(clientID, key, hostID, identity, tool string, arguments json.RawMessage, automatic bool) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return Operation{}, ErrUnavailable
	}
	if !validHex(clientID, 32) || len(key) < 8 || len(key) > 128 || hostID == "" || len(hostID) > 128 || !validHex(identity, 64) || tool == "" || len(tool) > 128 || len(arguments) > MaxOperationArguments {
		return Operation{}, ErrInvalid
	}
	// Canonical JSON ensures whitespace / object-key order cannot defeat replay
	// detection. The typed tool validator must run before persisting a plan.
	var value any
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.UseNumber()
	if !json.Valid(arguments) || decoder.Decode(&value) != nil {
		return Operation{}, ErrInvalid
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > MaxOperationArguments {
		return Operation{}, ErrInvalid
	}
	now := s.now().UTC()
	o := Operation{ClientID: clientID, Key: key, HostID: hostID, HostIdentity: identity, Tool: tool, Arguments: canonical, State: "pending", CreatedAt: now, UpdatedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
	o.Digest = operationDigest(o)
	for _, prior := range s.state.Items {
		if prior.ClientID == clientID && prior.Key == key {
			if prior.Digest != o.Digest {
				return Operation{}, ErrConflict
			}
			return cloneOperation(prior), nil
		}
	}
	next := operationState{Version: 1}
	for _, prior := range s.state.Items {
		if prior.State == "executing" || prior.State == "submitted" || now.Sub(prior.UpdatedAt) < 7*24*time.Hour {
			next.Items = append(next.Items, prior)
		}
	}
	if len(next.Items) >= MaxOperations {
		return Operation{}, ErrLimited
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Operation{}, err
	}
	o.ID = hex.EncodeToString(id[:])
	if automatic {
		o.State = "approved"
		o.ApprovedBy = "client_policy"
	}
	next.Items = append(next.Items, o)
	if err := s.save(next); err != nil {
		return Operation{}, err
	}
	return cloneOperation(o), nil
}

func (s *Operations) Get(id string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return Operation{}, ErrUnavailable
	}
	for _, o := range s.state.Items {
		if o.ID == id {
			return cloneOperation(o), nil
		}
	}
	return Operation{}, ErrInvalid
}

func (s *Operations) ByKey(clientID, key string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return Operation{}, ErrUnavailable
	}
	for _, o := range s.state.Items {
		if o.ClientID == clientID && o.Key == key {
			return cloneOperation(o), nil
		}
	}
	return Operation{}, ErrInvalid
}

func (s *Operations) List(clientID string) ([]Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return nil, ErrUnavailable
	}
	items := []Operation{}
	for _, o := range s.state.Items {
		if clientID == "" || clientID == o.ClientID {
			items = append(items, cloneOperation(o))
		}
	}
	return items, nil
}

// Decide is only exposed through the Panel's authenticated, CSRF-protected UI.
func (s *Operations) Decide(id, digest, actor string, approve bool) (Operation, error) {
	return s.change(id, func(o *Operation) error {
		if o.State != "pending" || o.Digest != digest || actor == "" {
			return ErrConflict
		}
		if !s.now().Before(o.ExpiresAt) {
			return ErrConflict
		}
		o.State = "rejected"
		if approve {
			o.State = "approved"
		}
		o.ApprovedBy = actor
		return nil
	})
}

// Claim must succeed durably exactly once before making a host request.
func (s *Operations) Claim(id, clientID, digest, identity string) (Operation, error) {
	return s.change(id, func(o *Operation) error {
		if o.State != "approved" || o.ClientID != clientID || o.Digest != digest || o.HostIdentity != identity || !s.now().Before(o.ExpiresAt) {
			return ErrConflict
		}
		o.State = "executing"
		return nil
	})
}

func (s *Operations) Finish(id, state string, result json.RawMessage, errorCode string) (Operation, error) {
	if len(result) > 0 {
		encoded, err := json.Marshal(result)
		if err != nil || len(encoded) > MaxOperationResult {
			return Operation{}, ErrInvalid
		}
		result = encoded
	}
	return s.change(id, func(o *Operation) error {
		if o.State != "executing" || !(slices.Contains([]string{"submitted", "succeeded", "failed", "unknown"}, state) || state == "approved" && o.HostID != "local") || len(result) > MaxOperationResult || (len(result) > 0 && !json.Valid(result)) || len(errorCode) > 128 {
			return ErrConflict
		}
		o.State, o.Result, o.Error = state, slices.Clone(result), errorCode
		return nil
	})
}

func (s *Operations) change(id string, update func(*Operation) error) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return Operation{}, ErrUnavailable
	}
	next := s.state
	next.Items = slices.Clone(s.state.Items)
	for i := range next.Items {
		if next.Items[i].ID != id {
			continue
		}
		o := cloneOperation(next.Items[i])
		if err := update(&o); err != nil {
			return Operation{}, err
		}
		o.UpdatedAt = s.now().UTC()
		next.Items[i] = o
		if err := s.save(next); err != nil {
			return Operation{}, err
		}
		return cloneOperation(o), nil
	}
	return Operation{}, ErrInvalid
}

func (s *Operations) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(operationState{Version: 1})
}

// Invalidate consumes unexecuted approvals when access is disabled or revoked.
// Executing/submitted records remain available for checking actual outcomes.
func (s *Operations) Invalidate(clientID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return ErrUnavailable
	}
	next := s.state
	next.Items = slices.Clone(s.state.Items)
	changed := false
	for i := range next.Items {
		o := &next.Items[i]
		if (clientID == "" || clientID == o.ClientID) && (o.State == "pending" || o.State == "approved") {
			o.State = "rejected"
			o.Error = "authorization_revoked"
			o.UpdatedAt = s.now().UTC()
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return s.save(next)
}

// Observe records an owner's verified task result. It never re-executes work.
func (s *Operations) Observe(id, state string, result json.RawMessage) (Operation, error) {
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > MaxOperationResult {
		return Operation{}, ErrInvalid
	}
	result = encoded
	return s.change(id, func(o *Operation) error {
		if (o.State != "submitted" && o.State != "unknown") || !(slices.Contains([]string{"submitted", "succeeded", "failed", "unknown"}, state) || state == "approved" && o.HostID != "local") || len(result) > MaxOperationResult || !json.Valid(result) {
			return ErrConflict
		}
		o.State = state
		o.Result = slices.Clone(result)
		o.Error = ""
		return nil
	})
}
