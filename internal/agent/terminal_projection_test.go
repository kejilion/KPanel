package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/appmarket"
	"github.com/kejilion/kejilion-panel/internal/diagnostics"
	"github.com/kejilion/kejilion-panel/internal/sites"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

func terminalFixtureReader(source string, chunkSize int) func(int64) (terminalSourceChunk, error) {
	return func(offset int64) (terminalSourceChunk, error) {
		end := min(offset+int64(chunkSize), int64(len(source)))
		return terminalSourceChunk{DataBase64: base64.StdEncoding.EncodeToString([]byte(source[offset:end])), NextOffset: end, Finished: end == int64(len(source)), InputOpen: true}, nil
	}
}

func TestTaskDisplayProjectsScriptMetadata(t *testing.T) {
	metadata := map[string]any{"id": "test-job", "status": "running", "message": "password=synthetic-message-secret", "events": []string{"服务密码: synthetic-event-secret"}}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/?projection=model-v1", nil)
	writeTaskDisplay(w, r, metadata, func(_ string, offset int64) (terminalSourceChunk, error) {
		return terminalFixtureReader("status: ready\n", 1024)(offset)
	})
	if w.Code != 200 || strings.Contains(w.Body.String(), "synthetic-") || !strings.Contains(w.Body.String(), "running") || !strings.Contains(w.Body.String(), "ready") {
		t.Fatal(w.Code, w.Body.String())
	}
	if metadata["message"] != "password=synthetic-message-secret" {
		t.Fatal("display projection changed owner metadata")
	}
}

func TestTerminalDisplayProjectsBeforeArbitraryPagination(t *testing.T) {
	source := strings.Repeat("visible\n", 5000) + "password='synthetic-first\nsynthetic-second'\n-----BEGIN PRIVATE KEY-----\nsynthetic-private\n-----END PRIVATE KEY-----\n状态: ready\n"
	var output strings.Builder
	var offset int64
	for i := 0; i < 10; i++ {
		page, err := buildTerminalDisplay(context.Background(), offset, terminalFixtureReader(source, 32<<10))
		if err != nil || page.Projection != terminalDisplayVersion || !utf8.ValidString(page.Text) || !page.InputOpen {
			t.Fatalf("%#v: %v", page, err)
		}
		output.WriteString(page.Text)
		if page.Finished {
			break
		}
		if page.NextOffset <= offset {
			t.Fatal("display cursor did not advance")
		}
		offset = page.NextOffset
	}
	if strings.Contains(output.String(), "synthetic-") || !strings.Contains(output.String(), "状态: ready") {
		t.Fatal("unsafe or unusable complete display")
	}
	// Any chosen source-like offset can reveal only a substring of the already
	// safe snapshot, even if the cursor lands inside the former secret value.
	for _, offset := range []int64{int64(strings.Index(source, "synthetic-first")), int64(strings.Index(source, "synthetic-private")), 1 << 40} {
		page, err := buildTerminalDisplay(context.Background(), offset, terminalFixtureReader(source, 32<<10))
		if err != nil || strings.Contains(page.Text, "synthetic-") {
			t.Fatalf("%#v: %v", page, err)
		}
	}
	for _, source := range []string{"password='synthetic-first\nsynthetic-second", "密码：synthetic-password", "-----BEGIN PRIVATE KEY-----\nsynthetic-private"} {
		page, err := buildTerminalDisplay(context.Background(), 0, terminalFixtureReader(source, 3))
		if err != nil || strings.Contains(page.Text, "synthetic-") {
			t.Fatalf("%#v: %v", page, err)
		}
	}
	page, err := buildTerminalDisplay(context.Background(), 1, terminalFixtureReader(strings.Repeat("字", 5000), 32<<10))
	if err != nil || !utf8.ValidString(page.Text) || len(page.Text) > maxTerminalDisplayPage || page.NextOffset%3 != 0 {
		t.Fatalf("UTF-8 display cursor: %#v %v", page, err)
	}
}

func TestTerminalReadProjectionQueryIsExplicit(t *testing.T) {
	query, err := parseTerminalReadQuery(url.Values{"offset": {"9"}, "projection": {"model-v1"}}, true)
	if err != nil || !query.ModelProjection || query.Offset != 9 {
		t.Fatalf("%#v %v", query, err)
	}
	for _, values := range []url.Values{{"offset": {"0"}, "projection": {"raw"}}, {"offset": {"0"}, "projection": {"model-v1", "model-v1"}}} {
		if _, err := parseTerminalReadQuery(values, true); err == nil {
			t.Fatal("invalid display contract accepted")
		}
	}
}

