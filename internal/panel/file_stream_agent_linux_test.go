//go:build linux

package panel

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

func realAgentFileStream(t *testing.T, writeTimeout time.Duration) (*cluster.Service, string, string) {
	t.Helper()
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	handler := agent.NewFileHandler(manager)
	center, host := newAgentFileStreamTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer diagnostic-only-token" {
			t.Error("Agent authorization was not preserved")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}), writeTimeout)
	return center, host, root
}

func TestFileStreamAgentSlowUploadReturnsCommittedResult(t *testing.T) {
	for _, path := range []string{"/v1/files/upload", "/v1/files/transfer/import"} {
		t.Run(path, func(t *testing.T) {
			center, host, root := realAgentFileStream(t, 200*time.Millisecond)
			body, writer := io.Pipe()
			defer body.Close()
			go func() {
				defer writer.Close()
				for range 4 {
					time.Sleep(150 * time.Millisecond)
					if _, err := writer.Write([]byte("part")); err != nil {
						return
					}
				}
			}()
			query := "path=%2F&name=slow.bin"
			if path == "/v1/files/transfer/import" {
				query += "&kind=file&size=16"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			response, err := center.OpenRemotePanelFile(ctx, host, cluster.LightFileRequest{Method: "POST", Path: path, RawQuery: query, Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: body, BodyLength: -1})
			if err != nil {
				t.Fatal(err)
			}
			result, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || response.StatusCode != http.StatusCreated {
				t.Fatalf("status=%d body=%s err=%v", response.StatusCode, result, err)
			}
			content, err := os.ReadFile(filepath.Join(root, "slow.bin"))
			if err != nil || string(content) != "partpartpartpart" {
				t.Fatalf("stored=%q error=%v", content, err)
			}
		})
	}
}

// Run explicitly with KPANEL_FILE_STREAM_LONG_TEST=1 to exercise the unchanged
// production Agent WriteTimeout (two minutes), not only the scaled fixture.
func TestFileStreamAgentProductionWriteDeadline(t *testing.T) {
	if os.Getenv("KPANEL_FILE_STREAM_LONG_TEST") != "1" {
		t.Skip("125-second production deadline integration")
	}
	center, host, root := realAgentFileStream(t, 2*time.Minute)
	body, writer := io.Pipe()
	defer body.Close()
	go func() {
		defer writer.Close()
		for range 25 {
			time.Sleep(5 * time.Second)
			if _, err := writer.Write([]byte("x")); err != nil {
				return
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 140*time.Second)
	defer cancel()
	response, err := center.OpenRemotePanelFile(ctx, host, cluster.LightFileRequest{Method: "POST", Path: "/v1/files/upload", RawQuery: "path=%2F&name=slow.bin", Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: body, BodyLength: -1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d body=%s error=%v", response.StatusCode, result, err)
	}
	content, err := os.ReadFile(filepath.Join(root, "slow.bin"))
	if err != nil || string(content) != "xxxxxxxxxxxxxxxxxxxxxxxxx" {
		t.Fatalf("stored=%q error=%v", content, err)
	}
}

type zeroFileStreamReader struct{}

func (zeroFileStreamReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestFileStreamAgentLargeDirectoryImport(t *testing.T) {
	if os.Getenv("KPANEL_FILE_STREAM_LARGE_TEST") != "1" {
		t.Skip("513 MiB real filesystem integration")
	}
	center, host, root := realAgentFileStream(t, 2*time.Minute)
	const size int64 = 513 << 20
	body, writer := io.Pipe()
	defer body.Close()
	go func() {
		archive := tar.NewWriter(writer)
		err := archive.WriteHeader(&tar.Header{Name: "large.bin", Mode: 0600, Size: size, Typeflag: tar.TypeReg})
		if err == nil {
			_, err = io.CopyN(archive, zeroFileStreamReader{}, size-4)
		}
		if err == nil {
			_, err = archive.Write([]byte("tail"))
		}
		if err == nil {
			err = archive.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	response, err := center.OpenRemotePanelFile(ctx, host, cluster.LightFileRequest{Method: "POST", Path: "/v1/files/transfer/import", RawQuery: "path=%2F&name=directory&kind=directory&size=0", Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: body, BodyLength: -1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusCreated {
		t.Fatalf("status=%d body=%s error=%v", response.StatusCode, result, err)
	}
	file, err := os.Open(filepath.Join(root, "directory", "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != size {
		t.Fatalf("info=%v err=%v", info, err)
	}
	end := make([]byte, 4)
	if _, err := file.ReadAt(end, size-4); err != nil || !bytes.Equal(end, []byte("tail")) {
		t.Fatalf("tail=%q error=%v", end, err)
	}
}
