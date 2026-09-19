package panel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpSettingsPath = "/api/v1/settings/mcp"
const mcpMaxRequest = 128 << 10
const mcpMaxResponse = 128 << 10

type mcpContextKey struct{}
type mcpRequest struct {
	principal        mcpaccess.Principal
	request          *http.Request
	authorize        func() bool
	controllerID     string
	credentialActive func() bool
}
type mcpService struct {
	access       *mcpaccess.Store
	gate         chan struct{}
	once         sync.Once
	handler      http.Handler
	operations   *mcpaccess.Operations
	serversMu    sync.Mutex
	servers      map[string]*mcp.Server
	workerMu     sync.Mutex
	workerClosed bool
	workerCtx    context.Context
	workerCancel context.CancelFunc
	workers      sync.WaitGroup
	workerSlots  chan struct{}
	oauth        *mcpaccess.OAuth
	oauthMu      sync.Mutex
	oauthMinute  int64
	oauthCounts  [3]int
}

func newMCPService(dataDir string) *mcpService {
	ctx, cancel := context.WithCancel(context.Background())
	return &mcpService{access: mcpaccess.Open(dataDir), oauth: mcpaccess.OpenOAuth(dataDir), operations: mcpaccess.OpenOperations(dataDir), gate: make(chan struct{}, 4), servers: make(map[string]*mcp.Server), workerCtx: ctx, workerCancel: cancel, workerSlots: make(chan struct{}, 2)}
}

func (m *mcpService) close() {
	m.workerMu.Lock()
	m.workerClosed = true
	m.workerCancel()
	m.workerMu.Unlock()
	m.workers.Wait()
}

