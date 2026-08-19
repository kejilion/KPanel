package panel

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/store"
)

const (
	clusterShareAPIPrefix    = "/api/v1/public/cluster-share/"
	clusterSharePagePrefix   = "/share/"
	clusterShareCacheTTL     = 10 * time.Second
	clusterShareRateWindow   = time.Minute
	clusterShareRateLimit    = 120
	clusterShareRateKeys     = 2048
	defaultClusterShareTitle = "我的 KPanel 集群"
)

type clusterShareSettingsInput struct {
	Enabled                 bool   `json:"enabled"`
	Title                   string `json:"title"`
	Description             string `json:"description"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

type clusterShareTokenInput struct {
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

type clusterShareSettingsResponse struct {
	Enabled         bool      `json:"enabled"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	SharePath       string    `json:"sharePath,omitempty"`
	ResourceVersion string    `json:"resourceVersion"`
	UpdatedAt       time.Time `json:"updatedAt,omitempty"`
}

type publicClusterShareSnapshot struct {
	Title       string                   `json:"title"`
	Description string                   `json:"description,omitempty"`
	GeneratedAt time.Time                `json:"generatedAt"`
	Total       int                      `json:"total"`
	Online      int                      `json:"online"`
	Attention   int                      `json:"attention"`
	Items       []publicClusterShareHost `json:"items"`
}

type publicClusterShareHost struct {
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	State         string                     `json:"state"`
	OS            string                     `json:"os,omitempty"`
	Architecture  string                     `json:"architecture,omitempty"`
	UptimeSeconds uint64                     `json:"uptimeSeconds,omitempty"`
	Load          publicClusterShareLoad     `json:"load,omitempty"`
	CPU           publicClusterShareCPU      `json:"cpu,omitempty"`
	Memory        publicClusterShareCapacity `json:"memory,omitempty"`
	Disk          publicClusterShareCapacity `json:"disk,omitempty"`
	Network       publicClusterShareNetwork  `json:"network,omitempty"`
	Location      publicClusterShareLocation `json:"location,omitempty"`
	CollectedAt   *time.Time                 `json:"collectedAt,omitempty"`
}

type publicClusterShareLoad struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

type publicClusterShareCPU struct {
	Cores        int     `json:"cores"`
	UsagePercent float64 `json:"usagePercent"`
}

type publicClusterShareCapacity struct {
	TotalBytes   uint64  `json:"totalBytes"`
	UsedBytes    uint64  `json:"usedBytes"`
	UsagePercent float64 `json:"usagePercent"`
}

type publicClusterShareNetwork struct {
	ReceiveBytesPerSecond  float64 `json:"receiveBytesPerSecond"`
	TransmitBytesPerSecond float64 `json:"transmitBytesPerSecond"`
}

type publicClusterShareLocation struct {
	ISP         string `json:"isp,omitempty"`
	Country     string `json:"country,omitempty"`
	CountryCode string `json:"countryCode,omitempty"`
	Region      string `json:"region,omitempty"`
	City        string `json:"city,omitempty"`
}

type clusterShareCacheEntry struct {
	resourceVersion string
	expiresAt       time.Time
	value           publicClusterShareSnapshot
}

type clusterShareRateEntry struct {
	startedAt time.Time
	count     int
}

func (s *Server) handleClusterShareSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_cluster_share_request", "Invalid cluster share request", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		value, version := s.store.ClusterShare()
		s.writeJSON(w, http.StatusOK, clusterShareSettingsView(value, version))
	case http.MethodPut:
		s.handleClusterShareUpdate(w, r)
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func (s *Server) handleClusterShareUpdate(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input clusterShareSettingsInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	title, ok := clusterShareText(input.Title, 80)
	if !ok {
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "cluster_share_title_invalid", "Cluster share title is invalid", "")
		return
	}
	description, ok := clusterShareText(input.Description, 240)
	if !ok {
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "cluster_share_description_invalid", "Cluster share description is invalid", "")
		return
	}
	if title == "" {
		title = defaultClusterShareTitle
	}
	current, _ := s.store.ClusterShare()
	token := current.Token
	if input.Enabled && token == "" {
		var err error
		token, err = newClusterShareToken()
		if err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "cluster_share_token_unavailable", "Cluster share token unavailable", "")
			return
		}
	}
	change := map[string]any{
		"enabled": input.Enabled, "title": title, "description": description,
		"tokenCreated": current.Token == "" && token != "",
	}
	if err := s.audit(r, session.User.ID, "cluster.share.update", "cluster-share", "public-page", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	value := store.ClusterShare{
		Enabled: input.Enabled, Token: token, Title: title, Description: description,
		UpdatedAt: time.Now().UTC(),
	}
	if err := s.store.ReplaceClusterShare(input.ExpectedResourceVersion, value); err != nil {
		_ = s.audit(r, session.User.ID, "cluster.share.update", "cluster-share", "public-page", "failure", change)
		s.writeClusterShareStoreError(w, r, err)
		return
	}
	s.invalidateClusterShareCache()
	_ = s.audit(r, session.User.ID, "cluster.share.update", "cluster-share", "public-page", "success", change)
	_, version := s.store.ClusterShare()
	s.writeJSON(w, http.StatusOK, clusterShareSettingsView(value, version))
}

