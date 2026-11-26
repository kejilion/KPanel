package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

const (
	checksDistributionPath   = "/api/v1/monitoring/checks/distribution"
	checksDistributionSchema = 1
	checksDistributionTick   = 30 * time.Second
	// Re-read a synced node now and then so a list edited on the node itself
	// is brought back to the center's list.
	checksDistributionVerify   = 10 * time.Minute
	checksDistributionAttempts = 3
)

// Delays after the first and second failed attempt for one desired list.
var checksDistributionBackoff = []time.Duration{time.Minute, 5 * time.Minute}

// Node states shown to the administrator.
const (
	checksNodeSynced      = "synced"
	checksNodePending     = "pending"
	checksNodeWaiting     = "waiting"
	checksNodeUnsupported = "unsupported"
	checksNodeFailed      = "failed"
)

type checksDistributionCluster interface {
	Hosts(context.Context) cluster.HostList
	LightChecksReadiness(string) cluster.LightChecksReadiness
	LightNodeChecks(context.Context, string) (monitoring.CheckSnapshot, error)
	ReplaceLightNodeChecks(context.Context, string, monitoring.ReplaceChecksInput) (monitoring.CheckSnapshot, error)
}

type checksDistributionFile struct {
	SchemaVersion int  `json:"schemaVersion"`
	Enabled       bool `json:"enabled"`
}

type checksNodeState struct {
	state       string
	errorCode   string
	attempts    int
	nextAttempt time.Time
	verifiedAt  time.Time
	syncedAt    time.Time
	connected   bool
}

// checksDistribution keeps every light node's service checks equal to this
// center's own list. The center's list is the only source: a node's list is
// overwritten, never merged.
type checksDistribution struct {
	path    string
	cluster checksDistributionCluster
	desired func(context.Context) (monitoring.CheckSnapshot, error)
	now     func() time.Time
	kickCh  chan struct{}

	run sync.Mutex // one reconcile pass at a time

	mu             sync.Mutex
	enabled        bool
	desiredVersion string
	desiredOK      bool
	nodes          map[string]*checksNodeState
}

type ChecksDistributionNode struct {
	HostID    string     `json:"hostId"`
	Name      string     `json:"name"`
	State     string     `json:"state"`
	ErrorCode string     `json:"errorCode,omitempty"`
	SyncedAt  *time.Time `json:"syncedAt,omitempty"`
}

type ChecksDistributionStatus struct {
	Enabled                bool                     `json:"enabled"`
	CenterChecksAvailable  bool                     `json:"centerChecksAvailable"`
	DesiredResourceVersion string                   `json:"desiredResourceVersion,omitempty"`
	Nodes                  []ChecksDistributionNode `json:"nodes"`
}

type checksDistributionUpdate struct {
	Enabled bool `json:"enabled"`
}

func openChecksDistribution(path string, clusterService checksDistributionCluster, desired func(context.Context) (monitoring.CheckSnapshot, error)) *checksDistribution {
	d := &checksDistribution{
		path: path, cluster: clusterService, desired: desired, now: time.Now,
		kickCh: make(chan struct{}, 1), nodes: make(map[string]*checksNodeState),
	}
	content, err := os.ReadFile(path)
	if err == nil {
		var state checksDistributionFile
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.DisallowUnknownFields()
		var extra any
		// An unreadable file leaves distribution off: never push by surprise.
		if decoder.Decode(&state) == nil && errors.Is(decoder.Decode(&extra), io.EOF) && state.SchemaVersion == checksDistributionSchema {
			d.enabled = state.Enabled
		} else {
			slog.Warn("monitoring checks distribution setting is invalid; distribution stays off")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		slog.Warn("monitoring checks distribution setting is unreadable; distribution stays off")
	}
	return d
}

func (d *checksDistribution) Enabled() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.enabled
}

func (d *checksDistribution) SetEnabled(enabled bool) error {
	data, err := json.Marshal(checksDistributionFile{SchemaVersion: checksDistributionSchema, Enabled: enabled})
	if err != nil {
		return err
	}
	if err := writeFileAtomic(d.path, append(data, '\n'), 0o600); err != nil {
		return err
	}
	d.mu.Lock()
	d.enabled = enabled
	d.nodes = make(map[string]*checksNodeState)
	d.mu.Unlock()
	d.Kick()
	return nil
}

