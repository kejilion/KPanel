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
	"github.com/kejilion/kejilion-panel/internal/store"
)

const (
	maxPanelBrowseWSSessions       = 16
	maxPanelBrowseWSSessionsByUser = 4
	panelBrowseWSIdleTTL           = 5 * time.Minute
	// browseWSMaxWaitMillis mirrors internal/agent's maxBrowseWSWait and
	// cluster's BrowseWSOutputRequest.Wait validation (both 1500ms) so a
	// client-requested long-poll never gets silently clamped by one layer
	// but not another.
	browseWSMaxWaitMillis = 1500
)

var (
	errBrowseWSSessionNotFound = errors.New("browse ws session not found")
	errBrowseWSSessionLimit    = errors.New("browse ws session limit reached")
	errBrowseWSTargetBlocked   = errors.New("browse ws target blocked")
)

// clusterBrowseWSSource implements cluster.BrowseWSBackend by calling this
// host's own Agent (internal/agent/browse_ws.go's /v1/browse/ws* routes) —
// the WS counterpart of clusterBrowseSource. Serves two roles, exactly like
// clusterTerminalSource: wired into cluster.NewService as the "being
// controlled by a remote federation request" backend, and called directly
// (bypassing cluster.Service) for this Panel's own local-hostID path in
// openBrowseWSSession/outputBrowseWSBackend/etc below.
type clusterBrowseWSSource struct {
	agent agentAPI
	// store carries the egress host's own LAN opt-in, for the same reason
	// clusterBrowseSource holds one.
	store *store.Store
}

// localBrowseWS builds the backend used for this Panel's own local-hostID
// path. It exists so the egress host's LAN opt-in is picked up at every call
// site instead of being easy to forget in one of them.
func (s *Server) localBrowseWS() clusterBrowseWSSource {
	return clusterBrowseWSSource{agent: s.agent, store: s.store}
}

type browseWSAgentOpenInput struct {
	Owner               string              `json:"owner"`
	URL                 string              `json:"url"`
	Headers             map[string][]string `json:"headers,omitempty"`
	AllowPrivateNetwork bool                `json:"allowPrivateNetwork,omitempty"`
}

type browseWSAgentOpenOutput struct {
	SessionID string `json:"sessionId"`
}

type browseWSAgentMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type browseWSAgentOutputOutput struct {
	Messages    []browseWSAgentMessage `json:"messages"`
	NextOffset  int                    `json:"nextOffset"`
	Closed      bool                   `json:"closed"`
	CloseReason string                 `json:"closeReason,omitempty"`
}

type browseWSAgentInputInput struct {
	Owner string `json:"owner"`
	Type  string `json:"type"`
	Data  string `json:"data"`
}

type browseWSAgentCloseInput struct {
	Owner string `json:"owner"`
}

func (s clusterBrowseWSSource) Open(ctx context.Context, owner, targetURL string, headers map[string][]string) (string, error) {
	body, err := json.Marshal(browseWSAgentOpenInput{
		Owner: owner, URL: targetURL, Headers: headers,
		AllowPrivateNetwork: browseAllowsPrivateNetworks(s.store),
	})
	if err != nil {
		return "", err
	}
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/browse/ws", "", newRequestID(), body)
	var decoded browseWSAgentOpenOutput
	if decodeErr := decodeBrowseWSAgentResponse(response, err, &decoded); decodeErr != nil {
		return "", decodeErr
	}
	return decoded.SessionID, nil
}

func (s clusterBrowseWSSource) Output(ctx context.Context, owner, id string, offset int, wait time.Duration) ([]cluster.BrowseWSFrame, int, bool, string, error) {
	query := url.Values{}
	query.Set("owner", owner)
	query.Set("offset", strconv.Itoa(offset))
	query.Set("wait", strconv.Itoa(int(wait/time.Millisecond)))
	response, err := s.agent.Get(ctx, "/v1/browse/ws/"+url.PathEscape(id)+"/output", query.Encode(), newRequestID())
	var decoded browseWSAgentOutputOutput
	if decodeErr := decodeBrowseWSAgentResponse(response, err, &decoded); decodeErr != nil {
		return nil, 0, false, "", decodeErr
	}
	frames := make([]cluster.BrowseWSFrame, len(decoded.Messages))
	for i, msg := range decoded.Messages {
		data, decodeErr := base64.StdEncoding.DecodeString(msg.Data)
		if decodeErr != nil {
			return nil, 0, false, "", errors.New("Agent browse ws response message is invalid")
		}
		frames[i] = cluster.BrowseWSFrame{Binary: msg.Type == "binary", Data: data}
	}
	return frames, decoded.NextOffset, decoded.Closed, decoded.CloseReason, nil
}

