package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/desktopworkspace"
)

func desktopIconPreviewLoopbackLookup(context.Context, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
}

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		address string
		public  bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2001:4860:4860::8888", true},
		{"0.0.0.0", false},
		{"10.0.0.1", false},
		{"10.255.255.255", false},
		{"100.64.0.1", false},
		{"127.0.0.1", false},
		{"127.53.1.2", false},
		{"169.254.169.254", false},
		{"172.16.0.1", false},
		{"172.31.255.255", false},
		{"192.0.0.1", false},
		{"192.0.2.1", false},
		{"192.168.1.1", false},
		{"198.18.0.1", false},
		{"198.51.100.1", false},
		{"203.0.113.1", false},
		{"224.0.0.1", false},
		{"240.0.0.1", false},
		{"255.255.255.255", false},
		{"::1", false},
		{"::", false},
		{"::ffff:10.0.0.1", false},
		{"::ffff:8.8.8.8", true},
		{"64:ff9b::8.8.8.8", false},
		{"100::1", false},
		{"2001:db8::1", false},
		{"2002::1", false},
		{"fc00::1", false},
		{"fd00::1", false},
		{"fe80::1", false},
		{"ff02::1", false},
	}
	for _, tc := range cases {
		addr := netip.MustParseAddr(tc.address)
		if got := isPublicIP(addr); got != tc.public {
			t.Errorf("isPublicIP(%s) = %v, want %v", tc.address, got, tc.public)
		}
	}
}

func TestLookupPublicIPsRejectsPrivateLiterals(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "10.0.0.1", "192.168.1.1", "169.254.169.254", "::1", "fc00::1"} {
		if _, err := lookupPublicIPs(context.Background(), host); err == nil {
			t.Errorf("lookupPublicIPs(%s) accepted a private host", host)
		}
	}
	if _, err := lookupPublicIPs(context.Background(), "8.8.8.8"); err != nil {
		t.Errorf("lookupPublicIPs(8.8.8.8) = %v", err)
	}
}

func TestParseDesktopIconPreviewURL(t *testing.T) {
	valid := []string{
		"https://example.com/",
		"http://example.com:8080/path?q=1",
		"https://example.com/icon.png#section",
	}
	for _, raw := range valid {
		parsed, ok := parseDesktopIconPreviewURL(raw)
		if !ok || parsed == nil {
			t.Errorf("parseDesktopIconPreviewURL(%q) rejected a valid URL", raw)
		}
	}
	if parsed, _ := parseDesktopIconPreviewURL("https://example.com/icon.png#section"); parsed != nil && parsed.Fragment != "" {
		t.Errorf("fragment was not stripped: %q", parsed.Fragment)
	}
	invalid := []string{
		"",
		"javascript:alert(1)",
		"ftp://example.com/",
		"//example.com/",
		"/relative/path",
		"https://user:pass@example.com/",
		"https://:pass@example.com/",
		"https://example.com:0/",
		"https://example.com:99999/",
		"https://example.com:abc/",
		"https:///path",
		" https://example.com/",
		"https://example.com/ ",
	}
	for _, raw := range invalid {
		if _, ok := parseDesktopIconPreviewURL(raw); ok {
			t.Errorf("parseDesktopIconPreviewURL(%q) accepted an invalid URL", raw)
		}
	}
	if _, ok := parseDesktopIconPreviewURL("https://example.com/" + string(make([]byte, 2048))); ok {
		t.Errorf("overlong URL was accepted")
	}
}

func desktopIconPreviewTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func desktopIconPreviewTestJPEG(t *testing.T) []byte {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			value.SetNRGBA(x, y, color.NRGBA{R: 32, G: 160, B: 208, A: 255})
		}
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, value, nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func desktopIconPreviewTestGIF(t *testing.T) []byte {
	t.Helper()
	value := image.NewPaletted(image.Rect(0, 0, 16, 16), color.Palette{color.NRGBA{A: 255}, color.NRGBA{R: 255, A: 255}})
	var output bytes.Buffer
	if err := gif.Encode(&output, value, nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestFetchDesktopIconPreviewLinkedIcon(t *testing.T) {
	png := panelTestPNG(t)
	server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><link rel="icon" href="/icon.png">`))
		case "/icon.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	})
	target, _ := url.Parse(server.URL + "/")
	icon, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target)
	if err != nil {
		t.Fatal(err)
	}
	if icon.contentType != "image/png" || !bytes.Equal(icon.data, png) {
		t.Fatalf("unexpected icon: %q %d bytes", icon.contentType, len(icon.data))
	}
}

func TestFetchDesktopIconPreviewFallsBackToFaviconICO(t *testing.T) {
	png := panelTestPNG(t)
	server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><title>No links here</title>`))
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	})
	target, _ := url.Parse(server.URL + "/")
	icon, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target)
	if err != nil {
		t.Fatal(err)
	}
	if icon.contentType != "image/png" || !bytes.Equal(icon.data, png) {
		t.Fatalf("unexpected icon: %q %d bytes", icon.contentType, len(icon.data))
	}
}