// Kick asks for a reconcile pass soon, for example after the center's list
// changed. It never blocks.
func (d *checksDistribution) Kick() {
	select {
	case d.kickCh <- struct{}{}:
	default:
	}
}

func (d *checksDistribution) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(checksDistributionTick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-d.kickCh:
			}
			d.Reconcile(ctx)
		}
	}()
}

// Reconcile runs one pass over every light node.
func (d *checksDistribution) Reconcile(ctx context.Context) {
	d.run.Lock()
	defer d.run.Unlock()
	if !d.Enabled() {
		return
	}
	desired, err := d.desired(ctx)
	if err != nil || !desired.Available || monitoring.ValidateCheckSnapshot(desired) != nil {
		d.mu.Lock()
		d.desiredOK = false
		d.mu.Unlock()
		return
	}
	d.mu.Lock()
	d.desiredOK = true
	if desired.ResourceVersion != d.desiredVersion {
		// A new list starts every node over, including ones that had given up.
		d.desiredVersion = desired.ResourceVersion
		d.nodes = make(map[string]*checksNodeState)
	}
	d.mu.Unlock()

	seen := make(map[string]bool)
	for _, host := range d.cluster.Hosts(ctx).Items {
		if ctx.Err() != nil || !d.Enabled() {
			return
		}
		if host.Kind != cluster.HostKindLightNode {
			continue
		}
		seen[host.ID] = true
		if d.due(host.ID, d.cluster.LightChecksReadiness(host.ID)) {
			d.syncNode(ctx, host.ID, desired)
		}
	}
	d.mu.Lock()
	for id := range d.nodes {
		if !seen[id] {
			delete(d.nodes, id)
		}
	}
	d.mu.Unlock()
}

func (d *checksDistribution) due(id string, readiness cluster.LightChecksReadiness) bool {
	now := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()
	state := d.nodes[id]
	if state == nil {
		state = &checksNodeState{state: checksNodePending}
		d.nodes[id] = state
	}
	if !readiness.Supported || !readiness.Connected {
		state.connected = false
		return false
	}
	if !state.connected {
		// A reconnected node gets a fresh set of attempts.
		state.connected = true
		state.attempts = 0
		state.nextAttempt = time.Time{}
		if state.state == checksNodeFailed {
			state.state = checksNodePending
		}
	}
	switch {
	case state.state == checksNodeFailed:
		return false
	case state.state == checksNodeSynced:
		return now.Sub(state.verifiedAt) >= checksDistributionVerify
	default:
		return !now.Before(state.nextAttempt)
	}
}

func (d *checksDistribution) syncNode(ctx context.Context, id string, desired monitoring.CheckSnapshot) {
	current, err := d.cluster.LightNodeChecks(ctx, id)
	if err == nil && !current.Available {
		err = errChecksNodeUnavailable
	}
	if err == nil && current.ResourceVersion != desired.ResourceVersion {
		var result monitoring.CheckSnapshot
		result, err = d.cluster.ReplaceLightNodeChecks(ctx, id, monitoring.ReplaceChecksInput{
			ExpectedResourceVersion: current.ResourceVersion, Items: desired.Items,
		})
		if err == nil && result.ResourceVersion != desired.ResourceVersion {
			err = errChecksNodeMismatch
		}
	}
	now := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()
	state := d.nodes[id]
	if state == nil || d.desiredVersion != desired.ResourceVersion {
		return // the list changed or the node left during this attempt
	}
	if err == nil {
		if state.state != checksNodeSynced {
			state.syncedAt = now
		}
		state.state, state.errorCode, state.attempts, state.verifiedAt = checksNodeSynced, "", 0, now
		return
	}
	state.errorCode = checksDistributionErrorCode(err)
	state.attempts++
	if errors.Is(err, cluster.ErrLightChecksRejected) || state.attempts >= checksDistributionAttempts {
		state.state = checksNodeFailed
		slog.Warn("monitoring checks distribution stopped for a light node", "hostId", id, "error", state.errorCode)
		return
	}
	state.state = checksNodePending
	state.nextAttempt = now.Add(checksDistributionBackoff[state.attempts-1])
}

var (
	errChecksNodeUnavailable = errors.New("node checks store is unavailable")
	errChecksNodeMismatch    = errors.New("node stored a different list")
)

