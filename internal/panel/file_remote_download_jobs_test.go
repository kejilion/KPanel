package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
	"github.com/kejilion/kejilion-panel/internal/store"
)

type confirmingFileRemoteDownloadAgent struct {
	fileRemoteDownloadAgent
	resultBody *blockingRemoteDownloadBody
}

type blockingRemoteDownloadEntryAgent struct {
	fileRemoteDownloadAgent
	started chan struct{}
}

func (agent *blockingRemoteDownloadEntryAgent) Get(
	ctx context.Context,
	_, _, _ string,
) (AgentResponse, error) {
	select {
	case <-agent.started:
	default:
		close(agent.started)
	}
	<-ctx.Done()
	return AgentResponse{}, ctx.Err()
}

func (agent *confirmingFileRemoteDownloadAgent) OpenStream(
	ctx context.Context,
	method, requestPath, rawQuery, requestID string,
	body io.Reader,
	headers http.Header,
	contentLength int64,
) (*http.Response, error) {
	response, err := agent.fileRemoteDownloadAgent.OpenStream(
		ctx, method, requestPath, rawQuery, requestID, body, headers, contentLength,
	)
	if err != nil {
		return nil, err
	}
	_ = response.Body.Close()
	response.Body = agent.resultBody
	response.ContentLength = -1
	return response, nil
}

