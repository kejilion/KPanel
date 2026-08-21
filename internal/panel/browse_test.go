package panel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/store"
)

// The browse tests exercise two origins, because the feature is defined by the
// split between them (see internal/panel/browse_origin.go): the panel answers
// on browseTestPanelHost, and the browse shell plus every browse endpoint
// answers only on browseTestHost. Both are loopback names, which is also the
// pair a developer actually reaches these endpoints from.
const (
	browseTestPanelOrigin = "http://localhost"
	browseTestPanelHost   = "localhost"
	browseTestOrigin      = "http://browse.localhost"
	browseTestHost        = "browse.localhost"
)

func newBrowseTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	server, tokenPath := newTestServerWithPublicURL(t, browseTestPanelOrigin)
	_, version := server.store.AllowedHosts()
	if err := server.store.ReplaceAllowedHosts(version, store.AllowedHosts{
		BrowseOrigin: browseTestHost, UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("configure browse origin: %v", err)
	}
	return server, tokenPath
}

// bootstrapBrowseCookies returns the *browse origin's* credentials, not the
// panel's. It walks the real crossing — bootstrap an admin on the panel
// origin, mint a handoff ticket there, redeem it on the browse origin — so
// every test that calls it proves that path still works, and none of them can
// accidentally authenticate a browse endpoint with a panel session.
func bootstrapBrowseCookies(t *testing.T, server *Server, tokenPath string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	sessionCookie, csrfCookie := bootstrapPanelCookies(t, server, tokenPath)
	return browseHandoffCookies(t, server, sessionCookie, csrfCookie)
}

func bootstrapPanelCookies(t *testing.T, server *Server, tokenPath string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]string{
		"token": string(token), "username": "admin", "password": "a-strong-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	response := browsePerformRequest(server, http.MethodPost, "/api/v1/auth/bootstrap", body, map[string]string{
		"Content-Type": "application/json", "Origin": browseTestPanelOrigin, "Host": browseTestPanelHost,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", response.Code, response.Body.String())
	}
	return authCookies(t, response)
}

func browseHandoffCookies(
	t *testing.T,
	server *Server,
	sessionCookie, csrfCookie *http.Cookie,
) (*http.Cookie, *http.Cookie) {
	t.Helper()
	minted := browseRequest(server, http.MethodPost, "/api/v1/browse/handoff", nil,
		sessionCookie, csrfCookie, map[string]string{
			"Host": browseTestPanelHost, "Origin": browseTestPanelOrigin,
			"X-CSRF-Token": csrfCookie.Value,
		})
	if minted.Code != http.StatusCreated {
		t.Fatalf("handoff failed: %d %s", minted.Code, minted.Body.String())
	}
	var handoff browseHandoffResponse
	if err := json.Unmarshal(minted.Body.Bytes(), &handoff); err != nil {
		t.Fatalf("decode handoff: %v", err)
	}
	target, err := url.Parse(handoff.URL)
	if err != nil {
		t.Fatalf("parse handoff url: %v", err)
	}
	entered := browsePerformRequest(server, http.MethodGet,
		browseEnterPath+"?"+target.RawQuery, nil, nil)
	if entered.Code != http.StatusSeeOther {
		t.Fatalf("enter failed: %d %s", entered.Code, entered.Body.String())
	}
	var browseSession, browseCSRF *http.Cookie
	for _, cookie := range entered.Result().Cookies() {
		switch cookie.Name {
		case browseSessionCookieName:
			browseSession = cookie
		case browseCSRFCookieName:
			browseCSRF = cookie
		}
	}
	if browseSession == nil || browseCSRF == nil {
		t.Fatal("enter did not set both browse cookies")
	}
	return browseSession, browseCSRF
}

func browsePerformRequest(handler http.Handler, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	return browseRequest(handler, method, path, body, nil, nil, headers)
}

func browseAuthenticatedRequest(
	handler http.Handler,
	method, path string,
	body []byte,
	sessionCookie, csrfCookie *http.Cookie,
	headers map[string]string,
) *httptest.ResponseRecorder {
	return browseRequest(handler, method, path, body, sessionCookie, csrfCookie, headers)
}

