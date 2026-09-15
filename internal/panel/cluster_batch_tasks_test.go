package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func TestClusterBatchTaskHTTPFlowRequiresAuthCSRFAndSupportsLifecycle(t *testing.T) {
	server, tokenPath := newTestServer(t)
	unauthenticated := performRequest(server, http.MethodGet, "/api/v1/cluster/batch-actions", nil, nil)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated catalog returned %d", unauthenticated.Code)
	}
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	catalog := authenticatedRequest(server, http.MethodGet, "/api/v1/cluster/batch-actions", nil, sessionCookie, csrfCookie, nil)
	if catalog.Code != http.StatusOK || !strings.Contains(catalog.Body.String(), `"system-update"`) || strings.Contains(catalog.Body.String(), `"shell"`) {
		t.Fatalf("unexpected catalog: %d %s", catalog.Code, catalog.Body.String())
	}

	body := []byte(`{"action":"refresh","hostIds":["local"],"concurrency":1,"timeoutSeconds":60}`)
	missingCSRF := authenticatedRequest(server, http.MethodPost, "/api/v1/cluster/batch-tasks", body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test",
	})
	if missingCSRF.Code != http.StatusForbidden || !strings.Contains(missingCSRF.Body.String(), "csrf_validation_failed") {
		t.Fatalf("missing CSRF returned %d %s", missingCSRF.Code, missingCSRF.Body.String())
	}
	invalidID := authenticatedRequest(server, http.MethodPost, "/api/v1/cluster/batch-tasks/"+strings.Repeat("z", 32)+"/cancel", nil, sessionCookie, csrfCookie, map[string]string{
		"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if invalidID.Code != http.StatusNotFound {
		t.Fatalf("invalid task ID returned %d %s", invalidID.Code, invalidID.Body.String())
	}
	eventsBeforeCreate, _ := server.store.ListAudit(200, "")
	for _, event := range eventsBeforeCreate {
		if event.Action == "cluster.batch.cancel" {
			t.Fatalf("invalid task ID reached audit storage: %#v", event)
		}
	}
	invalid := authenticatedRequest(server, http.MethodPost, "/api/v1/cluster/batch-tasks", []byte(`{"action":"shell","hostIds":["local"]}`), sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "cluster_batch_task_invalid") {
		t.Fatalf("arbitrary action returned %d %s", invalid.Code, invalid.Body.String())
	}
	reboot := authenticatedRequest(server, http.MethodPost, "/api/v1/cluster/batch-tasks", []byte(`{"action":"reboot","hostIds":["local"]}`), sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if reboot.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unconfirmed reboot returned %d %s", reboot.Code, reboot.Body.String())
	}

	createdResponse := authenticatedRequest(server, http.MethodPost, "/api/v1/cluster/batch-tasks", body, sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if createdResponse.Code != http.StatusAccepted {
		t.Fatalf("create returned %d %s", createdResponse.Code, createdResponse.Body.String())
	}
	var created cluster.BatchTask
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.State != cluster.BatchTaskQueued || created.Total != 1 || len(created.Targets) != 1 {
		t.Fatalf("unexpected created task: %#v", created)
	}
	list := authenticatedRequest(server, http.MethodGet, "/api/v1/cluster/batch-tasks", nil, sessionCookie, csrfCookie, nil)
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), `"targets"`) {
		t.Fatalf("task list leaked target details: %d %s", list.Code, list.Body.String())
	}
	detail := authenticatedRequest(server, http.MethodGet, "/api/v1/cluster/batch-tasks/"+created.ID, nil, sessionCookie, csrfCookie, nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"hostId":"local"`) {
		t.Fatalf("detail returned %d %s", detail.Code, detail.Body.String())
	}
	cancel := authenticatedRequest(server, http.MethodPost, "/api/v1/cluster/batch-tasks/"+created.ID+"/cancel", nil, sessionCookie, csrfCookie, map[string]string{
		"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if cancel.Code != http.StatusAccepted || !strings.Contains(cancel.Body.String(), `"state":"cancelled"`) {
		t.Fatalf("cancel returned %d %s", cancel.Code, cancel.Body.String())
	}
	jobs := authenticatedRequest(server, http.MethodGet, "/api/v1/jobs?limit=50", nil, sessionCookie, csrfCookie, nil)
	if jobs.Code != http.StatusOK ||
		!strings.Contains(jobs.Body.String(), `"id":"cluster-batch:`+created.ID+`"`) ||
		!strings.Contains(jobs.Body.String(), `"source":"cluster-batch"`) {
		t.Fatalf("batch task was not aggregated into jobs: %d %s", jobs.Code, jobs.Body.String())
	}
	jobDetail := authenticatedRequest(server, http.MethodGet, "/api/v1/jobs/cluster-batch/"+created.ID, nil, sessionCookie, csrfCookie, nil)
	if jobDetail.Code != http.StatusOK ||
		!strings.Contains(jobDetail.Body.String(), `"action":"cluster.batch.refresh"`) ||
		!strings.Contains(jobDetail.Body.String(), `"targetLabel":"1 台主机"`) {
		t.Fatalf("batch job detail returned %d %s", jobDetail.Code, jobDetail.Body.String())
	}
	retry := authenticatedRequest(server, http.MethodPost, "/api/v1/cluster/batch-tasks/"+created.ID+"/retry", []byte(`{}`), sessionCookie, csrfCookie, map[string]string{
		"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if retry.Code != http.StatusAccepted || !strings.Contains(retry.Body.String(), `"parentTaskId":"`+created.ID+`"`) {
		t.Fatalf("retry returned %d %s", retry.Code, retry.Body.String())
	}
	var retried cluster.BatchTask
	if err := json.Unmarshal(retry.Body.Bytes(), &retried); err != nil {
		t.Fatal(err)
	}
	secondCancel := authenticatedRequest(server, http.MethodPost, "/api/v1/cluster/batch-tasks/"+retried.ID+"/cancel", nil, sessionCookie, csrfCookie, map[string]string{
		"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if secondCancel.Code != http.StatusAccepted {
		t.Fatalf("retry cancel returned %d %s", secondCancel.Code, secondCancel.Body.String())
	}
	deleted := authenticatedRequest(server, http.MethodDelete, "/api/v1/cluster/batch-tasks/"+created.ID, nil, sessionCookie, csrfCookie, map[string]string{
		"Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value,
	})
	if deleted.Code != http.StatusOK || !strings.Contains(deleted.Body.String(), `"deleted":true`) {
		t.Fatalf("delete returned %d %s", deleted.Code, deleted.Body.String())
	}
	events, _ := server.store.ListAudit(200, "")
	actions := map[string]bool{}
	for _, event := range events {
		actions[event.Action] = true
	}
	for _, action := range []string{"cluster.batch.create", "cluster.batch.cancel", "cluster.batch.retry", "cluster.batch.delete"} {
		if !actions[action] {
			t.Fatalf("missing %s audit in %#v", action, events)
		}
	}
}
