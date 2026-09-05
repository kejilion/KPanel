package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/appmarket"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/store"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

type webEnvironmentHistoryAgent struct {
	*stubAgent
	service *webenv.Service
	reverse bool
}

func (a *webEnvironmentHistoryAgent) Get(ctx context.Context, path, query, requestID string) (AgentResponse, error) {
	if path != "/v1/web-environment/jobs" {
		return a.stubAgent.Get(ctx, path, query, requestID)
	}
	items := a.service.Jobs()
	if a.reverse {
		for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
			items[left], items[right] = items[right], items[left]
		}
	}
	body, err := json.Marshal(contract.PageResult[webenv.Job]{Items: items})
	return AgentResponse{StatusCode: http.StatusOK, Body: body}, err
}

func TestJobsAcceptsRealWebEnvironmentHistoryBeyondDisplayWindow(t *testing.T) {
	for _, count := range []int{200, 201, 450} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			root := t.TempDir()
			service, err := webenv.New(root)
			if err != nil {
				t.Fatal(err)
			}
			base := time.Date(2026, time.September, 5, 0, 0, 0, 0, time.UTC)
			for i := 0; i < count; i++ {
				created := base.Add(time.Duration(i) * time.Minute)
				finished := created.Add(time.Second)
				job := webenv.Job{ID: fmt.Sprintf("%032x", i+1), Action: "backup", Status: "succeeded", Stage: "completed", Progress: 100, CreatedAt: created, StartedAt: &created, FinishedAt: &finished}
				if i == count-1 {
					job.Status = "failed"
					job.Stage = "failed"
					job.Message = "latest backup failed"
				}
				data, err := json.Marshal(job)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, job.ID+".json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if len(service.Jobs()) != count {
				t.Fatal("real owner did not expose all legal history")
			}
			server, tokenPath := newTestServer(t)
			session, _ := bootstrapCookies(t, server, tokenPath)
			agent := &webEnvironmentHistoryAgent{stubAgent: &stubAgent{response: AgentResponse{StatusCode: 200, Body: []byte(`{"items":[]}`)}}, service: service}
			server.agent = agent
			for _, reverse := range []bool{false, true} {
				agent.reverse = reverse
				source, err := agent.Get(context.Background(), "/v1/web-environment/jobs", "", "")
				if err != nil {
					t.Fatal(err)
				}
				window, err := decodeOwnerJobs(jobsFromWebEnvironment)(context.Background(), source.Body)
				if err != nil || len(window) != 100 || cap(window) != 100 {
					t.Fatalf("unbounded or invalid owner display window: len=%d cap=%d err=%v", len(window), cap(window), err)
				}
				response := performRequest(server, "GET", "/api/v1/jobs?limit=50", nil, map[string]string{"Cookie": session.Name + "=" + session.Value})
				if response.Code != 200 {
					t.Fatalf("legal owner history count=%d reverse=%v returned %d: %s", count, reverse, response.Code, response.Body.String())
				}
				var page contract.PageResult[contract.Job]
				if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				if len(page.Items) != 50 || page.Items[0].ID != "webenv:"+fmt.Sprintf("%032x", count) || page.Items[0].State != contract.JobFailedNeedsAttention {
					t.Fatalf("latest failure omitted: %#v", page.Items)
				}
				for i, job := range page.Items {
					if job.ID != "webenv:"+fmt.Sprintf("%032x", count-i) {
						t.Fatalf("history order at %d: %s", i, job.ID)
					}
				}
			}
			if len(service.Jobs()) != count {
				t.Fatal("display selection altered owner history")
			}
		})
	}
}

