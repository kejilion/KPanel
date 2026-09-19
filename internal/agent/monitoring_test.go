package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

type fakeMonitoringProvider struct {
	rangeValue string
	start      time.Time
	end        time.Time
	result     contract.MonitoringHistory
	err        error
}

type fakeMonitoringChecksProvider struct {
	*fakeMonitoringProvider
	snapshot monitoring.CheckSnapshot
	input    monitoring.ReplaceChecksInput
	err      error
}

func (provider *fakeMonitoringChecksProvider) Checks() monitoring.CheckSnapshot {
	return provider.snapshot
}

func (provider *fakeMonitoringChecksProvider) ReplaceChecks(input monitoring.ReplaceChecksInput) (monitoring.CheckSnapshot, error) {
	provider.input = input
	return provider.snapshot, provider.err
}

func (provider *fakeMonitoringProvider) HistoryBetween(
	_ context.Context,
	rangeValue string,
	start time.Time,
	end time.Time,
) (contract.MonitoringHistory, error) {
	provider.rangeValue = rangeValue
	provider.start = start
	provider.end = end
	return provider.result, provider.err
}

func (provider *fakeMonitoringProvider) History(
	_ context.Context,
	rangeValue string,
) (contract.MonitoringHistory, error) {
	provider.rangeValue = rangeValue
	return provider.result, provider.err
}

func TestMonitoringHistoryRequiresAuthAndStrictRange(t *testing.T) {
	server := testServer(t)
	provider := &fakeMonitoringProvider{
		result: contract.MonitoringHistory{Range: "6h"},
	}
	server.monitoring = provider

	request := httptest.NewRequest(http.MethodGet, "/v1/monitoring/history?range=6h", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Code)
	}

	for _, path := range []string{
		"/v1/monitoring/history?range=31d",
		"/v1/monitoring/history?range=6h&range=24h",
		"/v1/monitoring/history?range=6h&unknown=1",
		"/v1/monitoring/history?range=6h&start=2026-08-05T00%3A00%3A00Z",
		"/v1/monitoring/history?range=6h&start=invalid&end=2026-08-05T01%3A00%3A00Z",
		"/v1/monitoring/history?range=6h&start=2026-08-05T00%3A00%3A00Z&start=2026-08-05T00%3A30%3A00Z&end=2026-08-05T01%3A00%3A00Z",
	} {
		request = httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		response = httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid query %q status = %d body=%s", path, response.Code, response.Body.String())
		}
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/monitoring/history?range=6h", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || provider.rangeValue != "6h" {
		t.Fatalf("history status=%d range=%q body=%s", response.Code, provider.rangeValue, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/monitoring/history?range=30d", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || provider.rangeValue != "30d" {
		t.Fatalf("30-day history status=%d range=%q body=%s", response.Code, provider.rangeValue, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/monitoring/history?range=12m", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || provider.rangeValue != "12m" {
		t.Fatalf("12-month history status=%d range=%q body=%s", response.Code, provider.rangeValue, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/monitoring/history?range=12m&start=2026-08-04T00%3A00%3A00Z&end=2026-08-05T00%3A00%3A00Z", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || provider.rangeValue != "12m" ||
		!provider.start.Equal(time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)) ||
		!provider.end.Equal(time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("custom history status=%d range=%q start=%s end=%s body=%s",
			response.Code, provider.rangeValue, provider.start, provider.end, response.Body.String())
	}
}

func TestMonitoringHistoryMapsBusyAndUnavailable(t *testing.T) {
	server := testServer(t)
	provider := &fakeMonitoringProvider{err: monitoring.ErrBusy}
	server.monitoring = provider
	request := httptest.NewRequest(http.MethodGet, "/v1/monitoring/history", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("busy status = %d body=%s", response.Code, response.Body.String())
	}

	server.monitoring = nil
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestMonitoringChecksRequireAuthAndSupportAtomicReplacement(t *testing.T) {
	server := testServer(t)
	version := "sha256:" + strings.Repeat("a", 64)
	provider := &fakeMonitoringChecksProvider{
		fakeMonitoringProvider: &fakeMonitoringProvider{},
		snapshot:               monitoring.CheckSnapshot{SchemaVersion: 1, ResourceVersion: version, Available: true, MaxItems: 16, Items: []monitoring.Check{}},
	}
	server.monitoring = provider

	request := httptest.NewRequest(http.MethodGet, "/v1/monitoring/checks", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/monitoring/checks", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"maxItems":16`) {
		t.Fatalf("GET status=%d body=%s", response.Code, response.Body.String())
	}

	body := `{"expectedResourceVersion":"` + version + `","items":[{"id":"tcp-web","kind":"tcp","name":"Web","target":"example.com:443"}]}`
	request = httptest.NewRequest(http.MethodPut, "/v1/monitoring/checks", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(provider.input.Items) != 1 || provider.input.Items[0].Kind != "tcp" {
		t.Fatalf("PUT status=%d input=%#v body=%s", response.Code, provider.input, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/v1/monitoring/checks", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d", response.Code)
	}
}