// Host identity is an authorization binding, never returned to the MCP client.
// CreatedAt also prevents a recreated legacy/light host from inheriting a grant.
func (s *Server) mcpHostIdentity(h cluster.Host) string {
	v := s.cluster.NodeID() + "\x00" + h.ID + "\x00" + string(h.Kind) + "\x00" + h.RemoteNodeID + "\x00" + h.PeerFingerprint
	if !h.IsLocal {
		v += "\x00" + h.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	d := sha256.Sum256([]byte(v))
	return hex.EncodeToString(d[:])
}

func (s *Server) mcpAllowed(p mcpaccess.Principal, h cluster.Host) bool {
	if !s.mcp.access.Active(p) {
		return false
	}
	for _, grant := range p.Hosts {
		if grant.ID == h.ID && grant.Identity == s.mcpHostIdentity(h) {
			return true
		}
	}
	return false
}

// SecureCookie is a cookie setting, not evidence that this request used TLS.
func (s *Server) mcpTransportAllowed(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if _, ok := s.trustedProxyHTTPSOrigin(r); ok {
		return true
	}
	peer, _ := requestPeerIP(r)
	u, err := url.Parse("http://" + r.Host)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || peer == nil || !peer.IsLoopback() {
		return false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return false
	}
	for key := range r.Header {
		if strings.EqualFold(key, "Forwarded") || strings.HasPrefix(strings.ToLower(key), "x-forwarded-") || strings.EqualFold(key, "X-Real-IP") {
			return false
		}
	}
	return true
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.mcp == nil || !s.auth.IsInitialized() || !s.mcp.access.Enabled() {
		http.NotFound(w, r)
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		http.Error(w, "invalid_mcp_request", http.StatusBadRequest)
		return
	}
	if !s.mcpTransportAllowed(r) {
		http.Error(w, "mcp_https_required", http.StatusForbidden)
		return
	}
	if origins, present := r.Header["Origin"]; present {
		if len(origins) != 1 || strings.TrimSpace(origins[0]) == "" {
			http.Error(w, "invalid_origin", http.StatusForbidden)
			return
		}
		if !s.checkOrigin(w, r) {
			return
		}
	}
	if len(r.Header.Values("Authorization")) != 1 {
		s.mcpOAuthChallenge(w, r)
		http.Error(w, "mcp_authentication_required", http.StatusUnauthorized)
		return
	}
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") {
		s.mcpOAuthChallenge(w, r)
		http.Error(w, "mcp_authentication_required", http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	principal, release, err := s.mcp.access.Begin(token)
	var credentialActive func() bool
	if strings.HasPrefix(token, "kpo_") {
		resource := s.mcpOAuthOrigin(r) + "/mcp"
		principal, release, err = s.mcp.oauth.Begin(s.mcp.access, token, resource)
		credentialActive = func() bool { return s.mcp.oauth.Active(s.mcp.access, token, resource, principal) }
	}
	if err != nil {
		status := http.StatusUnauthorized
		s.mcpOAuthChallenge(w, r)
		if errors.Is(err, mcpaccess.ErrLimited) {
			status = http.StatusTooManyRequests
			w.Header().Set("Retry-After", "60")
		}
		http.Error(w, err.Error(), status)
		return
	}
	defer release()
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method_not_allowed", http.StatusMethodNotAllowed)
		return
	}
	select {
	case s.mcp.gate <- struct{}{}:
		defer func() { <-s.mcp.gate }()
	default:
		w.Header().Set("Retry-After", "1")
		http.Error(w, "mcp_busy", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	// Also bound socket reads: a context alone cannot interrupt a stalled body.
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Second))
	defer http.NewResponseController(w).SetReadDeadline(time.Time{})
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, mcpMaxRequest))
	if err != nil {
		http.Error(w, "mcp_request_too_large_or_unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	// Older protocol versions permit batches. Enforce one dispatch per budgeted
	// HTTP request before the SDK can fan a batch out into concurrent tool calls.
	if !bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) {
		http.Error(w, "mcp_single_request_required", http.StatusBadRequest)
		return
	}
	r = r.WithContext(ctx)
	r = r.WithContext(context.WithValue(ctx, mcpContextKey{}, mcpRequest{principal: principal, request: r, credentialActive: credentialActive}))
	r.Body = io.NopCloser(bytes.NewReader(body))
	s.mcp.once.Do(func() {
		s.mcp.handler = mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
			p := r.Context().Value(mcpContextKey{}).(mcpRequest).principal
			s.mcp.serversMu.Lock()
			defer s.mcp.serversMu.Unlock()
			if server := s.mcp.servers[p.ID]; server != nil {
				return server
			}
			local := false
			for _, h := range p.Hosts {
				if h.ID == cluster.LocalHostID {
					local = true
				}
			}
			if len(s.mcp.servers) >= mcpaccess.MaxClients {
				clear(s.mcp.servers)
			}
			server := s.newMCPServer(local, p.Policy)
			s.mcp.servers[p.ID] = server
			return server
		}, &mcp.StreamableHTTPOptions{
			Stateless: true, JSONResponse: true,
			// Panel validates Host, Origin and TLS/trusted proxies above. The SDK's
			// loopback-only Host rule otherwise rejects legitimate HTTPS proxies
			// connected to a Panel listening on 127.0.0.1.
			DisableLocalhostProtection: true,
		})
	})
	buffer := &mcpResponseBuffer{header: make(http.Header)}
	s.mcp.handler.ServeHTTP(buffer, r)
	// A revoked/expired token must not receive data from an in-flight read.
	if !s.mcp.access.Active(principal) || (credentialActive != nil && !credentialActive()) {
		s.mcpOAuthChallenge(w, r)
		http.Error(w, "mcp_authentication_required", http.StatusUnauthorized)
		return
	}
	if ctx.Err() != nil {
		http.Error(w, "mcp_timeout", http.StatusGatewayTimeout)
		return
	}
	if buffer.overflow {
		http.Error(w, "mcp_response_too_large", http.StatusBadGateway)
		return
	}
	for k, values := range buffer.header {
		w.Header()[k] = values
	}
	w.Header().Set("Cache-Control", "no-store")
	status := buffer.status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(buffer.body.Bytes())
}

type mcpResponseBuffer struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (b *mcpResponseBuffer) Header() http.Header { return b.header }
func (b *mcpResponseBuffer) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}
func (b *mcpResponseBuffer) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	if b.body.Len()+len(p) > mcpMaxResponse {
		b.overflow = true
		return 0, errors.New("mcp_response_too_large")
	}
	return b.body.Write(p)
}

