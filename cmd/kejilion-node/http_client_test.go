package main

import (
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

type timeoutError string

func (err timeoutError) Error() string { return string(err) }
func (timeoutError) Timeout() bool     { return true }

func TestTLSFallbackTransportRetriesReplayableRequestAndCachesHost(t *testing.T) {
	t.Parallel()

	primaryCalls := 0
	classicalCalls := 0
	transport := &tlsFallbackTransport{
		primary: roundTripFunc(func(*http.Request) (*http.Response, error) {
			primaryCalls++
			return nil, timeoutError("net/http: TLS handshake timeout")
		}),
		classical: roundTripFunc(func(request *http.Request) (*http.Response, error) {
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
		}),
	}

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
	if _, err := transport.RoundTrip(second); err != nil {
		t.Fatal(err)
	}
	if primaryCalls != 1 || classicalCalls != 2 {
		t.Fatalf("calls = primary %d, classical %d; want 1 and 2", primaryCalls, classicalCalls)
	}
}

func TestTLSFallbackTransportDoesNotRetryOtherTimeouts(t *testing.T) {
	t.Parallel()

	wantErr := timeoutError("context deadline exceeded")
	classicalCalls := 0
	transport := &tlsFallbackTransport{
		primary: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, wantErr }),
		classical: roundTripFunc(func(*http.Request) (*http.Response, error) {
			classicalCalls++
			return nil, nil
		}),
	}
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

func TestTLSFallbackTransportDoesNotRetryNonReplayableBody(t *testing.T) {
	t.Parallel()

	wantErr := timeoutError("net/http: TLS handshake timeout")
	classicalCalls := 0
	transport := &tlsFallbackTransport{
		primary: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, wantErr }),
		classical: roundTripFunc(func(*http.Request) (*http.Response, error) {
			classicalCalls++
			return nil, nil
		}),
	}
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

func TestNodeHTTPClientTLSPreferences(t *testing.T) {
	t.Parallel()

	transport, ok := newHTTPClient().Transport.(*tlsFallbackTransport)
	if !ok {
		t.Fatal("HTTP client is not using TLS fallback transport")
	}
	primary := transport.primary.(*http.Transport)
	classical := transport.classical.(*http.Transport)
	if primary.TLSClientConfig.MinVersion != tls.VersionTLS12 || classical.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatal("TLS minimum version is not TLS 1.2")
	}
	if len(primary.TLSClientConfig.CurvePreferences) != 0 {
		t.Fatal("primary transport should retain Go's default curve preferences")
	}
	if !reflect.DeepEqual(classical.TLSClientConfig.CurvePreferences, classicalTLSCurves) {
		t.Fatalf("classical curves = %v, want %v", classical.TLSClientConfig.CurvePreferences, classicalTLSCurves)
	}
	if primary.TLSHandshakeTimeout != nodeTLSHandshakeTimeout || classical.TLSHandshakeTimeout != nodeTLSHandshakeTimeout {
		t.Fatal("unexpected TLS handshake timeout")
	}
}