func TestFileRemoteDownloadBackgroundDetachesListsRedactsAndDeletes(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	const (
		rawURL     = "https://downloads.example.com/private/path-bearer-secret/file.iso?token=query-secret"
		targetName = "artifact.bin"
	)
	entry := contract.FileEntry{
		Name: targetName, Path: "/home/" + targetName, Kind: "file", SizeBytes: 7,
		ResourceVersion: "sha256:background-entry",
	}
	entryBody, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	agent := &fileRemoteDownloadAgent{existing: make(map[string]contract.FileEntry), streamResponse: entryBody}
	server.agent = agent
	type openObservation struct {
		ctx context.Context
		raw string
	}
	opened := make(chan openObservation, 1)
	release := make(chan struct{})
	server.remoteDownloadOpen = func(ctx context.Context, raw string) (*http.Response, error) {
		opened <- openObservation{ctx: ctx, raw: raw}
		select {
		case <-release:
			return &http.Response{
				StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: 7,
				Body: io.NopCloser(strings.NewReader("payload")),
			}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	requestContext, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/files/remote-downloads",
		bytes.NewBufferString(`{"url":"`+rawURL+`","targetDirectory":"/home","name":"`+targetName+`","background":true}`),
	).WithContext(requestContext)
	request.Host = "panel.test"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://panel.test")
	request.Header.Set("X-CSRF-Token", csrfCookie.Value)
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	postResponse := httptest.NewRecorder()
	server.ServeHTTP(postResponse, request)
	if postResponse.Code != http.StatusAccepted {
		t.Fatalf("background POST = %d %s, want 202", postResponse.Code, postResponse.Body.String())
	}
	var accepted contract.FileRemoteDownloadJob
	if err := json.Unmarshal(postResponse.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted job: %v", err)
	}
	if accepted.ID == "" || accepted.State != "queued" || accepted.Source != "https://downloads.example.com" {
		t.Fatalf("accepted job = %#v", accepted)
	}

	var observation openObservation
	select {
	case observation = <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("background download did not open upstream")
	}
	if observation.raw != rawURL {
		t.Fatalf("opened URL = %q", observation.raw)
	}
	cancelRequest()
	select {
	case <-observation.ctx.Done():
		t.Fatalf("background work inherited request cancellation: %v", observation.ctx.Err())
	default:
	}
	close(release)
	waitFileRemoteDownloadWorkers(t, server)

	getResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/remote-downloads/"+accepted.ID, nil,
		sessionCookie, csrfCookie, nil,
	)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET job = %d %s", getResponse.Code, getResponse.Body.String())
	}
	var completed contract.FileRemoteDownloadJob
	if err := json.Unmarshal(getResponse.Body.Bytes(), &completed); err != nil {
		t.Fatalf("decode completed job: %v", err)
	}
	if completed.State != "complete" || completed.FinishedAt == nil || completed.Entry == nil ||
		completed.Entry.Path != entry.Path || completed.LoadedBytes != 7 {
		t.Fatalf("completed job = %#v", completed)
	}

	listResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/remote-downloads", nil,
		sessionCookie, csrfCookie, nil,
	)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("GET jobs = %d %s", listResponse.Code, listResponse.Body.String())
	}
	var listed contract.FileRemoteDownloadJobList
	if err := json.Unmarshal(listResponse.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode job list: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != accepted.ID || listed.Items[0].State != "complete" {
		t.Fatalf("listed jobs = %#v", listed.Items)
	}
	if calls := agent.calls(); len(calls) != 1 || string(calls[0].body) != "payload" {
		t.Fatalf("Agent stream calls = %#v", calls)
	}

	auditEvents, _, _ := server.store.ListAudit(50, "")
	auditJSON, err := json.Marshal(auditEvents)
	if err != nil {
		t.Fatal(err)
	}
	jobIndex, err := os.ReadFile(filepath.Join(filepath.Dir(tokenPath), "remote-downloads", "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	privacySurfaces := map[string]string{
		"POST response": postResponse.Body.String(),
		"GET response":  getResponse.Body.String(),
		"list response": listResponse.Body.String(),
		"job index":     string(jobIndex),
		"audit":         string(auditJSON),
	}
	for surface, value := range privacySurfaces {
		for _, secret := range []string{"path-bearer-secret", "query-secret", "/private/", "file.iso"} {
			if strings.Contains(value, secret) {
				t.Fatalf("%s leaked URL secret %q: %s", surface, secret, value)
			}
		}
		if !strings.Contains(value, "https://downloads.example.com") {
			t.Fatalf("%s omitted safe source origin: %s", surface, value)
		}
	}

	deleteResponse := authenticatedRequest(
		server, http.MethodDelete, "/api/v1/files/remote-downloads/"+accepted.ID, nil,
		sessionCookie, csrfCookie, map[string]string{
			"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("DELETE terminal job = %d %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	missingResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/remote-downloads/"+accepted.ID, nil,
		sessionCookie, csrfCookie, nil,
	)
	if missingResponse.Code != http.StatusNotFound {
		t.Fatalf("GET deleted job = %d %s, want 404", missingResponse.Code, missingResponse.Body.String())
	}
}

func TestFileRemoteDownloadBackgroundCancelClosesUpstreamWithoutAgentCommit(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	agent := &fileRemoteDownloadAgent{existing: make(map[string]contract.FileEntry)}
	server.agent = agent
	upstreamBody := newBlockingRemoteDownloadBody()
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: -1, Body: upstreamBody,
		}, nil
	}
	postResponse := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads",
		[]byte(`{"url":"https://downloads.example.com/cancel-me","targetDirectory":"/home","name":"cancelled.bin","background":true}`),
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if postResponse.Code != http.StatusAccepted {
		t.Fatalf("background POST = %d %s, want 202", postResponse.Code, postResponse.Body.String())
	}
	var accepted contract.FileRemoteDownloadJob
	if err := json.Unmarshal(postResponse.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode accepted job: %v", err)
	}
	select {
	case <-upstreamBody.started:
	case <-time.After(5 * time.Second):
		t.Fatal("background upload did not start reading upstream")
	}

	cancelResponse := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads/"+accepted.ID+"/cancel", nil,
		sessionCookie, csrfCookie, map[string]string{
			"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if cancelResponse.Code != http.StatusAccepted {
		t.Fatalf("cancel active job = %d %s, want 202", cancelResponse.Code, cancelResponse.Body.String())
	}
	select {
	case <-upstreamBody.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling active job did not close upstream body")
	}
	waitFileRemoteDownloadWorkers(t, server)

	getResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/remote-downloads/"+accepted.ID, nil,
		sessionCookie, csrfCookie, nil,
	)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET cancelled job = %d %s", getResponse.Code, getResponse.Body.String())
	}
	var cancelled contract.FileRemoteDownloadJob
	if err := json.Unmarshal(getResponse.Body.Bytes(), &cancelled); err != nil {
		t.Fatalf("decode cancelled job: %v", err)
	}
	if cancelled.State != "cancelled" || cancelled.Code != "remote_download_cancelled" || cancelled.FinishedAt == nil {
		t.Fatalf("cancelled job = %#v", cancelled)
	}
	if calls := agent.calls(); len(calls) != 0 {
		t.Fatalf("cancelled download committed Agent upload: %#v", calls)
	}
}

