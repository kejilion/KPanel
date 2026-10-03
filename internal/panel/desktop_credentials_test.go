package panel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/desktopcredentials"
)

// Uses real enrollment/HMAC/Noise on loopback, with no Windows/RDP service.
func desktopCredentialHost(t *testing.T, s *Server) string {
	t.Helper()
	enrollment, err := s.cluster.CreateLightEnrollmentForOrigin("https://panel.test")
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	key, err := cluster.GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	node, err := s.cluster.EnrollLightNodeAtOrigin("198.51.100.10", "https://panel.test", cluster.LightEnrollRequest{
		Token: strings.Trim(fields[len(fields)-1], "'"), Name: "windows", Platform: "windows", NodeVersion: "1.24.0", TerminalPublicKey: base64.RawURLEncoding.EncodeToString(key.Public),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	report := cluster.LightReportRequest{Platform: "windows", Capabilities: []string{"monitoring", "desktop"}, UnavailableMetrics: []string{"load"}, Telemetry: contract.HostTelemetry{CollectedAt: now, OSID: "windows", AgentVersion: "1.24.0"}}
	body, _ := json.Marshal(report)
	reporting, _ := base64.RawURLEncoding.DecodeString(node.ReportingKey)
	auth := cluster.LightReportAuth{Source: "198.51.100.10", NodeID: node.NodeID, Timestamp: strconv.FormatInt(now.Unix(), 10), RequestID: strings.Repeat("d", 32)}
	auth.Signature = cluster.LightRequestSignature(reporting, "POST", "/api/v3/federation/light/report", node.NodeID, auth.Timestamp, auth.RequestID, body)
	if _, err := s.cluster.AcceptLightReport(auth, body, report); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.cluster.ServeFileStream(w, r, "127.0.0.1") }))
	t.Cleanup(ts.Close)
	relay, err := cluster.NewTerminalRelayClient(ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	peer, _ := base64.RawURLEncoding.DecodeString(node.TerminalPeerPublicKey)
	ctx, cancel := context.WithCancel(context.Background())
	done, ready := make(chan error, 1), make(chan struct{})
	go func() {
		done <- relay.RunDesktopStream(ctx, ts.URL, node.NodeID, node.TargetNodeID, key, peer,
			func(context.Context, io.ReadWriteCloser, string) error { return nil }, func() { close(ready) })
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("desktop control did not stop")
		}
	})
	select {
	case <-ready:
	case err := <-done:
		t.Fatal("desktop control failed", err)
	case <-time.After(3 * time.Second):
		t.Fatal("desktop control not ready")
	}
	return node.NodeID
}

