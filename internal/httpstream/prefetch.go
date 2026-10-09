package httpstream

import (
	"io"
	"sync"
)

// NewPrefetchReadCloser overlaps a sequential consumer with at most one extra
// chunk of source reads. The source must allow Close to interrupt Read, as HTTP
// response bodies do. Close joins the worker before returning.
func NewPrefetchReadCloser(source io.ReadCloser, chunkBytes int) io.ReadCloser {
	if chunkBytes <= 0 || chunkBytes > 8<<20 {
		panic("invalid prefetch chunk size")
	}
	reader, writer := io.Pipe()
	stream := &prefetchReadCloser{source: source, reader: reader, done: make(chan struct{})}
	go func() {
		defer close(stream.done)
		buffer := make([]byte, chunkBytes)
		for {
			used, emptyReads := 0, 0
			var readErr error
			for used < len(buffer) {
				var n int
				n, readErr = source.Read(buffer[used:])
				used += n
				if readErr != nil {
					break
				}
				if n == 0 {
					emptyReads++
					if emptyReads == 100 {
						readErr = io.ErrNoProgress
						break
					}
				} else {
					emptyReads = 0
				}
			}
			if used > 0 {
				if _, err := writer.Write(buffer[:used]); err != nil {
					_ = writer.CloseWithError(err)
					return
				}
			}
			if readErr != nil {
				// Preserve an actual source EOF separately from a truncated HTTP
				// body (ErrUnexpectedEOF); the receiver must never publish the latter.
				_ = writer.CloseWithError(readErr)
				return
			}
		}
	}()
	return stream
}

type prefetchReadCloser struct {
	source io.ReadCloser
	reader *io.PipeReader
	done   chan struct{}
	once   sync.Once
	err    error
}

func (s *prefetchReadCloser) Read(buffer []byte) (int, error) { return s.reader.Read(buffer) }

func (s *prefetchReadCloser) Close() error {
	s.once.Do(func() {
		_ = s.reader.Close()
		s.err = s.source.Close()
		<-s.done
	})
	return s.err
}
