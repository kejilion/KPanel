package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/store"
)

type streamAgentCall struct {
	method   string
	path     string
	rawQuery string
	headers  http.Header
	body     []byte
}

type fileStubAgent struct {
	*stubAgent
	mu             sync.Mutex
	streamCalls    []streamAgentCall
	streamStatus   int
	streamHeaders  http.Header
	streamResponse []byte
}

func (agent *fileStubAgent) OpenStream(
	_ context.Context,
	method, path, rawQuery, _ string,
	body io.Reader,
	headers http.Header,
	_ int64,
) (*http.Response, error) {
	content, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	agent.mu.Lock()
	agent.streamCalls = append(agent.streamCalls, streamAgentCall{
		method: method, path: path, rawQuery: rawQuery, headers: headers.Clone(), body: content,
	})
	agent.mu.Unlock()
	status := agent.streamStatus
	if status == 0 {
		status = http.StatusOK
	}
	responseHeaders := agent.streamHeaders.Clone()
	if responseHeaders == nil {
		responseHeaders = make(http.Header)
	}
	return &http.Response{
		StatusCode:    status,
		Header:        responseHeaders,
		ContentLength: int64(len(agent.streamResponse)),
		Body:          io.NopCloser(bytes.NewReader(agent.streamResponse)),
	}, nil
}

func (agent *fileStubAgent) snapshotStreamCalls() []streamAgentCall {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return append([]streamAgentCall(nil), agent.streamCalls...)
}

func TestSuffixedFileTransferNamePreservesExtensionAndByteLimit(t *testing.T) {
	if got := suffixedFileTransferName("app", 1); got != "app (1)" {
		t.Fatalf("directory suffix=%q", got)
	}
	if got := suffixedFileTransferName("archive.tar.gz", 2); got != "archive.tar (2).gz" {
		t.Fatalf("file suffix=%q", got)
	}
	got := suffixedFileTransferName(strings.Repeat("界", 100)+".txt", 999)
	if len(got) > 255 || !strings.HasSuffix(got, " (999).txt") {
		t.Fatalf("bounded unicode suffix bytes=%d value=%q", len(got), got)
	}
}

