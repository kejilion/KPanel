//go:build linux

package cluster_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type integrationEchoProcess struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	done   chan struct{}
	once   sync.Once
}

func (p *integrationEchoProcess) Read(data []byte) (int, error)  { return p.reader.Read(data) }
func (p *integrationEchoProcess) Write(data []byte) (int, error) { return p.writer.Write(data) }
func (p *integrationEchoProcess) Resize(uint16, uint16) error    { return nil }
func (p *integrationEchoProcess) Wait() error                    { <-p.done; return nil }
func (p *integrationEchoProcess) Kill() error                    { return p.Close() }
func (p *integrationEchoProcess) Close() error {
	p.once.Do(func() { close(p.done); _ = p.writer.Close(); _ = p.reader.Close() })
	return nil
}

// managerBackend exposes a PTY manager with the cluster TerminalBackend
// signature, as the Panel's Agent adapter does in production.
type managerBackend struct{ manager *terminal.Manager }

func (b managerBackend) Open(_ context.Context, owner string, rows, columns uint16) (terminal.Snapshot, error) {
	return b.manager.Open(owner, rows, columns)
}
func (b managerBackend) Output(ctx context.Context, owner, id string, offset int64, wait time.Duration) (terminal.Output, error) {
	return b.manager.Output(ctx, owner, id, offset, wait)
}
func (b managerBackend) Input(_ context.Context, owner, id string, data []byte) error {
	return b.manager.Input(owner, id, data)
}
func (b managerBackend) Resize(_ context.Context, owner, id string, rows, columns uint16) error {
	return b.manager.Resize(owner, id, rows, columns)
}
func (b managerBackend) Close(_ context.Context, owner, id string) error {
	return b.manager.Close(owner, id)
}

func newIntegrationEchoManager(t *testing.T) *terminal.Manager {
	manager := terminal.New(terminal.Config{Starter: func(uint16, uint16) (terminal.Process, error) {
		reader, writer := io.Pipe()
		return &integrationEchoProcess{reader: reader, writer: writer, done: make(chan struct{})}, nil
	}})
	t.Cleanup(manager.CloseAll)
	return manager
}

func pairStreamNodes(t *testing.T, ctx context.Context, center, target *fileStreamNode) cluster.Host {
	t.Helper()
	code, err := target.service.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.service.AddHost(ctx, cluster.AddHostInput{Origin: target.origin, PairingCode: code.Code, ControllerOrigin: center.origin})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := center.service.Refresh(ctx, host.ID); err != nil {
		t.Fatal(err)
	}
	return host
}

func TestPanelStreamV3FileManagementOverReusableSockets(t *testing.T) {
	center := newFileStreamNode(t)
	target := newFileStreamNodeWith(t, fileStreamNodeOptions{advertiseStream: true})
	ctx, stop := context.WithTimeout(context.Background(), 25*time.Second)
	defer stop()
	host := pairStreamNodes(t, ctx, center, target)
	open := func(input cluster.LightFileRequest) (*http.Response, error) {
		return center.service.OpenRemotePanelFile(ctx, host.ID, input)
	}
	content := bytes.Repeat([]byte("stream-v3\x00\xff"), 300000)
	response, err := open(cluster.LightFileRequest{Method: "POST", Path: "/v1/files/upload", RawQuery: "path=%2F&name=v3.bin",
		Headers: map[string]string{"Content-Type": "application/octet-stream"}, Body: bytes.NewReader(content), BodyLength: int64(len(content))})
	streamRead(t, response, err, 201)
	stored, err := os.ReadFile(filepath.Join(target.root, "v3.bin"))
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("upload stored incorrectly: %v", err)
	}
	socketsBefore := target.calls.Load()
	for index := 0; index < 10; index++ {
		response, err = open(cluster.LightFileRequest{Method: "GET", Path: "/v1/files", RawQuery: "path=%2F"})
		if listing := streamRead(t, response, err, 200); !bytes.Contains(listing, []byte("v3.bin")) {
			t.Fatal("uploaded file missing in directory")
		}
	}
	if opened := target.calls.Load() - socketsBefore; opened > 1 {
		t.Fatalf("10 sequential listings opened %d sockets; expected pooled reuse", opened)
	}
	response, err = open(cluster.LightFileRequest{Method: "GET", Path: "/v1/files/content", RawQuery: "path=%2Fv3.bin", Headers: map[string]string{"Range": "bytes=7-40"}})
	if got := streamRead(t, response, err, 206); !bytes.Equal(got, content[7:41]) {
		t.Fatal("range mismatch")
	}
	response, err = open(cluster.LightFileRequest{Method: "GET", Path: "/v1/files/content", RawQuery: "path=%2Fv3.bin"})
	if got := streamRead(t, response, err, 200); !bytes.Equal(got, content) {
		t.Fatal("download mismatch")
	}
	if target.legacy.Load() != 0 {
		t.Fatalf("file management used the legacy relay %d times", target.legacy.Load())
	}
	t.Logf("real TLS: %d bytes over v3 stream, %d sockets for %d requests", len(content), target.calls.Load(), 14)
}

