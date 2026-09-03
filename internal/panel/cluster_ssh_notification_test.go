package panel

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestLightReportAdvertisesSSHLoginCapability(t *testing.T) {
	server, _ := newTestServerWithPublicURL(t, "https://panel.test")
	enrollment, err := server.cluster.CreateLightEnrollmentForOrigin("https://panel.test")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	if len(fields) == 0 {
		t.Fatalf("enrollment command is empty: %q", enrollment.Command)
	}
	token := strings.Trim(fields[len(fields)-1], "'")
	lightNode, err := server.cluster.EnrollLightNodeAtOrigin("198.51.100.10", "https://panel.test", cluster.LightEnrollRequest{
		Token: token, Name: "edge-ssh", NodeVersion: "1.0.2",
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	input := cluster.LightReportRequest{Telemetry: contract.HostTelemetry{
		AgentVersion: "1.0.2", AgentProtocolVersion: cluster.LightNodeProtocol,
		Hostname: "edge-ssh", OS: "Linux", CollectedAt: now,
	}}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := base64.RawURLEncoding.DecodeString(lightNode.ReportingKey)
	if err != nil {
		t.Fatal(err)
	}
	requestID := strings.Repeat("a", 32)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	signature := cluster.LightRequestSignature(secret, http.MethodPost, "/api/v3/federation/light/report", lightNode.NodeID, timestamp, requestID, body)
	request := httptest.NewRequest(http.MethodPost, "https://panel.test/api/v3/federation/light/report", bytes.NewReader(body))
	request.Host = "panel.test"
	request.RemoteAddr = "198.51.100.10:12345"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-KPanel-Light-Node-ID", lightNode.NodeID)
	request.Header.Set("X-KPanel-Timestamp", timestamp)
	request.Header.Set("X-KPanel-Request-ID", requestID)
	request.Header.Set("X-KPanel-Signature", signature)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("light report status = %d; body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get(cluster.LightResponseCapabilitiesHeader); got != cluster.SSHLoginCapability {
		t.Fatalf("light report capability header = %q, want %q", got, cluster.SSHLoginCapability)
	}
}