func TestFileRemoteDownloadBackgroundQueueIsBounded(t *testing.T) {
	server, _ := newTestServer(t)
	type reservation struct {
		id     string
		cancel context.CancelCauseFunc
	}
	reservations := make([]reservation, 0, remotedownload.MaxQueuedJobs)
	for index := range remotedownload.MaxQueuedJobs {
		id := strings.Repeat(string(rune('a'+index)), 32)
		ctx, cancel, ok := server.reserveRemoteDownloadJob(id)
		if !ok || ctx == nil || cancel == nil {
			t.Fatalf("reservation %d was rejected", index)
		}
		reservations = append(reservations, reservation{id: id, cancel: cancel})
	}
	if _, _, ok := server.reserveRemoteDownloadJob(strings.Repeat("z", 32)); ok {
		t.Fatal("queue accepted a job above its bound")
	}
	for _, item := range reservations {
		server.releaseRemoteDownloadReservation(item.id, item.cancel)
		server.remoteDownloadWG.Done()
	}
}

func TestFileRemoteDownloadJobAPIsRequireSessionOriginAndCSRF(t *testing.T) {
	server, tokenPath := newTestServer(t)
	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/files/remote-downloads", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list = %d %s", unauthenticated.Code, unauthenticated.Body.String())
	}
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	jobID := strings.Repeat("a", 32)
	missingOrigin := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads/"+jobID+"/cancel", nil,
		sessionCookie, csrfCookie, map[string]string{"X-CSRF-Token": csrfCookie.Value},
	)
	if missingOrigin.Code != http.StatusForbidden || !strings.Contains(missingOrigin.Body.String(), "origin_validation_failed") {
		t.Fatalf("missing Origin = %d %s", missingOrigin.Code, missingOrigin.Body.String())
	}
	missingCSRF := authenticatedRequest(
		server, http.MethodDelete, "/api/v1/files/remote-downloads/"+jobID, nil,
		sessionCookie, csrfCookie, map[string]string{"Origin": "http://panel.test"},
	)
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF = %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}
}

func TestFileRemoteDownloadServerCloseInterruptsQueuedJob(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	server.remoteDownloadGate <- struct{}{}
	server.remoteDownloadGate <- struct{}{}
	opened := false
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		opened = true
		return nil, nil
	}
	accepted := createBackgroundRemoteDownloadForTest(t, server, sessionCookie, csrfCookie, "queued.bin")
	server.closeRemoteDownloadJobs()
	<-server.remoteDownloadGate
	<-server.remoteDownloadGate
	job, err := server.remoteDownloadJobs.Get(accepted.ID)
	if err != nil || job.State != "interrupted" || job.Code != "remote_download_interrupted" || job.FinishedAt == nil {
		t.Fatalf("queued close job=%#v err=%v", job, err)
	}
	if opened {
		t.Fatal("queued job opened its source during server close")
	}
}

func TestFileRemoteDownloadServerCloseInterruptsTransferringJob(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	server.agent = &fileRemoteDownloadAgent{existing: make(map[string]contract.FileEntry)}
	upstreamBody := newBlockingRemoteDownloadBody()
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: -1, Body: upstreamBody,
		}, nil
	}
	accepted := createBackgroundRemoteDownloadForTest(t, server, sessionCookie, csrfCookie, "transferring.bin")
	select {
	case <-upstreamBody.started:
	case <-time.After(5 * time.Second):
		t.Fatal("background upload did not begin transferring")
	}
	server.closeRemoteDownloadJobs()
	select {
	case <-upstreamBody.closed:
	default:
		t.Fatal("server close did not close the transferring source")
	}
	job, err := server.remoteDownloadJobs.Get(accepted.ID)
	if err != nil || job.State != "interrupted" || job.Code != "remote_download_interrupted" || job.FinishedAt == nil {
		t.Fatalf("transferring close job=%#v err=%v", job, err)
	}
}

