package panel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	agentserver "github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

// Use the real receiving routes and filesystem, with faults only at transport
// boundaries. This catches disagreements between the Panel and Agent contract.
type transferIntegrationAgent struct {
	url                               string
	client                            *http.Client
	mu                                sync.Mutex
	exports                           []int64
	chunkBytes                        int64
	lostChunk, lostCommit, failSource bool
	commitAttempts                    int
}

func (a *transferIntegrationAgent) Get(ctx context.Context, path, query, id string) (AgentResponse, error) {
	return a.request(ctx, http.MethodGet, path, query, id, nil)
}
func (a *transferIntegrationAgent) Do(ctx context.Context, method, path, query, id string, body []byte) (AgentResponse, error) {
	return a.request(ctx, method, path, query, id, body)
}
func (a *transferIntegrationAgent) request(ctx context.Context, method, path, query, id string, body []byte) (AgentResponse, error) {
	r, err := a.OpenStream(ctx, method, path, query, id, bytes.NewReader(body), http.Header{"Content-Type": {"application/json"}}, int64(len(body)))
	if err != nil {
		return AgentResponse{}, err
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	return AgentResponse{StatusCode: r.StatusCode, ContentType: r.Header.Get("Content-Type"), Body: data}, err
}
func (a *transferIntegrationAgent) OpenStream(ctx context.Context, method, path, query, _ string, body io.Reader, headers http.Header, size int64) (*http.Response, error) {
	operation := ""
	if path == "/v1/files/transfer/sessions" && method == http.MethodPost {
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		var command contract.FileReceiveRequest
		_ = json.Unmarshal(data, &command)
		operation = command.Operation
		body = bytes.NewReader(data)
	}
	r, err := http.NewRequestWithContext(ctx, method, a.url+path+"?"+query, body)
	if err != nil {
		return nil, err
	}
	r.Header = headers.Clone()
	r.ContentLength = size
	response, err := a.client.Do(r)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if path == "/v1/files/transfer/export" {
		values, _ := url.ParseQuery(query)
		offset, _ := strconv.ParseInt(values.Get("offset"), 10, 64)
		a.exports = append(a.exports, offset)
		if a.failSource && len(a.exports) == 1 {
			response.Body = &transferCutBody{ReadCloser: response.Body, remaining: contract.FileTransferChunkBytes}
		}
	}
	if method == http.MethodPut && path == "/v1/files/transfer/sessions" {
		a.chunkBytes += size
		if a.lostChunk {
			a.lostChunk = false
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			return nil, io.ErrUnexpectedEOF // Agent has already durably accepted it.
		}
	}
	if operation == "commit" {
		a.commitAttempts++
		if a.lostCommit {
			a.lostCommit = false
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			return nil, io.ErrUnexpectedEOF // Recover by querying the same session.
		}
	}
	return response, nil
}

type transferCutBody struct {
	io.ReadCloser
	remaining int
}

func (b *transferCutBody) Read(data []byte) (int, error) {
	if b.remaining == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if len(data) > b.remaining {
		data = data[:b.remaining]
	}
	n, err := b.ReadCloser.Read(data)
	b.remaining -= n
	return n, err
}

func realTransferAgent(t *testing.T, server *Server) (*transferIntegrationAgent, *filemanager.Manager, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "home"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	httpServer := httptest.NewServer(agentserver.NewFileHandler(m))
	t.Cleanup(httpServer.Close)
	a := &transferIntegrationAgent{url: httpServer.URL, client: httpServer.Client()}
	server.agent = a
	return a, m, root
}

func TestFileTransferIntegrationResumesSourceAndLostAcknowledgements(t *testing.T) {
	server, _ := newTestServer(t)
	a, m, root := realTransferAgent(t, server)
	a.failSource, a.lostChunk, a.lostCommit = true, true, true
	data := bytes.Repeat([]byte("unified-transfer-pattern"), contract.FileTransferChunkBytes/20+123)
	if err := os.WriteFile(filepath.Join(root, "source.bin"), data, 0600); err != nil {
		t.Fatal(err)
	}
	entry, err := m.Stat("/source.bin")
	if err != nil {
		t.Fatal(err)
	}
	result := server.executeCrossFileTransfer(context.Background(), contract.FileTransferRequest{SourceNodeID: server.cluster.NodeID(), Path: entry.Path, ResourceVersion: entry.ResourceVersion, TargetDirectory: "/home"}, "", "", "integration", func(contract.FileTransferEvent) bool { return true })
	if result.State != "complete" || result.Entry == nil {
		t.Fatalf("result=%#v", result)
	}
	got, err := os.ReadFile(filepath.Join(root, "home", "source.bin"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("content mismatch: %v", err)
	}
	if len(a.exports) != 2 || a.exports[1] != contract.FileTransferChunkBytes {
		t.Fatalf("restart offsets=%v", a.exports)
	}
	if a.chunkBytes != int64(len(data)+contract.FileTransferChunkBytes) || a.commitAttempts != 1 {
		t.Fatalf("unbounded retries: bytes=%d commit=%d", a.chunkBytes, a.commitAttempts)
	}
	items, _ := os.ReadDir(filepath.Join(root, "home"))
	if len(items) != 1 {
		t.Fatalf("duplicate or partial target: %v", items)
	}
}

func TestFileTransferIntegrationURLRangeUsesSameReceiver(t *testing.T) {
	server, _ := newTestServer(t)
	a, _, root := realTransferAgent(t, server)
	data := bytes.Repeat([]byte("p"), contract.FileTransferChunkBytes+17)
	sourceURL := "https://downloads.example.com/file.bin?private=secret"
	u, _ := url.Parse(sourceURL)
	header := make(http.Header)
	header.Set("ETag", `"version-1"`)
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: header.Clone(), ContentLength: int64(len(data)), Request: &http.Request{URL: u}, Body: &transferCutBody{ReadCloser: io.NopCloser(bytes.NewReader(data)), remaining: contract.FileTransferChunkBytes}}, nil
	}
	ranges := 0
	server.remoteDownloadRangeOpen = func(_ context.Context, raw string, input remotedownload.ResumeRequest) (*http.Response, error) {
		ranges++
		if raw != sourceURL || input.Offset != contract.FileTransferChunkBytes || input.ETag != `"version-1"` {
			t.Fatalf("range=%#v", input)
		}
		return &http.Response{StatusCode: 206, Header: header.Clone(), ContentLength: 17, Request: &http.Request{URL: u}, Body: io.NopCloser(bytes.NewReader(data[input.Offset:]))}, nil
	}
	result := server.executeFileRemoteDownload(context.Background(), contract.FileRemoteDownloadRequest{URL: sourceURL, TargetDirectory: "/home", Name: "file.bin"}, "integration", func(contract.FileTransferEvent) bool { return true })
	if result.State != "complete" || ranges != 1 || a.chunkBytes != int64(len(data)) {
		t.Fatalf("result=%#v ranges=%d bytes=%d", result, ranges, a.chunkBytes)
	}
	got, _ := os.ReadFile(filepath.Join(root, "home", "file.bin"))
	if !bytes.Equal(got, data) {
		t.Fatal("range concatenation differs")
	}
	journal, _ := os.ReadFile(filepath.Join(root, ".kpanel-trash", "receive-sessions.json"))
	if bytes.Contains(journal, []byte("secret")) || bytes.Contains(journal, []byte(sourceURL)) {
		t.Fatal("source URL persisted in receiving journal")
	}
}