func checksDistributionErrorCode(err error) string {
	switch {
	case errors.Is(err, cluster.ErrLightChecksRejected):
		return "rejected"
	case errors.Is(err, cluster.ErrLightChecksUnsupported):
		return "unsupported"
	case errors.Is(err, monitoring.ErrChecksConflict):
		return "conflict"
	case errors.Is(err, errChecksNodeUnavailable):
		return "node_checks_unavailable"
	case errors.Is(err, errChecksNodeMismatch):
		return "mismatch"
	case errors.Is(err, cluster.ErrNotFound):
		return "not_found"
	default:
		return "unavailable"
	}
}

// Status describes every light node. It reads live readiness so the view
// stays correct between reconcile passes.
func (d *checksDistribution) Status(ctx context.Context) ChecksDistributionStatus {
	hosts := d.cluster.Hosts(ctx).Items
	d.mu.Lock()
	defer d.mu.Unlock()
	status := ChecksDistributionStatus{
		Enabled: d.enabled, CenterChecksAvailable: d.desiredOK,
		DesiredResourceVersion: d.desiredVersion, Nodes: []ChecksDistributionNode{},
	}
	if !d.enabled {
		status.CenterChecksAvailable, status.DesiredResourceVersion = false, ""
		return status
	}
	for _, host := range hosts {
		if host.Kind != cluster.HostKindLightNode {
			continue
		}
		node := ChecksDistributionNode{HostID: host.ID, Name: host.Name, State: checksNodePending}
		readiness := d.cluster.LightChecksReadiness(host.ID)
		switch {
		case host.Platform == "unknown":
			node.State = checksNodeUnsupported
		case host.State != cluster.HostOnline && host.State != cluster.HostStale:
			node.State = checksNodeWaiting
		case !readiness.Supported:
			node.State = checksNodeUnsupported
		case !readiness.Connected:
			node.State = checksNodeWaiting
		default:
			if state := d.nodes[host.ID]; state != nil {
				node.State, node.ErrorCode = state.state, state.errorCode
				if !state.syncedAt.IsZero() && state.state == checksNodeSynced {
					syncedAt := state.syncedAt.UTC()
					node.SyncedAt = &syncedAt
				}
			}
		}
		status.Nodes = append(status.Nodes, node)
	}
	sort.SliceStable(status.Nodes, func(i, j int) bool { return status.Nodes[i].Name < status.Nodes[j].Name })
	return status
}

// localChecksSnapshot reads this center's own list from its agent.
func (s *Server) localChecksSnapshot(ctx context.Context) (monitoring.CheckSnapshot, error) {
	var snapshot monitoring.CheckSnapshot
	response, err := s.hostOps.Get(ctx, "/v1/monitoring/checks", "", newRequestID())
	if err != nil {
		return snapshot, err
	}
	if response.StatusCode != http.StatusOK {
		return snapshot, errors.New("center checks are unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&snapshot) != nil || !errors.Is(decoder.Decode(&extra), io.EOF) {
		return monitoring.CheckSnapshot{}, errors.New("center checks response is invalid")
	}
	return snapshot, nil
}

func (s *Server) handleChecksDistribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "checks_distribution_request_invalid", "Checks distribution request is invalid", "")
		return
	}
	if r.Method == http.MethodPut && !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if s.checksDistribution == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "checks_distribution_unavailable", "Checks distribution unavailable", "")
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Cache-Control", "no-store")
		s.writeJSON(w, http.StatusOK, s.checksDistribution.Status(r.Context()))
		return
	}
	if !s.checkCSRF(w, r, session) {
		return
	}
	var input checksDistributionUpdate
	if err := decodeLimitedJSON(w, r, 1<<10, &input); err != nil {
		_ = s.audit(r, session.User.ID, "monitoring.checks.distribution.update", "monitoring-checks", "distribution", "failure", nil)
		return
	}
	change := map[string]any{"enabled": input.Enabled}
	if err := s.audit(r, session.User.ID, "monitoring.checks.distribution.update", "monitoring-checks", "distribution", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.checksDistribution.SetEnabled(input.Enabled); err != nil {
		_ = s.audit(r, session.User.ID, "monitoring.checks.distribution.update", "monitoring-checks", "distribution", "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "checks_distribution_unavailable", "Checks distribution setting could not be saved", "")
		return
	}
	_ = s.audit(r, session.User.ID, "monitoring.checks.distribution.update", "monitoring-checks", "distribution", "success", change)
	w.Header().Set("Cache-Control", "no-store")
	s.writeJSON(w, http.StatusOK, s.checksDistribution.Status(r.Context()))
}
