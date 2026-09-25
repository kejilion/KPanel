package panel

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

type managedJobAgent struct {
	calls          atomic.Int32
	completed      atomic.Bool
	foreignArchive bool
	holdTrash      atomic.Bool
	trashRelease   <-chan struct{}
}

func (a *managedJobAgent) Get(ctx context.Context, path, _, _ string) (AgentResponse, error) {
	if path == "/v1/files/trash" {
		if a.holdTrash.Load() {
			select {
			case <-a.trashRelease:
			case <-ctx.Done():
				return AgentResponse{}, ctx.Err()
			}
		}
		return AgentResponse{StatusCode: 200, Body: []byte(`{"entries":[{"id":"client","originalPath":"/home/web/client/a","resourceVersion":"v1"},{"id":"other","originalPath":"/home/web/other/b","resourceVersion":"v2"}],"total":2}`)}, nil
	}
	if path == "/v1/files/archive-jobs" {
		if a.foreignArchive {
			return AgentResponse{StatusCode: 200, Body: []byte(`{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","target":"/home/web/other/archive.zip","sources":["/home/web/other/file"],"state":"running"}`)}, nil
		}
		return AgentResponse{StatusCode: 200, Body: []byte(`{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","target":"/etc/private/archive.zip","sources":["/etc/private/secret"],"state":"running"}`)}, nil
	}
	state := "running"
	if a.completed.Load() {
		state = "succeeded"
	}
	return AgentResponse{StatusCode: 200, Body: []byte(`{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","status":"` + state + `","progress":50}`)}, nil
}
func (a *managedJobAgent) Do(context.Context, string, string, string, string, []byte) (AgentResponse, error) {
	a.calls.Add(1)
	return AgentResponse{StatusCode: 202, Body: []byte(`{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","status":"queued"}`)}, nil
}

