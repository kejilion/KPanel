package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type recoveryAgent struct {
	*stubAgent
	get func(context.Context, string) (AgentResponse, error)
}

func (a *recoveryAgent) Get(ctx context.Context, path, query, id string) (AgentResponse, error) {
	if query != "" {
		if path != "/v1/files/archive-jobs" || !strings.HasPrefix(query, "id=") {
			return AgentResponse{}, fmt.Errorf("unexpected query")
		}
		_, _ = a.stubAgent.Get(ctx, path, query, id)
		return a.get(ctx, path+"/"+strings.TrimPrefix(query, "id="))
	}
	_, _ = a.stubAgent.Get(ctx, path, query, id)
	return a.get(ctx, path)
}

func ownerBody(id, state string) string {
	archiveState := state
	if state == "succeeded" {
		archiveState = "complete"
	}
	return fmt.Sprintf(`{"id":%q,"status":%q,"state":%q,"action":"compress","stage":%q,"createdAt":"2026-09-05T00:00:00Z","updatedAt":"2026-09-05T00:00:00Z"}`, id, state, archiveState, state)
}

func TestJobDetailRecoversEveryOwnerBeyondLatest50ReadOnly(t *testing.T) {
	for _, owner := range jobOwners() {
		t.Run(owner.name, func(t *testing.T) {
			s, tokenPath := newTestServer(t)
			session, _ := bootstrapCookies(t, s, tokenPath)
			headers := map[string]string{"Cookie": session.Name + "=" + session.Value}
			id := strings.Repeat("a", 32)
			state := "queued"
			agent := &recoveryAgent{stubAgent: &stubAgent{}}
			agent.get = func(_ context.Context, path string) (AgentResponse, error) {
				body := `{"items":[]}`
				if path == owner.path {
					items := []string{ownerBody(id, state)}
					for i := 1; i <= 60; i++ {
						item := strings.ReplaceAll(ownerBody(fmt.Sprintf("%032x", i), "succeeded"), "2026-09-05", "2026-09-06")
						items = append(items, item)
					}
					body = `{"items":[` + strings.Join(items, ",") + `]}`
				} else if path == owner.path+"/"+id {
					body = ownerBody(id, state)
				}
				return AgentResponse{StatusCode: 200, Body: []byte(body)}, nil
			}
			s.agent = agent
			list := performRequest(s, "GET", "/api/v1/jobs?limit=50", nil, headers)
			if list.Code != 200 || strings.Contains(list.Body.String(), owner.name+":"+id) {
				t.Fatalf("old job must be outside latest50: %s", list.Body.String())
			}
			for _, status := range []string{"queued", "running", "cancelled", "interrupted", "succeeded", "unknown"} {
				state = status
				response := performRequest(s, "GET", "/api/v1/jobs/"+owner.name+"/"+id, nil, headers)
				var job contract.Job
				if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &job) != nil {
					t.Fatalf("detail: %d %s", response.Code, response.Body.String())
				}
				if job.ID != owner.name+":"+id || job.State != ownerJobState(status) {
					t.Fatalf("identity/state drift: %#v", job)
				}
				if owner.name == "file-archive" && job.StartedAt != nil {
					t.Fatalf("archive owner invented a start time: %#v", job.StartedAt)
				}
			}
			for _, call := range agent.snapshotCalls() {
				if call.method != "GET" {
					t.Fatalf("replayed mutation: %#v", call)
				}
			}
		})
	}
}