func TestFileRemoteDownloadServerCloseInterruptsConfirmingJob(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	resultBody := newBlockingRemoteDownloadBody()
	server.agent = &confirmingFileRemoteDownloadAgent{
		fileRemoteDownloadAgent: fileRemoteDownloadAgent{existing: make(map[string]contract.FileEntry)},
		resultBody:              resultBody,
	}
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: 7,
			Body: io.NopCloser(strings.NewReader("payload")),
		}, nil
	}
	accepted := createBackgroundRemoteDownloadForTest(t, server, sessionCookie, csrfCookie, "confirming.bin")
	select {
	case <-resultBody.started:
	case <-time.After(5 * time.Second):
		t.Fatal("background job did not begin confirming")
	}
	beforeClose, err := server.remoteDownloadJobs.Get(accepted.ID)
	if err != nil || beforeClose.State != "confirming" {
		t.Fatalf("before close job=%#v err=%v", beforeClose, err)
	}
	server.closeRemoteDownloadJobs()
	select {
	case <-resultBody.closed:
	default:
		t.Fatal("server close did not close the Agent result body")
	}
	job, err := server.remoteDownloadJobs.Get(accepted.ID)
	if err != nil || job.State != "interrupted" || job.Code != "remote_download_interrupted" || job.FinishedAt == nil {
		t.Fatalf("confirming close job=%#v err=%v", job, err)
	}
}

func TestFileRemoteDownloadTargetCheckUsesCancellationCause(t *testing.T) {
	for _, test := range []struct {
		name      string
		close     bool
		wantState string
		wantCode  string
	}{
		{name: "user cancel", wantState: "cancelled", wantCode: "remote_download_cancelled"},
		{name: "server close", close: true, wantState: "interrupted", wantCode: "remote_download_interrupted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, tokenPath := newTestServer(t)
			sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
			agent := &blockingRemoteDownloadEntryAgent{
				fileRemoteDownloadAgent: fileRemoteDownloadAgent{existing: make(map[string]contract.FileEntry)},
				started:                 make(chan struct{}),
			}
			server.agent = agent
			accepted := createBackgroundRemoteDownloadForTest(t, server, sessionCookie, csrfCookie, "target-check.bin")
			select {
			case <-agent.started:
			case <-time.After(5 * time.Second):
				t.Fatal("target check did not start")
			}
			if test.close {
				server.closeRemoteDownloadJobs()
			} else {
				response := authenticatedRequest(
					server, http.MethodPost, "/api/v1/files/remote-downloads/"+accepted.ID+"/cancel", nil,
					sessionCookie, csrfCookie, map[string]string{
						"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
					},
				)
				if response.Code != http.StatusAccepted {
					t.Fatalf("cancel = %d %s", response.Code, response.Body.String())
				}
				waitFileRemoteDownloadWorkers(t, server)
			}
			job, err := server.remoteDownloadJobs.Get(accepted.ID)
			if err != nil || job.State != test.wantState || job.Code != test.wantCode || job.FinishedAt == nil {
				t.Fatalf("job=%#v err=%v", job, err)
			}
		})
	}
}