func (s *Server) handleClusterShareTokenReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input clusterShareTokenInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	token, err := newClusterShareToken()
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "cluster_share_token_unavailable", "Cluster share token unavailable", "")
		return
	}
	change := map[string]any{"tokenRotated": true}
	if err := s.audit(r, session.User.ID, "cluster.share.token.rotate", "cluster-share", "public-page", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	current, _ := s.store.ClusterShare()
	current.Token = token
	if strings.TrimSpace(current.Title) == "" {
		current.Title = defaultClusterShareTitle
	}
	current.UpdatedAt = time.Now().UTC()
	if err := s.store.ReplaceClusterShare(input.ExpectedResourceVersion, current); err != nil {
		_ = s.audit(r, session.User.ID, "cluster.share.token.rotate", "cluster-share", "public-page", "failure", change)
		s.writeClusterShareStoreError(w, r, err)
		return
	}
	s.invalidateClusterShareCache()
	_ = s.audit(r, session.User.ID, "cluster.share.token.rotate", "cluster-share", "public-page", "success", change)
	_, version := s.store.ClusterShare()
	s.writeJSON(w, http.StatusOK, clusterShareSettingsView(current, version))
}

func (s *Server) handlePublicClusterShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.RawPath != "" || r.URL.RawQuery != "" || !isPublicClusterShareAPIPath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if !s.allowClusterShareRequest(s.remoteIP(r), time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		s.writeProblem(w, r, http.StatusTooManyRequests, "cluster_share_rate_limited", "Cluster share rate limited", "")
		return
	}
	token := strings.TrimPrefix(r.URL.Path, clusterShareAPIPrefix)
	value, version := s.store.ClusterShare()
	if !value.Enabled || value.Token == "" || !secureStringEqual(token, value.Token) {
		http.NotFound(w, r)
		return
	}
	s.writeJSON(w, http.StatusOK, s.clusterShareSnapshot(r.Context(), value, version))
}

func clusterShareSettingsView(value store.ClusterShare, version string) clusterShareSettingsResponse {
	response := clusterShareSettingsResponse{
		Enabled: value.Enabled, Title: value.Title, Description: value.Description,
		ResourceVersion: version, UpdatedAt: value.UpdatedAt,
	}
	if value.Token != "" {
		response.SharePath = clusterSharePagePrefix + value.Token
	}
	return response
}

func (s *Server) writeClusterShareStoreError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrConflict) {
		s.writeProblem(w, r, http.StatusConflict, "cluster_share_changed", "Cluster share settings changed", "")
		return
	}
	s.writeProblem(w, r, http.StatusServiceUnavailable, "cluster_share_storage_unavailable", "Cluster share storage unavailable", "")
}

func newClusterShareToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func clusterShareText(raw string, maxRunes int) (string, bool) {
	value := strings.TrimSpace(raw)
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxRunes || len(value) > maxRunes*4 {
		return "", false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", false
		}
	}
	return value, true
}

func isClusterSharePagePath(path string) bool {
	return validClusterSharePath(path, clusterSharePagePrefix)
}

func isPublicClusterShareAPIPath(path string) bool {
	return validClusterSharePath(path, clusterShareAPIPrefix)
}

func validClusterSharePath(path, prefix string) bool {
	token := strings.TrimPrefix(path, prefix)
	if token == path || len(token) != 64 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}