func (s clusterBrowseWSSource) Input(ctx context.Context, owner, id string, binary bool, data []byte) error {
	typ := "text"
	if binary {
		typ = "binary"
	}
	body, err := json.Marshal(browseWSAgentInputInput{Owner: owner, Type: typ, Data: base64.StdEncoding.EncodeToString(data)})
	if err != nil {
		return err
	}
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/browse/ws/"+url.PathEscape(id)+"/input", "", newRequestID(), body)
	return decodeBrowseWSAgentResponse(response, err, nil)
}

func (s clusterBrowseWSSource) Close(ctx context.Context, owner, id string) error {
	body, err := json.Marshal(browseWSAgentCloseInput{Owner: owner})
	if err != nil {
		return err
	}
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/browse/ws/"+url.PathEscape(id)+"/close", "", newRequestID(), body)
	return decodeBrowseWSAgentResponse(response, err, nil)
}

func decodeBrowseWSAgentResponse(response AgentResponse, err error, target any) error {
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var problem contract.Problem
		if json.Unmarshal(response.Body, &problem) == nil {
			switch problem.Code {
			case "browse_ws_not_found":
				return errBrowseWSSessionNotFound
			case "browse_ws_limit_reached":
				return errBrowseWSSessionLimit
			case "browse_target_blocked":
				return errBrowseWSTargetBlocked
			}
		}
		return fmt.Errorf("Agent browse ws request failed with status %d", response.StatusCode)
	}
	if target == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("Agent browse ws response is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("Agent browse ws response has multiple JSON values")
	}
	return nil
}

// resolveBrowseWSHost mirrors resolveBrowseHost but gates on BrowseWSAvailable
// instead of BrowseAvailable — a host paired with only cluster.browse.fetch
// scope must not be usable for the WS relay, and vice versa (see
// TestServiceV2BrowseWSRequiresBrowseWSScope in internal/cluster).
func (s *Server) resolveBrowseWSHost(ctx context.Context, hostID string) (cluster.Host, string, error) {
	if hostID == "" {
		hostID = cluster.LocalHostID
	}
	host, err := s.cluster.Host(ctx, hostID)
	if err != nil {
		return cluster.Host{}, hostID, errBrowseHostNotFound
	}
	if !host.BrowseWSAvailable {
		return cluster.Host{}, hostID, errBrowseHostUnsupported
	}
	return host, hostID, nil
}

