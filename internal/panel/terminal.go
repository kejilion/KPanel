package panel

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/desktopworkspace"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	maxPanelTerminals       = 16
	maxPanelTerminalsByUser = 4
	panelTerminalIdleTTL    = 35 * time.Minute
	panelTerminalLaunchTTL  = 5 * time.Minute
)

type clusterTerminalSource struct {
	agent agentAPI
	// Request-scoped only; never cached across Agent restarts or opens.
	capabilities *cluster.TerminalCapabilities
}

type panelTerminalSession struct {
	ID               string
	BackendSessionID string
	HostID           string
	UserID           string
	Owner            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CloseRequested   bool
	Detached         bool
	Detachable       bool
	WorkspaceVisible bool
	ReliableInput    bool
	InputClaimed     bool
	InputConnection  *terminalInputConnection
	Retained         bool
	ShortcutID       string
	Purpose          string
	Title            string
	LaunchIDs        []string
	LaunchUncertain  bool
}

type terminalOpenRequest struct {
	LookupOnly      bool   `json:"lookupOnly,omitempty"`
	PrepareOnly     bool   `json:"prepareOnly,omitempty"`
	LaunchID        string `json:"launchId,omitempty"`
	Retained        bool   `json:"retained,omitempty"`
	ShortcutID      string `json:"shortcutId,omitempty"`
	Purpose         string `json:"purpose,omitempty"`
	ShortcutVersion string `json:"shortcutVersion,omitempty"`
	HostID          string `json:"hostId"`
	Rows            uint16 `json:"rows"`
	Columns         uint16 `json:"columns"`
	ContainerID     string `json:"containerId,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

// Launch intents are issued before any PTY request. Removing an intent fences
// delayed opens without retaining cancellation tombstones indefinitely.
type panelTerminalLaunchIntent struct {
	UserID, ShortcutID, HostID string
	Purpose                    string
	ExpiresAt                  time.Time
}

func (intent panelTerminalLaunchIntent) matches(userID string, input terminalOpenRequest) bool {
	return intent.UserID == userID && intent.ShortcutID == input.ShortcutID && intent.Purpose == input.Purpose && intent.HostID == input.HostID
}

type terminalOpenResponse struct {
	Retained         bool       `json:"retained,omitempty"`
	Detachable       bool       `json:"detachable,omitempty"`
	WorkspaceVisible bool       `json:"workspaceVisible,omitempty"`
	ShortcutID       string     `json:"shortcutId,omitempty"`
	Purpose          string     `json:"purpose,omitempty"`
	Title            string     `json:"title,omitempty"`
	CloseRequested   bool       `json:"closeRequested,omitempty"`
	LaunchUncertain  bool       `json:"launchUncertain,omitempty"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	SessionID        string     `json:"sessionId"`
	HostID           string     `json:"hostId"`
	Offset           int64      `json:"offset"`
	CreatedAt        time.Time  `json:"createdAt"`
}

func (s clusterTerminalSource) Open(ctx context.Context, owner string, rows, columns uint16) (terminal.Snapshot, error) {
	body, _ := json.Marshal(map[string]any{"owner": owner, "rows": rows, "columns": columns})
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals", "", newRequestID(), body)
	var result terminal.Snapshot
	return result, decodeTerminalAgentResponse(response, err, &result)
}

func (s clusterTerminalSource) OpenRetained(ctx context.Context, owner string, rows, columns uint16) (terminal.Snapshot, error) {
	supported, err := s.SupportsRetainedTerminals(ctx)
	if err != nil {
		return terminal.Snapshot{}, err
	}
	if !supported {
		return terminal.Snapshot{}, cluster.ErrTerminalRetentionUnavailable
	}
	body, _ := json.Marshal(map[string]any{"owner": owner, "rows": rows, "columns": columns, "retained": true})
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals", "", newRequestID(), body)
	var result terminal.Snapshot
	if err := decodeTerminalAgentResponse(response, err, &result); err != nil {
		return result, err
	}
	if !result.Retained {
		if result.ID != "" {
			_ = s.Close(ctx, owner, result.ID)
		}
		return terminal.Snapshot{}, cluster.ErrTerminalRetentionUnavailable
	}
	return result, nil
}

func (s clusterTerminalSource) OpenContainer(ctx context.Context, owner string, input terminalOpenRequest) (terminal.Snapshot, error) {
	body, _ := json.Marshal(map[string]any{"owner": owner, "rows": input.Rows, "columns": input.Columns, "containerId": input.ContainerID, "resourceVersion": input.ResourceVersion})
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals", "", newRequestID(), body)
	var result terminal.Snapshot
	return result, decodeTerminalAgentResponse(response, err, &result)
}

func (s clusterTerminalSource) Output(ctx context.Context, owner, id string, offset int64, wait time.Duration) (terminal.Output, error) {
	query := url.Values{}
	query.Set("owner", owner)
	query.Set("offset", strconv.FormatInt(offset, 10))
	query.Set("wait", strconv.Itoa(int(wait/time.Millisecond)))
	response, err := s.agent.Get(ctx, "/v1/terminals/"+url.PathEscape(id)+"/output", query.Encode(), newRequestID())
	var result terminal.Output
	return result, decodeTerminalAgentResponse(response, err, &result)
}

