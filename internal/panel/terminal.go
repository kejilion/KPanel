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
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	maxPanelTerminals       = 16
	maxPanelTerminalsByUser = 4
	panelTerminalIdleTTL    = 35 * time.Minute
)

type clusterTerminalSource struct{ agent agentAPI }

type panelTerminalSession struct {
	ID               string
	BackendSessionID string
	HostID           string
	UserID           string
	Owner            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CloseRequested   bool
	ReliableInput    bool
	InputClaimed     bool
	InputConnection  *terminalInputConnection
}

type terminalOpenRequest struct {
	HostID          string `json:"hostId"`
	Rows            uint16 `json:"rows"`
	Columns         uint16 `json:"columns"`
	ContainerID     string `json:"containerId,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

type terminalOpenResponse struct {
	SessionID string    `json:"sessionId"`
	HostID    string    `json:"hostId"`
	Offset    int64     `json:"offset"`
	CreatedAt time.Time `json:"createdAt"`
}

func (s clusterTerminalSource) Open(ctx context.Context, owner string, rows, columns uint16) (terminal.Snapshot, error) {
	body, _ := json.Marshal(map[string]any{"owner": owner, "rows": rows, "columns": columns})
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals", "", newRequestID(), body)
	var result terminal.Snapshot
	return result, decodeTerminalAgentResponse(response, err, &result)
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
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/terminals/capabilities/input-protocol", "", newRequestID(), []byte("{}"))
	var result struct {
		Protocol string `json:"protocol"`
	}
	return decodeTerminalAgentResponse(response, err, &result) == nil && result.Protocol == terminal.InputProtocol
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
	stale := s.pruneTerminalSessions(time.Now().UTC().Add(-panelTerminalIdleTTL))
	for _, item := range stale {
		_ = s.closeTerminalBackend(r.Context(), item)
	}
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
	if host.IsLocal {
		var snapshot terminal.Snapshot
		var openErr error
		if input.ContainerID != "" {
			snapshot, openErr = (clusterTerminalSource{agent: s.agent}).OpenContainer(r.Context(), owner, input)
		} else {
			snapshot, openErr = (clusterTerminalSource{agent: s.agent}).Open(r.Context(), owner, input.Rows, input.Columns)
		}
		err = openErr
		opened = cluster.TerminalOpenResponse{SessionID: snapshot.ID, Offset: snapshot.Offset, CreatedAt: snapshot.CreatedAt}
	} else {
		opened, err = s.cluster.TerminalOpen(r.Context(), host.ID, cluster.TerminalOpenRequest{Rows: input.Rows, Columns: input.Columns})
	}
	if err != nil {
		_ = s.audit(r, userID, "terminal.open", auditType, auditTarget, "failure", nil)
		switch {
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
	publicID, err := randomTerminalSessionID()
	if err != nil {
		_ = s.closeTerminalBackend(r.Context(), panelTerminalSession{BackendSessionID: opened.SessionID, HostID: host.ID, Owner: owner})
		s.writeProblem(w, r, http.StatusInternalServerError, "terminal_session_failed", "Terminal session failed", "")
		return
	}
	now := time.Now().UTC()
	item := panelTerminalSession{ID: publicID, BackendSessionID: opened.SessionID, HostID: host.ID, UserID: userID, Owner: owner, CreatedAt: now, UpdatedAt: now}
	if host.IsLocal {
		item.ReliableInput = (clusterTerminalSource{agent: s.agent}).SupportsSequencedInput(r.Context())
	} else {
		item.ReliableInput = s.cluster.TerminalSupportsSequencedInput(host.ID, opened.SessionID)
	}
	s.terminalMu.Lock()
	s.terminalSessions[publicID] = item
	s.terminalMu.Unlock()
	_ = s.audit(r, userID, "terminal.open", auditType, auditTarget, "success", nil)
	s.writeJSON(w, http.StatusCreated, terminalOpenResponse{SessionID: publicID, HostID: host.ID, Offset: opened.Offset, CreatedAt: opened.CreatedAt})
}

func validContainerTerminalID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (s *Server) reserveTerminalOpen(userID string) bool {
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	userCount := 0
	for _, item := range s.terminalSessions {
		if item.UserID == userID {
			userCount++
		}
	}
	if len(s.terminalSessions)+s.terminalOpening >= maxPanelTerminals || userCount+s.terminalOpeningUser[userID] >= maxPanelTerminalsByUser {
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

	switch action {
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

func (s *Server) closeTerminalSessions() {
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
