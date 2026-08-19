package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/store"
)

type clusterShareTelemetrySource struct {
	value contract.HostTelemetry
}

func (s clusterShareTelemetrySource) Telemetry(context.Context) (contract.HostTelemetry, error) {
	return s.value, nil
}

func TestClusterShareLifecycleRedactsPrivateFieldsAndBypassesSecurityEntrance(t *testing.T) {
	server, tokenPath := newTestServerWithPublicURL(t, "https://panel.test")
	now := time.Now().UTC().Truncate(time.Second)
	if err := server.cluster.Close(); err != nil {
		t.Fatal(err)
	}
	sharedCluster, err := cluster.NewService(cluster.ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "cluster"), PanelVersion: "secret-panel-version",
		PublicURL: "https://panel.test", Hostname: "Showcase node",
		Telemetry: clusterShareTelemetrySource{value: contract.HostTelemetry{
			AgentVersion: "secret-agent-version", AgentProtocolVersion: "secret-protocol",
			Hostname: "Showcase node", OS: "Debian GNU/Linux 13", OSID: "debian",
			Kernel: "secret-kernel", Architecture: "x86_64", UptimeSeconds: 7_200,
			Load:    contract.LoadSummary{One: 0.2, Five: 0.3, Fifteen: 0.4},
			CPU:     contract.CPUSummary{Model: "secret-cpu-model", Cores: 4, UsagePercent: 12.5},
			Memory:  contract.MemorySummary{TotalBytes: 8 << 30, AvailableBytes: 5 << 30, UsedBytes: 3 << 30, UsagePercent: 37.5},
			Disk:    contract.DiskCapacitySummary{TotalBytes: 100 << 30, UsedBytes: 25 << 30, UsagePercent: 25},
			Network: contract.NetworkSummary{ReceivedBytes: 123456, SentBytes: 654321, TCPConnections: 17, UDPConnections: 3},
			PublicNetwork: contract.PublicNetworkSummary{
				IPv4: "203.0.113.10", IPv6: "2001:db8::10", ISP: "Example ISP",
				Country: "Singapore", CountryCode: "SG", Region: "Central", City: "Singapore",
				Timezone: "secret-timezone", Source: "secret-source", UpdatedAt: &now,
			},
			CollectedAt: now,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	server.cluster = sharedCluster

	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/cluster/share", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated settings status = %d", unauthenticated.Code)
	}
	sessionCookie, csrfCookie := bootstrapCookiesForOrigin(t, server, tokenPath, "https://panel.test")
	settingsResponse := authenticatedRequest(
		server, http.MethodGet, "/api/v1/cluster/share", nil, sessionCookie, csrfCookie, nil,
	)
	if settingsResponse.Code != http.StatusOK {
		t.Fatalf("settings status = %d; body=%s", settingsResponse.Code, settingsResponse.Body.String())
	}
	var settings clusterShareSettingsResponse
	if err := json.Unmarshal(settingsResponse.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}

	updateBody, _ := json.Marshal(clusterShareSettingsInput{
		Enabled: true, Title: "My global fleet", Description: "A small corner of the internet.",
		ExpectedResourceVersion: settings.ResourceVersion,
	})
	missingCSRF := authenticatedRequest(
		server, http.MethodPut, "/api/v1/cluster/share", updateBody,
		sessionCookie, csrfCookie, map[string]string{"Content-Type": "application/json", "Origin": "https://panel.test"},
	)
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("share update without CSRF = %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}
	updated := authenticatedRequest(
		server, http.MethodPut, "/api/v1/cluster/share", updateBody,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "https://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if updated.Code != http.StatusOK {
		t.Fatalf("share update status = %d; body=%s", updated.Code, updated.Body.String())
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &settings); err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled || !isClusterSharePagePath(settings.SharePath) {
		t.Fatalf("unexpected enabled settings %#v", settings)
	}
	shareToken := strings.TrimPrefix(settings.SharePath, clusterSharePagePrefix)
	publicResponse := performRequest(server, http.MethodGet, clusterShareAPIPrefix+shareToken, nil, nil)
	if publicResponse.Code != http.StatusOK || publicResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("public share status = %d cache=%q body=%s", publicResponse.Code, publicResponse.Header().Get("Cache-Control"), publicResponse.Body.String())
	}
	var snapshot publicClusterShareSnapshot
	if err := json.Unmarshal(publicResponse.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != 1 || snapshot.Online != 1 || len(snapshot.Items) != 1 {
		t.Fatalf("unexpected public snapshot %#v", snapshot)
	}
	item := snapshot.Items[0]
	if item.ID == cluster.LocalHostID || item.Name != "Showcase node" || item.OS != "Debian GNU/Linux 13" || item.Location.ISP != "Example ISP" {
		t.Fatalf("unexpected public host %#v", item)
	}
	serialized := publicResponse.Body.String()
	for _, privateValue := range []string{
		"203.0.113.10", "2001:db8::10", "secret-panel-version",
		"secret-agent-version", "secret-protocol", "secret-kernel", "secret-cpu-model",
		"secret-timezone", "secret-source", shareToken,
	} {
		if strings.Contains(serialized, privateValue) {
			t.Fatalf("private value %q leaked in public snapshot: %s", privateValue, serialized)
		}
	}
	var publicPayload map[string]json.RawMessage
	if err := json.Unmarshal(publicResponse.Body.Bytes(), &publicPayload); err != nil {
		t.Fatal(err)
	}
	assertClusterShareJSONKeys(t, publicPayload,
		"title", "description", "generatedAt", "total", "online", "attention", "items",
	)
	var publicItems []map[string]json.RawMessage
	if err := json.Unmarshal(publicPayload["items"], &publicItems); err != nil {
		t.Fatal(err)
	}
	if len(publicItems) != 1 {
		t.Fatalf("public item count = %d, want 1", len(publicItems))
	}
	assertClusterShareJSONKeys(t, publicItems[0],
		"id", "name", "state", "os", "architecture", "uptimeSeconds", "load", "cpu",
		"memory", "disk", "network", "location", "collectedAt",
	)
	for field, allowed := range map[string][]string{
		"load":     {"one", "five", "fifteen"},
		"cpu":      {"cores", "usagePercent"},
		"memory":   {"totalBytes", "usedBytes", "usagePercent"},
		"disk":     {"totalBytes", "usedBytes", "usagePercent"},
		"network":  {"receiveBytesPerSecond", "transmitBytesPerSecond"},
		"location": {"isp", "country", "countryCode", "region", "city"},
	} {
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(publicItems[0][field], &nested); err != nil {
			t.Fatalf("decode public %s: %v", field, err)
		}
		assertClusterShareJSONKeys(t, nested, allowed...)
	}

	entrance, entranceVersion := server.store.SecurityEntrance()
	entrance.Enabled = true
	entrance.Path = "panel-private-entry"
	entrance.UpdatedAt = time.Now().UTC()
	if err := server.store.ReplaceSecurityEntrance(entranceVersion, entrance); err != nil {
		t.Fatal(err)
	}
	if !securityEntrancePublicPath(settings.SharePath) {
		t.Fatalf("share page %q is not exempt from the security entrance", settings.SharePath)
	}
	publicBehindEntrance := performRequest(server, http.MethodGet, clusterShareAPIPrefix+shareToken, nil, nil)
	if publicBehindEntrance.Code != http.StatusOK {
		t.Fatalf("share API behind security entrance = %d %s", publicBehindEntrance.Code, publicBehindEntrance.Body.String())
	}

	resetBody, _ := json.Marshal(clusterShareTokenInput{ExpectedResourceVersion: settings.ResourceVersion})
	reset := authenticatedRequest(
		server, http.MethodPost, "/api/v1/cluster/share/token", resetBody,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "https://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if reset.Code != http.StatusOK {
		t.Fatalf("token reset status = %d; body=%s", reset.Code, reset.Body.String())
	}
	var rotated clusterShareSettingsResponse
	if err := json.Unmarshal(reset.Body.Bytes(), &rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.SharePath == settings.SharePath {
		t.Fatal("share token did not rotate")
	}
	oldLink := performRequest(server, http.MethodGet, clusterShareAPIPrefix+shareToken, nil, nil)
	if oldLink.Code != http.StatusNotFound {
		t.Fatalf("old share link status = %d, want 404", oldLink.Code)
	}

	events, _ := server.store.ListAudit(200, "")
	auditJSON, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	newShareToken := strings.TrimPrefix(rotated.SharePath, clusterSharePagePrefix)
	if strings.Contains(string(auditJSON), shareToken) || strings.Contains(string(auditJSON), newShareToken) {
		t.Fatalf("share token leaked into audit: %s", auditJSON)
	}

	disableBody, _ := json.Marshal(clusterShareSettingsInput{
		Enabled: false, Title: rotated.Title, Description: rotated.Description,
		ExpectedResourceVersion: rotated.ResourceVersion,
	})
	disabled := authenticatedRequest(
		server, http.MethodPut, "/api/v1/cluster/share", disableBody,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "https://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status = %d; body=%s", disabled.Code, disabled.Body.String())
	}
	disabledLink := performRequest(server, http.MethodGet, clusterShareAPIPrefix+newShareToken, nil, nil)
	if disabledLink.Code != http.StatusNotFound {
		t.Fatalf("disabled share status = %d, want 404", disabledLink.Code)
	}
}

func TestClusterSharePublicPathValidation(t *testing.T) {
	token := strings.Repeat("a", 64)
	for _, path := range []string{
		clusterSharePagePrefix + token,
		clusterShareAPIPrefix + token,
	} {
		if !securityEntrancePublicPath(path) {
			t.Fatalf("valid public path %q was rejected", path)
		}
	}
	for _, path := range []string{
		clusterSharePagePrefix,
		clusterSharePagePrefix + strings.Repeat("a", 63),
		clusterSharePagePrefix + strings.Repeat("a", 65),
		clusterSharePagePrefix + strings.Repeat("g", 64),
		clusterSharePagePrefix + token + "/extra",
		clusterShareAPIPrefix,
		clusterShareAPIPrefix + strings.Repeat("a", 63),
		clusterShareAPIPrefix + strings.Repeat("a", 65),
		clusterShareAPIPrefix + strings.Repeat("g", 64),
		clusterShareAPIPrefix + token + "/extra",
	} {
		if securityEntrancePublicPath(path) {
			t.Fatalf("invalid public path %q bypassed the security entrance", path)
		}
	}

	server, _ := newTestServer(t)
	response := performRequest(server, http.MethodGet, clusterShareAPIPrefix+token+"?unexpected=true", nil, nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("public API query status = %d, want 404", response.Code)
	}
}

func TestClusterShareRateLimiterIsBoundedAndResets(t *testing.T) {
	server := &Server{}
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	for range clusterShareRateLimit {
		if !server.allowClusterShareRequest("198.51.100.1", now) {
			t.Fatal("request was limited before the configured per-window limit")
		}
	}
	if server.allowClusterShareRequest("198.51.100.1", now) {
		t.Fatal("request above the configured per-window limit was allowed")
	}
	if !server.allowClusterShareRequest("198.51.100.1", now.Add(clusterShareRateWindow)) {
		t.Fatal("rate limit did not reset after its window")
	}

	server = &Server{clusterShareRates: make(map[string]clusterShareRateEntry)}
	for index := range clusterShareRateKeys {
		key := time.Unix(int64(index), 0).String()
		if !server.allowClusterShareRequest(key, now) {
			t.Fatalf("subject %d was rejected before the subject capacity", index)
		}
	}
	if server.allowClusterShareRequest("overflow", now) {
		t.Fatal("rate limiter accepted an unbounded subject")
	}
}

func TestClusterShareStoreConflictIsReported(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	_, staleVersion := server.store.ClusterShare()
	if err := server.store.ReplaceClusterShare(staleVersion, store.ClusterShare{Title: "changed", UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(clusterShareSettingsInput{
		Enabled: true, Title: "stale", ExpectedResourceVersion: staleVersion,
	})
	response := authenticatedRequest(
		server, http.MethodPut, "/api/v1/cluster/share", body,
		sessionCookie, csrfCookie, map[string]string{
			"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
		},
	)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "cluster_share_changed") {
		t.Fatalf("stale share update = %d %s", response.Code, response.Body.String())
	}
}

func assertClusterShareJSONKeys(t *testing.T, object map[string]json.RawMessage, expected ...string) {
	t.Helper()
	if len(object) != len(expected) {
		t.Fatalf("JSON keys = %v, want exactly %v", object, expected)
	}
	for _, key := range expected {
		if _, ok := object[key]; !ok {
			t.Fatalf("JSON key %q is missing from %v", key, object)
		}
	}
}
