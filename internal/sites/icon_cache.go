package sites

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	siteIconCacheVersion        = 2
	siteIconHTMLBytes           = 256 << 10
	siteIconBytes               = 256 << 10
	siteIconMetadataBytes       = 4 << 10
	siteIconSuccessTTL          = 7 * 24 * time.Hour
	siteIconMissingTTL          = 24 * time.Hour
	siteIconUnavailableTTL      = 15 * time.Minute
	siteIconDiscoveryTTL        = 15 * time.Second
	siteIconDiscoveryFailureTTL = 15 * time.Second
	siteIconFetchTimeout        = 4 * time.Second
	siteIconMaxLinkedCandidates = 3
	siteIconMaxEntries          = 256
	siteIconMaxCacheBytes       = 32 << 20
	siteIconMaxDimension        = 2048
	siteIconMaxPixels           = 4 << 20
	siteAppearanceNameRunes     = 160
	siteIconOrphanGrace         = time.Minute
)

var (
	ErrSiteIconNotFound    = errors.New("site icon not found")
	ErrSiteIconUnavailable = errors.New("site icon unavailable")

	errSiteIconAbsent    = errors.New("site does not expose a supported icon")
	errSiteIconTransient = errors.New("site icon fetch failed temporarily")
)

// SiteIcon is a validated bitmap returned by the local site icon cache.
type SiteIcon struct {
	ContentType string
	Data        []byte
	Name        string
}

// SiteAppearance contains presentation metadata fetched from the same trusted
// homepage request as the site's icon.
type SiteAppearance struct {
	Name string `json:"name,omitempty"`
}

type siteIconMetadata struct {
	Version       int       `json:"version"`
	SourceKey     string    `json:"sourceKey"`
	ContentSHA256 string    `json:"contentSha256,omitempty"`
	FetchedAt     time.Time `json:"fetchedAt,omitempty"`
	RetryAt       time.Time `json:"retryAt,omitempty"`
	Failure       string    `json:"failure,omitempty"`
	Name          string    `json:"name,omitempty"`
}

type siteIconCacheValue struct {
	icon SiteIcon
	meta siteIconMetadata
}

type siteIconCall struct {
	done chan struct{}
	icon SiteIcon
	err  error
}

type siteIconDiscoveryCall struct {
	done  chan struct{}
	items map[string]contract.SiteSummary
	err   error
}

// IconCache fetches favicon candidates only through the host's loopback Nginx
// listeners. The discovered site remains the authority for the accepted Host
// and TLS SNI names; arbitrary URLs are never dialled.
type IconCache struct {
	dir      string
	discover func() ([]contract.SiteSummary, error)
	now      func() time.Time
	client   *http.Client

	successTTL     time.Duration
	missingTTL     time.Duration
	unavailableTTL time.Duration
	discoveryTTL   time.Duration
	maxEntries     int
	maxCacheBytes  int64

	fetchSlots chan struct{}

	discoveryMu    sync.Mutex
	discoveredAt   time.Time
	discoveredByID map[string]contract.SiteSummary
	discoveryRetry time.Time
	discoveryErr   error
	discoveryCall  *siteIconDiscoveryCall

	callsMu sync.Mutex
	calls   map[string]*siteIconCall

	pruneMu sync.Mutex
}

func NewIconCache(
	dir string,
	discover func() ([]contract.SiteSummary, error),
) (*IconCache, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("site icon cache directory is required")
	}
	if discover == nil {
		return nil, errors.New("site discoverer is required")
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create site icon cache directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect site icon cache directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("site icon cache path must be a non-symlink directory")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("secure site icon cache directory: %w", err)
	}
	return &IconCache{
		dir:            dir,
		discover:       discover,
		now:            time.Now,
		client:         newLocalSiteIconClient(),
		successTTL:     siteIconSuccessTTL,
		missingTTL:     siteIconMissingTTL,
		unavailableTTL: siteIconUnavailableTTL,
		discoveryTTL:   siteIconDiscoveryTTL,
		maxEntries:     siteIconMaxEntries,
		maxCacheBytes:  siteIconMaxCacheBytes,
		fetchSlots:     make(chan struct{}, 4),
		discoveredByID: make(map[string]contract.SiteSummary),
		calls:          make(map[string]*siteIconCall),
	}, nil
}

