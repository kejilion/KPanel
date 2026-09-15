package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestClusterBatchActionBackendForwardsOnlyFixedTypedActions(t *testing.T) {
	server, _ := newTestServer(t)
	now := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	tests := []struct {
		action cluster.BatchAction
		input  contract.SystemActionRequest
	}{
		{cluster.BatchActionSystemUpdate, contract.SystemActionRequest{Action: "update", MaintenancePolicy: "full"}},
		{cluster.BatchActionCleanupCache, contract.SystemActionRequest{Action: "cleanup", MaintenancePolicy: "cache"}},
		{cluster.BatchActionCleanupStandard, contract.SystemActionRequest{Action: "cleanup", MaintenancePolicy: "standard"}},
		{cluster.BatchActionLogsRetain7Days, contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "retain-7d"}},
		{cluster.BatchActionLogsRetain3Days, contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "retain-3d"}},
		{cluster.BatchActionLogsMax500MiB, contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "max-500m"}},
		{cluster.BatchActionReboot, contract.SystemActionRequest{Action: "reboot"}},
	}
	for index, test := range tests {
		t.Run(string(test.action), func(t *testing.T) {
			taskID := fmt.Sprintf("maintenance-%d", index+1)
			result := contract.SystemActionResult{
				Action: test.input.Action, Status: "accepted", Changed: true, Message: "accepted",
				TaskID: taskID, MaintenancePolicy: test.input.MaintenancePolicy, AppliedAt: now,
			}
			if test.action == cluster.BatchActionReboot {
				result.TaskID = ""
			}
			body, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}}
			backend := &clusterBatchActionBackend{agent: agent, store: server.store, localNodeID: "local-node", now: func() time.Time { return now }}
			operationID := fmt.Sprintf("%032x", index+1)
			execution, err := backend.Submit(context.Background(), cluster.BatchActionInvocation{
				Action: test.action, OperationID: operationID, ControllerID: "local-node",
			})
			if err != nil {
				t.Fatal(err)
			}
			calls := agent.snapshotCalls()
			if len(calls) != 1 || calls[0].method != http.MethodPost || calls[0].path != "/v1/system/actions" || calls[0].request != operationID {
				t.Fatalf("unexpected Agent calls: %#v", calls)
			}
			var forwarded contract.SystemActionRequest
			if err := json.Unmarshal(calls[0].body, &forwarded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(forwarded, test.input) {
				t.Fatalf("forwarded request = %#v, want %#v", forwarded, test.input)
			}
			if test.action == cluster.BatchActionReboot {
				if execution.State != cluster.BatchTargetSucceeded || execution.Stage != "scheduled" || execution.ExecutionID != "" {
					t.Fatalf("unexpected reboot receipt: %#v", execution)
				}
			} else if execution.State != cluster.BatchTargetRunning || execution.ExecutionID != taskID {
				t.Fatalf("unexpected maintenance receipt: %#v", execution)
			}
			events, _ := server.store.ListAudit(200, "")
			var intent, accepted *map[string]any
			for eventIndex := range events {
				event := &events[eventIndex]
				if event.RequestID != operationID {
					continue
				}
				if event.ActorType != "system" || event.Action != "cluster.batch.target."+string(test.action) {
					t.Fatalf("unexpected target audit: %#v", event)
				}
				switch event.Result {
				case "intent":
					intent = &event.Change
				case "accepted":
					accepted = &event.Change
				}
			}
			if intent == nil || accepted == nil {
				t.Fatalf("missing audit pair for %s: %#v", operationID, events)
			}
			if _, exists := (*intent)["executionId"]; exists {
				t.Fatalf("intent audit was mutated after persistence: %#v", *intent)
			}
		})
	}

	agent := &stubAgent{}
	backend := &clusterBatchActionBackend{agent: agent, store: server.store}
	if _, err := backend.Submit(context.Background(), cluster.BatchActionInvocation{
		Action: "shell", OperationID: strings.Repeat("a", 32), ControllerID: "remote",
	}); err != cluster.ErrBatchTaskUnsupported {
		t.Fatalf("arbitrary action error = %v", err)
	}
	if len(agent.snapshotCalls()) != 0 {
		t.Fatal("arbitrary action reached Agent")
	}
}

