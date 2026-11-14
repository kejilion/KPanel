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
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

func TestFileConflictRemoteNameAndVersionBoundOverwrite(t *testing.T) {
	server, _ := newTestServer(t)
	_, _, root := realTransferAgent(t, server)
	file := filepath.Join(root, "home", "response.bin")
	if err := os.WriteFile(file, []byte("old"), 0640); err != nil {
		t.Fatal(err)
	}
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: 3, Header: http.Header{"Content-Disposition": {`attachment; filename="response.bin"`}, "ETag": {`"source-v1"`}}, Body: io.NopCloser(bytes.NewReader([]byte("new")))}, nil
	}
	prompts := 0
	session := &filemanager.ConflictSession{Prompt: func(_ context.Context, c contract.FileConflict) (contract.FileConflictDecision, error) {
		prompts++
		if c.Target.Path != "/home/response.bin" {
			t.Fatalf("URL name used instead of response: %+v", c)
		}
		return contract.FileConflictDecision{Policy: "keep-both", ApplyToRest: true}, nil
	}}
	result := server.executeFileRemoteDownload(filemanager.WithFileConflicts(context.Background(), session), contract.FileRemoteDownloadRequest{URL: "https://example.com/guessed-name", TargetDirectory: "/home", ConflictPolicies: &contract.FileConflictPolicies{}}, "conflict", func(contract.FileTransferEvent) bool { return true })
	if result.State != "complete" || result.Name != "response (1).bin" || prompts != 1 {
		t.Fatalf("result %+v prompts %d", result, prompts)
	}
	for name, want := range map[string]string{"response.bin": "old", "response (1).bin": "new"} {
		body, err := os.ReadFile(filepath.Join(root, "home", name))
		if err != nil || string(body) != want {
			t.Fatalf("%s=%q %v", name, body, err)
		}
	}
}

func TestFileConflictBackgroundPersistsWaitAndChecksDecisionRevision(t *testing.T) {
	server, token := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, token)
	_, _, root := realTransferAgent(t, server)
	os.WriteFile(filepath.Join(root, "home", "a.bin"), []byte("old"), 0600)
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: 3, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader([]byte("new")))}, nil
	}
	post := func(path, body string, csrf bool) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
		request.Host = "panel.test"
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://panel.test")
		request.AddCookie(sessionCookie)
		request.AddCookie(csrfCookie)
		if csrf {
			request.Header.Set("X-CSRF-Token", csrfCookie.Value)
		}
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}
	response := post("/api/v1/files/remote-downloads", `{"url":"https://example.com/a.bin?token=private","targetDirectory":"/home","name":"a.bin","background":true,"conflictPolicies":{"file":"ask","directory":"ask"}}`, true)
	if response.Code != 202 {
		t.Fatalf("create %d %s", response.Code, response.Body.String())
	}
	var job contract.FileRemoteDownloadJob
	json.Unmarshal(response.Body.Bytes(), &job)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, _ = server.remoteDownloadJobs.Get(job.ID)
		if job.State == "waiting_input" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if job.State != "waiting_input" || job.Conflict == nil || job.Revision == 0 {
		t.Fatalf("wait %+v", job)
	}
	if len(server.remoteDownloadGate) != 0 {
		t.Fatal("waiting task monopolized a transfer slot")
	}
	payload, _ := json.Marshal(contract.FileConflictDecision{Revision: job.Revision, Policy: "skip", ApplyToRest: true})
	path := "/api/v1/files/remote-downloads/" + job.ID + "/decide"
	if response := post(path, string(payload), false); response.Code != 403 {
		t.Fatalf("missing CSRF accepted %d", response.Code)
	}
	if response := post(path, string(payload), true); response.Code != 200 {
		t.Fatalf("decision %d %s", response.Code, response.Body.String())
	}
	if response := post(path, string(payload), true); response.Code != 409 {
		t.Fatalf("stale decision %d", response.Code)
	}
	for time.Now().Before(deadline) {
		job, _ = server.remoteDownloadJobs.Get(job.ID)
		if job.State == "skipped" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if job.State != "skipped" || job.FinishedAt == nil {
		t.Fatalf("not a skipped terminal %+v", job)
	}
	body, _ := os.ReadFile(filepath.Join(root, "home", "a.bin"))
	if string(body) != "old" {
		t.Fatalf("skip changed target %q", body)
	}
}

func TestFileConflictCrossDirectoryMergesWithoutBrowserFinalization(t *testing.T) {
	server, _ := newTestServer(t)
	_, m, root := realTransferAgent(t, server)
	for _, dir := range []string{"source", "home/source"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{"source/a.txt": "incoming", "source/b.txt": "new", "home/source/a.txt": "old", "home/source/only.txt": "retain"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source, _ := m.Stat("/source")
	prompts := 0
	session := &filemanager.ConflictSession{Prompt: func(_ context.Context, c contract.FileConflict) (contract.FileConflictDecision, error) {
		prompts++
		policy := "skip"
		if c.Source.Kind == "directory" {
			policy = "merge"
		}
		return contract.FileConflictDecision{Policy: policy, ApplyToRest: true}, nil
	}}
	ctx := filemanager.WithFileConflicts(context.Background(), session)
	result := server.executeCrossFileTransfer(ctx, contract.FileTransferRequest{SourceNodeID: server.cluster.NodeID(), Path: source.Path, ResourceVersion: source.ResourceVersion, TargetDirectory: "/home", ConflictPolicies: &contract.FileConflictPolicies{}}, "", "", "conflict", func(contract.FileTransferEvent) bool { return true })
	if result.State != "partial" || result.Entry == nil || prompts != 2 {
		t.Fatalf("result %+v prompts=%d", result, prompts)
	}
	for name, want := range map[string]string{"a.txt": "old", "b.txt": "new", "only.txt": "retain"} {
		body, err := os.ReadFile(filepath.Join(root, "home/source", name))
		if err != nil || string(body) != want {
			t.Fatalf("%s=%q %v", name, body, err)
		}
	}
}

func TestFileConflictChangedTargetRepromptsInsteadOfReplayingConsent(t *testing.T) {
	server, _ := newTestServer(t)
	_, _, root := realTransferAgent(t, server)
	file := filepath.Join(root, "home", "a.bin")
	os.WriteFile(file, []byte("old"), 0600)
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, ContentLength: 3, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader([]byte("new")))}, nil
	}
	prompts := 0
	session := &filemanager.ConflictSession{Prompt: func(_ context.Context, c contract.FileConflict) (contract.FileConflictDecision, error) {
		prompts++
		if prompts == 1 {
			os.WriteFile(file, []byte("changed"), 0600)
			return contract.FileConflictDecision{Policy: "overwrite", ApplyToRest: true}, nil
		}
		if !c.Changed {
			t.Fatal("changed target not marked")
		}
		return contract.FileConflictDecision{Policy: "skip"}, nil
	}}
	result := server.executeFileRemoteDownload(filemanager.WithFileConflicts(context.Background(), session), contract.FileRemoteDownloadRequest{URL: "https://example.com/a.bin", Name: "a.bin", TargetDirectory: "/home", ConflictPolicies: &contract.FileConflictPolicies{}}, "race", func(contract.FileTransferEvent) bool { return true })
	if result.State != "skipped" || prompts != 2 {
		t.Fatalf("result %+v prompts=%d", result, prompts)
	}
	got, _ := os.ReadFile(file)
	if string(got) != "changed" {
		t.Fatalf("changed target lost: %q", got)
	}
}