func newLocalSiteIconClient() *http.Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                  nil,
		DisableCompression:     true,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           8,
		MaxIdleConnsPerHost:    2,
		MaxConnsPerHost:        4,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    2 * time.Second,
		ResponseHeaderTimeout:  3 * time.Second,
		MaxResponseHeaderBytes: 32 << 10,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			target, err := localSiteIconDialAddress(address)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, "tcp4", target)
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   siteIconFetchTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func localSiteIconDialAddress(address string) (string, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil || (port != "80" && port != "443") {
		return "", errors.New("site icon transport rejected a non-web destination")
	}
	return net.JoinHostPort("127.0.0.1", port), nil
}

func (c *IconCache) Get(ctx context.Context, id string) (SiteIcon, error) {
	if !validIconSiteID(id) {
		return SiteIcon{}, ErrSiteIconNotFound
	}
	operationContext, cancel := context.WithTimeout(ctx, siteIconFetchTimeout)
	defer cancel()
	site, discoveryConfirmed, err := c.lookupSite(operationContext, id)
	if err != nil {
		if errors.Is(err, ErrSiteIconNotFound) {
			return SiteIcon{}, ErrSiteIconNotFound
		}
		return SiteIcon{}, fmt.Errorf("%w: discover sites", ErrSiteIconUnavailable)
	}
	if !eligibleIconSite(site) {
		return SiteIcon{}, ErrSiteIconNotFound
	}
	sourceKey := iconSourceKey(site)
	now := c.now().UTC()
	cached, hasIcon, hasMetadata := c.readCache(id, sourceKey)
	if hasIcon && cacheIsFresh(cached.meta.FetchedAt, now, c.successTTL) {
		return cached.icon, nil
	}
	if hasMetadata && cacheIsFresh(now, cached.meta.RetryAt, 0) {
		if hasIcon {
			return cached.icon, nil
		}
		return cached.icon, cachedFailure(cached.meta.Failure)
	}
	if !discoveryConfirmed {
		if hasIcon {
			return cached.icon, nil
		}
		return SiteIcon{}, ErrSiteIconUnavailable
	}

	return c.doRefresh(operationContext, id+"\x00"+sourceKey, func(
		refreshContext context.Context,
	) (SiteIcon, error) {
		refreshNow := c.now().UTC()
		current, currentHasIcon, currentHasMetadata := c.readCache(id, sourceKey)
		if currentHasIcon && cacheIsFresh(current.meta.FetchedAt, refreshNow, c.successTTL) {
			return current.icon, nil
		}
		if currentHasMetadata && cacheIsFresh(refreshNow, current.meta.RetryAt, 0) {
			if currentHasIcon {
				return current.icon, nil
			}
			return current.icon, cachedFailure(current.meta.Failure)
		}

		select {
		case c.fetchSlots <- struct{}{}:
			defer func() { <-c.fetchSlots }()
		case <-refreshContext.Done():
			if currentHasIcon {
				return current.icon, nil
			}
			return SiteIcon{}, refreshContext.Err()
		}

		icon, fetchErr := c.fetch(refreshContext, site)
		if fetchErr == nil {
			metadata := siteIconMetadata{
				Version:       siteIconCacheVersion,
				SourceKey:     sourceKey,
				ContentSHA256: hashSiteIcon(icon.Data),
				FetchedAt:     refreshNow,
				Name:          icon.Name,
			}
			if writeErr := c.writeSuccess(id, icon, metadata); writeErr != nil {
				slog.Warn("site icon cache write failed", "siteID", id, "error", writeErr)
			}
			return icon, nil
		}

		failure := "unavailable"
		retryAt := refreshNow.Add(c.unavailableTTL)
		publicErr := ErrSiteIconUnavailable
		if errors.Is(fetchErr, errSiteIconAbsent) {
			failure = "not_found"
			retryAt = refreshNow.Add(c.missingTTL)
			publicErr = ErrSiteIconNotFound
		}
		metadata := current.meta
		if metadata.SourceKey != sourceKey {
			metadata = siteIconMetadata{}
		}
		metadata.Version = siteIconCacheVersion
		metadata.SourceKey = sourceKey
		metadata.RetryAt = retryAt
		metadata.Failure = failure
		if errors.Is(fetchErr, errSiteIconAbsent) {
			metadata.Name = icon.Name
		}
		if writeErr := c.writeMetadata(id, metadata); writeErr != nil {
			slog.Warn("site icon negative cache write failed", "siteID", id, "error", writeErr)
		}
		if currentHasIcon {
			return current.icon, nil
		}
		return icon, publicErr
	})
}

