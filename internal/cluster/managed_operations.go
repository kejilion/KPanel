package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/flynn/noise"
	"github.com/kejilion/kejilion-panel/internal/backup"
)

// This is an explicitly versioned operation channel. Existing pairing scopes
// remain unchanged; an active pairing alone never grants host management.
const ManagedOperationsV2Path = "/api/v2/federation/managed-operations"
const MaxManagedPayload = 48 << 10

type ManagedPolicy struct {
	OperationVersions map[string]string `json:"operationVersions"`
	Write             bool              `json:"write"`
	FileRoots         []string          `json:"fileRoots,omitempty"`
}

type ManagedGrant struct {
	Revision     string        `json:"revision"`
	ControllerID string        `json:"controllerId"`
	Policy       ManagedPolicy `json:"policy"`
	CreatedAt    time.Time     `json:"createdAt"`
	ExpiresAt    time.Time     `json:"expiresAt"`
}

type managedGrantRecord struct {
	Grant         ManagedGrant `json:"grant"`
	ControllerKey string       `json:"controllerKey"`
}

type managedGrantState struct {
	Version int                  `json:"version"`
	Items   []managedGrantRecord `json:"items"`
}
type ManagedGrantSnapshot struct {
	Available       bool           `json:"available"`
	ResourceVersion string         `json:"resourceVersion"`
	Items           []ManagedGrant `json:"items"`
}

type ManagedRequest struct {
	Version     int             `json:"version"`
	Mode        string          `json:"mode"`
	ClientID    string          `json:"clientId,omitempty"`
	OperationID string          `json:"operationId,omitempty"`
	Tool        string          `json:"tool,omitempty"`
	Signature   string          `json:"signature,omitempty"`
	Arguments   json.RawMessage `json:"arguments,omitempty"`
	Offset      int             `json:"offset,omitempty"`
	Limit       int             `json:"limit,omitempty"`
}

