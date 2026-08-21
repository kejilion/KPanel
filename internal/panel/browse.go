package panel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/store"
)

// There is deliberately no server-side "is this a secure context?" check on
// the browse endpoints. The constraint that makes this feature need HTTPS is
// the browser's own: it refuses to register the Service Worker that drives
// Scramjet outside a secure context. The server cannot observe that reliably
// — behind a CDN or proxy it sees plain HTTP, and X-Forwarded-Proto from an
// untrusted peer is (correctly) ignored — so a server-side gate mispredicts
// the browser and blocks deployments that in fact work. The client checks
// window.isSecureContext, which is the authoritative predicate, and reports
// the accurate reason (see web/public/browser-app/app.js).

// clusterBrowseSource implements cluster.BrowseBackend by calling this
// host's own Agent — the same egress internal/agent/browse.go already
// exposes for the local-hostID path. cluster.Service uses one of these
// (wired in NewServer below) only for the "being controlled by a remote
// federation request" role: when another Panel asks this node to perform a
// fetch on its behalf. Mirrors clusterTerminalSource's role split in
// terminal.go exactly.
type clusterBrowseSource struct {
	agent agentAPI
	// store is this host's own store, not the controller's. The LAN-egress
	// opt-in is a property of the machine that makes the outbound request, so
	// the egress side reads it locally and it is deliberately absent from
	// cluster.BrowseFetchRequest — a controlling panel must not be able to
	// switch off a paired node's private-address guard from a distance. See
	// store.AllowedHosts.BrowseAllowPrivateNetworks.
	store *store.Store
}

// browseAllowsPrivateNetworks reads the egress host's own LAN opt-in,
// tolerating a nil store so tests that build a bare Server keep the safe
// default.
func browseAllowsPrivateNetworks(storage *store.Store) bool {
	if storage == nil {
		return false
	}
	return storage.BrowseAllowPrivateNetworks()
}

// browseAgentFetchOutput mirrors internal/agent's browseFetchOutput wire
// shape, the counterpart to browseAgentFetchInput above.
type browseAgentFetchOutput struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
}

func (s clusterBrowseSource) Fetch(ctx context.Context, input cluster.BrowseFetchRequest) (cluster.BrowseFetchResult, error) {
	body, err := json.Marshal(browseAgentFetchInput{
		URL: input.URL, Method: input.Method, Headers: input.Headers, Body: input.Body,
		AllowPrivateNetwork: browseAllowsPrivateNetworks(s.store),
	})
	if err != nil {
		return cluster.BrowseFetchResult{}, err
	}
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/browse/fetch", "", newRequestID(), body)
	if err != nil {
		return cluster.BrowseFetchResult{}, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return cluster.BrowseFetchResult{}, fmt.Errorf("Agent browse fetch failed with status %d", response.StatusCode)
	}
	var decoded browseAgentFetchOutput
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return cluster.BrowseFetchResult{}, errors.New("Agent browse fetch response is invalid")
	}
	decodedBody, err := base64.StdEncoding.DecodeString(decoded.Body)
	if err != nil {
		return cluster.BrowseFetchResult{}, errors.New("Agent browse fetch response body is invalid")
	}
	return cluster.BrowseFetchResult{StatusCode: decoded.StatusCode, Headers: decoded.Headers, Body: decodedBody}, nil
}

// maxFederatedBrowseFetchIterations safety-valves the Output polling loop in
// federatedBrowseFetch: at maxFederationBrowseFetchOutputBytes (32KiB) per
// poll, draining the Agent's largest possible buffered response
// (maxBrowseFetchResponseBytes, 8MiB) takes at most 256 polls. This is a
// generous multiple of that, guarding only against a misbehaving remote that
// never reports Done — not a limit real traffic should approach.
const maxFederatedBrowseFetchIterations = 1024

// maxFederatedBrowseFetchBytes mirrors internal/agent's
// maxBrowseFetchResponseBytes (8MiB): a remote host's browse egress cannot
// buffer more than that, so a federated response claiming more is not a large
// response, it is a remote that is not behaving like our Agent.
//
// Threat model, so the checks below are not mistaken for input validation
// against the internet: reaching this code requires a host that is already
// v2-paired, already authenticated through the Noise handshake, and already
// granted browse scope. The attacker is a *compromised peer* — a node the
// operator once paired that has since been taken over — not an arbitrary
// caller. That peer controls every field of the Open/Output responses, and
// paneld runs under a 256MB container limit, so a negative TotalBytes
// (make() panics and takes down the whole panel process, not just this
// request) or an inflated one (an allocation the panel funds on the peer's
// say-so) are both reachable from there.
const maxFederatedBrowseFetchBytes = 8 << 20

var (
	errBrowseHostNotFound    = errors.New("browse host not found")
	errBrowseHostUnsupported = errors.New("browse host does not support browse egress")
)

