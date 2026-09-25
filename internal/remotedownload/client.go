package remotedownload

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
)

const (
	MaxURLBytes      = 4096
	MaxRedirects     = 5
	maxDialAddresses = 8
)

var (
	ErrInvalidURL       = errors.New("remote download URL is invalid")
	ErrAddressBlocked   = errors.New("remote download address is blocked")
	ErrRedirectRejected = errors.New("remote download redirect is rejected")
	ErrTLS              = errors.New("remote download TLS validation failed")
	ErrUnreachable      = errors.New("remote download source is unreachable")
	ErrEncoding         = errors.New("remote download content encoding is unsupported")
	ErrPartialContent   = errors.New("remote download partial content is unsupported")
	ErrIdleTimeout      = errors.New("remote download body timed out")
)

type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("remote download source returned HTTP %d", e.StatusCode)
}

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type Config struct {
	Resolver              Resolver
	Dialer                func(context.Context, string, string) (net.Conn, error)
	ConnectTimeout        time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleTimeout           time.Duration
	RejectRedirects       bool
}

type Client struct {
	resolver       Resolver
	dialer         func(context.Context, string, string) (net.Conn, error)
	httpClient     *http.Client
	connectTimeout time.Duration
	idleTimeout    time.Duration
}

func NewClient(config Config) *Client {
	if config.Resolver == nil {
		config.Resolver = net.DefaultResolver
	}
	if config.ConnectTimeout <= 0 {
		config.ConnectTimeout = 5 * time.Second
	}
	if config.TLSHandshakeTimeout <= 0 {
		config.TLSHandshakeTimeout = 5 * time.Second
	}
	if config.ResponseHeaderTimeout <= 0 {
		config.ResponseHeaderTimeout = 15 * time.Second
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = 45 * time.Second
	}
	if config.Dialer == nil {
		dialer := &net.Dialer{Timeout: config.ConnectTimeout, KeepAlive: 30 * time.Second}
		config.Dialer = dialer.DialContext
	}
	client := &Client{
		resolver: config.Resolver, dialer: config.Dialer,
		connectTimeout: config.ConnectTimeout, idleTimeout: config.IdleTimeout,
	}
	transport := &http.Transport{
		Proxy:                  nil,
		DialContext:            client.dialContext,
		ForceAttemptHTTP2:      true,
		DisableCompression:     true,
		MaxIdleConns:           4,
		MaxIdleConnsPerHost:    2,
		MaxConnsPerHost:        2,
		IdleConnTimeout:        45 * time.Second,
		TLSHandshakeTimeout:    config.TLSHandshakeTimeout,
		ResponseHeaderTimeout:  config.ResponseHeaderTimeout,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 64 << 10,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	client.httpClient = &http.Client{
		Transport:     transport,
		CheckRedirect: checkRedirect,
	}
	if config.RejectRedirects {
		client.httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
			return ErrRedirectRejected
		}
	}
	return client
}

func ValidateURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > MaxURLBytes || hasControl(raw) || strings.Contains(raw, `\`) {
		return nil, ErrInvalidURL
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, ErrInvalidURL
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" ||
		strings.Contains(parsed.Path, `\`) || strings.Contains(parsed.RawPath, `\`) {
		return nil, ErrInvalidURL
	}
	hostname := parsed.Hostname()
	if strings.Contains(hostname, "%") || !validHostname(hostname) {
		return nil, ErrInvalidURL
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return nil, ErrInvalidURL
	}
	if port := parsed.Port(); port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return nil, ErrInvalidURL
		}
	}
	return parsed, nil
}

func SourceDisplay(parsed *url.URL) string {
	if parsed == nil {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func (c *Client) Open(ctx context.Context, raw string) (*http.Response, error) {
	parsed, err := ValidateURL(raw)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), http.NoBody)
	if err != nil {
		return nil, ErrInvalidURL
	}
	request.Header.Set("Accept", "application/octet-stream, */*")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "KPanel-Remote-Download/1")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, classifyError(ctx, err)
	}
	if response.StatusCode == http.StatusPartialContent {
		response.Body.Close()
		return nil, ErrPartialContent
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, &StatusError{StatusCode: response.StatusCode}
	}
	if response.Header.Get("Content-Range") != "" {
		response.Body.Close()
		return nil, ErrPartialContent
	}
	if encoding := strings.TrimSpace(response.Header.Get("Content-Encoding")); encoding != "" && !strings.EqualFold(encoding, "identity") {
		response.Body.Close()
		return nil, ErrEncoding
	}
	response.Body = newIdleReadCloser(response.Body, c.idleTimeout)
	return response, nil
}

func checkRedirect(request *http.Request, via []*http.Request) error {
	if len(via) > MaxRedirects {
		return ErrRedirectRejected
	}
	parsed, err := ValidateURL(request.URL.String())
	if err != nil {
		return ErrRedirectRejected
	}
	if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && parsed.Scheme != "https" {
		return ErrRedirectRejected
	}
	request.Header.Del("Authorization")
	request.Header.Del("Cookie")
	request.Header.Del("Referer")
	request.Header.Set("Accept", "application/octet-stream, */*")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "KPanel-Remote-Download/1")
	return nil
}

func classifyError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	for _, candidate := range []error{
		ErrInvalidURL, ErrAddressBlocked, ErrRedirectRejected, ErrPartialContent, ErrIdleTimeout,
	} {
		if errors.Is(err, candidate) {
			return candidate
		}
	}
	var certificateError *tls.CertificateVerificationError
	var hostnameError x509.HostnameError
	var unknownAuthorityError x509.UnknownAuthorityError
	if errors.As(err, &certificateError) || errors.As(err, &hostnameError) ||
		errors.As(err, &unknownAuthorityError) {
		return ErrTLS
	}
	return ErrUnreachable
}

func (c *Client) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, ErrAddressBlocked
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrInvalidURL
	}
	connectContext, cancel := context.WithTimeout(ctx, c.connectTimeout)
	defer cancel()
	addresses, err := c.resolve(connectContext, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) > maxDialAddresses {
		addresses = addresses[:maxDialAddresses]
	}
	var lastError error
	for _, candidate := range addresses {
		connection, err := c.dialer(connectContext, "tcp", net.JoinHostPort(candidate.String(), port))
		if err == nil {
			return connection, nil
		}
		lastError = err
		if connectContext.Err() != nil {
			break
		}
	}
	if lastError == nil {
		lastError = ErrUnreachable
	}
	return nil, lastError
}

func (c *Client) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap()
		if !publicAddress(address) {
			return nil, ErrAddressBlocked
		}
		return []netip.Addr{address}, nil
	}
	resolved, err := c.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(resolved) == 0 {
		return nil, ErrUnreachable
	}
	result := make([]netip.Addr, 0, len(resolved))
	seen := make(map[netip.Addr]struct{}, len(resolved))
	for _, address := range resolved {
		address = address.Unmap()
		if !publicAddress(address) {
			return nil, ErrAddressBlocked
		}
		if _, exists := seen[address]; !exists {
			seen[address] = struct{}{}
			result = append(result, address)
		}
	}
	return result, nil
}

func validHostname(host string) bool {
	if address, err := netip.ParseAddr(host); err == nil {
		return address.IsValid()
	}
	if len(host) > 253 || !isASCII(host) {
		return false
	}
	if strings.HasSuffix(host, ".") {
		return false
	}
	if strings.IndexFunc(host, func(character rune) bool {
		return (character < '0' || character > '9') && character != '.'
	}) == -1 {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for index := range len(label) {
			character := label[index]
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
				(character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func publicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || address.IsUnspecified() || address.IsLoopback() ||
		address.IsMulticast() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
		address.IsPrivate() || cgNATPrefix.Contains(address) || !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range reservedPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func hasControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func isASCII(value string) bool {
	for index := range len(value) {
		if value[index] > 0x7f {
			return false
		}
	}
	return true
}

var (
	cgNATPrefix      = netip.MustParsePrefix("100.64.0.0/10")
	reservedPrefixes = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		// Azure WireServer is publicly addressed but exposes host-node services.
		netip.MustParsePrefix("168.63.129.16/32"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("192.31.196.0/24"),
		netip.MustParsePrefix("192.52.193.0/24"),
		netip.MustParsePrefix("192.88.99.0/24"),
		netip.MustParsePrefix("192.175.48.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("::/96"),
		netip.MustParsePrefix("64:ff9b::/96"),
		netip.MustParsePrefix("64:ff9b:1::/48"),
		netip.MustParsePrefix("100::/64"),
		netip.MustParsePrefix("100:0:0:1::/64"),
		netip.MustParsePrefix("fec0::/10"),
		netip.MustParsePrefix("2001::/23"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("2002::/16"),
		netip.MustParsePrefix("2620:4f:8000::/48"),
		netip.MustParsePrefix("3fff::/20"),
		netip.MustParsePrefix("5f00::/16"),
	}
)

type idleReadCloser struct {
	source     io.ReadCloser
	timeout    time.Duration
	mu         sync.Mutex
	timer      *time.Timer
	generation uint64
	closed     bool
	timedOut   bool
}

func newIdleReadCloser(source io.ReadCloser, timeout time.Duration) io.ReadCloser {
	return &idleReadCloser{source: source, timeout: timeout}
}

func (r *idleReadCloser) Read(buffer []byte) (int, error) {
	r.mu.Lock()
	if r.timedOut {
		r.mu.Unlock()
		return 0, ErrIdleTimeout
	}
	if r.closed {
		r.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	r.armLocked()
	r.mu.Unlock()

	count, err := r.source.Read(buffer)
	r.mu.Lock()
	timedOut := r.timedOut
	r.generation++
	if r.timer != nil {
		r.timer.Stop()
	}
	r.mu.Unlock()
	if timedOut {
		return count, ErrIdleTimeout
	}
	return count, err
}

func (r *idleReadCloser) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.generation++
	if r.timer != nil {
		r.timer.Stop()
	}
	r.mu.Unlock()
	return r.source.Close()
}

func (r *idleReadCloser) armLocked() {
	r.generation++
	generation := r.generation
	if r.timer != nil {
		r.timer.Stop()
	}
	r.timer = time.AfterFunc(r.timeout, func() {
		r.mu.Lock()
		if r.closed || r.generation != generation {
			r.mu.Unlock()
			return
		}
		r.timedOut = true
		r.closed = true
		r.generation++
		r.mu.Unlock()
		_ = r.source.Close()
	})
}
