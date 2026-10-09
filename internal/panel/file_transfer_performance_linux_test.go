package panel

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	agentserver "github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

type transferBenchCase struct {
	name                   string
	size                   int64
	files, parallel        int
	sourceRate, targetRate int64
	delay                  time.Duration
	noRange                bool
}

// This opt-in test is copied unchanged into the baseline worktree by the runner.
// HTTP origin, Panel and real Agent/filemanager share one process: its CPU/RSS
// measures the whole fixture, not an independently deployed production Panel.
func TestFileTransferPerformance(t *testing.T) {
	if os.Getenv("KPANEL_TRANSFER_BENCHMARK") != "1" {
		t.Skip("opt-in HTTP/filesystem performance experiment")
	}
	cases := []transferBenchCase{
		{"fast", 64 << 20, 1, 1, 0, 0, 0, false},
		{"source-limited", 64 << 20, 1, 1, 8 << 20, 0, 0, false},
		{"balanced", 64 << 20, 1, 1, 8 << 20, 8 << 20, 0, false},
		{"no-range", 64 << 20, 1, 1, 8 << 20, 8 << 20, 0, true},
		{"request-delay-20ms", 64 << 20, 1, 1, 0, 0, 20 * time.Millisecond, false},
		{"request-delay-100ms", 64 << 20, 1, 1, 0, 0, 100 * time.Millisecond, false},
		{"small-files", 4 << 10, 32, 1, 0, 0, 0, false},
		{"two-tasks", 64 << 20, 2, 2, 8 << 20, 0, 0, false},
		{"large", 256 << 20, 1, 1, 0, 0, 0, false},
	}
	wanted := os.Getenv("KPANEL_TRANSFER_CASE")
	for _, spec := range cases {
		if spec.name != wanted {
			continue
		}
		t.Run(spec.name, func(t *testing.T) { runTransferBench(t, spec) })
		return
	}
	t.Fatalf("unknown KPANEL_TRANSFER_CASE %q", wanted)
}

