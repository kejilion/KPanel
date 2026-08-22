package panel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/store"
)

const testFileShareVersion = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testFileShareStrongVersion = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
const testFileShareStrongVersion2 = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

func TestFileShareLifecycleUsesOneGenericPublicLink(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{
		stubAgent:    &stubAgent{response: fileShareEntryResponse("/srv/public/photo.png", "photo.png", "image/png", 128)},
		streamStatus: http.StatusOK,
		streamHeaders: http.Header{
			"Content-Type":        []string{"image/png"},
			"Content-Disposition": []string{`inline; filename="photo.png"`},
			"ETag":                []string{`"` + testFileShareVersion + `"`},
			"Accept-Ranges":       []string{"bytes"},
		},
		streamResponse: []byte("image-bytes"),
	}
	server.agent = agent

	body, _ := json.Marshal(fileShareCreateInput{
		Path: "/srv/public/photo.png", ExpectedResourceVersion: testFileShareVersion, ExpiresIn: "7d",
	})
	unauthenticated := performRequest(server, http.MethodPost, fileSharesAdminPath, body, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated create = %d", unauthenticated.Code)
	}

	createdResponse := createPanelFileShare(t, server, sessionCookie, csrfCookie, body)
	if !createdResponse.LinksAvailable || createdResponse.SharePath == "" || createdResponse.DirectPath == "" {
		t.Fatalf("created response did not return one-time links: %#v", createdResponse)
	}
	token := strings.TrimPrefix(createdResponse.SharePath, fileSharePagePrefix)
	if !validFileShareToken(token) || createdResponse.DirectPath != fileShareRawPrefix+token {
		t.Fatalf("invalid share paths: %#v", createdResponse)
	}
	unauthenticatedList := performRequest(server, http.MethodGet, fileSharesAdminPath, nil, nil)
	if unauthenticatedList.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d", unauthenticatedList.Code)
	}
	list := authenticatedRequest(
		server, http.MethodGet, fileSharesAdminPath, nil, sessionCookie, csrfCookie, nil,
	)
	if list.Code != http.StatusOK {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	var listPayload fileShareListResponse
	if err := json.Unmarshal(list.Body.Bytes(), &listPayload); err != nil {
		t.Fatal(err)
	}
	if len(listPayload.Shares) != 1 || listPayload.Shares[0].ID != createdResponse.ID ||
		listPayload.Shares[0].Path != "/srv/public/photo.png" || listPayload.Shares[0].LinksAvailable ||
		listPayload.Shares[0].SharePath != "" || listPayload.Shares[0].DirectPath != "" {
		t.Fatalf("admin list leaked or lost share state: %#v", listPayload.Shares)
	}

	query := url.Values{
		"path":            []string{"/srv/public/photo.png"},
		"resourceVersion": []string{testFileShareVersion},
	}
	lookup := authenticatedRequest(
		server, http.MethodGet, fileSharesAdminPath+"?"+query.Encode(), nil,
		sessionCookie, csrfCookie, nil,
	)
	if lookup.Code != http.StatusOK {
		t.Fatalf("lookup = %d %s", lookup.Code, lookup.Body.String())
	}
	var lookupPayload fileShareLookupResponse
	if err := json.Unmarshal(lookup.Body.Bytes(), &lookupPayload); err != nil {
		t.Fatal(err)
	}
	if lookupPayload.Share == nil || lookupPayload.Share.ID != createdResponse.ID ||
		lookupPayload.Share.LinksAvailable || lookupPayload.Share.SharePath != "" || lookupPayload.Share.DirectPath != "" {
		t.Fatalf("persisted lookup leaked or lost link state: %#v", lookupPayload.Share)
	}

	metadata := performRequest(server, http.MethodGet, fileShareAPIPrefix+token, nil, nil)
	if metadata.Code != http.StatusOK {
		t.Fatalf("public metadata = %d %s", metadata.Code, metadata.Body.String())
	}
	var publicPayload map[string]json.RawMessage
	if err := json.Unmarshal(metadata.Body.Bytes(), &publicPayload); err != nil {
		t.Fatal(err)
	}
	assertJSONKeys(t, publicPayload, "name", "mime", "sizeBytes", "expiresAt", "directPath", "downloadPath")
	for _, forbidden := range []string{"path", "resourceVersion", "owner", "group", "mode", "id", "token"} {
		if _, exists := publicPayload[forbidden]; exists {
			t.Fatalf("public metadata contains private field %q: %s", forbidden, metadata.Body.String())
		}
	}
	agent.stubAgent.mu.Lock()
	originalEntryBody := append([]byte(nil), agent.stubAgent.response.Body...)
	agent.stubAgent.response.Body = []byte(strings.ReplaceAll(
		string(agent.stubAgent.response.Body), testFileShareStrongVersion, testFileShareStrongVersion2,
	))
	agent.stubAgent.mu.Unlock()
	metadataAfterStrongOnlyChange := performRequest(server, http.MethodGet, fileShareAPIPrefix+token, nil, nil)
	if metadataAfterStrongOnlyChange.Code != http.StatusOK {
		t.Fatalf("cheap public metadata rejected same-metadata rewrite: %d %s", metadataAfterStrongOnlyChange.Code, metadataAfterStrongOnlyChange.Body.String())
	}
	agent.stubAgent.mu.Lock()
	agent.stubAgent.response.Body = originalEntryBody
	agent.stubAgent.mu.Unlock()
	server.fileShareValidationMu.Lock()
	metadataCost := server.fileShareGlobalCost.units
	server.fileShareValidationMu.Unlock()
	if metadataCost != 0 {
		t.Fatalf("public metadata consumed strong-validation budget: %d", metadataCost)
	}

	direct := performRequest(server, http.MethodGet, createdResponse.DirectPath, nil, map[string]string{
		"Range": "bytes=0-4", "If-Match": `"attacker-controlled"`,
		"X-KPanel-File-Share-Version": "sha256:" + strings.Repeat("0", 64),
	})
	if direct.Code != http.StatusOK || direct.Body.String() != "image-bytes" {
		t.Fatalf("direct response = %d %q", direct.Code, direct.Body.String())
	}
	if direct.Header().Get("Cross-Origin-Resource-Policy") != "cross-origin" ||
		direct.Header().Get("Access-Control-Allow-Origin") != "" ||
		direct.Header().Get("Cache-Control") != "public, max-age=0, must-revalidate" ||
		direct.Header().Get("Content-Type") != "image/png" ||
		!strings.HasPrefix(direct.Header().Get("Content-Disposition"), "inline") ||
		direct.Header().Get("Content-Security-Policy") != fileShareContentSecurityPolicy {
		t.Fatalf("direct sharing headers = %#v", direct.Header())
	}
	streamCalls := agent.snapshotStreamCalls()
	if len(streamCalls) != 1 || streamCalls[0].path != "/v1/files/share-content" ||
		streamCalls[0].headers.Get("If-Match") != `"`+testFileShareVersion+`"` ||
		streamCalls[0].headers.Get("X-KPanel-File-Share-Version") != testFileShareStrongVersion ||
		streamCalls[0].headers.Get("Range") != "bytes=0-4" {
		t.Fatalf("unexpected public stream call: %#v", streamCalls)
	}
	head := performRequest(server, http.MethodHead, createdResponse.DirectPath+"?download=1", nil, nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("direct HEAD = %d %q", head.Code, head.Body.String())
	}
	streamCalls = agent.snapshotStreamCalls()
	if !strings.Contains(streamCalls[len(streamCalls)-1].rawQuery, "disposition=attachment") {
		t.Fatalf("download disposition was not forwarded: %#v", streamCalls[len(streamCalls)-1])
	}
	if !strings.HasPrefix(head.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download response was not forced to attachment: %#v", head.Header())
	}
	agent.streamStatus = http.StatusNotModified
	notModified := performRequest(server, http.MethodGet, createdResponse.DirectPath, nil, map[string]string{
		"If-None-Match": `"` + testFileShareVersion + `"`,
	})
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 ||
		notModified.Header().Get("Cross-Origin-Resource-Policy") != "cross-origin" {
		t.Fatalf("direct 304 = %d headers=%#v body=%q", notModified.Code, notModified.Header(), notModified.Body.String())
	}
	server.fileShareValidationMu.Lock()
	contentCost := server.fileShareValidations["/srv/public/photo.png"].units
	server.fileShareValidationMu.Unlock()
	if contentCost != 3 {
		t.Fatalf("Range, HEAD, and 304 validation cost = %d, want 3", contentCost)
	}
	agent.streamStatus = http.StatusPreconditionFailed
	preconditionFailed := performRequest(server, http.MethodGet, createdResponse.DirectPath, nil, nil)
	if preconditionFailed.Code != http.StatusNotFound || strings.Contains(preconditionFailed.Body.String(), "/srv/public/photo.png") {
		t.Fatalf("upstream precondition leaked state = %d %s", preconditionFailed.Code, preconditionFailed.Body.String())
	}
	agent.streamStatus = http.StatusRequestEntityTooLarge
	tooLargeUpstream := performRequest(server, http.MethodGet, createdResponse.DirectPath, nil, nil)
	if tooLargeUpstream.Code != http.StatusNotFound || strings.Contains(tooLargeUpstream.Body.String(), "/srv/public/photo.png") {
		t.Fatalf("upstream size limit leaked state = %d %s", tooLargeUpstream.Code, tooLargeUpstream.Body.String())
	}
	agent.streamStatus = http.StatusOK

	auditJSON, err := json.Marshal(mustAuditEvents(t, server))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(auditJSON), token) || strings.Contains(string(auditJSON), createdResponse.SharePath) {
		t.Fatalf("bearer token entered audit events: %s", auditJSON)
	}

	rotationBody, _ := json.Marshal(fileShareCreateInput{
		Path: "/srv/public/photo.png", ExpectedResourceVersion: testFileShareVersion,
		ExpectedShareID: createdResponse.ID, ExpiresIn: "7d",
	})
	rotatedResponse := createPanelFileShare(t, server, sessionCookie, csrfCookie, rotationBody)
	rotatedToken := strings.TrimPrefix(rotatedResponse.SharePath, fileSharePagePrefix)
	if rotatedToken == token {
		t.Fatal("link rotation reused the bearer token")
	}
	oldLink := performRequest(server, http.MethodGet, fileShareAPIPrefix+token, nil, nil)
	if oldLink.Code != http.StatusNotFound {
		t.Fatalf("rotated link status = %d, want 404", oldLink.Code)
	}

	deleted := authenticatedRequest(
		server, http.MethodDelete, fileSharesAdminPath+"/"+rotatedResponse.ID, nil,
		sessionCookie, csrfCookie, map[string]string{
			"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", deleted.Code, deleted.Body.String())
	}
	stopped := performRequest(server, http.MethodGet, fileShareAPIPrefix+rotatedToken, nil, nil)
	if stopped.Code != http.StatusNotFound {
		t.Fatalf("stopped link status = %d, want 404", stopped.Code)
	}
	list = authenticatedRequest(server, http.MethodGet, fileSharesAdminPath, nil, sessionCookie, csrfCookie, nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"shares":[]`) {
		t.Fatalf("list after delete = %d %s", list.Code, list.Body.String())
	}
}

func TestFileShareCreateRejectsOversizedFiles(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	server.agent = &fileStubAgent{stubAgent: &stubAgent{response: fileShareEntryResponse(
		"/srv/oversized.iso", "oversized.iso", "application/octet-stream", contract.MaxFileShareBytes+1,
	)}}
	body, _ := json.Marshal(fileShareCreateInput{
		Path: "/srv/oversized.iso", ExpectedResourceVersion: testFileShareVersion, ExpiresIn: "7d",
	})
	response := authenticatedRequest(
		server, http.MethodPost, fileSharesAdminPath, body, sessionCookie, csrfCookie,
		map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), "file_share_too_large") {
		t.Fatalf("oversized create = %d %s", response.Code, response.Body.String())
	}
}

func TestFileShareCreateMapsFingerprintConflictAndTimeout(t *testing.T) {
	for _, test := range []struct {
		name       string
		agent      *stubAgent
		wantStatus int
		wantCode   string
	}{
		{
			name: "file changed while fingerprinting",
			agent: &stubAgent{response: AgentResponse{
				StatusCode: http.StatusConflict, ContentType: "application/problem+json",
				Body: []byte(`{"code":"file_conflict"}`),
			}},
			wantStatus: http.StatusConflict,
			wantCode:   "file_share_changed",
		},
		{
			name:       "fingerprint timeout",
			agent:      &stubAgent{err: context.DeadlineExceeded},
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "file_share_fingerprint_timeout",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, tokenPath := newTestServer(t)
			sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
			server.agent = test.agent
			body, _ := json.Marshal(fileShareCreateInput{
				Path: "/srv/release.tar.gz", ExpectedResourceVersion: testFileShareVersion, ExpiresIn: "7d",
			})
			response := authenticatedRequest(
				server, http.MethodPost, fileSharesAdminPath, body, sessionCookie, csrfCookie,
				map[string]string{
					"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
				},
			)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantCode) {
				t.Fatalf("create = %d %s, want status %d code %q", response.Code, response.Body.String(), test.wantStatus, test.wantCode)
			}
		})
	}
}

func TestFileShareUnsafeContentIsDownloadOnly(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	attackBody := `<script src="/f/` + strings.Repeat("j", 43) + `"></script>`
	agent := &fileStubAgent{
		stubAgent:    &stubAgent{response: fileShareEntryResponse("/srv/public/payload.html", "payload.html", "text/html", 64)},
		streamStatus: http.StatusOK,
		streamHeaders: http.Header{
			"Content-Type":            []string{"text/html; charset=utf-8"},
			"Content-Disposition":     []string{`inline; filename="payload.html"`},
			"Content-Security-Policy": []string{"script-src 'self'"},
		},
		streamResponse: []byte(attackBody),
	}
	server.agent = agent

	body, _ := json.Marshal(fileShareCreateInput{
		Path: "/srv/public/payload.html", ExpectedResourceVersion: testFileShareVersion, ExpiresIn: "7d",
	})
	created := createPanelFileShare(t, server, sessionCookie, csrfCookie, body)
	direct := performRequest(server, http.MethodGet, created.DirectPath, nil, nil)
	if direct.Code != http.StatusOK || direct.Body.String() != attackBody {
		t.Fatalf("unsafe direct response = %d %q", direct.Code, direct.Body.String())
	}
	if direct.Header().Get("Content-Type") != "application/octet-stream" ||
		!strings.HasPrefix(direct.Header().Get("Content-Disposition"), "attachment") ||
		!strings.Contains(direct.Header().Get("Content-Disposition"), "payload.html") ||
		direct.Header().Get("Content-Security-Policy") != fileShareContentSecurityPolicy ||
		direct.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unsafe direct headers = %#v", direct.Header())
	}
	if calls := agent.snapshotStreamCalls(); len(calls) != 1 ||
		!strings.Contains(calls[0].rawQuery, "disposition=inline") {
		t.Fatalf("Panel defense did not cover an inline Agent response: %#v", calls)
	}
}

func TestFileShareSafeInlineContentType(t *testing.T) {
	for contentType, want := range map[string]bool{
		"image/jpeg": true, "image/png; charset=binary": true, "image/gif": true,
		"image/webp": true, "image/avif": true, "image/svg+xml": false,
		"text/html": false, "application/xhtml+xml": false, "application/javascript": false,
		"text/css": false, "application/pdf": false, "application/octet-stream": false, "": false,
	} {
		if got := safeFileShareInlineContentType(contentType); got != want {
			t.Errorf("safeFileShareInlineContentType(%q) = %t, want %t", contentType, got, want)
		}
	}
}

func TestFileShareVersionBindingAndAnonymousFailures(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{stubAgent: &stubAgent{response: fileShareEntryResponse(
		"/srv/release.tar.gz", "release.tar.gz", "application/gzip", 2048,
	)}}
	server.agent = agent

	staleBody, _ := json.Marshal(fileShareCreateInput{
		Path: "/srv/release.tar.gz", ExpectedResourceVersion: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ExpiresIn: "30d",
	})
	stale := authenticatedRequest(
		server, http.MethodPost, fileSharesAdminPath, staleBody, sessionCookie, csrfCookie,
		map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value},
	)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale version = %d %s", stale.Code, stale.Body.String())
	}

	validBody, _ := json.Marshal(fileShareCreateInput{
		Path: "/srv/release.tar.gz", ExpectedResourceVersion: testFileShareVersion, ExpiresIn: "never",
	})
	created := createPanelFileShare(t, server, sessionCookie, csrfCookie, validBody)
	token := strings.TrimPrefix(created.SharePath, fileSharePagePrefix)

	agent.stubAgent.mu.Lock()
	agent.stubAgent.response = fileShareEntryResponse(
		"/srv/release.tar.gz", "release.tar.gz", "application/gzip", 4096,
	)
	agent.stubAgent.response.Body = []byte(strings.ReplaceAll(
		string(agent.stubAgent.response.Body), testFileShareVersion,
		"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	))
	agent.stubAgent.response.Body = []byte(strings.ReplaceAll(
		string(agent.stubAgent.response.Body), testFileShareStrongVersion, testFileShareStrongVersion2,
	))
	agent.stubAgent.mu.Unlock()
	changed := performRequest(server, http.MethodGet, fileShareAPIPrefix+token, nil, nil)
	if changed.Code != http.StatusNotFound || strings.Contains(changed.Body.String(), "/srv/release.tar.gz") {
		t.Fatalf("changed file leaked state = %d %s", changed.Code, changed.Body.String())
	}
	changedVersion := "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	lookupQuery := url.Values{
		"path": []string{"/srv/release.tar.gz"}, "resourceVersion": []string{changedVersion},
	}
	lookup := authenticatedRequest(
		server, http.MethodGet, fileSharesAdminPath+"?"+lookupQuery.Encode(), nil,
		sessionCookie, csrfCookie, nil,
	)
	if lookup.Code != http.StatusOK || !strings.Contains(lookup.Body.String(), `"share":null`) {
		t.Fatalf("changed file lookup = %d %s", lookup.Code, lookup.Body.String())
	}
	streamContext, cancelStream := context.WithCancel(context.Background())
	finishStream := server.registerFileShareStream(created.ID, cancelStream)
	defer finishStream()
	newBody, _ := json.Marshal(fileShareCreateInput{
		Path: "/srv/release.tar.gz", ExpectedResourceVersion: changedVersion, ExpiresIn: "30d",
	})
	newShare := createPanelFileShare(t, server, sessionCookie, csrfCookie, newBody)
	select {
	case <-streamContext.Done():
	default:
		t.Fatal("replacing a stale-version share did not cancel its registered stream")
	}
	newToken := strings.TrimPrefix(newShare.SharePath, fileSharePagePrefix)
	if response := performRequest(server, http.MethodGet, fileShareAPIPrefix+newToken, nil, nil); response.Code != http.StatusOK {
		t.Fatalf("new-version public metadata = %d %s", response.Code, response.Body.String())
	}

	for _, requestPath := range []string{
		fileShareAPIPrefix + strings.Repeat("a", 42),
		fileShareAPIPrefix + strings.Repeat("a", 44),
		fileShareAPIPrefix + token + "/extra",
		fileShareAPIPrefix + token + "?extra=1",
		fileSharePagePrefix + token + "?extra=1",
		fileShareRawPrefix + token + "?download=0",
		fileShareRawPrefix + token + "?download=1&broken=%ZZ",
		fileShareRawPrefix + token + "?download=1;extra=1",
	} {
		response := performRequest(server, http.MethodGet, requestPath, nil, nil)
		if response.Code != http.StatusNotFound {
			t.Fatalf("invalid public request %q = %d", requestPath, response.Code)
		}
	}
	wrongPageMethod := performRequest(server, http.MethodPost, fileSharePagePrefix+newToken, nil, nil)
	if wrongPageMethod.Code != http.StatusNotFound {
		t.Fatalf("public share page accepted POST: %d", wrongPageMethod.Code)
	}
}

func TestFileShareContentDoesNotDowngradeToOldAgentEndpoint(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{
		stubAgent: &stubAgent{response: fileShareEntryResponse(
			"/srv/private/report.txt", "report.txt", "text/plain", 32,
		)},
	}
	server.agent = agent
	body, _ := json.Marshal(fileShareCreateInput{
		Path: "/srv/private/report.txt", ExpectedResourceVersion: testFileShareVersion, ExpiresIn: "7d",
	})
	created := createPanelFileShare(t, server, sessionCookie, csrfCookie, body)

	// An older Agent has no /v1/files/share-content route. It must fail closed;
	// Panel must never retry the ordinary weak-version content endpoint.
	agent.streamStatus = http.StatusNotFound
	response := performRequest(server, http.MethodGet, created.DirectPath, nil, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("old Agent share-content response = %d %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotStreamCalls()
	if len(calls) != 1 || calls[0].path != "/v1/files/share-content" {
		t.Fatalf("public stream downgraded Agent endpoint: %#v", calls)
	}
}

func TestFileSharePublicPathsBypassOnlyExactSecurityEntranceShapes(t *testing.T) {
	token := base64TokenForTest()
	for _, requestPath := range []string{
		fileSharePagePrefix + token, fileShareAPIPrefix + token, fileShareRawPrefix + token,
	} {
		if !securityEntrancePublicPath(requestPath) {
			t.Fatalf("valid public path %q was rejected", requestPath)
		}
	}
	for _, requestPath := range []string{
		fileSharePagePrefix, fileShareAPIPrefix, fileShareRawPrefix,
		fileSharePagePrefix + token + "/extra",
		fileShareAPIPrefix + strings.Repeat("a", 42),
		fileShareRawPrefix + strings.Repeat("a", 44),
	} {
		if securityEntrancePublicPath(requestPath) {
			t.Fatalf("invalid public path %q bypassed the security entrance", requestPath)
		}
	}
}

func TestFileShareRateLimiterIsBoundedAndResets(t *testing.T) {
	server := &Server{}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	for range fileShareRateLimit {
		if !server.allowFileShareRequest("198.51.100.9", now) {
			t.Fatal("request was limited before the configured limit")
		}
	}
	if server.allowFileShareRequest("198.51.100.9", now) {
		t.Fatal("request above the configured limit was allowed")
	}
	if !server.allowFileShareRequest("198.51.100.9", now.Add(fileShareRateWindow)) {
		t.Fatal("rate limit did not reset")
	}

	server = &Server{fileShareRates: make(map[string]fileShareRateEntry)}
	for index := range fileShareRateKeys {
		key := time.Unix(int64(index), 0).String()
		if !server.allowFileShareRequest(key, now) {
			t.Fatalf("subject %d rejected before capacity", index)
		}
	}
	if server.allowFileShareRequest("overflow", now) {
		t.Fatal("rate limiter accepted an unbounded subject")
	}
}

func TestFileShareValidationBudgetIsSizeWeightedAndResets(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		sizeBytes  int64
		admissions int
	}{
		{name: "2 MiB", sizeBytes: 2 << 20, admissions: 512},
		{name: "2 MiB plus one", sizeBytes: (2 << 20) + 1, admissions: 256},
		{name: "8 MiB", sizeBytes: 8 << 20, admissions: 128},
		{name: "12 MiB", sizeBytes: 12 << 20, admissions: 85},
		{name: "64 MiB", sizeBytes: 64 << 20, admissions: 16},
		{name: "512 MiB", sizeBytes: contract.MaxFileShareBytes, admissions: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := &Server{}
			for index := range test.admissions {
				if !server.allowFileShareValidation("/shared.bin", test.sizeBytes, now) {
					t.Fatalf("validation %d was limited before the expected boundary", index+1)
				}
			}
			if server.allowFileShareValidation("/shared.bin", test.sizeBytes, now) {
				t.Fatal("validation above the size-weighted boundary was allowed")
			}
		})
	}

	global := &Server{}
	if !global.allowFileShareValidation("/large-a.bin", contract.MaxFileShareBytes, now) ||
		!global.allowFileShareValidation("/large-b.bin", contract.MaxFileShareBytes, now) {
		t.Fatal("global budget rejected one of two 512 MiB validations")
	}
	if global.allowFileShareValidation("/large-c.bin", contract.MaxFileShareBytes, now) {
		t.Fatal("global budget allowed more than 1 GiB of validation per minute")
	}
	if !global.allowFileShareValidation(
		"/large-c.bin", contract.MaxFileShareBytes, now.Add(fileShareValidationWindow),
	) {
		t.Fatal("validation budget did not reset")
	}
}

func TestFileShareValidationBudgetIsAtomicUnderConcurrency(t *testing.T) {
	server := &Server{}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	var admitted atomic.Int64
	var workers sync.WaitGroup
	for range 1000 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if server.allowFileShareValidation("/icon.png", 1024, now) {
				admitted.Add(1)
			}
		}()
	}
	workers.Wait()
	if got := admitted.Load(); got != fileShareValidationGlobal {
		t.Fatalf("concurrent admissions = %d, want %d", got, fileShareValidationGlobal)
	}
}

type blockingFileShareAgent struct {
	response AgentResponse
	started  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (agent *blockingFileShareAgent) Get(
	ctx context.Context,
	_, _, _ string,
) (AgentResponse, error) {
	agent.once.Do(func() { close(agent.started) })
	select {
	case <-ctx.Done():
		return AgentResponse{}, ctx.Err()
	case <-agent.release:
		return agent.response, nil
	}
}

func (agent *blockingFileShareAgent) Do(
	context.Context,
	string,
	string,
	string,
	string,
	[]byte,
) (AgentResponse, error) {
	return agent.response, nil
}

func TestFileShareMetadataReadsAreBoundedAndRecheckRevocation(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	entryResponse := fileShareEntryResponse("/srv/public/manual.pdf", "manual.pdf", "application/pdf", 4096)
	server.agent = &fileStubAgent{stubAgent: &stubAgent{response: entryResponse}}
	body, _ := json.Marshal(fileShareCreateInput{
		Path: "/srv/public/manual.pdf", ExpectedResourceVersion: testFileShareVersion, ExpiresIn: "never",
	})
	created := createPanelFileShare(t, server, sessionCookie, csrfCookie, body)
	token := strings.TrimPrefix(created.SharePath, fileSharePagePrefix)

	for range maxFileShareMetadataReads {
		server.fileShareMetadataGate <- struct{}{}
	}
	busy := performRequest(server, http.MethodGet, fileShareAPIPrefix+token, nil, nil)
	if busy.Code != http.StatusTooManyRequests || busy.Header().Get("Retry-After") != "1" {
		t.Fatalf("full metadata gate = %d headers=%#v body=%q", busy.Code, busy.Header(), busy.Body.String())
	}
	unknown := performRequest(server, http.MethodGet, fileShareAPIPrefix+base64TokenForTest(), nil, nil)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown token consulted full metadata gate: %d %s", unknown.Code, unknown.Body.String())
	}
	for range maxFileShareMetadataReads {
		<-server.fileShareMetadataGate
	}

	timeoutAgent := &blockingFileShareAgent{
		response: entryResponse, started: make(chan struct{}), release: make(chan struct{}),
	}
	server.agent = timeoutAgent
	timeoutContext, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, _, status := server.resolvePublicFileShare(timeoutContext, token)
	if status != http.StatusServiceUnavailable || len(server.fileShareMetadataGate) != 0 {
		t.Fatalf("timed out metadata read status=%d gate=%d", status, len(server.fileShareMetadataGate))
	}

	revocationAgent := &blockingFileShareAgent{
		response: entryResponse, started: make(chan struct{}), release: make(chan struct{}),
	}
	server.agent = revocationAgent
	result := make(chan int, 1)
	go func() {
		_, _, resolvedStatus := server.resolvePublicFileShare(context.Background(), token)
		result <- resolvedStatus
	}()
	select {
	case <-revocationAgent.started:
	case <-time.After(time.Second):
		t.Fatal("metadata Agent read did not start")
	}
	if _, err := server.store.DeleteFileShare(created.ID); err != nil {
		t.Fatal(err)
	}
	close(revocationAgent.release)
	select {
	case resolvedStatus := <-result:
		if resolvedStatus != http.StatusNotFound {
			t.Fatalf("metadata returned after revoke with status %d", resolvedStatus)
		}
	case <-time.After(time.Second):
		t.Fatal("revoked metadata read did not finish")
	}
	if len(server.fileShareMetadataGate) != 0 {
		t.Fatalf("metadata gate leaked after revoke: %d", len(server.fileShareMetadataGate))
	}
}

type blockingFileShareStreamAgent struct {
	*fileStubAgent
	started chan struct{}
	release <-chan struct{}
}

func (agent *blockingFileShareStreamAgent) OpenStream(
	ctx context.Context,
	method, path, rawQuery, _ string,
	_ io.Reader,
	headers http.Header,
	contentLength int64,
) (*http.Response, error) {
	agent.mu.Lock()
	agent.streamCalls = append(agent.streamCalls, streamAgentCall{
		method: method, path: path, rawQuery: rawQuery, headers: headers.Clone(),
		contentLength: contentLength,
	})
	agent.mu.Unlock()
	select {
	case agent.started <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-agent.release:
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: 0, Body: http.NoBody,
		}, nil
	}
}

type fileShareStreamFixture struct {
	server        *Server
	sessionCookie *http.Cookie
	csrfCookie    *http.Cookie
	agent         *blockingFileShareStreamAgent
	created       fileShareAdminView
	token         string
	filePath      string
}

func newFileShareStreamFixture(t *testing.T, release <-chan struct{}) fileShareStreamFixture {
	t.Helper()
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	filePath := "/srv/public/stream.bin"
	agent := &blockingFileShareStreamAgent{
		fileStubAgent: &fileStubAgent{stubAgent: &stubAgent{response: fileShareEntryResponse(
			filePath, "stream.bin", "application/octet-stream", 4096,
		)}},
		started: make(chan struct{}, maxPublicFileShareStreams+4),
		release: release,
	}
	server.agent = agent
	body, _ := json.Marshal(fileShareCreateInput{
		Path: filePath, ExpectedResourceVersion: testFileShareVersion, ExpiresIn: "never",
	})
	created := createPanelFileShare(t, server, sessionCookie, csrfCookie, body)
	return fileShareStreamFixture{
		server: server, sessionCookie: sessionCookie, csrfCookie: csrfCookie,
		agent: agent, created: created, token: strings.TrimPrefix(created.SharePath, fileSharePagePrefix),
		filePath: filePath,
	}
}

func startPublicFileShareRequest(server *Server, requestPath string) <-chan int {
	result := make(chan int, 1)
	go func() {
		result <- performRequest(server, http.MethodGet, requestPath, nil, nil).Code
	}()
	return result
}

func waitFileShareStreamStart(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("public file share stream did not reach Agent")
	}
}

func waitFileShareStreamStatus(t *testing.T, result <-chan int) int {
	t.Helper()
	select {
	case status := <-result:
		return status
	case <-time.After(2 * time.Second):
		t.Fatal("public file share stream did not finish")
		return 0
	}
}

func TestFileSharePublicStreamGateIsBoundedAndReleased(t *testing.T) {
	release := make(chan struct{})
	fixture := newFileShareStreamFixture(t, release)
	results := make([]<-chan int, 0, maxPublicFileShareStreams)
	for range maxPublicFileShareStreams {
		results = append(results, startPublicFileShareRequest(fixture.server, fixture.created.DirectPath))
		waitFileShareStreamStart(t, fixture.agent.started)
	}

	busy := performRequest(fixture.server, http.MethodGet, fixture.created.DirectPath, nil, nil)
	if busy.Code != http.StatusTooManyRequests || busy.Header().Get("Retry-After") != "1" {
		t.Fatalf("full public stream gate = %d headers=%#v body=%q", busy.Code, busy.Header(), busy.Body.String())
	}
	fixture.server.fileShareValidationMu.Lock()
	chargedUnits := fixture.server.fileShareValidations[fixture.filePath].units
	fixture.server.fileShareValidationMu.Unlock()
	if chargedUnits != int64(maxPublicFileShareStreams) {
		t.Fatalf("busy stream consumed validation budget: units=%d", chargedUnits)
	}
	close(release)
	for _, result := range results {
		if status := waitFileShareStreamStatus(t, result); status != http.StatusOK {
			t.Fatalf("released public stream = %d, want 200", status)
		}
	}
	if len(fixture.server.fileShareStreamGate) != 0 {
		t.Fatalf("public stream gate leaked after release: %d", len(fixture.server.fileShareStreamGate))
	}
	recovered := performRequest(fixture.server, http.MethodGet, fixture.created.DirectPath, nil, nil)
	if recovered.Code != http.StatusOK {
		t.Fatalf("public stream gate did not recover: %d %s", recovered.Code, recovered.Body.String())
	}
	if calls := fixture.agent.snapshotStreamCalls(); len(calls) != maxPublicFileShareStreams+1 {
		t.Fatalf("Agent stream calls = %d, want %d", len(calls), maxPublicFileShareStreams+1)
	}
}

func TestFileShareValidationLimitRejectsBeforeAgentOpen(t *testing.T) {
	release := make(chan struct{})
	close(release)
	fixture := newFileShareStreamFixture(t, release)
	now := time.Now().UTC()
	for range fileShareValidationGlobal {
		if !fixture.server.allowFileShareValidation("/other-icon.png", 1024, now) {
			t.Fatal("failed to fill validation budget")
		}
	}

	response := performRequest(fixture.server, http.MethodGet, fixture.created.DirectPath, nil, nil)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "60" {
		t.Fatalf("validation-limited stream = %d headers=%#v body=%q", response.Code, response.Header(), response.Body.String())
	}
	if calls := fixture.agent.snapshotStreamCalls(); len(calls) != 0 {
		t.Fatalf("validation-limited request reached Agent: %#v", calls)
	}
}

func TestFileSharePublicStreamIsCancelledByDeleteAndRotation(t *testing.T) {
	for _, action := range []string{"delete", "rotation"} {
		t.Run(action, func(t *testing.T) {
			fixture := newFileShareStreamFixture(t, nil)
			result := startPublicFileShareRequest(fixture.server, fixture.created.DirectPath)
			waitFileShareStreamStart(t, fixture.agent.started)

			switch action {
			case "delete":
				response := authenticatedRequest(
					fixture.server, http.MethodDelete, fileSharesAdminPath+"/"+fixture.created.ID, nil,
					fixture.sessionCookie, fixture.csrfCookie, map[string]string{
						"Origin": "http://panel.test", "X-CSRF-Token": fixture.csrfCookie.Value,
					},
				)
				if response.Code != http.StatusNoContent {
					t.Fatalf("delete active share = %d %s", response.Code, response.Body.String())
				}
			case "rotation":
				body, _ := json.Marshal(fileShareCreateInput{
					Path: fixture.filePath, ExpectedResourceVersion: testFileShareVersion,
					ExpectedShareID: fixture.created.ID, ExpiresIn: "never",
				})
				rotated := createPanelFileShare(
					t, fixture.server, fixture.sessionCookie, fixture.csrfCookie, body,
				)
				if rotated.ID == fixture.created.ID || rotated.SharePath == fixture.created.SharePath {
					t.Fatalf("rotation did not replace active share: old=%#v new=%#v", fixture.created, rotated)
				}
			}

			if status := waitFileShareStreamStatus(t, result); status != http.StatusNotFound {
				t.Fatalf("%s cancelled public stream = %d, want 404", action, status)
			}
			if len(fixture.server.fileShareStreamGate) != 0 {
				t.Fatalf("public stream gate leaked after %s: %d", action, len(fixture.server.fileShareStreamGate))
			}
		})
	}
}

func TestFileSharePublicStreamExpiresAndReleasesCapacity(t *testing.T) {
	fixture := newFileShareStreamFixture(t, nil)
	now := time.Now().UTC()
	expiresAt := now.Add(500 * time.Millisecond)
	digest := sha256.Sum256([]byte(fixture.token))
	if _, err := fixture.server.store.DeleteFileShare(fixture.created.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.server.store.CreateFileShare(store.FileShare{
		ID: fixture.created.ID, TokenHash: hex.EncodeToString(digest[:]), Path: fixture.filePath,
		ResourceVersion: testFileShareVersion, ShareVersion: testFileShareStrongVersion,
		SizeBytes: 4096,
		CreatedAt: now, ExpiresAt: &expiresAt,
	}, "", now); err != nil {
		t.Fatal(err)
	}

	result := startPublicFileShareRequest(fixture.server, fixture.created.DirectPath)
	waitFileShareStreamStart(t, fixture.agent.started)
	if status := waitFileShareStreamStatus(t, result); status != http.StatusNotFound {
		t.Fatalf("expired public stream = %d, want 404", status)
	}
	if len(fixture.server.fileShareStreamGate) != 0 {
		t.Fatalf("public stream gate leaked after expiry: %d", len(fixture.server.fileShareStreamGate))
	}
}

func TestFileSharePublicStreamIsCancelledByServerClose(t *testing.T) {
	fixture := newFileShareStreamFixture(t, nil)
	result := startPublicFileShareRequest(fixture.server, fixture.created.DirectPath)
	waitFileShareStreamStart(t, fixture.agent.started)
	closeErr := fixture.server.Close()
	status := waitFileShareStreamStatus(t, result)
	if closeErr != nil {
		t.Fatalf("Server.Close error = %v", closeErr)
	}
	if status != http.StatusServiceUnavailable {
		t.Fatalf("Server.Close cancelled public stream = %d, want 503", status)
	}
	if len(fixture.server.fileShareStreamGate) != 0 {
		t.Fatalf("public stream gate leaked after Server.Close: %d", len(fixture.server.fileShareStreamGate))
	}
}

func TestFileSharePublicStreamRechecksRevocationBeforeAgentOpen(t *testing.T) {
	fixture := newFileShareStreamFixture(t, nil)
	fixture.server.fileShareStreamMu.Lock()
	locked := true
	defer func() {
		if locked {
			fixture.server.fileShareStreamMu.Unlock()
		}
	}()
	result := startPublicFileShareRequest(fixture.server, fixture.created.DirectPath)
	deadline := time.Now().Add(time.Second)
	for len(fixture.server.fileShareStreamGate) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(fixture.server.fileShareStreamGate) != 1 {
		t.Fatal("public stream did not reach the lookup-to-registration window")
	}
	if _, err := fixture.server.store.DeleteFileShare(fixture.created.ID); err != nil {
		t.Fatal(err)
	}
	fixture.server.fileShareStreamMu.Unlock()
	locked = false

	if status := waitFileShareStreamStatus(t, result); status != http.StatusNotFound {
		t.Fatalf("revoked-before-open public stream = %d, want 404", status)
	}
	if calls := fixture.agent.snapshotStreamCalls(); len(calls) != 0 {
		t.Fatalf("revoked share reached Agent OpenStream: %#v", calls)
	}
	fixture.server.fileShareValidationMu.Lock()
	revokedCost := fixture.server.fileShareGlobalCost.units
	fixture.server.fileShareValidationMu.Unlock()
	if revokedCost != 0 {
		t.Fatalf("revoked-before-open request consumed validation budget: %d", revokedCost)
	}
	if len(fixture.server.fileShareStreamGate) != 0 {
		t.Fatalf("public stream gate leaked after pre-open revocation: %d", len(fixture.server.fileShareStreamGate))
	}
}

func createPanelFileShare(
	t *testing.T,
	server *Server,
	sessionCookie, csrfCookie *http.Cookie,
	body []byte,
) fileShareAdminView {
	t.Helper()
	response := authenticatedRequest(
		server, http.MethodPost, fileSharesAdminPath, body, sessionCookie, csrfCookie,
		map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", response.Code, response.Body.String())
	}
	var result fileShareAdminView
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func fileShareEntryResponse(filePath, name, mime string, size int64) AgentResponse {
	body, _ := json.Marshal(map[string]any{
		"name": name, "path": filePath, "kind": "file", "mime": mime, "sizeBytes": size,
		"mode": "0644", "owner": "root", "group": "root", "modifiedAt": "2026-08-22T00:00:00Z",
		"resourceVersion": testFileShareVersion, "shareVersion": testFileShareStrongVersion,
		"editable": false, "previewable": true,
	})
	return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}
}

func mustAuditEvents(t *testing.T, server *Server) []store.AuditEvent {
	t.Helper()
	events, _ := server.store.ListAudit(50, "")
	return events
}

func assertJSONKeys(t *testing.T, object map[string]json.RawMessage, expected ...string) {
	t.Helper()
	if len(object) != len(expected) {
		t.Fatalf("JSON keys = %v, want exactly %v", object, expected)
	}
	for _, key := range expected {
		if _, ok := object[key]; !ok {
			t.Fatalf("missing JSON key %q in %v", key, object)
		}
	}
}

func base64TokenForTest() string {
	return "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
}
