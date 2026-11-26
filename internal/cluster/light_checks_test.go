package cluster

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

func TestHistoryRelayCarriesOnlyHistoryAndChecks(t *testing.T) {
	json := map[string]string{"Content-Type": "application/json"}
	for _, test := range []struct {
		name  string
		input LightFileRequest
		want  bool
	}{
		{"history", LightFileRequest{Method: http.MethodGet, Path: HistoryPath, RawQuery: "range=6h"}, true},
		{"read checks", LightFileRequest{Method: http.MethodGet, Path: ChecksPath}, true},
		{"replace checks", LightFileRequest{Method: http.MethodPut, Path: ChecksPath, Headers: json, BodyLength: 512}, true},
		{"largest checks update", LightFileRequest{Method: http.MethodPut, Path: ChecksPath, Headers: json, BodyLength: monitoring.MaxCheckUpdateBytes}, true},
		{"history with body", LightFileRequest{Method: http.MethodGet, Path: HistoryPath, BodyLength: 1}, false},
		{"history write", LightFileRequest{Method: http.MethodPut, Path: HistoryPath, Headers: json, BodyLength: 1}, false},
		{"read checks with query", LightFileRequest{Method: http.MethodGet, Path: ChecksPath, RawQuery: "a=1"}, false},
		{"read checks with body", LightFileRequest{Method: http.MethodGet, Path: ChecksPath, BodyLength: 1}, false},
		{"replace without body", LightFileRequest{Method: http.MethodPut, Path: ChecksPath, Headers: json}, false},
		{"replace with streamed body", LightFileRequest{Method: http.MethodPut, Path: ChecksPath, Headers: json, BodyLength: -1}, false},
		{"replace too large", LightFileRequest{Method: http.MethodPut, Path: ChecksPath, Headers: json, BodyLength: monitoring.MaxCheckUpdateBytes + 1}, false},
		{"replace without JSON type", LightFileRequest{Method: http.MethodPut, Path: ChecksPath, BodyLength: 10}, false},
		{"replace with extra header", LightFileRequest{Method: http.MethodPut, Path: ChecksPath, BodyLength: 10, Headers: map[string]string{"Content-Type": "application/json", "X-Other": "1"}}, false},
		{"replace with other type", LightFileRequest{Method: http.MethodPut, Path: ChecksPath, BodyLength: 10, Headers: map[string]string{"Content-Type": "text/plain"}}, false},
		{"delete checks", LightFileRequest{Method: http.MethodDelete, Path: ChecksPath}, false},
		{"post checks", LightFileRequest{Method: http.MethodPost, Path: ChecksPath, Headers: json, BodyLength: 10}, false},
		{"check status", LightFileRequest{Method: http.MethodGet, Path: "/v1/monitoring/check-status"}, false},
		{"files", LightFileRequest{Method: http.MethodGet, Path: "/v1/files"}, false},
	} {
		if got := validHistoryRelayRequest(test.input); got != test.want {
			t.Errorf("%s: valid = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestHistoryRelayBodyChunksStayWithinOneChecksUpdate(t *testing.T) {
	now := time.Now().UTC()
	body := func(offset int64, size int, final bool) FileRelayCommand {
		return FileRelayCommand{ID: strings.Repeat("a", 32), Kind: "body", RequestID: strings.Repeat("b", 32), Offset: offset, Data: make([]byte, size), Final: final, ExpiresAt: now.Add(time.Minute).Unix()}
	}
	for _, test := range []struct {
		name    string
		command FileRelayCommand
		valid   bool
	}{
		{"first chunk", body(0, lightFileChunkBytes, false), true},
		{"last chunk", body(lightFileChunkBytes, monitoring.MaxCheckUpdateBytes-lightFileChunkBytes, true), true},
		{"empty final", body(monitoring.MaxCheckUpdateBytes, 0, true), true},
		{"past the update limit", body(lightFileChunkBytes, monitoring.MaxCheckUpdateBytes-lightFileChunkBytes+1, true), false},
		{"far offset", body(1<<20, 1, true), false},
		{"offset overflows a sum", body(math.MaxInt64, 1, true), false},
		{"last byte of the update", body(monitoring.MaxCheckUpdateBytes-1, 1, true), true},
		{"oversized chunk", body(0, lightFileChunkBytes+1, false), false},
		{"empty non-final", body(0, 0, false), false},
	} {
		if err := validHistoryRelayCommand(test.command, now); (err == nil) != test.valid {
			t.Errorf("%s: error = %v, want valid %v", test.name, err, test.valid)
		}
	}
	request := FileRelayCommand{ID: strings.Repeat("c", 32), Kind: "request", RequestID: strings.Repeat("d", 32), Method: http.MethodPut, Path: ChecksPath, Headers: map[string]string{"Content-Type": "application/json"}, BodyLength: 100, ExpiresAt: now.Add(time.Minute).Unix()}
	if err := validHistoryRelayCommand(request, now); err != nil {
		t.Fatalf("checks replacement command rejected: %v", err)
	}
	request.Path = "/v1/files"
	if err := validHistoryRelayCommand(request, now); err == nil {
		t.Fatal("history relay accepted a file request")
	}
}

func TestLightChecksCapabilityFollowsAuthenticatedReports(t *testing.T) {
	now := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	enrollment := enrollLightHostForTest(t, service, "edge")
	report := func(id string, capabilities []string) error {
		input := LightReportRequest{Telemetry: serviceTelemetry(clock.Now(), "edge"), Capabilities: capabilities}
		body, auth := signedLightReportForTest(t, clock.Now(), enrollment, input, id)
		_, err := service.AcceptLightReport(auth, body, input)
		return err
	}
	if service.LightChecksReadiness(enrollment.NodeID).Supported {
		t.Fatal("a node that never reported was treated as supporting checks sync")
	}
	if err := report(strings.Repeat("1", 32), []string{"future-feature-v9", MonitoringChecksSyncCapability}); err != nil {
		t.Fatal(err)
	}
	readiness := service.LightChecksReadiness(enrollment.NodeID)
	if !readiness.Supported || readiness.Connected {
		t.Fatalf("readiness = %#v", readiness)
	}
	// The relay is not connected, so nothing is sent.
	if _, err := service.LightNodeChecks(context.Background(), enrollment.NodeID); !errors.Is(err, ErrLightChecksUnavailable) {
		t.Fatalf("disconnected read error = %v", err)
	}
	for index, invalid := range [][]string{{"Bad"}, {""}, {strings.Repeat("a", 65)}, strings.Split(strings.Repeat("a,", 9)[:17], ",")} {
		if err := report(strings.Repeat(string(rune('5'+index)), 32), invalid); !errors.Is(err, ErrProtocolMismatch) {
			t.Fatalf("features %q error = %v", invalid, err)
		}
	}
	// A downgraded node stops advertising the feature and is no longer sent it.
	clock.now = clock.now.Add(time.Minute)
	if err := report(strings.Repeat("3", 32), nil); err != nil {
		t.Fatal(err)
	}
	if service.LightChecksReadiness(enrollment.NodeID).Supported {
		t.Fatal("capability survived a report without it")
	}
	if err := report(strings.Repeat("4", 32), []string{MonitoringChecksSyncCapability}); err != nil {
		t.Fatal(err)
	}
	clock.now = clock.now.Add(lightChecksCapabilityTTL + time.Second)
	if service.LightChecksReadiness(enrollment.NodeID).Supported {
		t.Fatal("capability did not expire without fresh reports")
	}
	clock.now = clock.now.Add(-lightChecksCapabilityTTL)
	host, err := service.Host(context.Background(), enrollment.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DeleteHost(context.Background(), enrollment.NodeID, DeleteHostInput{ExpectedResourceVersion: host.ResourceVersion}); err != nil {
		t.Fatal(err)
	}
	if service.lightChecks.supported(enrollment.NodeID, clock.Now()) {
		t.Fatal("deleted node kept its capability")
	}
	if _, err := service.LightNodeChecks(context.Background(), enrollment.NodeID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted node read error = %v", err)
	}
}

func TestCenterAdvertisesChecksSync(t *testing.T) {
	for _, capability := range LightCenterCapabilities() {
		if capability == MonitoringChecksSyncCapability {
			return
		}
	}
	t.Fatal("center does not advertise checks sync to light nodes")
}
