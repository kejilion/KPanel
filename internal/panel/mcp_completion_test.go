package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/mcpaccess"
)

type mcpCompletionAgent struct {
	get      func(string, string) AgentResponse
	writes   int
	response AgentResponse
}

func (a *mcpCompletionAgent) Get(_ context.Context, path, query, _ string) (AgentResponse, error) {
	return a.get(path, query), nil
}
func (a *mcpCompletionAgent) Do(context.Context, string, string, string, string, []byte) (AgentResponse, error) {
	a.writes++
	return a.response, nil
}

func mcpDomainClient(t *testing.T, s *Server, domains []string, write bool) (mcpaccess.Client, string) {
	t.Helper()
	_, _ = s.mcp.access.SetEnabled(true, s.mcp.access.Snapshot().ResourceVersion)
	host, _ := s.cluster.Host(context.Background(), "local")
	roots := []string(nil)
	for _, domain := range domains {
		if domain == "files" {
			roots = []string{"/home/web"}
		}
	}
	policy, err := managedPolicy(domains, write, false, roots)
	if err != nil {
		t.Fatal(err)
	}
	client, token, err := s.mcp.access.CreateWithPolicy("Completion", []mcpaccess.HostGrant{{ID: "local", Identity: s.mcpHostIdentity(host)}}, policy, time.Hour, s.mcp.access.Snapshot().ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	return client, token
}

func TestMCPDirectoryPagesAndDomainIsolation(t *testing.T) {
	s, setup := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, setup, "https://panel.test")
	client, token := mcpDomainClient(t, s, []string{"files"}, false)
	s.agent = &mcpCompletionAgent{get: func(path, query string) AgentResponse {
		if path != "/v1/files" {
			t.Errorf("unexpected path %s", path)
		}
		q, _ := url.ParseQuery(query)
		offset, _ := strconv.Atoi(q.Get("offset"))
		limit, _ := strconv.Atoi(q.Get("limit"))
		items := []map[string]any{}
		end := min(offset+limit, 73)
		for i := offset; i < end; i++ {
			items = append(items, map[string]any{"name": fmt.Sprintf("file-%d", i), "path": fmt.Sprintf("/home/web/file-%d", i)})
		}
		body, _ := json.Marshal(map[string]any{"entries": items, "offset": offset, "nextOffset": end, "truncated": end < 73})
		return AgentResponse{StatusCode: 200, Body: body}
	}}
	list := mcpRPC(s, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`).Body.String()
	for _, tool := range []string{"containers_list", "sites_list", "apps_list", "host_summary"} {
		if strings.Contains(list, `"name":"`+tool+`"`) {
			t.Fatalf("discovered unauthorized %s", tool)
		}
		principal, _ := s.mcp.access.LookupClient(client.ID)
		call := mcpRequest{principal: principal, request: httptest.NewRequest("POST", "/mcp", nil)}
		ctx := context.WithValue(context.Background(), mcpContextKey{}, call)
		if result := s.callMCPTool(ctx, tool, json.RawMessage(`{"hostId":"local"}`)); !result.IsError {
			t.Fatalf("direct dispatch escaped domain: %s", tool)
		}
	}
	seen := map[string]bool{}
	for _, offset := range []int{0, 50} {
		result := managedCall(s, token, "host_file_list", map[string]any{"hostId": "local", "path": "/home/web", "limit": 50, "offset": offset})
		if result == nil {
			t.Fatal("no directory result")
		}
		data := result["data"].(map[string]any)
		if data["nextOffset"] != float64(min(offset+50, 73)) {
			t.Fatal(data)
		}
		for _, item := range data["entries"].([]any) {
			name := item.(map[string]any)["name"].(string)
			if seen[name] {
				t.Fatal("duplicate", name)
			}
			seen[name] = true
		}
	}
	if len(seen) != 73 {
		t.Fatalf("lost entries: %d", len(seen))
	}
	if got := managedCall(s, token, "host_file_list", map[string]any{"hostId": "local", "path": "/home/web", "limit": 51}); got != nil {
		t.Fatal("accepted page larger than projection budget")
	}
}

func TestMCPRevocationDuringPreflightPreventsDispatch(t *testing.T) {
	for _, delegated := range []bool{false, true} {
		t.Run(fmt.Sprint(delegated), func(t *testing.T) {
			s, setup := newTestServerWithPublicURL(t, "https://panel.test")
			bootstrapCookiesForOrigin(t, s, setup, "https://panel.test")
			client, _ := mcpDomainClient(t, s, []string{"files"}, true)
			active := true
			agent := &mcpCompletionAgent{get: func(string, string) AgentResponse {
				active = false
				if !delegated {
					_, err := s.mcp.access.Revoke(client.ID, s.mcp.access.Snapshot().ResourceVersion)
					if err != nil {
						t.Error(err)
					}
				}
				return AgentResponse{StatusCode: 200, Body: []byte(`{"target":"/home/web/out","sources":["/home/web/in"]}`)}
			}}
			s.agent = agent
			o, err := s.mcp.operations.Plan(client.ID, "cancel-job-1", "local", client.Hosts[0].Identity, "host_file_archive_job_cancel", json.RawMessage(`{"jobId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`), true)
			if err != nil {
				t.Fatal(err)
			}
			principal, _ := s.mcp.access.LookupClient(client.ID)
			call := mcpRequest{principal: principal, request: httptest.NewRequest("POST", "/mcp", nil)}
			if delegated {
				call.authorize = func() bool { return active }
				call.controllerID = "test-controller"
			}
			result, err := s.executeMCPOperation(context.Background(), call, o.ID, o.Digest)
			if err != nil || result.State != "failed" || agent.writes != 0 {
				t.Fatalf("revoked preflight dispatched: %#v writes=%d err=%v", result, agent.writes, err)
			}
		})
	}
}

