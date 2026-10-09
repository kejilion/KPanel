package panel

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

func TestFileTransferPipelineRecoversOnlyDurablePrefix(t *testing.T) {
	server, _ := newTestServer(t)
	a, _, root := realTransferAgent(t, server)
	a.lostChunk, a.lostCommit = true, true
	data := bytes.Repeat([]byte("prefetch-content"), 2*contract.FileTransferChunkBytes/15+123)
	sourceURL := "https://downloads.example.com/large.bin"
	u, _ := url.Parse(sourceURL)
	header := make(http.Header)
	header.Set("ETag", `"version-1"`)
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: header.Clone(), ContentLength: int64(len(data)), Request: &http.Request{URL: u},
			Body: &transferCutBody{ReadCloser: io.NopCloser(bytes.NewReader(data)), remaining: contract.FileTransferChunkBytes + 123}}, nil
	}
	ranges := 0
	server.remoteDownloadRangeOpen = func(_ context.Context, _ string, input remotedownload.ResumeRequest) (*http.Response, error) {
		ranges++
		if input.Offset != contract.FileTransferChunkBytes {
			t.Fatalf("resumed speculative prefix: %d", input.Offset)
		}
		return &http.Response{StatusCode: 206, Header: header.Clone(), ContentLength: int64(len(data)) - input.Offset, Request: &http.Request{URL: u},
			Body: io.NopCloser(bytes.NewReader(data[input.Offset:]))}, nil
	}
	result := server.executeFileRemoteDownload(context.Background(), contract.FileRemoteDownloadRequest{URL: sourceURL, TargetDirectory: "/home", Name: "large.bin"}, "pipeline", func(contract.FileTransferEvent) bool { return true })
	if result.State != "complete" || ranges != 1 || a.chunkBytes != int64(len(data)+contract.FileTransferChunkBytes) || a.commitAttempts != 1 {
		t.Fatalf("result=%#v ranges=%d bytes=%d commits=%d", result, ranges, a.chunkBytes, a.commitAttempts)
	}
	got, err := os.ReadFile(filepath.Join(root, "home", "large.bin"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("pipeline content differs: %v", err)
	}
}

type transferPipelineResolver struct{}

func (transferPipelineResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
}

func TestFileTransferSegmentedFailureResumesDurablePrefix(t *testing.T) {
	server, _ := newTestServer(t)
	_, _, root := realTransferAgent(t, server)
	data := bytes.Repeat([]byte{73}, 32<<20)
	var faults, resumes atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("Range") == "bytes=8388608-10485759" && faults.CompareAndSwap(0, 1) {
			w.Header().Set("Content-Range", "bytes 8388608-10485759/33554432")
			w.Header().Set("Content-Length", strconv.Itoa(2<<20))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte{73})
			return
		}
		if r.Header.Get("Range") == "bytes=8388608-" {
			resumes.Add(1)
		}
		http.ServeContent(w, r, "large.bin", time.Time{}, bytes.NewReader(data))
	}))
	defer origin.Close()
	client := remotedownload.NewClient(remotedownload.Config{MaxConnections: 4, Resolver: transferPipelineResolver{}, Dialer: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, origin.Listener.Addr().String())
	}})
	server.remoteDownloadOpen = func(ctx context.Context, raw string) (*http.Response, error) {
		return client.OpenSegmented(ctx, raw, 2)
	}
	server.remoteDownloadRangeOpen = client.OpenRange
	result := server.executeFileRemoteDownload(context.Background(), contract.FileRemoteDownloadRequest{URL: "http://download.example.com/large", TargetDirectory: "/home", Name: "large.bin"}, "segmented", func(contract.FileTransferEvent) bool { return true })
	if result.State != "complete" || faults.Load() != 1 || resumes.Load() != 1 {
		t.Fatalf("result=%#v faults=%d resumes=%d", result, faults.Load(), resumes.Load())
	}
	got, err := os.ReadFile(filepath.Join(root, "home", "large.bin"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("resumed content differs: %v", err)
	}
}

func TestFileTransferPipelineCancellationAbortsLargeReceive(t *testing.T) {
	server, _ := newTestServer(t)
	_, _, root := realTransferAgent(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = writer.Write(make([]byte, contract.FileTransferChunkBytes))
		<-ctx.Done()
		_ = writer.CloseWithError(ctx.Err())
	}()
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), ContentLength: 32 << 20, Body: reader}, nil
	}
	result := server.executeFileRemoteDownload(ctx, contract.FileRemoteDownloadRequest{URL: "http://download.example.com/large", TargetDirectory: "/home", Name: "large.bin"}, "cancel", func(event contract.FileTransferEvent) bool {
		if event.LoadedBytes >= contract.FileTransferChunkBytes {
			cancel()
			return false
		}
		return true
	})
	if result.State != "error" {
		t.Fatalf("cancel published: %#v", result)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("source retained after cancellation")
	}
	entries, err := os.ReadDir(filepath.Join(root, "home"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancel retained receive: %v %v", entries, err)
	}
}

func TestFileTransferPipelineRejectsTruncatedLargeSource(t *testing.T) {
	server, _ := newTestServer(t)
	_, _, root := realTransferAgent(t, server)
	data := bytes.Repeat([]byte{42}, 2*contract.FileTransferChunkBytes)
	server.remoteDownloadOpen = func(context.Context, string) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, ContentLength: int64(len(data)),
			Body: &transferCutBody{ReadCloser: io.NopCloser(bytes.NewReader(data)), remaining: contract.FileTransferChunkBytes + 123}}, nil
	}
	result := server.executeFileRemoteDownload(context.Background(), contract.FileRemoteDownloadRequest{URL: "https://downloads.example.com/large.bin", TargetDirectory: "/home", Name: "large.bin"}, "pipeline", func(contract.FileTransferEvent) bool { return true })
	if result.State != "error" {
		t.Fatalf("truncated source published: %#v", result)
	}
	entries, err := os.ReadDir(filepath.Join(root, "home"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("unfinished receive was retained: entries=%v err=%v", entries, err)
	}
}
