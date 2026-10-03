package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type stallingStreamProcess struct {
	*echoProcess
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *stallingStreamProcess) Write(data []byte) (int, error) {
	if string(data) == "block" {
		close(p.entered)
		<-p.release
		return 0, terminal.ErrClosed
	}
	return p.echoProcess.Write(data)
}
func (p *stallingStreamProcess) Kill() error {
	p.once.Do(func() { close(p.release) })
	return p.echoProcess.Kill()
}

type blockedSequencedBackend struct {
	managerTerminalBackend
	entered chan struct{}
}

func (b blockedSequencedBackend) InputSequenced(ctx context.Context, owner, id string, frame terminal.InputFrame) error {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestTerminalInputWindowBoundsUnacknowledgedOwnerWrites(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	entered := make(chan struct{}, 1)
	f.service.terminal = blockedSequencedBackend{managerTerminalBackend: managerTerminalBackend{manager: newEchoManager(t)}, entered: entered}
	var dials atomic.Int32
	stream, _, err := openStreamTerminal(context.Background(), context.Background(), "host", f.panelTerminalDialer(t, &dials), nil, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.shutdown()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for seq := uint64(1); seq <= terminal.InputWindow; seq++ {
		body, _ := json.Marshal(terminal.InputFrame{Stream: "00000000000000000000000000000001", Seq: seq, Data: []byte("x")})
		if _, err := stream.beginRequest(ctx, termInputSequenced, body); err != nil {
			t.Fatalf("window frame %d: %v", seq, err)
		}
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("target did not receive input")
	}
	body, _ := json.Marshal(terminal.InputFrame{Stream: "00000000000000000000000000000001", Seq: terminal.InputWindow + 1, Data: []byte("x")})
	if _, err := stream.beginRequest(ctx, termInputSequenced, body); err == nil {
		t.Fatal("unacknowledged window exceeded 32")
	}
}

func TestTerminalInputCanceledWhileWaitingForSendLockIsNotDispatched(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	manager := newEchoManager(t)
	f.service.terminal = managerTerminalBackend{manager: manager}
	var dials atomic.Int32
	stream, opened, err := openStreamTerminal(context.Background(), context.Background(), "host", f.panelTerminalDialer(t, &dials), nil, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.shutdown()
	stream.sendMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := stream.beginRequest(ctx, termInput, []byte("must-not-arrive")); done <- err }()
	cancel()
	stream.sendMu.Unlock()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled dispatcher: %v", err)
	}
	output, err := manager.Output(context.Background(), "federation:"+f.controller, opened.SessionID, 0, 0)
	if err != nil || len(output.Data) != 0 {
		t.Fatalf("canceled input reached PTY: %q %v", output.Data, err)
	}
}

func TestTerminalSequencedStreamPanelAndLightParity(t *testing.T) {
	for _, kind := range []string{"panel", "light"} {
		t.Run(kind, func(t *testing.T) {
			f := newStreamFixture(t, http.NotFoundHandler())
			process := &stallingStreamProcess{echoProcess: newEchoProcess(), entered: make(chan struct{}), release: make(chan struct{})}
			manager := terminal.New(terminal.Config{Starter: func(uint16, uint16) (terminal.Process, error) { return process, nil }})
			t.Cleanup(manager.CloseAll)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var stream *streamTerminal
			var opened TerminalOpenResponse
			hostID := "host-1"
			owner := "federation:" + f.controller
			if kind == "panel" {
				record := testHostRecordV2(1, time.Now().UTC())
				if err := f.service.storeV2.AddHost(record); err != nil {
					t.Fatal(err)
				}
				hostID = record.ID
				f.service.terminal = managerTerminalBackend{manager: manager}
				var dials atomic.Int32
				var err error
				stream, opened, err = openStreamTerminal(ctx, ctx, hostID, f.panelTerminalDialer(t, &dials), nil, 24, 80)
				if err != nil {
					t.Fatal(err)
				}
				f.service.streams.putTerminal(stream)
			} else {
				enrollment, err := f.service.CreateLightEnrollment()
				if err != nil {
					t.Fatal(err)
				}
				fields := strings.Fields(enrollment.Command)
				key, _ := GenerateFederationV2Keypair()
				enrolled, err := f.service.EnrollLightNode("198.51.100.10", LightEnrollRequest{Token: strings.Trim(fields[len(fields)-1], "'"), Name: "duplex-light", NodeVersion: "1.23.0", TerminalPublicKey: encodeTestKey(key.Public)})
				if err != nil {
					t.Fatal(err)
				}
				peer, _ := decodeTerminalRelayPublicKey(enrolled.TerminalPeerPublicKey)
				relay, _ := NewTerminalRelayClient(f.server.Client())
				owner = "light-owner"
				hostID = enrolled.NodeID
				go func() {
					_ = relay.RunTerminalStream(ctx, f.server.URL, enrolled.NodeID, enrolled.TargetNodeID, key, peer, manager, owner, nil)
				}()
				deadline := time.Now().Add(3 * time.Second)
				for !f.service.fileStreamHub.terminalAvailable(hostID) && time.Now().Before(deadline) {
					time.Sleep(5 * time.Millisecond)
				}
				opened, err = f.service.TerminalOpen(ctx, hostID, TerminalOpenRequest{Rows: 24, Columns: 80})
				if err != nil {
					t.Fatal(err)
				}
				stream = f.service.streams.terminal(hostID, opened.SessionID)
			}
			defer stream.shutdown()
			if !f.service.TerminalSupportsSequencedInput(hostID, opened.SessionID) {
				t.Fatal("sequenced input was not negotiated")
			}
			claim := terminal.InputFrame{Stream: "00000000000000000000000000000001"}
			for i := 0; i < 2; i++ {
				wait, err := f.service.BeginTerminalInput(ctx, hostID, opened.SessionID, claim)
				if err != nil {
					t.Fatal(err)
				}
				if err := wait(); err != nil {
					t.Fatal(err)
				}
			}
			if output, err := manager.Output(ctx, owner, opened.SessionID, 0, 0); err != nil || len(output.Data) != 0 {
				t.Fatalf("claim wrote PTY bytes: %q %v", output.Data, err)
			}
			if err := f.service.TerminalInput(ctx, hostID, TerminalInputRequest{SessionID: opened.SessionID, Data: "bGVnYWN5"}); !errors.Is(err, terminal.ErrInputSequence) {
				t.Fatalf("claimed terminal accepted raw input: %v", err)
			}
			var want strings.Builder
			waiters := make([]func() error, 0, terminal.InputWindow)
			var last terminal.InputFrame
			for seq := uint64(1); seq <= terminal.InputWindow; seq++ {
				last = terminal.InputFrame{Stream: "00000000000000000000000000000001", Seq: seq, Data: []byte(fmt.Sprintf("%02d中文\x03\r", seq))}
				want.Write(last.Data)
				wait, err := f.service.BeginTerminalInput(ctx, hostID, opened.SessionID, last)
				if err != nil {
					t.Fatal(err)
				}
				waiters = append(waiters, wait)
			}
			// All window frames are dispatched before the caller waits for any ACK.
			for _, wait := range waiters {
				if err := wait(); err != nil {
					t.Fatal(err)
				}
			}
			readStreamTerminalUntil(t, stream, 0, want.String())
			stream.mu.Lock()
			old := stream.conn
			old.close()
			stream.mu.Unlock()
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				stream.mu.Lock()
				connected := stream.conn != nil && stream.conn != old
				stream.mu.Unlock()
				if connected {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			wait, err := f.service.BeginTerminalInput(ctx, hostID, opened.SessionID, last)
			if err != nil {
				t.Fatal(err)
			}
			if err := wait(); err != nil {
				t.Fatal(err)
			}
			output, err := manager.Output(ctx, owner, opened.SessionID, 0, 0)
			if err != nil || string(output.Data) != want.String() {
				t.Fatalf("reconnect repeated/reordered input: %q %v", output.Data, err)
			}
			last.Seq++
			last.Data = []byte("block")
			blockedWait, err := f.service.BeginTerminalInput(ctx, hostID, opened.SessionID, last)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-process.entered:
			case <-time.After(time.Second):
				t.Fatal("PTY write did not block")
			}
			closeCtx, closeCancel := context.WithTimeout(ctx, time.Second)
			defer closeCancel()
			if err := f.service.TerminalResize(closeCtx, hostID, TerminalResizeRequest{SessionID: opened.SessionID, Rows: 30, Columns: 100}); err != nil {
				t.Fatalf("blocked PTY made stream resize unreachable: %v", err)
			}
			if err := f.service.TerminalClose(closeCtx, hostID, TerminalCloseRequest{SessionID: opened.SessionID}); err != nil {
				t.Fatalf("blocked PTY made stream close unreachable: %v", err)
			}
			if err := blockedWait(); err == nil {
				t.Fatal("blocked partial input was acknowledged")
			}
		})
	}
}

func TestReliableTerminalNeverDowngradesUnconfirmedInput(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	f.service.terminal = managerTerminalBackend{manager: newEchoManager(t)}
	var dials atomic.Int32
	stream, _, err := openStreamTerminal(context.Background(), context.Background(), "host", f.panelTerminalDialer(t, &dials), &recordingTerminalFallback{}, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.shutdown()
	if !stream.reliable {
		t.Fatal("missing capability")
	}
	stream.mu.Lock()
	stream.conn.close()
	stream.mu.Unlock()
	// Reliable reconnect always requests the same protocol and never marks the
	// session degraded. Legacy fallback remains independently covered.
	if stream.isDegraded() {
		t.Fatal("reliable session was downgraded")
	}
}

func TestRemovedPanelHostCannotReuseTerminalStream(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	f.service.terminal = managerTerminalBackend{manager: newEchoManager(t)}
	record := testHostRecordV2(1, time.Now().UTC())
	if err := f.service.storeV2.AddHost(record); err != nil {
		t.Fatal(err)
	}
	var dials atomic.Int32
	stream, opened, err := openStreamTerminal(context.Background(), context.Background(), record.ID, f.panelTerminalDialer(t, &dials), nil, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.shutdown()
	f.service.streams.putTerminal(stream)
	if _, err := f.service.storeV2.DeleteHost(record.ID, record.ResourceVersion); err != nil {
		t.Fatal(err)
	}
	frame := terminal.InputFrame{Stream: "00000000000000000000000000000001"}
	if _, err := f.service.BeginTerminalInput(context.Background(), record.ID, opened.SessionID, frame); !errors.Is(err, ErrTerminalUnavailable) {
		t.Fatalf("removed host accepted sequenced input: %v", err)
	}
	if err := f.service.TerminalInput(context.Background(), record.ID, TerminalInputRequest{SessionID: opened.SessionID, Data: "aW5wdXQ="}); !errors.Is(err, ErrTerminalUnavailable) {
		t.Fatalf("removed host accepted raw input: %v", err)
	}
	if err := f.service.TerminalResize(context.Background(), record.ID, TerminalResizeRequest{SessionID: opened.SessionID, Rows: 30, Columns: 100}); !errors.Is(err, ErrTerminalUnavailable) {
		t.Fatalf("removed host accepted resize: %v", err)
	}
}