func browseRequest(
	handler http.Handler,
	method, path string,
	body []byte,
	sessionCookie, csrfCookie *http.Cookie,
	headers map[string]string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Host = browseTestHost
	for name, value := range headers {
		if name == "Host" {
			request.Host = value
			continue
		}
		request.Header.Set(name, value)
	}
	if sessionCookie != nil {
		request.AddCookie(sessionCookie)
	}
	if csrfCookie != nil {
		request.AddCookie(csrfCookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestHandleBrowseFetchRejectsUnauthenticated(t *testing.T) {
	server, _ := newBrowseTestServer(t)
	body, _ := json.Marshal(browseFetchRequest{URL: "https://example.com/"})
	response := browsePerformRequest(server, http.MethodPost, "/api/v1/browse/fetch", body, map[string]string{
		"Content-Type": "application/json", "Origin": browseTestOrigin,
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleBrowseFetchRejectsMissingCSRF(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	body, _ := json.Marshal(browseFetchRequest{URL: "https://example.com/"})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/fetch", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin,
			// X-CSRF-Token intentionally omitted.
		})
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleBrowseFetchRejectsWrongMethod(t *testing.T) {
	server, _ := newBrowseTestServer(t)
	response := browsePerformRequest(server, http.MethodGet, "/api/v1/browse/fetch", nil, nil)
	if response.Code == http.StatusOK {
		t.Fatalf("GET unexpectedly succeeded: %d", response.Code)
	}
}

func TestHandleBrowseFetchUnknownHostRejected(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	body, _ := json.Marshal(browseFetchRequest{HostID: "does-not-exist", URL: "https://example.com/"})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/fetch", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
		})
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var problem struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(response.Body.Bytes(), &problem)
	if problem.Code != "browse_host_not_found" {
		t.Fatalf("code = %q", problem.Code)
	}
}

func TestHandleBrowseFetchDefaultsToLocalHostAndRelaysThroughAgent(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)

	agentOutput, _ := json.Marshal(map[string]any{
		"statusCode": 201,
		"headers":    map[string][]string{"X-Upstream": {"1"}},
		"body":       base64.StdEncoding.EncodeToString([]byte("relayed body")),
	})
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json; charset=utf-8", Body: agentOutput,
	}}
	server.agent = agent

	body, _ := json.Marshal(browseFetchRequest{
		URL:     "https://example.com/path",
		Method:  "POST",
		Headers: map[string][]string{"X-Forwarded": {"client-value"}},
		Body:    base64.StdEncoding.EncodeToString([]byte("payload")),
	})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/fetch", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), base64.StdEncoding.EncodeToString([]byte("relayed body"))) {
		t.Fatalf("response did not carry the agent's payload: %s", response.Body.String())
	}

	var browseCall *agentCall
	for _, call := range agent.snapshotCalls() {
		if call.path == "/v1/browse/fetch" {
			c := call
			browseCall = &c
		}
	}
	if browseCall == nil {
		t.Fatal("handler never called the agent's browse fetch endpoint")
	}
	if browseCall.method != http.MethodPost {
		t.Fatalf("agent call method = %q", browseCall.method)
	}
	var forwarded map[string]any
	if err := json.Unmarshal(browseCall.body, &forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded["url"] != "https://example.com/path" {
		t.Fatalf("forwarded url = %v", forwarded["url"])
	}
	if _, hasHostID := forwarded["hostId"]; hasHostID {
		t.Fatalf("hostId leaked into the agent-facing payload: %v", forwarded)
	}
}