func (s clusterTerminalSource) Input(ctx context.Context, owner, id string, data []byte) error {
	body, _ := json.Marshal(map[string]string{"owner": owner, "data": base64.RawStdEncoding.EncodeToString(data)})
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals/"+url.PathEscape(id)+"/input", "", newRequestID(), body)
	return decodeTerminalAgentResponse(response, err, nil)
}

func (s clusterTerminalSource) SupportsSequencedInput(ctx context.Context) bool {
	result, err := s.TerminalCapabilities(ctx)
	return err == nil && result.Protocol == terminal.InputProtocol
}

func (s clusterTerminalSource) SupportsRetainedTerminals(ctx context.Context) (bool, error) {
	result, err := s.TerminalCapabilities(ctx)
	return result.Retained, err
}

func (s clusterTerminalSource) TerminalCapabilities(ctx context.Context) (cluster.TerminalCapabilities, error) {
	if s.capabilities != nil {
		return *s.capabilities, nil
	}
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals/capabilities/input-protocol", "", newRequestID(), []byte("{}"))
	if err != nil {
		return cluster.TerminalCapabilities{}, err
	}
	if response.StatusCode == http.StatusNotFound {
		return cluster.TerminalCapabilities{}, nil
	}
	var result cluster.TerminalCapabilities
	if err := decodeTerminalAgentResponse(response, nil, &result); err != nil {
		return cluster.TerminalCapabilities{}, err
	}
	return result, nil
}

func (s clusterTerminalSource) InputSequenced(ctx context.Context, owner, id string, frame terminal.InputFrame) error {
	body, _ := json.Marshal(map[string]any{"owner": owner, "frame": frame})
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals/"+url.PathEscape(id)+"/input-sequenced", "", newRequestID(), body)
	return decodeTerminalAgentResponse(response, err, nil)
}

func (s clusterTerminalSource) Resize(ctx context.Context, owner, id string, rows, columns uint16) error {
	body, _ := json.Marshal(map[string]any{"owner": owner, "rows": rows, "columns": columns})
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals/"+url.PathEscape(id)+"/resize", "", newRequestID(), body)
	return decodeTerminalAgentResponse(response, err, nil)
}

func (s clusterTerminalSource) Close(ctx context.Context, owner, id string) error {
	body, _ := json.Marshal(map[string]string{"owner": owner})
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals/"+url.PathEscape(id)+"/close", "", newRequestID(), body)
	var result struct {
		Closed bool `json:"closed"`
	}
	err = decodeTerminalAgentResponse(response, err, &result)
	// Normalize confirmed absence at the authenticated target before federation
	// transports the result; transport and authorization errors remain failures.
	if errors.Is(err, terminal.ErrNotFound) || errors.Is(err, terminal.ErrClosed) {
		return nil
	}
	if err == nil && !result.Closed {
		return errors.New("Agent terminal close was not confirmed")
	}
	return err
}

func decodeTerminalAgentResponse(response AgentResponse, err error, target any) error {
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var problem contract.Problem
		if json.Unmarshal(response.Body, &problem) == nil {
			switch problem.Code {
			case "resource_conflict":
				return dockerx.ErrResourceConflict
			case "container_terminal_unavailable":
				return dockerx.ErrActionUnsupported
			case "container_shell_unavailable":
				return dockerx.ErrContainerShellUnavailable
			case "terminal_cleanup_pending":
				return terminal.ErrCleanupPending
			case "terminal_input_sequence":
				return terminal.ErrInputSequence
			case "terminal_input_uncertain":
				return terminal.ErrInputUncertain
			case "terminal_not_found":
				return terminal.ErrNotFound
			case "terminal_closed":
				return terminal.ErrClosed
			case "terminal_limit":
				return terminal.ErrLimit
			}
		}
		return fmt.Errorf("Agent terminal request failed with status %d", response.StatusCode)
	}
	if target == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("Agent terminal response is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("Agent terminal response has multiple JSON values")
	}
	return nil
}

func (s *Server) handleTerminalSession(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_terminal_request", "Invalid terminal request", "")
		return
	}
	token, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && (!s.checkOrigin(w, r) || !s.checkCSRF(w, r, session)) {
		return
	}
	if r.URL.Path == "/api/v1/terminal-sessions" {
		if r.Method == http.MethodGet {
			s.listRetainedTerminals(w, r, session.User.ID)
			return
		}
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		s.openTerminalSession(w, r, session.User.ID)
		return
	}
	const prefix = "/api/v1/terminal-sessions/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(rest, "/")
	if rest == r.URL.Path || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	if parts[1] == "input-stream" {
		s.handleTerminalInputSocket(w, r, token, session, parts[0])
		return
	}
	if parts[1] == "input-batch" {
		s.handleTerminalInputBatch(w, r, token, session, parts[0])
		return
	}
	s.handleTerminalOperation(w, r, session.User.ID, parts[0], parts[1])
}