func TestPanelStreamV3FallsBackWhenTargetRejectsUpgrade(t *testing.T) {
	center := newFileStreamNode(t)
	// Advertises the capability but refuses the upgrade (for example a proxy
	// that strips WebSocket): requests must still succeed over v2.
	target := newFileStreamNodeWith(t, fileStreamNodeOptions{advertiseStream: true, rejectStream: true})
	ctx, stop := context.WithTimeout(context.Background(), 25*time.Second)
	defer stop()
	host := pairStreamNodes(t, ctx, center, target)
	content := []byte("fallback-body")
	response, err := center.service.OpenRemotePanelFile(ctx, host.ID, cluster.LightFileRequest{Method: "POST", Path: "/v1/files/upload",
		RawQuery: "path=%2F&name=fallback.txt", Headers: map[string]string{"Content-Type": "application/octet-stream"},
		Body: bytes.NewReader(content), BodyLength: int64(len(content))})
	streamRead(t, response, err, 201)
	if stored, err := os.ReadFile(filepath.Join(target.root, "fallback.txt")); err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("fallback upload stored incorrectly: %v", err)
	}
	if target.calls.Load() == 0 || target.legacy.Load() == 0 {
		t.Fatalf("expected one rejected upgrade then v2: stream=%d legacy=%d", target.calls.Load(), target.legacy.Load())
	}
	attempts := target.calls.Load()
	response, err = center.service.OpenRemotePanelFile(ctx, host.ID, cluster.LightFileRequest{Method: "GET", Path: "/v1/files", RawQuery: "path=%2F"})
	streamRead(t, response, err, 200)
	if target.calls.Load() != attempts {
		t.Fatal("legacy window did not suppress repeated upgrade attempts")
	}
}

func TestPanelStreamV3TerminalAndLegacyTargets(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), 25*time.Second)
	defer stop()
	for _, advertise := range []bool{true, false} {
		manager := newIntegrationEchoManager(t)
		center := newFileStreamNode(t)
		target := newFileStreamNodeWith(t, fileStreamNodeOptions{advertiseStream: advertise, terminal: managerBackend{manager: manager}})
		host := pairStreamNodes(t, ctx, center, target)
		opened, err := center.service.TerminalOpen(ctx, host.ID, cluster.TerminalOpenRequest{Rows: 24, Columns: 80})
		if err != nil {
			t.Fatal(err)
		}
		if err := center.service.TerminalInput(ctx, host.ID, cluster.TerminalInputRequest{SessionID: opened.SessionID, Data: base64.StdEncoding.EncodeToString([]byte("echo-v3"))}); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		offset, seen := opened.Offset, ""
		for !strings.Contains(seen, "echo-v3") && time.Now().Before(deadline) {
			output, err := center.service.TerminalOutput(ctx, host.ID, cluster.TerminalOutputRequest{SessionID: opened.SessionID, Offset: offset, Wait: 500})
			if err != nil {
				t.Fatal(err)
			}
			seen += string(output.Data)
			offset = output.NextOffset
		}
		if !strings.Contains(seen, "echo-v3") {
			t.Fatalf("advertise=%v: echo not received: %q", advertise, seen)
		}
		if err := center.service.TerminalClose(ctx, host.ID, cluster.TerminalCloseRequest{SessionID: opened.SessionID}); err != nil {
			t.Fatal(err)
		}
		legacy := target.terminalLegacy.Load()
		if advertise && legacy != 0 {
			t.Fatalf("stream-capable target received %d v2 terminal requests", legacy)
		}
		if !advertise && legacy == 0 {
			t.Fatal("legacy target did not use v2 terminal requests")
		}
	}
}