func TestFileListRequiresSessionAndForwardsStrictQuery(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{stubAgent: &stubAgent{response: AgentResponse{
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body:        []byte(`{"path":"/","entries":[],"truncated":false,"readAt":"2026-07-30T00:00:00Z"}`),
	}}}
	server.agent = agent

	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/files?path=%2F", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}
	if len(agent.snapshotCalls()) != 0 {
		t.Fatal("Agent called before authentication")
	}

	response := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files?path=%2F&limit=100&offset=0&search=log", nil,
		sessionCookie, csrfCookie, nil,
	)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"path":"/"`) {
		t.Fatalf("list response = %d %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/files" ||
		calls[0].rawQuery != "path=%2F&limit=100&offset=0&search=log" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}

	invalid := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files?path=%2F&unknown=true", nil,
		sessionCookie, csrfCookie, nil,
	)
	if invalid.Code != http.StatusBadRequest || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("invalid query = %d calls=%#v", invalid.Code, agent.snapshotCalls())
	}
}

func TestFileEntryRequiresSessionAndForwardsExactPath(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{stubAgent: &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"name":"nginx.conf","path":"/etc/nginx/nginx.conf","kind":"file","sizeBytes":9,"mode":"-rw-r--r--","owner":"root","group":"root","modifiedAt":"2026-08-14T00:00:00Z","resourceVersion":"sha256:test","editable":true,"previewable":true}`),
	}}}
	server.agent = agent

	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/files/entry?path=%2Fetc%2Fnginx%2Fnginx.conf", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("unauthenticated status=%d calls=%#v", unauthenticated.Code, agent.snapshotCalls())
	}
	response := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/entry?path=%2Fetc%2Fnginx%2Fnginx.conf", nil,
		sessionCookie, csrfCookie, nil,
	)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"path":"/etc/nginx/nginx.conf"`) {
		t.Fatalf("entry response=%d %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/files/entry" || calls[0].rawQuery != "path=%2Fetc%2Fnginx%2Fnginx.conf" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	invalid := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/entry?path=%2Fetc&extra=1", nil,
		sessionCookie, csrfCookie, nil,
	)
	if invalid.Code != http.StatusBadRequest || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("invalid query=%d calls=%#v", invalid.Code, agent.snapshotCalls())
	}
}

func TestFileEntriesRequiresCSRFAndForwardsCanonicalBatch(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{stubAgent: &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"entries":[{"name":"app","path":"/home/app","kind":"directory","resourceVersion":"sha256:app"}],"unavailable":["/missing"]}`),
	}}}
	server.agent = agent
	body := []byte(`{"paths":["/home/app","/missing"]}`)

	unauthenticated := performRequest(server, http.MethodPost, "/api/v1/files/entries", body, nil)
	if unauthenticated.Code != http.StatusUnauthorized || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("unauthenticated status=%d calls=%#v", unauthenticated.Code, agent.snapshotCalls())
	}
	withoutCSRF := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/entries", body, sessionCookie, csrfCookie,
		map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test"},
	)
	if withoutCSRF.Code != http.StatusForbidden || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("without CSRF status=%d calls=%#v", withoutCSRF.Code, agent.snapshotCalls())
	}
	response := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/entries", body, sessionCookie, csrfCookie,
		map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value},
	)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"path":"/home/app"`) {
		t.Fatalf("entries response=%d %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/files/entries" || calls[0].rawQuery != "" || string(calls[0].body) != string(body) {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
	invalid := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/entries", []byte(`{"paths":["/home//app"]}`), sessionCookie, csrfCookie,
		map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value},
	)
	if invalid.Code != http.StatusUnprocessableEntity || len(agent.snapshotCalls()) != 1 {
		t.Fatalf("invalid status=%d calls=%#v", invalid.Code, agent.snapshotCalls())
	}
}

func TestFileTrashListRequiresSessionAndForwardsToAgent(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{stubAgent: &stubAgent{response: AgentResponse{
		StatusCode: http.StatusOK, ContentType: "application/json",
		Body: []byte(`{"entries":[],"total":0,"readAt":"2026-07-30T00:00:00Z"}`),
	}}}
	server.agent = agent

	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/files/trash", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", unauthenticated.Code)
	}
	response := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/trash", nil, sessionCookie, csrfCookie, nil,
	)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"total":0`) {
		t.Fatalf("trash response = %d %s", response.Code, response.Body.String())
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].path != "/v1/files/trash" || calls[0].rawQuery != "" {
		t.Fatalf("unexpected Agent calls: %#v", calls)
	}
}

func TestFileContentStreamsRangeAndUploadRequiresCSRF(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{
		stubAgent:      &stubAgent{},
		streamStatus:   http.StatusPartialContent,
		streamResponse: []byte("hello"),
		streamHeaders: http.Header{
			"Content-Type":  []string{"text/plain"},
			"Content-Range": []string{"bytes 0-4/5"},
			"ETag":          []string{`"sha256:test"`},
		},
	}
	server.agent = agent

	download := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/content?path=%2Fhello.txt&disposition=inline",
		nil, sessionCookie, csrfCookie, map[string]string{"Range": "bytes=0-4"},
	)
	if download.Code != http.StatusPartialContent || download.Body.String() != "hello" {
		t.Fatalf("download = %d %q", download.Code, download.Body.String())
	}
	if download.Header().Get("Content-Length") != "5" {
		t.Fatalf("download content length = %q", download.Header().Get("Content-Length"))
	}
	if download.Header().Get("Content-Security-Policy") == "" ||
		download.Header().Get("X-Content-Type-Options") != "nosniff" ||
		download.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("download security headers = %#v", download.Header())
	}
	streamCalls := agent.snapshotStreamCalls()
	if len(streamCalls) != 1 || streamCalls[0].headers.Get("Range") != "bytes=0-4" {
		t.Fatalf("range not forwarded: %#v", streamCalls)
	}

	agent.streamStatus = http.StatusCreated
	agent.streamResponse = []byte(`{"name":"upload.txt","path":"/upload.txt"}`)
	withoutCSRF := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/upload?path=%2F&name=upload.txt",
		[]byte("payload"), sessionCookie, csrfCookie,
		map[string]string{"Content-Type": "application/octet-stream", "Origin": "http://panel.test"},
	)
	if withoutCSRF.Code != http.StatusForbidden {
		t.Fatalf("upload without CSRF = %d", withoutCSRF.Code)
	}

	upload := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/upload?path=%2F&name=upload.txt",
		[]byte("payload"), sessionCookie, csrfCookie,
		map[string]string{
			"Content-Type": "application/octet-stream",
			"Origin":       "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if upload.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", upload.Code, upload.Body.String())
	}
	streamCalls = agent.snapshotStreamCalls()
	if len(streamCalls) != 2 || streamCalls[1].method != http.MethodPost ||
		string(streamCalls[1].body) != "payload" {
		t.Fatalf("upload stream calls = %#v", streamCalls)
	}
}

