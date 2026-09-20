package tlsfallback

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type timeoutError string

func (err timeoutError) Error() string { return string(err) }
func (timeoutError) Timeout() bool     { return true }

func TestTransportRetriesReplayableRequestAndSharesCache(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	classicalCalls := 0
	cache := &HostCache{}
	classical := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		classicalCalls++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "payload" {
			t.Fatalf("body = %q, want payload", body)
		}
		if request.Header.Get("X-Test") != "preserved" {
			t.Fatal("request header was not preserved")
		}
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}, nil
	})
	primary := roundTripFunc(func(*http.Request) (*http.Response, error) {
		primaryCalls++
		return nil, timeoutError("net/http: TLS handshake timeout")
	})
	transport := newTransport(primary, classical, cache)

	request, err := http.NewRequest(http.MethodPost, "https://panel.example/report", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Test", "preserved")
	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatal(err)
	}

	second, err := http.NewRequest(http.MethodPost, "https://panel.example/report", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	second.Header.Set("X-Test", "preserved")
	if _, err := newTransport(primary, classical, cache).RoundTrip(second); err != nil {
		t.Fatal(err)
	}
	if primaryCalls != 1 || classicalCalls != 2 {
		t.Fatalf("calls = primary %d, classical %d; want 1 and 2", primaryCalls, classicalCalls)
	}
}

func TestTransportDoesNotRetryOtherTimeouts(t *testing.T) {
	t.Parallel()

	wantErr := timeoutError("context deadline exceeded")
	classicalCalls := 0
	transport := newTransport(
		roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, wantErr }),
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			classicalCalls++
			return nil, nil
		}),
		nil,
	)
	request, err := http.NewRequest(http.MethodGet, "https://panel.example/report", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(request); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if classicalCalls != 0 {
		t.Fatalf("classical calls = %d, want 0", classicalCalls)
	}
}

func TestTransportDoesNotRetryNonReplayableBody(t *testing.T) {
	t.Parallel()

	wantErr := timeoutError("net/http: TLS handshake timeout")
	classicalCalls := 0
	transport := newTransport(
		roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, wantErr }),
		roundTripFunc(func(*http.Request) (*http.Response, error) {
			classicalCalls++
			return nil, nil
		}),
		nil,
	)
	request, err := http.NewRequest(http.MethodPost, "https://panel.example/report", io.NopCloser(strings.NewReader("payload")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.RoundTrip(request); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if classicalCalls != 0 {
		t.Fatalf("classical calls = %d, want 0", classicalCalls)
	}
}

func TestNewClonesTransportAndPreservesTLSSettings(t *testing.T) {
	t.Parallel()

	roots := x509.NewCertPool()
	primary := &http.Transport{
		TLSHandshakeTimeout:   7 * time.Second,
		ResponseHeaderTimeout: 11 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
		},
	}
	transport := New(primary, nil)
	classical := transport.classical.(*http.Transport)
	if transport.primary != primary || classical == primary {
		t.Fatal("primary transport was replaced or reused as the fallback")
	}
	if primary.TLSClientConfig == classical.TLSClientConfig {
		t.Fatal("fallback shares mutable TLS configuration with primary")
	}
	if len(primary.TLSClientConfig.CurvePreferences) != 0 {
		t.Fatal("primary transport should retain Go's default curve preferences")
	}
	if !reflect.DeepEqual(classical.TLSClientConfig.CurvePreferences, classicalCurves) {
		t.Fatalf("classical curves = %v, want %v", classical.TLSClientConfig.CurvePreferences, classicalCurves)
	}
	if classical.TLSClientConfig.MinVersion != tls.VersionTLS12 || classical.TLSClientConfig.RootCAs != roots {
		t.Fatal("fallback did not preserve TLS security settings")
	}
	if classical.TLSHandshakeTimeout != 7*time.Second || classical.ResponseHeaderTimeout != 11*time.Second {
		t.Fatal("fallback did not preserve transport timeouts")
	}
}
