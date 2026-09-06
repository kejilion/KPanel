package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNotificationResourcesReadOnlyBoundedProjection(t *testing.T) {
	var reads, active, peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("unexpected Docker write %s", r.Method)
		}
		if r.URL.Path == "/containers/json" {
			items := []map[string]any{}
			for i := range 12 {
				items = append(items, map[string]any{"Id": fmt.Sprintf("%064x", i+1), "Names": []string{"/important"}})
			}
			_ = json.NewEncoder(w).Encode(items)
			return
		}
		reads.Add(1)
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/containers/"), "/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/renamed", "RestartCount": 4, "State": map[string]any{"Status": "running", "Health": map[string]any{"Status": "unhealthy"}}, "Config": map[string]any{"Env": []string{"SECRET=must-not-leak"}}})
	}))
	defer server.Close()
	c := &Client{httpClient: server.Client(), baseURL: server.URL}
	items, err := c.NotificationResources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 12 || reads.Load() != 12 || peak.Load() > 4 {
		t.Fatalf("items=%d reads=%d peak=%d", len(items), reads.Load(), peak.Load())
	}
	if !items[0].Known || items[0].Name != "renamed" || *items[0].RestartCount != 4 || items[0].Health != "unhealthy" {
		t.Fatal(items[0])
	}
	data, _ := json.Marshal(items)
	if strings.Contains(string(data), "SECRET") || strings.Contains(string(data), "Config") {
		t.Fatal("inspect fields leaked")
	}
}

func TestNotificationResourcesLimitFailureAndCancellation(t *testing.T) {
	count := MaxNotificationContainers + 1
	inspectCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/json" {
			items := []map[string]any{}
			for i := range count {
				items = append(items, map[string]any{"Id": fmt.Sprintf("%064x", i+1), "Names": []string{"/name"}})
			}
			_ = json.NewEncoder(w).Encode(items)
			return
		}
		inspectCount++
		w.WriteHeader(500)
	}))
	defer server.Close()
	c := &Client{httpClient: server.Client(), baseURL: server.URL}
	if _, err := c.NotificationResources(context.Background()); !errors.Is(err, ErrNotificationResourceLimit) || inspectCount != 0 {
		t.Fatalf("unbounded read: %v %d", err, inspectCount)
	}
	count = 1
	items, err := c.NotificationResources(context.Background())
	if err != nil || len(items) != 1 || items[0].Known || items[0].RestartCount != nil {
		t.Fatalf("failed inspect fabricated state: %+v %v", items, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.NotificationResources(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func BenchmarkNotificationResources128(b *testing.B) {
	items := make([]map[string]any, 128)
	for i := range items {
		items[i] = map[string]any{"Id": fmt.Sprintf("%064x", i+1), "Names": []string{"/container"}}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/json" {
			_ = json.NewEncoder(w).Encode(items)
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/containers/"), "/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/container", "RestartCount": 0, "State": map[string]string{"Status": "running"}})
	}))
	defer server.Close()
	c := &Client{httpClient: server.Client(), baseURL: server.URL}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, err := c.NotificationResources(context.Background())
		if err != nil || len(result) != 128 {
			b.Fatalf("projection: %v", err)
		}
	}
}

func TestNotificationResourcesOversizedInspectIsUnknown(t *testing.T) {
	id := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/containers/json" {
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": id, "Names": []string{"/large"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/large", "RestartCount": 0, "State": map[string]string{"Status": "running"}, "Config": map[string]string{"large": strings.Repeat("x", 300<<10)}})
	}))
	defer server.Close()
	c := &Client{httpClient: server.Client(), baseURL: server.URL}
	items, err := c.NotificationResources(context.Background())
	if err != nil || len(items) != 1 || items[0].Known {
		t.Fatal("oversized inspect accepted as known")
	}
}
