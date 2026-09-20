package main

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/tlsfallback"
)

const nodeTLSHandshakeTimeout = 5 * time.Second

func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.TLSHandshakeTimeout = nodeTLSHandshakeTimeout
	transport.MaxIdleConns = 4
	transport.MaxIdleConnsPerHost = 2
	return &http.Client{
		Transport: tlsfallback.New(transport, nil), Timeout: 40 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