func TestTerminalDisplayBoundsMissingContextAndCancellation(t *testing.T) {
	for name, reader := range map[string]func(int64) (terminalSourceChunk, error){
		"rotation": func(int64) (terminalSourceChunk, error) {
			return terminalSourceChunk{Truncated: true, NextOffset: 100}, nil
		},
		"gap": func(int64) (terminalSourceChunk, error) {
			return terminalSourceChunk{DataBase64: "YQ==", NextOffset: 100}, nil
		},
		"malformed":    func(int64) (terminalSourceChunk, error) { return terminalSourceChunk{DataBase64: "!"}, nil },
		"input budget": terminalFixtureReader(strings.Repeat("x", maxTerminalDisplaySource+1), 64<<10),
		"read budget":  terminalFixtureReader(strings.Repeat("x", maxTerminalDisplayReads+1), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if page, err := buildTerminalDisplay(context.Background(), 0, reader); err == nil || page.Text != "" {
				t.Fatalf("%#v: %v", page, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := buildTerminalDisplay(ctx, 0, func(int64) (terminalSourceChunk, error) {
		t.Fatal("cancelled read invoked owner")
		return terminalSourceChunk{}, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for i := 0; i < cap(terminalDisplaySlots); i++ {
		terminalDisplaySlots <- struct{}{}
	}
	_, err := projectTerminalRead(context.Background(), 0, func(int64) (terminalSourceChunk, error) {
		t.Fatal("saturated projection invoked owner")
		return terminalSourceChunk{}, nil
	})
	for i := 0; i < cap(terminalDisplaySlots); i++ {
		<-terminalDisplaySlots
	}
	if err == nil {
		t.Fatal("projection concurrency unbounded")
	}
	page, err := projectTerminalRead(context.Background(), 0, terminalFixtureReader("status: ready", 64<<10))
	if err != nil || page.Text != "status: ready" {
		t.Fatalf("slot recovery: %#v %v", page, err)
	}
}

func TestTerminalDisplayAllOwnerChunkContracts(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("password=synthetic-secret\nstatus: ready"))
	checks := []func() (terminalDisplay, error){
		func() (terminalDisplay, error) {
			return projectTerminalRead(context.Background(), 0, func(int64) (appmarket.TerminalChunk, error) {
				return appmarket.TerminalChunk{DataBase64: encoded, NextOffset: int64(len("password=synthetic-secret\nstatus: ready")), Finished: true}, nil
			})
		},
		func() (terminalDisplay, error) {
			return projectTerminalRead(context.Background(), 0, func(int64) (sites.SiteTerminalChunk, error) {
				return sites.SiteTerminalChunk{DataBase64: encoded, NextOffset: int64(len("password=synthetic-secret\nstatus: ready")), Finished: true}, nil
			})
		},
		func() (terminalDisplay, error) {
			return projectTerminalRead(context.Background(), 0, func(int64) (diagnostics.TerminalChunk, error) {
				return diagnostics.TerminalChunk{DataBase64: encoded, NextOffset: int64(len("password=synthetic-secret\nstatus: ready")), Finished: true}, nil
			})
		},
		func() (terminalDisplay, error) {
			return projectTerminalRead(context.Background(), 0, func(int64) (webenv.TerminalChunk, error) {
				return webenv.TerminalChunk{DataBase64: encoded, NextOffset: int64(len("password=synthetic-secret\nstatus: ready")), Finished: true}, nil
			})
		},
	}
	for i, check := range checks {
		page, err := check()
		if err != nil || strings.Contains(page.Text, "synthetic-") || !strings.Contains(page.Text, "ready") {
			t.Fatalf("owner %d: %#v %v", i, page, err)
		}
	}
}

func TestTerminalDisplayRealAppOwnerPreservesRawAdminOutput(t *testing.T) {
	s := testServer(t)
	dir := filepath.Join(t.TempDir(), "jobs")
	if err := s.appMarket.ConfigureJobs(dir, filepath.Join(t.TempDir(), "agent")); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 32)
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(`{"id":"`+id+`","interactive":true,"status":"succeeded"}`), 0600); err != nil {
		t.Fatal(err)
	}
	raw := "password=synthetic-owner-secret\n状态: ready\n"
	if err := os.WriteFile(filepath.Join(dir, id+".log"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	request := func(query string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/v1/app-jobs/"+id+"/terminal?"+query, nil)
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	w := request("offset=0&projection=model-v1")
	if w.Code != 200 || strings.Contains(w.Body.String(), "synthetic-") || !strings.Contains(w.Body.String(), "ready") {
		t.Fatalf("model %d %s", w.Code, w.Body.String())
	}
	w = request("offset=0")
	var chunk terminalSourceChunk
	_ = json.Unmarshal(w.Body.Bytes(), &chunk)
	data, _ := base64.StdEncoding.DecodeString(chunk.DataBase64)
	if w.Code != 200 || string(data) != raw {
		t.Fatalf("admin output changed: %d %s", w.Code, w.Body.String())
	}
	// Offset 9 is exactly where the raw password begins.
	w = request("offset=9&projection=model-v1")
	if w.Code != 200 || strings.Contains(w.Body.String(), "synthetic-") {
		t.Fatalf("model offset bypass: %d %s", w.Code, w.Body.String())
	}
}

func TestTerminalDisplayNonInteractiveSummaryAndListKeepContextBeforeTail(t *testing.T) {
	s := testServer(t)
	dir := filepath.Join(t.TempDir(), "jobs")
	if err := s.appMarket.ConfigureJobs(dir, filepath.Join(t.TempDir(), "agent")); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("b", 32)
	if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(`{"id":"`+id+`","interactive":false,"status":"succeeded"}`), 0600); err != nil {
		t.Fatal(err)
	}
	raw := "password='synthetic-start\n" + strings.Repeat("synthetic-body\n", 130) + "synthetic-end'\nstatus: ready\n"
	if err := os.WriteFile(filepath.Join(dir, id+".log"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	request := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/v1/app-jobs/" + id + "?projection=model-v1", "/v1/app-jobs?projection=model-v1", "/v1/app-jobs/" + id + "/terminal?offset=0&projection=model-v1"} {
		w := request(path)
		if w.Code != 200 || strings.Contains(w.Body.String(), "synthetic-") || !strings.Contains(w.Body.String(), "ready") {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	// The administrator-facing summary must redact secrets from its log tail too.
	w := request("/v1/app-jobs/" + id)
	if w.Code != 200 {
		t.Fatalf("admin summary status: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "synthetic-body") {
		t.Fatalf("admin summary leaked redacted log content: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "status: ready") {
		t.Fatalf("admin summary lost non-sensitive log context: %s", w.Body.String())
	}
	if err := os.Rename(filepath.Join(dir, id+".log"), filepath.Join(dir, id+".log.1")); err != nil {
		t.Fatal(err)
	}
	w := request("/v1/app-jobs/" + id + "?projection=model-v1")
	if w.Code != 200 || strings.Contains(w.Body.String(), "synthetic-") || !strings.Contains(w.Body.String(), "logsContextUnavailable") {
		t.Fatalf("rotated context: %d %s", w.Code, w.Body.String())
	}
}

func TestTerminalDisplayListSharesOneInputBudget(t *testing.T) {
	source := "password=" + strings.Repeat("x", 4<<20)
	reader := terminalFixtureReader(source, 64<<10)
	r := httptest.NewRequest("GET", "/model?projection=model-v1", nil)
	w := httptest.NewRecorder()
	writeTaskDisplay(w, r, map[string]any{"items": []any{map[string]any{"id": "one"}, map[string]any{"id": "two"}, map[string]any{"id": "three"}}}, func(_ string, offset int64) (terminalSourceChunk, error) { return reader(offset) })
	if w.Code != 200 || !strings.Contains(w.Body.String(), "logsContextUnavailable") || strings.Contains(w.Body.String(), strings.Repeat("x", 20)) {
		t.Fatalf("shared budget: %d %s", w.Code, w.Body.String())
	}
}

func TestTerminalDisplayRealRemainingOwnersReadNonInteractiveLogs(t *testing.T) {
	for _, kind := range []string{"site", "diagnostic", "web"} {
		t.Run(kind, func(t *testing.T) {
			s := testServer(t)
			dir := filepath.Join(t.TempDir(), "jobs")
			executable := filepath.Join(t.TempDir(), "agent")
			id := strings.Repeat("c", 32)
			var path, logName string
			switch kind {
			case "site":
				if err := s.sitesManager.ConfigureRecipeJobState(dir, executable); err != nil {
					t.Fatal(err)
				}
				path = "/v1/site-installations/" + id
				logName = id + ".terminal.log"
			case "diagnostic":
				var err error
				s.diagnostics, err = diagnostics.New(dir, executable)
				if err != nil {
					t.Fatal(err)
				}
				path = "/v1/diagnostic-jobs/" + id
				logName = id + ".log"
			case "web":
				var err error
				s.webEnvironment, err = webenv.New(dir)
				if err != nil {
					t.Fatal(err)
				}
				path = "/v1/web-environment/jobs/" + id
				logName = id + ".log"
			}
			if err := os.WriteFile(filepath.Join(dir, id+".json"), []byte(`{"id":"`+id+`","checkId":"memory","interactive":false,"status":"succeeded"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, logName), []byte("password='synthetic-secret\nsynthetic-continuation'\nstatus: ready\n"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, query := range []string{"?projection=model-v1", "/terminal?offset=9&projection=model-v1"} {
				r := httptest.NewRequest("GET", path+query, nil)
				r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
				w := httptest.NewRecorder()
				s.ServeHTTP(w, r)
				if w.Code != 200 || strings.Contains(w.Body.String(), "synthetic-") || !strings.Contains(w.Body.String(), "ready") || strings.Contains(w.Body.String(), "logsContextUnavailable") {
					t.Fatalf("%s: %d %s", query, w.Code, w.Body.String())
				}
			}
		})
	}
}
