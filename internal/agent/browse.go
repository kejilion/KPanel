package agent

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/netguard"
)

// Browse relays a single buffered HTTP request to a URL the admin is
// browsing through the desktop "browser" feature (see docs/browser-egress.md).
// This host's own network egress is the trust boundary — the caller never
// supplies a separate credential the way the original SSH-tunnel prototype
// did — so the one guard that matters here is refusing to let that egress be
// pointed back at this host's own private/link-local network.
const (
	maxBrowseURLBytes           = 2048
	maxBrowseHeaderCount        = 64
	maxBrowseHeaderBytes        = 8 << 10
	maxBrowseFetchResponseBytes = 8 << 20
	browseFetchTimeout          = 20 * time.Second
	browseDialTimeout           = 8 * time.Second
)

// browseHopByHopHeaders mirrors the standard hop-by-hop set plus Host, which
// must be derived only from the parsed target URL below, never from a
// caller-supplied header.
var browseHopByHopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
	"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Host",
}

var browseAllowedMethods = map[string]bool{
	"GET": true, "HEAD": true, "POST": true, "PUT": true,
	"PATCH": true, "DELETE": true, "OPTIONS": true,
}

type browseFetchInput struct {
	URL     string              `json:"url"`
	Method  string              `json:"method"`
	Headers map[string][]string `json:"headers,omitempty"`
	Body    string              `json:"body,omitempty"` // base64, raw request body
	// AllowPrivateNetwork relaxes the private-address guard for this one
	// request. Only the Panel on this same host can set it, and it only ever
	// sets it from this host's own store.AllowedHosts.BrowseAllowPrivateNetworks
	// — never from anything a federation controller sent. See the comment on
	// that field for why the default has to be off.
	AllowPrivateNetwork bool `json:"allowPrivateNetwork,omitempty"`
}

type browseFetchOutput struct {
	StatusCode int                 `json:"statusCode"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"` // base64, raw response body
}

func (s *Server) browseFetch(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	var input browseFetchInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}

	target, method, body, err := validateBrowseFetchInput(input)
	if err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "browse_request_invalid", "浏览请求无效", safeDetail(err))
		return
	}

	outHeader := make(http.Header, len(input.Headers))
	for name, values := range input.Headers {
		for _, value := range values {
			outHeader.Add(name, value)
		}
	}
	stripBrowseHopByHopHeaders(outHeader)

	ctx, cancel := context.WithTimeout(r.Context(), browseFetchTimeout)
	defer cancel()

	outReq, err := http.NewRequestWithContext(ctx, method, target.String(), strings.NewReader(body))
	if err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "browse_request_invalid", "浏览请求无效", safeDetail(err))
		return
	}
	outReq.Header = outHeader
	outReq.Host = target.Host // authoritative Host, derived only from the parsed target URL

	resp, err := s.browseHTTPClient(input.AllowPrivateNetwork).Do(outReq)
	if err != nil {
		status, code := browseFetchErrorStatus(err)
		writeProblem(w, requestID, status, code, "浏览目标不可达", safeDetail(err))
		return
	}
	defer resp.Body.Close()

	reader := io.LimitReader(resp.Body, maxBrowseFetchResponseBytes+1)
	payload, err := io.ReadAll(reader)
	if err != nil {
		writeProblem(w, requestID, http.StatusBadGateway, "browse_fetch_read_failed", "读取浏览响应失败", safeDetail(err))
		return
	}
	if len(payload) > maxBrowseFetchResponseBytes {
		writeProblem(w, requestID, http.StatusBadGateway, "browse_fetch_response_too_large", "浏览响应超出大小限制", "")
		return
	}

	respHeader := resp.Header.Clone()
	stripBrowseHopByHopHeaders(respHeader)
	writeJSON(w, http.StatusOK, browseFetchOutput{
		StatusCode: resp.StatusCode,
		Headers:    map[string][]string(respHeader),
		Body:       base64.StdEncoding.EncodeToString(payload),
	})
}

// browseHTTPClient picks between the two long-lived clients built at startup.
// They differ only in their dialer's blocked-IP predicate, and are kept as
// two clients rather than one client rebuilt per request so connection reuse
// still works — and so a relaxed connection can never be reused to serve a
// request that did not ask for the relaxation.
func (s *Server) browseHTTPClient(allowPrivate bool) *http.Client {
	if allowPrivate && s.browseLANClient != nil {
		return s.browseLANClient
	}
	return s.browseClient
}

