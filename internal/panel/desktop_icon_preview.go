package panel

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/desktopworkspace"
	"github.com/kejilion/kejilion-panel/internal/sites"
)

// Desktop shortcut icon preview.
//
// POST /api/v1/desktop/icon-preview fetches a site's favicon so
// the shortcut dialog can pre-fill the icon slot while the user is typing a
// URL. The endpoint is read-only: it never writes the workspace, never
// persists the fetched bytes, and is not audited. The bytes it returns are
// guaranteed to pass the same validation PutIcon applies, so an accepted
// preview can later be uploaded unchanged.
//
// A shortcut URL may legitimately point at a private address (it is only a
// navigation target for the admin's browser), but the panel itself must never
// dial such a target: every host the fetcher contacts is resolved first and
// all of its addresses must be globally routable. See
// docs/desktop-icon-workspace.md.

const desktopIconPreviewPath = "/api/v1/desktop/icon-preview"

const (
	desktopIconPreviewMaxURLBytes    = 2048
	desktopIconPreviewMaxPageBytes   = 256 << 10
	desktopIconPreviewMaxRedirects   = 5
	desktopIconPreviewRequestTimeout = 4 * time.Second
	desktopIconPreviewTotalTimeout   = 12 * time.Second
)

var errDesktopIconPreviewNotFound = errors.New("desktop icon preview not found")
var errDesktopIconPreviewTooLarge = errors.New("desktop icon preview response exceeds limit")

// Fixed-size, per-Panel budget: no attacker-controlled map or request queue.
type desktopIconPreviewBudget struct {
	mu       sync.Mutex
	window   time.Time
	requests int
	active   int
}

func (b *desktopIconPreviewBudget) acquire(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active >= 4 {
		return false
	}
	if b.window.IsZero() || now.Sub(b.window) >= time.Minute {
		b.window, b.requests = now, 0
	}
	if b.requests >= 60 {
		return false
	}
	b.requests++
	b.active++
	return true
}

func (b *desktopIconPreviewBudget) release() {
	b.mu.Lock()
	b.active--
	b.mu.Unlock()
}

// hostLookupFunc resolves a hostname to the addresses the preview fetcher may
// dial. Production lookups require every address to be public; tests inject a
// loopback lookup so the fetch flow can run against httptest servers.
type hostLookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// desktopIconPreviewBlockedPrefixes lists the IANA special-purpose ranges the
// preview fetcher never dials, in addition to the netip predicates in
// isPublicIP. The list is explicit so the policy does not silently change
// when a standard library predicate is redefined.
var desktopIconPreviewBlockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this" network
	netip.MustParsePrefix("10.0.0.0/8"),      // RFC 1918
	netip.MustParsePrefix("100.64.0.0/10"),   // shared address space
	netip.MustParsePrefix("127.0.0.0/8"),     // loopback
	netip.MustParsePrefix("169.254.0.0/16"),  // link-local, including cloud metadata 169.254.169.254
	netip.MustParsePrefix("172.16.0.0/12"),   // RFC 1918
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("192.88.99.0/24"),  // deprecated 6to4 relay
	netip.MustParsePrefix("192.168.0.0/16"),  // RFC 1918
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("224.0.0.0/4"),     // multicast
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, including 255.255.255.255
	netip.MustParsePrefix("::1/128"),         // loopback
	netip.MustParsePrefix("::/128"),          // unspecified
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64 well-known prefix
	netip.MustParsePrefix("64:ff9b:1::/48"),  // NAT64 local-use prefix
	netip.MustParsePrefix("100::/64"),        // discard-only
	netip.MustParsePrefix("2001::/23"),       // special-purpose and transition protocols
	netip.MustParsePrefix("2001:2::/48"),     // benchmarking
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
	netip.MustParsePrefix("2001:10::/28"),    // ORCHID
	netip.MustParsePrefix("2002::/16"),       // 6to4
	netip.MustParsePrefix("fc00::/7"),        // unique-local
	netip.MustParsePrefix("fe80::/10"),       // link-local
	netip.MustParsePrefix("ff00::/8"),        // multicast
}