func (s *Server) handleMCPSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && !s.checkCSRF(w, r, session) {
		return
	}
	if s.mcp == nil {
		s.writeProblem(w, r, 503, "mcp_access_unavailable", "MCP unavailable", "")
		return
	}
	if strings.HasPrefix(r.URL.Path, mcpSettingsPath+"/oauth/") {
		s.handleMCPOAuthConsent(w, r, session.User.ID)
		return
	}
	if strings.HasPrefix(r.URL.Path, mcpSettingsPath+"/operations") {
		s.handleMCPOperations(w, r, session.User.ID)
		return
	}
	if strings.HasPrefix(r.URL.Path, mcpSettingsPath+"/cluster-grants") {
		s.handleMCPClusterGrants(w, r, session.User.ID)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == mcpSettingsPath {
		s.writeJSON(w, 200, s.mcpSettingsView(r))
		return
	}
	var input struct {
		ExpectedResourceVersion string   `json:"expectedResourceVersion"`
		Enabled                 bool     `json:"enabled"`
		Name                    string   `json:"name"`
		HostIDs                 []string `json:"hostIds"`
		ExpiresInDays           int      `json:"expiresInDays"`
		Domains                 []string `json:"domains"`
		Write                   bool     `json:"write"`
		AutoApprove             bool     `json:"autoApprove"`
		FileRoots               []string `json:"fileRoots"`
	}
	action, target := "", "mcp"
	switch {
	case r.Method == http.MethodPut && r.URL.Path == mcpSettingsPath:
		action = "mcp.settings.update"
	case r.Method == http.MethodPost && r.URL.Path == mcpSettingsPath+"/clients":
		action = "mcp.client.create"
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, mcpSettingsPath+"/clients/"):
		target = strings.TrimPrefix(r.URL.Path, mcpSettingsPath+"/clients/")
		if !mcpClientID(target) {
			http.NotFound(w, r)
			return
		}
		action = "mcp.client.revoke"
	default:
		http.NotFound(w, r)
		return
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) {
		s.writeValidationProblem(w, r, "expectedResourceVersion", "a valid resourceVersion is required")
		return
	}
	var grants []mcpaccess.HostGrant
	var policy *mcpaccess.Policy
	if action == "mcp.client.create" {
		if len(input.Domains) > 0 {
			var err error
			policy, err = managedPolicy(input.Domains, input.Write, input.AutoApprove, input.FileRoots)
			if err != nil {
				s.mcpSettingsError(w, r, mcpaccess.ErrInvalid)
				return
			}
		} else if input.Write || input.AutoApprove || len(input.FileRoots) > 0 {
			s.mcpSettingsError(w, r, mcpaccess.ErrInvalid)
			return
		}
		if !s.mcpTransportAllowed(r) {
			s.writeProblem(w, r, 403, "mcp_https_required", "Use HTTPS or a loopback connection", "")
			return
		}
		if len(input.HostIDs) == 0 || len(input.HostIDs) > mcpaccess.MaxHosts || input.ExpiresInDays < 1 || input.ExpiresInDays > 90 {
			s.mcpSettingsError(w, r, mcpaccess.ErrInvalid)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		for _, id := range input.HostIDs {
			h, err := s.cluster.Host(ctx, id)
			if err != nil {
				s.mcpSettingsError(w, r, mcpaccess.ErrInvalid)
				return
			}
			grants = append(grants, mcpaccess.HostGrant{ID: h.ID, Identity: s.mcpHostIdentity(h)})
		}
	}
	if err := s.audit(r, session.User.ID, action, "mcp_client", target, "intent", nil); err != nil {
		s.writeProblem(w, r, 503, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	var err error
	var token string
	var client mcpaccess.Client
	switch action {
	case "mcp.settings.update":
		_, err = s.mcp.access.SetEnabled(input.Enabled, input.ExpectedResourceVersion)
		if err == nil && !input.Enabled {
			err = s.mcp.operations.Invalidate("")
		}
	case "mcp.client.create":
		client, token, err = s.mcp.access.CreateWithPolicy(strings.TrimSpace(input.Name), grants, policy, time.Duration(input.ExpiresInDays)*24*time.Hour, input.ExpectedResourceVersion)
		if err == nil {
			target = client.ID
		}
	case "mcp.client.revoke":
		_, err = s.mcp.access.Revoke(target, input.ExpectedResourceVersion)
		if err == nil {
			err = s.mcp.operations.Invalidate(target)
		}
	}
	if err != nil {
		_ = s.audit(r, session.User.ID, action, "mcp_client", target, "failure", nil)
		s.mcpSettingsError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, action, "mcp_client", target, "success", nil)
	if token != "" {
		s.writeJSON(w, 201, map[string]any{"client": client, "token": token, "settings": s.mcpSettingsView(r)})
		return
	}
	s.writeJSON(w, 200, s.mcpSettingsView(r))
}

func mcpClientID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && len(id) == 32
}
func (s *Server) mcpSettingsError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := 503, "mcp_access_unavailable"
	if errors.Is(err, mcpaccess.ErrConflict) {
		status, code = 409, "resource_version_changed"
	}
	if errors.Is(err, mcpaccess.ErrInvalid) {
		status, code = 422, "invalid_mcp_access"
	}
	s.writeProblem(w, r, status, code, "MCP access update failed", "")
}

func (s *Server) mcpSettingsView(r *http.Request) map[string]any {
	endpoint := ""
	if origin, ok := s.requestHTTPSOrigin(r); ok {
		endpoint = origin + "/mcp"
	} else if s.mcpTransportAllowed(r) {
		endpoint = "http://" + r.Host + "/mcp"
	}
	return map[string]any{"access": s.mcp.access.Snapshot(), "endpoint": endpoint, "transportReady": endpoint != "", "maxClients": mcpaccess.MaxClients, "permission": "inspect"}
}