func (s *Server) clusterShareSnapshot(ctx context.Context, value store.ClusterShare, resourceVersion string) publicClusterShareSnapshot {
	now := time.Now().UTC()
	s.clusterShareMu.Lock()
	defer s.clusterShareMu.Unlock()
	if s.clusterShareCache.resourceVersion == resourceVersion && now.Before(s.clusterShareCache.expiresAt) {
		return s.clusterShareCache.value
	}
	inventory := s.cluster.Hosts(ctx)
	result := publicClusterShareSnapshot{
		Title: value.Title, Description: value.Description, GeneratedAt: now,
		Items: make([]publicClusterShareHost, 0, len(inventory.Items)),
	}
	if result.Title == "" {
		result.Title = defaultClusterShareTitle
	}
	for _, host := range inventory.Items {
		item := publicClusterShareHost{
			ID: publicClusterShareHostID(value.Token, host.ID), Name: host.Name,
			State: publicClusterShareState(host.State),
		}
		if item.State == "online" {
			result.Online++
		} else if item.State != "pending" {
			result.Attention++
		}
		if host.LastSnapshot != nil {
			snapshot := host.LastSnapshot
			telemetry := snapshot.Telemetry
			collectedAt := telemetry.CollectedAt
			item.OS = telemetry.OS
			item.Architecture = telemetry.Architecture
			item.UptimeSeconds = telemetry.UptimeSeconds
			item.Load = publicClusterShareLoad{
				One: telemetry.Load.One, Five: telemetry.Load.Five, Fifteen: telemetry.Load.Fifteen,
			}
			item.CPU = publicClusterShareCPU{Cores: telemetry.CPU.Cores, UsagePercent: telemetry.CPU.UsagePercent}
			item.Memory = publicClusterShareCapacity{
				TotalBytes: telemetry.Memory.TotalBytes, UsedBytes: telemetry.Memory.UsedBytes,
				UsagePercent: telemetry.Memory.UsagePercent,
			}
			item.Disk = publicClusterShareCapacity{
				TotalBytes: telemetry.Disk.TotalBytes, UsedBytes: telemetry.Disk.UsedBytes,
				UsagePercent: telemetry.Disk.UsagePercent,
			}
			item.Network = publicClusterShareNetwork{
				ReceiveBytesPerSecond:  snapshot.ReceiveBytesPerSecond,
				TransmitBytesPerSecond: snapshot.TransmitBytesPerSecond,
			}
			item.Location = publicClusterShareLocation{
				ISP: telemetry.PublicNetwork.ISP, Country: telemetry.PublicNetwork.Country,
				CountryCode: telemetry.PublicNetwork.CountryCode, Region: telemetry.PublicNetwork.Region,
				City: telemetry.PublicNetwork.City,
			}
			item.CollectedAt = &collectedAt
		}
		result.Items = append(result.Items, item)
	}
	result.Total = len(result.Items)
	s.clusterShareCache = clusterShareCacheEntry{
		resourceVersion: resourceVersion, expiresAt: now.Add(clusterShareCacheTTL), value: result,
	}
	return result
}

func publicClusterShareState(state cluster.HostState) string {
	switch state {
	case cluster.HostOnline:
		return "online"
	case cluster.HostDegraded, cluster.HostStale:
		return "degraded"
	case cluster.HostUnknown, cluster.HostPairing:
		return "pending"
	default:
		return "offline"
	}
}

func publicClusterShareHostID(token, hostID string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte("kpanel-public-cluster-host\x00" + hostID))
	return "host-" + hex.EncodeToString(mac.Sum(nil)[:12])
}

func (s *Server) invalidateClusterShareCache() {
	s.clusterShareMu.Lock()
	s.clusterShareCache = clusterShareCacheEntry{}
	s.clusterShareMu.Unlock()
}

func (s *Server) allowClusterShareRequest(key string, now time.Time) bool {
	s.clusterShareRateMu.Lock()
	defer s.clusterShareRateMu.Unlock()
	if s.clusterShareRates == nil {
		s.clusterShareRates = make(map[string]clusterShareRateEntry)
	}
	entry, exists := s.clusterShareRates[key]
	if exists && now.Sub(entry.startedAt) >= clusterShareRateWindow {
		entry = clusterShareRateEntry{}
		exists = false
	}
	if !exists && len(s.clusterShareRates) >= clusterShareRateKeys {
		for existingKey, existing := range s.clusterShareRates {
			if now.Sub(existing.startedAt) >= clusterShareRateWindow {
				delete(s.clusterShareRates, existingKey)
			}
		}
		if len(s.clusterShareRates) >= clusterShareRateKeys {
			return false
		}
	}
	if !exists {
		entry = clusterShareRateEntry{startedAt: now}
	}
	if entry.count >= clusterShareRateLimit {
		return false
	}
	entry.count++
	s.clusterShareRates[key] = entry
	return true
}
