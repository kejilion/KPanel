package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/systemmanage"
)

func TestOverviewReadRoutesAuthMethodsAndQueries(t *testing.T) {
	s := testServer(t)
	s.system.CPUSampleInterval = 0
	for _, path := range []string{"/v1/system/runtime", "/v1/system/management/config", "/v1/system/management/ssh-defense", "/v1/system/management/bbrv3"} {
		for _, tc := range []struct {
			method, suffix, token string
			want                  int
		}{
			{http.MethodGet, "", "", http.StatusUnauthorized},
			{http.MethodPost, "", strings.Repeat("x", 32), http.StatusMethodNotAllowed},
			{http.MethodGet, "?command=touch", strings.Repeat("x", 32), http.StatusBadRequest},
		} {
			r := httptest.NewRequest(tc.method, path+tc.suffix, nil)
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%s %s: got %d want %d", tc.method, path+tc.suffix, w.Code, tc.want)
			}
		}
	}
}

func TestOverviewRuntimeAndConfigurationDoNotWaitForOptionalProbe(t *testing.T) {
	s := testServer(t)
	s.system.CPUSampleInterval = 0
	s.system.PublicNetworkLookup = func(context.Context) (summary contract.PublicNetworkSummary, err error) {
		t.Error("overview runtime/config must not look up public network")
		return
	}
	runner := &summaryStatusRunner{started: make(chan string, 2), release: make(chan struct{})}
	finder := func() (string, error) { return "/trusted/kejilion.sh", nil }
	s.systemManager = systemmanage.NewManager(systemmanage.Config{EtcRoot: t.TempDir(), StateDir: t.TempDir(), Runner: runner, F2BScript: finder, BBRv3Script: finder})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	done := make(chan struct{})
	go func() { request("/v1/system/management/ssh-defense"); close(done) }()
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}
	for _, path := range []string{"/v1/system/runtime", "/v1/system/management/config"} {
		response := make(chan *httptest.ResponseRecorder, 1)
		go func() { response <- request(path) }()
		var w *httptest.ResponseRecorder
		select {
		case w = <-response:
		case <-time.After(time.Second):
			t.Fatal("fast read waited for optional probe: " + path)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(path, "/config") && result["observedAt"] == nil {
			t.Fatal("missing observation time")
		}
	}
	select {
	case action := <-runner.started:
		t.Fatalf("runtime/config unexpectedly launched %s", action)
	default:
	}
	select {
	case <-done:
		t.Fatal("fixture probe did not remain blocked")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("probe did not honor cancellation")
	}
}
