package cluster

import (
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestTelemetrySSHLoginIsReturnedOnlyForOptedInControllers(t *testing.T) {
	when := time.Date(2026, 8, 31, 16, 0, 0, 0, time.UTC)
	value := contract.HostTelemetry{
		SSHLogin: &contract.SSHLoginEvent{
			ID: "event-1", OccurredAt: when, Username: "root",
			RemoteAddress: "203.0.113.10", Method: "publickey",
		},
	}
	if got := telemetryForFederation(value, ""); got.SSHLogin != nil {
		t.Fatal("telemetryForFederation() returned SSH login without capability")
	}
	if got := telemetryForFederation(value, SSHLoginCapability); got.SSHLogin == nil || got.SSHLogin.ID != "event-1" {
		t.Fatalf("telemetryForFederation() with capability = %#v", got.SSHLogin)
	}
}