func TestClusterBatchActionBackendPreservesAmbiguousSubmissionOutcomes(t *testing.T) {
	server, _ := newTestServer(t)
	operationID := strings.Repeat("c", 32)
	agent := &stubAgent{err: errors.New("connection closed after write")}
	backend := &clusterBatchActionBackend{agent: agent, store: server.store, localNodeID: "local-node"}
	if execution, err := backend.Submit(context.Background(), cluster.BatchActionInvocation{
		Action: cluster.BatchActionSystemUpdate, OperationID: operationID, ControllerID: "local-node",
	}); err == nil || execution.State != "" {
		t.Fatalf("transport ambiguity = %#v, %v", execution, err)
	}
	events, _ := server.store.ListAudit(200, "")
	foundUnknown := false
	for _, event := range events {
		if event.RequestID == operationID && event.Result == "unknown" {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatalf("ambiguous submission audit missing: %#v", events)
	}

	result := contract.SystemActionResult{Action: "update", Status: "accepted"}
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	agent = &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, Body: body}}
	backend.agent = agent
	attention, err := backend.Submit(context.Background(), cluster.BatchActionInvocation{
		Action: cluster.BatchActionSystemUpdate, OperationID: strings.Repeat("d", 32), ControllerID: "local-node",
	})
	if err != nil || attention.State != cluster.BatchTargetNeedsAttention || attention.ErrorCode != "agent_response_invalid" {
		t.Fatalf("accepted action without tracking ID = %#v, %v", attention, err)
	}

	result.TaskID = "maintenance-wrong-policy"
	result.MaintenancePolicy = "cache"
	body, _ = json.Marshal(result)
	agent.response.Body = body
	attention, err = backend.Submit(context.Background(), cluster.BatchActionInvocation{
		Action: cluster.BatchActionSystemUpdate, OperationID: strings.Repeat("e", 32), ControllerID: "local-node",
	})
	if err != nil || attention.State != cluster.BatchTargetNeedsAttention || attention.ErrorCode != "agent_response_invalid" {
		t.Fatalf("accepted action with mismatched policy = %#v, %v", attention, err)
	}
}

func TestClusterBatchActionBackendTracksExactMaintenanceIdentity(t *testing.T) {
	server, _ := newTestServer(t)
	operationID := strings.Repeat("b", 32)
	executionID := "maintenance-exact"
	summary := contract.SystemSummary{}
	summary.Management.Maintenance = contract.SystemMaintenanceSummary{
		ID: executionID, State: "running", Action: "cleanup", Policy: "standard",
		Stage: "packages", Progress: 42, Message: "working",
	}
	body, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	agent := &stubAgent{response: AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}}
	backend := &clusterBatchActionBackend{agent: agent, store: server.store}
	execution, err := backend.Status(context.Background(), cluster.BatchActionInvocation{
		Action: cluster.BatchActionCleanupStandard, OperationID: operationID, ControllerID: "remote-controller",
	}, executionID)
	if err != nil || execution.State != cluster.BatchTargetRunning || execution.Progress != 42 || execution.ExecutionID != executionID {
		t.Fatalf("status = %#v, %v", execution, err)
	}

	summary.Management.Maintenance.Policy = "cache"
	body, _ = json.Marshal(summary)
	agent.response.Body = body
	mismatch, err := backend.Status(context.Background(), cluster.BatchActionInvocation{
		Action: cluster.BatchActionCleanupStandard, OperationID: operationID, ControllerID: "remote-controller",
	}, executionID)
	if err != nil || mismatch.State != cluster.BatchTargetNeedsAttention || mismatch.ErrorCode != "batch_execution_mismatch" {
		t.Fatalf("mismatched status = %#v, %v", mismatch, err)
	}

	agent.err = errors.New("temporary Agent transport failure")
	transport, err := backend.Status(context.Background(), cluster.BatchActionInvocation{
		Action: cluster.BatchActionCleanupStandard, OperationID: operationID, ControllerID: "remote-controller",
	}, executionID)
	if err == nil || transport.State != "" {
		t.Fatalf("transient status transport failure became a final state: %#v, %v", transport, err)
	}
}
