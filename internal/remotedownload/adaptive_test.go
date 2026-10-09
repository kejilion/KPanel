package remotedownload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdaptivePolicyUsesEndToEndBenefit(t *testing.T) {
	for _, tc := range []struct {
		name        string
		read, write time.Duration
		want        string
	}{
		{"source-limited", 2 * time.Second, 100 * time.Millisecond, "probing"},
		{"balanced", time.Second, time.Second, "single"},
		{"fast", 100 * time.Millisecond, 50 * time.Millisecond, "single"},
		{"target-limited", 100 * time.Millisecond, time.Second, "single"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := AdaptivePolicy{}
			p.Observe(16<<20, 48<<20, tc.read, tc.write)
			if p.Mode != tc.want {
				t.Fatalf("mode=%s", p.Mode)
			}
		})
	}
	p := AdaptivePolicy{}
	p.Observe(16<<20, 64<<20, 2*time.Second, 100*time.Millisecond)
	p.Observe(16<<20, 48<<20, time.Second, 100*time.Millisecond)
	if p.Mode != "parallel" {
		t.Fatal(p.Mode)
	}
	p.Observe(16<<20, 32<<20, time.Second, 3*time.Second)
	if p.Mode != "single" {
		t.Fatal(p.Mode)
	}
	p.Observe(16<<20, 64<<20, 2*time.Second, time.Millisecond)
	if p.Mode != "single" {
		t.Fatal("repeated probe")
	}
}

func TestAdaptiveBodyKeepsExactCheckpointAcrossSwitches(t *testing.T) {
	for _, kind := range []string{"range", "ignored", "changed", "no-benefit"} {
		t.Run(kind, func(t *testing.T) {
			client := NewClient(Config{})
			var requests atomic.Int32
			client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests.Add(1)
				response := segmentTestResponse(r, 80<<20)
				response.Header.Set("Accept-Ranges", "bytes")
				if r.Header.Get("Range") != "" && kind == "ignored" {
					response.StatusCode = 200
					response.ContentLength = 80 << 20
					response.Header.Del("Content-Range")
					response.Body = io.NopCloser(io.LimitReader(segmentZeroReader{}, 80<<20))
				}
				if r.Header.Get("Range") != "" && kind == "changed" {
					response.Header.Set("ETag", `"v2"`)
				}
				return response, nil
			})
			response, err := client.Open(context.Background(), "https://example.com/file")
			if err != nil {
				t.Fatal(err)
			}
			budget := make(chan struct{}, 1)
			body := client.Adaptive(context.Background(), "https://example.com/file", response, budget).(*AdaptiveBody)
			defer body.Close()
			hash := sha256.New()
			written, err := io.CopyN(hash, body, 16<<20)
			if err != nil {
				t.Fatal(err)
			}
			err = body.Checkpoint(written, hex.EncodeToString(hash.Sum(nil)), 2*time.Second, 100*time.Millisecond)
			if kind == "changed" {
				if !errors.Is(err, ErrSourceChanged) {
					t.Fatal(err)
				}
				if len(budget) != 0 {
					t.Fatal("budget retained")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if kind == "ignored" && body.TransferMode() != "single" {
				t.Fatal("ignored range did not fall back")
			}
			if kind == "no-benefit" {
				n, err := io.CopyN(hash, body, 16<<20)
				written += n
				if err != nil {
					t.Fatal(err)
				}
				if err = body.Checkpoint(written, hex.EncodeToString(hash.Sum(nil)), 2*time.Second, time.Second); err != nil {
					t.Fatal(err)
				}
				if body.TransferMode() != "single" || len(budget) != 0 {
					t.Fatal("no-benefit trial retained")
				}
			}
			n, err := io.Copy(hash, body)
			written += n
			if err != nil || written != 80<<20 {
				t.Fatalf("bytes=%d err=%v", written, err)
			}
			want := sha256.New()
			_, _ = io.CopyN(want, segmentZeroReader{}, 80<<20)
			if hex.EncodeToString(hash.Sum(nil)) != hex.EncodeToString(want.Sum(nil)) {
				t.Fatal("joined incorrect bytes")
			}
			body.Close()
			if len(budget) != 0 {
				t.Fatal("budget leaked")
			}
		})
	}
}

func TestAdaptiveAdmissionAndConcurrentClose(t *testing.T) {
	client := NewClient(Config{})
	entered := make(chan struct{})
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Range") != "" {
			close(entered)
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		response := segmentTestResponse(r, 64<<20)
		response.Header.Set("Accept-Ranges", "bytes")
		return response, nil
	})
	response, _ := client.Open(context.Background(), "https://example.com/file")
	budget := make(chan struct{}, 1)
	body := client.Adaptive(context.Background(), "https://example.com/file", response, budget).(*AdaptiveBody)
	done := make(chan error, 1)
	go func() { done <- body.Checkpoint(16<<20, "", 2*time.Second, time.Millisecond) }()
	<-entered
	body.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("closed probe succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not interrupt probe")
	}
	if len(budget) != 0 {
		t.Fatal("closed probe leaked budget")
	}
	response, _ = client.Open(context.Background(), "https://example.com/file")
	response.Header.Del("Accept-Ranges")
	if client.Adaptive(context.Background(), "https://example.com/file", response, budget) != response.Body {
		t.Fatal("probed unadvertised ranges")
	}
	response.Body.Close()
}