type panelBrowseWSSession struct {
	ID               string
	BackendSessionID string
	HostID           string
	UserID           string
	Owner            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type browseWSOpenRequest struct {
	HostID  string              `json:"hostId,omitempty"`
	URL     string              `json:"url"`
	Headers map[string][]string `json:"headers,omitempty"`
}

type browseWSOpenResponse struct {
	SessionID string `json:"sessionId"`
	HostID    string `json:"hostId"`
}

type browseWSMessageWire struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

type browseWSOutputResponse struct {
	Messages    []browseWSMessageWire `json:"messages"`
	NextOffset  int                   `json:"nextOffset"`
	Closed      bool                  `json:"closed"`
	CloseReason string                `json:"closeReason,omitempty"`
}

type browseWSInputRequest struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// handleBrowseWSSession routes both "open a new session" (POST on the
// collection) and per-session operations, mirroring handleTerminalSession's
// split exactly.
func (s *Server) handleBrowseWSSession(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	session, ok := s.requireBrowseSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && (!s.checkBrowseOrigin(w, r) || !s.checkBrowseCSRF(w, r, session)) {
		return
	}
	if r.URL.Path == "/api/v1/browse/ws-sessions" {
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		s.openBrowseWSSession(w, r, session.UserID)
		return
	}
	const prefix = "/api/v1/browse/ws-sessions/"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.Split(rest, "/")
	if rest == r.URL.Path || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	s.handleBrowseWSOperation(w, r, session.UserID, parts[0], parts[1])
}

func (s *Server) openBrowseWSSession(w http.ResponseWriter, r *http.Request, userID string) {
	if r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	var input browseWSOpenRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if input.URL == "" {
		s.writeValidationProblem(w, r, "url", "a target ws(s) URL is required")
		return
	}
	_, hostID, err := s.resolveBrowseWSHost(r.Context(), input.HostID)
	if err != nil {
		s.writeBrowseHostError(w, r, err)
		return
	}

	stale := s.pruneBrowseWSSessions(time.Now().UTC().Add(-panelBrowseWSIdleTTL))
	for _, item := range stale {
		_ = s.closeBrowseWSBackend(r.Context(), item)
	}
	if !s.reserveBrowseWSOpen(userID) {
		s.writeProblem(w, r, http.StatusTooManyRequests, "browse_ws_limit", "Browse WebSocket session limit reached", "")
		return
	}
	defer s.releaseBrowseWSOpen(userID)

	owner := "panel:" + userID
	var backendSessionID string
	if hostID == cluster.LocalHostID {
		backendSessionID, err = s.localBrowseWS().Open(r.Context(), owner, input.URL, input.Headers)
	} else {
		var opened cluster.BrowseWSOpenResponse
		opened, err = s.cluster.BrowseWSOpen(r.Context(), hostID, cluster.BrowseWSOpenRequest{URL: input.URL, Headers: input.Headers})
		backendSessionID = opened.SessionID
	}
	if err != nil {
		s.writeProblem(w, r, http.StatusBadGateway, "browse_ws_open_failed", "浏览 WebSocket 打开失败", "")
		return
	}
	publicID, err := randomBrowseWSSessionID()
	if err != nil {
		_ = s.closeBrowseWSBackend(r.Context(), panelBrowseWSSession{BackendSessionID: backendSessionID, HostID: hostID, Owner: owner})
		s.writeProblem(w, r, http.StatusInternalServerError, "browse_ws_session_failed", "Browse WebSocket session failed", "")
		return
	}
	now := time.Now().UTC()
	item := panelBrowseWSSession{ID: publicID, BackendSessionID: backendSessionID, HostID: hostID, UserID: userID, Owner: owner, CreatedAt: now, UpdatedAt: now}
	s.browseWSMu.Lock()
	s.browseWSSessions[publicID] = item
	s.browseWSMu.Unlock()
	s.writeJSON(w, http.StatusCreated, browseWSOpenResponse{SessionID: publicID, HostID: hostID})
}

func (s *Server) reserveBrowseWSOpen(userID string) bool {
	s.browseWSMu.Lock()
	defer s.browseWSMu.Unlock()
	userCount := 0
	for _, item := range s.browseWSSessions {
		if item.UserID == userID {
			userCount++
		}
	}
	if len(s.browseWSSessions)+s.browseWSOpening >= maxPanelBrowseWSSessions ||
		userCount+s.browseWSOpeningUser[userID] >= maxPanelBrowseWSSessionsByUser {
		return false
	}
	if s.browseWSOpeningUser == nil {
		s.browseWSOpeningUser = make(map[string]int)
	}
	s.browseWSOpening++
	s.browseWSOpeningUser[userID]++
	return true
}

func (s *Server) releaseBrowseWSOpen(userID string) {
	s.browseWSMu.Lock()
	defer s.browseWSMu.Unlock()
	if s.browseWSOpening > 0 {
		s.browseWSOpening--
	}
	if s.browseWSOpeningUser[userID] <= 1 {
		delete(s.browseWSOpeningUser, userID)
		return
	}
	s.browseWSOpeningUser[userID]--
}

func (s *Server) handleBrowseWSOperation(w http.ResponseWriter, r *http.Request, userID, id, action string) {
	if action != "output" && r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	s.browseWSMu.Lock()
	item, ok := s.browseWSSessions[id]
	if ok && item.UserID == userID {
		item.UpdatedAt = time.Now().UTC()
		s.browseWSSessions[id] = item
	} else {
		ok = false
	}
	s.browseWSMu.Unlock()
	if !ok {
		s.writeProblem(w, r, http.StatusNotFound, "browse_ws_not_found", "Browse WebSocket session not found", "")
		return
	}

	switch action {
	case "output":
		if r.Method != http.MethodGet {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		offset, wait, ok := browseWSOutputQuery(r)
		if !ok {
			s.writeProblem(w, r, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
			return
		}
		output, err := s.outputBrowseWSBackend(r.Context(), item, offset, wait)
		if err != nil {
			if errors.Is(err, errBrowseWSSessionNotFound) {
				s.deleteBrowseWSSession(id)
				s.writeProblem(w, r, http.StatusNotFound, "browse_ws_not_found", "Browse WebSocket session not found", "")
				return
			}
			s.writeProblem(w, r, http.StatusBadGateway, "browse_ws_output_failed", "浏览 WebSocket 读取失败", "")
			return
		}
		if output.Closed {
			s.deleteBrowseWSSession(id)
		}
		s.writeJSON(w, http.StatusOK, output)
	case "input":
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		var input browseWSInputRequest
		if err := s.decodeJSON(w, r, &input); err != nil {
			return
		}
		if input.Type != "text" && input.Type != "binary" {
			s.writeValidationProblem(w, r, "type", "type must be text or binary")
			return
		}
		data, err := base64.StdEncoding.DecodeString(input.Data)
		if err != nil || len(data) == 0 {
			s.writeValidationProblem(w, r, "data", "valid browse ws input is required")
			return
		}
		if err := s.inputBrowseWSBackend(r.Context(), item, input.Type == "binary", data); err != nil {
			if errors.Is(err, errBrowseWSSessionNotFound) {
				s.deleteBrowseWSSession(id)
				s.writeProblem(w, r, http.StatusNotFound, "browse_ws_not_found", "Browse WebSocket session not found", "")
				return
			}
			s.writeProblem(w, r, http.StatusBadGateway, "browse_ws_input_failed", "浏览 WebSocket 写入失败", "")
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	case "close":
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		_ = s.closeBrowseWSBackend(r.Context(), item)
		s.deleteBrowseWSSession(id)
		s.writeJSON(w, http.StatusOK, map[string]bool{"closed": true})
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func browseWSOutputQuery(r *http.Request) (int, time.Duration, bool) {
	query := r.URL.Query()
	if len(query) != 2 || len(query["offset"]) != 1 || len(query["wait"]) != 1 {
		return 0, 0, false
	}
	offset, err := strconv.Atoi(query.Get("offset"))
	waitMS, waitErr := strconv.Atoi(query.Get("wait"))
	ok := err == nil && waitErr == nil && offset >= 0 && waitMS >= 0 && waitMS <= browseWSMaxWaitMillis
	return offset, time.Duration(waitMS) * time.Millisecond, ok
}

func (s *Server) outputBrowseWSBackend(ctx context.Context, item panelBrowseWSSession, offset int, wait time.Duration) (browseWSOutputResponse, error) {
	if item.HostID == cluster.LocalHostID {
		frames, next, closed, reason, err := s.localBrowseWS().Output(ctx, item.Owner, item.BackendSessionID, offset, wait)
		if err != nil {
			return browseWSOutputResponse{}, err
		}
		messages := make([]browseWSMessageWire, len(frames))
		for i, frame := range frames {
			typ := "text"
			if frame.Binary {
				typ = "binary"
			}
			messages[i] = browseWSMessageWire{Type: typ, Data: base64.StdEncoding.EncodeToString(frame.Data)}
		}
		return browseWSOutputResponse{Messages: messages, NextOffset: next, Closed: closed, CloseReason: reason}, nil
	}
	resp, err := s.cluster.BrowseWSOutput(ctx, item.HostID, cluster.BrowseWSOutputRequest{
		SessionID: item.BackendSessionID, Offset: offset, Wait: int(wait / time.Millisecond),
	})
	if err != nil {
		return browseWSOutputResponse{}, err
	}
	messages := make([]browseWSMessageWire, len(resp.Messages))
	for i, msg := range resp.Messages {
		messages[i] = browseWSMessageWire{Type: msg.Type, Data: msg.Data}
	}
	return browseWSOutputResponse{Messages: messages, NextOffset: resp.NextOffset, Closed: resp.Closed, CloseReason: resp.CloseReason}, nil
}

func (s *Server) inputBrowseWSBackend(ctx context.Context, item panelBrowseWSSession, binary bool, data []byte) error {
	if item.HostID == cluster.LocalHostID {
		return s.localBrowseWS().Input(ctx, item.Owner, item.BackendSessionID, binary, data)
	}
	typ := "text"
	if binary {
		typ = "binary"
	}
	return s.cluster.BrowseWSInput(ctx, item.HostID, cluster.BrowseWSInputRequest{
		SessionID: item.BackendSessionID, Type: typ, Data: base64.StdEncoding.EncodeToString(data),
	})
}

func (s *Server) closeBrowseWSBackend(ctx context.Context, item panelBrowseWSSession) error {
	if item.HostID == cluster.LocalHostID {
		return s.localBrowseWS().Close(ctx, item.Owner, item.BackendSessionID)
	}
	return s.cluster.BrowseWSClose(ctx, item.HostID, cluster.BrowseWSCloseRequest{SessionID: item.BackendSessionID})
}

func (s *Server) deleteBrowseWSSession(id string) {
	s.browseWSMu.Lock()
	delete(s.browseWSSessions, id)
	s.browseWSMu.Unlock()
}

func (s *Server) pruneBrowseWSSessions(before time.Time) []panelBrowseWSSession {
	s.browseWSMu.Lock()
	defer s.browseWSMu.Unlock()
	stale := make([]panelBrowseWSSession, 0)
	for id, item := range s.browseWSSessions {
		if item.UpdatedAt.After(before) {
			continue
		}
		stale = append(stale, item)
		delete(s.browseWSSessions, id)
	}
	return stale
}

func (s *Server) closeBrowseWSSessions() {
	s.browseWSMu.Lock()
	items := make([]panelBrowseWSSession, 0, len(s.browseWSSessions))
	for _, item := range s.browseWSSessions {
		items = append(items, item)
	}
	s.browseWSSessions = make(map[string]panelBrowseWSSession)
	s.browseWSMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, item := range items {
		_ = s.closeBrowseWSBackend(ctx, item)
	}
}

func randomBrowseWSSessionID() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
