package main

import (
	"net/http"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/tlsfallback"
)

func TestNodeHTTPClientUsesTLSFallback(t *testing.T) {
	t.Parallel()

	client := newHTTPClient()
	if _, ok := client.Transport.(*tlsfallback.Transport); !ok {
		t.Fatal("HTTP client is not using TLS fallback transport")
	}
	if client.Timeout <= nodeTLSHandshakeTimeout {
		t.Fatal("HTTP client timeout cannot cover fallback handshake")
	}
	if client.CheckRedirect == nil {
		t.Fatal("redirect policy missing")
	}
	request, err := http.NewRequest(http.MethodGet, "https://panel.example", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(request, nil); err != http.ErrUseLastResponse {
		t.Fatalf("redirect error = %v, want %v", err, http.ErrUseLastResponse)
	}
}
