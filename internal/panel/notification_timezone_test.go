package panel

import (
	"context"
	"net/http"
	"testing"
)

func TestNotificationTimezoneSourceUsesAgentManagementTimezone(t *testing.T) {
	agent := &stubAgent{response: AgentResponse{
		StatusCode:  http.StatusOK,
		ContentType: "application/json",
		Body:        []byte(`{"management":{"timezone":"Asia/Shanghai"}}`),
	}}
	source := newNotificationTimezoneSource(agent)

	if got := source.Location(context.Background()); got.String() != "Asia/Shanghai" {
		t.Fatalf("notification timezone = %q, want Asia/Shanghai", got)
	}
	calls := agent.snapshotCalls()
	if len(calls) != 1 || calls[0].method != http.MethodGet || calls[0].path != "/v1/system/summary" {
		t.Fatalf("unexpected timezone Agent calls: %#v", calls)
	}
	if got := source.Location(context.Background()); got.String() != "Asia/Shanghai" {
		t.Fatalf("cached notification timezone = %q, want Asia/Shanghai", got)
	}
	if got := len(agent.snapshotCalls()); got != 1 {
		t.Fatalf("timezone source did not use cache, calls = %d", got)
	}
}
