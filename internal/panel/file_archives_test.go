package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type failingArchiveStreamAgent struct {
	*stubAgent
	err error
}

func (agent *failingArchiveStreamAgent) OpenStream(
	_ context.Context,
	_, _, _, _ string,
	_ io.Reader,
	_ http.Header,
	_ int64,
) (*http.Response, error) {
	return nil, agent.err
}

func TestArchiveProxySessionCSRFAndExactRoute(t *testing.T) {
	for _, route := range []string{"archive-contents", "archive-jobs"} {
		t.Run(route, func(t *testing.T) {
			s, token := newTestServer(t)
			session, csrf := bootstrapCookies(t, s, token)
			a := &fileStubAgent{stubAgent: &stubAgent{response: AgentResponse{StatusCode: 202, ContentType: "application/json", Body: []byte(`{"id":"` + strings.Repeat("a", 32) + `"}`)}}}
			s.agent = a
			a.streamStatus = http.StatusAccepted
			a.streamResponse = a.response.Body
			body := []byte(`{"path":"/site.zip","resourceVersion":"sha256:fixture"}`)
			if route == "archive-jobs" {
				body = []byte(`{"operation":"create","input":{"action":"extract","sources":["/site.zip"],"target":"/","name":"site","format":"tar.gz","expectedResourceVersion":"sha256:fixture"}}`)
			}
			url := "/api/v1/files/" + route
			if response := performRequest(s, http.MethodPost, url, body, nil); response.Code != 401 {
				t.Fatalf("session bypass=%d", response.Code)
			}
			headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test"}
			if response := authenticatedRequest(s, http.MethodPost, url, body, session, csrf, headers); response.Code != 403 {
				t.Fatalf("csrf bypass=%d", response.Code)
			}
			if len(a.snapshotStreamCalls()) != 0 {
				t.Fatal("unauthorized call reached Agent")
			}
			headers["X-CSRF-Token"] = csrf.Value
			if response := authenticatedRequest(s, http.MethodPost, url, body, session, csrf, headers); response.Code != 202 {
				t.Fatalf("forward=%d %s", response.Code, response.Body.String())
			}
			calls := a.snapshotStreamCalls()
			if len(calls) != 1 || calls[0].path != "/v1/files/"+route || calls[0].method != "POST" {
				t.Fatalf("route drift=%+v", calls)
			}
			for _, suffix := range []string{"?unknown=1", "?id=a&id=b"} {
				if response := authenticatedRequest(s, http.MethodPost, url+suffix, body, session, csrf, headers); response.Code != 400 {
					t.Fatalf("query allowed=%d", response.Code)
				}
			}
			if len(a.snapshotStreamCalls()) != 1 {
				t.Fatal("invalid query forwarded")
			}
			if route == "archive-jobs" {
				events, _ := s.store.ListAudit(20, "")
				results := map[string]int{}
				for _, event := range events {
					if event.Action != "file.extract" {
						continue
					}
					results[event.Result]++
					if event.TargetKind != "file" || event.TargetID != "/" || event.Change["sourceCount"] != 1 || event.Change["memberCount"] != 0 {
						t.Fatalf("archive audit identity drift: %#v", event)
					}
					if event.Change["name"] != "site" {
						t.Fatalf("effective extract name missing: %#v", event.Change)
					}
					if _, exists := event.Change["format"]; exists {
						t.Fatalf("unused extract format persisted in audit: %#v", event.Change)
					}
					encoded, _ := json.Marshal(event.Change)
					if strings.Contains(string(encoded), "/site.zip") || strings.Contains(string(encoded), "sha256:fixture") {
						t.Fatalf("archive inputs leaked into audit: %s", encoded)
					}
					if event.Result == "accepted" && (event.Change["jobSource"] != "file-archive" || event.Change["jobId"] != strings.Repeat("a", 32)) {
						t.Fatalf("accepted archive identity missing: %#v", event.Change)
					}
					if event.Result == "intent" {
						if _, exists := event.Change["jobId"]; exists {
							t.Fatalf("intent audit was mutated after acceptance: %#v", event.Change)
						}
					}
				}
				if results["intent"] != 1 || results["accepted"] != 1 || len(results) != 2 {
					t.Fatalf("archive audit results=%v", results)
				}
			}
		})
	}
}

