package panel

import (
	"net/http"
	"strings"
	"testing"
)

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
				body = []byte(`{"operation":"create","input":{"action":"extract","sources":["/site.zip"],"target":"/","name":"site","expectedResourceVersion":"sha256:fixture"}}`)
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
		})
	}
}