func (s *Server) openTerminalSession(w http.ResponseWriter, r *http.Request, userID string) {
	if r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_terminal_request", "Invalid terminal request", "")
		return
	}
	var input terminalOpenRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
		s.writeValidationProblem(w, r, "dimensions", "valid terminal rows and columns are required")
		return
	}
	if (input.ContainerID == "") != (input.ResourceVersion == "") ||
		(input.ContainerID != "" && (!validContainerTerminalID(input.ContainerID) || len(input.ResourceVersion) > 256)) {
		s.writeValidationProblem(w, r, "containerId", "a full container identity and resource version are required together")
		return
	}
	if input.Purpose != "" && (input.Purpose != "site-management" || input.HostID != cluster.LocalHostID || input.ContainerID != "" || input.ShortcutID != "" || input.ShortcutVersion != "") {
		s.writeValidationProblem(w, r, "purpose", "a supported local terminal purpose is required")
		return
	}
	namedLaunch := input.ShortcutID != "" || input.Purpose != ""
	if input.Retained && input.ContainerID != "" || input.ShortcutID != "" && (input.ContainerID != "" || !desktopworkspace.ValidShortcutID(input.ShortcutID)) || !namedLaunch && (input.ShortcutVersion != "" || input.LookupOnly || input.PrepareOnly || input.LaunchID != "") || input.LookupOnly && input.PrepareOnly || input.LaunchID != "" && !validTerminalLaunchID(input.LaunchID) || input.Purpose != "" && !input.PrepareOnly && !input.LookupOnly && input.LaunchID == "" {
		s.writeValidationProblem(w, r, "retained", "shortcuts require a valid shortcut ID and cannot target containers")
		return
	}
	var launchCommand, launchTitle string
	if namedLaunch {
		// Serialize shortcut resolution, spawn, registration and initial input. A lost
		// HTTP response must never cause a second execution on the next launch.
		s.terminalLaunchMu.Lock()
		defer s.terminalLaunchMu.Unlock()
		if input.PrepareOnly {
			s.prepareTerminalLaunch(w, r, userID, input)
			return
		}
		if input.LookupOnly && input.LaunchID != "" {
			if intent, ok := s.terminalLaunchIntents[input.LaunchID]; ok && intent.matches(userID, input) {
				delete(s.terminalLaunchIntents, input.LaunchID)
			}
		}
		if s.resumeShortcut(w, r, userID, input) {
			if intent, ok := s.terminalLaunchIntents[input.LaunchID]; ok && intent.matches(userID, input) {
				delete(s.terminalLaunchIntents, input.LaunchID)
			}
			return
		}
		if input.LookupOnly {
			// A fenced launch cannot arrive later and create a replacement.
			// Legacy callers without an intent can only inspect existing state.
			s.writeProblem(w, r, 404, "terminal_shortcut_session_not_found", "Shortcut session not found", "")
			return
		}
		if input.LaunchID != "" {
			intent, ok := s.terminalLaunchIntents[input.LaunchID]
			if !ok || !intent.matches(userID, input) || !time.Now().Before(intent.ExpiresAt) {
				s.writeProblem(w, r, 409, "terminal_launch_intent_invalid", "Terminal launch expired or was cancelled", "")
				return
			}
		}
		if input.Purpose != "" {
			// Only this fixed script entry may be launched by a purpose. No browser command is accepted.
			launchCommand, launchTitle = "k web", "k web"
		} else {
			if s.desktopWorkspace == nil {
				s.writeProblem(w, r, 503, "desktop_workspace_unavailable", "Desktop workspace unavailable", "")
				return
			}
			workspace := s.desktopWorkspace.Workspace()
			if !workspace.Available {
				s.writeProblem(w, r, 503, "desktop_workspace_unavailable", "Desktop workspace unavailable", "")
				return
			}
			if input.ShortcutVersion == "" || input.ShortcutVersion != workspace.ResourceVersion {
				s.writeProblem(w, r, 409, "desktop_workspace_changed", "Desktop shortcut changed", "Reload the shortcut before starting it")
				return
			}
			for _, shortcut := range workspace.Shortcuts {
				if shortcut.ID != input.ShortcutID || !desktopworkspace.IsCommandTargetType(shortcut.TargetType) {
					continue
				}
				hostID := shortcut.HostID
				if hostID == "" {
					hostID = cluster.LocalHostID
				}
				if hostID != input.HostID {
					break
				}
				launchCommand, launchTitle = shortcut.Command, shortcut.Name
				break
			}
			if launchCommand == "" {
				s.writeProblem(w, r, 404, "terminal_shortcut_not_found", "Command shortcut not found", "")
				return
			}
		}
	}
	// Local terminal capability is fixed; collecting host telemetry here adds
	// CPU sampling and unrelated monitoring latency to every cold-cache open.
	host := cluster.Host{ID: cluster.LocalHostID, IsLocal: true, TerminalAvailable: true}
	var err error
	if input.HostID != cluster.LocalHostID {
		host, err = s.cluster.Host(r.Context(), input.HostID)
	}
	if err != nil || !host.TerminalAvailable {
		s.writeProblem(w, r, http.StatusConflict, "terminal_unavailable", "Terminal unavailable", "This host does not expose an authenticated terminal")
		return
	}
	if input.ContainerID != "" && !host.IsLocal {
		s.writeProblem(w, r, http.StatusConflict, "container_terminal_unavailable", "Container terminal unavailable", "Docker management targets the local Agent")
		return
	}
	s.scheduleTerminalCleanup()
	if !s.reserveTerminalOpen(userID) {
		s.writeProblem(w, r, http.StatusTooManyRequests, "terminal_limit", "Terminal session limit reached", "")
		return
	}
	defer s.releaseTerminalOpen(userID)

	owner := "panel:" + userID
	auditType, auditTarget := "cluster_host", host.ID
	if input.ContainerID != "" {
		auditType, auditTarget = "docker_container", input.ContainerID
	}
	var opened cluster.TerminalOpenResponse
	var localReliable bool
	if host.IsLocal {
		source := clusterTerminalSource{agent: s.agent}
		var snapshot terminal.Snapshot
		var openErr error
		if input.ContainerID != "" {
			snapshot, openErr = source.OpenContainer(r.Context(), owner, input)
		} else if input.Retained {
			var capability cluster.TerminalCapabilities
			capability, openErr = source.TerminalCapabilities(r.Context())
			if openErr == nil {
				source.capabilities = &capability
				snapshot, openErr = source.OpenRetained(r.Context(), owner, input.Rows, input.Columns)
			}
		} else {
			snapshot, openErr = source.Open(r.Context(), owner, input.Rows, input.Columns)
		}
		if openErr == nil {
			localReliable = source.SupportsSequencedInput(r.Context())
		}
		err = openErr
		opened = cluster.TerminalOpenResponse{SessionID: snapshot.ID, Offset: snapshot.Offset, CreatedAt: snapshot.CreatedAt, Retained: snapshot.Retained}
	} else {
		opened, err = s.cluster.TerminalOpen(r.Context(), host.ID, cluster.TerminalOpenRequest{Rows: input.Rows, Columns: input.Columns, Retained: input.Retained})
	}
	if err != nil {
		_ = s.audit(r, userID, "terminal.open", auditType, auditTarget, "failure", nil)
		s.rememberRemoteTerminalOpen(userID, host.ID, opened)
		switch {
		case errors.Is(err, cluster.ErrTerminalRetentionUnavailable):
			s.writeProblem(w, r, http.StatusConflict, "terminal_retention_unavailable", "This host does not support retained sessions", "Upgrade the node or explicitly open a temporary terminal")
			return
		case errors.Is(err, terminal.ErrLimit):
			s.writeProblem(w, r, http.StatusTooManyRequests, "terminal_limit", "Terminal session limit reached", "")
			return
		case errors.Is(err, dockerx.ErrResourceConflict):
			s.writeProblem(w, r, http.StatusConflict, "resource_conflict", "Container state changed", "")
			return
		case errors.Is(err, dockerx.ErrActionUnsupported):
			s.writeProblem(w, r, http.StatusConflict, "container_terminal_unavailable", "Container terminal unavailable", "")
			return
		case errors.Is(err, dockerx.ErrContainerShellUnavailable):
			s.writeProblem(w, r, http.StatusConflict, "container_shell_unavailable", "Container shell unavailable", "The container does not contain the required /bin/sh")
			return
		}
		s.writeProblem(w, r, http.StatusBadGateway, "terminal_open_failed", "Terminal open failed", "")
		return
	}
	if input.Retained && !opened.Retained {
		_ = s.closeTerminalBackend(r.Context(), panelTerminalSession{BackendSessionID: opened.SessionID, HostID: host.ID, Owner: owner})
		s.writeProblem(w, r, http.StatusConflict, "terminal_retention_unavailable", "This host does not support retained sessions", "")
		return
	}
	publicID, err := randomTerminalSessionID()
	if err != nil {
		_ = s.closeTerminalBackend(r.Context(), panelTerminalSession{BackendSessionID: opened.SessionID, HostID: host.ID, Owner: owner})
		s.writeProblem(w, r, http.StatusInternalServerError, "terminal_session_failed", "Terminal session failed", "")
		return
	}
	now := time.Now().UTC()
	item := panelTerminalSession{ID: publicID, BackendSessionID: opened.SessionID, HostID: host.ID, UserID: userID, Owner: owner, CreatedAt: now, UpdatedAt: now, Retained: input.Retained, ShortcutID: input.ShortcutID, Purpose: input.Purpose, Title: launchTitle, LaunchUncertain: launchCommand != "", Detachable: true, WorkspaceVisible: input.ContainerID == ""}
	if input.LaunchID != "" {
		item.LaunchIDs = []string{input.LaunchID}
	}
	if host.IsLocal {
		item.ReliableInput = localReliable
	} else {
		item.ReliableInput = s.cluster.TerminalSupportsSequencedInput(host.ID, opened.SessionID)
	}
	if !s.registerTerminalSession(item) {
		s.writeProblem(w, r, 403, "remote_terminal_disabled", "Remote terminal disabled by this center", "")
		return
	}
	if input.LaunchID != "" {
		delete(s.terminalLaunchIntents, input.LaunchID)
	}
	if launchCommand != "" {
		// Persist the identity in bounded server memory BEFORE input. Any write error
		// is uncertain, never a reason to repeat the command on a new connection.
		// Input remains frozen from registration until the launch is acknowledged.
		command := strings.ReplaceAll(strings.ReplaceAll(launchCommand, "\r\n", "\n"), "\r", "\n") + "\n"
		if err := s.inputTerminalBackend(r.Context(), item, []byte(command)); err == nil {
			item.LaunchUncertain = false
			s.terminalMu.Lock()
			if current, ok := s.terminalSessions[publicID]; ok {
				current.LaunchUncertain = false
				s.terminalSessions[publicID] = current
			}
			s.terminalMu.Unlock()
		}
	}
	_ = s.audit(r, userID, "terminal.open", auditType, auditTarget, "success", nil)
	s.writeJSON(w, http.StatusCreated, terminalSessionResponse(item))
}