func TestFileArchiveDownloadRequiresSessionAndStreamsAgentZIP(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{
		stubAgent:      &stubAgent{},
		streamStatus:   http.StatusOK,
		streamResponse: []byte("PK\x03\x04archive"),
		streamHeaders: http.Header{
			"Content-Type":        []string{"application/zip"},
			"Content-Disposition": []string{`attachment; filename="bundle.zip"`},
		},
	}
	server.agent = agent
	query := "selection=%7B%22sources%22%3A%5B%22%2Fapp%22%5D%7D&name=bundle.zip"
	unauthenticated := performRequest(
		server, http.MethodGet, "/api/v1/files/archive?"+query, nil, nil,
	)
	if unauthenticated.Code != http.StatusUnauthorized || len(agent.snapshotStreamCalls()) != 0 {
		t.Fatalf("unauthenticated archive=%d calls=%#v", unauthenticated.Code, agent.snapshotStreamCalls())
	}

	response := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/archive?"+query, nil,
		sessionCookie, csrfCookie, nil,
	)
	if response.Code != http.StatusOK || response.Body.String() != "PK\x03\x04archive" ||
		response.Header().Get("Content-Type") != "application/zip" ||
		!strings.Contains(response.Header().Get("Content-Disposition"), "bundle.zip") ||
		response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("archive response=%d headers=%#v body=%q", response.Code, response.Header(), response.Body.String())
	}
	calls := agent.snapshotStreamCalls()
	if len(calls) != 1 || calls[0].method != http.MethodGet || calls[0].path != "/v1/files/archive" ||
		calls[0].rawQuery != query {
		t.Fatalf("archive Agent calls=%#v", calls)
	}

	invalid := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/archive?"+query+"&extra=1", nil,
		sessionCookie, csrfCookie, nil,
	)
	if invalid.Code != http.StatusBadRequest || len(agent.snapshotStreamCalls()) != 1 {
		t.Fatalf("invalid archive=%d calls=%#v", invalid.Code, agent.snapshotStreamCalls())
	}
}

