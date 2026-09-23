//go:build linux

package cluster_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

// latencyConn delays delivery in both directions without blocking the sender
// or limiting bandwidth, approximating a long-haul link on loopback.
type latencyConn struct {
	net.Conn
	delay     time.Duration
	outbound  chan timedChunk
	reader    *io.PipeReader
	closeOnce sync.Once
	done      chan struct{}
}

type timedChunk struct {
	data []byte
	at   time.Time
}

func newLatencyConn(conn net.Conn, delay time.Duration) net.Conn {
	reader, writer := io.Pipe()
	c := &latencyConn{Conn: conn, delay: delay, outbound: make(chan timedChunk, 4096), reader: reader, done: make(chan struct{})}
	go func() {
		for chunk := range c.outbound {
			time.Sleep(time.Until(chunk.at))
			if _, err := conn.Write(chunk.data); err != nil {
				return
			}
		}
	}()
	inbound := make(chan timedChunk, 4096)
	go func() {
		defer close(inbound)
		for {
			buffer := make([]byte, 64<<10)
			n, err := conn.Read(buffer)
			if n > 0 {
				inbound <- timedChunk{data: buffer[:n], at: time.Now().Add(delay)}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		for chunk := range inbound {
			time.Sleep(time.Until(chunk.at))
			if _, err := writer.Write(chunk.data); err != nil {
				return
			}
		}
		_ = writer.Close()
	}()
	return c
}

func (c *latencyConn) Read(data []byte) (int, error) { return c.reader.Read(data) }

func (c *latencyConn) Write(data []byte) (int, error) {
	select {
	case <-c.done:
		return 0, net.ErrClosed
	case c.outbound <- timedChunk{data: append([]byte(nil), data...), at: time.Now().Add(c.delay)}:
		return len(data), nil
	}
}

func (c *latencyConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.done)
		time.AfterFunc(c.delay+50*time.Millisecond, func() { close(c.outbound) })
		_ = c.reader.Close()
	})
	return c.Conn.Close()
}

type transportMeasure struct {
	listings, download, upload, echo time.Duration
}

func measureTransport(t *testing.T, advertise bool, oneWay time.Duration) transportMeasure {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Minute)
	defer stop()
	manager := newIntegrationEchoManager(t)
	center := newFileStreamNodeWith(t, fileStreamNodeOptions{latency: oneWay})
	target := newFileStreamNodeWith(t, fileStreamNodeOptions{advertiseStream: advertise, terminal: managerBackend{manager: manager}})
	host := pairStreamNodes(t, ctx, center, target)
	content := bytes.Repeat([]byte("0123456789abcdef"), 512<<10) // 8 MiB
	if err := os.WriteFile(filepath.Join(target.root, "bench.bin"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	open := func(input cluster.LightFileRequest) []byte {
		response, err := center.service.OpenRemotePanelFile(ctx, host.ID, input)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode >= 300 {
			t.Fatalf("status=%d err=%v", response.StatusCode, err)
		}
		return body
	}
	var result transportMeasure
	open(cluster.LightFileRequest{Method: http.MethodGet, Path: "/v1/files", RawQuery: "path=%2F"}) // warm up
	started := time.Now()
	for range 10 {
		open(cluster.LightFileRequest{Method: http.MethodGet, Path: "/v1/files", RawQuery: "path=%2F"})
	}
	result.listings = time.Since(started) / 10
	started = time.Now()
	if got := open(cluster.LightFileRequest{Method: http.MethodGet, Path: "/v1/files/content", RawQuery: "path=%2Fbench.bin"}); len(got) != len(content) {
		t.Fatalf("download length %d", len(got))
	}
	result.download = time.Since(started)
	started = time.Now()
	open(cluster.LightFileRequest{Method: http.MethodPost, Path: "/v1/files/upload", RawQuery: "path=%2F&name=up.bin",
		Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: bytes.NewReader(content), BodyLength: int64(len(content))})
	result.upload = time.Since(started)

	opened, err := center.service.TerminalOpen(ctx, host.ID, cluster.TerminalOpenRequest{Rows: 24, Columns: 80})
	if err != nil {
		t.Fatal(err)
	}
	offset := opened.Offset
	var total time.Duration
	for index := range 10 {
		marker := "echo-" + string(rune('a'+index))
		started = time.Now()
		if err := center.service.TerminalInput(ctx, host.ID, cluster.TerminalInputRequest{SessionID: opened.SessionID, Data: base64.StdEncoding.EncodeToString([]byte(marker))}); err != nil {
			t.Fatal(err)
		}
		seen := ""
		for !strings.Contains(seen, marker) {
			output, err := center.service.TerminalOutput(ctx, host.ID, cluster.TerminalOutputRequest{SessionID: opened.SessionID, Offset: offset, Wait: 1000})
			if err != nil {
				t.Fatal(err)
			}
			seen += string(output.Data)
			offset = output.NextOffset
		}
		total += time.Since(started)
	}
	result.echo = total / 10
	_ = center.service.TerminalClose(ctx, host.ID, cluster.TerminalCloseRequest{SessionID: opened.SessionID})
	return result
}

// TestTransportV2V3Comparison is an opt-in benchmark:
// KPANEL_TRANSPORT_BENCH=1 go test -run TransportV2V3 -v ./internal/cluster/
func TestTransportV2V3Comparison(t *testing.T) {
	if os.Getenv("KPANEL_TRANSPORT_BENCH") != "1" {
		t.Skip("set KPANEL_TRANSPORT_BENCH=1 to run the latency comparison")
	}
	for _, oneWay := range []time.Duration{25 * time.Millisecond, 75 * time.Millisecond} {
		v2 := measureTransport(t, false, oneWay)
		v3 := measureTransport(t, true, oneWay)
		mib := func(d time.Duration) float64 { return 8 / d.Seconds() }
		t.Logf("RTT %v | listing v2 %v v3 %v | 8MiB download v2 %.2f MiB/s v3 %.2f MiB/s | upload v2 %.2f MiB/s v3 %.2f MiB/s | echo v2 %v v3 %v",
			2*oneWay, v2.listings.Round(time.Millisecond), v3.listings.Round(time.Millisecond),
			mib(v2.download), mib(v3.download), mib(v2.upload), mib(v3.upload),
			v2.echo.Round(time.Millisecond), v3.echo.Round(time.Millisecond))
	}
}