func TestMCPClusterHTTPSApprovalLostReceiptRecoveryAndTargetRevocation(t *testing.T) {
	target, _ := newTestServerWithPublicURL(t, "https://example.com")
	_ = target.cluster.Close()
	var err error
	target.cluster, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}})
	if err != nil {
		t.Fatal(err)
	}
	target.cluster.SetManagedOperationHandler(target.handleManagedClusterOperation, func() bool { return target.mcp.access.Snapshot().Enabled })
	_, err = target.mcp.access.SetEnabled(true, target.mcp.access.Snapshot().ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	deniedRelease := make(chan struct{})
	releaseDenied := sync.OnceFunc(func() { close(deniedRelease) })
	agent := &managedJobAgent{trashRelease: deniedRelease}
	target.agent = agent
	var dropReceipt atomic.Bool
	wire := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded := httptest.NewRecorder()
		target.ServeHTTP(recorded, r)
		if r.URL.Path == cluster.ManagedOperationsV2Path && agent.calls.Load() > 0 && dropReceipt.CompareAndSwap(true, false) {
			w.WriteHeader(503)
			return
		}
		for key, values := range recorded.Header() {
			w.Header()[key] = values
		}
		w.WriteHeader(recorded.Code)
		_, _ = w.Write(recorded.Body.Bytes())
	}))
	defer wire.Close()
	defer releaseDenied()
	roots := x509.NewCertPool()
	roots.AddCert(wire.Certificate())
	remote, err := cluster.NewRemoteClient(cluster.RemoteClientConfig{RootCAs: roots, Resolver: historyTestResolver{}, Dialer: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", wire.Listener.Addr().String())
	}})
	if err != nil {
		t.Fatal(err)
	}
	center, tokenPath := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, center, tokenPath, "https://panel.test")
	_ = center.cluster.Close()
	center.cluster, err = cluster.NewService(cluster.ServiceConfig{DataDir: t.TempDir(), Telemetry: historyTestTelemetry{}, Remote: remote})
	if err != nil {
		t.Fatal(err)
	}
	code, err := target.cluster.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	host, err := center.cluster.AddHost(context.Background(), cluster.AddHostInput{Name: "target", Origin: "https://example.com", PairingCode: code.Code})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := managedPolicy([]string{"apps"}, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = center.mcp.access.SetEnabled(true, center.mcp.access.Snapshot().ResourceVersion)
	_, token, err := center.mcp.access.CreateWithPolicy("Cluster client", []mcpaccess.HostGrant{{ID: host.ID, Identity: center.mcpHostIdentity(host)}}, policy, time.Hour, center.mcp.access.Snapshot().ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	capability := managedCall(center, token, "host_capabilities", map[string]any{"hostId": host.ID})
	if capability["authorized"] != false {
		t.Fatalf("pairing granted control: %#v", capability)
	}
	args := map[string]any{"hostId": host.ID, "requestKey": "install-1", "appId": "builtin-nginx", "action": "restart", "resourceVersion": "sha256:" + strings.Repeat("a", 64)}
	if got := managedCall(center, token, "host_app_action", args); got != nil || agent.calls.Load() != 0 {
		t.Fatal("write without target grant")
	}
	controller := target.cluster.ManagedControllers()[0]
	if err := target.cluster.SetManagedGrant(controller.ID, cluster.ManagedPolicy{Write: true, OperationVersions: policy.OperationVersions}, time.Hour, target.cluster.ManagedGrants().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	planned := managedCall(center, token, "host_app_action", args)
	if planned["state"] != "pending" || agent.calls.Load() != 0 {
		t.Fatalf("no durable approval: %#v", planned)
	}
	id, digest := planned["operationId"].(string), planned["digest"].(string)
	_, err = center.mcp.operations.Decide(id, digest, "admin", true)
	if err != nil {
		t.Fatal(err)
	}
	// Exhaust target admission without claiming the operation. Retrying the
	// same approved operation after capacity returns must remain possible.
	target.mcp.workerSlots <- struct{}{}
	target.mcp.workerSlots <- struct{}{}
	busy := managedCall(center, token, "operation_execute", map[string]any{"operationId": id, "digest": digest})
	<-target.mcp.workerSlots
	<-target.mcp.workerSlots
	if busy["state"] != "approved" || agent.calls.Load() != 0 {
		t.Fatalf("busy target lost unclaimed approval: %#v", busy)
	}
	dropReceipt.Store(true)
	result := managedCall(center, token, "operation_execute", map[string]any{"operationId": id, "digest": digest})
	deadline := time.Now().Add(3 * time.Second)
	for result["state"] == "executing" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		item, _ := center.mcp.operations.Get(id)
		result = mcpOperationView(item)
	}
	if result["state"] != "unknown" || agent.calls.Load() != 1 {
		t.Fatalf("lost receipt %#v calls=%d", result, agent.calls.Load())
	}
	result = managedCall(center, token, "operation_status", map[string]any{"operationId": id})
	if result["state"] != "submitted" {
		t.Fatalf("did not recover existing remote task: %#v", result)
	}
	agent.completed.Store(true)
	result = managedCall(center, token, "operation_status", map[string]any{"operationId": id})
	if result["state"] != "succeeded" {
		t.Fatalf("owner completion missing: %#v", result)
	}
	managedCall(center, token, "operation_execute", map[string]any{"operationId": id, "digest": digest})
	if agent.calls.Load() != 1 {
		t.Fatal("duplicate remote execution")
	}
	// The target trusts the controller for /home/web, but this particular AI
	// client may only access /home/web/client. ID-only tools must also intersect
	// both grants; checking only request paths would miss these operations.
	files, _ := managedPolicy([]string{"apps", "files"}, true, false, []string{"/home/web"})
	if err := target.cluster.SetManagedGrant(controller.ID, cluster.ManagedPolicy{Write: true, FileRoots: files.FileRoots, OperationVersions: files.OperationVersions}, time.Hour, target.cluster.ManagedGrants().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	narrow, _ := managedPolicy([]string{"files"}, true, false, []string{"/home/web/client"})
	_, narrowToken, err := center.mcp.access.CreateWithPolicy("Narrow files", []mcpaccess.HostGrant{{ID: host.ID, Identity: center.mcpHostIdentity(host)}}, narrow, time.Hour, center.mcp.access.Snapshot().ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	listing := managedCall(center, narrowToken, "host_file_trash_list", map[string]any{"hostId": host.ID})
	encoded, _ := json.Marshal(listing)
	if listing == nil || !strings.Contains(string(encoded), "/home/web/client/a") || strings.Contains(string(encoded), "/home/web/other") {
		t.Fatalf("remote roots not intersected: %s", encoded)
	}
	agent.foreignArchive = true
	if got := managedCall(center, narrowToken, "host_file_archive_job", map[string]any{"hostId": host.ID, "jobId": strings.Repeat("a", 32)}); got != nil {
		t.Fatal("remote job ID escaped client roots")
	}
	foreign := managedCall(center, narrowToken, "host_file_trash_action", map[string]any{"hostId": host.ID, "requestKey": "foreign-trash-1", "action": "trash_delete", "trashIds": []string{"other"}, "expectedResourceVersions": map[string]string{"other": "v2"}})
	if foreign == nil {
		t.Fatal("missing foreign plan")
	}
	foreignID, foreignDigest := foreign["operationId"].(string), foreign["digest"].(string)
	_, _ = center.mcp.operations.Decide(foreignID, foreignDigest, "admin", true)
	// Force the legal asynchronous response: the remote denial cannot finish
	// until operation_execute has returned its in-progress operation.
	agent.holdTrash.Store(true)
	denied := managedCall(center, narrowToken, "operation_execute", map[string]any{"operationId": foreignID, "digest": foreignDigest})
	releaseDenied()
	if denied["state"] != "executing" {
		t.Fatalf("expected delayed remote operation to execute asynchronously: %#v", denied)
	}
	deadline = time.Now().Add(3 * time.Second)
	for denied["state"] == "executing" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		item, err := center.mcp.operations.Get(foreignID)
		if err != nil {
			t.Fatal(err)
		}
		denied = mcpOperationView(item)
	}
	if denied["state"] != "failed" || agent.calls.Load() != 1 {
		t.Fatalf("remote trash ID escaped client roots: %#v", denied)
	}
	_, _ = target.mcp.access.SetEnabled(false, target.mcp.access.Snapshot().ResourceVersion)
	capability = managedCall(center, token, "host_capabilities", map[string]any{"hostId": host.ID})
	if capability["authorized"] != false {
		t.Fatal("disabled target still authorized")
	}
	_, _ = target.mcp.access.SetEnabled(true, target.mcp.access.Snapshot().ResourceVersion)
	if err := target.cluster.RevokeManagedGrant(controller.ID, target.cluster.ManagedGrants().ResourceVersion); err != nil {
		t.Fatal(err)
	}
	args["requestKey"] = "install-2"
	if got := managedCall(center, token, "host_app_action", args); got != nil || agent.calls.Load() != 1 {
		t.Fatal("revoked target grant ignored")
	}
}

func TestMCPJobScopesAndTerminalOffsets(t *testing.T) {
	s, _ := newTestServerWithPublicURL(t, "https://panel.test")
	s.agent = &managedJobAgent{}
	policy, err := managedPolicy([]string{"files"}, true, false, []string{"/home/web"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"host_file_archive_job", "host_file_archive_job_cancel"} {
		if err := s.mcpPreflight(context.Background(), tool, json.RawMessage(`{"jobId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`), policy); err == nil {
			t.Fatal("foreign archive job escaped file roots")
		}
	}
	text := strings.Repeat("x", 20<<10) + "password=do-not-reveal"
	body, _ := json.Marshal(map[string]any{"dataBase64": base64.StdEncoding.EncodeToString([]byte(text)), "nextOffset": 100 + len(text), "finished": true, "inputOpen": false})
	output, err := mcpManagedOutput(body, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	data := output["data"].(map[string]any)
	if data["nextOffset"] != float64(100+(12<<10)) || data["finished"] != false || output["truncated"] != true || data["dataBase64"] != nil {
		t.Fatalf("offset corruption: %#v", data)
	}
	body, _ = json.Marshal(map[string]any{"dataBase64": base64.StdEncoding.EncodeToString([]byte("password=do-not-reveal")), "nextOffset": 22, "finished": true})
	output, err = mcpManagedOutput(body, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(output)
	if strings.Contains(string(encoded), "do-not-reveal") {
		t.Fatal("terminal secret exposed")
	}
}
