package panel

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFailureDiagnosticsPreserveStatusAndExcludeSecrets(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	s := &Server{}
	r := httptest.NewRequest("GET", "/api/v1/files?path=private-secret", nil)
	r.Header.Set("Authorization", "Bearer private-secret")
	r = r.WithContext(context.WithValue(r.Context(), requestIDKey, "request-123"))
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		s.writeProblem(w, r, 503, "agent_unavailable", "Unavailable", "private-secret")
		if w.Code != 503 || !strings.Contains(w.Body.String(), "request-123") {
			t.Fatal("problem response changed")
		}
	}
	if strings.Count(logs.String(), "\n") != 1 || s.failures.suppressed != 9 {
		t.Fatal("unbounded diagnostic logging")
	}
	if strings.Contains(logs.String(), "private-secret") || !strings.Contains(logs.String(), "agent_unavailable") || !strings.Contains(logs.String(), "request-123") {
		t.Fatalf("unexpected diagnostics: %s", logs.String())
	}
	logs.Reset()
	s.failures = failureDiagnostics{}
	s.writeAgentResponse(httptest.NewRecorder(), r, AgentResponse{StatusCode: 503, Body: []byte("private-secret")})
	if !strings.Contains(logs.String(), `"layer":"agent"`) || strings.Contains(logs.String(), "private-secret") {
		t.Fatal("agent diagnostic missing or leaked body")
	}
	logs.Reset()
	s.failures = failureDiagnostics{}
	s.writeProblem(httptest.NewRecorder(), r, 401, "unauthorized", "Unauthorized", "")
	if logs.Len() != 0 {
		t.Fatal("normal authentication rejection logged as server failure")
	}
}