func TestNewServerDegradesBrokenRemoteDownloadIndexAndKeepsSyncDownload(t *testing.T) {
	server, tokenPath, jobRoot, original := newTestServerWithBrokenRemoteDownloadRoot(t)
	if server.remoteDownloadJobs == nil || server.remoteDownloadJobs.Available() {
		t.Fatalf("remote download job store = %#v, want non-nil unavailable store", server.remoteDownloadJobs)
	}
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	listResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/remote-downloads", nil,
		sessionCookie, csrfCookie, nil,
	)
	getResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/files/remote-downloads/"+strings.Repeat("a", 32), nil,
		sessionCookie, csrfCookie, nil,
	)
	for name, response := range map[string]*httptest.ResponseRecorder{
		"list": listResponse,
		"get":  getResponse,
	} {
		if response.Code != http.StatusServiceUnavailable ||
			!strings.Contains(response.Body.String(), `"code":"remote_download_jobs_unavailable"`) {
			t.Fatalf("%s = %d %s, want unavailable", name, response.Code, response.Body.String())
		}
	}

	entry := contract.FileEntry{
		Name: "sync.bin", Path: "/home/sync.bin", Kind: "file", SizeBytes: 7,
		ResourceVersion: "sha256:sync-with-broken-job-index",
	}
	entryBody, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	server.agent = &fileRemoteDownloadAgent{
		existing: make(map[string]contract.FileEntry), streamResponse: entryBody,
	}
	opened := 0
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		opened++
		return &http.Response{
			StatusCode: http.StatusOK, Header: make(http.Header), ContentLength: 7,
			Body: io.NopCloser(strings.NewReader("payload")),
		}, nil
	}
	mutationHeaders := map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	}
	backgroundResponse := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads",
		[]byte(`{"url":"https://downloads.example.com/background.bin","targetDirectory":"/home","name":"background.bin","background":true}`),
		sessionCookie, csrfCookie, mutationHeaders,
	)
	if backgroundResponse.Code != http.StatusServiceUnavailable || opened != 0 ||
		!strings.Contains(backgroundResponse.Body.String(), `"code":"remote_download_jobs_unavailable"`) {
		t.Fatalf("background POST = %d %s opened=%d", backgroundResponse.Code, backgroundResponse.Body.String(), opened)
	}

	syncResponse := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads",
		[]byte(`{"url":"https://downloads.example.com/sync.bin","targetDirectory":"/home","name":"sync.bin"}`),
		sessionCookie, csrfCookie, mutationHeaders,
	)
	if syncResponse.Code != http.StatusOK || opened != 1 {
		t.Fatalf("sync POST = %d %s opened=%d", syncResponse.Code, syncResponse.Body.String(), opened)
	}
	events := decodeFileRemoteDownloadEvents(t, syncResponse.Body.String())
	if len(events) == 0 || events[len(events)-1].State != "complete" ||
		events[len(events)-1].Entry == nil || events[len(events)-1].Entry.Path != entry.Path {
		t.Fatalf("sync events = %#v", events)
	}

	info, err := os.Lstat(jobRoot)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(jobRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || !bytes.Equal(content, original) {
		t.Fatalf("broken job index path was changed: mode=%s content=%q", info.Mode(), content)
	}
}

func newTestServerWithBrokenRemoteDownloadRoot(t *testing.T) (*Server, string, string, []byte) {
	t.Helper()
	directory := t.TempDir()
	dataDir := filepath.Join(directory, "data")
	webRoot := filepath.Join(directory, "web")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(webRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte("<!doctype html><title>panel</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	jobRoot := filepath.Join(dataDir, "remote-downloads")
	original := []byte("preserve broken remote download job root")
	if err := os.WriteFile(jobRoot, original, 0o600); err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.DataDir = dataDir
	config.StorePath = filepath.Join(dataDir, "state.json")
	config.BootstrapTokenPath = filepath.Join(dataDir, "bootstrap.token")
	config.AgentSocket = filepath.Join(directory, "run", "agent.sock")
	config.AgentTokenFile = filepath.Join(directory, "secrets", "agent.token")
	config.WebRoot = webRoot
	config.PublicURL = "http://panel.test"
	config.SecureCookie = false
	config.CookieName = "kejilion_session"
	config.SessionTTL = time.Hour
	config.SessionTTLText = "1h"
	storage, err := store.Open(config.StorePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	hasher, err := auth.NewArgon2idHasher(auth.Argon2idParams{
		MemoryKiB: 8 * 1024, Iterations: 1, Threads: 1, SaltLength: 16, KeyLength: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	authService, err := auth.NewService(storage, hasher, auth.Config{
		BootstrapTokenPath: config.BootstrapTokenPath,
		SessionTTL:         time.Hour,
		LoginWindow:        time.Minute,
		MaxLoginFailures:   3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := authService.EnsureBootstrapToken(); err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(
		config,
		authService,
		storage,
		NewAgentClient(config.AgentSocket, config.AgentTokenFile, config.MaxAgentBytes),
	)
	if err != nil {
		t.Fatalf("NewServer with broken remote download index: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server, config.BootstrapTokenPath, jobRoot, original
}

func createBackgroundRemoteDownloadForTest(
	t *testing.T,
	server *Server,
	sessionCookie, csrfCookie *http.Cookie,
	name string,
) contract.FileRemoteDownloadJob {
	t.Helper()
	response := authenticatedRequest(
		server, http.MethodPost, "/api/v1/files/remote-downloads",
		[]byte(`{"url":"https://downloads.example.com/file","targetDirectory":"/home","name":"`+name+`","background":true}`),
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusAccepted {
		t.Fatalf("background POST = %d %s", response.Code, response.Body.String())
	}
	var job contract.FileRemoteDownloadJob
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	return job
}

func waitFileRemoteDownloadWorkers(t *testing.T, server *Server) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		server.remoteDownloadWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("background remote download workers did not finish")
	}
}