func TestFileTransferIntegrationSourceEndFailureNeverPublishes(t *testing.T) {
	server, _ := newTestServer(t)
	_, _, root := realTransferAgent(t, server)
	digest := sha256.Sum256([]byte("payload"))
	metadata := contract.FileTransferMetadata{Name: "source.bin", Kind: "file", SizeBytes: 7, ResourceVersion: "sha256:" + hex.EncodeToString(digest[:]), TransferVersion: 1}
	body := &localFileTransferBody{ReadCloser: io.NopCloser(strings.NewReader("payload")), response: &http.Response{Trailer: http.Header{fileTransferAgentResultTrailer: {"error"}}}}
	_, _, err := server.receiveFileTransfer(context.Background(), contract.FileReceiveInput{Directory: "/home", Name: metadata.Name, Kind: "file", SizeBytes: 7, SourceKey: strings.Repeat("a", 64)}, body, nil, server.fileReceiver("", "", "integration"), func(contract.FileTransferEvent) bool { return true }, nil)
	if !errors.Is(err, filemanager.ErrReceiveChecksum) {
		t.Fatalf("end failure=%v", err)
	}
	items, _ := os.ReadDir(filepath.Join(root, "home"))
	if len(items) != 0 {
		t.Fatalf("published incomplete source: %v", items)
	}
}

func TestFileTransferIntegrationBackgroundSurvivesSubmittingRequest(t *testing.T) {
	server, tokenPath := newTestServer(t)
	cookie, csrf := bootstrapCookies(t, server, tokenPath)
	_, m, root := realTransferAgent(t, server)
	if err := os.WriteFile(filepath.Join(root, "source.bin"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	entry, _ := m.Stat("/source.bin")
	input := contract.FileTransferRequest{SourceNodeID: server.cluster.NodeID(), Path: entry.Path, ResourceVersion: entry.ResourceVersion, TargetDirectory: "/home", Background: true}
	payload, _ := json.Marshal(input)
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodPost, "/api/v1/files/transfers", bytes.NewReader(payload)).WithContext(ctx)
	r.Host = "panel.test"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://panel.test")
	r.Header.Set("X-CSRF-Token", csrf.Value)
	r.AddCookie(cookie)
	r.AddCookie(csrf)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	cancel()
	if w.Code != http.StatusAccepted {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	var created contract.FileRemoteDownloadJob
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := server.remoteDownloadJobs.Get(created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.State == "complete" {
			if job.SourceKind != "cross-host" || job.Entry == nil {
				t.Fatalf("job=%#v", job)
			}
			return
		}
		if job.State == "error" {
			t.Fatalf("job=%#v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("accepted job did not finish after submitting context closed")
}