func runTransferBench(t *testing.T, spec transferBenchCase) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "source.bin")
	source, err := os.Create(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, 64<<10)
	for i := range block {
		block[i] = byte((i*31 + i/257) % 251)
	}
	hash := sha256.New()
	for left := spec.size; left > 0; {
		// Distinguish every absolute block: a repeated/swapped 2 MiB segment or
		// 8 MiB receive chunk must change the final digest.
		binary.LittleEndian.PutUint64(block[:8], uint64(spec.size-left))
		n := min(left, int64(len(block)))
		if _, err := source.Write(block[:n]); err != nil {
			t.Fatal(err)
		}
		_, _ = hash.Write(block[:n])
		left -= n
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	wantDigest := hex.EncodeToString(hash.Sum(nil))
	var requests, sourceBytes, active, maxActive atomic.Int64
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		n := active.Add(1)
		defer active.Add(-1)
		for old := maxActive.Load(); n > old && !maxActive.CompareAndSwap(old, n); old = maxActive.Load() {
		}
		if !benchPause(r.Context(), spec.delay) {
			return
		}
		start, end, partial := int64(0), spec.size, false
		if value := r.Header.Get("Range"); value != "" && !spec.noRange {
			var err error
			bounds := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
			if len(bounds) != 2 {
				http.Error(w, "range", 416)
				return
			}
			start, err = strconv.ParseInt(bounds[0], 10, 64)
			if err != nil || start < 0 || start >= spec.size {
				http.Error(w, "range", 416)
				return
			}
			if bounds[1] != "" {
				end, err = strconv.ParseInt(bounds[1], 10, 64)
				end++
				if err != nil || end <= start || end > spec.size {
					http.Error(w, "range", 416)
					return
				}
			}
			partial = true
		}
		w.Header().Set("ETag", `"transfer-benchmark-v1"`)
		w.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
		if partial {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, spec.size))
			w.WriteHeader(http.StatusPartialContent)
		}
		w.(http.Flusher).Flush()
		file, openErr := os.Open(sourcePath)
		if openErr != nil {
			t.Error(openErr)
			return
		}
		defer file.Close()
		reader := &benchPacedReader{ctx: r.Context(), reader: io.NewSectionReader(file, start, end-start), rate: spec.sourceRate, started: time.Now()}
		written, _ := io.CopyBuffer(w, reader, make([]byte, 64<<10))
		sourceBytes.Add(written)
	}))
	defer origin.Close()
	if err := os.Mkdir(filepath.Join(root, "home"), 0700); err != nil {
		t.Fatal(err)
	}
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	handler := agentserver.NewFileHandler(manager)
	agentHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !benchPause(r.Context(), spec.delay) {
			return
		}
		if r.Method == http.MethodPut && spec.targetRate > 0 {
			r.Body = &benchPacedBody{ReadCloser: r.Body, paced: benchPacedReader{ctx: r.Context(), reader: r.Body, rate: spec.targetRate, started: time.Now()}}
		}
		handler.ServeHTTP(w, r)
	}))
	defer agentHTTP.Close()
	agent := &transferIntegrationAgent{url: agentHTTP.URL, client: agentHTTP.Client()}
	server := &Server{agent: agent}
	config := remotedownload.Config{Resolver: benchPublicResolver{}, Dialer: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, origin.Listener.Addr().String())
	}}
	// Keep the identical harness compilable against the unmodified baseline.
	if limit := reflect.ValueOf(&config).Elem().FieldByName("MaxConnections"); limit.IsValid() {
		limit.SetInt(4)
	}
	client := remotedownload.NewClient(config)
	mode := os.Getenv("KPANEL_TRANSFER_MODE")
	server.remoteDownloadOpen, server.remoteDownloadRangeOpen = client.Open, client.OpenRange
	if mode == "segmented" || mode == "segmented-2" {
		accelerated, ok := any(client).(interface {
			OpenSegmented(context.Context, string, int) (*http.Response, error)
		})
		if !ok {
			t.Fatal("candidate does not implement segmentation")
		}
		connections := 4
		if mode == "segmented-2" {
			connections = 2
		}
		server.remoteDownloadOpen = func(ctx context.Context, raw string) (*http.Response, error) {
			return accelerated.OpenSegmented(ctx, raw, connections)
		}
	}
	runtime.GC()
	var before, after syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &before)
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	started := time.Now()
	var wg sync.WaitGroup
	results := make(chan contract.FileTransferEvent, spec.files)
	for worker := 0; worker < spec.parallel; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for index := worker; index < spec.files; index += spec.parallel {
				result := server.executeFileRemoteDownload(context.Background(), contract.FileRemoteDownloadRequest{
					URL: "http://download.example.com/source.bin", TargetDirectory: "/home", Name: fmt.Sprintf("copy-%d.bin", index),
				}, "benchmark", func(contract.FileTransferEvent) bool { return true })
				results <- result
			}
		}(worker)
	}
	wg.Wait()
	elapsed := time.Since(started)
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &after)
	runtime.ReadMemStats(&memAfter)
	close(results)
	for result := range results {
		if result.State != "complete" || result.LoadedBytes != spec.size {
			t.Fatalf("incomplete: %#v", result)
		}
	}
	for index := 0; index < spec.files; index++ {
		file, err := os.Open(filepath.Join(root, "home", fmt.Sprintf("copy-%d.bin", index)))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.New()
		written, err := io.Copy(digest, file)
		_ = file.Close()
		if err != nil || written != spec.size || hex.EncodeToString(digest.Sum(nil)) != wantDigest {
			t.Fatalf("content mismatch %d: %v", index, err)
		}
	}
	// Ensure source handlers have accounted for the probe bytes before sampling.
	origin.Close()
	seconds := func(v syscall.Timeval) float64 { return float64(v.Sec) + float64(v.Usec)/1e6 }
	data, _ := json.Marshal(map[string]any{
		"case": spec.name, "mode": mode, "label": os.Getenv("KPANEL_TRANSFER_LABEL"), "sample": os.Getenv("KPANEL_TRANSFER_SAMPLE"),
		"bytes": spec.size * int64(spec.files), "parallel": spec.parallel, "seconds": elapsed.Seconds(),
		"cpu_seconds": seconds(after.Utime) + seconds(after.Stime) - seconds(before.Utime) - seconds(before.Stime),
		"max_rss_kib": after.Maxrss, "allocated_bytes": memAfter.TotalAlloc - memBefore.TotalAlloc,
		"source_requests": requests.Load(), "source_bytes": sourceBytes.Load(), "max_source_active": maxActive.Load(),
		"agent_bytes": agent.chunkBytes, "hash_verified": true, "scope": "combined-origin-panel-agent-process",
	})
	fmt.Printf("TRANSFER_BENCH %s\n", data)
}

type benchPublicResolver struct{}

func (benchPublicResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
}

type benchPacedReader struct {
	ctx     context.Context
	reader  io.Reader
	rate    int64
	started time.Time
	read    int64
}

func (r *benchPacedReader) Read(buffer []byte) (int, error) {
	if len(buffer) > 64<<10 {
		buffer = buffer[:64<<10]
	}
	n, err := r.reader.Read(buffer)
	r.read += int64(n)
	if r.rate > 0 && n > 0 && !benchPause(r.ctx, time.Until(r.started.Add(time.Duration(float64(r.read)/float64(r.rate)*float64(time.Second))))) {
		return 0, r.ctx.Err()
	}
	return n, err
}

type benchPacedBody struct {
	io.ReadCloser
	paced benchPacedReader
}

func (b *benchPacedBody) Read(buffer []byte) (int, error) { return b.paced.Read(buffer) }
func benchPause(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