func TestMCPTrashScopeAndPartialFailure(t *testing.T) {
	s, setup := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, setup, "https://panel.test")
	_, token := mcpDomainClient(t, s, []string{"files"}, true)
	agent := &mcpCompletionAgent{get: func(string, string) AgentResponse {
		return AgentResponse{StatusCode: 200, Body: []byte(`{"entries":[{"id":"allowed","originalPath":"/home/web/file","resourceVersion":"v1"},{"id":"private","originalPath":"/srv/private","resourceVersion":"v2"}],"total":2}`)}
	}, response: AgentResponse{StatusCode: 200, Body: []byte(`{"succeeded":[],"failed":[{"path":"allowed","detail":"destination exists"}]}`)}}
	s.agent = agent
	listing := managedCall(s, token, "host_file_trash_list", map[string]any{"hostId": "local"})
	encoded, _ := json.Marshal(listing)
	if strings.Contains(string(encoded), "private") || listing["data"].(map[string]any)["total"] != float64(1) {
		t.Fatalf("trash scope: %s", encoded)
	}
	for _, id := range []string{"private", "allowed"} {
		version := "v1"
		if id == "private" {
			version = "v2"
		}
		plan := managedCall(s, token, "host_file_trash_action", map[string]any{"hostId": "local", "requestKey": "trash-" + id, "action": "trash_restore", "trashIds": []string{id}, "expectedResourceVersions": map[string]string{id: version}})
		if plan == nil {
			t.Fatal("missing trash plan")
		}
		opID, digest := plan["operationId"].(string), plan["digest"].(string)
		_, _ = s.mcp.operations.Decide(opID, digest, "admin", true)
		result := managedCall(s, token, "operation_execute", map[string]any{"operationId": opID, "digest": digest})
		if result["state"] != "failed" {
			t.Fatal("partial or denied action reported success", result)
		}
		if id == "private" && agent.writes != 0 {
			t.Fatal("out-of-root trash action executed")
		}
	}
	if agent.writes != 1 {
		t.Fatal("authorized operation was not attempted exactly once")
	}
}

func TestMCPBackupExportUsesRealOwner(t *testing.T) {
	s, setup := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, setup, "https://panel.test")
	s.config.TOTPKeyPath = filepath.Join(s.config.DataDir, "totp.key")
	_, token := mcpDomainClient(t, s, []string{"backups"}, true)
	plan := managedCall(s, token, "host_backup_export", map[string]any{"hostId": "local", "requestKey": "backup-export-1", "password": "long-test-password-5", "modules": []string{"panel"}})
	if plan["state"] != "pending" {
		t.Fatal(plan)
	}
	id, digest := plan["operationId"].(string), plan["digest"].(string)
	_, _ = s.mcp.operations.Decide(id, digest, "admin", true)
	result := managedCall(s, token, "operation_execute", map[string]any{"operationId": id, "digest": digest})
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) && result["state"] != "succeeded" && result["state"] != "failed" {
		time.Sleep(20 * time.Millisecond)
		result = managedCall(s, token, "operation_status", map[string]any{"operationId": id})
	}
	if result["state"] != "succeeded" {
		t.Fatalf("backup owner not completed: %#v", result)
	}
	receipt := result["result"].(map[string]any)
	job := receipt["task"].(map[string]any)
	actual, err := s.backups.Get(job["id"].(string))
	if err != nil || actual.Status != "completed" {
		t.Fatalf("MCP completion disagrees with backup owner: %#v %v", actual, err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "long-test-password-5") {
		t.Fatal("password in operation result")
	}
}

func TestMCPMaintenanceReceiptUsesRealIDs(t *testing.T) {
	for _, id := range []string{"20260919T120000Z-0123abcd", "20260919T120000.123456789Z"} {
		body, _ := json.Marshal(map[string]string{"taskId": id, "status": "accepted"})
		for _, tool := range []string{"host_system_action", "host_system_ssh_defense_action", "host_system_tuning_action"} {
			task := mcpReceiptTask(tool, AgentResponse{StatusCode: http.StatusOK, Body: body})
			if task == nil || task.Owner != "system" || task.ID != id {
				t.Fatal("lost maintenance receipt", tool, task)
			}
		}
	}
}

func TestMCPFullToolDiscoveryFitsTransport(t *testing.T) {
	s, setup := newTestServerWithPublicURL(t, "https://panel.test")
	bootstrapCookiesForOrigin(t, s, setup, "https://panel.test")
	_, token := mcpDomainClient(t, s, []string{"system", "docker", "sites", "apps", "diagnostics", "backups", "files"}, true)
	w := mcpRPC(s, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	var response struct {
		Result struct {
			Tools []struct{ Name string } `json:"tools"`
		} `json:"result"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || len(response.Result.Tools) != len(managedCatalog())+10 {
		t.Fatalf("incomplete discovery: %d tools=%d catalog=%d body=%s", w.Code, len(response.Result.Tools), len(managedCatalog()), w.Body.String())
	}
	if len(response.Result.Tools) > 100 {
		t.Fatal("full profile exceeds supported client tool budget")
	}
	t.Logf("full discovery: %d tools, %d response bytes", len(response.Result.Tools), w.Body.Len())
}