func TestHandleBrowseFetchExplicitLocalHostID(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	agentOutput, _ := json.Marshal(map[string]any{"statusCode": 200, "headers": map[string][]string{}, "body": ""})
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: agentOutput}}
	server.agent = agent

	body, _ := json.Marshal(browseFetchRequest{HostID: "local", URL: "https://example.com/"})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/fetch", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleBrowseSessionStartRejectsUnauthenticated(t *testing.T) {
	server, _ := newBrowseTestServer(t)
	body, _ := json.Marshal(browseSessionStartRequest{})
	response := browsePerformRequest(server, http.MethodPost, "/api/v1/browse/sessions", body, map[string]string{
		"Content-Type": "application/json", "Origin": browseTestOrigin,
	})
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleBrowseSessionStartUnknownHostRejected(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	body, _ := json.Marshal(browseSessionStartRequest{HostID: "does-not-exist"})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/sessions", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
		})
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleBrowseSessionStartRecordsOneAuditEntryPerSessionNotPerFetch(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	agentOutput, _ := json.Marshal(map[string]any{"statusCode": 200, "headers": map[string][]string{}, "body": ""})
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: agentOutput}}

	startBody, _ := json.Marshal(browseSessionStartRequest{HostID: "local"})
	start := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/sessions", startBody,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
		})
	if start.Code != http.StatusCreated {
		t.Fatalf("session start status = %d body=%s", start.Code, start.Body.String())
	}
	var startResponse browseSessionStartResponse
	if err := json.Unmarshal(start.Body.Bytes(), &startResponse); err != nil || startResponse.HostID != "local" {
		t.Fatalf("session start response = %s (err=%v)", start.Body.String(), err)
	}

	// Several relayed sub-resource fetches must not add further audit entries.
	for i := 0; i < 3; i++ {
		fetchBody, _ := json.Marshal(browseFetchRequest{HostID: "local", URL: "https://example.com/asset"})
		fetch := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/fetch", fetchBody,
			sessionCookie, csrfCookie, map[string]string{
				"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
			})
		if fetch.Code != http.StatusOK {
			t.Fatalf("fetch %d status = %d body=%s", i, fetch.Code, fetch.Body.String())
		}
	}

	events, _ := server.store.ListAudit(50, "")
	var sessionStarts int
	for _, event := range events {
		if event.Action != "browse.session.start" {
			continue
		}
		sessionStarts++
		if event.TargetID != "local" {
			t.Fatalf("audit target = %q, want %q", event.TargetID, "local")
		}
		if event.Result != "success" {
			t.Fatalf("audit result = %q", event.Result)
		}
	}
	if sessionStarts != 1 {
		t.Fatalf("browse.session.start audit entries = %d, want 1 (fetch calls must stay unaudited)", sessionStarts)
	}
}

func TestHandleBrowseFetchAgentFailureSurfacesAsServiceUnavailable(t *testing.T) {
	server, tokenPath := newBrowseTestServer(t)
	sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
	server.agent = &stubAgent{err: errors.New("socket unavailable")}

	body, _ := json.Marshal(browseFetchRequest{URL: "https://example.com/"})
	response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/fetch", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
		})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestDrainFederatedBrowseFetchAssemblesMultipleChunks(t *testing.T) {
	chunks := []string{"first-", "second-", "third"}
	var outputCalls int
	open := func(context.Context, string, cluster.BrowseFetchRequest) (cluster.BrowseFetchOpenResponse, error) {
		// 18 = len("first-second-third"): TotalBytes is now checked against
		// what actually arrives, so it has to be the real length.
		return cluster.BrowseFetchOpenResponse{SessionID: "sess-1", StatusCode: 201, Headers: map[string][]string{"X-Test": {"1"}}, TotalBytes: 18}, nil
	}
	output := func(_ context.Context, hostID string, input cluster.BrowseFetchOutputRequest) (cluster.BrowseFetchOutputResponse, error) {
		if hostID != "remote-1" || input.SessionID != "sess-1" {
			t.Fatalf("unexpected output call: host=%q input=%+v", hostID, input)
		}
		index := outputCalls
		outputCalls++
		if index >= len(chunks) {
			t.Fatal("more output calls than expected chunks")
		}
		next := input.Offset + len(chunks[index])
		return cluster.BrowseFetchOutputResponse{
			Data: base64.StdEncoding.EncodeToString([]byte(chunks[index])), NextOffset: next, Done: index == len(chunks)-1,
		}, nil
	}
	closeCalled := false
	closeFn := func(context.Context, string, cluster.BrowseFetchCloseRequest) error {
		closeCalled = true
		return nil
	}

	result, err := drainFederatedBrowseFetch(context.Background(), "remote-1", cluster.BrowseFetchRequest{URL: "https://example.com/"}, open, output, closeFn)
	if err != nil {
		t.Fatalf("drainFederatedBrowseFetch() error = %v", err)
	}
	if string(result.Body) != "first-second-third" || result.StatusCode != 201 || result.Headers["X-Test"][0] != "1" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if outputCalls != len(chunks) {
		t.Fatalf("output calls = %d, want %d", outputCalls, len(chunks))
	}
	if closeCalled {
		t.Fatal("Close must not be called after a successful full drain")
	}
}

