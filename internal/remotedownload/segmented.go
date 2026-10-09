package remotedownload

import (
	"context"
	"io"
	"net/http"
	"sync"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	segmentBytes      = 2 << 20
	minSegmentedBytes = 32 << 20
)

// OpenSegmented opportunistically reads a large immutable representation through
// bounded Range requests. Small, unknown-length, weakly validated and non-Range
// responses keep the ordinary stream. Every request retains the SSRF policy.
func (c *Client) OpenSegmented(ctx context.Context, raw string, connections int) (*http.Response, error) {
	response, err := c.Open(ctx, raw)
	if err != nil {
		return nil, err
	}
	if connections < 2 || response.ContentLength < minSegmentedBytes || response.ContentLength > contract.MaxFileTransferBytes || !StrongETag(response.Header.Get("ETag")) {
		return response, nil
	}
	connections = min(connections, 4)
	identity := ResumeRequest{SizeBytes: response.ContentLength, ETag: response.Header.Get("ETag"), FinalURL: response.Request.URL.String()}
	// Release the initial connection before probing. Concurrent tasks must never
	// hold every connection in the shared pool while waiting for another one.
	_ = response.Body.Close()
	streamContext, cancel := context.WithCancel(ctx)
	probe := identity
	probe.endOffset = segmentBytes
	first, err := c.open(streamContext, raw, &probe)
	if err != nil {
		cancel()
		return nil, err
	}
	if first.StatusCode == http.StatusOK {
		if first.ContentLength != identity.SizeBytes {
			_ = first.Body.Close()
			cancel()
			return nil, ErrSourceChanged
		}
		// An ignored Range already supplied a complete response: consume it once.
		first.Body = &cancelReadCloser{ReadCloser: first.Body, cancel: cancel}
		return first, nil
	}
	body := &segmentedBody{ctx: streamContext, cancel: cancel, done: make(chan struct{}),
		first: first.Body,
		jobs:  make(chan segmentJob, connections), results: make(chan segmentResult, connections),
		pending: make(map[int64]segmentResult), size: identity.SizeBytes}
	var workers sync.WaitGroup
	for index := 0; index < connections; index++ {
		job := segmentJob{offset: int64(index * segmentBytes), buffer: make([]byte, segmentBytes)}
		if index == 0 {
			job.response = first
		}
		body.jobs <- job
		body.scheduled += segmentBytes
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-streamContext.Done():
					return
				case job := <-body.jobs:
					request := identity
					request.Offset, request.endOffset = job.offset, min(job.offset+segmentBytes, identity.SizeBytes)
					result := segmentResult{offset: job.offset, buffer: job.buffer}
					part := job.response
					if part == nil {
						part, result.err = c.open(streamContext, raw, &request)
					}
					if result.err == nil {
						if part.StatusCode != http.StatusPartialContent {
							result.err = ErrSourceChanged
						} else {
							result.buffer = job.buffer[:request.endOffset-request.Offset]
							result.err = readExactSegment(streamContext, part.Body, result.buffer)
						}
						_ = part.Body.Close()
					}
					select {
					case body.results <- result:
					case <-streamContext.Done():
						return
					}
				}
			}
		}()
	}
	go func() { workers.Wait(); close(body.done) }()
	response.Body = body
	return response, nil
}

func readExactSegment(ctx context.Context, body io.ReadCloser, buffer []byte) error {
	stop := context.AfterFunc(ctx, func() { _ = body.Close() })
	defer stop()
	used, empty := 0, 0
	for used < len(buffer) {
		n, err := body.Read(buffer[used:])
		used += n
		if err == io.EOF {
			if used == len(buffer) {
				return nil
			}
			return io.ErrUnexpectedEOF
		}
		if err != nil {
			return err
		}
		if n == 0 {
			empty++
		} else {
			empty = 0
		}
		if empty == 100 {
			return io.ErrNoProgress
		}
	}
	var extra [1]byte
	for empty := 0; empty < 100; empty++ {
		n, err := body.Read(extra[:])
		if n != 0 {
			return ErrSourceChanged
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return io.ErrNoProgress
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelReadCloser) Close() error { r.cancel(); return r.ReadCloser.Close() }

type segmentJob struct {
	offset   int64
	buffer   []byte
	response *http.Response
}
type segmentResult struct {
	offset int64
	buffer []byte
	err    error
}

type segmentedBody struct {
	ctx                   context.Context
	first                 io.ReadCloser
	cancel                context.CancelFunc
	done                  chan struct{}
	jobs                  chan segmentJob
	results               chan segmentResult
	pending               map[int64]segmentResult
	current               segmentResult
	used                  int
	next, scheduled, size int64
	err                   error
}

// ReadAheadBytes lets the receiver avoid stacking another prefetch buffer.
func (s *segmentedBody) ReadAheadBytes() int { return cap(s.jobs) * segmentBytes }

func (s *segmentedBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	if s.err != nil {
		return 0, s.err
	}
	if len(s.current.buffer) == s.used {
		if s.current.buffer != nil && s.scheduled < s.size {
			select {
			case s.jobs <- segmentJob{offset: s.scheduled, buffer: s.current.buffer[:segmentBytes]}:
				s.scheduled += segmentBytes
			case <-s.ctx.Done():
				return 0, s.ctx.Err()
			}
		}
		s.current, s.used = segmentResult{}, 0
		if s.next >= s.size {
			return 0, io.EOF
		}
		for {
			if result, ok := s.pending[s.next]; ok {
				delete(s.pending, s.next)
				s.current = result
				break
			}
			select {
			case result := <-s.results:
				s.pending[result.offset] = result
			case <-s.ctx.Done():
				return 0, s.ctx.Err()
			}
		}
		if err := s.ctx.Err(); err != nil {
			return 0, err
		}
		if s.current.err != nil {
			s.err = s.current.err
			s.cancel()
			return 0, s.err
		}
		s.next += int64(len(s.current.buffer))
	}
	n := copy(buffer, s.current.buffer[s.used:])
	s.used += n
	return n, nil
}

func (s *segmentedBody) Close() error {
	s.cancel()
	_ = s.first.Close()
	<-s.done
	return nil
}
