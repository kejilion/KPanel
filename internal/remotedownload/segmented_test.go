package remotedownload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type segmentZeroReader struct{}

func (segmentZeroReader) Read(buffer []byte) (int, error) { clear(buffer); return len(buffer), nil }

func segmentTestResponse(request *http.Request, size int64) *http.Response {
	header := make(http.Header)
	header.Set("ETag", `"v1"`)
	response := &http.Response{StatusCode: 200, Header: header, ContentLength: size, Request: request}
	start, end := int64(0), size-1
	if value := request.Header.Get("Range"); value != "" {
		_, _ = fmt.Sscanf(value, "bytes=%d-%d", &start, &end)
		response.StatusCode = 206
		response.ContentLength = end - start + 1
		header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	}
	response.Body = io.NopCloser(io.LimitReader(segmentZeroReader{}, response.ContentLength))
	return response
}

func TestSegmentedReadsExactBytesWithBoundedRequests(t *testing.T) {
	client := NewClient(Config{MaxConnections: 4})
	var requests atomic.Int32
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		if r.Header.Get("Range") != "" && r.Header.Get("If-Range") != `"v1"` {
			t.Error("missing strong validator")
		}
		return segmentTestResponse(r, minSegmentedBytes+123), nil
	})
	response, err := client.OpenSegmented(context.Background(), "https://files.example.com/large", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	written, err := io.Copy(io.Discard, response.Body)
	if err != nil || written != minSegmentedBytes+123 || requests.Load() != 18 {
		t.Fatalf("bytes=%d err=%v requests=%d", written, err, requests.Load())
	}
}

func TestSegmentedFallsBackWithoutJoiningRepresentations(t *testing.T) {
	for _, kind := range []string{"weak", "unknown", "small", "ignored"} {
		t.Run(kind, func(t *testing.T) {
			client := NewClient(Config{})
			requests := 0
			client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				response := segmentTestResponse(r, minSegmentedBytes)
				switch kind {
				case "weak":
					response.Header.Set("ETag", `W/"v1"`)
				case "unknown":
					response.ContentLength = -1
				case "small":
					response.ContentLength = 1024
				case "ignored":
					response.StatusCode = 200
					response.Header.Del("Content-Range")
					response.ContentLength = minSegmentedBytes
					response.Body = io.NopCloser(io.LimitReader(segmentZeroReader{}, minSegmentedBytes))
				}
				return response, nil
			})
			response, err := client.OpenSegmented(context.Background(), "https://files.example.com/large", 4)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "ignored" {
				count, readErr := io.Copy(io.Discard, response.Body)
				if readErr != nil || count != minSegmentedBytes {
					t.Fatalf("incomplete fallback: %d %v", count, readErr)
				}
			}
			_ = response.Body.Close()
			want := 1
			if kind == "ignored" {
				want = 2
			}
			if requests != want || response.StatusCode != 200 {
				t.Fatalf("requests=%d status=%d", requests, response.StatusCode)
			}
		})
	}
}

func TestSegmentedRejectsChangedOrMalformedParts(t *testing.T) {
	for _, fault := range []string{"etag", "range", "length", "encoding", "full-after-probe", "truncated", "extra", "end-error"} {
		t.Run(fault, func(t *testing.T) {
			client := NewClient(Config{})
			client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				response := segmentTestResponse(r, minSegmentedBytes)
				if r.Header.Get("Range") == "" {
					return response, nil
				}
				switch fault {
				case "etag":
					response.Header.Set("ETag", `"v2"`)
				case "range":
					response.Header.Set("Content-Range", "bytes 1-2/3")
				case "length":
					response.ContentLength++
				case "encoding":
					response.Header.Set("Content-Encoding", "gzip")
				case "full-after-probe":
					if !strings.HasPrefix(r.Header.Get("Range"), "bytes=0-") {
						response.StatusCode = 200
						response.Header.Del("Content-Range")
					}
				case "truncated":
					response.Body = io.NopCloser(io.LimitReader(segmentZeroReader{}, 100))
				case "extra":
					response.ContentLength = -1
					response.Body = io.NopCloser(io.LimitReader(segmentZeroReader{}, segmentBytes+1))
				case "end-error":
					response.Body = io.NopCloser(&segmentEndError{remaining: segmentBytes})
				}
				return response, nil
			})
			response, err := client.OpenSegmented(context.Background(), "https://files.example.com/large", 4)
			if err == nil {
				_, err = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
			}
			if err == nil {
				t.Fatal("invalid representation accepted")
			}
		})
	}
}

type segmentEndError struct{ remaining int }

func (r *segmentEndError) Read(buffer []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}
	n := min(r.remaining, len(buffer))
	clear(buffer[:n])
	r.remaining -= n
	if r.remaining == 0 {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}

func TestSegmentedSlowFirstPartBoundsWindowAndCloseJoins(t *testing.T) {
	client := NewClient(Config{})
	first := &blockingBody{closed: make(chan struct{})}
	var ranges atomic.Int32
	ready := make(chan struct{}, 4)
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		response := segmentTestResponse(r, minSegmentedBytes)
		if value := r.Header.Get("Range"); value != "" {
			ranges.Add(1)
			if strings.HasPrefix(value, "bytes=0-") {
				response.Body = first
			} else {
				ready <- struct{}{}
			}
		}
		return response, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := client.OpenSegmented(ctx, "https://files.example.com/large", 4)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	readDone := make(chan error, 1)
	go func() { _, err := io.Copy(io.Discard, response.Body); readDone <- err }()
	for i := 0; i < 3; i++ {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if ranges.Load() != 4 {
		t.Fatalf("unbounded read-ahead: %d requests", ranges.Load())
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("read error=%v", err)
		}
	case <-ctx.Done():
		t.Fatal("close did not join")
	}
	select {
	case <-response.Body.(*segmentedBody).done:
	default:
		t.Fatal("workers retained after close")
	}
}

func TestSegmentedRetainsAddressPolicy(t *testing.T) {
	client := NewClient(Config{})
	if _, err := client.OpenSegmented(context.Background(), "http://127.0.0.1/private", 4); !errors.Is(err, ErrAddressBlocked) {
		t.Fatalf("address error=%v", err)
	}
}