// Appearance returns the cached homepage title even when the site does not
// expose a supported favicon. Unknown or ineligible sites remain hidden.
func (c *IconCache) Appearance(ctx context.Context, id string) (SiteAppearance, error) {
	if !validIconSiteID(id) {
		return SiteAppearance{}, ErrSiteIconNotFound
	}
	operationContext, cancel := context.WithTimeout(ctx, siteIconFetchTimeout)
	defer cancel()
	site, _, err := c.lookupSite(operationContext, id)
	if err != nil {
		if errors.Is(err, ErrSiteIconNotFound) {
			return SiteAppearance{}, ErrSiteIconNotFound
		}
		return SiteAppearance{}, fmt.Errorf("%w: discover sites", ErrSiteIconUnavailable)
	}
	if !eligibleIconSite(site) {
		return SiteAppearance{}, ErrSiteIconNotFound
	}
	icon, err := c.Get(operationContext, id)
	if err == nil || errors.Is(err, ErrSiteIconNotFound) {
		return SiteAppearance{Name: icon.Name}, nil
	}
	return SiteAppearance{}, err
}

func cacheIsFresh(start, end time.Time, ttl time.Duration) bool {
	if start.IsZero() || end.IsZero() {
		return false
	}
	if ttl == 0 {
		return start.Before(end)
	}
	return end.Before(start.Add(ttl))
}

func cachedFailure(value string) error {
	if value == "not_found" {
		return ErrSiteIconNotFound
	}
	return ErrSiteIconUnavailable
}

func (c *IconCache) lookupSite(
	ctx context.Context,
	id string,
) (contract.SiteSummary, bool, error) {
	c.discoveryMu.Lock()
	now := c.now().UTC()
	if !c.discoveredAt.IsZero() && now.Before(c.discoveredAt.Add(c.discoveryTTL)) {
		site, ok := c.discoveredByID[id]
		c.discoveryMu.Unlock()
		if !ok {
			return contract.SiteSummary{}, true, ErrSiteIconNotFound
		}
		return site, true, nil
	}
	if !c.discoveryRetry.IsZero() && now.Before(c.discoveryRetry) {
		site, ok := c.discoveredByID[id]
		discoveryErr := c.discoveryErr
		c.discoveryMu.Unlock()
		if ok {
			return site, false, nil
		}
		if discoveryErr == nil {
			discoveryErr = errors.New("site discovery is cooling down")
		}
		return contract.SiteSummary{}, false, discoveryErr
	}
	call := c.discoveryCall
	if call == nil {
		call = &siteIconDiscoveryCall{done: make(chan struct{})}
		c.discoveryCall = call
		go c.refreshDiscovery(call)
	}
	c.discoveryMu.Unlock()

	select {
	case <-call.done:
	case <-ctx.Done():
		return contract.SiteSummary{}, false, ctx.Err()
	}
	if call.err != nil {
		c.discoveryMu.Lock()
		site, ok := c.discoveredByID[id]
		c.discoveryMu.Unlock()
		if ok {
			return site, false, nil
		}
		return contract.SiteSummary{}, false, call.err
	}
	site, ok := call.items[id]
	if !ok {
		return contract.SiteSummary{}, true, ErrSiteIconNotFound
	}
	return site, true, nil
}

func (c *IconCache) refreshDiscovery(call *siteIconDiscoveryCall) {
	items, err := c.discover()
	now := c.now().UTC()
	next := make(map[string]contract.SiteSummary, len(items))
	if err == nil {
		for _, item := range items {
			if validIconSiteID(item.ID) {
				next[item.ID] = item
			}
		}
	}

	c.discoveryMu.Lock()
	call.items = next
	call.err = err
	if err == nil {
		c.discoveredByID = next
		c.discoveredAt = now
		c.discoveryRetry = time.Time{}
		c.discoveryErr = nil
	} else {
		c.discoveryRetry = now.Add(siteIconDiscoveryFailureTTL)
		c.discoveryErr = err
	}
	if c.discoveryCall == call {
		c.discoveryCall = nil
	}
	close(call.done)
	c.discoveryMu.Unlock()
}

