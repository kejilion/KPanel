package main

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

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
