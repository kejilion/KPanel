package panel

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/desktopworkspace"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/store"
	"github.com/kejilion/kejilion-panel/internal/version"
)

type contextKey string

const requestIDKey contextKey = "request-id"

var (
	containerIDPattern       = regexp.MustCompile(`^[a-fA-F0-9]{12,64}$`)
	composeProjectPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	resourceVersionPattern   = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	environmentBackupPattern = regexp.MustCompile(`^web_[0-9]{14}\.tar\.gz$`)
	securityEntrancePattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{4,46}[a-z0-9])$`)
)

type Server struct {
	config              Config
	auth                *auth.Service
	store               *store.Store
	agent               agentAPI
	hostOps             *HostOperationService
	trustedProxies      []*net.IPNet
	auditMu             sync.Mutex
	lastAuthAudit       map[string]time.Time
	lastGlobalAuthAudit time.Time
	cluster             *cluster.Service
	clusterShareMu      sync.Mutex
	clusterShareCache   clusterShareCacheEntry
	clusterShareRateMu  sync.Mutex
	clusterShareRates   map[string]clusterShareRateEntry
	terminalMu          sync.Mutex
	terminalSessions    map[string]panelTerminalSession
	terminalOpening     int
	terminalOpeningUser map[string]int
	downloadTicketMu    sync.Mutex
	downloadTickets     map[[32]byte]fileDownloadTicket
	ai                  *ai.Service
	aiError             string
	desktopWorkspace    *desktopworkspace.Store
}

type agentAPI interface {
	Get(context.Context, string, string, string) (AgentResponse, error)
	Do(context.Context, string, string, string, string, []byte) (AgentResponse, error)
}

type authResponse struct {
	User      auth.PublicUser `json:"user"`
	CSRFToken string          `json:"csrfToken"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

