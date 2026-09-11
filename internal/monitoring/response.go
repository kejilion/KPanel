package monitoring

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
)

// Compression applies to metric JSON before encrypted framing. The decoded
// limit is unchanged; compressed input has only a small framing allowance.
const GzipHistoryContentType = "application/vnd.kpanel.monitoring+gzip"
const MaxHistoryWireBytes = MaxHistoryResponseBytes + 64<<10

func ReadHistoryPayload(source io.Reader, compressed bool) ([]byte, error) {
	if !compressed {
		return readHistoryPayload(source)
	}
	wire := &io.LimitedReader{R: source, N: MaxHistoryWireBytes + 1}
	buffer := bufio.NewReader(wire)
	reader, err := gzip.NewReader(buffer)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	reader.Multistream(false)
	content, err := readHistoryPayload(reader)
	if err != nil {
		return nil, err
	}
	// A valid gzip footer is not an authenticated transport END. Read through
	// the underlying stream and reject trailing members/data or a missing END.
	if _, err := buffer.ReadByte(); err != io.EOF {
		return nil, errors.New("history stream has trailing data or no authenticated end")
	}
	if wire.N <= 0 {
		return nil, errors.New("compressed history exceeds limit")
	}
	return content, nil
}

func readHistoryPayload(source io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(source, MaxHistoryResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > MaxHistoryResponseBytes {
		return nil, errors.New("history response exceeds limit")
	}
	return content, nil
}

// CopyHistoryPayload keeps compression streaming and low-CPU. Never close a
// partial gzip member successfully when the source fails or exceeds its limit.
func CopyHistoryPayload(destination io.Writer, source io.Reader, compressed bool) error {
	var zipper *gzip.Writer
	var buffer *bufio.Writer
	if compressed {
		buffer = bufio.NewWriterSize(destination, 32<<10)
		zipper, _ = gzip.NewWriterLevel(buffer, gzip.BestSpeed)
		destination = zipper
	}
	reader := &io.LimitedReader{R: source, N: MaxHistoryResponseBytes + 1}
	_, err := io.CopyBuffer(destination, reader, make([]byte, 60<<10))
	if err != nil {
		return err
	}
	if reader.N <= 0 {
		return errors.New("history response exceeds limit")
	}
	if zipper != nil {
		if err := zipper.Close(); err != nil {
			return err
		}
		return buffer.Flush()
	}
	return nil
}

// HistoryResponseWriter compresses successful node responses only. Errors keep
// their normal problem JSON. Close must succeed before the relay sends END.
type HistoryResponseWriter struct {
	http.ResponseWriter
	zipper  *gzip.Writer
	buffer  *bufio.Writer
	status  int
	written int64
	err     error
}

func NewHistoryResponseWriter(w http.ResponseWriter) *HistoryResponseWriter {
	return &HistoryResponseWriter{ResponseWriter: w}
}

func (w *HistoryResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	if status == http.StatusOK {
		w.Header().Set("Content-Type", GzipHistoryContentType)
		w.buffer = bufio.NewWriterSize(w.ResponseWriter, 32<<10)
		w.zipper, _ = gzip.NewWriterLevel(w.buffer, gzip.BestSpeed)
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *HistoryResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.err != nil {
		return 0, w.err
	}
	if w.written+int64(len(data)) > MaxHistoryResponseBytes {
		w.err = errors.New("history response exceeds limit")
		return 0, w.err
	}
	var n int
	if w.zipper != nil {
		n, w.err = w.zipper.Write(data)
	} else {
		n, w.err = w.ResponseWriter.Write(data)
	}
	w.written += int64(n)
	return n, w.err
}

func (w *HistoryResponseWriter) Close() error {
	if w.err != nil {
		return w.err
	}
	if w.zipper != nil {
		if err := w.zipper.Close(); err != nil {
			return err
		}
		return w.buffer.Flush()
	}
	return nil
}
