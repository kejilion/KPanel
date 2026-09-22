package cluster

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

// echoProcess is an in-memory PTY: every byte written is read back.
type echoProcess struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	done   chan struct{}
	once   sync.Once
}

func newEchoProcess() *echoProcess {
	reader, writer := io.Pipe()
	return &echoProcess{reader: reader, writer: writer, done: make(chan struct{})}
}

func (p *echoProcess) Read(data []byte) (int, error)  { return p.reader.Read(data) }
func (p *echoProcess) Write(data []byte) (int, error) { return p.writer.Write(data) }
func (p *echoProcess) Resize(uint16, uint16) error    { return nil }
func (p *echoProcess) Wait() error                    { <-p.done; return nil }
func (p *echoProcess) Kill() error                    { return p.Close() }
func (p *echoProcess) Close() error {
	p.once.Do(func() {
		close(p.done)
		_ = p.writer.Close()
		_ = p.reader.Close()
	})
	return nil
}

func newEchoManager(t *testing.T) *terminal.Manager {
	t.Helper()
	manager := terminal.New(terminal.Config{Starter: func(uint16, uint16) (terminal.Process, error) { return newEchoProcess(), nil }})
	t.Cleanup(manager.CloseAll)
	return manager
}

func readStreamTerminalUntil(t *testing.T, stream *streamTerminal, offset int64, want string) int64 {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var seen strings.Builder
	for time.Now().Before(deadline) {
		output, err := stream.output(context.Background(), TerminalOutputRequest{SessionID: stream.sessionID, Offset: offset, Wait: 500})
		if err != nil {
			if errors.Is(err, ErrTerminalUnavailable) {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			t.Fatalf("output: %v", err)
		}
		seen.Write(output.Data)
		offset = output.NextOffset
		if strings.Contains(seen.String(), want) {
			return offset
		}
	}
	t.Fatalf("output %q never contained %q", seen.String(), want)
	return offset
}

func (f *streamFixture) panelTerminalDialer(t *testing.T, dials *atomic.Int32) streamTerminalDialer {
	return func(ctx context.Context, first byte, payload any) (*fileStreamConn, terminalStreamReady, error) {
		dials.Add(1)
		conn, err := dialFileStreamWithParent(ctx, context.Background(), f.client.streamClient, f.server.URL, f.controller, f.service.NodeID(),
			f.key, nodeNoiseKeyV2(f.service.nodeIdentityV2).Public, time.Now(), fileStreamHello{Role: streamRolePanelTerminal})
		if err != nil {
			return nil, terminalStreamReady{}, &streamDialError{err}
		}
		return startTerminalStream(conn, first, payload)
	}
}

func TestPanelFileStreamReusesOneAuthenticatedSocket(t *testing.T) {
	var requests atomic.Int32
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodPost {
			_, _ = io.Copy(w, r.Body)
			return
		}
		_, _ = w.Write([]byte("listing:" + r.URL.RawQuery))
	}))
	conn, err := f.dial(context.Background(), f.key, fileStreamHello{Role: streamRolePanelFile})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.close()
	upload := bytes.Repeat([]byte("reuse"), 50000)
	inputs := []LightFileRequest{
		{Method: http.MethodGet, Path: "/v1/files", RawQuery: "path=%2F"},
		{Method: http.MethodPost, Path: "/v1/files/upload", Body: bytes.NewReader(upload), BodyLength: int64(len(upload))},
		{Method: http.MethodGet, Path: "/v1/files", RawQuery: "path=%2Ftmp"},
	}
	for index, input := range inputs {
		var clean atomic.Bool
		finished := make(chan struct{})
		response, err := exchangeStreamRequest(conn, input, func(ok bool) { clean.Store(ok); close(finished) })
		if err != nil {
			t.Fatalf("request %d: %v", index, err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatalf("request %d body: %v", index, err)
		}
		<-finished
		if !clean.Load() {
			t.Fatalf("request %d did not return the socket for reuse", index)
		}
		if input.Method == http.MethodPost && !bytes.Equal(body, upload) {
			t.Fatal("upload echo mismatch")
		}
		if input.Method == http.MethodGet && string(body) != "listing:"+input.RawQuery {
			t.Fatalf("listing = %q", body)
		}
	}
	if f.streamCalls.Load() != 1 || requests.Load() != 3 || f.legacyCalls.Load() != 0 {
		t.Fatalf("stream=%d requests=%d legacy=%d", f.streamCalls.Load(), requests.Load(), f.legacyCalls.Load())
	}
}

