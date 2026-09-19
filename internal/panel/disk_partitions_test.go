package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiskPartitionGetUsesExactAuthenticatedAgentPath(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body: []byte(`{"resourceVersion":"` + strings.Repeat("a", 64) +
			`","platform":{"kind":"linux","label":"Linux","writable":true},"devices":[],"observedAt":"2026-08-23T00:00:00Z"}`),
	}}
	server.agent = agent

	unauthenticated := httptest.NewRequest(http.MethodGet, "/api/v1/system/disk-partitions", nil)
	unauthenticated.Host = "panel.test"
	unauthenticatedResponse := httptest.NewRecorder()
	server.ServeHTTP(unauthenticatedResponse, unauthenticated)
	if unauthenticatedResponse.Code != http.StatusUnauthorized || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("unauthenticated status=%d calls=%#v", unauthenticatedResponse.Code, agent.snapshotCalls())
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/disk-partitions", nil)
	request.Host = "panel.test"
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodGet ||
		calls[0].path != "/v1/system/disk-partitions" || calls[0].rawQuery != "" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}

	for _, test := range []struct {
		name    string
		path    string
		rawPath string
	}{
		{name: "query", path: "/api/v1/system/disk-partitions?refresh=1"},
		{
			name: "raw path", path: "/api/v1/system/disk-partitions",
			rawPath: "/api/v1/system/%64isk-partitions",
		},
		{name: "trailing slash", path: "/api/v1/system/disk-partitions/"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Host = "panel.test"
			request.AddCookie(sessionCookie)
			request.AddCookie(csrfCookie)
			if test.rawPath != "" {
				request.URL.RawPath = test.rawPath
			}
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != http.StatusNotFound || len(agent.snapshotCalls()) != 1 {
				t.Fatalf("status=%d body=%s calls=%#v", response.Code, response.Body.String(), agent.snapshotCalls())
			}
		})
	}
}