func (s *Server) prepareTerminalLaunch(w http.ResponseWriter, r *http.Request, userID string, input terminalOpenRequest) {
	now := time.Now()
	userCount := 0
	for id, intent := range s.terminalLaunchIntents {
		if !now.Before(intent.ExpiresAt) {
			delete(s.terminalLaunchIntents, id)
		} else if intent.UserID == userID {
			userCount++
		}
	}
	if len(s.terminalLaunchIntents) >= maxPanelTerminals || userCount >= maxPanelTerminalsByUser {
		s.writeProblem(w, r, 429, "terminal_limit", "Terminal launch limit reached", "")
		return
	}
	id, err := randomTerminalSessionID()
	if err != nil {
		s.writeProblem(w, r, 500, "terminal_session_failed", "Terminal session failed", "")
		return
	}
	if s.terminalLaunchIntents == nil {
		s.terminalLaunchIntents = make(map[string]panelTerminalLaunchIntent)
	}
	expires := now.Add(panelTerminalLaunchTTL)
	s.terminalLaunchIntents[id] = panelTerminalLaunchIntent{UserID: userID, ShortcutID: input.ShortcutID, HostID: input.HostID, Purpose: input.Purpose, ExpiresAt: expires}
	s.writeJSON(w, 201, map[string]any{"launchId": id, "expiresAt": expires})
}