func validateBrowseFetchInput(input browseFetchInput) (*url.URL, string, string, error) {
	if len(input.URL) == 0 || len(input.URL) > maxBrowseURLBytes {
		return nil, "", "", errors.New("url must be a non-empty absolute http(s) URL within the size limit")
	}
	target, err := url.Parse(input.URL)
	if err != nil || target.Host == "" || target.Hostname() == "" {
		return nil, "", "", errors.New("url must be an absolute http(s) URL")
	}
	if target.Scheme != "http" && target.Scheme != "https" {
		return nil, "", "", errors.New("url scheme must be http or https")
	}
	if target.User != nil {
		return nil, "", "", errors.New("url must not contain userinfo")
	}

	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !browseAllowedMethods[method] {
		return nil, "", "", fmt.Errorf("unsupported method %q", method)
	}

	if len(input.Headers) > maxBrowseHeaderCount {
		return nil, "", "", errors.New("too many headers")
	}
	for name, values := range input.Headers {
		if len(name) > maxBrowseHeaderBytes {
			return nil, "", "", errors.New("header name too large")
		}
		for _, value := range values {
			if len(value) > maxBrowseHeaderBytes {
				return nil, "", "", errors.New("header value too large")
			}
		}
	}

	var body string
	if input.Body != "" {
		decoded, err := base64.StdEncoding.DecodeString(input.Body)
		if err != nil {
			return nil, "", "", errors.New("body must be base64-encoded")
		}
		body = string(decoded)
	}

	return target, method, body, nil
}

func stripBrowseHopByHopHeaders(h http.Header) {
	for _, name := range browseHopByHopHeaders {
		h.Del(name)
	}
}

func browseFetchErrorStatus(err error) (int, string) {
	if errors.Is(err, errBrowseTargetBlocked) {
		return http.StatusForbidden, "browse_target_blocked"
	}
	return http.StatusBadGateway, "browse_fetch_failed"
}

// errBrowseTargetBlocked is returned by browseDialer when every address a
// target hostname resolves to falls inside isBlockedIP — including the
// common DNS-rebinding shape where the hostname is public at request-parse
// time but resolves to a loopback/private/link-local address at dial time.
var errBrowseTargetBlocked = errors.New("browse fetch target resolves only to blocked addresses")

// ipResolver abstracts hostname resolution so tests can inject canned
// results instead of depending on real DNS.
type ipResolver func(ctx context.Context, host string) ([]net.IP, error)

func lookupBrowseIPs(ctx context.Context, host string) ([]net.IP, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(addrs))
	for i, addr := range addrs {
		ips[i] = addr.IP
	}
	return ips, nil
}

// browseDialer resolves the target hostname itself and dials only an
// address that passed the blocked-IP check, rather than letting
// net.Dialer.DialContext resolve-and-dial in one step — the latter would
// let a hostname that is public when validated but resolves to a
// loopback/private address at connection time (DNS rebinding) reach the
// host's internal network. blocked defaults to isBlockedIP in production;
// tests override it to exercise the dial plumbing without depending on
// what a fake hostname actually resolves to.
type browseDialer struct {
	resolve ipResolver
	blocked func(net.IP) bool
	dial    net.Dialer
}

func newBrowseDialer(allowPrivate bool) *browseDialer {
	blocked := isBlockedIP
	if allowPrivate {
		blocked = isBlockedIPAllowingPrivate
	}
	return &browseDialer{
		resolve: lookupBrowseIPs,
		blocked: blocked,
		dial:    net.Dialer{Timeout: browseDialTimeout, KeepAlive: 15 * time.Second},
	}
}

func (d *browseDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	var candidates []net.IP
	if literal := net.ParseIP(host); literal != nil {
		candidates = []net.IP{literal}
	} else {
		candidates, err = d.resolve(ctx, host)
		if err != nil {
			return nil, err
		}
	}

	for _, ip := range candidates {
		if d.blocked(ip) {
			continue
		}
		return d.dial.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	return nil, errBrowseTargetBlocked
}

// isBlockedIP and isBlockedIPAllowingPrivate are this package's two names for
// the shared egress guard in internal/netguard, which decides by the IANA
// special-purpose registry rather than by net.IP's IsPrivate/IsGlobalUnicast
// combination these two used to spell out independently. They stay as
// separate function values because newBrowseDialer injects one of them into
// browseDialer.blocked, which is what makes the dial path testable without a
// network.
func isBlockedIP(ip net.IP) bool {
	return netguard.Blocked(ip, false)
}

// isBlockedIPAllowingPrivate is the relaxed guard used when the operator has
// opted this host into LAN browsing (store.AllowedHosts.BrowseAllowPrivateNetworks).
// It unblocks exactly what "LAN" means — loopback, RFC1918/ULA, and the CGNAT
// range a tailnet peer lives in. Link-local stays blocked on purpose:
// 169.254.0.0/16 is not a LAN, and the address most likely to answer in it is
// the cloud metadata service at 169.254.169.254, which hands out IAM
// credentials to whatever asks. The other metadata endpoints stay blocked in
// this mode too; see netguard.metadataAddresses.
func isBlockedIPAllowingPrivate(ip net.IP) bool {
	return netguard.Blocked(ip, true)
}

func newBrowseHTTPClient(allowPrivate bool) *http.Client {
	dialer := newBrowseDialer(allowPrivate)
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			DisableCompression:    true,
			MaxIdleConns:          32,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       60 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
		Timeout: browseFetchTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			// Never auto-follow: the caller (Panel/frontend) rewrites
			// Location itself, same invariant the original tunnel core used.
			return http.ErrUseLastResponse
		},
	}
}
