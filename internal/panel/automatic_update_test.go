package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAutomaticUpdateSettingsRequireSessionCSRFAndForwardTypedPolicy(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	statusBody := []byte(`{"available":true,"enabled":false,"state":"disabled","resourceVersion":"sha256:` + strings.Repeat("a", 64) + `"}`)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: statusBody}}
	server.agent = agent

	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/settings/automatic-update", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("unauthenticated status=%d calls=%#v", unauthenticated.Code, agent.snapshotCalls())
	}

	read := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodGet, "/api/v1/settings/automatic-update", nil, false)
	if read.Code != http.StatusOK || read.Body.String() != string(statusBody) {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}

	input := []byte(`{"enabled":true,"channel":"preview","expectedResourceVersion":"sha256:` + strings.Repeat("b", 64) + `"}`)
	withoutCSRF := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPut, "/api/v1/settings/automatic-update", input, false)
	if withoutCSRF.Code != http.StatusForbidden || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("missing csrf status=%d calls=%#v", withoutCSRF.Code, agent.snapshotCalls())
	}

	updated := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPut, "/api/v1/settings/automatic-update", input, true)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 2 || calls[1].method != http.MethodPut || calls[1].path != "/v1/self-update" || calls[1].rawQuery != "" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	var forwarded map[string]any
	if err := json.Unmarshal(calls[1].body, &forwarded); err != nil {
		t.Fatal(err)
	}
	if forwarded["enabled"] != true || forwarded["channel"] != "preview" ||
		forwarded["expectedResourceVersion"] != "sha256:"+strings.Repeat("b", 64) || len(forwarded) != 3 {
		t.Fatalf("unexpected forwarded policy: %#v", forwarded)
	}
	audits, _, _ := server.store.ListAudit(20, "")
	found := false
	for _, event := range audits {
		if event.Action == "settings.automatic_update.update" && event.Result == "success" {
			found = true
		}
	}
	if !found {
		t.Fatalf("automatic update policy success audit missing: %#v", audits)
	}
}

func TestAutomaticUpdateInstallIsExplicitAuditedMutation(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{
		StatusCode:  http.StatusAccepted,
		ContentType: "application/json",
		Body:        []byte(`{"state":"queued","installRequested":true}`),
	}}
	server.agent = agent
	resourceVersion := "sha256:" + strings.Repeat("d", 64)
	input := []byte(`{"expectedResourceVersion":"` + resourceVersion + `"}`)

	response := authenticatedSiteRequest(
		server,
		sessionCookie,
		csrfCookie,
		http.MethodPost,
		"/api/v1/settings/automatic-update/install",
		input,
		true,
	)
	if response.Code != http.StatusAccepted {
		t.Fatalf("install status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodPost ||
		calls[0].path != "/v1/self-update/install" || string(calls[0].body) != string(input) {
		t.Fatalf("unexpected install call: %#v", calls)
	}
	audits, _, _ := server.store.ListAudit(20, "")
	found := false
	for _, event := range audits {
		if event.Action == "settings.automatic_update.install" && event.Result == "success" {
			found = true
		}
	}
	if !found {
		t.Fatalf("automatic update install success audit missing: %#v", audits)
	}
}

func TestAutomaticUpdateCheckIsExplicitAuditedMutation(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{"state":"waiting"}`)}}
	server.agent = agent

	response := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPost, "/api/v1/settings/automatic-update/check", nil, true)
	if response.Code != http.StatusOK {
		t.Fatalf("check status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodPost || calls[0].path != "/v1/self-update/check" || len(calls[0].body) != 0 {
		t.Fatalf("unexpected check call: %#v", calls)
	}
	audits, _, _ := server.store.ListAudit(20, "")
	found := false
	for _, event := range audits {
		if event.Action == "settings.automatic_update.check" && event.Result == "success" {
			found = true
		}
	}
	if !found {
		t.Fatalf("automatic update check success audit missing: %#v", audits)
	}
}

func TestKPanelReleaseSummaryIsReadOnlyAndForwardsValidatedChannel(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	body := []byte(`{"channel":"stable","version":"1.2.0","cached":true,"stale":false}`)
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}}
	server.agent = agent

	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/settings/kpanel-release?channel=stable", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("unauthenticated status=%d calls=%#v", unauthenticated.Code, agent.snapshotCalls())
	}
	response := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodGet,
		"/api/v1/settings/kpanel-release?channel=stable", nil, false,
	)
	if response.Code != http.StatusOK || response.Body.String() != string(body) {
		t.Fatalf("release status=%d body=%s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodGet ||
		calls[0].path != "/v1/self-update/release" || calls[0].rawQuery != "channel=stable" {
		t.Fatalf("release calls=%#v", calls)
	}

	invalid := authenticatedSiteRequest(
		server, sessionCookie, csrfCookie, http.MethodGet,
		"/api/v1/settings/kpanel-release?channel=beta", nil, false,
	)
	if invalid.Code != http.StatusUnprocessableEntity || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("invalid status=%d calls=%#v", invalid.Code, agent.snapshotCalls())
	}
}

func TestAutomaticUpdateSettingsRejectUnknownOrQueriedInputs(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK}}
	version := "sha256:" + strings.Repeat("c", 64)

	unknown := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodPut, "/api/v1/settings/automatic-update", []byte(`{"enabled":true,"expectedResourceVersion":"`+version+`","channel":"beta"}`), true)
	if unknown.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown field status=%d body=%s", unknown.Code, unknown.Body.String())
	}
	queried := authenticatedSiteRequest(server, sessionCookie, csrfCookie, http.MethodGet, "/api/v1/settings/automatic-update?channel=beta", nil, false)
	if queried.Code != http.StatusNotFound {
		t.Fatalf("queried status=%d body=%s", queried.Code, queried.Body.String())
	}
}