// isPublicIP reports whether addr is a globally routable unicast address.
// IPv4-mapped IPv6 addresses are judged by the IPv4 address they embed.
func isPublicIP(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if addr.Is6() && !netip.MustParsePrefix("2000::/3").Contains(addr) {
		return false
	}
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range desktopIconPreviewBlockedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return addr.IsGlobalUnicast()
}

// lookupPublicIPs resolves host and requires every resolved address to be
// public. A single non-public record rejects the whole host, so a DNS
// response cannot smuggle a private target past the check, and the dialer
// below reuses exactly these addresses instead of resolving a second time.
func lookupPublicIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		if !isPublicIP(addr) {
			return nil, fmt.Errorf("desktop icon preview host is not public")
		}
		return []netip.Addr{addr}, nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	result := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok || !isPublicIP(addr) {
			return nil, fmt.Errorf("desktop icon preview host is not public")
		}
		result = append(result, addr)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("desktop icon preview host did not resolve")
	}
	return result, nil
}

// parseDesktopIconPreviewURL validates a user-supplied page URL for the icon
// preview. Only absolute HTTP(S) URLs without credentials are accepted; the
// port is left to the URL because a favicon may live on any port of a public
// host, and the dialer still refuses non-public addresses.
func parseDesktopIconPreviewURL(raw string) (*url.URL, bool) {
	if raw == "" || len(raw) > desktopIconPreviewMaxURLBytes || strings.TrimSpace(raw) != raw {
		return nil, false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.User != nil ||
		parsed.Host == "" || parsed.Hostname() == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, false
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return nil, false
		}
	}
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed, true
}

func desktopIconPreviewDialContext(lookup hostLookupFunc) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: desktopIconPreviewRequestTimeout, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		addrs, err := lookup(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, addr := range addrs {
			conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(addr.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, errors.New("desktop icon preview host did not resolve")
	}
}

func newDesktopIconPreviewClient(lookup hostLookupFunc) *http.Client {
	transport := &http.Transport{
		Proxy:                  nil,
		DisableCompression:     true,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           4,
		MaxIdleConnsPerHost:    2,
		MaxConnsPerHost:        2,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    desktopIconPreviewRequestTimeout,
		ResponseHeaderTimeout:  desktopIconPreviewRequestTimeout,
		MaxResponseHeaderBytes: 32 << 10,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext:            desktopIconPreviewDialContext(lookup),
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > desktopIconPreviewMaxRedirects {
				return errors.New("desktop icon preview redirect limit reached")
			}
			if _, ok := parseDesktopIconPreviewURL(req.URL.String()); !ok {
				return errors.New("desktop icon preview redirect target is invalid")
			}
			if _, err := lookup(req.Context(), req.URL.Hostname()); err != nil {
				return err
			}
			return nil
		},
	}
}

type desktopIconPreview struct {
	contentType string
	data        []byte
}

func fetchDesktopIconPreviewBytes(ctx context.Context, client *http.Client, target *url.URL, maxBytes int64, accept string) ([]byte, int, *url.URL, error) {
	requestCtx, cancel := context.WithTimeout(ctx, desktopIconPreviewRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, target.String(), http.NoBody)
	if err != nil {
		return nil, 0, nil, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", "KPanel-Site-Icon/1")
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, nil, err
	}
	defer response.Body.Close()
	finalURL := response.Request.URL
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil, response.StatusCode, finalURL, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, response.StatusCode, finalURL, err
	}
	if int64(len(body)) > maxBytes {
		return body[:maxBytes], response.StatusCode, finalURL, errDesktopIconPreviewTooLarge
	}
	return body, response.StatusCode, finalURL, nil
}

// desktopIconPreviewFromBytes reports whether body is a shortcut icon: a PNG,
// JPEG or WebP image that passes the same validation PutIcon applies. GIF and
// ICO candidates are skipped because shortcut icons do not accept them.
func desktopIconPreviewFromBytes(body []byte) (desktopIconPreview, bool) {
	contentType, err := sites.ValidateSiteIcon(body)
	if err != nil {
		return desktopIconPreview{}, false
	}
	switch contentType {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return desktopIconPreview{}, false
	}
	if _, err := desktopworkspace.ValidateIcon(contentType, body); err != nil {
		return desktopIconPreview{}, false
	}
	return desktopIconPreview{contentType: contentType, data: body}, true
}