func validContainerTerminalID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func validTerminalLaunchID(id string) bool {
	if len(id) != 64 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (s *Server) reserveTerminalOpen(userID string) bool {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	userCount := 0
	activeCount := 0
	for _, item := range s.terminalSessions {
		if item.Detached {
			continue
		}
		activeCount++
		if item.UserID == userID {
			userCount++
		}
	}
	if activeCount+s.terminalOpening >= maxPanelTerminals || userCount+s.terminalOpeningUser[userID] >= maxPanelTerminalsByUser {
		return false
	}
	if s.terminalOpeningUser == nil {
		s.terminalOpeningUser = make(map[string]int)
	}
	s.terminalOpening++
	s.terminalOpeningUser[userID]++
	return true
}

func (s *Server) releaseTerminalOpen(userID string) {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	if s.terminalOpening > 0 {
		s.terminalOpening--
	}
	if s.terminalOpeningUser[userID] <= 1 {
		delete(s.terminalOpeningUser, userID)
		return
	}
	s.terminalOpeningUser[userID]--
}

func (s *Server) handleTerminalOperation(w http.ResponseWriter, r *http.Request, userID, id, action string) {
	if action != "output" && r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_terminal_request", "Invalid terminal request", "")
		return
	}
	s.terminalMu.Lock()
	item, ok := s.terminalSessions[id]
	if ok && item.UserID == userID {
		item.UpdatedAt = time.Now().UTC()
		if action == "close" && r.Method == http.MethodPost {
			item.CloseRequested = true
		}
		s.terminalSessions[id] = item
	} else {
		ok = false
	}
	s.terminalMu.Unlock()
	if !ok {
		s.writeProblem(w, r, http.StatusNotFound, "terminal_not_found", "Terminal session not found", "")
		return
	}
	if item.CloseRequested && item.InputConnection != nil {
		item.InputConnection.cancel()
	}

	if item.CloseRequested && action != "close" && action != "output" && action != "detach" {
		s.writeProblem(w, r, http.StatusConflict, "terminal_close_pending", "Terminal is closing", "")
		return
	}
	if item.LaunchUncertain && (action == "input" || action == "input-transport") {
		s.writeProblem(w, r, http.StatusConflict, "terminal_input_uncertain", "Shortcut launch is uncertain", "Inspect output and end this session before starting again")
		return
	}
	switch action {
	case "launch-ack":
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		var input struct {
			LaunchID string `json:"launchId"`
		}
		if err := s.decodeJSON(w, r, &input); err != nil {
			return
		}
		if !validTerminalLaunchID(input.LaunchID) {
			s.writeValidationProblem(w, r, "launchId", "a valid launch identity is required")
			return
		}
		s.terminalMu.Lock()
		if current, exists := s.terminalSessions[id]; exists && current.UserID == userID {
			current.LaunchIDs = slices.DeleteFunc(current.LaunchIDs, func(value string) bool { return value == input.LaunchID })
			s.terminalSessions[id] = current
		}
		s.terminalMu.Unlock()
		s.writeJSON(w, http.StatusOK, map[string]bool{"acknowledged": true})
		return
	case "detach":
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		if !item.CloseRequested || !item.Detachable {
			s.writeProblem(w, r, http.StatusConflict, "terminal_detach_unavailable", "Terminal cannot be detached", "")
			return
		}
		if err := s.audit(r, userID, "terminal.detach", "cluster_host", item.HostID, "intent", nil); err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		s.terminalMu.Lock()
		current, exists := s.terminalSessions[id]
		count, own := 0, 0
		for _, session := range s.terminalSessions {
			if session.Detached {
				count++
				if session.UserID == userID {
					own++
				}
			}
		}
		allowed := exists && current.UserID == userID && current.CloseRequested && current.Detachable && (current.Detached || count < maxPanelTerminals && own < maxPanelTerminalsByUser)
		if allowed {
			current.Detached = true
			s.terminalSessions[id] = current
		}
		s.terminalMu.Unlock()
		if !allowed {
			_ = s.audit(r, userID, "terminal.detach", "cluster_host", item.HostID, "failure", nil)
			s.writeProblem(w, r, http.StatusConflict, "terminal_detach_unavailable", "Terminal cannot be detached", "")
			return
		}
		_ = s.audit(r, userID, "terminal.detach", "cluster_host", item.HostID, "success", nil)
		s.writeJSON(w, http.StatusOK, map[string]bool{"detached": true})
		s.scheduleTerminalCleanup()
		return
	case "input-transport":
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		protocol := ""
		if item.ReliableInput {
			protocol = terminal.InputProtocol
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"protocol": protocol})
	case "output":
		if r.Method != http.MethodGet {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		offset, wait, ok := terminalOutputQuery(r)
		if !ok {
			s.writeProblem(w, r, http.StatusBadRequest, "invalid_terminal_query", "Invalid terminal query", "")
			return
		}
		output, err := s.outputTerminalBackend(r.Context(), item, offset, wait)
		if err != nil {
			if errors.Is(err, terminal.ErrNotFound) || errors.Is(err, terminal.ErrClosed) {
				s.deleteFinishedTerminalSession(id)
				s.writeProblem(w, r, http.StatusNotFound, "terminal_not_found", "Terminal session not found", "")
				return
			}
			s.writeProblem(w, r, http.StatusBadGateway, "terminal_output_failed", "Terminal output failed", "")
			return
		}
		if output.Closed || output.ExitedAt != nil {
			s.deleteFinishedTerminalSession(id)
		}
		s.writeJSON(w, http.StatusOK, output)
	case "input":
		if item.InputClaimed {
			s.writeProblem(w, r, http.StatusConflict, "terminal_input_sequence", "Terminal input stream owns this session", "")
			return
		}
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		var input struct {
			Data string `json:"data"`
		}
		if err := s.decodeJSON(w, r, &input); err != nil {
			return
		}
		data, err := decodePanelTerminalInput(input.Data)
		if err != nil || len(data) == 0 || len(data) > terminal.MaxInputBytes {
			s.writeValidationProblem(w, r, "data", "valid terminal input is required")
			return
		}
		if err := s.inputTerminalBackend(r.Context(), item, data); err != nil {
			if errors.Is(err, terminal.ErrNotFound) || errors.Is(err, terminal.ErrClosed) {
				s.deleteFinishedTerminalSession(id)
				s.writeProblem(w, r, http.StatusNotFound, "terminal_not_found", "Terminal session not found", "")
				return
			}
			s.writeProblem(w, r, http.StatusBadGateway, "terminal_input_failed", "Terminal input failed", "")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	case "resize":
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		var input struct {
			Rows    uint16 `json:"rows"`
			Columns uint16 `json:"columns"`
		}
		if err := s.decodeJSON(w, r, &input); err != nil {
			return
		}
		if input.Rows == 0 || input.Columns == 0 || input.Rows > 500 || input.Columns > 1000 {
			s.writeValidationProblem(w, r, "dimensions", "valid terminal rows and columns are required")
			return
		}
		if err := s.resizeTerminalBackend(r.Context(), item, input.Rows, input.Columns); err != nil {
			if errors.Is(err, terminal.ErrNotFound) || errors.Is(err, terminal.ErrClosed) {
				s.deleteFinishedTerminalSession(id)
				s.writeProblem(w, r, http.StatusNotFound, "terminal_not_found", "Terminal session not found", "")
				return
			}
			s.writeProblem(w, r, http.StatusBadGateway, "terminal_resize_failed", "Terminal resize failed", "")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	case "close":
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if err := s.closeTerminalBackend(ctx, item); err != nil && !errors.Is(err, terminal.ErrNotFound) && !errors.Is(err, terminal.ErrClosed) {
			_ = s.audit(r, userID, "terminal.close", "cluster_host", item.HostID, "failure", nil)
			s.writeProblem(w, r, http.StatusBadGateway, "terminal_close_failed", "Terminal close failed", "The terminal session was retained; retry closing it")
			return
		}
		s.deleteTerminalSession(id)
		_ = s.audit(r, userID, "terminal.close", "cluster_host", item.HostID, "success", nil)
		s.writeJSON(w, http.StatusOK, map[string]bool{"closed": true})
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func terminalOutputQuery(r *http.Request) (int64, time.Duration, bool) {
	query := r.URL.Query()
	if len(query) != 2 || len(query["offset"]) != 1 || len(query["wait"]) != 1 {
		return 0, 0, false
	}
	offset, err := strconv.ParseInt(query.Get("offset"), 10, 64)
	waitMS, waitErr := strconv.Atoi(query.Get("wait"))
	return offset, time.Duration(waitMS) * time.Millisecond, err == nil && waitErr == nil && offset >= 0 && waitMS >= 0 && waitMS <= 1000
}

func (s *Server) outputTerminalBackend(ctx context.Context, item panelTerminalSession, offset int64, wait time.Duration) (terminal.Output, error) {
	if item.HostID == cluster.LocalHostID {
		return clusterTerminalSource{agent: s.agent}.Output(ctx, item.Owner, item.BackendSessionID, offset, wait)
	}
	return s.cluster.TerminalOutput(ctx, item.HostID, cluster.TerminalOutputRequest{SessionID: item.BackendSessionID, Offset: offset, Wait: int(wait / time.Millisecond)})
}

func (s *Server) inputTerminalBackend(ctx context.Context, item panelTerminalSession, data []byte) error {
	if item.HostID == cluster.LocalHostID {
		return clusterTerminalSource{agent: s.agent}.Input(ctx, item.Owner, item.BackendSessionID, data)
	}
	return s.cluster.TerminalInput(ctx, item.HostID, cluster.TerminalInputRequest{SessionID: item.BackendSessionID, Data: base64.RawStdEncoding.EncodeToString(data)})
}

func (s *Server) resizeTerminalBackend(ctx context.Context, item panelTerminalSession, rows, columns uint16) error {
	if item.HostID == cluster.LocalHostID {
		return clusterTerminalSource{agent: s.agent}.Resize(ctx, item.Owner, item.BackendSessionID, rows, columns)
	}
	return s.cluster.TerminalResize(ctx, item.HostID, cluster.TerminalResizeRequest{SessionID: item.BackendSessionID, Rows: rows, Columns: columns})
}

func (s *Server) closeTerminalBackend(ctx context.Context, item panelTerminalSession) error {
	if item.HostID == cluster.LocalHostID {
		return clusterTerminalSource{agent: s.agent}.Close(ctx, item.Owner, item.BackendSessionID)
	}
	return s.cluster.TerminalClose(ctx, item.HostID, cluster.TerminalCloseRequest{SessionID: item.BackendSessionID})
}

func (s *Server) deleteTerminalSession(id string) {
	s.terminalMu.Lock()
	if item, ok := s.terminalSessions[id]; ok && item.InputConnection != nil {
		item.InputConnection.cancel()
	}
	delete(s.terminalSessions, id)
	s.terminalMu.Unlock()
}

func (s *Server) deleteFinishedTerminalSession(id string) {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	// A stale output/input response must not erase an unconfirmed close retry.
	if item, ok := s.terminalSessions[id]; ok && !item.CloseRequested {
		if item.InputConnection != nil {
			item.InputConnection.cancel()
		}
		delete(s.terminalSessions, id)
	}
}

func (s *Server) pruneTerminalSessions(before time.Time) []panelTerminalSession {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	stale := make([]panelTerminalSession, 0)
	for id, item := range s.terminalSessions {
		if item.CloseRequested {
			stale = append(stale, item)
			continue
		}
		// Batch workers own these identities. A stale/failed close must not
		// silently free their shared quota before the target confirms cleanup.
		if item.Purpose == "batch" {
			if item.UpdatedAt.After(before) {
				continue
			}
			item.CloseRequested = true
			s.terminalSessions[id] = item
			stale = append(stale, item)
			continue
		}
		if item.Retained {
			if item.CreatedAt.Add(terminal.DefaultLifetime).After(before.Add(panelTerminalIdleTTL)) {
				continue
			}
			item.CloseRequested = true
			s.terminalSessions[id] = item
			stale = append(stale, item)
			if item.InputConnection != nil {
				item.InputConnection.cancel()
			}
			continue
		}
		if item.UpdatedAt.After(before) {
			continue
		}
		stale = append(stale, item)
		if item.InputConnection != nil {
			item.InputConnection.cancel()
		}
		delete(s.terminalSessions, id)
	}
	return stale
}

// Cleanup must not put an unreachable node on the critical path of opening a
// different terminal. Only one bounded cleanup pass can run at a time.
func (s *Server) scheduleTerminalCleanup() {
	s.terminalMu.Lock()
	if s.terminalCleanupStopped {
		s.terminalMu.Unlock()
		return
	}
	if s.terminalCleanupRunning {
		s.terminalCleanupAgain = true
		s.terminalMu.Unlock()
		return
	}
	s.terminalCleanupRunning = true
	parent, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.terminalCleanupCancel = cancel
	s.terminalCleanupDone = done
	s.terminalMu.Unlock()
	stale := s.pruneTerminalSessions(time.Now().UTC().Add(-panelTerminalIdleTTL))
	go func() {
		defer func() {
			cancel()
			s.terminalMu.Lock()
			s.terminalCleanupRunning = false
			s.terminalCleanupCancel = nil
			again := s.terminalCleanupAgain
			s.terminalCleanupAgain = false
			s.terminalMu.Unlock()
			close(done)
			if again {
				s.scheduleTerminalCleanup()
			}
		}()
		for pass := 0; pass < 4; pass++ {
			if pass > 0 {
				select {
				case <-parent.Done():
					return
				case <-time.After(5 * time.Second):
				}
				stale = s.pruneTerminalSessions(time.Now().UTC().Add(-panelTerminalIdleTTL))
			}
			if len(stale) == 0 {
				return
			}
			queue := make(chan panelTerminalSession, len(stale))
			for _, item := range stale {
				queue <- item
			}
			close(queue)
			var workers sync.WaitGroup
			for n := 0; n < min(4, len(stale)); n++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					for item := range queue {
						if parent.Err() != nil {
							return
						}
						ctx, cancel := context.WithTimeout(parent, 5*time.Second)
						err := s.closeTerminalBackend(ctx, item)
						cancel()
						if err == nil || errors.Is(err, terminal.ErrNotFound) || errors.Is(err, terminal.ErrClosed) {
							s.deleteTerminalSession(item.ID)
						}
					}
				}()
			}
			workers.Wait()
			s.terminalMu.Lock()
			pendingDetached := false
			for _, item := range s.terminalSessions {
				if item.CloseRequested && (item.Detached || item.HostID != cluster.LocalHostID && !s.cluster.HostRemoteAccess(item.HostID).EffectiveTerminalEnabled) {
					pendingDetached = true
					break
				}
			}
			s.terminalMu.Unlock()
			if !pendingDetached {
				return
			}
		}
	}()
}

func (s *Server) closeTerminalSessions() {
	s.terminalMu.Lock()
	s.terminalCleanupStopped = true
	cancel, done := s.terminalCleanupCancel, s.terminalCleanupDone
	if cancel != nil {
		cancel()
	}
	s.terminalMu.Unlock()
	if done != nil {
		<-done
	}
	s.terminalMu.Lock()
	items := make([]panelTerminalSession, 0, len(s.terminalSessions))
	for _, item := range s.terminalSessions {
		if item.InputConnection != nil {
			item.InputConnection.cancel()
		}
		items = append(items, item)
	}
	s.terminalSessions = make(map[string]panelTerminalSession)
	s.terminalMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, item := range items {
		_ = s.closeTerminalBackend(ctx, item)
	}
}

func decodePanelTerminalInput(value string) ([]byte, error) {
	data, err := base64.RawStdEncoding.DecodeString(value)
	if err == nil {
		return data, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

func randomTerminalSessionID() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

// Lists metadata only. A network failure is not evidence that the process ended.
// Neither listing nor reattaching extends the target's fixed lifetime.
func terminalSessionResponse(item panelTerminalSession) terminalOpenResponse {
	result := terminalOpenResponse{SessionID: item.ID, HostID: item.HostID, Offset: 0, CreatedAt: item.CreatedAt, Retained: item.Retained, Detachable: item.Detachable, WorkspaceVisible: item.WorkspaceVisible, ShortcutID: item.ShortcutID, Purpose: item.Purpose, Title: item.Title, CloseRequested: item.CloseRequested, LaunchUncertain: item.LaunchUncertain}
	if item.Retained {
		expiry := item.CreatedAt.Add(terminal.DefaultLifetime)
		result.ExpiresAt = &expiry
	}
	return result
}
func (s *Server) listRetainedTerminals(w http.ResponseWriter, r *http.Request, userID string) {
	if r.URL.RawQuery != "" {
		s.writeProblem(w, r, 400, "invalid_terminal_query", "Invalid terminal query", "")
		return
	}
	items := make([]terminalOpenResponse, 0)
	s.terminalMu.Lock()
	for _, item := range s.terminalSessions {
		if item.UserID == userID && !item.Detached && (item.Retained || item.CloseRequested) {
			items = append(items, terminalSessionResponse(item))
		}
	}
	s.terminalMu.Unlock()
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	s.writeJSON(w, 200, map[string]any{"items": items})
}
func (s *Server) resumeShortcut(w http.ResponseWriter, r *http.Request, userID string, input terminalOpenRequest) bool {
	s.terminalMu.Lock()
	var item panelTerminalSession
	for _, existing := range s.terminalSessions {
		if existing.UserID == userID && existing.ShortcutID == input.ShortcutID && existing.Purpose == input.Purpose {
			if input.LookupOnly && input.LaunchID != "" && !slices.Contains(existing.LaunchIDs, input.LaunchID) {
				continue
			}
			item = existing
			break
		}
	}
	s.terminalMu.Unlock()
	if item.ID == "" {
		return false
	}
	if item.HostID != input.HostID {
		s.writeProblem(w, r, 409, "terminal_shortcut_host_changed", "Existing shortcut session belongs to another host", "End the existing session before changing its host")
		return true
	}
	if input.LookupOnly {
		// Closing must recover the identity even when the host is offline,
		// expiry was reached, or a previous close remains unconfirmed.
		s.writeJSON(w, http.StatusOK, terminalSessionResponse(item))
		return true
	}
	if item.HostID != cluster.LocalHostID {
		host, err := s.cluster.Host(r.Context(), item.HostID)
		if err != nil || !host.TerminalAvailable {
			s.writeProblem(w, r, 409, "terminal_unavailable", "Terminal host unavailable", "")
			return true
		}
	}
	// A failed close is still a real tracked process; do not start its replacement.
	if item.CloseRequested || !time.Now().Before(item.CreatedAt.Add(terminal.DefaultLifetime)) {
		s.scheduleTerminalCleanup()
		s.writeProblem(w, r, 409, "terminal_close_pending", "End the previous session before restarting", "")
		return true
	}
	if input.LaunchID != "" {
		intent, ok := s.terminalLaunchIntents[input.LaunchID]
		if !ok || !intent.matches(userID, input) || !time.Now().Before(intent.ExpiresAt) {
			s.writeProblem(w, r, 409, "terminal_launch_intent_invalid", "Terminal launch expired or was cancelled", "")
			return true
		}
		s.terminalMu.Lock()
		if current, ok := s.terminalSessions[item.ID]; ok {
			if len(current.LaunchIDs) >= maxPanelTerminals {
				s.terminalMu.Unlock()
				s.writeProblem(w, r, 429, "terminal_limit", "Terminal launch identity limit reached", "Close the existing session before reopening it")
				return true
			}
			current.LaunchIDs = append(current.LaunchIDs, input.LaunchID)
			s.terminalSessions[item.ID] = current
		}
		s.terminalMu.Unlock()
	}
	s.writeJSON(w, http.StatusOK, terminalSessionResponse(item))
	return true
}
