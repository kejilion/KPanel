package contract

import "testing"

func TestAgentHealthCoreReady(t *testing.T) {
	tests := []struct {
		name   string
		health AgentHealth
		ready  bool
	}{
		{name: "healthy", health: AgentHealth{Status: "ok"}, ready: true},
		{
			name: "inconsistent healthy response",
			health: AgentHealth{
				Status: "ok", Reasons: []string{"docker_unavailable"},
			},
		},
		{
			name: "missing optional web root",
			health: AgentHealth{
				Status: "degraded", Reasons: []string{"web_root_unavailable"},
			},
			ready: true,
		},
		{
			name: "Docker unavailable",
			health: AgentHealth{
				Status: "degraded", Reasons: []string{"docker_unavailable"},
			},
		},
		{
			name: "web root and Docker unavailable",
			health: AgentHealth{
				Status:  "degraded",
				Reasons: []string{"web_root_unavailable", "docker_unavailable"},
			},
		},
		{name: "unknown status", health: AgentHealth{Status: "unknown"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := test.health.CoreReady(); actual != test.ready {
				t.Fatalf("CoreReady() = %v, want %v", actual, test.ready)
			}
		})
	}
}