func TestFileDownloadTicketSupportsCookielessRangeHeadAndSecurityEntrance(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{
		stubAgent:      &stubAgent{},
		streamStatus:   http.StatusPartialContent,
		streamResponse: []byte("hello"),
		streamHeaders: http.Header{
			"Content-Type":   []string{"text/plain"},
			"Content-Range":  []string{"bytes 0-4/5"},
			"Content-Length": []string{"5"},
			"Accept-Ranges":  []string{"bytes"},
		},
	}
	server.agent = agent
	body := []byte(`{"path":"/hello.txt"}`)
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
		"X-CSRF-Token": csrfCookie.Value,
	}

	unauthenticated := performRequest(
		server, http.MethodPost, "/api/v1/files/download-tickets", body,
		map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test"},
	)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated ticket = %d %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	missingOrigin := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/download-tickets", body,
		sessionCookie, csrfCookie,
		map[string]string{"Content-Type": "application/json", "X-CSRF-Token": csrfCookie.Value},
	)
	if missingOrigin.Code != http.StatusForbidden || !strings.Contains(missingOrigin.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing origin ticket = %d %s", missingOrigin.Code, missingOrigin.Body.String())
	}
	missingCSRF := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/download-tickets", body,
		sessionCookie, csrfCookie,
		map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test"},
	)
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF ticket = %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}

	created := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/download-tickets", body,
		sessionCookie, csrfCookie, headers,
	)
	if created.Code != http.StatusCreated {
		t.Fatalf("ticket create = %d %s", created.Code, created.Body.String())
	}
	var ticket fileDownloadTicketResponse
	if err := json.Unmarshal(created.Body.Bytes(), &ticket); err != nil || ticket.DownloadURL == "" || !ticket.ExpiresAt.After(time.Now()) {
		t.Fatalf("ticket response = %#v err=%v", ticket, err)
	}

	_, version := server.store.SecurityEntrance()
	if err := server.store.ReplaceSecurityEntrance(version, store.SecurityEntrance{Enabled: true, Path: "panel-secure1"}); err != nil {
		t.Fatal(err)
	}
	download := performRequest(server, http.MethodGet, ticket.DownloadURL, nil, map[string]string{"Range": "bytes=0-4"})
	if download.Code != http.StatusPartialContent || download.Body.String() != "hello" {
		t.Fatalf("cookieless download = %d %q", download.Code, download.Body.String())
	}
	calls := agent.snapshotStreamCalls()
	if len(calls) != 1 || calls[0].method != http.MethodGet ||
		calls[0].rawQuery != "disposition=attachment&path=%2Fhello.txt" || calls[0].headers.Get("Range") != "bytes=0-4" {
		t.Fatalf("download Agent call = %#v", calls)
	}

	agent.mu.Lock()
	agent.streamStatus = http.StatusOK
	agent.streamHeaders.Set("Content-Length", "5")
	agent.mu.Unlock()
	head := performRequest(server, http.MethodHead, ticket.DownloadURL, nil, nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != "5" {
		t.Fatalf("ticket HEAD = %d length=%q body=%q", head.Code, head.Header().Get("Content-Length"), head.Body.String())
	}
	calls = agent.snapshotStreamCalls()
	if len(calls) != 2 || calls[1].method != http.MethodHead {
		t.Fatalf("HEAD Agent call = %#v", calls)
	}

	key, ok := fileDownloadTicketKey(strings.TrimPrefix(ticket.DownloadURL, "/api/v1/files/download/"))
	if !ok {
		t.Fatal("created ticket token was invalid")
	}
	server.downloadTicketMu.Lock()
	server.downloadTickets[key] = fileDownloadTicket{Path: "/hello.txt", ExpiresAt: time.Now().Add(-time.Second)}
	server.downloadTicketMu.Unlock()
	expired := performRequest(server, http.MethodGet, ticket.DownloadURL, nil, nil)
	if expired.Code != http.StatusNotFound || len(agent.snapshotStreamCalls()) != 2 {
		t.Fatalf("expired ticket = %d calls=%#v", expired.Code, agent.snapshotStreamCalls())
	}

	server.downloadTicketMu.Lock()
	server.downloadTickets = make(map[[32]byte]fileDownloadTicket, maxFileDownloadTickets)
	for index := range maxFileDownloadTickets {
		var itemKey [32]byte
		itemKey[0] = byte(index)
		server.downloadTickets[itemKey] = fileDownloadTicket{Path: "/hello.txt", ExpiresAt: time.Now().Add(time.Minute)}
	}
	server.downloadTicketMu.Unlock()
	limited := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/download-tickets", body,
		sessionCookie, csrfCookie, headers,
	)
	if limited.Code != http.StatusTooManyRequests || !strings.Contains(limited.Body.String(), "file_download_ticket_limit") {
		t.Fatalf("ticket limit = %d %s", limited.Code, limited.Body.String())
	}
}

func TestFileThumbnailQueryIsAuthenticatedAndForwarded(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{
		stubAgent:      &stubAgent{},
		streamStatus:   http.StatusOK,
		streamResponse: []byte("thumbnail"),
		streamHeaders:  http.Header{"Content-Type": []string{"image/png"}},
	}
	server.agent = agent

	unauthenticated := performRequest(
		server,
		http.MethodGet,
		"/api/v1/files/content?path=%2Fphoto.png&disposition=inline&mode=thumbnail&version=version-1",
		nil,
		nil,
	)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated thumbnail status=%d", unauthenticated.Code)
	}
	response := authenticatedRequest(
		server,
		http.MethodGet,
		"/api/v1/files/content?path=%2Fphoto.png&disposition=inline&mode=thumbnail&version=version-1",
		nil,
		sessionCookie,
		csrfCookie,
		nil,
	)
	if response.Code != http.StatusOK || response.Body.String() != "thumbnail" {
		t.Fatalf("thumbnail response=%d %q", response.Code, response.Body.String())
	}
	calls := agent.snapshotStreamCalls()
	if len(calls) != 1 || calls[0].rawQuery != "path=%2Fphoto.png&disposition=inline&mode=thumbnail&version=version-1" {
		t.Fatalf("thumbnail stream call=%#v", calls)
	}
}

