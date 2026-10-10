package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContainerTerminalRejectsInvalidOrStaleTargetsBeforeCreatingExec(t *testing.T) {
	id := strings.Repeat("a", 64)
	inspect := managedInspect(id, "2026-10-10T00:00:00Z", 0)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/containers/"+id+"/json" {
			t.Errorf("unexpected mutation %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", 500)
			return
		}
		_ = json.NewEncoder(w).Encode(inspect)
	}))
	defer server.Close()
	c := testHTTPClient(server)
	for _, target := range []struct {
		id, version string
		rows, cols  uint16
	}{
		{"../../host", "v", 24, 80}, {id, "", 24, 80}, {id, "v", 0, 80}, {id, "v", 24, 1001},
	} {
		if _, err := c.OpenContainerTerminal(context.Background(), target.id, target.version, target.rows, target.cols); !errors.Is(err, ErrInvalidDockerExec) {
			t.Fatalf("invalid target = %v", err)
		}
	}
	if requests != 0 {
		t.Fatalf("invalid target issued %d requests", requests)
	}
	if _, err := c.OpenContainerTerminal(context.Background(), id, "stale", 24, 80); !errors.Is(err, ErrResourceConflict) {
		t.Fatalf("stale target = %v", err)
	}
	if requests != 1 {
		t.Fatalf("stale target requests = %d", requests)
	}
}