// resolveBrowseHost defaults an empty hostID to the local Panel host, looks
// it up via the cluster service, and rejects anything without
// BrowseAvailable. The local host is always browse-available (it's this
// Panel's own trusted management of itself); a remote v2-paired host is
// browse-available only when its pairing was explicitly granted
// cluster.browse.fetch scope — light nodes and v1 hosts never are (see
// cluster.Host construction in internal/cluster). Shared by the fetch and
// session-start handlers so the two can never validate a host differently.
func (s *Server) resolveBrowseHost(ctx context.Context, hostID string) (cluster.Host, string, error) {
	if hostID == "" {
		hostID = cluster.LocalHostID
	}
	host, err := s.cluster.Host(ctx, hostID)
	if err != nil {
		return cluster.Host{}, hostID, errBrowseHostNotFound
	}
	if !host.BrowseAvailable {
		return cluster.Host{}, hostID, errBrowseHostUnsupported
	}
	return host, hostID, nil
}

func (s *Server) writeBrowseHostError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, errBrowseHostNotFound):
		s.writeProblem(w, r, http.StatusNotFound, "browse_host_not_found", "浏览出口主机不存在", "")
	case errors.Is(err, errBrowseHostUnsupported):
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "browse_host_unsupported", "该节点不支持作为浏览出口", "")
	default:
		s.writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal server error", "")
	}
}

// browseFetchRequest is the desktop "browser" feature's wire contract. hostId
// selects the egress: "local" (or omitted) routes through this Panel's own
// Agent; any other id must name an already-paired host of Kind
// HostKindPanel — a host that actually runs kejilion-agent and therefore has
// a control channel Panel can call. HostKindLightNode nodes are outbound-only
// telemetry pushers with no such channel and can never appear here.
//
// Deliberately not audited per request: this endpoint relays every
// sub-resource a browsed page loads (images, scripts, XHRs), so a per-call
// audit entry would flood the log the way a single page view of desktop
// icon/workspace changes does not. This mirrors the existing choice not to
// record full URLs in desktop-workspace audit entries (docs/desktop-icon-workspace.md
// SS4), taken one step further: no per-fetch trail at all for this feature.
// handleBrowseSessionStart below records the coarse "who browsed through
// which host" event instead.
type browseFetchRequest struct {
	HostID  string              `json:"hostId,omitempty"`
	URL     string              `json:"url"`
	Method  string              `json:"method,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"`
}

// browseAgentFetchInput mirrors internal/agent's browseFetchInput wire shape.
// Panel does not import internal/agent's types (each layer owns its own wire
// struct, same as every other s.agent.Do call in this package) — only the
// JSON field names have to match.
type browseAgentFetchInput struct {
	URL                 string              `json:"url"`
	Method              string              `json:"method,omitempty"`
	Headers             map[string][]string `json:"headers,omitempty"`
	Body                string              `json:"body,omitempty"`
	AllowPrivateNetwork bool                `json:"allowPrivateNetwork,omitempty"`
}

func (s *Server) handleBrowseFetch(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	if !s.checkBrowseOrigin(w, r) {
		return
	}
	session, ok := s.requireBrowseSession(w, r)
	if !ok || !s.checkBrowseCSRF(w, r, session) {
		return
	}

	var input browseFetchRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}

	_, hostID, err := s.resolveBrowseHost(r.Context(), input.HostID)
	if err != nil {
		s.writeBrowseHostError(w, r, err)
		return
	}
	if hostID != cluster.LocalHostID {
		result, err := s.federatedBrowseFetch(r.Context(), hostID, cluster.BrowseFetchRequest{
			URL: input.URL, Method: input.Method, Headers: input.Headers, Body: input.Body,
		})
		if err != nil {
			s.writeProblem(w, r, http.StatusBadGateway, "browse_fetch_failed", "浏览目标不可达", "")
			return
		}
		s.writeJSON(w, http.StatusOK, browseAgentFetchOutput{
			StatusCode: result.StatusCode, Headers: result.Headers,
			Body: base64.StdEncoding.EncodeToString(result.Body),
		})
		return
	}

	agentBody, err := json.Marshal(browseAgentFetchInput{
		URL: input.URL, Method: input.Method, Headers: input.Headers, Body: input.Body,
		AllowPrivateNetwork: browseAllowsPrivateNetworks(s.store),
	})
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	response, err := s.agent.Do(r.Context(), http.MethodPost, "/v1/browse/fetch", "", requestID(r), agentBody)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

// federatedBrowseFetch drives the Open/Output.../Close exchange against a
// remote v2-paired host and assembles the chunked response into one buffer,
// so callers see exactly the same shape as the local path
// (browseAgentFetchOutput) regardless of which egress served the request.
func (s *Server) federatedBrowseFetch(ctx context.Context, hostID string, input cluster.BrowseFetchRequest) (cluster.BrowseFetchResult, error) {
	return drainFederatedBrowseFetch(ctx, hostID, input, s.cluster.BrowseFetchOpen, s.cluster.BrowseFetchOutput, s.cluster.BrowseFetchClose)
}