func TestOwnerJobWindowValidatesWholePageAndHonorsCancellation(t *testing.T) {
	decode := decodeOwnerJobs(jobsFromWebEnvironment)
	if jobs, err := decode(context.Background(), []byte(`{"nextCursor":"future-compatible","items":[]}`)); err != nil || jobs == nil || len(jobs) != 0 {
		t.Fatalf("valid empty page: %#v %v", jobs, err)
	}
	for _, body := range []string{`{}`, `{"items":null}`, `{"items":{}}`, `{"items":[]} {}`, `{"items":[],"items":[]}`} {
		if _, err := decode(context.Background(), []byte(body)); err == nil {
			t.Errorf("invalid page accepted: %s", body)
		}
	}
	items := make([]webenv.Job, 201)
	for i := range items {
		items[i] = webenv.Job{ID: fmt.Sprintf("%032x", i+1), Status: "succeeded", CreatedAt: time.Unix(int64(i), 0)}
	}
	body, err := json.Marshal(contract.PageResult[webenv.Job]{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	// Invalid data after the retained window still invalidates the response.
	malformed := append(append([]byte(nil), body[:len(body)-2]...), []byte(`,invalid]}`)...)
	if _, err := decode(context.Background(), malformed); err == nil {
		t.Fatal("ignored malformed tail after display window")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := decode(ctx, body); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

type jobTruthAgent struct {
	status  string
	offline bool
}

func (a *jobTruthAgent) Get(ctx context.Context, path, query, id string) (AgentResponse, error) {
	return a.Do(ctx, "GET", path, query, id, nil)
}
func (a *jobTruthAgent) Do(_ context.Context, method, path, query, id string, body []byte) (AgentResponse, error) {
	if a.offline {
		return AgentResponse{}, fmt.Errorf("offline")
	}
	job := fmt.Sprintf(`{"id":"%s","action":"image_pull","status":"%s","stage":"%s","createdAt":"2026-09-05T00:00:00Z"}`, strings.Repeat("a", 32), a.status, a.status)
	code := 200
	if method == "POST" {
		code = 202
	} else if path == "/v1/docker/jobs" {
		job = `{"items":[` + job + `]}`
	} else if path == "/v1/app-jobs" || path == "/v1/web-environment/jobs" {
		job = `{"items":[]}`
	}
	return AgentResponse{StatusCode: code, ContentType: "application/json", Body: []byte(job)}, nil
}

func TestDockerAcceptedFollowsOwnerStateAndSurvivesAuditTruncation(t *testing.T) {
	server, tokenPath := newTestServer(t)
	session, csrf := bootstrapCookies(t, server, tokenPath)
	agent := &jobTruthAgent{status: "queued"}
	server.agent = agent
	headers := map[string]string{"Cookie": session.Name + "=" + session.Value + "; " + csrf.Name + "=" + csrf.Value, "X-CSRF-Token": csrf.Value, "Origin": "http://panel.test", "Content-Type": "application/json"}
	response := performRequest(server, "POST", "/api/v1/docker/tasks", []byte(`{"action":"image_pull","image":"nginx:alpine"}`), headers)
	if response.Code != 202 {
		t.Fatalf("submit: %d %s", response.Code, response.Body.String())
	}
	events, _ := server.store.ListAudit(200, "")
	found := false
	for _, event := range events {
		if event.Action == "docker.image_pull" {
			found = true
			if event.Result != "accepted" {
				t.Fatalf("202 marked %s", event.Result)
			}
		}
	}
	if !found {
		t.Fatal("submission audit missing")
	}
	for _, state := range []string{"queued", "running", "succeeded", "failed"} {
		agent.status = state
		r := performRequest(server, "GET", "/api/v1/jobs", nil, headers)
		var page contract.PageResult[contract.Job]
		if err := json.Unmarshal(r.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		want := contract.JobState(state)
		if state == "failed" {
			want = contract.JobFailedNeedsAttention
		}
		if len(page.Items) != 1 || page.Items[0].State != want || page.Items[0].ID != "docker:"+strings.Repeat("a", 32) {
			t.Fatalf("owner state %s: %s", state, r.Body.String())
		}
	}
	agent.offline = true
	r := performRequest(server, "GET", "/api/v1/jobs", nil, headers)
	if r.Code != http.StatusServiceUnavailable || strings.Contains(r.Body.String(), `"succeeded"`) {
		t.Fatalf("offline manufactured success: %s", r.Body.String())
	}
	agent.offline = false
	for i := 0; i < 210; i++ {
		_ = server.store.AppendAudit(store.AuditEvent{ID: fmt.Sprint(i), Action: "auth.login", Result: "success", OccurredAt: time.Now()}, 10000)
	}
	r = performRequest(server, "GET", "/api/v1/jobs", nil, headers)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"failed_needs_attention"`) {
		t.Fatalf("detail must query owner by ID despite audit truncation: %d %s", r.Code, r.Body.String())
	}
}

func TestAppAndWebEnvironmentSubmissionAuditLinksOnlyOwnerIdentity(t *testing.T) {
	for _, tc := range []struct{ path, action, source, body string }{
		{"/api/v1/apps/builtin-4/install", "app.install", "app", `{"hostPort":8081}`},
		{"/api/v1/web-environment/jobs", "web.environment.backup", "webenv", `{"action":"backup"}`},
	} {
		t.Run(tc.source, func(t *testing.T) {
			server, tokenPath := newTestServer(t)
			session, csrf := bootstrapCookies(t, server, tokenPath)
			server.agent = &stubAgent{response: AgentResponse{StatusCode: 202, Body: []byte(`{"id":"` + strings.Repeat("b", 32) + `","status":"queued","secret":"must-not-copy"}`)}}
			r := authenticatedSiteRequest(server, session, csrf, "POST", tc.path, []byte(tc.body), true)
			if r.Code != 202 {
				t.Fatalf("submit: %d %s", r.Code, r.Body.String())
			}
			events, _ := server.store.ListAudit(200, "")
			var outcomes []store.AuditEvent
			for _, event := range events {
				if event.Action == tc.action {
					outcomes = append(outcomes, event)
				}
			}
			jobs := jobsFromAudit(outcomes, 50)
			if len(jobs) != 1 || jobs[0].ID != tc.source+":"+strings.Repeat("b", 32) || jobs[0].State == contract.JobSucceeded || jobs[0].FinishedAt != nil {
				t.Fatalf("submission treated as execution: %#v", jobs)
			}
			for _, event := range outcomes {
				if event.Result == "success" || event.Change["secret"] != nil {
					t.Fatalf("invalid audit: %#v", event)
				}
			}
		})
	}
}

func TestOwnerJobsHaveDistinctSourceIDsAndUnknownStatesFailClosed(t *testing.T) {
	id := strings.Repeat("c", 32)
	apps := jobsFromAppJobs([]appmarket.AppJob{{ID: id, Status: "unrecognized"}})
	web := jobsFromWebEnvironment([]webenv.Job{{ID: id, Status: "queued"}})
	merged := mergeOwnerJobs(apps, web)
	if len(merged) != 2 || merged[0].State != contract.JobFailedNeedsAttention || merged[1].State != contract.JobQueued {
		t.Fatalf("source collision or guessed status: %#v", merged)
	}
	if _, err := decodeOwnerJobs(jobsFromAppJobs)(context.Background(), []byte(`{}`)); err == nil {
		t.Fatal("malformed owner page accepted as empty")
	}
	legacy := jobsFromAudit([]store.AuditEvent{{ID: "old", Action: "docker.image_pull", Result: "success"}}, 50)
	if len(legacy) != 0 {
		t.Fatalf("legacy 202 success synthesized a completed job: %#v", legacy)
	}
}

func TestJobsBuildsManagementViewWithoutAuditChange(t *testing.T) {
	server, tokenPath := newTestServer(t)
	server.agent = &stubAgent{response: AgentResponse{StatusCode: 200, Body: []byte(`{"items":[]}`)}}
	sessionCookie, _ := bootstrapCookies(t, server, tokenPath)
	base := time.Date(2026, time.July, 25, 7, 0, 0, 0, time.UTC)
	events := []store.AuditEvent{
		{
			ID: "audit-1", OccurredAt: base, Action: "site.create",
			TargetKind: "site", TargetID: "example.com", Result: "intent",
			RequestID: "request-success", Change: map[string]any{"upstream": "secret.internal"},
		},
		{
			ID: "audit-2", OccurredAt: base.Add(time.Second), Action: "site.create",
			TargetKind: "site", TargetID: "example.com", Result: "success",
			RequestID: "request-success", Change: map[string]any{"upstream": "secret.internal"},
		},
		{
			ID: "audit-3", OccurredAt: base.Add(2 * time.Second), Action: "docker.restart",
			TargetKind: "container", TargetID: "container-1", Result: "intent",
			RequestID: "request-running", Change: map[string]any{"resourceVersion": "secret-version"},
		},
		{
			ID: "audit-4", OccurredAt: base.Add(3 * time.Second), Action: "site.update",
			TargetKind: "site", TargetID: "site-1", Result: "denied",
			RequestID: "request-failed", Change: map[string]any{"upstream": "do-not-expose"},
		},
		{
			ID: "audit-5", OccurredAt: base.Add(4 * time.Second), Action: "auth.login",
			TargetKind: "user", TargetID: "admin", Result: "failure", RequestID: "ignored",
		},
	}
	for _, event := range events {
		if err := server.store.AppendAudit(event, 10_000); err != nil {
			t.Fatal(err)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?limit=3", nil)
	request.Host = "panel.test"
	request.AddCookie(sessionCookie)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("jobs failed: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret") ||
		strings.Contains(response.Body.String(), "do-not-expose") ||
		strings.Contains(response.Body.String(), `"change"`) {
		t.Fatalf("jobs exposed audit change: %s", response.Body.String())
	}
	var page contract.PageResult[contract.Job]
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected three management jobs, got %#v", page.Items)
	}
	if page.Items[0].ID != "request-failed" ||
		page.Items[0].State != contract.JobFailedNeedsAttention ||
		page.Items[1].ID != "request-running" ||
		page.Items[1].State != contract.JobFailedNeedsAttention ||
		page.Items[2].ID != "request-success" ||
		page.Items[2].State != contract.JobSucceeded {
		t.Fatalf("unexpected merged jobs: %#v", page.Items)
	}
	if page.Items[2].StartedAt == nil || page.Items[2].FinishedAt == nil ||
		page.Items[2].Progress != 100 {
		t.Fatalf("successful job lacks timing/progress: %#v", page.Items[2])
	}
}

func TestJobsRequiresSessionAndStrictLimit(t *testing.T) {
	server, tokenPath := newTestServer(t)

	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/jobs", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated jobs returned %d", unauthenticated.Code)
	}

	sessionCookie, _ := bootstrapCookies(t, server, tokenPath)
	for _, path := range []string{
		"/api/v1/jobs?limit=0",
		"/api/v1/jobs?limit=101",
		"/api/v1/jobs?limit=1&limit=2",
		"/api/v1/jobs?cursor=unexpected",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Host = "panel.test"
		request.AddCookie(sessionCookie)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
	}
}

func TestApplicationJobsMapToManagementJobs(t *testing.T) {
	now := time.Date(2026, time.July, 26, 8, 0, 0, 0, time.UTC)
	finished := now.Add(time.Minute)
	jobs := jobsFromAppJobs([]appmarket.AppJob{
		{
			ID: strings.Repeat("a", 32), AppID: "builtin-4", AppName: "Nginx Proxy Manager",
			Action: "install", Status: "failed", Stage: "failed", Progress: 100,
			Message: "port conflict", CreatedAt: now, FinishedAt: &finished,
		},
		{
			ID: strings.Repeat("b", 32), AppID: "builtin-114", AppName: "OpenClaw",
			Action: "manage", Status: "cancelled", Stage: "cancelled", Progress: 100,
			Message: "ended by administrator", CreatedAt: now, FinishedAt: &finished,
		},
	})
	if len(jobs) != 2 || jobs[0].Action != "app.install" ||
		jobs[0].State != contract.JobFailedNeedsAttention ||
		jobs[0].TargetID != "builtin-4" || jobs[0].Error == nil ||
		jobs[0].Error.Detail != "port conflict" ||
		jobs[1].State != contract.JobCancelled || jobs[1].Error != nil {
		t.Fatalf("application job mapping = %#v", jobs)
	}
}

func TestWebEnvironmentJobsMapToManagementJobs(t *testing.T) {
	now := time.Date(2026, time.July, 28, 8, 0, 0, 0, time.UTC)
	finished := now.Add(time.Minute)
	jobs := jobsFromWebEnvironment([]webenv.Job{
		{
			ID: strings.Repeat("b", 32), Action: "restore", Target: "web_20260728080000.tar.gz",
			Status: "needs_attention", Stage: "receipt_missing", Progress: 100,
			Message: "completion receipt missing", CreatedAt: now, FinishedAt: &finished,
		},
	})
	if len(jobs) != 1 || jobs[0].Action != "web.environment.restore" ||
		jobs[0].State != contract.JobFailedNeedsAttention ||
		jobs[0].TargetKind != "web_environment" || jobs[0].Error == nil ||
		jobs[0].Error.Detail != "completion receipt missing" {
		t.Fatalf("web environment job mapping = %#v", jobs)
	}
}