func TestDrainFederatedBrowseFetchClosesSessionOnMidLoopError(t *testing.T) {
	open := func(context.Context, string, cluster.BrowseFetchRequest) (cluster.BrowseFetchOpenResponse, error) {
		return cluster.BrowseFetchOpenResponse{SessionID: "sess-2"}, nil
	}
	output := func(context.Context, string, cluster.BrowseFetchOutputRequest) (cluster.BrowseFetchOutputResponse, error) {
		return cluster.BrowseFetchOutputResponse{}, errors.New("simulated federation failure")
	}
	var closedSessionID string
	closeFn := func(_ context.Context, _ string, input cluster.BrowseFetchCloseRequest) error {
		closedSessionID = input.SessionID
		return nil
	}

	_, err := drainFederatedBrowseFetch(context.Background(), "remote-1", cluster.BrowseFetchRequest{URL: "https://example.com/"}, open, output, closeFn)
	if err == nil {
		t.Fatal("expected an error from the failing Output call")
	}
	if closedSessionID != "sess-2" {
		t.Fatalf("Close was not called with the open session, got %q", closedSessionID)
	}
}

func TestDrainFederatedBrowseFetchStopsAtIterationSafetyValve(t *testing.T) {
	open := func(context.Context, string, cluster.BrowseFetchRequest) (cluster.BrowseFetchOpenResponse, error) {
		return cluster.BrowseFetchOpenResponse{SessionID: "sess-3"}, nil
	}
	calls := 0
	output := func(_ context.Context, _ string, input cluster.BrowseFetchOutputRequest) (cluster.BrowseFetchOutputResponse, error) {
		calls++
		// Never reports Done: a misbehaving remote must not spin forever.
		return cluster.BrowseFetchOutputResponse{Data: "", NextOffset: input.Offset, Done: false}, nil
	}
	closeCalled := false
	closeFn := func(context.Context, string, cluster.BrowseFetchCloseRequest) error {
		closeCalled = true
		return nil
	}

	_, err := drainFederatedBrowseFetch(context.Background(), "remote-1", cluster.BrowseFetchRequest{URL: "https://example.com/"}, open, output, closeFn)
	if err == nil {
		t.Fatal("expected the safety valve to eventually return an error")
	}
	if calls != maxFederatedBrowseFetchIterations {
		t.Fatalf("output calls = %d, want exactly %d", calls, maxFederatedBrowseFetchIterations)
	}
	if !closeCalled {
		t.Fatal("Close must be called when the safety valve trips")
	}
}

// enableBrowsePrivateNetworks flips this host's LAN-egress opt-in the same way
// the settings endpoint does, preserving the rest of the AllowedHosts record.
func enableBrowsePrivateNetworks(t *testing.T, server *Server) {
	t.Helper()
	current, version := server.store.AllowedHosts()
	current.BrowseAllowPrivateNetworks = true
	current.UpdatedAt = time.Now().UTC()
	if err := server.store.ReplaceAllowedHosts(version, current); err != nil {
		t.Fatalf("enable browse private networks: %v", err)
	}
}

func forwardedBrowseFetchPayload(t *testing.T, agent *stubAgent) map[string]any {
	t.Helper()
	for _, call := range agent.snapshotCalls() {
		if call.path != "/v1/browse/fetch" {
			continue
		}
		var forwarded map[string]any
		if err := json.Unmarshal(call.body, &forwarded); err != nil {
			t.Fatal(err)
		}
		return forwarded
	}
	t.Fatal("handler never called the agent's browse fetch endpoint")
	return nil
}