func TestFileArchiveAuditRecordsOnlyEffectiveMetadata(t *testing.T) {
	tests := []struct {
		name       string
		input      contract.FileActionRequest
		wantName   bool
		wantFormat bool
	}{
		{name: "compress", input: contract.FileActionRequest{Action: "compress", Sources: []string{"/private"}, Target: "/safe", Name: "bundle.zip", Format: "zip"}, wantName: true, wantFormat: true},
		{name: "single extract", input: contract.FileActionRequest{Action: "extract", Sources: []string{"/private.zip"}, Target: "/safe", Name: "result", Format: "tar.gz"}, wantName: true},
		{name: "batch extract", input: contract.FileActionRequest{Action: "extract", Sources: []string{"/private-one.zip", "/private-two.tar"}, Target: "/safe", Name: "private-one", Format: "tar.gz"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			action, targetKind, targetID, change := fileArchiveAudit(contract.FileArchiveJobRequest{Operation: "create", Input: &test.input})
			if action != "file."+test.input.Action || targetKind != "file" || targetID != "/safe" || change["sourceCount"] != len(test.input.Sources) || change["memberCount"] != 0 {
				t.Fatalf("audit identity=%q %q %q %#v", action, targetKind, targetID, change)
			}
			_, hasName := change["name"]
			_, hasFormat := change["format"]
			if hasName != test.wantName || hasFormat != test.wantFormat {
				t.Fatalf("effective metadata=%#v", change)
			}
			encoded, _ := json.Marshal(change)
			if strings.Contains(string(encoded), "/private") || strings.Contains(string(encoded), "private-one") {
				t.Fatalf("source metadata leaked into audit: %s", encoded)
			}
		})
	}
}

func TestArchiveCreateRejectsUnsupportedActionBeforeAgent(t *testing.T) {
	s, token := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, token)
	agent := &fileStubAgent{stubAgent: &stubAgent{}}
	s.agent = agent
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	body := []byte(`{"operation":"create","input":{"action":"delete","sources":["/private.zip"],"target":"/"}}`)
	response := authenticatedRequest(s, http.MethodPost, "/api/v1/files/archive-jobs", body, session, csrf, headers)
	if response.Code != http.StatusBadRequest || len(agent.snapshotStreamCalls()) != 0 {
		t.Fatalf("unsupported action status=%d calls=%#v", response.Code, agent.snapshotStreamCalls())
	}
}

func TestArchiveStreamFailureRecordsSafeMatchingOutcome(t *testing.T) {
	s, token := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, token)
	s.agent = &failingArchiveStreamAgent{stubAgent: &stubAgent{}, err: errors.New("private stream error")}
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	body := []byte(`{"operation":"create","input":{"action":"extract","sources":["/private-source.zip"],"target":"/safe","name":"result","archiveEntries":["private-member.txt"],"expectedResourceVersion":"sha256:private"}}`)
	response := authenticatedRequest(s, http.MethodPost, "/api/v1/files/archive-jobs", body, session, csrf, headers)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("unsafe transport response=%d %s", response.Code, response.Body.String())
	}
	events, _ := s.store.ListAudit(20, "")
	results := map[string]int{}
	for _, event := range events {
		if event.Action != "file.extract" {
			continue
		}
		results[event.Result]++
		encoded, _ := json.Marshal(event.Change)
		if event.TargetID != "/safe" || event.Change["sourceCount"] != 1 || event.Change["memberCount"] != 1 || strings.Contains(string(encoded), "private") {
			t.Fatalf("unsafe archive failure audit: %#v", event)
		}
	}
	if results["intent"] != 1 || results["failure"] != 1 || len(results) != 2 {
		t.Fatalf("archive failure audit results=%v", results)
	}
}

func TestArchiveOversizedResponseRecordsFailure(t *testing.T) {
	s, token := newTestServer(t)
	session, csrf := bootstrapCookies(t, s, token)
	agent := &fileStubAgent{stubAgent: &stubAgent{}, streamStatus: http.StatusAccepted, streamResponse: make([]byte, (4<<20)+1)}
	s.agent = agent
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	body := []byte(`{"operation":"clear","id":"` + strings.Repeat("b", 32) + `"}`)
	response := authenticatedRequest(s, http.MethodPost, "/api/v1/files/archive-jobs", body, session, csrf, headers)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("oversized response status=%d", response.Code)
	}
	events, _ := s.store.ListAudit(20, "")
	results := map[string]int{}
	for _, event := range events {
		if event.Action == "file.archive.clear" {
			results[event.Result]++
			if event.TargetKind != "file-archive-job" || event.TargetID != strings.Repeat("b", 32) {
				t.Fatalf("clear audit identity drift: %#v", event)
			}
		}
	}
	if results["intent"] != 1 || results["failure"] != 1 || len(results) != 2 {
		t.Fatalf("oversized response audit results=%v", results)
	}
}