func TestFileActionUsesFixedEnumAndWritesAudit(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{stubAgent: &stubAgent{response: AgentResponse{
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body:        []byte(`{"action":"trash","succeeded":[{"path":"/old.txt"}]}`),
	}}}
	server.agent = agent
	headers := map[string]string{
		"Content-Type": "application/json",
		"Origin":       "http://panel.test",
		"X-CSRF-Token": csrfCookie.Value,
	}

	rejected := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/actions",
		[]byte(`{"action":"shell","sources":["/old.txt"]}`),
		sessionCookie, csrfCookie, headers,
	)
	if rejected.Code != http.StatusUnprocessableEntity || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("unsupported action = %d calls=%#v", rejected.Code, agent.snapshotCalls())
	}
	directDelete := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/actions",
		[]byte(`{"action":"delete","sources":["/old.txt"]}`),
		sessionCookie, csrfCookie, headers,
	)
	if directDelete.Code != http.StatusUnprocessableEntity || len(agent.snapshotCalls()) != 0 {
		t.Fatalf("ordinary permanent delete = %d calls=%#v", directDelete.Code, agent.snapshotCalls())
	}

	response := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/actions",
		[]byte(`{"action":"trash","sources":["/old.txt"]}`),
		sessionCookie, csrfCookie, headers,
	)
	if response.Code != http.StatusOK {
		t.Fatalf("trash action = %d %s", response.Code, response.Body.String())
	}
	events, _ := server.store.ListAudit(20, "")
	var intent, success bool
	for _, event := range events {
		if event.Action != "file.trash" {
			continue
		}
		intent = intent || event.Result == "intent"
		success = success || event.Result == "success"
	}
	if !intent || !success {
		t.Fatalf("file audit events missing: %#v", events)
	}

	agent.streamResponse = []byte(`{"action":"compress","succeeded":[{"path":"/backup.tar.gz"}]}`)
	agent.streamHeaders = http.Header{"Content-Type": []string{"application/json"}}
	archive := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/actions",
		[]byte(`{"action":"compress","sources":["/website"],"target":"/","name":"backup.tar.gz","format":"tar.gz"}`),
		sessionCookie, csrfCookie, headers,
	)
	if archive.Code != http.StatusOK {
		t.Fatalf("compress action = %d %s", archive.Code, archive.Body.String())
	}
	streamCalls := agent.snapshotStreamCalls()
	if len(streamCalls) != 1 ||
		!strings.Contains(string(streamCalls[0].body), `"format":"tar.gz"`) {
		t.Fatalf("compress action was not forwarded: %#v", streamCalls)
	}
}

func TestFileActionAuditsPartialFailure(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileStubAgent{stubAgent: &stubAgent{response: AgentResponse{
		StatusCode:  http.StatusMultiStatus,
		ContentType: "application/json",
		Body: []byte(
			`{"action":"trash","succeeded":[{"path":"/one.txt"}],` +
				`"failed":[{"path":"/missing.txt","detail":"文件不存在"}]}`,
		),
	}}}
	server.agent = agent
	response := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/actions",
		[]byte(`{"action":"trash","sources":["/one.txt","/missing.txt"]}`),
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json",
			"Origin":       "http://panel.test",
			"X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusMultiStatus {
		t.Fatalf("partial response = %d %s", response.Code, response.Body.String())
	}
	events, _ := server.store.ListAudit(20, "")
	for _, event := range events {
		if event.Action == "file.trash" && event.Result == "partial_failure" {
			return
		}
	}
	t.Fatalf("partial failure audit missing: %#v", events)
}
