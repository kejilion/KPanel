package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

func TestContainerTerminalCannotFallBackToHostShell(t *testing.T) {
	s := testServer(t)
	s.docker = nil
	hostStarts := 0
	s.terminals = terminal.New(terminal.Config{Starter: func(uint16, uint16) (terminal.Process, error) { hostStarts++; return nil, nil }})
	defer s.terminals.CloseAll()
	for _, target := range []struct {
		container, version string
		status             int
	}{
		{strings.Repeat("a", 64), "", http.StatusBadRequest},
		{"", "version", http.StatusBadRequest},
		{strings.Repeat("a", 64), "version", http.StatusServiceUnavailable},
	} {
		body, _ := json.Marshal(map[string]any{"owner": "owner", "rows": 24, "columns": 80, "containerId": target.container, "resourceVersion": target.version})
		request := httptest.NewRequest(http.MethodPost, "/v1/terminals", strings.NewReader(string(body)))
		request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		s.ServeHTTP(response, request)
		if response.Code != target.status {
			t.Fatalf("container target = %d %s", response.Code, response.Body.String())
		}
	}
	if hostStarts != 0 {
		t.Fatalf("container request started %d host shells", hostStarts)
	}
}
