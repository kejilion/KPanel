package cluster

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type delayedTerminalBackend struct {
	managerTerminalBackend
	entered   chan struct{}
	delayOpen bool
}

func (b delayedTerminalBackend) Open(ctx context.Context, owner string, rows, columns uint16) (terminal.Snapshot, error) {
	if b.delayOpen {
		close(b.entered)
		<-ctx.Done()
		return terminal.Snapshot{}, ctx.Err()
	}
	return b.managerTerminalBackend.Open(ctx, owner, rows, columns)
}

func (b delayedTerminalBackend) Input(ctx context.Context, owner, id string, data []byte) error {
	close(b.entered)
	<-ctx.Done()
	return ctx.Err()
}

func TestTerminalStreamCanceledRequestReleasesPendingReply(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	entered := make(chan struct{})
	f.service.terminal = delayedTerminalBackend{managerTerminalBackend: managerTerminalBackend{manager: newEchoManager(t)}, entered: entered}
	var dials atomic.Int32
	stream, opened, err := openStreamTerminal(context.Background(), context.Background(), "host-1", f.panelTerminalDialer(t, &dials), nil, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.shutdown()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- stream.input(ctx, TerminalInputRequest{SessionID: opened.SessionID, Data: "eA=="}) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("input was not dispatched")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("input error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled input did not return")
	}
	stream.mu.Lock()
	pending := len(stream.pending)
	stream.mu.Unlock()
	if pending != 0 {
		t.Fatalf("canceled request left %d pending replies", pending)
	}
}

func TestTerminalStreamSentOpenCannotFallBackAsAnUnsentRequest(t *testing.T) {
	f := newStreamFixture(t, http.NotFoundHandler())
	entered := make(chan struct{})
	f.service.terminal = delayedTerminalBackend{managerTerminalBackend: managerTerminalBackend{manager: newEchoManager(t)}, entered: entered, delayOpen: true}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var dials atomic.Int32
	done := make(chan error, 1)
	go func() {
		_, _, err := openStreamTerminal(ctx, context.Background(), "host-1", f.panelTerminalDialer(t, &dials), nil, 24, 80)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("open was not dispatched")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || isStreamDialError(err) {
			t.Fatalf("already dispatched open must not be eligible for automatic v2 replay: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled open did not return")
	}
}
