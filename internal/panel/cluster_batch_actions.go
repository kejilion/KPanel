package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/store"
)

type clusterBatchActionBackend struct {
	agent       agentAPI
	store       *store.Store
	localNodeID string
	now         func() time.Time
}

func (b *clusterBatchActionBackend) Submit(ctx context.Context, invocation cluster.BatchActionInvocation) (cluster.BatchTargetExecution, error) {
	input, ok := clusterBatchSystemAction(invocation.Action)
	if !ok || b == nil || b.agent == nil || b.store == nil || !batchAuditID(invocation.OperationID) {
		return cluster.BatchTargetExecution{}, cluster.ErrBatchTaskUnsupported
	}
	change := map[string]any{"action": invocation.Action}
	if err := b.appendAudit(invocation, "intent", change); err != nil {
		return batchExecutionFailure("audit_unavailable", "审计存储不可用，未向主机提交动作"), nil
	}
	body, err := json.Marshal(input)
	if err != nil {
		_ = b.appendAudit(invocation, "failure", change)
		return batchExecutionFailure("request_encoding_failed", "无法生成固定动作请求"), nil
	}
	response, err := b.agent.Do(ctx, http.MethodPost, "/v1/system/actions", "", invocation.OperationID, body)
	if err != nil {
		_ = b.appendAudit(invocation, "unknown", change)
		return cluster.BatchTargetExecution{}, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		code, message := batchAgentProblem(response)
		_ = b.appendAudit(invocation, "failure", change)
		return batchExecutionFailure(code, message), nil
	}
	var result contract.SystemActionResult
	if strictBatchJSON(response.Body, &result) != nil || result.Action != input.Action ||
		result.MaintenancePolicy != input.MaintenancePolicy {
		_ = b.appendAudit(invocation, "unknown", change)
		return batchExecutionAttention("", "agent_response_invalid", "目标 Agent 返回了无效的动作凭据，动作结果需要人工核对"), nil
	}
	message := safeBatchText(result.Message, 300)
	if result.Status == "accepted" {
		if invocation.Action == cluster.BatchActionReboot {
			_ = b.appendAudit(invocation, "accepted", change)
			return cluster.BatchTargetExecution{
				State: cluster.BatchTargetSucceeded, Stage: "scheduled", Progress: 100, Message: message,
			}, nil
		}
		if !batchExecutionToken(result.TaskID) {
			_ = b.appendAudit(invocation, "unknown", change)
			return batchExecutionAttention("", "agent_response_invalid", "目标 Agent 已接受动作但未返回可追踪的任务 ID，请人工核对"), nil
		}
		change["executionId"] = result.TaskID
		_ = b.appendAudit(invocation, "accepted", change)
		return cluster.BatchTargetExecution{
			ExecutionID: result.TaskID, State: cluster.BatchTargetRunning,
			Stage: "accepted", Progress: 5, Message: message,
		}, nil
	}
	if result.Status != "succeeded" {
		_ = b.appendAudit(invocation, "unknown", change)
		return batchExecutionAttention("", "system_action_status_unknown", "目标 Agent 未确认固定动作的最终状态，请人工核对"), nil
	}
	_ = b.appendAudit(invocation, "success", change)
	return cluster.BatchTargetExecution{
		State: cluster.BatchTargetSucceeded, Stage: "completed", Progress: 100, Message: message,
	}, nil
}

func (b *clusterBatchActionBackend) Status(ctx context.Context, invocation cluster.BatchActionInvocation, executionID string) (cluster.BatchTargetExecution, error) {
	if b == nil || b.agent == nil || !batchExecutionToken(executionID) || !batchAuditID(invocation.OperationID) {
		return cluster.BatchTargetExecution{}, cluster.ErrBatchTaskUnsupported
	}
	response, err := b.agent.Get(ctx, "/v1/system/summary", "", invocation.OperationID)
	if err != nil {
		return cluster.BatchTargetExecution{}, err
	}
	if response.StatusCode != http.StatusOK {
		code, message := batchAgentProblem(response)
		return batchExecutionAttention(executionID, code, message), nil
	}
	var summary contract.SystemSummary
	if strictBatchJSON(response.Body, &summary) != nil {
		return batchExecutionAttention(executionID, "agent_response_invalid", "目标 Agent 返回了无效的系统摘要"), nil
	}
	maintenance := summary.Management.Maintenance
	if maintenance.ID != executionID {
		return batchExecutionAttention(executionID, "batch_execution_replaced", "目标主机已不再保留这条维护任务的当前状态，请人工核对"), nil
	}
	expected, ok := clusterBatchSystemAction(invocation.Action)
	if !ok || maintenance.Action != expected.Action || maintenance.Policy != expected.MaintenancePolicy {
		return batchExecutionAttention(executionID, "batch_execution_mismatch", "目标主机返回的维护动作与批量任务不一致，请人工核对"), nil
	}
	message := safeBatchText(maintenance.Message, 300)
	stage := safeBatchText(maintenance.Stage, 80)
	switch maintenance.State {
	case "running":
		progress := maintenance.Progress
		if progress < 1 {
			progress = 1
		} else if progress > 99 {
			progress = 99
		}
		return cluster.BatchTargetExecution{
			ExecutionID: executionID, State: cluster.BatchTargetRunning,
			Stage: stage, Progress: progress, Message: message,
		}, nil
	case "succeeded":
		return cluster.BatchTargetExecution{
			ExecutionID: executionID, State: cluster.BatchTargetSucceeded,
			Stage: stage, Progress: 100, Message: message,
		}, nil
	case "failed":
		return cluster.BatchTargetExecution{
			ExecutionID: executionID, State: cluster.BatchTargetFailed,
			Stage: stage, Progress: 100, Message: message, ErrorCode: "system_maintenance_failed",
		}, nil
	default:
		return batchExecutionAttention(executionID, "batch_target_status_unknown", "目标 Agent 无法确认这条维护任务的状态"), nil
	}
}

