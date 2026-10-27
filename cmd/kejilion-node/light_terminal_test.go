package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type terminalRetryTransport func(*http.Request) (*http.Response, error)

func (transport terminalRetryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestLightTerminalAuthenticationFailuresBackOff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows capability admission uses a separate authenticated transport")
	}
	synctest.Test(t, func(t *testing.T) {
		key, err := cluster.GenerateFederationV2Keypair()
		if err != nil {
			t.Fatal(err)
		}
		requests := make(chan time.Time)
		client := &http.Client{Transport: terminalRetryTransport(func(request *http.Request) (*http.Response, error) {
			requests <- time.Now()
			return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		})}
		relay, err := cluster.NewTerminalRelayClient(client)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		manager := terminal.New(terminal.Config{})
		defer manager.CloseAll()
		done := make(chan struct{})
		go func() {
			defer close(done)
			runLightTerminalControl(ctx, nodeConfig{Origin: "https://example.com", NodeID: strings.Repeat("a", 32), TargetNodeID: strings.Repeat("b", 32)}, terminalIdentity{Key: key, Peer: key.Public}, relay, manager)
		}()
		previous := <-requests
		for _, expected := range []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, time.Minute, time.Minute} {
			current := <-requests
			if delay := current.Sub(previous); delay != expected {
				t.Fatalf("retry delay = %v, want %v", delay, expected)
			}
			previous = current
		}
		cancel()
		<-done
	})
}

type lightTerminalTestProcess struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	mu     sync.Mutex
	closed bool
}

func newLightTerminalTestProcess() *lightTerminalTestProcess {
	reader, writer := io.Pipe()
	return &lightTerminalTestProcess{reader: reader, writer: writer}
}

func (process *lightTerminalTestProcess) Read(data []byte) (int, error) {
	return process.reader.Read(data)
}

func (process *lightTerminalTestProcess) Write(data []byte) (int, error) {
	return len(data), nil
}

func (process *lightTerminalTestProcess) Close() error {
	process.mu.Lock()
	process.closed = true
	process.mu.Unlock()
	_ = process.writer.Close()
	return process.reader.Close()
}

func (process *lightTerminalTestProcess) Wait() error                 { return nil }
func (process *lightTerminalTestProcess) Kill() error                 { return nil }
func (process *lightTerminalTestProcess) Resize(uint16, uint16) error { return nil }

func TestLightTerminalOutputFitsAggregateV2PayloadLimit(t *testing.T) {
	control := &lightTerminalControl{
		sessions: map[string]lightNodeTerminalSession{
			strings.Repeat("a", 32): {},
			strings.Repeat("b", 32): {},
			strings.Repeat("c", 32): {},
			strings.Repeat("d", 32): {},
		},
		pendingEvents: []cluster.TerminalRelayEvent{
			{Kind: "output", SessionID: strings.Repeat("b", 32), Data: make([]byte, 8<<10), NextOffset: 8 << 10},
			{Kind: "output", SessionID: strings.Repeat("c", 32), Data: make([]byte, 8<<10), NextOffset: 8 << 10},
			{Kind: "output", SessionID: strings.Repeat("d", 32), Data: make([]byte, 8<<10), NextOffset: 8 << 10},
		},
	}
	event := cluster.TerminalRelayEvent{
		Kind: "output", SessionID: strings.Repeat("a", 32),
		Offset: 0, NextOffset: 32 << 10, Data: make([]byte, 32<<10),
	}
	fitted, ok := control.fitOutputEvent(event)
	if !ok || len(fitted.Data) == 0 || len(fitted.Data) >= len(event.Data) {
		t.Fatalf("fitOutputEvent() = ok:%v bytes:%d, want a smaller non-empty chunk", ok, len(fitted.Data))
	}
	payload, err := json.Marshal(cluster.TerminalRelayPollRequest{
		SessionIDs: control.sessionIDs(), Events: []cluster.TerminalRelayEvent{fitted},
	})
	if err != nil || len(payload) > cluster.MaxSummaryBytes {
		t.Fatalf("fitted relay payload size = %d, error = %v", len(payload), err)
	}
	if fitted.NextOffset != int64(len(fitted.Data)) {
		t.Fatalf("fitted relay offset = %d, data = %d", fitted.NextOffset, len(fitted.Data))
	}
}

func TestLightTerminalControlClosesSessionsWhenCenterEpochChanges(t *testing.T) {
	process := newLightTerminalTestProcess()
	manager := terminal.New(terminal.Config{
		Starter: func(uint16, uint16) (terminal.Process, error) { return process, nil },
	})
	defer manager.CloseAll()
	nodeID := strings.Repeat("a", 32)
	snapshot, err := manager.Open(lightTerminalOwner(nodeID), 24, 80)
	if err != nil {
		t.Fatalf("manager.Open() error = %v", err)
	}
	control := &lightTerminalControl{
		config:      nodeConfig{NodeID: nodeID},
		manager:     manager,
		sessions:    map[string]lightNodeTerminalSession{strings.Repeat("b", 32): {localID: snapshot.ID}},
		centerEpoch: strings.Repeat("c", 32),
		processed:   make(map[string]cluster.TerminalRelayEvent),
	}
	control.acceptRelayResponse(cluster.TerminalRelayPollResponse{Epoch: strings.Repeat("d", 32)})
	if len(control.sessions) != 0 {
		t.Fatalf("center epoch change kept %d local sessions", len(control.sessions))
	}
	output, err := manager.Output(context.Background(), lightTerminalOwner(nodeID), snapshot.ID, 0, 0)
	if err != nil || !output.Closed {
		t.Fatalf("manager.Output() after epoch reset = %#v, %v; want closed", output, err)
	}
}

func TestRetainedRelayOpenAcknowledgesAndEpochClosesStreamSessions(t *testing.T) {
	manager := terminal.New(terminal.Config{Starter: func(uint16, uint16) (terminal.Process, error) { return newLightTerminalTestProcess(), nil }})
	defer manager.CloseAll()
	nodeID := strings.Repeat("a", 32)
	owner := lightTerminalOwner(nodeID)
	control := &lightTerminalControl{config: nodeConfig{NodeID: nodeID}, manager: manager, sessions: make(map[string]lightNodeTerminalSession), processed: make(map[string]cluster.TerminalRelayEvent), centerEpoch: strings.Repeat("c", 32)}
	payload, _ := json.Marshal(cluster.TerminalOpenRequest{Rows: 24, Columns: 80, Retained: true})
	command := cluster.TerminalRelayCommand{ID: strings.Repeat("d", 32), SessionID: strings.Repeat("e", 32), Path: cluster.V2TerminalOpenPath, Payload: payload}
	if err := control.executeCommand(command); err != nil {
		t.Fatal(err)
	}
	item := control.sessions[command.SessionID]
	if !manager.IsRetained(owner, item.localID) || len(control.pendingEvents) != 1 || !control.pendingEvents[0].Retained {
		t.Fatal("retention not acknowledged")
	}
	// Stream sessions are deliberately absent from relay's session map.
	stream, err := manager.OpenRetained(owner, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	control.acceptRelayResponse(cluster.TerminalRelayPollResponse{Epoch: strings.Repeat("f", 32)})
	for _, id := range []string{item.localID, stream.ID} {
		out, err := manager.Output(context.Background(), owner, id, 0, 0)
		if err != nil || !out.Closed {
			t.Fatalf("epoch kept session: %+v %v", out, err)
		}
	}
}
