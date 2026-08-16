package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func TestClusterHostsRequireSessionAndIncludeLocalHost(t *testing.T) {
	server, tokenPath := newTestServer(t)

	unauthenticated := performRequest(
		server, http.MethodGet, "/api/v1/cluster/hosts", nil, nil,
	)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf(
			"unauthenticated cluster hosts status = %d, want %d; body=%s",
			unauthenticated.Code, http.StatusUnauthorized, unauthenticated.Body.String(),
		)
	}

	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	response := authenticatedRequest(
		server, http.MethodGet, "/api/v1/cluster/hosts", nil,
		sessionCookie, csrfCookie, nil,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("cluster hosts status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var inventory cluster.HostList
	if err := json.Unmarshal(response.Body.Bytes(), &inventory); err != nil {
		t.Fatalf("decode cluster hosts: %v", err)
	}
	if inventory.Total != 1 || inventory.RemoteTotal != 0 || len(inventory.Items) != 1 {
		t.Fatalf("unexpected local-only inventory: %#v", inventory)
	}
	local := inventory.Items[0]
	if local.ID != cluster.LocalHostID || !local.IsLocal {
		t.Fatalf("local host marker missing: %#v", local)
	}
	if local.Origin != "" {
		t.Fatalf("local host unexpectedly exposes a remote origin: %#v", local)
	}

	detail := authenticatedRequest(
		server, http.MethodGet, "/api/v1/cluster/hosts/local", nil,
		sessionCookie, csrfCookie, nil,
	)
	if detail.Code != http.StatusOK {
		t.Fatalf("local host detail status = %d, want %d; body=%s", detail.Code, http.StatusOK, detail.Body.String())
	}
	var localDetail cluster.Host
	if err := json.Unmarshal(detail.Body.Bytes(), &localDetail); err != nil {
		t.Fatalf("decode local host detail: %v", err)
	}
	if localDetail.ID != cluster.LocalHostID || !localDetail.IsLocal {
		t.Fatalf("local host detail marker missing: %#v", localDetail)
	}
}

func TestClusterMutationsRequireOriginAndCSRF(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	missingOrigin := authenticatedRequest(
		server, http.MethodPost, "/api/v1/cluster/pairing-codes", nil,
		sessionCookie, csrfCookie, map[string]string{
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if missingOrigin.Code != http.StatusForbidden ||
		!strings.Contains(missingOrigin.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing Origin returned %d %s", missingOrigin.Code, missingOrigin.Body.String())
	}

	missingCSRF := authenticatedRequest(
		server, http.MethodPost, "/api/v1/cluster/pairing-codes", nil,
		sessionCookie, csrfCookie, map[string]string{
			"Origin": "http://panel.test",
		},
	)
	if missingCSRF.Code != http.StatusForbidden ||
		!strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF returned %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}
}

func TestClusterLocalHostCanBeRenamed(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	list := authenticatedRequest(
		server, http.MethodGet, "/api/v1/cluster/hosts", nil,
		sessionCookie, csrfCookie, nil,
	)
	if list.Code != http.StatusOK {
		t.Fatalf("cluster hosts status = %d; body=%s", list.Code, list.Body.String())
	}
	var inventory cluster.HostList
	if err := json.Unmarshal(list.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(cluster.UpdateHostInput{
		Name: "控制中心", ExpectedResourceVersion: inventory.Items[0].ResourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(
		server, http.MethodPatch, "/api/v1/cluster/hosts/local", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json",
			"Origin":       "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusOK {
		t.Fatalf("local rename returned %d %s", response.Code, response.Body.String())
	}
	var renamed cluster.Host
	if err := json.Unmarshal(response.Body.Bytes(), &renamed); err != nil {
		t.Fatal(err)
	}
	if !renamed.IsLocal || renamed.Name != "控制中心" {
		t.Fatalf("unexpected renamed local host: %#v", renamed)
	}
}

func TestClusterLocalHostCannotBeDeleted(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	list := authenticatedRequest(
		server, http.MethodGet, "/api/v1/cluster/hosts", nil,
		sessionCookie, csrfCookie, nil,
	)
	if list.Code != http.StatusOK {
		t.Fatalf("cluster hosts status = %d; body=%s", list.Code, list.Body.String())
	}
	var inventory cluster.HostList
	if err := json.Unmarshal(list.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	if len(inventory.Items) == 0 {
		t.Fatal("local host is missing")
	}
	body, err := json.Marshal(cluster.DeleteHostInput{
		ExpectedResourceVersion: inventory.Items[0].ResourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(
		server, http.MethodDelete, "/api/v1/cluster/hosts/local", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json",
			"Origin":       "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusConflict ||
		!strings.Contains(response.Body.String(), `"cluster_local_host"`) {
		t.Fatalf("local delete returned %d %s", response.Code, response.Body.String())
	}
}

func TestClusterPairingCodeSecretIsNotAudited(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	response := authenticatedRequest(
		server, http.MethodPost, "/api/v1/cluster/pairing-codes/v2", nil,
		sessionCookie, csrfCookie, map[string]string{
			"Origin":       "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("pairing code status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var code cluster.PairingCode
	if err := json.Unmarshal(response.Body.Bytes(), &code); err != nil {
		t.Fatalf("decode pairing code: %v", err)
	}
	if code.Code == "" {
		t.Fatal("pairing code response is empty")
	}
	if !strings.HasPrefix(code.Code, "kp2.") {
		t.Fatalf("pairing code does not use the encrypted v2 protocol: %q", code.Code)
	}
	if code.Scope != cluster.SummaryTerminalScope {
		t.Fatalf("omitted grants must default to today's behavior (terminal, no browse); got scope %q", code.Scope)
	}

	events, _ := server.store.ListAudit(200, "")
	serialized, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal audit events: %v", err)
	}
	if strings.Contains(string(serialized), code.Code) {
		t.Fatalf("pairing code leaked into audit events: %s", serialized)
	}
	intent, success := false, false
	for _, event := range events {
		if event.Action != "cluster.pairing-code.create" {
			continue
		}
		if event.Change != nil {
			t.Fatalf("pairing code audit contains change data: %#v", event)
		}
		intent = intent || event.Result == "intent"
		success = success || event.Result == "success"
	}
	if !intent || !success {
		t.Fatalf("pairing code audit intent/success missing: %#v", events)
	}
}

func TestClusterPairingCodeV2AcceptsExplicitGrants(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	post := func(t *testing.T, body string) cluster.PairingCode {
		t.Helper()
		response := authenticatedRequest(
			server, http.MethodPost, "/api/v1/cluster/pairing-codes/v2", []byte(body),
			sessionCookie, csrfCookie, map[string]string{
				"Content-Type": "application/json",
				"Origin":       "http://panel.test",
				"X-CSRF-Token": csrfCookie.Value,
			},
		)
		if response.Code != http.StatusCreated {
			t.Fatalf("pairing code status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
		}
		var code cluster.PairingCode
		if err := json.Unmarshal(response.Body.Bytes(), &code); err != nil {
			t.Fatalf("decode pairing code: %v", err)
		}
		return code
	}

	if code := post(t, `{"browseFetch":true}`); code.Scope != "cluster.summary.read cluster.terminal.open cluster.browse.fetch" {
		t.Fatalf(`{"browseFetch":true} scope = %q`, code.Scope)
	}
	if code := post(t, `{"terminal":false}`); code.Scope != cluster.SummaryScope {
		t.Fatalf(`{"terminal":false} scope = %q`, code.Scope)
	}
	if code := post(t, `{"terminal":false,"browseFetch":true}`); code.Scope != "cluster.summary.read cluster.browse.fetch" {
		t.Fatalf(`{"terminal":false,"browseFetch":true} scope = %q`, code.Scope)
	}
	if code := post(t, `{"browseWs":true}`); code.Scope != "cluster.summary.read cluster.terminal.open cluster.browse.ws" {
		t.Fatalf(`{"browseWs":true} scope = %q`, code.Scope)
	}
	if code := post(t, `{"terminal":false,"browseFetch":true,"browseWs":true}`); code.Scope != "cluster.summary.read cluster.browse.fetch cluster.browse.ws" {
		t.Fatalf(`{"terminal":false,"browseFetch":true,"browseWs":true} scope = %q`, code.Scope)
	}
	if code := post(t, `{}`); code.Scope != cluster.SummaryTerminalScope {
		t.Fatalf(`{} scope = %q, want default terminal-only`, code.Scope)
	}
}

func TestLightNodeEnrollmentUsesAuthenticatedIntentAndPublicOneUseExchange(t *testing.T) {
	server, tokenPath := newTestServerWithPublicURL(t, "https://panel.test")
	sessionCookie, csrfCookie := bootstrapCookiesForOrigin(t, server, tokenPath, "https://panel.test")

	created := authenticatedRequest(
		server, http.MethodPost, "/api/v1/cluster/light-enrollments", nil,
		sessionCookie, csrfCookie, map[string]string{
			"Origin": "https://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if created.Code != http.StatusCreated {
		t.Fatalf("light enrollment status = %d; body=%s", created.Code, created.Body.String())
	}
	var enrollment cluster.LightEnrollment
	if err := json.Unmarshal(created.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	if len(fields) == 0 || !strings.Contains(enrollment.Command, "kpanel node join 'kpl1.") {
		t.Fatalf("unexpected light enrollment command: %q", enrollment.Command)
	}
	token := strings.Trim(fields[len(fields)-1], "'")
	requestBody, err := json.Marshal(cluster.LightEnrollRequest{Token: token, Name: "edge-1", NodeVersion: "0.40.0"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "https://panel.test/api/v3/federation/light/enroll", strings.NewReader(string(requestBody)))
	request.Host = "panel.test"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("public light enrollment status = %d; body=%s", response.Code, response.Body.String())
	}
	var enrolled cluster.LightEnrollResponse
	if err := json.Unmarshal(response.Body.Bytes(), &enrolled); err != nil {
		t.Fatal(err)
	}
	if enrolled.NodeID == "" || enrolled.ReportingKey == "" || enrolled.ReportInterval != 30 {
		t.Fatalf("unexpected light enrollment response: %#v", enrolled)
	}

	replay := httptest.NewRequest(http.MethodPost, "https://panel.test/api/v3/federation/light/enroll", strings.NewReader(string(requestBody)))
	replay.Host = "panel.test"
	replay.Header.Set("Content-Type", "application/json")
	replayResponse := httptest.NewRecorder()
	server.ServeHTTP(replayResponse, replay)
	if replayResponse.Code != http.StatusUnauthorized {
		t.Fatalf("reused enrollment status = %d; body=%s", replayResponse.Code, replayResponse.Body.String())
	}

	inventoryResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/cluster/hosts", nil,
		sessionCookie, csrfCookie, nil,
	)
	var inventory cluster.HostList
	if err := json.Unmarshal(inventoryResponse.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, host := range inventory.Items {
		if host.ID == enrolled.NodeID && host.Kind == cluster.HostKindLightNode && host.Origin == "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("enrolled light host missing from inventory: %#v", inventory)
	}

	events, _ := server.store.ListAudit(200, "")
	serialized, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{token, enrollment.Command, enrolled.ReportingKey} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("light enrollment secret leaked into audit: %s", serialized)
		}
	}
}

func TestLightNodeEnrollmentUsesTrustedHTTPSProxyOrigin(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/light-enrollments", nil)
	request.Host = "panel.example"
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("Origin", "https://panel.example")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-CSRF-Token", csrfCookie.Value)
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	created := httptest.NewRecorder()
	server.ServeHTTP(created, request)
	if created.Code != http.StatusCreated {
		t.Fatalf("proxied light enrollment status = %d; body=%s", created.Code, created.Body.String())
	}
	var enrollment cluster.LightEnrollment
	if err := json.Unmarshal(created.Body.Bytes(), &enrollment); err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	token := strings.Trim(fields[len(fields)-1], "'")
	body, err := json.Marshal(cluster.LightEnrollRequest{Token: token, Name: "edge-proxy", NodeVersion: "0.40.0"})
	if err != nil {
		t.Fatal(err)
	}
	enrollRequest := httptest.NewRequest(http.MethodPost, "/api/v3/federation/light/enroll", strings.NewReader(string(body)))
	enrollRequest.Host = "panel.example"
	enrollRequest.RemoteAddr = "127.0.0.1:23456"
	enrollRequest.Header.Set("Content-Type", "application/json")
	enrollRequest.Header.Set("X-Forwarded-Proto", "https")
	enrolled := httptest.NewRecorder()
	server.ServeHTTP(enrolled, enrollRequest)
	if enrolled.Code != http.StatusCreated {
		t.Fatalf("proxied public light enrollment status = %d; body=%s", enrolled.Code, enrolled.Body.String())
	}
}

func TestLightNodeEnrollmentRejectsUntrustedForwardedHTTPS(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/cluster/light-enrollments", nil)
	request.Host = "panel.test"
	request.RemoteAddr = "198.51.100.10:12345"
	request.Header.Set("Origin", "http://panel.test")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-CSRF-Token", csrfCookie.Value)
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(response.Body.String(), "cluster_light_https_required") {
		t.Fatalf("untrusted forwarded HTTPS returned %d %s", response.Code, response.Body.String())
	}
}

func TestLightNodePublicEndpointsKeepHostAndBodyGuards(t *testing.T) {
	server, _ := newTestServerWithPublicURL(t, "https://panel.test")
	for _, test := range []struct {
		name   string
		target string
		host   string
		body   string
		status int
	}{
		{
			name: "untrusted host", target: "https://evil.example/api/v3/federation/light/enroll",
			host: "evil.example", body: `{}`, status: http.StatusMisdirectedRequest,
		},
		{
			name: "query rejected", target: "https://panel.test/api/v3/federation/light/enroll?unexpected=1",
			host: "panel.test", body: `{}`, status: http.StatusBadRequest,
		},
		{
			name: "oversized report", target: "https://panel.test/api/v3/federation/light/report",
			host: "panel.test", body: strings.Repeat("x", cluster.MaxSummaryBytes+1), status: http.StatusRequestEntityTooLarge,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.target, strings.NewReader(test.body))
			request.Host = test.host
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}
}

func TestLegacyPairingCodeAPIStillIssuesV1Code(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	response := authenticatedRequest(
		server,
		http.MethodPost,
		"/api/v1/cluster/pairing-codes",
		nil,
		sessionCookie,
		csrfCookie,
		map[string]string{
			"Origin":       "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("legacy pairing code status = %d; body=%s", response.Code, response.Body.String())
	}
	var code cluster.PairingCode
	if err := json.Unmarshal(response.Body.Bytes(), &code); err != nil {
		t.Fatalf("decode legacy pairing code: %v", err)
	}
	if len(code.Code) != 81 || strings.HasPrefix(code.Code, "kp2.") {
		t.Fatalf("legacy pairing endpoint returned an incompatible code: %q", code.Code)
	}
}

func TestFederationV2BypassesPublicHostCheckButStillAuthenticates(t *testing.T) {
	server, _ := newTestServer(t)
	request := httptest.NewRequest(
		http.MethodPost,
		"http://8.8.8.8:1801/api/v2/federation/pair",
		strings.NewReader(`{}`),
	)
	request.Host = "8.8.8.8:1801"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf(
			"unauthenticated v2 federation status = %d, want %d; body=%s",
			response.Code,
			http.StatusUnauthorized,
			response.Body.String(),
		)
	}
	if strings.Contains(response.Body.String(), "host_header_rejected") {
		t.Fatalf("v2 federation request was rejected by the browser Host policy: %s", response.Body.String())
	}
}

func TestFederationV2RejectsWrongMethodAndQuery(t *testing.T) {
	server, _ := newTestServer(t)
	for _, test := range []struct {
		name   string
		method string
		target string
		status int
	}{
		{
			name:   "wrong method",
			method: http.MethodGet,
			target: "http://8.8.8.8:1801/api/v2/federation/pair",
			status: http.StatusMisdirectedRequest,
		},
		{
			name:   "query",
			method: http.MethodPost,
			target: "http://8.8.8.8:1801/api/v2/federation/pair?unexpected=1",
			status: http.StatusBadRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, strings.NewReader(`{}`))
			request.Host = "8.8.8.8:1801"
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			server.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf(
					"invalid v2 federation status = %d, want %d; body=%s",
					response.Code,
					test.status,
					response.Body.String(),
				)
			}
		})
	}
}

func TestFederationV2RejectsOversizedEnvelope(t *testing.T) {
	server, _ := newTestServer(t)
	request := httptest.NewRequest(
		http.MethodPost,
		"http://8.8.8.8:1801/api/v2/federation/pair",
		strings.NewReader(
			`{"message":"`+
				strings.Repeat("x", cluster.MaxFederationV2Bytes+1)+
				`"}`,
		),
	)
	request.Host = "8.8.8.8:1801"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	server.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf(
			"oversized v2 federation status = %d, want %d; body=%s",
			response.Code,
			http.StatusRequestEntityTooLarge,
			response.Body.String(),
		)
	}
}
