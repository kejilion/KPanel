package tlsfallback

import (
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

var classicalCurves = []tls.CurveID{
	tls.X25519,
	tls.CurveP256,
	tls.CurveP384,
	tls.CurveP521,
}

// HostCache remembers endpoints that require a classical TLS key exchange.
// Its zero value is ready for use and may be shared by multiple transports.
type HostCache struct {
	classical sync.Map
}

// Transport tries Go's default TLS curve preferences first and retries an
// exact TLS handshake timeout with classical curves when the request body can
// be replayed safely.
type Transport struct {
	primary   http.RoundTripper
	classical http.RoundTripper
	cache     *HostCache
}

func New(primary *http.Transport, cache *HostCache) *Transport {
	if cache == nil {
		cache = &HostCache{}
	}
	classical := primary.Clone()
	if classical.TLSClientConfig == nil {
		classical.TLSClientConfig = &tls.Config{}
	} else {
		classical.TLSClientConfig = classical.TLSClientConfig.Clone()
	}
	classical.TLSClientConfig.CurvePreferences = append([]tls.CurveID(nil), classicalCurves...)
	return newTransport(primary, classical, cache)
}

func newTransport(primary, classical http.RoundTripper, cache *HostCache) *Transport {
	if cache == nil {
		cache = &HostCache{}
	}
	return &Transport{primary: primary, classical: classical, cache: cache}
}

func (transport *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	if _, ok := transport.cache.classical.Load(request.URL.Host); ok {
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
		transport.cache.classical.Store(request.URL.Host, struct{}{})
	}
	return response, retryErr
}

func (transport *Transport) CloseIdleConnections() {
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
