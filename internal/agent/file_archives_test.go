package agent

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

func TestArchiveJobHTTPRoundTripUsesRealFiles(t *testing.T) {
	s := testServer(t)
	root := t.TempDir()
	m, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	s.files = m
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	source, err := m.Stat("/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	input := contract.FileActionRequest{Action: "compress", Sources: []string{source.Path}, Target: "/", Name: "hello.zip", Format: "zip", ExpectedResourceVersion: source.ResourceVersion}
	body, _ := json.Marshal(contract.FileArchiveJobRequest{Operation: "create", Input: &input})
	created := fileRequest(s, http.MethodPost, "/v1/files/archive-jobs", string(body))
	var job contract.FileArchiveJob
	if created.Code != 202 || json.Unmarshal(created.Body.Bytes(), &job) != nil || job.ID == "" {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response := fileRequest(s, http.MethodGet, "/v1/files/archive-jobs?id="+job.ID, "")
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &job) != nil {
			t.Fatalf("detail=%d %s", response.Code, response.Body.String())
		}
		if job.State == "complete" {
			break
		}
		if job.State == "error" || job.State == "interrupted" {
			t.Fatalf("job=%+v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.State != "complete" {
		t.Fatalf("job timed out=%+v", job)
	}
	archive, err := m.Stat("/hello.zip")
	if err != nil {
		t.Fatal(err)
	}
	query, _ := json.Marshal(contract.FileArchiveQuery{Path: archive.Path, ResourceVersion: archive.ResourceVersion})
	response := fileRequest(s, http.MethodPost, "/v1/files/archive-contents", string(query))
	var contents contract.FileArchiveDirectory
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &contents) != nil || len(contents.Entries) != 1 || contents.Entries[0].Path != "hello.txt" {
		t.Fatalf("contents=%d %s", response.Code, response.Body.String())
	}
	for _, target := range []string{"/v1/files/archive-jobs?id=a&id=b", "/v1/files/archive-contents?unknown=1"} {
		if response := fileRequest(s, http.MethodPost, target, string(query)); response.Code != 400 {
			t.Fatalf("bad query=%d", response.Code)
		}
	}
}