type ManagedResponse struct {
	Version    int             `json:"version"`
	Supported  bool            `json:"supported"`
	Authorized bool            `json:"authorized"`
	Policy     *ManagedPolicy  `json:"policy,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
	ErrorCode  string          `json:"errorCode,omitempty"`
}

type ManagedOperationError struct{ Code string }

func (e *ManagedOperationError) Error() string { return e.Code }

// active must be rechecked immediately before dispatch and before returning
// data. The callback is local authority, never a boolean from the wire.
type ManagedOperationHandler func(context.Context, string, ManagedGrant, ManagedRequest, func() bool) (json.RawMessage, error)

type managedControl struct {
	mu        sync.Mutex
	path      string
	state     managedGrantState
	available bool
	handler   ManagedOperationHandler
	enabled   func() bool
}

func openManagedControl(dataDir string) *managedControl {
	c := &managedControl{path: filepath.Join(dataDir, "cluster-managed-grants.json"), state: managedGrantState{Version: 1}, available: true}
	b, err := backup.ReadFile(c.path, 4<<20)
	if errors.Is(err, os.ErrNotExist) {
		return c
	}
	if err != nil || json.Unmarshal(b, &c.state) != nil || c.state.Version != 1 || len(c.state.Items) > 128 {
		c.available = false
		return c
	}
	seen := map[string]bool{}
	for _, record := range c.state.Items {
		g := record.Grant
		if !managedHex(g.Revision, 32) || !validID(g.ControllerID) || seen[g.ControllerID] || !validManagedPolicy(g.Policy) || record.ControllerKey == "" || g.CreatedAt.IsZero() || !g.ExpiresAt.After(g.CreatedAt) || g.ExpiresAt.Sub(g.CreatedAt) > 90*24*time.Hour {
			c.available = false
			return c
		}
		seen[g.ControllerID] = true
	}
	return c
}

func validManagedPolicy(p ManagedPolicy) bool {
	if len(p.OperationVersions) == 0 || len(p.OperationVersions) > 128 || len(p.FileRoots) > 16 {
		return false
	}
	for name, signature := range p.OperationVersions {
		if len(name) == 0 || len(name) > 128 || !managedHex(signature, 64) {
			return false
		}
		for _, ch := range name {
			if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_') {
				return false
			}
		}
	}
	for _, root := range p.FileRoots {
		if len(root) > 4096 || root == "/" || !strings.HasPrefix(root, "/") || strings.ContainsAny(root, "\x00\r\n\\") || strings.Contains(root, "/../") {
			return false
		}
	}
	return true
}

func managedHex(value string, length int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(value) == length && len(decoded)*2 == length
}
func cloneManagedPolicy(p ManagedPolicy) ManagedPolicy {
	copy := p
	copy.FileRoots = slices.Clone(p.FileRoots)
	copy.OperationVersions = make(map[string]string, len(p.OperationVersions))
	for name, signature := range p.OperationVersions {
		copy.OperationVersions[name] = signature
	}
	return copy
}
func (c *managedControl) version() string {
	data, _ := json.Marshal(c.state)
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}
func (c *managedControl) save(state managedGrantState) error {
	data, err := json.Marshal(state)
	if err != nil || len(data) > 4<<20 {
		return errors.New("managed_grants_limit")
	}
	if err := backup.WriteJSON(c.path, state); err != nil {
		c.available = false
		return err
	}
	c.state = state
	return nil
}

// ResetManagedGrants is used at the offline Panel identity-restore boundary.
// Pairing identity may be restored; delegated management authority never is.
func ResetManagedGrants(dataDir string) error {
	return openManagedControl(dataDir).save(managedGrantState{Version: 1})
}

func (s *Service) SetManagedOperationHandler(handler ManagedOperationHandler, enabled ...func() bool) {
	s.managed.mu.Lock()
	defer s.managed.mu.Unlock()
	s.managed.handler = handler
	if len(enabled) > 0 {
		s.managed.enabled = enabled[0]
	}
}

func (s *Service) ManagedGrants() ManagedGrantSnapshot {
	c := s.managed
	c.mu.Lock()
	defer c.mu.Unlock()
	view := ManagedGrantSnapshot{Available: c.available, ResourceVersion: c.version(), Items: []ManagedGrant{}}
	for _, record := range c.state.Items {
		grant := record.Grant
		grant.Policy = cloneManagedPolicy(grant.Policy)
		view.Items = append(view.Items, grant)
	}
	return view
}

func (s *Service) ManagedControllers() []Controller {
	items := []Controller{}
	for _, record := range s.storeV2.Controllers() {
		if record.State == controllerStateV2Active {
			items = append(items, Controller{ID: record.ID, Name: record.Name, Fingerprint: record.Fingerprint, Scope: record.Scope, CreatedAt: record.CreatedAt})
		}
	}
	return items
}

func (s *Service) SetManagedGrant(controllerID string, policy ManagedPolicy, lifetime time.Duration, version string) error {
	controller, err := s.storeV2.Controller(controllerID)
	if err != nil || controller.State != controllerStateV2Active || !validManagedPolicy(policy) || lifetime < time.Hour || lifetime > 90*24*time.Hour {
		return ErrAuthentication
	}
	c := s.managed
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.available {
		return errors.New("managed_grants_unavailable")
	}
	if version != c.version() {
		return ErrConflict
	}
	next := managedGrantState{Version: 1}
	for _, r := range c.state.Items {
		if r.Grant.ControllerID != controllerID {
			next.Items = append(next.Items, r)
		}
	}
	if len(next.Items) >= 128 {
		return ErrRateLimited
	}
	now := s.now().UTC()
	revision, err := randomHex(16)
	if err != nil {
		return err
	}
	next.Items = append(next.Items, managedGrantRecord{Grant: ManagedGrant{Revision: revision, ControllerID: controllerID, Policy: cloneManagedPolicy(policy), CreatedAt: now, ExpiresAt: now.Add(lifetime)}, ControllerKey: controller.PublicKey})
	return c.save(next)
}

func (s *Service) RevokeManagedGrant(controllerID, version string) error {
	c := s.managed
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.available {
		return errors.New("managed_grants_unavailable")
	}
	if version != c.version() {
		return ErrConflict
	}
	next := managedGrantState{Version: 1}
	found := false
	for _, r := range c.state.Items {
		if r.Grant.ControllerID == controllerID {
			found = true
		} else {
			next.Items = append(next.Items, r)
		}
	}
	if !found {
		return ErrNotFound
	}
	return c.save(next)
}

func (s *Service) managedGrant(controllerID, key string) (ManagedGrant, bool) {
	controller, err := s.storeV2.Controller(controllerID)
	if err != nil || controller.State != controllerStateV2Active || controller.PublicKey != key {
		return ManagedGrant{}, false
	}
	c := s.managed
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.available {
		return ManagedGrant{}, false
	}
	for _, r := range c.state.Items {
		if r.Grant.ControllerID == controllerID && r.ControllerKey == key && s.now().Before(r.Grant.ExpiresAt) {
			g := r.Grant
			g.Policy = cloneManagedPolicy(g.Policy)
			return g, true
		}
	}
	return ManagedGrant{}, false
}

func validManagedRequest(r ManagedRequest) bool {
	if r.Version != 1 || r.Offset < 0 || r.Offset > 100000 || r.Limit < 0 || r.Limit > 50 || len(r.Arguments) > MaxManagedPayload {
		return false
	}
	if r.Mode == "capabilities" {
		return r.ClientID == "" && r.OperationID == "" && r.Tool == "" && len(r.Arguments) == 0
	}
	if !managedHex(r.ClientID, 32) || (r.Mode != "read" && !managedHex(r.OperationID, 32)) {
		return false
	}
	if r.Mode == "status" {
		return r.Tool == "" && len(r.Arguments) == 0
	}
	return (r.Mode == "read" || r.Mode == "execute") && len(r.Tool) > 0 && len(r.Tool) <= 128 && managedHex(r.Signature, 64) && json.Valid(r.Arguments)
}

func (s *Service) handleManagedOperationsV2(ctx context.Context, envelope v2Envelope, now time.Time) (FederationEnvelopeV2, error) {
	controller, payload, handshake, err := s.openControllerV2(ManagedOperationsV2Path, envelope, now, controllerStateV2Active)
	if err != nil {
		return FederationEnvelopeV2{}, err
	}
	var input ManagedRequest
	if decodeV2Payload(payload, &input) != nil || !validManagedRequest(input) {
		return FederationEnvelopeV2{}, ErrAuthentication
	}
	c := s.managed
	c.mu.Lock()
	handler := c.handler
	enabled := c.enabled
	c.mu.Unlock()
	grant, allowed := s.managedGrant(controller.ID, controller.PublicKey)
	allowed = allowed && (enabled == nil || enabled())
	response := ManagedResponse{Version: 1, Supported: handler != nil, Authorized: allowed}
	if input.Mode == "capabilities" {
		if allowed {
			response.Policy = &grant.Policy
		}
		return sealV2JSONResponse(envelope, handshake, response)
	}
	if handler == nil {
		response.ErrorCode = "remote_operations_unsupported"
	} else if !allowed {
		response.ErrorCode = "remote_operations_not_authorized"
	} else if input.Mode != "status" && grant.Policy.OperationVersions[input.Tool] != input.Signature {
		response.ErrorCode = "remote_operation_not_granted_or_version_changed"
	} else {
		active := func() bool {
			current, ok := s.managedGrant(controller.ID, controller.PublicKey)
			return ok && current.Revision == grant.Revision && (enabled == nil || enabled())
		}
		response.Data, err = handler(ctx, controller.ID, grant, input, active)
		if err != nil {
			response.Data = nil
			response.ErrorCode = "remote_operation_failed"
			var managedErr *ManagedOperationError
			if errors.As(err, &managedErr) && len(managedErr.Code) <= 96 {
				response.ErrorCode = managedErr.Code
			}
		}
		if len(response.Data) > MaxManagedPayload {
			response.Data = nil
			response.ErrorCode = "remote_response_too_large"
		}
		if !active() {
			response.Data = nil
			response.ErrorCode = "remote_operations_not_authorized"
			response.Authorized = false
		}
	}
	return sealV2JSONResponse(envelope, handshake, response)
}

type remoteManagedAPI interface {
	ManagedV2(context.Context, string, string, string, noise.DHKey, []byte, time.Time, ManagedRequest) (ManagedResponse, error)
}

func (c *RemoteClient) ManagedV2(ctx context.Context, origin, controllerID, targetID string, key noise.DHKey, peer []byte, now time.Time, input ManagedRequest) (ManagedResponse, error) {
	var response ManagedResponse
	if !validManagedRequest(input) {
		return response, ErrAuthentication
	}
	err := c.exchangeV2(ctx, origin, ManagedOperationsV2Path, controllerID, targetID, "", key, peer, nil, now, input, &response)
	if err == nil && (response.Version != 1 || len(response.Data) > MaxManagedPayload) {
		err = ErrProtocolMismatch
	}
	return response, err
}

func (s *Service) ManagedRemote(ctx context.Context, hostID string, input ManagedRequest) (ManagedResponse, error) {
	record, err := s.storeV2.Host(hostID)
	remote, ok := s.remoteV2.(remoteManagedAPI)
	if err != nil || record.State != hostStateV2Active || !ok {
		return ManagedResponse{}, ErrProtocolMismatch
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil {
		return ManagedResponse{}, err
	}
	return remote.ManagedV2(ctx, record.Origin, record.ControllerID, record.RemoteNodeID, noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(), input)
}
