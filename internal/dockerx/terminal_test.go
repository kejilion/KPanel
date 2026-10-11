package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestContainerTerminalMissingShellRequiresAnUnchangedRunningTarget(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, scenario := range []struct {
		name          string
		head, inspect int
		change        func(*containerInspect)
		missing       bool
	}{
		{name: "missing-shell", head: 404, inspect: 200, missing: true},
		{name: "shell-present", head: 200, inspect: 200},
		{name: "permission-denied", head: 403, inspect: 200},
		{name: "daemon-failure", head: 500, inspect: 200},
		{name: "container-deleted", head: 404, inspect: 404},
		{name: "inspect-failure", head: 404, inspect: 500},
		{name: "changed-version", head: 404, inspect: 200, change: func(v *containerInspect) { v.RestartCount++ }},
		{name: "stopped", head: 404, inspect: 200, change: func(v *containerInspect) { v.State.Running = false }},
		{name: "paused", head: 404, inspect: 200, change: func(v *containerInspect) { v.State.Paused = true }},
		{name: "restarting", head: 404, inspect: 200, change: func(v *containerInspect) { v.State.Restarting = true }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			inspect := managedInspect(id, "2026-10-10T00:00:00Z", 0)
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodHead && r.URL.Path == "/containers/"+id+"/archive" && r.URL.RawQuery == "path=%2Fbin%2Fsh":
					w.WriteHeader(scenario.head)
				case r.Method == http.MethodGet && r.URL.Path == "/containers/"+id+"/json":
					reads++
					w.WriteHeader(scenario.inspect)
					_ = json.NewEncoder(w).Encode(inspect)
				default:
					t.Errorf("unexpected Docker request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			c := testHTTPClient(server)
			version := c.summaryFromInspect(inspect).ResourceVersion
			if scenario.change != nil {
				scenario.change(&inspect)
			}
			err := c.containerTerminalStartError(context.Background(), id, version, errContainerShellStart)
			if scenario.missing {
				if !errors.Is(err, ErrContainerShellUnavailable) {
					t.Fatalf("missing shell = %v", err)
				}
			} else if err != errContainerShellStart {
				t.Fatalf("diagnostic replaced the original failure: %v", err)
			}
			if (reads == 1) != (scenario.head == http.StatusNotFound) {
				t.Fatalf("identity rechecks = %d", reads)
			}
		})
	}
}

func TestContainerTerminalShellDiagnosisPreservesCancellationAndOtherErrors(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		<-r.Context().Done()
	}))
	defer server.Close()
	c := testHTTPClient(server)
	for _, original := range []error{nil, context.DeadlineExceeded, ErrContainerTerminalUnsupported, errors.New("process identity changed")} {
		if err := c.containerTerminalStartError(context.Background(), strings.Repeat("a", 64), "version", original); err != original {
			t.Fatalf("unrelated result changed: %v", err)
		}
	}
	if requests != 0 {
		t.Fatalf("unrelated results issued %d diagnostic requests", requests)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := c.containerTerminalStartError(ctx, strings.Repeat("a", 64), "version", errContainerShellStart); err != errContainerShellStart {
		t.Fatalf("timeout replaced launch failure: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("diagnostic did not honor caller deadline")
	}
}
