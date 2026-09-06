//go:build linux

package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/systeminfo"
	"github.com/kejilion/kejilion-panel/internal/systemmanage"
)

func TestSystemProcessesReturnsBoundedLiveQuery(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/v1/system/processes?sort=pid&order=asc&limit=1", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var snapshot systeminfo.ProcessSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 || snapshot.Total < 1 || snapshot.Items[0].PID < 1 ||
		snapshot.Items[0].StartTimeTicks == 0 {
		t.Fatalf("unexpected live process snapshot: %#v", snapshot)
	}
}

func TestProcessSignalSealsSamplesBeforeAndAfterExecution(t *testing.T) {
	server := testServer(t)
	procRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(procRoot, "42"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(procRoot, "42", "stat"), []byte("42 (worker) S 1 0 0 0 0 0 0 0 0 0 10 5 0 0 0 0 0 0 999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := &processReadCall{ctx: context.Background()}
	during := &processReadCall{ctx: context.Background()}
	server.processReads.active = before
	signaled := false
	server.systemManager = systemmanage.NewManager(systemmanage.Config{
		Enabled: true, ProcRoot: procRoot, EtcRoot: t.TempDir(), StateDir: t.TempDir(), EffectiveUID: func() int { return 0 },
		ProcessSignaler: func(pid int, signal string) error {
			server.processReads.mu.Lock()
			defer server.processReads.mu.Unlock()
			if !before.sealed || pid != 42 || signal != "term" {
				t.Error("pre-write seal or signal identity lost")
			}
			// Model an old sample finishing and another starting during the write.
			server.processReads.active = during
			signaled = true
			return nil
		},
	})
	request := httptest.NewRequest(http.MethodPost, "/v1/system/actions", strings.NewReader(`{"action":"process-signal","pid":42,"startTimeTicks":999,"signal":"term"}`))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !signaled || !during.sealed {
		t.Fatalf("post-write sample can still be joined: HTTP %d, signaled=%v, sealed=%v, body=%s", response.Code, signaled, during.sealed, response.Body.String())
	}
}
