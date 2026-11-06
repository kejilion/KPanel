//go:build linux

package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"golang.org/x/sys/unix"
)

func TestStableReadLeaseRejectsExistingWriterAndMapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte("synthetic fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if lease, err := tryStableReadLease(reader, nil); err == nil {
		lease.close()
		t.Fatal("lease accepted existing writer")
	}
	mapping, err := unix.Mmap(int(writer.Fd()), 0, len("synthetic fixture"), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Munmap(mapping)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if lease, err := tryStableReadLease(reader, nil); err == nil {
		lease.close()
		t.Fatal("lease accepted existing writable mapping")
	}
}

func TestAITailFailsClosedForActiveWriterAndKeepsAdminTail(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	path := filepath.Join(root, "live.log")
	if err := os.WriteFile(path, []byte("ordinary synthetic log\n"), 0600); err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	ai := fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Flive.log", "")
	if ai.Code != http.StatusUnprocessableEntity || !strings.Contains(ai.Body.String(), "projection_unavailable") || strings.Contains(ai.Body.String(), "ordinary synthetic") {
		t.Fatalf("AI tail status=%d body=%s", ai.Code, ai.Body.String())
	}
	admin := fileRequest(server, http.MethodGet, "/v1/files/tail?path=%2Flive.log", "")
	if admin.Code != http.StatusOK || !strings.Contains(admin.Body.String(), "ordinary synthetic") {
		t.Fatalf("admin tail status=%d body=%s", admin.Code, admin.Body.String())
	}
}

func TestAITailPathRevalidationUsesExistingDownloadSlot(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	path := filepath.Join(root, "fixture.log")
	if err := os.WriteFile(path, []byte("ordinary log line\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		file, _, err := manager.Open(context.Background(), "/fixture.log")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
	}
	response := fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Ffixture.log", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "ordinary log line") {
		t.Fatalf("projection with full download capacity status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAITailRedactsTextAuthAssignment(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	for _, value := range []string{"auth: OPAQUE_BODY\n", "auths:\n  registry.example:\n    auth: OPAQUE_BODY\n", "https://example.test/?auth=OPAQUE_BODY&safe=visible\n"} {
		if err := os.WriteFile(filepath.Join(root, "fixture.yaml"), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		response := fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Ffixture.yaml", "")
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "OPAQUE_BODY") {
			t.Fatalf("auth value exposed status=%d body=%s", response.Code, response.Body.String())
		}
	}
}

func TestAITailPathRevalidationRejectsReplacement(t *testing.T) {
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	path := filepath.Join(root, "fixture.log")
	if err := os.WriteFile(path, []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	file, entry, err := manager.Open(context.Background(), "/fixture.log")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	opened, err := file.(interface{ Stat() (os.FileInfo, error) }).Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.VerifyOpenIdentity(context.Background(), "/fixture.log", opened, entry.ResourceVersion); err == nil {
		t.Fatal("replacement path passed identity verification")
	}
}

func TestStableReadLeaseReleasesPromptlyWhenWriterArrives(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte("synthetic fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	lease, err := tryStableReadLease(reader, nil)
	if err != nil {
		t.Fatalf("fixture filesystem does not support lease: %v", err)
	}
	defer lease.close()
	if !lease.valid() {
		t.Fatal("fresh lease invalid")
	}
	written := make(chan error, 1)
	go func() {
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			err = file.Close()
		}
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("competing writer did not resume promptly")
	}
	if lease.valid() {
		t.Fatal("broken lease still valid")
	}
}

func TestAITailRejectsConcurrentWriterWithoutContent(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	path := filepath.Join(root, "concurrent.log")
	data := strings.Repeat("127.0.0.1 - - [06/Nov/2026:02:55:56 +0000] \"GET /sample HTTP/1.1\" 200 896 \"-\" \"Wget\" \"-\"\n", 140000)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- fileRequest(server, http.MethodGet, "/v1/files/ai-tail?path=%2Fconcurrent.log", "") }()
	deadline := time.After(3 * time.Second)
	for len(server.aiTailGate) == 0 {
		select {
		case response := <-done:
			t.Fatalf("projection finished before writer: %d", response.Code)
		case <-deadline:
			t.Fatal("projection did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	writer, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	select {
	case response := <-done:
		if response.Code == http.StatusOK || strings.Contains(response.Body.String(), "/sample") {
			t.Fatalf("concurrent projection returned content status=%d body=%s", response.Code, response.Body.String())
		}
	case <-time.After(12 * time.Second):
		t.Fatal("projection did not terminate")
	}
}