func eligibleIconSite(site contract.SiteSummary) bool {
	if !site.Enabled || !validIconDomain(site.PrimaryDomain) {
		return false
	}
	for _, domain := range site.Domains {
		if strings.EqualFold(domain, site.PrimaryDomain) && validIconDomain(domain) {
			return true
		}
	}
	return false
}

func validIconSiteID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') &&
			(character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validIconDomain(value string) bool {
	value = strings.ToLower(value)
	return value != "" &&
		strings.TrimSpace(value) == value &&
		!strings.HasPrefix(value, "*.") &&
		strings.Contains(value, ".") &&
		net.ParseIP(value) == nil &&
		validDomain(value)
}

func iconSourceKey(site contract.SiteSummary) string {
	scheme := "http"
	if site.TLS.Enabled {
		scheme = "https"
	}
	sum := sha256.Sum256([]byte(scheme + "\n" + strings.ToLower(site.PrimaryDomain)))
	return hex.EncodeToString(sum[:])
}

func (c *IconCache) doRefresh(
	ctx context.Context,
	key string,
	refresh func(context.Context) (SiteIcon, error),
) (SiteIcon, error) {
	c.callsMu.Lock()
	if call, ok := c.calls[key]; ok {
		c.callsMu.Unlock()
		select {
		case <-call.done:
			return call.icon, call.err
		case <-ctx.Done():
			return SiteIcon{}, ctx.Err()
		}
	}
	call := &siteIconCall{done: make(chan struct{})}
	c.calls[key] = call
	c.callsMu.Unlock()

	deadline, ok := ctx.Deadline()
	var refreshContext context.Context
	var cancel context.CancelFunc
	if ok {
		refreshContext, cancel = context.WithDeadline(context.Background(), deadline)
	} else {
		refreshContext, cancel = context.WithTimeout(context.Background(), siteIconFetchTimeout)
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				call.icon = SiteIcon{}
				call.err = ErrSiteIconUnavailable
				slog.Error("site icon refresh panic recovered", "panic", recovered)
			}
			cancel()
			c.callsMu.Lock()
			delete(c.calls, key)
			close(call.done)
			c.callsMu.Unlock()
		}()
		call.icon, call.err = refresh(refreshContext)
	}()

	select {
	case <-call.done:
		return call.icon, call.err
	case <-ctx.Done():
		return SiteIcon{}, ctx.Err()
	}
}

func (c *IconCache) fetch(ctx context.Context, site contract.SiteSummary) (SiteIcon, error) {
	scheme := "http"
	if site.TLS.Enabled {
		scheme = "https"
	}
	base := &url.URL{Scheme: scheme, Host: strings.ToLower(site.PrimaryDomain), Path: "/"}
	allowedHosts := make(map[string]bool, len(site.Domains)+1)
	allowedHosts[base.Hostname()] = true
	for _, domain := range site.Domains {
		domain = strings.ToLower(domain)
		if validIconDomain(domain) {
			allowedHosts[domain] = true
		}
	}

	var transient bool
	page, status, err := c.fetchBytes(ctx, base, siteIconHTMLBytes, "text/html,application/xhtml+xml")
	if err != nil || status >= http.StatusInternalServerError || status == http.StatusTooManyRequests {
		transient = true
	}
	candidates := make([]*url.URL, 0, siteIconMaxLinkedCandidates+1)
	name := ""
	if err == nil && status == http.StatusOK {
		name = parseSiteAppearanceName(page)
		candidates = append(candidates, parseSiteIconLinks(page, base, allowedHosts)...)
	}
	fallback := *base
	fallback.Path = "/favicon.ico"
	fallback.RawPath = ""
	fallback.RawQuery = ""
	candidates = appendUniqueIconURL(candidates, &fallback)

	for _, candidate := range candidates {
		body, candidateStatus, candidateErr := c.fetchBytes(
			ctx,
			candidate,
			siteIconBytes,
			"image/webp,image/png,image/jpeg,image/gif,image/x-icon,*/*;q=0.1",
		)
		if candidateErr != nil {
			if errors.Is(candidateErr, context.Canceled) ||
				errors.Is(candidateErr, context.DeadlineExceeded) {
				transient = true
			}
			continue
		}
		if candidateStatus != http.StatusOK {
			if candidateStatus >= http.StatusInternalServerError ||
				candidateStatus == http.StatusTooManyRequests {
				transient = true
			}
			continue
		}
		contentType, validationErr := validateSiteIcon(body)
		if validationErr != nil {
			continue
		}
		return SiteIcon{ContentType: contentType, Data: body, Name: name}, nil
	}
	if transient {
		return SiteIcon{}, errSiteIconTransient
	}
	return SiteIcon{Name: name}, errSiteIconAbsent
}

