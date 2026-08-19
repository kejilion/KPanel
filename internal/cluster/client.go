package cluster

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	pairPath    = "/api/v1/federation/pair"
	summaryPath = "/api/v1/federation/summary"
	revokePath  = "/api/v1/federation/revoke"
)

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]net.IP, error)
}

type netResolver struct {
	resolver *net.Resolver
}

func (r netResolver) LookupNetIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return r.resolver.LookupIP(ctx, network, host)
}

type RemoteClientConfig struct {
	PrivateCIDRs []string
	RootCAs      *x509.CertPool
	Resolver     resolver
	Dialer       func(context.Context, string, string) (net.Conn, error)
	Timeout      time.Duration
}

type RemoteClient struct {
	allowedPrivate []netip.Prefix
	resolver       resolver
	dialer         func(context.Context, string, string) (net.Conn, error)
	client         *http.Client
	streamClient   *http.Client
}

type RemoteError struct {
	Code       string
	StatusCode int
}

func (e *RemoteError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("remote KPanel request failed with HTTP %d", e.StatusCode)
	}
	return "remote KPanel request failed"
}

func NewRemoteClient(config RemoteClientConfig) (*RemoteClient, error) {
	allowed := make([]netip.Prefix, 0, len(config.PrivateCIDRs))
	for _, raw := range config.PrivateCIDRs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("parse cluster private CIDR %q: %w", raw, err)
		}
		allowed = append(allowed, prefix.Masked())
	}
	if config.Resolver == nil {
		config.Resolver = netResolver{resolver: net.DefaultResolver}
	}
	if config.Dialer == nil {
		dialer := &net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}
		config.Dialer = dialer.DialContext
	}
	if config.Timeout == 0 {
		config.Timeout = 6 * time.Second
	}
	if config.Timeout < time.Second || config.Timeout > 15*time.Second {
		return nil, errors.New("cluster remote timeout must be between 1 and 15 seconds")
	}
	remote := &RemoteClient{
		allowedPrivate: allowed,
		resolver:       config.Resolver,
		dialer:         config.Dialer,
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           remote.dialContext,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   2,
		MaxConnsPerHost:       2,
		IdleConnTimeout:       45 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    config.RootCAs,
		},
	}
	remote.client = &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("cluster redirect rejected")
		},
	}
	remote.streamClient = &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("cluster redirect rejected")
		},
	}
	return remote, nil
}

func NormalizeOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 512 || strings.ContainsAny(raw, "\r\n\t\x00") {
		return "", ErrInvalidOrigin
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.Path != "" || parsed.RawPath != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.ForceQuery {
		return "", ErrInvalidOrigin
	}
	if strings.ContainsAny(parsed.Host, "%/\\?#@,; ") {
		return "", ErrInvalidOrigin
	}
	host := strings.ToLower(parsed.Hostname())
	if strings.HasSuffix(host, ".") || !validOriginHost(host) {
		return "", ErrInvalidOrigin
	}
	port := parsed.Port()
	if port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", ErrInvalidOrigin
		}
	}
	if port == "443" {
		port = ""
	}
	if address, err := netip.ParseAddr(host); err == nil {
		host = address.String()
		if address.Is6() {
			host = "[" + host + "]"
		}
	}
	if port != "" {
		host = net.JoinHostPort(strings.Trim(host, "[]"), port)
	}
	return "https://" + host, nil
}