func TestPanelFileStreamEarlyRejectionIsNeverReused(t *testing.T) {
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	conn, err := f.dial(context.Background(), f.key, fileStreamHello{Role: streamRolePanelFile})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.close()
	// A slow body keeps the upload in flight while the conflict arrives.
	reader, writer := io.Pipe()
	go func() {
		_, _ = writer.Write(bytes.Repeat([]byte("x"), 64<<10))
		time.Sleep(500 * time.Millisecond)
		_ = writer.CloseWithError(io.ErrClosedPipe)
	}()
	finished := make(chan bool, 1)
	response, err := exchangeStreamRequest(conn, LightFileRequest{Method: http.MethodPost, Path: "/v1/files/upload", Body: reader, BodyLength: -1},
		func(ok bool) { finished <- ok })
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d", response.StatusCode)
	}
	if <-finished {
		t.Fatal("socket with an unfinished upload was offered for reuse")
	}
}

func TestPanelFileStreamRechecksAuthorizationBeforeEachRequest(t *testing.T) {
	f := newStreamFixture(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	conn, err := f.dial(context.Background(), f.key, fileStreamHello{Role: streamRolePanelFile})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.close()
	finished := make(chan bool, 1)
	response, err := exchangeStreamRequest(conn, LightFileRequest{Method: http.MethodGet, Path: "/v1/files"}, func(ok bool) { finished <- ok })
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !<-finished {
		t.Fatal("first request did not complete cleanly")
	}
	if err := f.service.storeV2.DeleteController(f.controller); err != nil {
		t.Fatal(err)
	}
	if _, err := exchangeStreamRequest(conn, LightFileRequest{Method: http.MethodGet, Path: "/v1/files"}, func(bool) {}); err == nil {
		t.Fatal("request after scope removal was served")
	}
}

func TestPanelStreamRolesRequireMatchingScope(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	controller, err := f.service.storeV2.Controller(f.controller)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.storeV2.DeleteController(f.controller); err != nil {
		t.Fatal(err)
	}
	controller.Scope = SummaryScope
	if err := f.service.storeV2.AddController(controller); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{streamRolePanelFile, streamRolePanelTerminal, "light-control"} {
		conn, err := f.dial(context.Background(), f.key, fileStreamHello{Role: role})
		if conn != nil {
			conn.close()
		}
		if err == nil {
			t.Fatalf("role %s upgraded without the matching scope", role)
		}
	}
}

func TestPanelTerminalStreamEchoResizeReattachAndClose(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	manager := newEchoManager(t)
	f.service.terminal = managerTerminalBackend{manager: manager}
	var dials atomic.Int32
	stream, opened, err := openStreamTerminal(context.Background(), "host-1", f.panelTerminalDialer(t, &dials), nil, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.shutdown()
	if err := stream.input(context.Background(), TerminalInputRequest{SessionID: opened.SessionID, Data: "aGVsbG8="}); err != nil {
		t.Fatal(err)
	}
	offset := readStreamTerminalUntil(t, stream, opened.Offset, "hello")
	if err := stream.resize(context.Background(), TerminalResizeRequest{SessionID: opened.SessionID, Rows: 40, Columns: 120}); err != nil {
		t.Fatal(err)
	}
	// Drop the socket; the client must re-attach at its offset without losing
	// or duplicating output.
	stream.mu.Lock()
	stream.conn.close()
	stream.mu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for dials.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	var inputErr error
	for time.Now().Before(deadline) {
		if inputErr = stream.input(context.Background(), TerminalInputRequest{SessionID: opened.SessionID, Data: "d29ybGQ="}); inputErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if inputErr != nil {
		t.Fatalf("input after re-attach: %v", inputErr)
	}
	readStreamTerminalUntil(t, stream, offset, "world")
	if err := stream.close(context.Background(), TerminalCloseRequest{SessionID: opened.SessionID}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Output(context.Background(), "federation:"+f.controller, opened.SessionID, 0, 0); err != nil {
		t.Fatalf("closed session lookup: %v", err)
	}
	if output, _ := manager.Output(context.Background(), "federation:"+f.controller, opened.SessionID, 0, 0); !output.Closed {
		t.Fatal("remote PTY was not closed")
	}
	if f.legacyCalls.Load() != 0 {
		t.Fatalf("terminal stream used legacy endpoints: %d", f.legacyCalls.Load())
	}
}

type recordingTerminalFallback struct{ outputs atomic.Int32 }

func (r *recordingTerminalFallback) Output(context.Context, TerminalOutputRequest) (terminal.Output, error) {
	r.outputs.Add(1)
	return terminal.Output{Data: []byte("v2"), Offset: 0, NextOffset: 2}, nil
}
func (r *recordingTerminalFallback) Input(context.Context, TerminalInputRequest) error   { return nil }
func (r *recordingTerminalFallback) Resize(context.Context, TerminalResizeRequest) error { return nil }
func (r *recordingTerminalFallback) Close(context.Context, TerminalCloseRequest) error   { return nil }

func TestPanelTerminalStreamFallsBackToV2WhenStreamStaysDown(t *testing.T) {
	previous := terminalStreamFallbackAfter
	terminalStreamFallbackAfter = 300 * time.Millisecond
	defer func() { terminalStreamFallbackAfter = previous }()
	f := newStreamFixture(t, http.NotFoundHandler())
	f.service.terminal = managerTerminalBackend{manager: newEchoManager(t)}
	var dials atomic.Int32
	real := f.panelTerminalDialer(t, &dials)
	dial := func(ctx context.Context, first byte, payload any) (*fileStreamConn, terminalStreamReady, error) {
		if first == termAttach {
			return nil, terminalStreamReady{}, &streamDialError{ErrFileStreamUnsupported}
		}
		return real(ctx, first, payload)
	}
	fallback := &recordingTerminalFallback{}
	stream, opened, err := openStreamTerminal(context.Background(), "host-1", dial, fallback, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.shutdown()
	stream.mu.Lock()
	stream.conn.close()
	stream.mu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	for !stream.isDegraded() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	output, err := stream.output(context.Background(), TerminalOutputRequest{SessionID: opened.SessionID, Offset: 0})
	if err != nil || string(output.Data) != "v2" || fallback.outputs.Load() == 0 {
		t.Fatalf("fallback output = %+v, %v (calls %d)", output, err, fallback.outputs.Load())
	}
}

func TestLightTerminalStreamRunsOverControlConnection(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	enrollment, err := f.service.CreateLightEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	key, _ := GenerateFederationV2Keypair()
	enrolled, err := f.service.EnrollLightNode("198.51.100.10", LightEnrollRequest{Token: strings.Trim(fields[len(fields)-1], "'"), Name: "stream-light",
		NodeVersion: "1.14.1", TerminalPublicKey: encodeTestKey(key.Public)})
	if err != nil {
		t.Fatal(err)
	}
	peer, _ := decodeTerminalRelayPublicKey(enrolled.TerminalPeerPublicKey)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	relay, _ := NewTerminalRelayClient(f.server.Client())
	manager := newEchoManager(t)
	done := make(chan error, 1)
	var connected atomic.Bool
	go func() {
		done <- relay.RunTerminalStream(ctx, f.server.URL, enrolled.NodeID, enrolled.TargetNodeID, key, peer, manager, "light-owner", func() { connected.Store(true) })
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !f.service.fileStreamHub.terminalAvailable(enrolled.NodeID) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !f.service.fileStreamHub.terminalAvailable(enrolled.NodeID) || !connected.Load() {
		t.Fatal("terminal control did not register")
	}
	if f.service.fileStreamHub.available(enrolled.NodeID) {
		t.Fatal("terminal control was mistaken for the file control")
	}
	opened, err := f.service.TerminalOpen(ctx, enrolled.NodeID, TerminalOpenRequest{Rows: 24, Columns: 80})
	if err != nil {
		t.Fatal(err)
	}
	stream := f.service.streams.terminal(enrolled.NodeID, opened.SessionID)
	if stream == nil {
		t.Fatal("light terminal did not use the stream")
	}
	if err := f.service.TerminalInput(ctx, enrolled.NodeID, TerminalInputRequest{SessionID: opened.SessionID, Data: "cGluZw=="}); err != nil {
		t.Fatal(err)
	}
	readStreamTerminalUntil(t, stream, opened.Offset, "ping")
	if err := f.service.TerminalClose(ctx, enrolled.NodeID, TerminalCloseRequest{SessionID: opened.SessionID}); err != nil {
		t.Fatal(err)
	}
	if f.service.streams.terminal(enrolled.NodeID, opened.SessionID) != nil {
		t.Fatal("closed terminal handle was retained")
	}
	if output, _ := manager.Output(ctx, "light-owner", opened.SessionID, 0, 0); !output.Closed {
		t.Fatal("node PTY was not closed")
	}
	if f.legacyCalls.Load() != 0 {
		t.Fatalf("light terminal stream used legacy endpoints: %d", f.legacyCalls.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("terminal control did not stop")
	}
}

func TestTerminalAttachmentsCloseOnlyAfterGrace(t *testing.T) {
	closed := make(chan string, 2)
	attachments := newTerminalAttachments(50*time.Millisecond, func(id string) { closed <- id })
	attachments.attach("a")
	attachments.detach("a")
	attachments.attach("a")
	time.Sleep(100 * time.Millisecond)
	select {
	case id := <-closed:
		t.Fatalf("re-attached session %s was closed", id)
	default:
	}
	attachments.detach("a")
	select {
	case id := <-closed:
		if id != "a" {
			t.Fatalf("closed %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("detached session was not closed after grace")
	}
}

func TestPanelStreamCapabilityAndLegacyWindow(t *testing.T) {
	streams := newPanelStreams()
	defer streams.closeAll()
	if streams.usable("host") {
		t.Fatal("unknown host is usable")
	}
	streams.setCapable("host", true)
	if !streams.usable("host") {
		t.Fatal("capable host is not usable")
	}
	streams.markLegacy("host")
	if streams.usable("host") {
		t.Fatal("legacy window ignored")
	}
	streams.forgetHost("host")
	if streams.usable("host") {
		t.Fatal("forgotten host still usable")
	}
}