func NewServer(config Config, authService *auth.Service, storage *store.Store, agent *AgentClient) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate server config: %w", err)
	}
	if authService == nil {
		return nil, errors.New("auth service is required")
	}
	if storage == nil {
		return nil, errors.New("store is required")
	}
	if agent == nil {
		return nil, errors.New("agent client is required")
	}
	trustedProxies, err := parseTrustedProxyCIDRs(config.TrustedProxyCIDRs)
	if err != nil {
		return nil, err
	}
	clusterService, err := cluster.NewService(cluster.ServiceConfig{
		DataDir: config.DataDir, PanelVersion: version.Version,
		PublicURL:    config.PublicURL,
		PrivateCIDRs: config.ClusterPrivateCIDRs,
		Telemetry:    clusterTelemetrySource{agent: agent},
		Terminal:     clusterTerminalSource{agent: agent},
		SecurityEntrancePath: func() string {
			entrance, _ := storage.SecurityEntrance()
			if !authService.IsInitialized() || !entrance.Enabled {
				return ""
			}
			return entrance.Path
		},
	})
	if err != nil {
		return nil, fmt.Errorf("initialize cluster service: %w", err)
	}
	desktopWorkspace, err := desktopworkspace.Open(filepath.Join(config.DataDir, "desktop-workspace"))
	if err != nil {
		return nil, fmt.Errorf("initialize desktop workspace: %w", err)
	}
	server := &Server{
		config: config, auth: authService, store: storage, agent: agent,
		cluster:             clusterService,
		terminalSessions:    make(map[string]panelTerminalSession),
		terminalOpeningUser: make(map[string]int),
		trustedProxies:      trustedProxies,
		lastAuthAudit:       make(map[string]time.Time),
		desktopWorkspace:    desktopWorkspace,
	}
	server.hostOps = newHostOperationService(server)
	return server, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.setSecurityHeaders(w, r)
	requestID := newRequestID()
	w.Header().Set("X-Request-ID", requestID)
	r = r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID))
	defer func() {
		if recover() != nil {
			s.writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal server error", "")
		}
	}()

	if r.URL.Path != "/api/v1/health" &&
		!isFederationV2Request(r) &&
		!s.checkHost(w, r) {
		return
	}
	if s.handleSecurityEntrance(w, r) {
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Cache-Control", "no-store")
		s.serveAPI(w, r)
		return
	}
	s.serveSPA(w, r)
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	switch {
	case isLightNodeRequest(r):
		s.handleLightNodeFederation(w, r)
	case isFederationV2Request(r):
		s.handleFederationV2(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/federation/pair":
		s.handleFederationPair(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/federation/summary":
		s.handleFederationSummary(w, r)
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/federation/revoke":
		s.handleFederationRevoke(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/ai/") || r.URL.Path == "/api/v1/ai":
		s.handleAI(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/health":
		s.writeJSON(w, http.StatusOK, map[string]any{
			"status":          "ok",
			"version":         version.Version,
			"protocolVersion": version.ProtocolVersion,
			"initialized":     s.auth.IsInitialized(),
			"checkedAt":       time.Now().UTC(),
		})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/bootstrap":
		s.writeJSON(w, http.StatusOK, map[string]bool{"required": !s.auth.IsInitialized()})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/bootstrap":
		s.handleBootstrap(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/login":
		s.handleLogin(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/session":
		s.handleSession(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/auth/logout":
		s.handleLogout(w, r)
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/settings/password":
		s.handlePasswordChange(w, r)
	case r.Method == http.MethodPut && r.URL.Path == "/api/v1/settings/username":
		s.handleUsernameChange(w, r)
	case r.URL.Path == "/api/v1/settings/totp":
		s.handleTOTPSettings(w, r)
	case r.URL.Path == "/api/v1/settings/totp/enrollment":
		s.handleTOTPEnrollment(w, r)
	case r.URL.Path == "/api/v1/settings/totp/recovery-codes":
		s.handleTOTPRecoveryCodes(w, r)
	case (r.Method == http.MethodGet || r.Method == http.MethodPut) && r.URL.Path == "/api/v1/settings/security-entry":
		s.handleSecurityEntranceSettings(w, r)
	case r.URL.Path == "/api/v1/cluster/share":
		s.handleClusterShareSettings(w, r)
	case r.URL.Path == "/api/v1/cluster/share/token":
		s.handleClusterShareTokenReset(w, r)
	case strings.HasPrefix(r.URL.Path, clusterShareAPIPrefix):
		s.handlePublicClusterShare(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/audit":
		s.handleAudit(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/cluster/"):
		s.handleCluster(w, r)
	case r.URL.Path == "/api/v1/terminal-sessions" ||
		strings.HasPrefix(r.URL.Path, "/api/v1/terminal-sessions/"):
		s.handleTerminalSession(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/jobs":
		s.handleJobs(w, r)
	case r.URL.Path == "/api/v1/desktop/workspace":
		s.handleDesktopWorkspace(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/desktop/shortcuts/"):
		s.handleDesktopShortcutIcon(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/sites/") &&
		strings.HasSuffix(r.URL.Path, "/icon"):
		s.handleSiteIcon(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/apps/icons/"):
		s.handleAppIcon(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sites":
		s.handleSiteCreate(w, r)
	case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v1/sites/"):
		s.handleSiteUpdate(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/sites/"):
		s.handleSiteDelete(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/site-installations/") &&
		strings.HasSuffix(r.URL.Path, "/input"):
		s.handleSiteInstallationInput(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/app-jobs/") &&
		strings.HasSuffix(r.URL.Path, "/cancel"):
		s.handleAppJobCancel(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/app-jobs/") &&
		strings.HasSuffix(r.URL.Path, "/input"):
		s.handleAppJobInput(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/apps/"):
		s.handleAppAction(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/diagnostic-jobs":
		s.handleDiagnosticStart(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/web-environment/jobs":
		s.handleWebEnvironmentAction(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/web-environment/jobs/") &&
		strings.HasSuffix(r.URL.Path, "/input"):
		s.handleWebEnvironmentInput(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/web-environment/backups/"):
		s.handleWebEnvironmentBackupDownload(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/diagnostic-jobs/") &&
		strings.HasSuffix(r.URL.Path, "/input"):
		s.handleDiagnosticInput(w, r)
	case r.URL.Path == "/api/v1/files":
		s.handleFileList(w, r)
	case r.URL.Path == "/api/v1/files/entry":
		s.handleFileEntry(w, r)
	case r.URL.Path == "/api/v1/files/entries":
		s.handleFileEntries(w, r)
	case r.URL.Path == "/api/v1/files/trash":
		s.handleFileTrashList(w, r)
	case r.URL.Path == "/api/v1/files/content":
		s.handleFileContent(w, r)
	case r.URL.Path == "/api/v1/files/download-tickets":
		s.handleFileDownloadTicketCreate(w, r)
	case isFileDownloadTicketPath(r.URL.Path):
		s.handleFileDownloadTicket(w, r)
	case r.URL.Path == "/api/v1/files/upload":
		s.handleFileUpload(w, r)
	case r.URL.Path == "/api/v1/files/transfers":
		s.handleFileTransfer(w, r)
	case r.URL.Path == "/api/v1/files/actions":
		s.handleFileAction(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/docker/containers/") &&
		strings.HasSuffix(r.URL.Path, "/exec"):
		s.handleDockerExec(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/docker/containers/"):
		s.handleDockerAction(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/docker/tasks":
		s.handleDockerTask(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/actions":
		s.handleSystemAction(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/resource-actions":
		s.handleSystemResourceAction(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/traffic-shutdown/actions":
		s.handleTrafficShutdownAction(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/account-actions":
		s.handleAccountManagementAction(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/ssh-defense/actions":
		s.handleSSHDefenseAction(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/system/system-tuning/actions":
		s.handleSystemTuningAction(w, r)
	case r.Method == http.MethodGet:
		s.handleAgentProxy(w, r)
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func isFederationV2Request(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/v2/federation/pair",
		"/api/v2/federation/commit",
		"/api/v2/federation/summary",
		"/api/v2/federation/revoke",
		"/api/v2/federation/terminal/open",
		"/api/v2/federation/terminal/output",
		"/api/v2/federation/terminal/input",
		"/api/v2/federation/terminal/resize",
		"/api/v2/federation/terminal/close",
		"/api/v2/federation/files/open",
		"/api/v2/federation/files/link",
		"/api/v2/federation/files/open-linked":
		return true
	default:
		return false
	}
}

func (s *Server) handleDockerAction(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	agentPath, containerID, action, allowed := allowedDockerActionPath(r.URL.Path)
	if !allowed {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	var input struct {
		ResourceVersion string `json:"resourceVersion"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		_ = s.audit(r, session.User.ID, "docker."+action, "container", containerID, "failure", nil)
		return
	}
	if !resourceVersionPattern.MatchString(input.ResourceVersion) {
		s.writeValidationProblem(w, r, "resourceVersion", "a valid resourceVersion is required")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	change := map[string]any{"resourceVersion": input.ResourceVersion}
	if err := s.audit(r, session.User.ID, "docker."+action, "container", containerID, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.hostOps.Do(r.Context(), http.MethodPost, agentPath, "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, "docker."+action, "container", containerID, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, "docker."+action, "container", containerID, result, change)
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleDockerExec(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_docker_exec_request", "Invalid Docker exec request", "")
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	const prefix = "/api/v1/docker/containers/"
	const suffix = "/exec"
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, prefix), suffix)
	if !containerIDPattern.MatchString(id) {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	var input dockerx.ContainerExecInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		_ = s.audit(r, session.User.ID, "docker.exec", "container", id, "failure", nil)
		return
	}
	if !resourceVersionPattern.MatchString(input.ResourceVersion) ||
		strings.TrimSpace(input.Command) == "" || len(input.Command) > 2048 ||
		strings.ContainsAny(input.Command, "\r\n\x00") {
		s.writeValidationProblem(w, r, "command", "a single command up to 2048 bytes is required")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	change := map[string]any{"resourceVersion": input.ResourceVersion}
	if err := s.audit(r, session.User.ID, "docker.exec", "container", id, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.hostOps.Do(
		r.Context(),
		http.MethodPost,
		"/v1/docker/containers/"+id+"/exec",
		"",
		requestID(r),
		body,
	)
	if err != nil {
		_ = s.audit(r, session.User.ID, "docker.exec", "container", id, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	outcome := "failure"
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		outcome = "success"
	}
	_ = s.audit(r, session.User.ID, "docker.exec", "container", id, outcome, change)
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleDockerTask(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input dockerx.MaintenanceInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		_ = s.audit(r, session.User.ID, "docker.task", "docker", input.Action, "failure", nil)
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	target := input.Target
	if target == "" {
		target = input.Name
	}
	if target == "" {
		target = input.Image
	}
	response, err := s.hostOps.Do(r.Context(), http.MethodPost, "/v1/docker/tasks", "", requestID(r), body)
	if err != nil {
		_ = s.audit(r, session.User.ID, "docker."+input.Action, "docker", target, "failure", nil)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	outcome := "success"
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		outcome = "failure"
	}
	_ = s.audit(r, session.User.ID, "docker."+input.Action, "docker", target, outcome, nil)
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	var input struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		s.auditAuthFailure(r, "auth.bootstrap")
		return
	}
	credentials, err := s.auth.Bootstrap(input.Token, input.Username, input.Password)
	if err != nil {
		s.auditAuthFailure(r, "auth.bootstrap")
		switch {
		case errors.Is(err, auth.ErrBootstrapUnavailable):
			s.writeProblem(w, r, http.StatusConflict, "bootstrap_unavailable", "Bootstrap unavailable", "")
		case errors.Is(err, auth.ErrInvalidBootstrapToken):
			s.writeProblem(w, r, http.StatusUnauthorized, "invalid_bootstrap_token", "Invalid bootstrap token", "")
		case errors.Is(err, auth.ErrInvalidUsername):
			s.writeValidationProblem(w, r, "username", err.Error())
		case errors.Is(err, auth.ErrWeakPassword):
			s.writeValidationProblem(w, r, "password", err.Error())
		default:
			s.writeProblem(w, r, http.StatusInternalServerError, "bootstrap_failed", "Bootstrap failed", "")
		}
		return
	}
	s.setAuthCookies(w, r, credentials)
	_ = s.audit(r, credentials.User.ID, "auth.bootstrap", "user", credentials.User.ID, "success", nil)
	s.writeJSON(w, http.StatusCreated, authResponse{
		User: credentials.User, CSRFToken: credentials.CSRFToken, ExpiresAt: credentials.ExpiresAt,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTPCode string `json:"totpCode,omitempty"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		s.auditAuthFailure(r, "auth.login")
		return
	}
	credentials, err := s.auth.Login(s.remoteIP(r), input.Username, input.Password, input.TOTPCode)
	if err != nil {
		var rateError *auth.RateLimitError
		switch {
		case errors.As(err, &rateError):
			seconds := max(int(rateError.RetryAfter.Seconds()), 1)
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			s.writeProblem(w, r, http.StatusTooManyRequests, "login_rate_limited", "Too many login attempts", "")
		case errors.Is(err, auth.ErrInvalidCredentials):
			s.auditAuthFailure(r, "auth.login")
			s.writeProblem(w, r, http.StatusUnauthorized, "invalid_credentials", "Invalid credentials", "")
		case errors.Is(err, auth.ErrTOTPRequired):
			s.writeProblem(w, r, http.StatusUnauthorized, "totp_required", "Two-factor authentication required", "")
		case errors.Is(err, auth.ErrInvalidSecondFactor):
			s.auditAuthFailure(r, "auth.login.second_factor")
			s.writeProblem(w, r, http.StatusUnauthorized, "invalid_second_factor", "Invalid two-factor authentication code", "")
		case errors.Is(err, auth.ErrSecondFactorUnavailable):
			s.auditAuthFailure(r, "auth.login.second_factor_unavailable")
			s.writeProblem(w, r, http.StatusServiceUnavailable, "second_factor_unavailable", "Two-factor authentication is temporarily unavailable", "Use a recovery code or restore the Panel TOTP encryption key.")
		default:
			s.auditAuthFailure(r, "auth.login")
			s.writeProblem(w, r, http.StatusInternalServerError, "login_failed", "Login failed", "")
		}
		return
	}
	s.setAuthCookies(w, r, credentials)
	_ = s.audit(r, credentials.User.ID, "auth.login", "session", "", "success", nil)
	s.writeJSON(w, http.StatusOK, authResponse{
		User: credentials.User, CSRFToken: credentials.CSRFToken, ExpiresAt: credentials.ExpiresAt,
	})
}

type securityEntranceResponse struct {
	Enabled         bool   `json:"enabled"`
	Path            string `json:"path,omitempty"`
	ResourceVersion string `json:"resourceVersion"`
}

func (s *Server) handleSecurityEntranceSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut && !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method == http.MethodPut && !s.checkCSRF(w, r, session) {
		return
	}
	current, version := s.store.SecurityEntrance()
	if r.Method == http.MethodGet {
		s.writeJSON(w, http.StatusOK, securityEntranceResponse{
			Enabled: current.Enabled, Path: current.Path, ResourceVersion: version,
		})
		return
	}

	var input struct {
		Enabled                 bool   `json:"enabled"`
		Path                    string `json:"path"`
		Regenerate              bool   `json:"regenerate"`
		ExpectedResourceVersion string `json:"expectedResourceVersion"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) {
		s.writeValidationProblem(w, r, "expectedResourceVersion", "a valid resourceVersion is required")
		return
	}
	entrancePath := strings.Trim(strings.ToLower(strings.TrimSpace(input.Path)), "/")
	if input.Enabled && (input.Regenerate || entrancePath == "") {
		generated, err := newSecurityEntrancePath()
		if err != nil {
			s.writeProblem(w, r, http.StatusInternalServerError, "security_entry_generation_failed", "Security entry generation failed", "")
			return
		}
		entrancePath = generated
	}
	if input.Enabled && !securityEntrancePattern.MatchString(entrancePath) {
		s.writeValidationProblem(w, r, "path", "安全入口须为 6–48 位小写字母、数字或连字符")
		return
	}
	if !input.Enabled {
		entrancePath = ""
	}
	updated := store.SecurityEntrance{Enabled: input.Enabled, Path: entrancePath, UpdatedAt: time.Now().UTC()}
	change := map[string]any{"enabled": input.Enabled, "regenerated": input.Regenerate}
	if err := s.audit(r, session.User.ID, "settings.security_entry.update", "panel", "security-entry", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.store.ReplaceSecurityEntrance(input.ExpectedResourceVersion, updated); err != nil {
		_ = s.audit(r, session.User.ID, "settings.security_entry.update", "panel", "security-entry", "failure", change)
		if errors.Is(err, store.ErrConflict) {
			s.writeProblem(w, r, http.StatusConflict, "resource_version_changed", "Security entry changed; refresh and retry", "")
			return
		}
		s.writeProblem(w, r, http.StatusInternalServerError, "security_entry_update_failed", "Security entry update failed", "")
		return
	}
	_ = s.audit(r, session.User.ID, "settings.security_entry.update", "panel", "security-entry", "success", change)
	value, newVersion := s.store.SecurityEntrance()
	s.writeJSON(w, http.StatusOK, securityEntranceResponse{
		Enabled: value.Enabled, Path: value.Path, ResourceVersion: newVersion,
	})
}

func newSecurityEntrancePath() (string, error) {
	buffer := make([]byte, 10)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "panel-" + hex.EncodeToString(buffer), nil
}

func (s *Server) handleSecurityEntrance(w http.ResponseWriter, r *http.Request) bool {
	value, _ := s.store.SecurityEntrance()
	if !value.Enabled || !s.auth.IsInitialized() {
		return false
	}
	if r.Method == http.MethodGet && r.URL.RawQuery == "" && r.URL.Path == "/"+value.Path {
		expires := time.Now().Add(s.config.SessionTTL)
		http.SetCookie(w, &http.Cookie{
			Name: s.securityEntranceCookieName(), Value: securityEntranceToken(value), Path: "/",
			MaxAge: max(int(s.config.SessionTTL.Seconds()), 1), Expires: expires, HttpOnly: true,
			Secure: s.requestUsesHTTPS(r), SameSite: http.SameSiteStrictMode,
		})
		// A redirect in the original cross-site navigation chain does not carry a
		// freshly issued SameSite=Strict cookie in current browsers. Complete one
		// target-origin document load first, then let a static meta refresh enter
		// the login route without weakening the cookie policy.
		const body = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta http-equiv="refresh" content="0;url=/login"><meta name="referrer" content="no-referrer"><title>KPanel</title></head><body><p><a href="/login">Continue to KPanel</a></p></body></html>`
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, body)
		return true
	}
	if securityEntrancePublicPath(r.URL.Path) || s.hasSecurityEntranceCookie(r, value) || s.hasValidSession(r) {
		return false
	}
	http.NotFound(w, r)
	return true
}

func securityEntrancePublicPath(requestPath string) bool {
	return requestPath == "/api/v1/health" || isFileDownloadTicketPath(requestPath) || isStaticAssetPath(requestPath) || isClusterSharePagePath(requestPath) || isPublicClusterShareAPIPath(requestPath) || strings.HasPrefix(requestPath, "/api/v2/federation/") || strings.HasPrefix(requestPath, "/api/v3/federation/light/")
}

func isStaticAssetPath(requestPath string) bool {
	if strings.HasPrefix(requestPath, "/assets/") {
		return true
	}
	switch requestPath {
	case "/favicon.ico", "/favicon.svg", "/apple-touch-icon.png", "/manifest.webmanifest", "/robots.txt":
		return true
	default:
		return false
	}
}

func (s *Server) hasValidSession(r *http.Request) bool {
	cookie, err := r.Cookie(s.config.CookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	_, err = s.auth.Authenticate(cookie.Value)
	return err == nil
}

func (s *Server) hasSecurityEntranceCookie(r *http.Request, entrance store.SecurityEntrance) bool {
	cookie, err := r.Cookie(s.securityEntranceCookieName())
	return err == nil && secureStringEqual(cookie.Value, securityEntranceToken(entrance))
}

func securityEntranceToken(entrance store.SecurityEntrance) string {
	digest := sha256.Sum256([]byte("kpanel-security-entry\x00" + entrance.Path + "\x00" + entrance.UpdatedAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(digest[:])
}

func (s *Server) securityEntranceCookieName() string {
	if s.config.SecureCookie {
		return "__Host-kejilion_entry"
	}
	return "kejilion_entry"
}

func (s *Server) auditAuthFailure(r *http.Request, action string) {
	now := time.Now().UTC()
	key := action + "\x00" + s.remoteIP(r)
	s.auditMu.Lock()
	if now.Sub(s.lastGlobalAuthAudit) < 5*time.Second || now.Sub(s.lastAuthAudit[key]) < time.Minute {
		s.auditMu.Unlock()
		return
	}
	s.lastGlobalAuthAudit = now
	s.lastAuthAudit[key] = now
	if len(s.lastAuthAudit) > 4096 {
		cutoff := now.Add(-time.Hour)
		for item, seenAt := range s.lastAuthAudit {
			if seenAt.Before(cutoff) {
				delete(s.lastAuthAudit, item)
			}
		}
		for item := range s.lastAuthAudit {
			if len(s.lastAuthAudit) <= 4096 {
				break
			}
			delete(s.lastAuthAudit, item)
		}
	}
	s.auditMu.Unlock()
	_ = s.audit(r, "", action, "authentication", "", "failure", nil)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	sessionToken, session, ok := s.requireSession(w, r)
	_ = sessionToken
	if !ok {
		return
	}
	csrfToken := s.csrfCookieValue(r)
	if err := s.auth.ValidateCSRF(session, csrfToken); err != nil {
		// A stale/missing readable CSRF cookie does not invalidate the session,
		// but the caller must log in again before any write operation.
		csrfToken = ""
	}
	s.writeJSON(w, http.StatusOK, authResponse{
		User: session.User, CSRFToken: csrfToken, ExpiresAt: session.ExpiresAt,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	sessionToken, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !s.checkCSRF(w, r, session) {
		return
	}
	if err := s.auth.Logout(sessionToken); err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "logout_failed", "Logout failed", "")
		return
	}
	s.clearAuthCookies(w, r)
	_ = s.audit(r, session.User.ID, "auth.logout", "session", "", "success", nil)
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handlePasswordChange(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}

	const action = "auth.password.change"
	if err := s.audit(r, session.User.ID, action, "user", session.User.ID, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.auth.ChangePassword(session.User.ID, input.CurrentPassword, input.NewPassword); err != nil {
		_ = s.audit(r, session.User.ID, action, "user", session.User.ID, "failure", nil)
		var rateError *auth.RateLimitError
		switch {
		case errors.As(err, &rateError):
			seconds := max(int(rateError.RetryAfter.Seconds()), 1)
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			s.writeProblem(w, r, http.StatusTooManyRequests, "password_change_rate_limited", "Password change rate limited", "")
		case errors.Is(err, auth.ErrInvalidCurrentPassword):
			s.writeValidationProblem(w, r, "currentPassword", err.Error())
		case errors.Is(err, auth.ErrWeakPassword), errors.Is(err, auth.ErrPasswordUnchanged):
			s.writeValidationProblem(w, r, "newPassword", err.Error())
		default:
			s.writeProblem(w, r, http.StatusInternalServerError, "password_change_failed", "Password change failed", "")
		}
		return
	}

	s.clearAuthCookies(w, r)
	_ = s.audit(r, session.User.ID, action, "user", session.User.ID, "success", nil)
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleUsernameChange(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewUsername     string `json:"newUsername"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}

	const action = "auth.username.change"
	if err := s.audit(r, session.User.ID, action, "user", session.User.ID, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.auth.ChangeUsername(session.User.ID, input.CurrentPassword, input.NewUsername); err != nil {
		_ = s.audit(r, session.User.ID, action, "user", session.User.ID, "failure", nil)
		var rateError *auth.RateLimitError
		switch {
		case errors.As(err, &rateError):
			seconds := max(int(rateError.RetryAfter.Seconds()), 1)
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			s.writeProblem(w, r, http.StatusTooManyRequests, "username_change_rate_limited", "Username change rate limited", "")
		case errors.Is(err, auth.ErrInvalidCurrentPassword):
			s.writeValidationProblem(w, r, "currentPassword", err.Error())
		case errors.Is(err, auth.ErrInvalidUsername), errors.Is(err, auth.ErrUsernameUnchanged):
			s.writeValidationProblem(w, r, "newUsername", err.Error())
		case errors.Is(err, auth.ErrUsernameUnavailable):
			s.writeProblem(w, r, http.StatusConflict, "username_unavailable", "Username is already in use", "")
		default:
			s.writeProblem(w, r, http.StatusInternalServerError, "username_change_failed", "Username change failed", "")
		}
		return
	}

	s.clearAuthCookies(w, r)
	_ = s.audit(r, session.User.ID, action, "user", session.User.ID, "success", nil)
	s.writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	_, _, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			s.writeValidationProblem(w, r, "limit", "limit must be between 1 and 200")
			return
		}
		limit = parsed
	}
	events, next := s.store.ListAudit(limit, r.URL.Query().Get("cursor"))
	result := contract.PageResult[contract.AuditEvent]{
		Items: make([]contract.AuditEvent, 0, len(events)),
	}
	result.NextCursor = next
	for _, event := range events {
		result.Items = append(result.Items, contract.AuditEvent{
			ID: event.ID, OccurredAt: event.OccurredAt, ActorType: event.ActorType,
			ActorID: event.ActorID, SourceIP: event.SourceIP, Action: event.Action,
			TargetKind: event.TargetKind, TargetID: event.TargetID, Result: event.Result,
			RequestID: event.RequestID, Change: event.Change,
		})
	}
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAgentProxy(w http.ResponseWriter, r *http.Request) {
	_, _, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if isSystemResourcePublicPath(r.URL.Path) && (r.URL.RawPath != "" || r.URL.RawQuery != "") {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	agentPath, allowed := allowedAgentPath(r.URL.Path)
	if !allowed {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	response, err := s.hostOps.Get(r.Context(), agentPath, r.URL.RawQuery, requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) writeAgentResponse(w http.ResponseWriter, r *http.Request, response AgentResponse) {
	w.Header().Set("Content-Type", response.ContentType)
	if len(response.Body) >= 1024 &&
		r.Method != http.MethodHead &&
		r.Header.Get("Range") == "" &&
		acceptsGzip(r.Header.Get("Accept-Encoding")) &&
		isCompressibleContentType(response.ContentType) {
		var compressed bytes.Buffer
		writer, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
		if err == nil {
			_, writeErr := writer.Write(response.Body)
			closeErr := writer.Close()
			if writeErr == nil && closeErr == nil && compressed.Len() < len(response.Body) {
				addVary(w.Header(), "Accept-Encoding")
				w.Header().Set("Content-Encoding", "gzip")
				response.Body = compressed.Bytes()
			}
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(response.Body)
}

func allowedDockerActionPath(publicPath string) (agentPath, containerID, action string, allowed bool) {
	const prefix = "/api/v1/docker/containers/"
	rest := strings.TrimPrefix(publicPath, prefix)
	if rest == publicPath {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || !containerIDPattern.MatchString(parts[0]) {
		return "", "", "", false
	}
	switch parts[1] {
	case "start", "stop", "restart", "pause", "unpause", "remove":
		return "/v1/docker/containers/" + parts[0] + "/" + parts[1], parts[0], parts[1], true
	default:
		return "", "", "", false
	}
}

func allowedAgentPath(publicPath string) (string, bool) {
	exact := map[string]string{
		"/api/v1/agent/health":              "/v1/health",
		"/api/v1/capabilities":              "/v1/capabilities",
		"/api/v1/system/summary":            "/v1/system/summary",
		"/api/v1/system/public-network":     "/v1/system/public-network",
		"/api/v1/system/processes":          "/v1/system/processes",
		"/api/v1/system/hosts":              "/v1/system/hosts",
		"/api/v1/system/cron":               "/v1/system/cron",
		"/api/v1/system/network-interfaces": "/v1/system/network-interfaces",
		"/api/v1/system/firewall":           "/v1/system/firewall",
		"/api/v1/system/port-usage":         "/v1/system/port-usage",
		"/api/v1/system/traffic-shutdown":   "/v1/system/traffic-shutdown",
		"/api/v1/system/accounts":           "/v1/system/accounts",
		"/api/v1/system/ssh-defense":        "/v1/system/ssh-defense",
		"/api/v1/system/system-tuning":      "/v1/system/system-tuning",
		"/api/v1/monitoring/history":        "/v1/monitoring/history",
		"/api/v1/sites":                     "/v1/sites",
		"/api/v1/site-installations":        "/v1/site-installations",
		"/api/v1/web-environment":           "/v1/web-environment",
		"/api/v1/web-environment/catalog":   "/v1/web-environment/catalog",
		"/api/v1/web-environment/backups":   "/v1/web-environment/backups",
		"/api/v1/web-environment/jobs":      "/v1/web-environment/jobs",
		"/api/v1/apps":                      "/v1/apps",
		"/api/v1/app-jobs":                  "/v1/app-jobs",
		"/api/v1/diagnostics":               "/v1/diagnostics",
		"/api/v1/diagnostic-jobs":           "/v1/diagnostic-jobs",
		"/api/v1/docker/summary":            "/v1/docker/summary",
		"/api/v1/docker/environment":        "/v1/docker/environment",
		"/api/v1/docker/containers":         "/v1/docker/containers",
		"/api/v1/docker/compose-projects":   "/v1/docker/compose-projects",
		"/api/v1/docker/images":             "/v1/docker/images",
		"/api/v1/docker/networks":           "/v1/docker/networks",
		"/api/v1/docker/volumes":            "/v1/docker/volumes",
		"/api/v1/docker/backups":            "/v1/docker/backups",
		"/api/v1/docker/jobs":               "/v1/docker/jobs",
	}
	if path, ok := exact[publicPath]; ok {
		return path, true
	}
	const sitePrefix = "/api/v1/sites/"
	if strings.HasPrefix(publicPath, sitePrefix) {
		rest := strings.TrimPrefix(publicPath, sitePrefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && siteIDPattern.MatchString(parts[0]) && parts[1] == "appearance" {
			return "/v1/sites/" + parts[0] + "/appearance", true
		}
	}
	const appJobPrefix = "/api/v1/app-jobs/"
	if strings.HasPrefix(publicPath, appJobPrefix) {
		rest := strings.TrimPrefix(publicPath, appJobPrefix)
		if strings.HasSuffix(rest, "/terminal") {
			id := strings.TrimSuffix(rest, "/terminal")
			if siteIDPattern.MatchString(id) {
				return "/v1/app-jobs/" + id + "/terminal", true
			}
		}
		id := rest
		if siteIDPattern.MatchString(id) {
			return "/v1/app-jobs/" + id, true
		}
	}
	const appPrefix = "/api/v1/apps/"
	if strings.HasPrefix(publicPath, appPrefix) {
		rest := strings.TrimPrefix(publicPath, appPrefix)
		parts := strings.Split(rest, "/")
		if len(parts) == 2 && appIDPattern.MatchString(parts[0]) &&
			parts[1] == "install-port" {
			return "/v1/apps/" + parts[0] + "/install-port", true
		}
	}
	const environmentJobPrefix = "/api/v1/web-environment/jobs/"
	if strings.HasPrefix(publicPath, environmentJobPrefix) {
		rest := strings.TrimPrefix(publicPath, environmentJobPrefix)
		if strings.HasSuffix(rest, "/terminal") {
			id := strings.TrimSuffix(rest, "/terminal")
			if siteIDPattern.MatchString(id) {
				return "/v1/web-environment/jobs/" + id + "/terminal", true
			}
		}
		if strings.HasSuffix(rest, "/input") {
			id := strings.TrimSuffix(rest, "/input")
			if siteIDPattern.MatchString(id) {
				return "/v1/web-environment/jobs/" + id + "/input", true
			}
		}
		if siteIDPattern.MatchString(rest) {
			return "/v1/web-environment/jobs/" + rest, true
		}
	}
	const environmentBackupPrefix = "/api/v1/web-environment/backups/"
	if strings.HasPrefix(publicPath, environmentBackupPrefix) {
		id := strings.TrimPrefix(publicPath, environmentBackupPrefix)
		if environmentBackupPattern.MatchString(id) {
			return "/v1/web-environment/backups/" + url.PathEscape(id), true
		}
	}
	const diagnosticJobPrefix = "/api/v1/diagnostic-jobs/"
	if strings.HasPrefix(publicPath, diagnosticJobPrefix) {
		rest := strings.TrimPrefix(publicPath, diagnosticJobPrefix)
		if strings.HasSuffix(rest, "/terminal") {
			id := strings.TrimSuffix(rest, "/terminal")
			if siteIDPattern.MatchString(id) {
				return "/v1/diagnostic-jobs/" + id + "/terminal", true
			}
		}
		id := rest
		if siteIDPattern.MatchString(id) {
			return "/v1/diagnostic-jobs/" + id, true
		}
	}
	const dockerJobPrefix = "/api/v1/docker/jobs/"
	if strings.HasPrefix(publicPath, dockerJobPrefix) {
		id := strings.TrimPrefix(publicPath, dockerJobPrefix)
		if siteIDPattern.MatchString(id) {
			return "/v1/docker/jobs/" + id, true
		}
	}
	const composeProjectPrefix = "/api/v1/docker/compose-projects/"
	if strings.HasPrefix(publicPath, composeProjectPrefix) {
		name := strings.TrimPrefix(publicPath, composeProjectPrefix)
		if composeProjectPattern.MatchString(name) {
			return "/v1/docker/compose-projects/" + url.PathEscape(name), true
		}
	}
	const installationPrefix = "/api/v1/site-installations/"
	if strings.HasPrefix(publicPath, installationPrefix) {
		rest := strings.TrimPrefix(publicPath, installationPrefix)
		if strings.HasSuffix(rest, "/terminal") {
			id := strings.TrimSuffix(rest, "/terminal")
			if siteIDPattern.MatchString(id) {
				return "/v1/site-installations/" + id + "/terminal", true
			}
		}
		id := rest
		if siteIDPattern.MatchString(id) {
			return "/v1/site-installations/" + id, true
		}
	}
	const prefix = "/api/v1/docker/containers/"
	for _, suffix := range []string{"/logs", "/stats"} {
		if strings.HasPrefix(publicPath, prefix) && strings.HasSuffix(publicPath, suffix) {
			id := strings.TrimSuffix(strings.TrimPrefix(publicPath, prefix), suffix)
			if containerIDPattern.MatchString(id) {
				return "/v1/docker/containers/" + url.PathEscape(id) + suffix, true
			}
		}
	}
	return "", false
}

func (s *Server) requireSession(w http.ResponseWriter, r *http.Request) (string, auth.Session, bool) {
	cookie, err := r.Cookie(s.config.CookieName)
	if err != nil || cookie.Value == "" {
		s.writeProblem(w, r, http.StatusUnauthorized, "authentication_required", "Authentication required", "")
		return "", auth.Session{}, false
	}
	session, err := s.auth.Authenticate(cookie.Value)
	if err != nil {
		s.clearAuthCookies(w, r)
		s.writeProblem(w, r, http.StatusUnauthorized, "session_expired", "Session expired", "")
		return "", auth.Session{}, false
	}
	return cookie.Value, session, true
}

func (s *Server) checkCSRF(w http.ResponseWriter, r *http.Request, session auth.Session) bool {
	headerToken := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	cookieToken := s.csrfCookieValue(r)
	if !secureStringEqual(headerToken, cookieToken) || s.auth.ValidateCSRF(session, headerToken) != nil {
		s.writeProblem(w, r, http.StatusForbidden, "csrf_validation_failed", "CSRF validation failed", "")
		return false
	}
	return true
}

func (s *Server) checkOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
	if origin == "" {
		if s.config.PublicURL != "" {
			s.writeProblem(w, r, http.StatusForbidden, "origin_validation_failed", "Origin validation failed", "")
			return false
		}
		if fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
			s.writeProblem(w, r, http.StatusForbidden, "origin_validation_failed", "Origin validation failed", "")
			return false
		}
		return true
	}
	expected, trustedProxyOrigin := s.trustedProxyHTTPSOrigin(r)
	if !trustedProxyOrigin {
		expected = s.config.PublicURL
		if s.config.AllowIPHosts {
			if ipOrigin, ok := directIPOrigin(r); ok {
				expected = ipOrigin
			}
		}
	}
	if expected == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		expected = scheme + "://" + r.Host
	}
	if !secureStringEqual(strings.ToLower(origin), strings.ToLower(expected)) {
		s.writeProblem(w, r, http.StatusForbidden, "origin_validation_failed", "Origin validation failed", "")
		return false
	}
	return true
}

func (s *Server) checkHost(w http.ResponseWriter, r *http.Request) bool {
	if s.config.PublicURL == "" {
		return true
	}
	publicURL, err := url.Parse(s.config.PublicURL)
	hostMatchesPublicURL := err == nil &&
		secureStringEqual(strings.ToLower(strings.TrimSpace(r.Host)), strings.ToLower(publicURL.Host))
	_, trustedProxyOrigin := s.trustedProxyHTTPSOrigin(r)
	_, allowedIPHost := directIPOrigin(r)
	if !hostMatchesPublicURL && !trustedProxyOrigin &&
		!(s.config.AllowIPHosts && allowedIPHost) {
		s.writeProblem(w, r, http.StatusMisdirectedRequest, "host_validation_failed", "Host validation failed", "")
		return false
	}
	return true
}

// directIPOrigin returns an origin only for a literal IPv4 or IPv6 Host. It
// supports servers reached through LAN addresses or NAT without accepting
// arbitrary DNS names, which would weaken the fixed PublicURL Host boundary.
func directIPOrigin(r *http.Request) (string, bool) {
	host := strings.TrimSpace(r.Host)
	if host == "" || strings.ContainsAny(host, "/\\?#@,;%") {
		return "", false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	origin := scheme + "://" + host
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != scheme || parsed.Host != host ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || net.ParseIP(parsed.Hostname()) == nil {
		return "", false
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", false
		}
	}
	return origin, true
}

// trustedProxyHTTPSOrigin returns the browser-facing origin only when the
// immediate peer is explicitly trusted and has forwarded a single HTTPS
// scheme. This lets kejilion.sh's k fd proxy a direct-port installation
// without allowing public clients to spoof Host or X-Forwarded-Proto.
func (s *Server) trustedProxyHTTPSOrigin(r *http.Request) (string, bool) {
	if !s.isTrustedProxyPeer(r) ||
		!secureStringEqual(strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))), "https") {
		return "", false
	}
	host := strings.TrimSpace(r.Host)
	if host == "" || strings.ContainsAny(host, "/\\?#@,;%") {
		return "", false
	}
	origin := "https://" + host
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host != host || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return origin, true
}

// requestHTTPSOrigin returns the externally reachable HTTPS origin only when
// it is authenticated by direct TLS or by a trusted reverse proxy.
func (s *Server) requestHTTPSOrigin(r *http.Request) (string, bool) {
	if origin, ok := s.trustedProxyHTTPSOrigin(r); ok {
		return origin, true
	}
	if r.TLS == nil {
		return "", false
	}
	host := strings.TrimSpace(r.Host)
	if host == "" || strings.ContainsAny(host, "/\\?#@,;%") {
		return "", false
	}
	origin := "https://" + host
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host != host || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return origin, true
}

func (s *Server) lightEnrollmentOrigin(r *http.Request) (string, bool) {
	if origin, ok := s.requestHTTPSOrigin(r); ok {
		return origin, true
	}
	parsed, err := url.Parse(strings.TrimSpace(s.config.PublicURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.ForceQuery {
		return "", false
	}
	return strings.TrimRight(s.config.PublicURL, "/"), true
}

func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "json_required", "JSON request body required", "")
		return errors.New("invalid content type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.config.MaxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "Request body too large", "")
			return err
		}
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid JSON", "")
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "Request body too large", "")
			return err
		}
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_json", "Invalid JSON", "")
		return errors.New("multiple JSON values")
	}
	return nil
}

func (s *Server) setAuthCookies(w http.ResponseWriter, r *http.Request, credentials auth.Credentials) {
	maxAge := max(int(time.Until(credentials.ExpiresAt).Seconds()), 1)
	secure := s.requestUsesHTTPS(r)
	http.SetCookie(w, &http.Cookie{
		Name: s.config.CookieName, Value: credentials.Token, Path: "/",
		MaxAge: maxAge, Expires: credentials.ExpiresAt, HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteStrictMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: s.csrfCookieName(), Value: credentials.CSRFToken, Path: "/",
		MaxAge: maxAge, Expires: credentials.ExpiresAt, HttpOnly: false,
		Secure: secure, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	secure := s.requestUsesHTTPS(r)
	for _, cookie := range []*http.Cookie{
		{Name: s.config.CookieName, HttpOnly: true},
		{Name: s.csrfCookieName(), HttpOnly: false},
	} {
		cookie.Value = ""
		cookie.Path = "/"
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
		cookie.Secure = secure
		cookie.SameSite = http.SameSiteStrictMode
		http.SetCookie(w, cookie)
	}
}

func (s *Server) requestUsesHTTPS(r *http.Request) bool {
	if s.config.SecureCookie || r.TLS != nil {
		return true
	}
	_, trustedProxyOrigin := s.trustedProxyHTTPSOrigin(r)
	return trustedProxyOrigin
}

func (s *Server) csrfCookieName() string {
	if s.config.SecureCookie {
		return "__Host-kejilion_csrf"
	}
	return "kejilion_csrf"
}

func (s *Server) csrfCookieValue(r *http.Request) string {
	cookie, err := r.Cookie(s.csrfCookieName())
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (s *Server) audit(r *http.Request, actorID, action, targetKind, targetID, result string, change map[string]any) error {
	return s.store.AppendAudit(store.AuditEvent{
		ID: newRequestID(), OccurredAt: time.Now().UTC(), ActorType: actorType(actorID),
		ActorID: actorID, SourceIP: s.remoteIP(r), Action: action, TargetKind: targetKind,
		TargetID: targetID, Result: result, RequestID: requestID(r), Change: change,
	}, 10_000)
}

func actorType(actorID string) string {
	if actorID == "" {
		return "anonymous"
	}
	return "user"
}

func (s *Server) writeValidationProblem(w http.ResponseWriter, r *http.Request, field, detail string) {
	problem := contract.Problem{
		Type: "about:blank", Title: "Validation failed", Status: http.StatusUnprocessableEntity,
		Code: "validation_failed", RequestID: requestID(r), Retryable: false,
		FieldErrors: map[string]string{field: detail},
	}
	writeProblemJSON(w, problem)
}

func (s *Server) writeProblem(w http.ResponseWriter, r *http.Request, status int, code, title, detail string) {
	writeProblemJSON(w, contract.Problem{
		Type: "about:blank", Title: title, Status: status, Code: code,
		Detail: detail, RequestID: requestID(r), Retryable: status >= 500,
	})
}

func writeProblemJSON(w http.ResponseWriter, problem contract.Problem) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) setSecurityHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; frame-src 'self' blob:; form-action 'self'; object-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if r.TLS != nil {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	} else if _, trustedProxyHTTPS := s.trustedProxyHTTPSOrigin(r); trustedProxyHTTPS {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000")
	}
}

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	root, err := filepath.Abs(s.config.WebRoot)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	requestPath := filepath.FromSlash(strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), "/"))
	candidate := filepath.Join(root, requestPath)
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	regular, err := staticRegularFile(root, candidate)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if regular {
		s.serveStaticFile(w, r, root, candidate, requestPath)
		return
	}
	index := filepath.Join(root, "index.html")
	regular, err = staticRegularFile(root, index)
	if err != nil || !regular {
		http.NotFound(w, r)
		return
	}
	s.serveStaticFile(w, r, root, index, "index.html")
}

func (s *Server) serveStaticFile(
	w http.ResponseWriter,
	r *http.Request,
	root string,
	candidate string,
	requestPath string,
) {
	if filepath.Base(candidate) == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(filepath.ToSlash(requestPath), "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	if r.Header.Get("Range") == "" && acceptsGzip(r.Header.Get("Accept-Encoding")) {
		compressed := candidate + ".gz"
		if regular, err := staticRegularFile(root, compressed); err == nil && regular {
			if contentType := mime.TypeByExtension(filepath.Ext(candidate)); contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			addVary(w.Header(), "Accept-Encoding")
			w.Header().Set("Content-Encoding", "gzip")
			http.ServeFile(w, r, compressed)
			return
		}
	}
	http.ServeFile(w, r, candidate)
}

func acceptsGzip(header string) bool {
	var wildcard bool
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		encoding := strings.ToLower(strings.TrimSpace(fields[0]))
		quality := 1.0
		for _, parameter := range fields[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && strings.EqualFold(strings.TrimSpace(key), "q") {
				parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				if err != nil {
					quality = 0
				} else {
					quality = parsed
				}
			}
		}
		switch encoding {
		case "gzip":
			return quality > 0
		case "*":
			wildcard = quality > 0
		}
	}
	return wildcard
}

func isCompressibleContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return strings.HasPrefix(mediaType, "text/") ||
		mediaType == "application/json" ||
		mediaType == "application/problem+json" ||
		mediaType == "application/javascript" ||
		mediaType == "image/svg+xml" ||
		strings.HasSuffix(mediaType, "+json")
}

func addVary(header http.Header, value string) {
	for _, existing := range header.Values("Vary") {
		for _, candidate := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(candidate), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}

func staticRegularFile(root, candidate string) (bool, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	if !samePath(root, resolvedRoot) {
		return false, errors.New("web root contains a symbolic link")
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return false, errors.New("static path escapes web root")
	}

	current := root
	parts := []string{}
	if relative != "." {
		parts = strings.Split(relative, string(filepath.Separator))
	}
	for index := -1; index < len(parts); index++ {
		if index >= 0 {
			current = filepath.Join(current, parts[index])
		}
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				return false, nil
			}
			return false, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false, errors.New("static path contains a symbolic link")
		}
		if index < len(parts)-1 && !info.IsDir() {
			return false, errors.New("static path component is not a directory")
		}
		if index == len(parts)-1 {
			return info.Mode().IsRegular(), nil
		}
	}
	return false, nil
}

func parseTrustedProxyCIDRs(values []string) ([]*net.IPNet, error) {
	result := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("parse trusted proxy CIDR %q: %w", value, err)
		}
		result = append(result, network)
	}
	return result, nil
}

func (s *Server) remoteIP(r *http.Request) string {
	peer, host := requestPeerIP(r)
	if peer != nil && ipInNetworks(peer, s.trustedProxies) {
		if forwarded := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); forwarded != nil {
			return forwarded.String()
		}
		var rightmost net.IP
		values := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for index := len(values) - 1; index >= 0; index-- {
			forwarded := net.ParseIP(strings.TrimSpace(values[index]))
			if forwarded == nil {
				continue
			}
			if rightmost == nil {
				rightmost = forwarded
			}
			if !ipInNetworks(forwarded, s.trustedProxies) {
				return forwarded.String()
			}
		}
		if rightmost != nil {
			return rightmost.String()
		}
	}
	if peer != nil {
		return peer.String()
	}
	return strings.TrimSpace(host)
}

func (s *Server) isTrustedProxyPeer(r *http.Request) bool {
	peer, _ := requestPeerIP(r)
	return peer != nil && ipInNetworks(peer, s.trustedProxies)
}

func requestPeerIP(r *http.Request) (net.IP, string) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.TrimSpace(host)
	return net.ParseIP(host), host
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func requestID(r *http.Request) string {
	value, _ := r.Context().Value(requestIDKey).(string)
	return value
}

func newRequestID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value)
}

func secureStringEqual(left, right string) bool {
	leftSum := sha256.Sum256([]byte(left))
	rightSum := sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftSum[:], rightSum[:]) == 1
}
