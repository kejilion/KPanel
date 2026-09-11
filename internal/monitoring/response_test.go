package monitoring

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func packedHistory(t *testing.T, data []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := CopyHistoryPayload(&output, bytes.NewReader(data), true); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

type failedHistoryEnd struct{ io.Reader }

func (r failedHistoryEnd) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		return n, errors.New("authenticated END missing")
	}
	return n, err
}

func TestHistoryCompressionIntegrityAndLimits(t *testing.T) {
	data := []byte(strings.Repeat(`{"at":"2026-09-11","cpu":32.5}`, 300))
	packed := packedHistory(t, data)
	for _, compressed := range []bool{false, true} {
		input := data
		if compressed {
			input = packed
		}
		got, err := ReadHistoryPayload(bytes.NewReader(input), compressed)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("compressed=%v: %v", compressed, err)
		}
	}
	corrupt := bytes.Clone(packed)
	corrupt[len(corrupt)-8] ^= 1
	for name, input := range map[string]io.Reader{
		"checksum":            bytes.NewReader(corrupt),
		"truncated":           bytes.NewReader(packed[:len(packed)-1]),
		"trailing":            bytes.NewReader(append(bytes.Clone(packed), 'x')),
		"members":             bytes.NewReader(append(bytes.Clone(packed), packed...)),
		"unauthenticated-end": failedHistoryEnd{bytes.NewReader(packed)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ReadHistoryPayload(input, true); err == nil {
				t.Fatal("invalid compressed response accepted")
			}
		})
	}
	// A very small compressed response must not bypass the decoded byte budget.
	var bomb bytes.Buffer
	writer := NewHistoryResponseWriter(httptest.NewRecorder())
	if _, err := writer.Write(make([]byte, MaxHistoryResponseBytes+1)); err == nil || writer.Close() == nil {
		t.Fatal("oversized source accepted")
	}
	if err := CopyHistoryPayload(&bomb, strings.NewReader(strings.Repeat("x", int(MaxHistoryResponseBytes)+1)), true); err == nil {
		t.Fatal("oversized copy accepted")
	}
	if _, err := ReadHistoryPayload(bytes.NewReader(bomb.Bytes()), true); err == nil {
		t.Fatal("partial oversized gzip accepted")
	}
	bomb.Reset()
	zipper := gzip.NewWriter(&bomb)
	_, _ = io.Copy(zipper, strings.NewReader(strings.Repeat("x", int(MaxHistoryResponseBytes)+1)))
	_ = zipper.Close()
	if _, err := ReadHistoryPayload(bytes.NewReader(bomb.Bytes()), true); err == nil {
		t.Fatal("valid gzip bypassed decoded size limit")
	}
}

func TestHistoryResponseWriterKeepsErrorsAndQueryContract(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		recorder := httptest.NewRecorder()
		writer := NewHistoryResponseWriter(recorder)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		if _, err := writer.Write([]byte(`{"value":42}`)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		compressed := recorder.Header().Get("Content-Type") == GzipHistoryContentType
		if compressed != (status == http.StatusOK) {
			t.Fatal("error response changed encoding")
		}
		data, err := ReadHistoryPayload(recorder.Body, compressed)
		if err != nil || string(data) != `{"value":42}` {
			t.Fatalf("response: %q %v", data, err)
		}
	}
	q, err := ParseQuery("gzip=1&range=6h")
	if err != nil || !q.Gzip || q.Encode() != "gzip=1&range=6h" {
		t.Fatalf("query: %+v %v", q, err)
	}
	for _, raw := range []string{"gzip=0", "gzip=other", "gzip=1&gzip=1", "Gzip=1"} {
		if _, err := ParseQuery(raw); err == nil {
			t.Fatalf("invalid negotiation accepted: %s", raw)
		}
	}
}