func clusterBatchSystemAction(action cluster.BatchAction) (contract.SystemActionRequest, bool) {
	switch action {
	case cluster.BatchActionSystemUpdate:
		return contract.SystemActionRequest{Action: "update", MaintenancePolicy: "full"}, true
	case cluster.BatchActionCleanupCache:
		return contract.SystemActionRequest{Action: "cleanup", MaintenancePolicy: "cache"}, true
	case cluster.BatchActionCleanupStandard:
		return contract.SystemActionRequest{Action: "cleanup", MaintenancePolicy: "standard"}, true
	case cluster.BatchActionLogsRetain7Days:
		return contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "retain-7d"}, true
	case cluster.BatchActionLogsRetain3Days:
		return contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "retain-3d"}, true
	case cluster.BatchActionLogsMax500MiB:
		return contract.SystemActionRequest{Action: "log-cleanup", MaintenancePolicy: "max-500m"}, true
	case cluster.BatchActionReboot:
		return contract.SystemActionRequest{Action: "reboot"}, true
	default:
		return contract.SystemActionRequest{}, false
	}
}

func (b *clusterBatchActionBackend) appendAudit(invocation cluster.BatchActionInvocation, result string, change map[string]any) error {
	now := time.Now
	if b.now != nil {
		now = b.now
	}
	actorType := "external"
	if invocation.ControllerID == b.localNodeID {
		actorType = "system"
	}
	changeCopy := make(map[string]any, len(change))
	for key, value := range change {
		changeCopy[key] = value
	}
	return b.store.AppendAudit(store.AuditEvent{
		ID: newRequestID(), OccurredAt: now().UTC(), ActorType: actorType,
		ActorID: invocation.ControllerID, Action: "cluster.batch.target." + string(invocation.Action),
		TargetKind: "system", TargetID: string(invocation.Action), Result: result,
		RequestID: invocation.OperationID, Change: changeCopy,
	}, 10_000)
}

func batchExecutionFailure(code, message string) cluster.BatchTargetExecution {
	return cluster.BatchTargetExecution{
		State: cluster.BatchTargetFailed, Stage: "failed", Progress: 100,
		ErrorCode: safeBatchText(code, 80), Message: safeBatchText(message, 300),
	}
}

func batchExecutionAttention(executionID, code, message string) cluster.BatchTargetExecution {
	return cluster.BatchTargetExecution{
		ExecutionID: executionID, State: cluster.BatchTargetNeedsAttention,
		Stage: "status_unknown", Progress: 100,
		ErrorCode: safeBatchText(code, 80), Message: safeBatchText(message, 300),
	}
}

func batchAgentProblem(response AgentResponse) (string, string) {
	code, message := "system_action_failed", "目标 Agent 未能完成固定动作"
	var problem contract.Problem
	if strictBatchJSON(response.Body, &problem) == nil {
		if value := safeBatchText(problem.Code, 80); value != "" {
			code = value
		}
		if value := safeBatchText(problem.Title, 200); value != "" {
			message = value
		}
	}
	return code, message
}

func strictBatchJSON(content []byte, target any) error {
	if len(content) == 0 || len(content) > 1<<20 {
		return errors.New("batch action response size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("batch action response contains multiple JSON values")
	}
	return nil
}

func safeBatchText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || limit < 1 {
		return ""
	}
	result := make([]rune, 0, min(len([]rune(value)), limit))
	for _, character := range value {
		if unicode.IsControl(character) && character != '\t' {
			continue
		}
		result = append(result, character)
		if len(result) == limit {
			break
		}
	}
	return strings.TrimSpace(string(result))
}

func batchAuditID(value string) bool {
	return len(value) == 32 && strings.IndexFunc(value, func(character rune) bool {
		return (character < '0' || character > '9') && (character < 'a' || character > 'f')
	}) == -1
}

func batchExecutionToken(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	return strings.IndexFunc(value, func(character rune) bool {
		return (character < 'A' || character > 'Z') &&
			(character < 'a' || character > 'z') &&
			(character < '0' || character > '9') && character != '.' && character != '_' && character != '-'
	}) == -1
}