// TestBrowseFetchForwardsThePrivateNetworkOptIn covers the local-hostID path:
// the flag must come from this host's own store, never from the request, so a
// browsed page cannot ask for the relaxation itself.
func TestBrowseFetchForwardsThePrivateNetworkOptIn(t *testing.T) {
	fetch := func(t *testing.T, optIn bool) map[string]any {
		t.Helper()
		server, tokenPath := newBrowseTestServer(t)
		sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
		if optIn {
			enableBrowsePrivateNetworks(t, server)
		}
		agentOutput, _ := json.Marshal(map[string]any{
			"statusCode": 200, "headers": map[string][]string{}, "body": "",
		})
		agent := &stubAgent{response: AgentResponse{
			StatusCode: http.StatusOK, ContentType: "application/json", Body: agentOutput,
		}}
		server.agent = agent

		body := []byte(`{"url":"https://example.com/","method":"GET"}`)
		response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/fetch", body,
			sessionCookie, csrfCookie, map[string]string{
				"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
			})
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
		}
		return forwardedBrowseFetchPayload(t, agent)
	}

	t.Run("off by default and not settable by the caller", func(t *testing.T) {
		forwarded := fetch(t, false)
		if value, present := forwarded["allowPrivateNetwork"]; present && value != false {
			t.Fatalf("allowPrivateNetwork = %v, want absent or false", value)
		}
	})

	t.Run("on when the egress host opted in", func(t *testing.T) {
		forwarded := fetch(t, true)
		if forwarded["allowPrivateNetwork"] != true {
			t.Fatalf("allowPrivateNetwork = %v, want true", forwarded["allowPrivateNetwork"])
		}
	})

	// browseFetchRequest has no allowPrivateNetwork field and decodeJSON
	// rejects unknown ones, so a browsed page cannot ask for the relaxation
	// even by hand-rolling the request body.
	t.Run("rejected when the caller tries to set it", func(t *testing.T) {
		server, tokenPath := newBrowseTestServer(t)
		sessionCookie, csrfCookie := bootstrapBrowseCookies(t, server, tokenPath)
		server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK}}
		body := []byte(`{"url":"https://example.com/","method":"GET","allowPrivateNetwork":true}`)
		response := browseAuthenticatedRequest(server, http.MethodPost, "/api/v1/browse/fetch", body,
			sessionCookie, csrfCookie, map[string]string{
				"Content-Type": "application/json", "Origin": browseTestOrigin, "X-CSRF-Token": csrfCookie.Value,
			})
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body=%s, want 400", response.Code, response.Body.String())
		}
	})
}

// TestFederatedBrowseFetchUsesTheEgressHostOwnOptIn is the point of keeping the
// flag off cluster.BrowseFetchRequest: a controlling panel sends a request with
// no such field, and the node performing the egress answers with its own
// setting.
func TestFederatedBrowseFetchUsesTheEgressHostOwnOptIn(t *testing.T) {
	server, _ := newBrowseTestServer(t)
	enableBrowsePrivateNetworks(t, server)

	agentOutput, _ := json.Marshal(map[string]any{
		"statusCode": 200, "headers": map[string][]string{}, "body": "",
	})
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json", Body: agentOutput,
	}}

	source := clusterBrowseSource{agent: agent, store: server.store}
	if _, err := source.Fetch(context.Background(), cluster.BrowseFetchRequest{
		URL: "https://example.com/", Method: http.MethodGet,
	}); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if forwarded := forwardedBrowseFetchPayload(t, agent); forwarded["allowPrivateNetwork"] != true {
		t.Fatalf("allowPrivateNetwork = %v, want true", forwarded["allowPrivateNetwork"])
	}
}

