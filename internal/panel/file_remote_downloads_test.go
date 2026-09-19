package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

type fileRemoteDownloadAgent struct {
	mu             sync.Mutex
	existing       map[string]contract.FileEntry
	streamCalls    []streamAgentCall
	streamStatus   int
	streamResponse []byte
}

type blockingRemoteDownloadBody struct {
	started     chan struct{}
	closed      chan struct{}
	startedOnce sync.Once
	closedOnce  sync.Once
}

func newBlockingRemoteDownloadBody() *blockingRemoteDownloadBody {
	return &blockingRemoteDownloadBody{started: make(chan struct{}), closed: make(chan struct{})}
}

func (body *blockingRemoteDownloadBody) Read([]byte) (int, error) {
	body.startedOnce.Do(func() { close(body.started) })
	<-body.closed
	return 0, io.ErrClosedPipe
}

func (body *blockingRemoteDownloadBody) Close() error {
	body.closedOnce.Do(func() { close(body.closed) })
	return nil
}

type earlyFileRemoteDownloadAgent struct {
	fileRemoteDownloadAgent
	status   int
	started  <-chan struct{}
	readDone chan error
}

type observedCloseBody struct {
	closed chan struct{}
	once   sync.Once
}

func (body *observedCloseBody) Read([]byte) (int, error) { return 0, io.EOF }

func (body *observedCloseBody) Close() error {
	body.once.Do(func() { close(body.closed) })
	return nil
}

type cancelledFileRemoteDownloadAgent struct {
	fileRemoteDownloadAgent
	started      chan struct{}
	release      chan struct{}
	responseBody *observedCloseBody
}

func (agent *cancelledFileRemoteDownloadAgent) OpenStream(
	ctx context.Context,
	_, _, _, _ string,
	_ io.Reader,
	_ http.Header,
	_ int64,
) (*http.Response, error) {
	close(agent.started)
	<-ctx.Done()
	<-agent.release
	return &http.Response{
		StatusCode: http.StatusConflict,
		Header:     make(http.Header),
		Body:       agent.responseBody,
	}, nil
}