func parseSiteAppearanceName(document []byte) string {
	lower := bytes.ToLower(document)
	for offset := 0; offset < len(lower); {
		index := bytes.Index(lower[offset:], []byte("<title"))
		if index < 0 {
			return ""
		}
		start := offset + index
		afterName := start + len("<title")
		if afterName >= len(lower) || !isHTMLSpaceOrTagEnd(lower[afterName]) {
			offset = afterName
			continue
		}
		openingEnd := htmlTagEnd(document, afterName, 2048)
		if openingEnd < 0 {
			return ""
		}
		closingOffset := bytes.Index(lower[openingEnd+1:], []byte("</title"))
		if closingOffset < 0 {
			return ""
		}
		name := string(document[openingEnd+1 : openingEnd+1+closingOffset])
		if !utf8.ValidString(name) {
			return ""
		}
		name = html.UnescapeString(name)
		name = strings.Map(func(character rune) rune {
			if unicode.IsControl(character) && !unicode.IsSpace(character) {
				return -1
			}
			return character
		}, name)
		name = strings.Join(strings.Fields(name), " ")
		runes := []rune(name)
		if len(runes) > siteAppearanceNameRunes {
			name = string(runes[:siteAppearanceNameRunes])
		}
		return name
	}
	return ""
}

func (c *IconCache) fetchBytes(
	ctx context.Context,
	target *url.URL,
	maxBytes int64,
	accept string,
) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), http.NoBody)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", "KPanel-Site-Icon/1")
	response, err := c.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil, response.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, response.StatusCode, err
	}
	if int64(len(body)) > maxBytes {
		return nil, response.StatusCode, errors.New("site icon response exceeds configured limit")
	}
	return body, response.StatusCode, nil
}

func parseSiteIconLinks(
	document []byte,
	base *url.URL,
	allowedHosts map[string]bool,
) []*url.URL {
	lower := bytes.ToLower(document)
	result := make([]*url.URL, 0, siteIconMaxLinkedCandidates)
	for offset := 0; offset < len(lower) && len(result) < siteIconMaxLinkedCandidates; {
		index := bytes.Index(lower[offset:], []byte("<link"))
		if index < 0 {
			break
		}
		start := offset + index
		afterName := start + len("<link")
		if afterName >= len(lower) || !isHTMLSpaceOrTagEnd(lower[afterName]) {
			offset = afterName
			continue
		}
		end := htmlTagEnd(document, afterName, 2048)
		if end < 0 {
			offset = afterName
			continue
		}
		attributes := parseHTMLAttributes(document[afterName:end])
		if siteIconRel(attributes["rel"]) && declaredSiteIconTypeSupported(attributes["type"]) {
			if candidate, ok := resolveSiteIconURL(attributes["href"], base, allowedHosts); ok {
				result = appendUniqueIconURL(result, candidate)
			}
		}
		offset = end + 1
	}
	return result
}

func isHTMLSpaceOrTagEnd(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', '/', '>':
		return true
	default:
		return false
	}
}

func htmlTagEnd(value []byte, start, maxLength int) int {
	endLimit := start + maxLength
	if endLimit > len(value) {
		endLimit = len(value)
	}
	var quote byte
	for index := start; index < endLimit; index++ {
		switch {
		case quote != 0 && value[index] == quote:
			quote = 0
		case quote == 0 && (value[index] == '\'' || value[index] == '"'):
			quote = value[index]
		case quote == 0 && value[index] == '>':
			return index
		}
	}
	return -1
}

func parseHTMLAttributes(value []byte) map[string]string {
	result := make(map[string]string)
	for index := 0; index < len(value); {
		for index < len(value) && (isHTMLSpace(value[index]) || value[index] == '/') {
			index++
		}
		nameStart := index
		for index < len(value) && !isHTMLSpace(value[index]) &&
			value[index] != '=' && value[index] != '/' && value[index] != '>' {
			index++
		}
		if nameStart == index {
			index++
			continue
		}
		name := strings.ToLower(string(value[nameStart:index]))
		for index < len(value) && isHTMLSpace(value[index]) {
			index++
		}
		attributeValue := ""
		if index < len(value) && value[index] == '=' {
			index++
			for index < len(value) && isHTMLSpace(value[index]) {
				index++
			}
			if index < len(value) && (value[index] == '\'' || value[index] == '"') {
				quote := value[index]
				index++
				valueStart := index
				for index < len(value) && value[index] != quote {
					index++
				}
				attributeValue = string(value[valueStart:index])
				if index < len(value) {
					index++
				}
			} else {
				valueStart := index
				for index < len(value) && !isHTMLSpace(value[index]) && value[index] != '>' {
					index++
				}
				attributeValue = string(value[valueStart:index])
			}
		}
		if _, exists := result[name]; !exists {
			result[name] = html.UnescapeString(attributeValue)
		}
	}
	return result
}

func isHTMLSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func siteIconRel(value string) bool {
	hasIcon := false
	for _, token := range strings.Fields(strings.ToLower(value)) {
		if token == "mask-icon" {
			return false
		}
		if token == "icon" || strings.HasSuffix(token, "-icon") {
			hasIcon = true
		}
	}
	return hasIcon
}

func declaredSiteIconTypeSupported(value string) bool {
	value = strings.TrimSpace(strings.ToLower(strings.SplitN(value, ";", 2)[0]))
	if value == "" {
		return true
	}
	if strings.HasPrefix(value, "text/") {
		return false
	}
	switch value {
	case "image/svg+xml", "application/xml", "application/xhtml+xml":
		return false
	default:
		// The HTML type attribute is only advisory and is frequently wrong
		// (for example image/ico or application/octet-stream). The fetched
		// bytes still have to pass validateSiteIcon before they are cached.
		return true
	}
}

func resolveSiteIconURL(
	raw string,
	base *url.URL,
	allowedHosts map[string]bool,
) (*url.URL, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return nil, false
	}
	reference, err := url.Parse(raw)
	if err != nil {
		return nil, false
	}
	candidate := base.ResolveReference(reference)
	candidate.Fragment = ""
	host := strings.ToLower(candidate.Hostname())
	if (candidate.Scheme != "http" && candidate.Scheme != "https") ||
		candidate.User != nil ||
		candidate.Opaque != "" ||
		candidate.Port() != "" ||
		!allowedHosts[host] ||
		len(candidate.String()) > 2048 {
		return nil, false
	}
	if candidate.Path == "" {
		candidate.Path = "/"
	}
	return candidate, true
}

func appendUniqueIconURL(items []*url.URL, candidate *url.URL) []*url.URL {
	value := candidate.String()
	for _, item := range items {
		if item.String() == value {
			return items
		}
	}
	return append(items, candidate)
}

func validateSiteIcon(data []byte) (string, error) {
	if len(data) == 0 || len(data) > siteIconBytes {
		return "", errors.New("invalid site icon size")
	}
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		if err := validateDecodedIcon(data, "png"); err != nil {
			return "", err
		}
		return "image/png", nil
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		if err := validateDecodedIcon(data, "jpeg"); err != nil {
			return "", err
		}
		return "image/jpeg", nil
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		if err := validateDecodedIcon(data, "gif"); err != nil {
			return "", err
		}
		return "image/gif", nil
	case len(data) >= 6 && bytes.Equal(data[:4], []byte{0, 0, 1, 0}):
		if err := validateICO(data); err != nil {
			return "", err
		}
		return "image/vnd.microsoft.icon", nil
	case len(data) >= 16 && bytes.Equal(data[:4], []byte("RIFF")) &&
		bytes.Equal(data[8:12], []byte("WEBP")):
		if err := validateWebP(data); err != nil {
			return "", err
		}
		return "image/webp", nil
	default:
		return "", errors.New("unsupported site icon format")
	}
}

func validateDecodedIcon(data []byte, expectedFormat string) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != expectedFormat {
		return errors.New("invalid site icon image")
	}
	return validateIconDimensions(config.Width, config.Height)
}

func validateIconDimensions(width, height int) error {
	if width < 1 || height < 1 ||
		width > siteIconMaxDimension || height > siteIconMaxDimension ||
		int64(width)*int64(height) > siteIconMaxPixels {
		return errors.New("site icon dimensions exceed configured limit")
	}
	return nil
}