func TestDiskPartitionWriteRequiresSecurityForwards202AndRedactsAudit(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	deviceID := strings.Repeat("0123456789abcdef", 4)
	version := strings.Repeat("b", 64)
	mountPoint := "/mnt/private-audit-secret-storage-9381"
	jobBody := []byte(`{"id":"0123456789abcdef0123456789abcdef","action":"mount","deviceId":"` + deviceID +
		`","devicePath":"/dev/sdb1","status":"queued","stage":"launching","progress":2,"message":"queued","createdAt":"2026-08-23T00:00:00Z"}`)
	agent := &stubAgent{response: AgentResponse{
		StatusCode: http.StatusAccepted, ContentType: "application/json", Body: jobBody,
	}}
	server.agent = agent
	body := []byte(`{"action":"mount","deviceId":"` + deviceID + `","expectedResourceVersion":"` + version +
		`","mountPoint":"` + mountPoint + `","persist":false}`)

	unauthenticated := httptest.NewRequest(http.MethodPost, "/api/v1/system/disk-partition-actions", strings.NewReader(string(body)))
	unauthenticated.Host = "panel.test"
	unauthenticated.Header.Set("Origin", "http://panel.test")
	unauthenticated.Header.Set("Content-Type", "application/json")
	unauthenticatedResponse := httptest.NewRecorder()
	server.ServeHTTP(unauthenticatedResponse, unauthenticated)
	if unauthenticatedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticatedResponse.Code, unauthenticatedResponse.Body.String())
	}

	missingOriginRequest := newAuthenticatedSiteRequest(
		sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/disk-partition-actions", body, true,
	)
	missingOriginRequest.Header.Del("Origin")
	missingOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(missingOriginResponse, missingOriginRequest)
	if missingOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("missing Origin status=%d body=%s", missingOriginResponse.Code, missingOriginResponse.Body.String())
	}

	crossOriginRequest := newAuthenticatedSiteRequest(
		sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/disk-partition-actions", body, true,
	)
	crossOriginRequest.Header.Set("Origin", "https://evil.example")
	crossOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(crossOriginResponse, crossOriginRequest)
	if crossOriginResponse.Code != http.StatusForbidden {
		t.Fatalf("cross Origin status=%d body=%s", crossOriginResponse.Code, crossOriginResponse.Body.String())
	}

	missingCSRF := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/disk-partition-actions", body, false,
	)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d body=%s", missingCSRF.Code, missingCSRF.Body.String())
	}

	query := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost,
		"/api/v1/system/disk-partition-actions?force=1", body, true,
	)
	if query.Code != http.StatusNotFound {
		t.Fatalf("query status=%d body=%s", query.Code, query.Body.String())
	}

	rawPathRequest := newAuthenticatedSiteRequest(
		sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/disk-partition-actions", body, true,
	)
	rawPathRequest.URL.RawPath = "/api/v1/system/disk-partition-%61ctions"
	rawPathResponse := httptest.NewRecorder()
	server.ServeHTTP(rawPathResponse, rawPathRequest)
	if rawPathResponse.Code != http.StatusNotFound {
		t.Fatalf("raw path status=%d body=%s", rawPathResponse.Code, rawPathResponse.Body.String())
	}

	unknown := []byte(`{"action":"check","deviceId":"` + deviceID + `","expectedResourceVersion":"` + version +
		`","unknown":true}`)
	unknownResponse := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/disk-partition-actions", unknown, true,
	)
	if unknownResponse.Code != http.StatusBadRequest || !strings.Contains(unknownResponse.Body.String(), "invalid_json") {
		t.Fatalf("unknown field status=%d body=%s", unknownResponse.Code, unknownResponse.Body.String())
	}

	inapplicable := []byte(`{"action":"mount","deviceId":"` + deviceID + `","expectedResourceVersion":"` + version +
		`","mountPoint":"/mnt/data","removePersistence":false}`)
	inapplicableResponse := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/disk-partition-actions", inapplicable, true,
	)
	if inapplicableResponse.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(inapplicableResponse.Body.String(), "removePersistence") {
		t.Fatalf("inapplicable field status=%d body=%s", inapplicableResponse.Code, inapplicableResponse.Body.String())
	}
	if calls := agent.snapshotCalls(); len(calls) != 0 {
		t.Fatalf("rejected requests reached Agent: %#v", calls)
	}

	accepted := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/system/disk-partition-actions", body, true,
	)
	if accepted.Code != http.StatusAccepted || accepted.Header().Get("Content-Type") != "application/json" ||
		accepted.Body.String() != string(jobBody) {
		t.Fatalf("accepted status=%d type=%q body=%s", accepted.Code, accepted.Header().Get("Content-Type"), accepted.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodPost ||
		calls[0].path != "/v1/system/disk-partition-actions" || calls[0].rawQuery != "" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	var forwarded map[string]any
	if err := json.Unmarshal(calls[0].body, &forwarded); err != nil {
		t.Fatalf("decode forwarded body: %v", err)
	}
	if forwarded["deviceId"] != deviceID || forwarded["mountPoint"] != mountPoint || forwarded["persist"] != false {
		t.Fatalf("unexpected forwarded body: %#v", forwarded)
	}

	events, _, _ := server.store.ListAudit(100, "")
	results := make(map[string]int)
	var serialized strings.Builder
	for _, event := range events {
		if event.Action != "system.disk-partition.mount" {
			continue
		}
		results[event.Result]++
		if event.TargetID != deviceID[:12] {
			t.Fatalf("unsafe target ID: %q", event.TargetID)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		serialized.Write(encoded)
	}
	if results["intent"] != 1 || results["success"] != 1 || len(results) != 2 {
		t.Fatalf("audit results=%v want intent/success", results)
	}
	auditText := serialized.String()
	if strings.Contains(auditText, mountPoint) || strings.Contains(auditText, deviceID) {
		t.Fatalf("audit leaked mount point or complete device ID: %s", auditText)
	}
	for _, required := range []string{"mountPointHash", "mountPointLength", "deviceIdPrefix", deviceID[:12]} {
		if !strings.Contains(auditText, required) {
			t.Fatalf("audit metadata %q missing: %s", required, auditText)
		}
	}
}
