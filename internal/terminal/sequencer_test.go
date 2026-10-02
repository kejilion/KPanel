package terminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	streamA = "00000000000000000000000000000001"
	streamB = "00000000000000000000000000000002"
)

type sink struct {
	mu     sync.Mutex
	data   bytes.Buffer
	writes int
	err    error
}

func (s *sink) write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes++
	if s.err != nil {
		return s.err
	}
	s.data.Write(data)
	return nil
}

func (s *sink) content() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.String()
}

func admit(q *InputSequencer, s *sink, stream string, seq uint64, data string) error {
	return q.Admit(context.Background(), InputFrame{Stream: stream, Seq: seq, Data: []byte(data)}, s.write)
}

func TestInputSequencerClaimOrderAndReplay(t *testing.T) {
	q, s := NewInputSequencer(), &sink{}
	if q.Claimed() {
		t.Fatal("new sequencer is claimed")
	}
	if err := admit(q, s, streamA, 1, "early"); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("data before claim: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := admit(q, s, streamA, 0, ""); err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
	}
	if !q.Claimed() {
		t.Fatal("claim was not recorded")
	}
	if err := admit(q, s, streamA, 2, "gap"); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("gap: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := admit(q, s, streamA, 1, "中文\x03\r"); err != nil {
			t.Fatalf("seq 1 attempt %d: %v", i, err)
		}
	}
	if s.content() != "中文\x03\r" || s.writes != 1 {
		t.Fatalf("replay reached the sink: %q writes=%d", s.content(), s.writes)
	}
	if err := admit(q, s, streamA, 1, "different"); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("same seq changed bytes: %v", err)
	}
	for seq := uint64(2); seq <= InputWindow+1; seq++ {
		if err := admit(q, s, streamA, seq, fmt.Sprintf("<%d>", seq)); err != nil {
			t.Fatalf("seq %d: %v", seq, err)
		}
	}
	if err := admit(q, s, streamA, 1, "中文\x03\r"); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("replay outside the window: %v", err)
	}
	if err := admit(q, s, streamA, 2, "<2>"); err != nil {
		t.Fatalf("replay inside the window: %v", err)
	}
}