func validateICO(data []byte) error {
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count < 1 || count > 64 || 6+count*16 > len(data) {
		return errors.New("invalid ICO directory")
	}
	for index := 0; index < count; index++ {
		offset := 6 + index*16
		width := int(data[offset])
		height := int(data[offset+1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		size := int64(binary.LittleEndian.Uint32(data[offset+8 : offset+12]))
		imageOffset := int64(binary.LittleEndian.Uint32(data[offset+12 : offset+16]))
		if validateIconDimensions(width, height) != nil ||
			size < 1 || imageOffset < int64(6+count*16) ||
			imageOffset+size > int64(len(data)) {
			return errors.New("invalid ICO image entry")
		}
		payload := data[int(imageOffset):int(imageOffset+size)]
		if err := validateICOEntry(payload, width, height); err != nil {
			return err
		}
	}
	return nil
}

func validateICOEntry(data []byte, directoryWidth, directoryHeight int) error {
	if bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || format != "png" ||
			validateIconDimensions(config.Width, config.Height) != nil ||
			config.Width != directoryWidth || config.Height != directoryHeight {
			return errors.New("invalid embedded ICO PNG")
		}
		return nil
	}
	if len(data) < 12 {
		return errors.New("invalid ICO bitmap header")
	}
	headerSize := int64(binary.LittleEndian.Uint32(data[:4]))
	if headerSize >= int64(len(data)) {
		return errors.New("invalid ICO bitmap header")
	}
	var width, doubledHeight int64
	switch headerSize {
	case 12:
		width = int64(binary.LittleEndian.Uint16(data[4:6]))
		doubledHeight = int64(binary.LittleEndian.Uint16(data[6:8]))
	default:
		if headerSize < 40 || len(data) < 12 {
			return errors.New("unsupported ICO bitmap header")
		}
		width = int64(int32(binary.LittleEndian.Uint32(data[4:8])))
		doubledHeight = int64(int32(binary.LittleEndian.Uint32(data[8:12])))
		if doubledHeight < 0 {
			doubledHeight = -doubledHeight
		}
	}
	if width < 1 || doubledHeight < 2 || doubledHeight%2 != 0 {
		return errors.New("invalid ICO bitmap dimensions")
	}
	height := doubledHeight / 2
	if width > int64(^uint(0)>>1) || height > int64(^uint(0)>>1) ||
		validateIconDimensions(int(width), int(height)) != nil ||
		int(width) != directoryWidth || int(height) != directoryHeight {
		return errors.New("ICO bitmap dimensions do not match its directory")
	}
	return nil
}

func validateWebP(data []byte) error {
	if len(data) < 30 || int64(binary.LittleEndian.Uint32(data[4:8]))+8 > int64(len(data)) {
		return errors.New("invalid WebP container")
	}
	var width, height int
	switch string(data[12:16]) {
	case "VP8X":
		width = 1 + int(data[24]) + int(data[25])<<8 + int(data[26])<<16
		height = 1 + int(data[27]) + int(data[28])<<8 + int(data[29])<<16
	case "VP8L":
		if data[20] != 0x2f {
			return errors.New("invalid lossless WebP header")
		}
		bits := binary.LittleEndian.Uint32(data[21:25])
		width = 1 + int(bits&0x3fff)
		height = 1 + int((bits>>14)&0x3fff)
	case "VP8 ":
		if !bytes.Equal(data[23:26], []byte{0x9d, 0x01, 0x2a}) {
			return errors.New("invalid lossy WebP header")
		}
		width = int(binary.LittleEndian.Uint16(data[26:28]) & 0x3fff)
		height = int(binary.LittleEndian.Uint16(data[28:30]) & 0x3fff)
	default:
		return errors.New("unsupported WebP bitstream")
	}
	return validateIconDimensions(width, height)
}

func (c *IconCache) readCache(
	id string,
	sourceKey string,
) (siteIconCacheValue, bool, bool) {
	metadataData, err := readRegularFile(c.metadataPath(id), siteIconMetadataBytes)
	if err != nil {
		return siteIconCacheValue{}, false, false
	}
	var metadata siteIconMetadata
	decoder := json.NewDecoder(bytes.NewReader(metadataData))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&metadata) != nil ||
		metadata.Version != siteIconCacheVersion ||
		metadata.SourceKey != sourceKey ||
		(!metadata.FetchedAt.IsZero() && metadata.FetchedAt.After(c.now().UTC().Add(5*time.Minute))) {
		return siteIconCacheValue{}, false, false
	}
	value := siteIconCacheValue{meta: metadata, icon: SiteIcon{Name: metadata.Name}}
	iconData, err := readRegularFile(c.iconPath(id), siteIconBytes)
	if err != nil || metadata.ContentSHA256 == "" ||
		!strings.EqualFold(metadata.ContentSHA256, hashSiteIcon(iconData)) {
		return value, false, true
	}
	contentType, err := validateSiteIcon(iconData)
	if err != nil {
		return value, false, true
	}
	value.icon = SiteIcon{ContentType: contentType, Data: iconData, Name: metadata.Name}
	return value, true, true
}

