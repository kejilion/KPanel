package monitoring

import (
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestProjectFederatedHistoryRemovesURLCredentialsAndQueries(t *testing.T) {
	value := contract.MonitoringHistory{OperatorLatency: []contract.MonitoringOperatorLatencySeries{
		{Address: "https://user:password@check.example:8443/health?token=secret#fragment", Target: "https://user:password@check.example:8443/health?token=secret#fragment"},
		{Address: "192.0.2.15", Target: "192.0.2.15"},
		{Address: "tcp://internal.example:443?password=secret", Target: "tcp://internal.example:443?password=secret"},
	}}
	ProjectFederatedHistory(&value)
	if value.OperatorLatency[0].Address != "check.example:8443" || value.OperatorLatency[0].Target != "check.example:8443" {
		t.Fatalf("HTTP target projection = %#v", value.OperatorLatency[0])
	}
	if value.OperatorLatency[1].Address != "192.0.2.15" || value.OperatorLatency[1].Target != "192.0.2.15" {
		t.Fatalf("plain target projection = %#v", value.OperatorLatency[1])
	}
	if value.OperatorLatency[2].Address != "configured-target" || value.OperatorLatency[2].Target != "configured-target" {
		t.Fatalf("opaque target projection = %#v", value.OperatorLatency[2])
	}
}

func TestProjectFederatedHistoryFallsBackToAddress(t *testing.T) {
	value := contract.MonitoringHistory{OperatorLatency: []contract.MonitoringOperatorLatencySeries{{Address: "https://check.example/ping?key=secret"}}}
	ProjectFederatedHistory(&value)
	if value.OperatorLatency[0].Target != "check.example" || value.OperatorLatency[0].Address != "check.example" {
		t.Fatalf("address fallback projection = %#v", value.OperatorLatency[0])
	}
}