func validOriginHost(host string) bool {
	if address, err := netip.ParseAddr(host); err == nil {
		return address.Zone() == ""
	}
	if len(host) > 253 || strings.Contains(host, "..") {
		return false
	}
	onlyNumeric := true
	for _, character := range host {
		if (character < '0' || character > '9') && character != '.' {
			onlyNumeric = false
		}
		if character > 127 {
			return false
		}
	}
	if onlyNumeric {
		return false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') &&
				(character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func (c *RemoteClient) Pair(ctx context.Context, origin string, input PairRequest) (PairResponse, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return PairResponse{}, fmt.Errorf("encode federation pair request: %w", err)
	}
	if len(body) > MaxPairBytes {
		return PairResponse{}, errors.New("federation pair request is too large")
	}
	request, err := c.newRequest(ctx, http.MethodPost, origin, pairPath, bytes.NewReader(body))
	if err != nil {
		return PairResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	var response PairResponse
	if err := c.doJSON(request, MaxPairBytes, &response); err != nil {
		return PairResponse{}, err
	}
	return response, nil
}

func (c *RemoteClient) Summary(
	ctx context.Context,
	origin, controllerID, targetID string,
	privateKey ed25519.PrivateKey,
	now time.Time,
) (FederationSummary, error) {
	request, err := c.newRequest(ctx, http.MethodGet, origin, summaryPath, http.NoBody)
	if err != nil {
		return FederationSummary{}, err
	}
	nonce, err := randomHex(16)
	if err != nil {
		return FederationSummary{}, err
	}
	if err := SignRequest(request, controllerID, targetID, privateKey, now, nonce); err != nil {
		return FederationSummary{}, err
	}
	request.Header.Set(FederationCapabilitiesHeader, SecurityEntrancePathCapability)
	var response FederationSummary
	if err := c.doJSON(request, MaxSummaryBytes, &response); err != nil {
		return FederationSummary{}, err
	}
	return response, nil
}

func (c *RemoteClient) Revoke(
	ctx context.Context,
	origin, controllerID, targetID string,
	privateKey ed25519.PrivateKey,
	now time.Time,
) error {
	request, err := c.newRequest(ctx, http.MethodDelete, origin, revokePath, http.NoBody)
	if err != nil {
		return err
	}
	nonce, err := randomHex(16)
	if err != nil {
		return err
	}
	if err := SignRequest(request, controllerID, targetID, privateKey, now, nonce); err != nil {
		return err
	}
	var result struct {
		Revoked bool `json:"revoked"`
	}
	if err := c.doJSON(request, 4<<10, &result); err != nil {
		return err
	}
	if !result.Revoked {
		return errors.New("remote controller was not revoked")
	}
	return nil
}

func (c *RemoteClient) newRequest(
	ctx context.Context,
	method, origin, path string,
	body io.Reader,
) (*http.Request, error) {
	normalized, err := NormalizeOrigin(origin)
	if err != nil || normalized != origin {
		return nil, ErrInvalidOrigin
	}
	request, err := http.NewRequestWithContext(ctx, method, origin+path, body)
	if err != nil {
		return nil, fmt.Errorf("create federation request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "KPanel federation/"+FederationProtocol)
	return request, nil
}

func (c *RemoteClient) doJSON(request *http.Request, limit int64, target any) error {
	response, err := c.client.Do(request)
	if err != nil {
		return classifyRemoteTransportError(err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		code := "remote_error"
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "authentication_failed"
		case http.StatusConflict, http.StatusGone:
			code = "identity_changed"
		case http.StatusTooManyRequests:
			code = "rate_limited"
		case http.StatusUpgradeRequired:
			code = "protocol_incompatible"
		}
		return &RemoteError{Code: code, StatusCode: response.StatusCode}
	}
	if mediaType := strings.ToLower(response.Header.Get("Content-Type")); !strings.HasPrefix(mediaType, "application/json") {
		return &RemoteError{Code: "invalid_response"}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return &RemoteError{Code: "invalid_response"}
	}
	if int64(len(body)) > limit {
		return &RemoteError{Code: "response_too_large"}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &RemoteError{Code: "invalid_response"}
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return &RemoteError{Code: "invalid_response"}
	}
	return nil
}

func classifyRemoteTransportError(err error) error {
	var certificateError *tls.CertificateVerificationError
	var hostnameError x509.HostnameError
	var unknownAuthority x509.UnknownAuthorityError
	if errors.As(err, &certificateError) || errors.As(err, &hostnameError) ||
		errors.As(err, &unknownAuthority) {
		return &RemoteError{Code: "tls_error"}
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		if errors.As(urlError.Err, &certificateError) || errors.As(urlError.Err, &hostnameError) ||
			errors.As(urlError.Err, &unknownAuthority) {
			return &RemoteError{Code: "tls_error"}
		}
	}
	if errors.Is(err, ErrPrivateOrigin) {
		return ErrPrivateOrigin
	}
	return &RemoteError{Code: "unreachable"}
}

func (c *RemoteClient) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("cluster remote network is unsupported")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrInvalidOrigin
	}
	addresses, err := c.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastError error
	for _, candidate := range addresses {
		connection, err := c.dialer(ctx, "tcp", net.JoinHostPort(candidate.String(), port))
		if err == nil {
			return connection, nil
		}
		lastError = err
	}
	if lastError == nil {
		lastError = errors.New("cluster origin did not resolve")
	}
	return nil, lastError
}

func (c *RemoteClient) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap()
		if !c.addressAllowed(address) {
			return nil, ErrPrivateOrigin
		}
		return []netip.Addr{address}, nil
	}
	resolved, err := c.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(resolved) == 0 {
		return nil, errors.New("cluster origin DNS resolution failed")
	}
	result := make([]netip.Addr, 0, len(resolved))
	seen := make(map[netip.Addr]struct{}, len(resolved))
	for _, value := range resolved {
		address, ok := netip.AddrFromSlice(value)
		if !ok {
			return nil, errors.New("cluster origin DNS returned an invalid address")
		}
		address = address.Unmap()
		if !c.addressAllowed(address) {
			return nil, ErrPrivateOrigin
		}
		if _, ok := seen[address]; !ok {
			seen[address] = struct{}{}
			result = append(result, address)
		}
	}
	return result, nil
}

func (c *RemoteClient) addressAllowed(address netip.Addr) bool {
	if !address.IsValid() || address.IsUnspecified() || address.IsLoopback() ||
		address.IsMulticast() || address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() {
		return false
	}
	if address.IsPrivate() || cgNATPrefix.Contains(address) {
		for _, prefix := range c.allowedPrivate {
			if prefix.Contains(address) {
				return true
			}
		}
		return false
	}
	if !address.IsGlobalUnicast() || reservedAddress(address) {
		return false
	}
	return true
}

var (
	cgNATPrefix      = netip.MustParsePrefix("100.64.0.0/10")
	reservedPrefixes = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("::/96"),
		netip.MustParsePrefix("64:ff9b::/96"),
		netip.MustParsePrefix("64:ff9b:1::/48"),
		netip.MustParsePrefix("100::/64"),
		netip.MustParsePrefix("fec0::/10"),
		netip.MustParsePrefix("2001::/32"),
		netip.MustParsePrefix("2001:2::/48"),
		netip.MustParsePrefix("2001:10::/28"),
		netip.MustParsePrefix("2001:20::/28"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("2002::/16"),
	}
)

func reservedAddress(address netip.Addr) bool {
	for _, prefix := range reservedPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
