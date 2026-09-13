package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/filetransfer"
)

func TestFileTransferJobsAuthValidationAndDurableAcceptance(t *testing.T) {
	s, token := newTestServer(t)
	base := "/api/v1/files/transfer-jobs"
	if response := performRequest(s, http.MethodGet, base, nil, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated=%d", response.Code)
	}
	session, csrf := bootstrapCookies(t, s, token)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	input := filetransfer.Request{ID: strings.Repeat("d", 32), SourceNodeID: strings.Repeat("a", 32), TargetDirectory: "/destination", Items: []filetransfer.Source{{Path: "/one", ResourceVersion: "v1"}}}
	body, _ := json.Marshal(input)
	if response := authenticatedRequest(s, http.MethodPost, base, body, session, csrf, nil); response.Code != http.StatusForbidden {
		t.Fatalf("csrf=%d", response.Code)
	}
	for _, suffix := range []string{"?hostId=other", "?path=%2F", "?unexpected=1"} {
		if response := authenticatedRequest(s, http.MethodPost, base+suffix, body, session, csrf, headers); response.Code != http.StatusBadRequest {
			t.Fatalf("query %s=%d", suffix, response.Code)
		}
	}
	same := input
	same.SourceNodeID = s.cluster.NodeID()
	sameBody, _ := json.Marshal(same)
	if response := authenticatedRequest(s, http.MethodPost, base, sameBody, session, csrf, headers); response.Code != http.StatusConflict {
		t.Fatalf("same host=%d", response.Code)
	}
	invalid := input
	invalid.Items[0].Path = "/../one"
	invalidBody, _ := json.Marshal(invalid)
	input.Items[0].Path = "/one"
	if response := authenticatedRequest(s, http.MethodPost, base, invalidBody, session, csrf, headers); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid path=%d", response.Code)
	}
	accepted := authenticatedRequest(s, http.MethodPost, base, body, session, csrf, headers)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("accept=%d %s", accepted.Code, accepted.Body.String())
	}
	// No usable Agent/remote exists: acceptance survives the HTTP request and the
	// worker records a real failure instead of treating HTTP 202 as completion.
	var jobs []filetransfer.Job
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		jobs, _ = s.fileTransferJobs.List()
		if len(jobs) == 1 && !filetransfer.Active(jobs[0].State) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(jobs) != 1 || jobs[0].State != "error" || !jobs[0].Items[0].Retryable {
		t.Fatalf("result=%+v", jobs)
	}
	duplicate := authenticatedRequest(s, http.MethodPost, base, body, session, csrf, headers)
	if duplicate.Code != http.StatusAccepted || !strings.Contains(duplicate.Body.String(), `"state":"error"`) {
		t.Fatalf("duplicate=%d %s", duplicate.Code, duplicate.Body.String())
	}
	listed := authenticatedRequest(s, http.MethodGet, base, nil, session, csrf, nil)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), input.ID) {
		t.Fatalf("list=%d %s", listed.Code, listed.Body.String())
	}
	if response := authenticatedRequest(s, http.MethodPost, base+"/"+input.ID+"/clear", nil, session, csrf, headers); response.Code != http.StatusNoContent {
		t.Fatalf("clear=%d %s", response.Code, response.Body.String())
	}
	if response := authenticatedRequest(s, http.MethodPost, base+"/"+input.ID+"/cancel", nil, session, csrf, headers); response.Code != http.StatusNotFound {
		t.Fatalf("missing=%d", response.Code)
	}
}