// drainFederatedBrowseFetch is federatedBrowseFetch's orchestration split out
// as pure function-value plumbing (open/output/closeFn), so the assembly
// loop — multi-chunk reassembly, mid-loop error cleanup, the iteration
// safety valve — is unit-testable without a real *cluster.Service or the
// Noise-encrypted federation harness that already covers Open/Output/Close
// themselves in internal/cluster's own tests.
func drainFederatedBrowseFetch(
	ctx context.Context,
	hostID string,
	input cluster.BrowseFetchRequest,
	open func(context.Context, string, cluster.BrowseFetchRequest) (cluster.BrowseFetchOpenResponse, error),
	output func(context.Context, string, cluster.BrowseFetchOutputRequest) (cluster.BrowseFetchOutputResponse, error),
	closeFn func(context.Context, string, cluster.BrowseFetchCloseRequest) error,
) (cluster.BrowseFetchResult, error) {
	opened, err := open(ctx, hostID, input)
	if err != nil {
		return cluster.BrowseFetchResult{}, err
	}
	sessionID := opened.SessionID
	// Validated before make(): a negative capacity panics, and the panic is
	// not contained to this request.
	if opened.TotalBytes < 0 || opened.TotalBytes > maxFederatedBrowseFetchBytes {
		_ = closeFn(ctx, hostID, cluster.BrowseFetchCloseRequest{SessionID: sessionID})
		return cluster.BrowseFetchResult{}, fmt.Errorf(
			"federated browse fetch declared an out-of-range length (%d bytes)", opened.TotalBytes)
	}
	body := make([]byte, 0, opened.TotalBytes)
	offset := 0
	for iteration := 0; iteration < maxFederatedBrowseFetchIterations; iteration++ {
		chunk, err := output(ctx, hostID, cluster.BrowseFetchOutputRequest{SessionID: sessionID, Offset: offset})
		if err != nil {
			_ = closeFn(ctx, hostID, cluster.BrowseFetchCloseRequest{SessionID: sessionID})
			return cluster.BrowseFetchResult{}, err
		}
		decoded, err := base64.StdEncoding.DecodeString(chunk.Data)
		if err != nil {
			_ = closeFn(ctx, hostID, cluster.BrowseFetchCloseRequest{SessionID: sessionID})
			return cluster.BrowseFetchResult{}, errors.New("federated browse fetch chunk is invalid")
		}
		// The declared length is a promise, not a bound: enforce the real one
		// against what actually arrived, or a peer that under-reports
		// TotalBytes and then streams forever defeats the check above.
		if len(body)+len(decoded) > maxFederatedBrowseFetchBytes {
			_ = closeFn(ctx, hostID, cluster.BrowseFetchCloseRequest{SessionID: sessionID})
			return cluster.BrowseFetchResult{}, errors.New("federated browse fetch exceeded the maximum response size")
		}
		body = append(body, decoded...)
		// NextOffset has to advance to exactly what we have received. A
		// rewinding or skipping offset means the two sides disagree about
		// which bytes these are, and the assembled buffer would be neither
		// side's idea of the response.
		if chunk.NextOffset != len(body) {
			_ = closeFn(ctx, hostID, cluster.BrowseFetchCloseRequest{SessionID: sessionID})
			return cluster.BrowseFetchResult{}, fmt.Errorf(
				"federated browse fetch chunk offset is not contiguous (got %d, want %d)", chunk.NextOffset, len(body))
		}
		offset = chunk.NextOffset
		if chunk.Done {
			if len(body) != opened.TotalBytes {
				_ = closeFn(ctx, hostID, cluster.BrowseFetchCloseRequest{SessionID: sessionID})
				return cluster.BrowseFetchResult{}, fmt.Errorf(
					"federated browse fetch delivered %d bytes but declared %d", len(body), opened.TotalBytes)
			}
			return cluster.BrowseFetchResult{StatusCode: opened.StatusCode, Headers: opened.Headers, Body: body}, nil
		}
	}
	_ = closeFn(ctx, hostID, cluster.BrowseFetchCloseRequest{SessionID: sessionID})
	return cluster.BrowseFetchResult{}, errors.New("federated browse fetch did not complete")
}

type browseSessionStartRequest struct {
	HostID string `json:"hostId,omitempty"`
}

type browseSessionStartResponse struct {
	HostID string `json:"hostId"`
}

// handleBrowseSessionStart is the one audited moment in the browse feature:
// the frontend calls this once when an admin picks an egress and opens the
// browser window, before any handleBrowseFetch calls follow. It records who
// started browsing through which host — not what they browsed — giving the
// audit log a single "browse.session.start" entry per session instead of one
// per relayed sub-resource request.
func (s *Server) handleBrowseSessionStart(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	if !s.checkBrowseOrigin(w, r) {
		return
	}
	session, ok := s.requireBrowseSession(w, r)
	if !ok || !s.checkBrowseCSRF(w, r, session) {
		return
	}

	var input browseSessionStartRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}

	_, hostID, err := s.resolveBrowseHost(r.Context(), input.HostID)
	if err != nil {
		s.writeBrowseHostError(w, r, err)
		return
	}

	if err := s.audit(r, session.UserID, "browse.session.start", "browse-session", hostID, "success", map[string]any{
		"hostId": hostID,
	}); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	s.writeJSON(w, http.StatusCreated, browseSessionStartResponse{HostID: hostID})
}