func (c *IconCache) writeSuccess(
	id string,
	icon SiteIcon,
	metadata siteIconMetadata,
) error {
	if _, err := validateSiteIcon(icon.Data); err != nil {
		return err
	}
	metadata.ContentSHA256 = hashSiteIcon(icon.Data)
	if err := writeAtomicPrivateFile(c.dir, c.iconPath(id), icon.Data); err != nil {
		return err
	}
	if err := c.writeMetadata(id, metadata); err != nil {
		return err
	}
	return nil
}

func (c *IconCache) writeMetadata(id string, metadata siteIconMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if len(data) > siteIconMetadataBytes {
		return errors.New("site icon metadata exceeds configured limit")
	}
	if err := writeAtomicPrivateFile(c.dir, c.metadataPath(id), data); err != nil {
		return err
	}
	c.prune()
	return nil
}

func writeAtomicPrivateFile(dir, target string, data []byte) error {
	file, err := os.CreateTemp(dir, ".site-icon-*")
	if err != nil {
		return err
	}
	tempPath := file.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, target); err != nil {
		return err
	}
	if directory, openErr := os.Open(dir); openErr == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func (c *IconCache) iconPath(id string) string {
	return filepath.Join(c.dir, id+".icon")
}

func (c *IconCache) metadataPath(id string) string {
	return filepath.Join(c.dir, id+".meta.json")
}

func hashSiteIcon(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type siteIconCacheEntry struct {
	id        string
	fetchedAt time.Time
	bytes     int64
}

func (c *IconCache) prune() {
	c.pruneMu.Lock()
	defer c.pruneMu.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	now := c.now().UTC()
	recordsByID := make(map[string]*siteIconCacheEntry)
	records := make([]siteIconCacheEntry, 0)
	var total int64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 ||
			!strings.HasSuffix(name, ".meta.json") {
			continue
		}
		id := strings.TrimSuffix(name, ".meta.json")
		if !validIconSiteID(id) {
			continue
		}
		record := &siteIconCacheEntry{id: id}
		if metadataData, readErr := readRegularFile(c.metadataPath(id), siteIconMetadataBytes); readErr == nil {
			var metadata siteIconMetadata
			if json.Unmarshal(metadataData, &metadata) == nil {
				record.fetchedAt = metadata.FetchedAt
				if record.fetchedAt.IsZero() {
					record.fetchedAt = metadata.RetryAt
				}
			}
		}
		recordsByID[id] = record
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			continue
		}
		if strings.HasPrefix(name, ".site-icon-") {
			if info.ModTime().Before(now.Add(-siteIconOrphanGrace)) {
				removeRegularCacheFile(filepath.Join(c.dir, name))
			}
			continue
		}
		if !strings.HasSuffix(name, ".icon") {
			continue
		}
		id := strings.TrimSuffix(name, ".icon")
		if !validIconSiteID(id) {
			continue
		}
		record, ok := recordsByID[id]
		if !ok {
			if info.ModTime().Before(now.Add(-siteIconOrphanGrace)) {
				removeRegularCacheFile(c.iconPath(id))
			}
			continue
		}
		record.bytes = info.Size()
		total += record.bytes
		if record.fetchedAt.IsZero() {
			record.fetchedAt = info.ModTime()
		}
	}
	for _, record := range recordsByID {
		records = append(records, *record)
	}
	sort.Slice(records, func(left, right int) bool {
		return records[left].fetchedAt.Before(records[right].fetchedAt)
	})
	for len(records) > c.maxEntries || total > c.maxCacheBytes {
		record := records[0]
		records = records[1:]
		removeRegularCacheFile(c.iconPath(record.id))
		removeRegularCacheFile(c.metadataPath(record.id))
		total -= record.bytes
	}
}

func removeRegularCacheFile(path string) {
	info, err := os.Lstat(path)
	if err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		_ = os.Remove(path)
	}
}