func (agent *earlyFileRemoteDownloadAgent) OpenStream(
	_ context.Context,
	_, _, _, _ string,
	body io.Reader,
	_ http.Header,
	_ int64,
) (*http.Response, error) {
	go func() {
		_, err := io.Copy(io.Discard, body)
		agent.readDone <- err
	}()
	<-agent.started
	return &http.Response{
		StatusCode: agent.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"code":"early_rejection"}`)),
	}, nil
}

func (agent *fileRemoteDownloadAgent) Get(
	_ context.Context,
	requestPath string,
	rawQuery string,
	_ string,
) (AgentResponse, error) {
	if requestPath != "/v1/files/entry" {
		return AgentResponse{}, nil
	}
	query, _ := url.ParseQuery(rawQuery)
	target := query.Get("path")
	if target == "/home" {
		body, _ := json.Marshal(contract.FileEntry{Name: "home", Path: "/home", Kind: "directory"})
		return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}, nil
	}
	agent.mu.Lock()
	entry, exists := agent.existing[target]
	agent.mu.Unlock()
	if !exists {
		return AgentResponse{StatusCode: http.StatusNotFound, ContentType: "application/problem+json"}, nil
	}
	body, _ := json.Marshal(entry)
	return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}, nil
}

func (agent *fileRemoteDownloadAgent) Do(
	context.Context,
	string,
	string,
	string,
	string,
	[]byte,
) (AgentResponse, error) {
	return AgentResponse{}, nil
}

func (agent *fileRemoteDownloadAgent) OpenStream(
	_ context.Context,
	method, requestPath, rawQuery, _ string,
	body io.Reader,
	headers http.Header,
	contentLength int64,
) (*http.Response, error) {
	content, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	agent.mu.Lock()
	agent.streamCalls = append(agent.streamCalls, streamAgentCall{
		method: method, path: requestPath, rawQuery: rawQuery, headers: headers.Clone(),
		body: content, contentLength: contentLength,
	})
	agent.mu.Unlock()
	status := agent.streamStatus
	if status == 0 {
		status = http.StatusCreated
	}
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}},
		ContentLength: int64(len(agent.streamResponse)),
		Body:          io.NopCloser(bytes.NewReader(agent.streamResponse)),
	}, nil
}

func (agent *fileRemoteDownloadAgent) calls() []streamAgentCall {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return append([]streamAgentCall(nil), agent.streamCalls...)
}

func TestFileRemoteDownloadStreamsToAtomicAgentUploadAndAuditsRedactedSource(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	entry := contract.FileEntry{
		Name: "download (1)", Path: "/home/download (1)", Kind: "file", SizeBytes: 7,
		ResourceVersion: "sha256:test-entry",
	}
	entryBody, _ := json.Marshal(entry)
	agent := &fileRemoteDownloadAgent{
		existing: map[string]contract.FileEntry{
			"/home/download": {Name: "download", Path: "/home/download", Kind: "file"},
		},
		streamResponse: entryBody,
	}
	server.agent = agent
	opened := 0
	server.remoteDownloadOpen = func(_ context.Context, raw string) (*http.Response, error) {
		opened++
		if raw != "https://downloads.example.com/build/source-bearer-token?token=secret-token" {
			t.Fatalf("opened URL = %q", raw)
		}
		request, _ := http.NewRequest(
			http.MethodGet,
			"https://cdn.example.net/assets/redirect-bearer-token?signature=redirect-secret-token",
			nil,
		)
		return &http.Response{
			StatusCode: http.StatusOK, Request: request,
			Header:        make(http.Header),
			ContentLength: 7, Body: io.NopCloser(strings.NewReader("payload")),
		}, nil
	}
	body := []byte(`{"url":"https://downloads.example.com/build/source-bearer-token?token=secret-token","targetDirectory":"/home"}`)
	response := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusOK || opened != 1 {
		t.Fatalf("response = %d %s opened=%d", response.Code, response.Body.String(), opened)
	}
	events := decodeFileRemoteDownloadEvents(t, response.Body.String())
	if len(events) < 4 || events[0].State != "connecting" || events[len(events)-1].State != "complete" ||
		events[len(events)-1].Entry == nil || events[len(events)-1].Entry.Path != entry.Path {
		t.Fatalf("events = %#v", events)
	}
	calls := agent.calls()
	if len(calls) != 1 || calls[0].method != http.MethodPost || calls[0].path != "/v1/files/upload" ||
		calls[0].body == nil || string(calls[0].body) != "payload" || calls[0].contentLength != 7 {
		t.Fatalf("stream calls = %#v", calls)
	}
	query, _ := url.ParseQuery(calls[0].rawQuery)
	if query.Get("path") != "/home" || query.Get("name") != "download (1)" || query.Get("overwrite") != "false" {
		t.Fatalf("upload query = %#v", query)
	}
	auditEvents, _, _ := server.store.ListAudit(20, "")
	serializedAudit, _ := json.Marshal(auditEvents)
	leakedValues := []string{
		"source-bearer-token", "secret-token", "redirect-bearer-token", "redirect-secret-token", "/build", "/assets",
	}
	for _, value := range leakedValues {
		if strings.Contains(response.Body.String(), value) {
			t.Fatalf("response leaked source URL value %q: %s", value, response.Body.String())
		}
		if strings.Contains(string(serializedAudit), value) {
			t.Fatalf("audit leaked source URL value %q: %s", value, serializedAudit)
		}
	}
	if !strings.Contains(string(serializedAudit), "https://downloads.example.com") {
		t.Fatalf("audit omitted safe source origin: %s", serializedAudit)
	}
}

func TestFileRemoteDownloadNameUsesOnlyExplicitOrResponseMetadata(t *testing.T) {
	request, _ := http.NewRequest(http.MethodGet, "https://cdn.example.net/bearer-path-token", nil)
	response := &http.Response{
		Request: request,
		Header: http.Header{
			"Content-Disposition": []string{"attachment; filename*=UTF-8''release%20v1.zip"},
		},
	}
	if name := fileRemoteDownloadName("chosen.tar.gz", response); name != "chosen.tar.gz" {
		t.Fatalf("explicit name = %q", name)
	}
	if name := fileRemoteDownloadName("", response); name != "release v1.zip" {
		t.Fatalf("Content-Disposition name = %q", name)
	}
	response.Header = make(http.Header)
	if name := fileRemoteDownloadName("", response); name != "download" {
		t.Fatalf("safe fallback name = %q", name)
	}
	response.Header.Set("Content-Disposition", `attachment; filename="../token"`)
	if name := fileRemoteDownloadName("", response); name != "download" {
		t.Fatalf("invalid metadata fallback name = %q", name)
	}
}

func TestFileRemoteDownloadRejectsInvalidInputAndBlockedAddressBeforeAgentWrite(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileRemoteDownloadAgent{existing: make(map[string]contract.FileEntry)}
	server.agent = agent
	opened := 0
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		opened++
		return nil, remotedownload.ErrAddressBlocked
	}
	headers := map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}
	invalid := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads",
		[]byte(`{"url":"file:///etc/passwd","targetDirectory":"/home"}`),
		sessionCookie, csrfCookie, headers,
	)
	if invalid.Code != http.StatusUnprocessableEntity || opened != 0 {
		t.Fatalf("invalid response=%d opened=%d body=%s", invalid.Code, opened, invalid.Body.String())
	}
	blocked := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads",
		[]byte(`{"url":"http://127.0.0.1/metadata","targetDirectory":"/home"}`),
		sessionCookie, csrfCookie, headers,
	)
	if blocked.Code != http.StatusOK || opened != 1 || len(agent.calls()) != 0 ||
		!strings.Contains(blocked.Body.String(), `"code":"remote_download_address_blocked"`) ||
		strings.Contains(blocked.Body.String(), "127.0.0.1") {
		t.Fatalf("blocked response=%d opened=%d calls=%#v body=%s", blocked.Code, opened, agent.calls(), blocked.Body.String())
	}
}

func TestFileRemoteDownloadBusyFailsBeforeOpeningSource(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	server.remoteDownloadGate <- struct{}{}
	server.remoteDownloadGate <- struct{}{}
	opened := false
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		opened = true
		return nil, nil
	}
	response := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads",
		[]byte(`{"url":"https://downloads.example.com/file","targetDirectory":"/home"}`),
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "1" || opened {
		t.Fatalf("busy response=%d headers=%v opened=%t", response.Code, response.Header(), opened)
	}
}

func TestFileRemoteDownloadEarlyAgentResponseClosesSourceAndWritesOneTerminalEvent(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server, tokenPath := newTestServer(t)
			sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
			body := newBlockingRemoteDownloadBody()
			agent := &earlyFileRemoteDownloadAgent{
				fileRemoteDownloadAgent: fileRemoteDownloadAgent{existing: make(map[string]contract.FileEntry)},
				status:                  status,
				started:                 body.started,
				readDone:                make(chan error, 1),
			}
			server.agent = agent
			server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
				request, _ := http.NewRequest(http.MethodGet, "https://downloads.example.com/large.bin", nil)
				return &http.Response{
					StatusCode: http.StatusOK, Request: request, Header: make(http.Header),
					ContentLength: -1, Body: body,
				}, nil
			}
			response := authenticatedRequest(
				server, http.MethodPost, "/api/v1/files/remote-downloads",
				[]byte(`{"url":"https://downloads.example.com/large.bin","targetDirectory":"/home"}`),
				sessionCookie, csrfCookie, map[string]string{
					"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
				},
			)
			events := decodeFileRemoteDownloadEvents(t, response.Body.String())
			terminalEvents := 0
			for _, event := range events {
				if event.State == "complete" || event.State == "error" {
					terminalEvents++
				}
			}
			if response.Code != http.StatusOK || terminalEvents != 1 || events[len(events)-1].State != "error" {
				t.Fatalf("status=%d events=%#v", response.Code, events)
			}
			select {
			case <-body.closed:
			case <-time.After(time.Second):
				t.Fatal("remote response body was not closed after Agent rejected the upload")
			}
			select {
			case <-agent.readDone:
			case <-time.After(time.Second):
				t.Fatal("Agent request-body reader did not stop")
			}
			if len(server.remoteDownloadGate) != 0 {
				t.Fatalf("remote download gate was not released: %d", len(server.remoteDownloadGate))
			}
		})
	}
}

func TestFileRemoteDownloadCancellationClosesLateAgentResponse(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	responseBody := &observedCloseBody{closed: make(chan struct{})}
	agent := &cancelledFileRemoteDownloadAgent{
		fileRemoteDownloadAgent: fileRemoteDownloadAgent{existing: make(map[string]contract.FileEntry)},
		started:                 make(chan struct{}),
		release:                 make(chan struct{}),
		responseBody:            responseBody,
	}
	server.agent = agent
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		request, _ := http.NewRequest(http.MethodGet, "https://downloads.example.com/file.bin", nil)
		return &http.Response{
			StatusCode: http.StatusOK, Request: request, Header: make(http.Header),
			ContentLength: 7, Body: io.NopCloser(strings.NewReader("payload")),
		}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/remote-downloads",
		strings.NewReader(`{"url":"https://downloads.example.com/file.bin","targetDirectory":"/home"}`),
	).WithContext(ctx)
	request.Host = "panel.test"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://panel.test")
	request.Header.Set("X-CSRF-Token", csrfCookie.Value)
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	recorder := httptest.NewRecorder()
	handlerDone := make(chan struct{})
	go func() {
		server.ServeHTTP(recorder, request)
		close(handlerDone)
	}()
	select {
	case <-agent.started:
	case <-time.After(time.Second):
		t.Fatal("Agent upload did not start")
	}
	cancel()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("remote download handler did not stop after cancellation")
	}
	close(agent.release)
	select {
	case <-responseBody.closed:
	case <-time.After(time.Second):
		t.Fatal("late Agent response body was not closed")
	}
	if len(server.remoteDownloadGate) != 0 {
		t.Fatalf("remote download gate was not released: %d", len(server.remoteDownloadGate))
	}
}

func TestFileRemoteDownloadLimitReaderAllowsBoundaryAndRejectsExtraByte(t *testing.T) {
	exact := &fileRemoteDownloadLimitReader{source: strings.NewReader("1234"), remaining: 4}
	content, err := io.ReadAll(exact)
	if err != nil || string(content) != "1234" || exact.exceeded.Load() {
		t.Fatalf("exact content=%q err=%v exceeded=%t", content, err, exact.exceeded.Load())
	}
	over := &fileRemoteDownloadLimitReader{source: strings.NewReader("12345"), remaining: 4}
	content, err = io.ReadAll(over)
	if !errors.Is(err, filemanager.ErrTooLarge) || string(content) != "1234" || !over.exceeded.Load() {
		t.Fatalf("over content=%q err=%v exceeded=%t", content, err, over.exceeded.Load())
	}
}

func decodeFileRemoteDownloadEvents(t *testing.T, body string) []contract.FileTransferEvent {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	events := make([]contract.FileTransferEvent, 0, len(lines))
	for _, line := range lines {
		var event contract.FileTransferEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode event %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}
