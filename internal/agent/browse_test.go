package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv4 RFC1918 10/8", "10.1.2.3", true},
		{"IPv4 RFC1918 172.16/12", "172.16.5.6", true},
		{"IPv4 RFC1918 192.168/16", "192.168.1.1", true},
		{"IPv4 link-local", "169.254.1.1", true},
		{"IPv4 cloud metadata", "169.254.169.254", true},
		{"IPv4 unspecified", "0.0.0.0", true},
		{"IPv4 multicast", "224.0.0.1", true},
		{"IPv4 public", "8.8.8.8", false},
		{"IPv4 public 2", "1.1.1.1", false},
		{"IPv6 loopback", "::1", true},
		{"IPv6 unique-local", "fc00::1", true},
		{"IPv6 link-local", "fe80::1", true},
		{"IPv6 unspecified", "::", true},
		{"IPv6 public", "2001:4860:4860::8888", false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ip := net.ParseIP(testCase.ip)
			if ip == nil {
				t.Fatalf("invalid test IP %q", testCase.ip)
			}
			if got := isBlockedIP(ip); got != testCase.blocked {
				t.Errorf("isBlockedIP(%s) = %v, want %v", testCase.ip, got, testCase.blocked)
			}
		})
	}
}

func TestBrowseDialerBlocksWhenAllResolvedAddressesBlocked(t *testing.T) {
	dialer := &browseDialer{
		resolve: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1")}, nil
		},
		blocked: isBlockedIP,
	}
	_, err := dialer.DialContext(context.Background(), "tcp", "internal.example:80")
	if !errors.Is(err, errBrowseTargetBlocked) {
		t.Fatalf("DialContext() error = %v, want errBrowseTargetBlocked", err)
	}
}

func TestBrowseDialerSkipsBlockedResolvedAddressAndDialsTheRest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer upstream.Close()
	upstreamAddr, err := net.ResolveTCPAddr("tcp", upstream.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	dialer := &browseDialer{
		// A blocked address ordered first proves the dialer keeps trying the
		// remaining candidates instead of failing on the first hit.
		resolve: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.2"), upstreamAddr.IP}, nil
		},
		blocked: func(ip net.IP) bool { return ip.Equal(net.ParseIP("127.0.0.2")) },
		dial:    net.Dialer{},
	}
	client := &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}
	resp, err := client.Get("http://fake-browse-target.invalid:" + portOf(t, upstream) + "/")
	if err != nil {
		t.Fatalf("client.Get() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func portOf(t *testing.T, server *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestValidateBrowseFetchInput(t *testing.T) {
	validBody := base64.StdEncoding.EncodeToString([]byte("hello"))
	cases := []struct {
		name    string
		input   browseFetchInput
		wantErr bool
	}{
		{"valid minimal", browseFetchInput{URL: "https://example.com/path"}, false},
		{"valid with method and body", browseFetchInput{URL: "https://example.com/", Method: "post", Body: validBody}, false},
		{"empty url", browseFetchInput{URL: ""}, true},
		{"oversized url", browseFetchInput{URL: "https://example.com/" + strings.Repeat("a", maxBrowseURLBytes)}, true},
		{"relative url", browseFetchInput{URL: "/relative/path"}, true},
		{"non-http scheme", browseFetchInput{URL: "ftp://example.com/"}, true},
		{"userinfo", browseFetchInput{URL: "https://user:pass@example.com/"}, true},
		{"unsupported method", browseFetchInput{URL: "https://example.com/", Method: "CONNECT"}, true},
		{"too many headers", browseFetchInput{URL: "https://example.com/", Headers: manyHeaders(maxBrowseHeaderCount + 1)}, true},
		{"oversized header value", browseFetchInput{URL: "https://example.com/", Headers: map[string][]string{"X-Test": {strings.Repeat("a", maxBrowseHeaderBytes+1)}}}, true},
		{"invalid body encoding", browseFetchInput{URL: "https://example.com/", Body: "not-base64!!"}, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, _, _, err := validateBrowseFetchInput(testCase.input)
			if (err != nil) != testCase.wantErr {
				t.Fatalf("validateBrowseFetchInput(%+v) error = %v, wantErr %v", testCase.input, err, testCase.wantErr)
			}
		})
	}
}

func manyHeaders(count int) map[string][]string {
	headers := make(map[string][]string, count)
	for i := 0; i < count; i++ {
		headers["X-Header-"+strconv.Itoa(i)] = []string{"v"}
	}
	return headers
}

