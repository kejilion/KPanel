package panel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

// The browse tests run against a loopback public origin. Nothing in the
// handlers requires it — the server does no secure-context check (see the
// note in browse.go) — it simply mirrors the origin a developer actually
// reaches these endpoints from.
const browseTestOrigin = "http://localhost"
const browseTestHost = "localhost"

func newBrowseTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	return newTestServerWithPublicURL(t, browseTestOrigin)
}

func bootstrapBrowseCookies(t *testing.T, server *Server, tokenPath string) (*http.Cookie, *http.Cookie) {
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
		"Content-Type": "application/json", "Origin": browseTestOrigin,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("bootstrap failed: %d %s", response.Code, response.Body.String())
	}
	return authCookies(t, response)
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
		"Content-Type": "application/json", "Origin": "http://localhost",
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
			"Content-Type": "application/json", "Origin": "http://localhost",
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
			"Content-Type": "application/json", "Origin": "http://localhost", "X-CSRF-Token": csrfCookie.Value,
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
			"Content-Type": "application/json", "Origin": "http://localhost", "X-CSRF-Token": csrfCookie.Value,
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
			"Content-Type": "application/json", "Origin": "http://localhost", "X-CSRF-Token": csrfCookie.Value,
		})
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleBrowseSessionStartRejectsUnauthenticated(t *testing.T) {
	server, _ := newBrowseTestServer(t)
	body, _ := json.Marshal(browseSessionStartRequest{})
	response := browsePerformRequest(server, http.MethodPost, "/api/v1/browse/sessions", body, map[string]string{
		"Content-Type": "application/json", "Origin": "http://localhost",
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
			"Content-Type": "application/json", "Origin": "http://localhost", "X-CSRF-Token": csrfCookie.Value,
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
			"Content-Type": "application/json", "Origin": "http://localhost", "X-CSRF-Token": csrfCookie.Value,
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
				"Content-Type": "application/json", "Origin": "http://localhost", "X-CSRF-Token": csrfCookie.Value,
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
			"Content-Type": "application/json", "Origin": "http://localhost", "X-CSRF-Token": csrfCookie.Value,
		})
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestDrainFederatedBrowseFetchAssemblesMultipleChunks(t *testing.T) {
	chunks := []string{"first-", "second-", "third"}
	var outputCalls int
	open := func(context.Context, string, cluster.BrowseFetchRequest) (cluster.BrowseFetchOpenResponse, error) {
		return cluster.BrowseFetchOpenResponse{SessionID: "sess-1", StatusCode: 201, Headers: map[string][]string{"X-Test": {"1"}}, TotalBytes: 16}, nil
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