func TestInputSequencerNewStreamFencesThePreviousWriter(t *testing.T) {
	q, s := NewInputSequencer(), &sink{}
	if err := admit(q, s, streamA, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := admit(q, s, streamA, 1, "a1"); err != nil {
		t.Fatal(err)
	}
	// A reloaded page claims with a fresh stream and restarts at seq 1.
	if err := admit(q, s, streamB, 0, ""); err != nil {
		t.Fatalf("takeover: %v", err)
	}
	if err := admit(q, s, streamA, 2, "a2"); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("fenced writer: %v", err)
	}
	if err := admit(q, s, streamA, 0, ""); err != nil {
		t.Fatalf("a stale page may reclaim; it fences the newer one: %v", err)
	}
	if err := admit(q, s, streamB, 1, "b1"); !errors.Is(err, ErrInputSequence) {
		t.Fatalf("writer fenced by the later claim: %v", err)
	}
	if err := admit(q, s, streamB, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := admit(q, s, streamB, 1, "b1"); err != nil {
		t.Fatalf("new stream starts at seq 1: %v", err)
	}
	if s.content() != "a1b1" {
		t.Fatalf("bytes: %q", s.content())
	}
}

func TestInputSequencerUnwrittenFailureStaysRetryable(t *testing.T) {
	q, s := NewInputSequencer(), &sink{}
	if err := admit(q, s, streamA, 0, ""); err != nil {
		t.Fatal(err)
	}
	s.err = fmt.Errorf("fifo busy: %w", ErrInputNotWritten)
	if err := admit(q, s, streamA, 1, "cmd\r"); !errors.Is(err, ErrInputNotWritten) {
		t.Fatalf("unwritten failure: %v", err)
	}
	s.err = nil
	if err := admit(q, s, streamA, 1, "cmd\r"); err != nil {
		t.Fatalf("retry of the same frame: %v", err)
	}
	if err := admit(q, s, streamA, 2, "more"); err != nil {
		t.Fatalf("stream continues: %v", err)
	}
	if s.content() != "cmd\rmore" {
		t.Fatalf("bytes: %q", s.content())
	}
}

func TestInputSequencerUncertainWriteFreezesUntilANewStreamClaims(t *testing.T) {
	q, s := NewInputSequencer(), &sink{}
	if err := admit(q, s, streamA, 0, ""); err != nil {
		t.Fatal(err)
	}
	s.err = errors.New("short write")
	if err := admit(q, s, streamA, 1, "cmd\r"); !errors.Is(err, ErrInputUncertain) {
		t.Fatalf("uncertain write: %v", err)
	}
	s.err = nil
	writes := s.writes
	for _, call := range []func() error{
		func() error { return admit(q, s, streamA, 1, "cmd\r") },
		func() error { return admit(q, s, streamA, 2, "next") },
		func() error { return admit(q, s, streamA, 0, "") },
	} {
		if err := call(); !errors.Is(err, ErrInputUncertain) {
			t.Fatalf("frozen stream: %v", err)
		}
	}
	if s.writes != writes {
		t.Fatalf("frozen stream wrote again: %d", s.writes-writes)
	}
	// The unsure bytes can never be replayed by a fresh stream, so it may continue.
	if err := admit(q, s, streamB, 0, ""); err != nil {
		t.Fatalf("new stream after freeze: %v", err)
	}
	if err := admit(q, s, streamB, 1, "fresh"); err != nil {
		t.Fatal(err)
	}
}

func TestInputSequencerFinishedSinkIsReportedWithoutFreezing(t *testing.T) {
	q, s := NewInputSequencer(), &sink{}
	if err := admit(q, s, streamA, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := admit(q, s, streamA, 1, "a"); err != nil {
		t.Fatal(err)
	}
	for _, finished := range []error{ErrClosed, ErrNotFound} {
		s.err = fmt.Errorf("task ended: %w", finished)
		if err := admit(q, s, streamA, 2, "b"); !errors.Is(err, finished) {
			t.Fatalf("finished sink %v: %v", finished, err)
		}
	}
	if !q.Claimed() || s.content() != "a" {
		t.Fatalf("finished sink changed state: claimed=%v bytes=%q", q.Claimed(), s.content())
	}
	// Nothing was written, so the frame is still owed and the stream not frozen.
	s.err = nil
	if err := admit(q, s, streamA, 2, "b"); err != nil {
		t.Fatalf("a finished sink does not freeze the stream: %v", err)
	}
}

func TestInputSequencerRejectsMalformedFrames(t *testing.T) {
	q, s := NewInputSequencer(), &sink{}
	for _, frame := range []InputFrame{
		{Stream: "short", Seq: 0},
		{Stream: streamA, Seq: 0, Data: []byte("x")},
		{Stream: streamA, Seq: 1},
		{Stream: streamA, Seq: 1, Data: bytes.Repeat([]byte("x"), InputFrameBytes+1)},
	} {
		if err := q.Admit(context.Background(), frame, s.write); !errors.Is(err, ErrInputSequence) {
			t.Fatalf("frame %+v: %v", frame, err)
		}
	}
	if s.writes != 0 {
		t.Fatal("malformed frame reached the sink")
	}
}

func TestInputSequencerSerializesWritesAndHonoursCancellation(t *testing.T) {
	q := NewInputSequencer()
	if err := q.Admit(context.Background(), InputFrame{Stream: streamA}, nil); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- q.Admit(context.Background(), InputFrame{Stream: streamA, Seq: 1, Data: []byte("one")}, func([]byte) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	queued := make(chan error, 1)
	go func() {
		queued <- q.Admit(ctx, InputFrame{Stream: streamA, Seq: 2, Data: []byte("two")}, func([]byte) error {
			t.Error("a canceled frame was written")
			return nil
		})
	}()
	cancel()
	select {
	case err := <-queued:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued frame: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled frame stayed behind the blocked write")
	}
	// Claimed() must not wait for an in-flight write either.
	done := make(chan bool, 1)
	go func() { done <- q.Claimed() }()
	select {
	case claimed := <-done:
		if !claimed {
			t.Fatal("claim lost")
		}
	case <-time.After(time.Second):
		t.Fatal("Claimed blocked behind a PTY-style write")
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestInputSequencerEpochDiffersPerInstance(t *testing.T) {
	a, b := NewInputSequencer(), NewInputSequencer()
	if len(a.Epoch()) != 32 || a.Epoch() == b.Epoch() {
		t.Fatalf("epochs %q %q", a.Epoch(), b.Epoch())
	}
}

// The task-terminal sequencer and the PTY owner implement one contract. Run the
// single-stream scenario through both so they cannot drift apart silently.
func TestInputSequencerMatchesManagerForSingleStreamTraffic(t *testing.T) {
	process := newFakeProcess()
	manager := New(Config{Starter: func(uint16, uint16) (Process, error) { return process, nil }})
	defer manager.CloseAll()
	opened, err := manager.Open("owner", 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	q, s := NewInputSequencer(), &sink{}
	steps := []struct {
		name  string
		frame InputFrame
		want  error
	}{
		{"claim", InputFrame{Stream: streamA}, nil},
		{"claim again", InputFrame{Stream: streamA}, nil},
		{"first", InputFrame{Stream: streamA, Seq: 1, Data: []byte("one")}, nil},
		{"replay", InputFrame{Stream: streamA, Seq: 1, Data: []byte("one")}, nil},
		{"changed replay", InputFrame{Stream: streamA, Seq: 1, Data: []byte("uno")}, ErrInputSequence},
		{"gap", InputFrame{Stream: streamA, Seq: 3, Data: []byte("three")}, ErrInputSequence},
		{"second", InputFrame{Stream: streamA, Seq: 2, Data: []byte("two")}, nil},
		{"oversized", InputFrame{Stream: streamA, Seq: 3, Data: []byte(strings.Repeat("x", InputFrameBytes+1))}, ErrInputSequence},
	}
	for _, step := range steps {
		got := manager.InputSequenced("owner", opened.ID, step.frame)
		mine := q.Admit(context.Background(), step.frame, s.write)
		if !errors.Is(got, step.want) && !(step.want == nil && got == nil) {
			t.Fatalf("%s: manager returned %v, want %v", step.name, got, step.want)
		}
		if !errors.Is(mine, step.want) && !(step.want == nil && mine == nil) {
			t.Fatalf("%s: sequencer returned %v, want %v", step.name, mine, step.want)
		}
	}
	process.mu.Lock()
	managerBytes := process.input.String()
	process.mu.Unlock()
	if managerBytes != s.content() || s.content() != "onetwo" {
		t.Fatalf("delivered bytes differ: manager %q sequencer %q", managerBytes, s.content())
	}
}
