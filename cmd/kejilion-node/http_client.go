package main

import (
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const nodeTLSHandshakeTimeout = 5 * time.Second

var classicalTLSCurves = []tls.CurveID{
	tls.X25519,
	tls.CurveP256,
	tls.CurveP384,
	tls.CurveP521,
}

type tlsFallbackTransport struct {
	primary      http.RoundTripper
	classical    http.RoundTripper
	classicalFor sync.Map
}

func newHTTPClient() *http.Client {
	transport := &tlsFallbackTransport{
		primary:   newNodeTransport(nil),
		classical: newNodeTransport(classicalTLSCurves),
	}
	return &http.Client{
		Transport: transport, Timeout: 40 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func newNodeTransport(curves []tls.CurveID) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if len(curves) > 0 {
		transport.TLSClientConfig.CurvePreferences = append([]tls.CurveID(nil), curves...)
	}
	transport.TLSHandshakeTimeout = nodeTLSHandshakeTimeout
	transport.MaxIdleConns = 4
	transport.MaxIdleConnsPerHost = 2
	return transport
}

func (transport *tlsFallbackTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if _, ok := transport.classicalFor.Load(request.URL.Host); ok {
		return transport.classical.RoundTrip(request)
	}

	response, err := transport.primary.RoundTrip(request)
	if err == nil || response != nil || !isTLSHandshakeTimeout(err) || !requestReplayable(request) {
		return response, err
	}

	retry := request.Clone(request.Context())
	if request.Body != nil && request.Body != http.NoBody {
		retryBody, bodyErr := request.GetBody()
		if bodyErr != nil {
			return response, err
		}
		retry.Body = retryBody
	}
	slog.Warn("TLS handshake timed out; retrying with classical key exchange", "host", request.URL.Host)
	response, retryErr := transport.classical.RoundTrip(retry)
	if retryErr == nil {
		transport.classicalFor.Store(request.URL.Host, struct{}{})
	}
	return response, retryErr
}

func (transport *tlsFallbackTransport) CloseIdleConnections() {
	for _, roundTripper := range []http.RoundTripper{transport.primary, transport.classical} {
		if closer, ok := roundTripper.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	}
}

func requestReplayable(request *http.Request) bool {
	return request.Body == nil || request.Body == http.NoBody || request.GetBody != nil
}

func isTLSHandshakeTimeout(err error) bool {
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout() && strings.Contains(err.Error(), "TLS handshake timeout")
}