func TestBrowseFetchHandlerRejectsUnauthenticated(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/browse/fetch", bytes.NewReader([]byte(`{"url":"https://example.com/"}`)))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestBrowseFetchHandlerRejectsWrongMethod(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/browse/fetch", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestBrowseFetchHandlerRejectsInvalidJSON(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/browse/fetch", bytes.NewReader([]byte(`not json`)))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestBrowseFetchHandlerBlocksLoopbackTarget(t *testing.T) {
	server := testServer(t)
	body, _ := json.Marshal(browseFetchInput{URL: "http://127.0.0.1:1/"})
	request := httptest.NewRequest(http.MethodPost, "/v1/browse/fetch", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var problem struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "browse_target_blocked" {
		t.Fatalf("code = %q", problem.Code)
	}
}

// TestBrowseFetchHandlerRelaysRequestAndResponse swaps the server's client
// for one whose dialer resolves a fake hostname to the local httptest
// upstream, so the handler's request/response plumbing (method, headers,
// body, Host derivation, hop-by-hop stripping) is exercised end to end
// without depending on real internet access or weakening the production
// isBlockedIP guard exercised separately above.
func TestBrowseFetchHandlerRelaysRequestAndResponse(t *testing.T) {
	var gotHost, gotConnectionHeader, gotXForwarded string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotConnectionHeader = r.Header.Get("Connection")
		gotXForwarded = r.Header.Get("X-Forwarded")
		w.Header().Set("X-Upstream", "1")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("relayed"))
	}))
	defer upstream.Close()
	upstreamAddr, err := net.ResolveTCPAddr("tcp", upstream.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	server := testServer(t)
	dialer := &browseDialer{
		resolve: func(context.Context, string) ([]net.IP, error) { return []net.IP{upstreamAddr.IP}, nil },
		blocked: func(net.IP) bool { return false },
	}
	server.browseClient = &http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}

	target := "http://browse-fetch-test.invalid:" + portOf(t, upstream) + "/path"
	input := browseFetchInput{
		URL:    target,
		Method: "post",
		Headers: map[string][]string{
			"X-Forwarded": {"client-value"},
			"Connection":  {"close"},            // hop-by-hop, must be stripped
			"Host":        {"evil.example.com"}, // must never override the derived Host
		},
		Body: base64.StdEncoding.EncodeToString([]byte("payload")),
	}
	body, _ := json.Marshal(input)
	request := httptest.NewRequest(http.MethodPost, "/v1/browse/fetch", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var output browseFetchOutput
	if err := json.Unmarshal(response.Body.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if output.StatusCode != http.StatusCreated {
		t.Fatalf("relayed status = %d", output.StatusCode)
	}
	decodedBody, err := base64.StdEncoding.DecodeString(output.Body)
	if err != nil || string(decodedBody) != "relayed" {
		t.Fatalf("relayed body = %q (err=%v)", output.Body, err)
	}
	if gotXForwarded != "client-value" {
		t.Fatalf("upstream did not receive forwarded header, got %q", gotXForwarded)
	}
	if gotConnectionHeader != "" {
		t.Fatalf("Connection header was not stripped, got %q", gotConnectionHeader)
	}
	if gotHost != "browse-fetch-test.invalid:"+portOf(t, upstream) {
		t.Fatalf("Host was not derived from the target URL, got %q", gotHost)
	}
}

func TestIsBlockedIPAllowingPrivate(t *testing.T) {
	// The relaxed guard is what store.AllowedHosts.BrowseAllowPrivateNetworks
	// switches on. It must unblock the LAN and nothing else — link-local in
	// particular stays blocked, because 169.254.169.254 is the cloud metadata
	// service, not a machine on the operator's network.
	cases := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{"IPv4 loopback", "127.0.0.1", false},
		{"IPv4 loopback range", "127.5.6.7", false},
		{"IPv4 RFC1918 10/8", "10.1.2.3", false},
		{"IPv4 RFC1918 172.16/12", "172.16.5.6", false},
		{"IPv4 RFC1918 192.168/16", "192.168.1.1", false},
		{"IPv4 public", "8.8.8.8", false},
		{"IPv6 loopback", "::1", false},
		{"IPv6 unique-local", "fc00::1", false},
		{"IPv6 public", "2001:4860:4860::8888", false},
		{"IPv4 link-local", "169.254.1.1", true},
		{"IPv4 cloud metadata", "169.254.169.254", true},
		{"IPv6 link-local", "fe80::1", true},
		{"IPv4 unspecified", "0.0.0.0", true},
		{"IPv6 unspecified", "::", true},
		{"IPv4 multicast", "224.0.0.1", true},
		{"IPv4 broadcast", "255.255.255.255", true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ip := net.ParseIP(testCase.ip)
			if ip == nil {
				t.Fatalf("invalid test IP %q", testCase.ip)
			}
			if got := isBlockedIPAllowingPrivate(ip); got != testCase.blocked {
				t.Errorf("isBlockedIPAllowingPrivate(%s) = %v, want %v", testCase.ip, got, testCase.blocked)
			}
		})
	}
}

// TestBrowseFetchHonoursThePrivateNetworkOptIn uses the production clients
// built by NewServer — no injected dialer — so it proves the opt-in actually
// reaches a real loopback address and that omitting it still does not.
func TestBrowseFetchHonoursThePrivateNetworkOptIn(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("lan-service"))
	}))
	defer upstream.Close()
	target := upstream.URL + "/"

	fetch := func(t *testing.T, allowPrivate bool) *httptest.ResponseRecorder {
		t.Helper()
		server := testServer(t)
		body, _ := json.Marshal(browseFetchInput{URL: target, Method: "GET", AllowPrivateNetwork: allowPrivate})
		request := httptest.NewRequest(http.MethodPost, "/v1/browse/fetch", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		return response
	}

	t.Run("blocked by default", func(t *testing.T) {
		response := fetch(t, false)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d body=%s, want 403", response.Code, response.Body.String())
		}
		var problem struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
			t.Fatal(err)
		}
		if problem.Code != "browse_target_blocked" {
			t.Fatalf("code = %q, want browse_target_blocked", problem.Code)
		}
	})

	t.Run("allowed when opted in", func(t *testing.T) {
		response := fetch(t, true)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s, want 200", response.Code, response.Body.String())
		}
		var output browseFetchOutput
		if err := json.Unmarshal(response.Body.Bytes(), &output); err != nil {
			t.Fatal(err)
		}
		payload, err := base64.StdEncoding.DecodeString(output.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(payload) != "lan-service" {
			t.Fatalf("body = %q, want %q", payload, "lan-service")
		}
	})
}
