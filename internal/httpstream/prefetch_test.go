package httpstream

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type prefetchFaultReader struct {
	*bytes.Reader
	err error
}

func (r prefetchFaultReader) Read(p []byte) (int, error) {
	if r.Len() == 0 {
		return 0, r.err
	}
	return r.Reader.Read(p)
}

func TestPrefetchPreservesBytesAndTransportEnd(t *testing.T) {
	for _, size := range []int{0, 7, 8, 9, 25} {
		for _, end := range []error{io.EOF, io.ErrUnexpectedEOF, errors.New("connection lost")} {
			data := bytes.Repeat([]byte{byte(size)}, size)
			stream := NewPrefetchReadCloser(io.NopCloser(prefetchFaultReader{bytes.NewReader(data), end}), 8)
			got, err := io.ReadAll(stream)
			if !bytes.Equal(got, data) || (end == io.EOF && err != nil) || (end != io.EOF && !errors.Is(err, end)) {
				t.Fatalf("size=%d end=%v bytes=%x err=%v", size, end, got, err)
			}
			if err := stream.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

type prefetchBlockedSource struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
	closes  atomic.Int32
}

func (s *prefetchBlockedSource) Read([]byte) (int, error) {
	s.once.Do(func() { close(s.started) })
	<-s.closed
	return 0, io.ErrClosedPipe
}
func (s *prefetchBlockedSource) Close() error {
	if s.closes.Add(1) == 1 {
		close(s.closed)
	}
	return nil
}

func TestPrefetchCloseInterruptsSourceAndJoinsWorker(t *testing.T) {
	source := &prefetchBlockedSource{started: make(chan struct{}), closed: make(chan struct{})}
	stream := NewPrefetchReadCloser(source, 8).(*prefetchReadCloser)
	<-source.started
	finished := make(chan struct{})
	go func() { _ = stream.Close(); _ = stream.Close(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("close left the worker blocked")
	}
	select {
	case <-stream.done:
	default:
		t.Fatal("close returned before worker completion")
	}
	if source.closes.Load() != 1 {
		t.Fatalf("source closes=%d", source.closes.Load())
	}
}

type prefetchObservedSource struct {
	reads chan int
	count int
}

func (s *prefetchObservedSource) Read(p []byte) (int, error) {
	clear(p)
	s.count += len(p)
	s.reads <- s.count
	return len(p), nil
}
func (*prefetchObservedSource) Close() error { return nil }

func TestPrefetchBackpressureLimitsReadAheadToOneChunk(t *testing.T) {
	source := &prefetchObservedSource{reads: make(chan int, 8)}
	stream := NewPrefetchReadCloser(source, 8)
	defer stream.Close()
	if got := <-source.reads; got != 8 {
		t.Fatalf("first read=%d", got)
	}
	buffer := make([]byte, 8)
	if _, err := io.ReadFull(stream, buffer); err != nil {
		t.Fatal(err)
	}
	if got := <-source.reads; got != 16 {
		t.Fatalf("read ahead=%d", got)
	}
	select {
	case got := <-source.reads:
		t.Fatalf("unbounded read ahead=%d", got)
	case <-time.After(25 * time.Millisecond):
	}
}
