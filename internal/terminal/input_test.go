package terminal

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type blockedInputProcess struct {
	*fakeProcess
	entered  chan struct{}
	released chan struct{}
	once     sync.Once
}

func (p *blockedInputProcess) Write([]byte) (int, error) {
	close(p.entered)
	<-p.released
	return 0, ErrClosed
}
func (p *blockedInputProcess) Kill() error {
	p.once.Do(func() { close(p.released) })
	return p.fakeProcess.Kill()
}

func TestSequencedInputBlockedWriteDoesNotBlockOutputOrClose(t *testing.T) {
	p := &blockedInputProcess{fakeProcess: newFakeProcess(), entered: make(chan struct{}), released: make(chan struct{})}
	m := New(Config{Starter: func(uint16, uint16) (Process, error) { return p, nil }})
	defer m.CloseAll()
	s, err := m.Open("owner", 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- m.InputSequenced("owner", s.ID, InputFrame{Stream: "00000000000000000000000000000001", Seq: 1, Data: []byte("blocked")})
	}()
	<-p.entered
	queuedCtx, queuedCancel := context.WithCancel(context.Background())
	queuedDone := make(chan error, 1)
	go func() {
		queuedDone <- m.InputSequencedContext(queuedCtx, "owner", s.ID, InputFrame{Stream: "00000000000000000000000000000001", Seq: 2, Data: []byte("queued")})
	}()
	queuedCancel()
	select {
	case err := <-queuedDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued input cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled input remained stuck behind PTY write")
	}
	closed := make(chan error, 1)
	go func() {
		_, err := m.Output(context.Background(), "owner", s.ID, 0, 0)
		if err == nil {
			err = m.Close("owner", s.ID)
		}
		closed <- err
	}()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("PTY write held the state lock and prevented close")
	}
	if err := <-done; !errors.Is(err, ErrInputUncertain) {
		t.Fatalf("interrupted partial input: %v", err)
	}
}

func TestSequencedInputDeduplicatesAndRejectsAmbiguity(t *testing.T) {
	p := newFakeProcess()
	m := New(Config{Starter: func(uint16, uint16) (Process, error) { return p, nil }})
	defer m.CloseAll()
	s, err := m.Open("owner", 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	frame := InputFrame{Stream: "00000000000000000000000000000001", Seq: 1, Data: []byte("中文\x03\r")}
	claim := InputFrame{Stream: frame.Stream}
	for i := 0; i < 2; i++ {
		if err := m.InputSequenced("owner", s.ID, claim); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Input("owner", s.ID, []byte("before-first-frame")); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("unsequenced input after claim: %v", err)
	}
	if err := m.InputSequenced("other", s.ID, frame); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong owner: %v", err)
	}
	if err := m.InputSequenced("owner", s.ID, InputFrame{Stream: frame.Stream, Seq: 2, Data: frame.Data}); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("gap: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := m.InputSequenced("owner", s.ID, frame); err != nil {
			t.Fatal(err)
		}
	}
	p.mu.Lock()
	actual := p.input.String()
	p.mu.Unlock()
	if actual != string(frame.Data) {
		t.Fatalf("replayed bytes: %q", actual)
	}
	changed := frame
	changed.Data = []byte("different")
	if err := m.InputSequenced("owner", s.ID, changed); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("same seq changed bytes: %v", err)
	}
	changed = frame
	changed.Stream = "00000000000000000000000000000002"
	if err := m.InputSequenced("owner", s.ID, changed); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("changed stream: %v", err)
	}
	if err := m.Input("owner", s.ID, []byte("legacy")); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("mixed legacy writer: %v", err)
	}
	for seq := uint64(2); seq <= InputWindow+1; seq++ {
		next := frame
		next.Seq = seq
		if err := m.InputSequenced("owner", s.ID, next); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.InputSequenced("owner", s.ID, frame); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("expired replay: %v", err)
	}
	if err := m.Close("owner", s.ID); err != nil {
		t.Fatal(err)
	}
	frame.Seq = InputWindow + 1
	if err := m.InputSequenced("owner", s.ID, frame); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed replay: %v", err)
	}
}

type partialInputProcess struct {
	*fakeProcess
	writeErr error
	calls    int
}

func (p *partialInputProcess) Write(data []byte) (int, error) {
	p.calls++
	return len(data) - 1, p.writeErr
}

func TestSequencedInputPartialWriteFreezesWithoutReplay(t *testing.T) {
	for _, writeErr := range []error{nil, errors.New("write failed")} {
		p := &partialInputProcess{fakeProcess: newFakeProcess(), writeErr: writeErr}
		m := New(Config{Starter: func(uint16, uint16) (Process, error) { return p, nil }})
		s, err := m.Open("owner", 24, 80)
		if err != nil {
			t.Fatal(err)
		}
		frame := InputFrame{Stream: "00000000000000000000000000000001", Seq: 1, Data: []byte("command\r")}
		for i := 0; i < 2; i++ {
			if err := m.InputSequenced("owner", s.ID, frame); !errors.Is(err, ErrInputUncertain) {
				t.Fatalf("partial write: %v", err)
			}
		}
		if p.calls != 1 {
			t.Fatalf("partial write replayed %d times", p.calls)
		}
		m.CloseAll()
	}
}
