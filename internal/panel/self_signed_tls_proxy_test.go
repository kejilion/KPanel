package panel

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSelfSignedTLSColdReverseProxyBurst(t *testing.T) {
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener := newSelfSignedTLSListener(raw, func() *tls.Config { return nil })
	var reached atomic.Int64
	backend := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		w.Header().Set("Content-Type", "text/javascript")
		io.WriteString(w, "export const loaded = true;")
	})}
	go backend.Serve(listener)
	defer backend.Close()
	target, _ := url.Parse("http://" + raw.Addr().String())
	proxy := httputil.NewSingleHostReverseProxy(target)
	transport := &http.Transport{DisableKeepAlives: true, ResponseHeaderTimeout: 2 * time.Second}
	proxy.Transport = transport
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	var upstreamErrors atomic.Int64
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		upstreamErrors.Add(1)
		http.Error(w, "Bad Gateway", 502)
	}
	frontend := httptest.NewServer(proxy)
	defer frontend.Close()
	client := &http.Client{Transport: &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 64}, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	var ok, badGateway, other atomic.Int64
	for round := 0; round < 20; round++ {
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 40; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				r, err := client.Get(frontend.URL + "/assets/probe.js")
				if err != nil {
					other.Add(1)
					return
				}
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
				switch r.StatusCode {
				case 200:
					ok.Add(1)
				case 502:
					badGateway.Add(1)
				default:
					other.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()
		time.Sleep(5 * time.Millisecond)
	}
	if badGateway.Load() != 0 || other.Load() != 0 || ok.Load() != 800 {
		t.Fatalf("cold resource burst: ok=%d bad_gateway=%d other=%d", ok.Load(), badGateway.Load(), other.Load())
	}
	t.Logf("reverse_proxy requests=800 status_200=%d status_502=%d other=%d upstream_errors=%d handler_reached=%d", ok.Load(), badGateway.Load(), other.Load(), upstreamErrors.Load(), reached.Load())
}