func appendDesktopIconPreviewCandidate(items []*url.URL, candidate *url.URL) []*url.URL {
	value := candidate.String()
	for _, item := range items {
		if item.String() == value {
			return items
		}
	}
	return append(items, candidate)
}

// fetchDesktopIconPreview returns the first usable favicon for target: the
// page's declared icon links in order, then /favicon.ico. When the target URL
// itself serves an icon (for example a direct image link), that is returned
// without parsing HTML. Every failure mode degrades to
// errDesktopIconPreviewNotFound so the dialog can silently keep its current
// icon.
func fetchDesktopIconPreview(ctx context.Context, client *http.Client, target *url.URL) (desktopIconPreview, error) {
	page, status, finalURL, err := fetchDesktopIconPreviewBytes(
		ctx, client, target, desktopIconPreviewMaxPageBytes, "text/html,application/xhtml+xml",
	)
	candidates := make([]*url.URL, 0, 4)
	// Large homepages commonly declare their icons in the head. Inspect only
	// the bounded prefix; oversized image candidates remain rejected below.
	if (err == nil || errors.Is(err, errDesktopIconPreviewTooLarge)) && status == http.StatusOK {
		if icon, ok := desktopIconPreviewFromBytes(page); ok {
			return icon, nil
		}
		if finalURL != nil {
			candidates = append(candidates, sites.ParseSiteIconLinks(page, finalURL)...)
		}
	}
	fallbackBase := target
	if finalURL != nil {
		fallbackBase = finalURL
	}
	fallback := *fallbackBase
	fallback.Path = "/favicon.ico"
	fallback.RawPath = ""
	fallback.RawQuery = ""
	fallback.Fragment = ""
	fallback.RawFragment = ""
	candidates = appendDesktopIconPreviewCandidate(candidates, &fallback)
	for _, candidate := range candidates {
		body, candidateStatus, _, candidateErr := fetchDesktopIconPreviewBytes(
			ctx, client, candidate, desktopworkspace.MaxIconBytes,
			"image/webp,image/png,image/jpeg,image/gif,image/x-icon,*/*;q=0.1",
		)
		if candidateErr != nil || candidateStatus != http.StatusOK {
			continue
		}
		if icon, ok := desktopIconPreviewFromBytes(body); ok {
			return icon, nil
		}
	}
	return desktopIconPreview{}, errDesktopIconPreviewNotFound
}

func (s *Server) handleDesktopIconPreview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "desktop_icon_preview_request_invalid", "Desktop icon preview request is invalid", "")
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if !s.checkOrigin(w, r) || !s.checkCSRF(w, r, session) {
		return
	}
	var input struct {
		URL string `json:"url"`
	}
	if err := decodeLimitedJSON(w, r, 4096, &input); err != nil {
		return
	}
	target, valid := parseDesktopIconPreviewURL(input.URL)
	if !valid {
		s.writeProblem(w, r, http.StatusBadRequest, "desktop_icon_preview_url_invalid", "A valid URL is required", "")
		return
	}
	if !s.iconPreviewBudget.acquire(time.Now()) {
		w.Header().Set("Retry-After", "60")
		s.writeProblem(w, r, http.StatusTooManyRequests, "desktop_icon_preview_busy", "Icon lookup is busy; retry later", "")
		return
	}
	defer s.iconPreviewBudget.release()
	ctx, cancel := context.WithTimeout(r.Context(), desktopIconPreviewTotalTimeout)
	defer cancel()
	client := newDesktopIconPreviewClient(lookupPublicIPs)
	defer client.CloseIdleConnections()
	icon, err := fetchDesktopIconPreview(ctx, client, target)
	if err != nil {
		s.writeProblem(w, r, http.StatusNotFound, "desktop_icon_preview_not_found", "No suitable icon was found", "")
		return
	}
	w.Header().Set("Content-Type", icon.contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(icon.data)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(icon.data)
}