func TestFetchDesktopIconPreviewLargePageHead(t *testing.T) {
	png := panelTestPNG(t)
	server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/icon.png" {
			_, _ = w.Write(png)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(append([]byte(`<html><head><link rel="icon" href="/icon.png"></head>`), bytes.Repeat([]byte("x"), desktopIconPreviewMaxPageBytes)...))
	})
	target, _ := url.Parse(server.URL)
	icon, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target)
	if err != nil || !bytes.Equal(icon.data, png) {
		t.Fatalf("large page icon: %v", err)
	}
}

func TestFetchDesktopIconPreviewRejectsOversizedDirectImage(t *testing.T) {
	body := append(panelTestPNG(t), bytes.Repeat([]byte("x"), desktopworkspace.MaxIconBytes)...)
	server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/large.png" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	})
	target, _ := url.Parse(server.URL + "/large.png")
	if _, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target); err == nil {
		t.Fatal("oversized direct image accepted after truncation")
	}
}

func TestFetchDesktopIconPreviewAcceptsDirectImageURL(t *testing.T) {
	jpeg := desktopIconPreviewTestJPEG(t)
	server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpeg)
	})
	target, _ := url.Parse(server.URL + "/logo.jpg")
	icon, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target)
	if err != nil {
		t.Fatal(err)
	}
	if icon.contentType != "image/jpeg" || !bytes.Equal(icon.data, jpeg) {
		t.Fatalf("unexpected icon: %q %d bytes", icon.contentType, len(icon.data))
	}
}

func TestFetchDesktopIconPreviewSkipsGIFAndUsesFallback(t *testing.T) {
	png := panelTestPNG(t)
	gif := desktopIconPreviewTestGIF(t)
	server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><link rel="icon" href="/icon.gif">`))
		case "/icon.gif":
			w.Header().Set("Content-Type", "image/gif")
			_, _ = w.Write(gif)
		case "/favicon.ico":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	})
	target, _ := url.Parse(server.URL + "/")
	icon, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target)
	if err != nil {
		t.Fatal(err)
	}
	if icon.contentType != "image/png" || !bytes.Equal(icon.data, png) {
		t.Fatalf("GIF was not skipped: %q %d bytes", icon.contentType, len(icon.data))
	}
}

func TestFetchDesktopIconPreviewRejectsPrivateTarget(t *testing.T) {
	server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(panelTestPNG(t))
	})
	target, _ := url.Parse(server.URL + "/")
	_, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(lookupPublicIPs), target)
	if err == nil {
		t.Fatal("loopback target was fetched with the production lookup")
	}
}

func TestFetchDesktopIconPreviewRedirectLimit(t *testing.T) {
	png := panelTestPNG(t)
	t.Run("six redirects are rejected", func(t *testing.T) {
		server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			step := redirectStep(r.URL.Path)
			if step < 0 {
				http.NotFound(w, r)
				return
			}
			if step >= 6 {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(png)
				return
			}
			http.Redirect(w, r, "/hop/"+itoa(step+1), http.StatusFound)
		})
		target, _ := url.Parse(server.URL + "/hop/0")
		if _, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target); err == nil {
			t.Fatal("six redirects were followed")
		}
	})
	t.Run("five redirects are followed", func(t *testing.T) {
		server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
			step := redirectStep(r.URL.Path)
			if step < 0 {
				http.NotFound(w, r)
				return
			}
			if step >= 5 {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(png)
				return
			}
			http.Redirect(w, r, "/hop/"+itoa(step+1), http.StatusFound)
		})
		target, _ := url.Parse(server.URL + "/hop/0")
		icon, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(icon.data, png) {
			t.Fatalf("unexpected icon after five redirects: %d bytes", len(icon.data))
		}
	})
}

func redirectStep(path string) int {
	if len(path) < 5 || path[:5] != "/hop/" {
		return -1
	}
	step := 0
	for _, c := range path[5:] {
		if c < '0' || c > '9' {
			return -1
		}
		step = step*10 + int(c-'0')
	}
	return step
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 4)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

func TestFetchDesktopIconPreviewOversizedIconIsSkipped(t *testing.T) {
	server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><link rel="icon" href="/big.png">`))
		case "/big.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(bytes.Repeat([]byte("x"), desktopworkspace.MaxIconBytes+1))
		default:
			http.NotFound(w, r)
		}
	})
	target, _ := url.Parse(server.URL + "/")
	if _, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target); err == nil {
		t.Fatal("oversized icon was accepted")
	}
}