func TestJobDetailFailureAndIdentityBoundaries(t *testing.T) {
	id := strings.Repeat("a", 32)
	for _, tc := range []struct {
		name, body   string
		status, want int
		err          error
	}{
		{"missing", `{"secret":"hidden"}`, 404, 404, nil},
		{"offline", "", 0, 503, fmt.Errorf("secret offline")},
		{"invalidJSON", "invalid secret", 200, 502, nil},
		{"null", "null", 200, 502, nil},
		{"missingFields", `{"id":"` + id + `"}`, 200, 502, nil},
		{"wrongIdentity", ownerBody(strings.Repeat("b", 32), "succeeded"), 200, 502, nil},
		{"oversize", ownerBody(id, "succeeded") + strings.Repeat(" ", maxJobDetailBytes), 200, 502, nil},
		{"trailing", ownerBody(id, "succeeded") + ` {}`, 200, 502, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, tokenPath := newTestServer(t)
			session, _ := bootstrapCookies(t, s, tokenPath)
			s.agent = &stubAgent{response: AgentResponse{StatusCode: tc.status, Body: []byte(tc.body)}, err: tc.err}
			response := performRequest(s, "GET", "/api/v1/jobs/docker/"+id, nil, map[string]string{"Cookie": session.Name + "=" + session.Value})
			if response.Code != tc.want || strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), `"succeeded"`) {
				t.Fatalf("unsafe response: %d %s", response.Code, response.Body.String())
			}
		})
	}
	s, tokenPath := newTestServer(t)
	session, _ := bootstrapCookies(t, s, tokenPath)
	agent := &stubAgent{}
	s.agent = agent
	for _, path := range []string{"evil/" + id, "docker/short", "docker/" + strings.Repeat("a", 4096), "docker/" + id + "/terminal", "docker/" + id + "?input=1", "docker/%61" + id[1:]} {
		response := performRequest(s, "GET", "/api/v1/jobs/"+path, nil, map[string]string{"Cookie": session.Name + "=" + session.Value})
		if response.Code != 404 {
			t.Fatalf("invalid path accepted: %d", response.Code)
		}
	}
	if response := performRequest(s, "GET", "/api/v1/jobs/docker/"+id, nil, nil); response.Code != 401 {
		t.Fatalf("session bypass: %d", response.Code)
	}
	if len(agent.snapshotCalls()) != 0 {
		t.Fatal("invalid request reached owner")
	}
}

func TestJobsPartialSourcesRetainOnlyConfirmedResults(t *testing.T) {
	for _, failure := range []string{"offline", "invalid", "oversize"} {
		t.Run(failure, func(t *testing.T) {
			s, tokenPath := newTestServer(t)
			session, _ := bootstrapCookies(t, s, tokenPath)
			agent := &recoveryAgent{stubAgent: &stubAgent{}}
			agent.get = func(_ context.Context, path string) (AgentResponse, error) {
				if path == "/v1/docker/jobs" {
					if failure == "offline" {
						return AgentResponse{}, fmt.Errorf("secret")
					}
					body := "invalid secret"
					if failure == "oversize" {
						body = strings.Repeat(" ", maxJobPageBytes+1)
					}
					return AgentResponse{StatusCode: 200, Body: []byte(body)}, nil
				}
				return AgentResponse{StatusCode: 200, Body: []byte(`{"items":[` + ownerBody(strings.Repeat("b", 32), "running") + `]}`)}, nil
			}
			s.agent = agent
			response := performRequest(s, "GET", "/api/v1/jobs", nil, map[string]string{"Cookie": session.Name + "=" + session.Value})
			var page jobsPage
			if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &page) != nil || !page.Partial || len(page.Items) != len(jobOwners())-1 || len(page.Sources) != len(jobOwners())+1 {
				t.Fatalf("partial result lost: %d %s", response.Code, response.Body.String())
			}
			if page.Sources[1].State == "available" || strings.Contains(response.Body.String(), "secret") {
				t.Fatal("source error was hidden or leaked")
			}
			for _, job := range page.Items {
				if strings.HasPrefix(job.ID, "docker:") || job.State != contract.JobRunning {
					t.Fatalf("unconfirmed state: %#v", job)
				}
			}
		})
	}
}

func TestJobsSlowSourceDoesNotConsumeOtherOwnerDeadline(t *testing.T) {
	s, tokenPath := newTestServer(t)
	session, _ := bootstrapCookies(t, s, tokenPath)
	var mu sync.Mutex
	deadlines := map[string]time.Time{}
	agent := &recoveryAgent{stubAgent: &stubAgent{}}
	agent.get = func(ctx context.Context, path string) (AgentResponse, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			return AgentResponse{}, fmt.Errorf("unbounded")
		}
		mu.Lock()
		deadlines[path] = deadline
		mu.Unlock()
		if path == "/v1/docker/jobs" {
			<-ctx.Done()
			return AgentResponse{}, ctx.Err()
		}
		return AgentResponse{StatusCode: 200, Body: []byte(`{"items":[]}`)}, nil
	}
	s.agent = agent
	started := time.Now()
	response := performRequest(s, "GET", "/api/v1/jobs", nil, map[string]string{"Cookie": session.Name + "=" + session.Value})
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"partial":true`) {
		t.Fatalf("deadline swallowed healthy sources: %s", response.Body.String())
	}
	if time.Since(started) > 7*time.Second {
		t.Fatal("owner reads were not bounded/concurrent")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(deadlines) != len(jobOwners()) {
		t.Fatalf("expected every owner to have a bounded source read: %#v", deadlines)
	}
}