// TestDrainFederatedBrowseFetchRejectsMalformedRemoteResponses covers the four
// ways a compromised paired peer can lie about the shape of its response. The
// peer is past the Noise handshake and holds browse scope, so nothing earlier
// in the stack is going to catch this; the negative-length case in particular
// is not a failed request but a panic inside make(), which takes down paneld
// rather than the one call.
func TestDrainFederatedBrowseFetchRejectsMalformedRemoteResponses(t *testing.T) {
	const body = "payload"

	testCases := []struct {
		name       string
		totalBytes int
		chunks     []cluster.BrowseFetchOutputResponse
		wantErr    string
	}{
		{
			name:       "negative total bytes",
			totalBytes: -1,
			wantErr:    "out-of-range length",
		},
		{
			name:       "total bytes above the 8MiB ceiling",
			totalBytes: maxFederatedBrowseFetchBytes + 1,
			wantErr:    "out-of-range length",
		},
		{
			name:       "chunk offset is not contiguous",
			totalBytes: len(body),
			chunks: []cluster.BrowseFetchOutputResponse{
				{Data: base64.StdEncoding.EncodeToString([]byte(body)), NextOffset: 99, Done: true},
			},
			wantErr: "not contiguous",
		},
		{
			name:       "done with fewer bytes than declared",
			totalBytes: len(body) + 10,
			chunks: []cluster.BrowseFetchOutputResponse{
				{Data: base64.StdEncoding.EncodeToString([]byte(body)), NextOffset: len(body), Done: true},
			},
			wantErr: "declared",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			closeCalls := 0
			open := func(context.Context, string, cluster.BrowseFetchRequest) (cluster.BrowseFetchOpenResponse, error) {
				return cluster.BrowseFetchOpenResponse{SessionID: "sess-1", StatusCode: 200, TotalBytes: testCase.totalBytes}, nil
			}
			index := 0
			output := func(context.Context, string, cluster.BrowseFetchOutputRequest) (cluster.BrowseFetchOutputResponse, error) {
				if index >= len(testCase.chunks) {
					t.Fatal("drain asked for more chunks than the peer offered")
				}
				chunk := testCase.chunks[index]
				index++
				return chunk, nil
			}
			closeFn := func(context.Context, string, cluster.BrowseFetchCloseRequest) error {
				closeCalls++
				return nil
			}

			_, err := drainFederatedBrowseFetch(context.Background(), "remote-1",
				cluster.BrowseFetchRequest{URL: "https://example.com/"}, open, output, closeFn)
			if err == nil {
				t.Fatal("drainFederatedBrowseFetch() error = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, testCase.wantErr)
			}
			// Every rejection has to release the remote session, or a peer can
			// pin buffers on the far side by opening sessions that never drain.
			if closeCalls != 1 {
				t.Fatalf("Close called %d times, want exactly 1", closeCalls)
			}
		})
	}
}

// TestDrainFederatedBrowseFetchRejectsOversizedStream is the under-reporting
// variant: TotalBytes is inside the ceiling, but the peer keeps streaming.
// The declared length is a promise, so the real bound has to be enforced
// against the bytes that actually arrive.
func TestDrainFederatedBrowseFetchRejectsOversizedStream(t *testing.T) {
	chunk := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), 64<<10))
	open := func(context.Context, string, cluster.BrowseFetchRequest) (cluster.BrowseFetchOpenResponse, error) {
		return cluster.BrowseFetchOpenResponse{SessionID: "sess-1", StatusCode: 200, TotalBytes: 1}, nil
	}
	received := 0
	output := func(_ context.Context, _ string, input cluster.BrowseFetchOutputRequest) (cluster.BrowseFetchOutputResponse, error) {
		received += 64 << 10
		return cluster.BrowseFetchOutputResponse{Data: chunk, NextOffset: received, Done: false}, nil
	}
	closeCalls := 0
	closeFn := func(context.Context, string, cluster.BrowseFetchCloseRequest) error {
		closeCalls++
		return nil
	}

	_, err := drainFederatedBrowseFetch(context.Background(), "remote-1",
		cluster.BrowseFetchRequest{URL: "https://example.com/"}, open, output, closeFn)
	if err == nil || !strings.Contains(err.Error(), "maximum response size") {
		t.Fatalf("error = %v, want the maximum response size rejection", err)
	}
	if closeCalls != 1 {
		t.Fatalf("Close called %d times, want exactly 1", closeCalls)
	}
	if received > maxFederatedBrowseFetchBytes+(64<<10) {
		t.Fatalf("drain kept reading past the ceiling: %d bytes", received)
	}
}