func TestFetchDesktopIconPreviewTimeoutIsSilent(t *testing.T) {
	server := desktopIconPreviewTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(5 * time.Second)
		_, _ = w.Write(panelTestPNG(t))
	})
	target, _ := url.Parse(server.URL + "/")
	start := time.Now()
	if _, err := fetchDesktopIconPreview(context.Background(), newDesktopIconPreviewClient(desktopIconPreviewLoopbackLookup), target); err == nil {
		t.Fatal("timed-out fetch was accepted")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("fetch took too long: %v", elapsed)
	}
}

func TestDesktopIconPreviewAPIValidation(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)

	unauthenticated := performRequest(server, http.MethodPost, desktopIconPreviewPath, []byte(`{"url":"https://example.com/"}`), nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated preview status = %d", unauthenticated.Code)
	}
	post := func(raw string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"url": raw})
		return authenticatedRequest(server, http.MethodPost, desktopIconPreviewPath, body, sessionCookie, csrfCookie,
			map[string]string{"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value, "Content-Type": "application/json"})
	}
	if got := post(""); got.Code != http.StatusBadRequest {
		t.Fatalf("missing url status = %d", got.Code)
	}
	if got := post("javascript:alert(1)"); got.Code != http.StatusBadRequest {
		t.Fatalf("javascript url status = %d", got.Code)
	}
	if got := post("https://user:pass@example.com/"); got.Code != http.StatusBadRequest {
		t.Fatalf("credential url status = %d", got.Code)
	}
	// A loopback target must never be dialled: the lookup rejects it before
	// any connection is attempted, so the endpoint reports "not found".
	if got := post("http://127.0.0.1:9/"); got.Code != http.StatusNotFound {
		t.Fatalf("loopback url status = %d %s", got.Code, got.Body.String())
	}
	if got := authenticatedRequest(server, http.MethodGet, desktopIconPreviewPath, nil, sessionCookie, csrfCookie, nil); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET preview status = %d", got.Code)
	}
}

func TestDesktopIconPreviewRejectsCrossSiteAndMissingCSRF(t *testing.T) {
	server, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, server, tokenPath)
	for _, headers := range []map[string]string{
		{"Origin": "https://attacker.test", "X-CSRF-Token": csrf.Value},
		{"Origin": "http://panel.test"},
		{"Origin": "http://panel.test", "X-CSRF-Token": "wrong"},
	} {
		got := authenticatedRequest(server, http.MethodPost, desktopIconPreviewPath, []byte(`{"url":"https://example.com/"}`), session, csrf, headers)
		if got.Code != http.StatusForbidden {
			t.Fatalf("status = %d %s", got.Code, got.Body.String())
		}
	}
	if server.iconPreviewBudget.requests != 0 {
		t.Fatal("rejected requests consumed outbound budget")
	}
}

func TestDesktopIconPreviewBudget(t *testing.T) {
	var budget desktopIconPreviewBudget
	now := time.Now()
	for i := 0; i < 4; i++ {
		if !budget.acquire(now) {
			t.Fatal("initial slot rejected")
		}
	}
	if budget.acquire(now) {
		t.Fatal("concurrency cap exceeded")
	}
	for i := 0; i < 4; i++ {
		budget.release()
	}
	for i := 4; i < 60; i++ {
		if !budget.acquire(now) {
			t.Fatal("budget rejected early")
		}
		budget.release()
	}
	if budget.acquire(now) {
		t.Fatal("rate cap exceeded")
	}
	if !budget.acquire(now.Add(time.Minute)) {
		t.Fatal("budget did not reset")
	}
	budget.release()
}

func TestDesktopIconPreviewRateLimitBeforeNetwork(t *testing.T) {
	server, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, server, tokenPath)
	server.iconPreviewBudget.window = time.Now()
	server.iconPreviewBudget.requests = 60
	got := authenticatedRequest(server, http.MethodPost, desktopIconPreviewPath, []byte(`{"url":"https://example.com/"}`), session, csrf,
		map[string]string{"Origin": "http://panel.test", "X-CSRF-Token": csrf.Value, "Content-Type": "application/json"})
	if got.Code != http.StatusTooManyRequests || got.Header().Get("Retry-After") == "" {
		t.Fatalf("rate status = %d", got.Code)
	}
}
