//go:build linux

package panel

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/filetransfer"
)

func TestFileTransferJobsRealAgentsBidirectionalPartialAndIdempotent(t *testing.T) {
	center, remoteID, remoteRoot := realAgentFileStream(t, 2*time.Minute)
	s, token := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, token)
	_ = s.cluster.Close()
	s.cluster = center
	localRoot := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: localRoot})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	backend := httptest.NewServer(agent.NewFileHandler(manager))
	t.Cleanup(backend.Close)
	tokenPath := filepath.Join(t.TempDir(), "agent-token")
	if err := os.WriteFile(tokenPath, []byte("diagnostic-only-token"), 0600); err != nil {
		t.Fatal(err)
	}
	client := NewAgentClient("unused", tokenPath, 8<<20)
	client.client.Transport.(*http.Transport).DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", backend.Listener.Addr().String())
	}
	t.Cleanup(client.client.CloseIdleConnections)
	s.agent = client
	write := func(root, name, content string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(localRoot, "one.txt", "new content")
	write(localRoot, "folder/nested.txt", "directory content")
	write(remoteRoot, "one.txt", "existing content")
	write(remoteRoot, "remote.txt", "remote to local")
	entry := func(hostID, p string) contract.FileEntry {
		t.Helper()
		var data []byte
		if hostID == "" {
			response, err := client.Get(context.Background(), "/v1/files/entry", url.Values{"path": {p}}.Encode(), "test")
			if err != nil {
				t.Fatal(err)
			}
			data = response.Body
		} else {
			response, err := center.OpenRemotePanelFile(context.Background(), hostID, cluster.LightFileRequest{Method: "GET", Path: "/v1/files/entry", RawQuery: url.Values{"path": {p}}.Encode()})
			if err != nil {
				t.Fatal(err)
			}
			data, err = io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		var value contract.FileEntry
		if json.Unmarshal(data, &value) != nil || value.ResourceVersion == "" {
			t.Fatalf("entry=%s", data)
		}
		return value
	}
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	create := func(input filetransfer.Request) filetransfer.Job {
		t.Helper()
		body, _ := json.Marshal(input)
		response := authenticatedRequest(s, http.MethodPost, "/api/v1/files/transfer-jobs", body, session, csrf, headers)
		if response.Code != http.StatusAccepted {
			t.Fatalf("create=%d %s", response.Code, response.Body.String())
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			jobs, err := s.fileTransferJobs.List()
			if err != nil {
				t.Fatal(err)
			}
			for _, job := range jobs {
				if job.ID == input.ID && !filetransfer.Active(job.State) {
					return job
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("copy did not finish")
		return filetransfer.Job{}
	}
	input := filetransfer.Request{ID: strings.Repeat("d", 32), SourceNodeID: center.NodeID(), TargetHostID: remoteID, TargetDirectory: "/", Items: []filetransfer.Source{
		{Path: "/one.txt", ResourceVersion: entry("", "/one.txt").ResourceVersion},
		{Path: "/missing.txt", ResourceVersion: "missing"},
		{Path: "/folder", ResourceVersion: entry("", "/folder").ResourceVersion},
	}}
	job := create(input)
	if job.State != "partial" || job.Items[0].State != "complete" || !job.Items[1].Retryable || job.Items[2].State != "complete" {
		t.Fatalf("copy result=%+v", job)
	}
	if job.Items[0].Entry.Path != "/one (1).txt" {
		t.Fatalf("name collision=%+v", job.Items[0])
	}
	check := func(root, name, want string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != want {
			t.Fatalf("%s=%q %v", name, data, err)
		}
	}
	check(remoteRoot, "one.txt", "existing content")
	check(remoteRoot, "one (1).txt", "new content")
	check(remoteRoot, "folder/nested.txt", "directory content")
	check(localRoot, "one.txt", "new content")
	duplicate := create(input)
	if duplicate.Items[0].Entry.Path != job.Items[0].Entry.Path {
		t.Fatal("duplicate copied again")
	}
	if _, err := os.Stat(filepath.Join(remoteRoot, "one (2).txt")); !os.IsNotExist(err) {
		t.Fatalf("duplicate artifact=%v", err)
	}
	host, err := center.Host(context.Background(), remoteID)
	if err != nil {
		t.Fatal(err)
	}
	reverse := create(filetransfer.Request{ID: strings.Repeat("e", 32), SourceNodeID: host.RemoteNodeID, TargetDirectory: "/", Items: []filetransfer.Source{{Path: "/remote.txt", ResourceVersion: entry(remoteID, "/remote.txt").ResourceVersion}}})
	if reverse.State != "complete" {
		t.Fatalf("reverse=%+v", reverse)
	}
	check(localRoot, "remote.txt", "remote to local")
}