func TestDesktopSavedCredentialsHTTPContractAndBoundaries(t *testing.T) {
	s, tokenPath := newTestServer(t)
	cookie, csrf := bootstrapCookies(t, s, tokenPath)
	host := desktopCredentialHost(t, s)
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	post := func(path string, value any) *httptest.ResponseRecorder {
		body, _ := json.Marshal(value)
		return authenticatedRequest(s, http.MethodPost, desktopSessionsPath+path, body, cookie, csrf, headers)
	}
	metadata := map[string]any{"hostId": host}
	if got := post("/credentials/status", metadata); got.Code != 200 || !strings.Contains(got.Body.String(), `"saved":false`) {
		t.Fatal(got.Code, got.Body.String())
	}
	session, _ := s.auth.Authenticate(cookie.Value)
	binding, err := s.desktopCredentialBinding(session.User.ID, host)
	if err != nil {
		t.Fatal(err)
	}
	credentials := desktopcredentials.Credentials{Username: "synthetic-user", Domain: "", Password: "synthetic-password-not-real"}
	other := binding
	other.UserID = "another-panel-user"
	if err := s.desktopCredentials.Put(other, credentials); err != nil {
		t.Fatal(err)
	}
	if got := post("/credentials/status", metadata); strings.Contains(got.Body.String(), credentials.Username) {
		t.Fatal("cross-user metadata exposed")
	}
	if got := post("", map[string]any{"hostId": host, "useSavedCredentials": true}); got.Code != 409 || !strings.Contains(got.Body.String(), "desktop_credentials_missing") {
		t.Fatal(got.Code, got.Body.String())
	}
	save := map[string]any{"hostId": host, "username": credentials.Username, "domain": credentials.Domain, "password": credentials.Password}
	body, _ := json.Marshal(save)
	for _, bad := range []map[string]string{
		{"Origin": "http://evil.test", "X-CSRF-Token": csrf.Value}, {"Origin": "http://panel.test"}, {"X-CSRF-Token": csrf.Value},
	} {
		got := authenticatedRequest(s, http.MethodPost, desktopSessionsPath+"/credentials/save", body, cookie, csrf, bad)
		if got.Code < 400 {
			t.Fatal("invalid Origin/CSRF admitted")
		}
	}
	if got := post("/credentials/save", save); got.Code != 200 || strings.Contains(got.Body.String(), credentials.Password) || got.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("save response", got.Code, got.Body.String())
	}
	if got := post("/credentials/status", metadata); got.Code != 200 || !strings.Contains(got.Body.String(), credentials.Username) || strings.Contains(got.Body.String(), "password") {
		t.Fatal("status exposed password or lost account")
	}
	manual := post("", metadata)
	if manual.Code != 201 || strings.Contains(manual.Body.String(), "credentials") {
		t.Fatal("manual open exposed saved account", manual.Code, manual.Body.String())
	}
	var opened struct {
		SessionID   string                          `json:"sessionId"`
		Credentials *desktopcredentials.Credentials `json:"credentials"`
	}
	if err := json.Unmarshal(manual.Body.Bytes(), &opened); err != nil {
		t.Fatal(err)
	}
	if got := post("/"+opened.SessionID+"/close", nil); got.Code != 200 {
		t.Fatal(got.Code)
	}
	auto := post("", map[string]any{"hostId": host, "useSavedCredentials": true})
	if auto.Code != 201 || auto.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(auto.Code, auto.Body.String())
	}
	if err := json.Unmarshal(auto.Body.Bytes(), &opened); err != nil || opened.Credentials == nil || *opened.Credentials != credentials {
		t.Fatal("saved open contract", err)
	}
	if got := post("/"+opened.SessionID+"/close", nil); got.Code != 200 {
		t.Fatal(got.Code)
	}
	// Runtime disable remains authoritative even with a valid saved account.
	if err := s.cluster.SetDesktopAllowed(host, false); err != nil {
		t.Fatal(err)
	}
	if got := post("", map[string]any{"hostId": host, "useSavedCredentials": true}); got.Code != 409 || strings.Contains(got.Body.String(), credentials.Password) {
		t.Fatal("disabled host disclosed credential")
	}
	if got := post("/credentials/clear", metadata); got.Code != 200 {
		t.Fatal(got.Code)
	}
	if got := post("/credentials/status", metadata); !strings.Contains(got.Body.String(), `"saved":false`) {
		t.Fatal("clear failed")
	}
	if _, err := s.desktopCredentials.Get(other); err != nil {
		t.Fatal("clear removed another user's credentials", err)
	}
	// Neither ordinary Panel backup nor its identity/audit JSON includes logins.
	files, err := readPanelBackupFiles(s.config.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		if strings.Contains(name, "desktop-credentials") || strings.Contains(string(data), credentials.Password) {
			t.Fatal("saved password entered backup")
		}
	}
	state, _ := os.ReadFile(s.config.StorePath)
	if strings.Contains(string(state), credentials.Password) || strings.Contains(string(state), credentials.Username) {
		t.Fatal("saved account entered ordinary state/audit")
	}
}

func TestDesktopCredentialsSecurityRevisionAndVaultFailure(t *testing.T) {
	s, tokenPath := newTestServer(t)
	cookie, csrf := bootstrapCookies(t, s, tokenPath)
	host := desktopCredentialHost(t, s)
	session, _ := s.auth.Authenticate(cookie.Value)
	binding, _ := s.desktopCredentialBinding(session.User.ID, host)
	credentials := desktopcredentials.Credentials{Username: "synthetic", Password: "not-a-real-password"}
	if err := s.desktopCredentials.Put(binding, credentials); err != nil {
		t.Fatal(err)
	}
	available := s.desktopCredentials
	s.desktopCredentials = nil
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}
	body, _ := json.Marshal(map[string]string{"hostId": host})
	for _, path := range []string{"/credentials/status", "/credentials/save", "/credentials/clear"} {
		if got := authenticatedRequest(s, http.MethodPost, desktopSessionsPath+path, body, cookie, csrf, headers); got.Code != 503 {
			t.Fatal("missing vault admitted", path, got.Code)
		}
	}
	savedBody, _ := json.Marshal(map[string]any{"hostId": host, "useSavedCredentials": true})
	if got := authenticatedRequest(s, http.MethodPost, desktopSessionsPath, savedBody, cookie, csrf, headers); got.Code != 503 {
		t.Fatal("missing vault saved login", got.Code)
	}
	manual := authenticatedRequest(s, http.MethodPost, desktopSessionsPath, body, cookie, csrf, headers)
	if manual.Code != 201 || strings.Contains(manual.Body.String(), "credentials") {
		t.Fatal("missing vault broke manual login", manual.Code)
	}
	s.closeDesktopSessions()
	s.desktopCredentials = available
	user, _ := s.store.UserByID(session.User.ID)
	if err := s.store.ReplaceUserUsername(user.ID, user.Username, "new-test-name", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.savedDesktopCredentials(user.ID, host); err != desktopcredentials.ErrMissing {
		t.Fatal("security revision retained old login", err)
	}
	// The previous security change revoked this login; all access is rejected.
	if got := authenticatedRequest(s, http.MethodPost, desktopSessionsPath+"/credentials/status", body, cookie, csrf, headers); got.Code != 401 {
		t.Fatal("revoked login admitted", got.Code)
	}
	if _, err := os.Stat(filepath.Join(s.config.DataDir, "desktop-credentials", "key")); err != nil {
		t.Fatal(err)
	}
}
