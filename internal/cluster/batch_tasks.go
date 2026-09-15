package cluster

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func BatchTaskCatalogInfo() BatchTaskCatalog {
	return BatchTaskCatalog{
		Actions: []BatchActionDefinition{
			{ID: BatchActionRefresh, Risk: BatchActionRiskRead, SupportsLegacyPanels: true},
			{ID: BatchActionSystemUpdate, Risk: BatchActionRiskWrite, RequiresTaskScope: true},
			{ID: BatchActionCleanupCache, Risk: BatchActionRiskWrite, RequiresTaskScope: true},
			{ID: BatchActionCleanupStandard, Risk: BatchActionRiskWrite, RequiresTaskScope: true},
			{ID: BatchActionLogsRetain7Days, Risk: BatchActionRiskWrite, RequiresTaskScope: true},
			{ID: BatchActionLogsRetain3Days, Risk: BatchActionRiskWrite, RequiresTaskScope: true},
			{ID: BatchActionLogsMax500MiB, Risk: BatchActionRiskWrite, RequiresTaskScope: true},
			{ID: BatchActionReboot, Risk: BatchActionRiskDisruptive, RequiresTaskScope: true},
		},
		Limits: BatchTaskLimits{
			MaxTasks: maxBatchTasks, MaxActiveTasks: maxActiveBatchTasks,
			MaxTargets: maxBatchTargets, MaxConcurrency: maxBatchConcurrency,
			MinTimeoutSeconds: minBatchTimeoutSeconds, MaxTimeoutSeconds: maxBatchTimeoutSeconds,
			DefaultTimeoutSeconds: defaultBatchTimeout,
		},
	}
}

func (s *Service) BatchTasks() BatchTaskList {
	items := s.batchTasks.List()
	for index := range items {
		items[index].Targets = nil
	}
	return BatchTaskList{Items: items, Total: len(items)}
}

func (s *Service) BatchTask(id string) (BatchTask, error) {
	if !validID(id) {
		return BatchTask{}, ErrNotFound
	}
	return s.batchTasks.Get(id)
}

func (s *Service) CreateBatchTask(ctx context.Context, input CreateBatchTaskInput) (BatchTask, error) {
	return s.createBatchTask(ctx, input, "")
}

func (s *Service) createBatchTask(ctx context.Context, input CreateBatchTaskInput, parentID string) (BatchTask, error) {
	if !validBatchAction(input.Action) || len(input.HostIDs) < 1 || len(input.HostIDs) > maxBatchTargets ||
		(parentID != "" && !validID(parentID)) {
		return BatchTask{}, ErrBatchTaskInvalid
	}
	if input.Action == BatchActionReboot && !input.ConfirmDisruptive {
		return BatchTask{}, ErrBatchTaskInvalid
	}
	if input.Concurrency == 0 {
		input.Concurrency = 2
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = defaultBatchTimeout
	}
	if input.Concurrency < 1 || input.Concurrency > maxBatchConcurrency ||
		input.TimeoutSeconds < minBatchTimeoutSeconds || input.TimeoutSeconds > maxBatchTimeoutSeconds {
		return BatchTask{}, ErrBatchTaskInvalid
	}
	seen := make(map[string]struct{}, len(input.HostIDs))
	targets := make([]BatchTaskTarget, 0, len(input.HostIDs))
	executable := 0
	createdAt := s.now().UTC()
	for _, hostID := range input.HostIDs {
		hostID = strings.TrimSpace(hostID)
		if hostID != LocalHostID && !validID(hostID) {
			return BatchTask{}, ErrBatchTaskInvalid
		}
		if _, exists := seen[hostID]; exists {
			return BatchTask{}, ErrBatchTaskInvalid
		}
		seen[hostID] = struct{}{}
		host, err := s.Host(ctx, hostID)
		if err != nil {
			return BatchTask{}, err
		}
		operationID, err := randomHex(16)
		if err != nil {
			return BatchTask{}, err
		}
		target := BatchTaskTarget{
			HostID: host.ID, HostName: cleanDisplayText(host.Name, 80), HostKind: host.Kind,
			OperationID: operationID, State: BatchTargetQueued, Stage: "queued",
		}
		if target.HostName == "" {
			target.HostName = host.ID
		}
		if code, message := s.batchTargetUnsupported(host, input.Action); code != "" {
			target.State = BatchTargetUnsupported
			target.Stage = "unsupported"
			target.Progress = 100
			target.ErrorCode = code
			target.Message = message
			finishedAt := createdAt
			target.FinishedAt = &finishedAt
		} else {
			executable++
		}
		targets = append(targets, target)
	}
	if executable == 0 {
		return BatchTask{}, ErrBatchTaskUnsupported
	}
	id, err := randomHex(16)
	if err != nil {
		return BatchTask{}, err
	}
	task := BatchTask{
		ID: id, ParentTaskID: parentID, Action: input.Action, State: BatchTaskQueued,
		Concurrency: input.Concurrency, TimeoutSeconds: input.TimeoutSeconds,
		Targets: targets, CreatedAt: createdAt,
	}
	recalculateBatchTaskAt(&task, createdAt)
	if err := s.batchTasks.Create(task); err != nil {
		return BatchTask{}, err
	}
	s.launchBatchTask(task.ID)
	return s.batchTasks.Get(task.ID)
}

func (s *Service) RetryBatchTask(ctx context.Context, id string, input RetryBatchTaskInput) (BatchTask, error) {
	original, err := s.BatchTask(id)
	if err != nil {
		return BatchTask{}, err
	}
	if !batchTaskTerminal(original.State) {
		return BatchTask{}, ErrBatchTaskState
	}
	if input.ConfirmDisruptive != (original.Action == BatchActionReboot) {
		return BatchTask{}, ErrBatchTaskInvalid
	}
	hostIDs := make([]string, 0, len(original.Targets))
	for _, target := range original.Targets {
		switch target.State {
		case BatchTargetFailed, BatchTargetCancelled, BatchTargetNeedsAttention, BatchTargetUnsupported:
			hostIDs = append(hostIDs, target.HostID)
		}
	}
	if len(hostIDs) == 0 {
		return BatchTask{}, ErrBatchTaskState
	}
	return s.createBatchTask(ctx, CreateBatchTaskInput{
		Action: original.Action, HostIDs: hostIDs, Concurrency: original.Concurrency,
		TimeoutSeconds: original.TimeoutSeconds, ConfirmDisruptive: input.ConfirmDisruptive,
	}, original.ID)
}

func (s *Service) CancelBatchTask(id string) (BatchTask, error) {
	if !validID(id) {
		return BatchTask{}, ErrNotFound
	}
	task, err := s.batchTasks.Update(id, func(task *BatchTask) error {
		if batchTaskTerminal(task.State) {
			return ErrBatchTaskState
		}
		task.CancelRequested = true
		now := s.now().UTC()
		for index := range task.Targets {
			target := &task.Targets[index]
			if target.State != BatchTargetQueued {
				continue
			}
			target.State = BatchTargetCancelled
			target.Stage = "cancelled"
			target.Progress = 100
			target.Message = "任务在提交到目标主机前已取消"
			target.FinishedAt = &now
		}
		return nil
	})
	if err != nil {
		return BatchTask{}, err
	}
	s.batchMu.Lock()
	cancel := s.batchRuns[id]
	s.batchMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return task, nil
}

func (s *Service) DeleteBatchTask(id string) error {
	if !validID(id) {
		return ErrNotFound
	}
	return s.batchTasks.Delete(id)
}

func (s *Service) batchTargetUnsupported(host Host, action BatchAction) (string, string) {
	if host.Kind != HostKindPanel {
		return "batch_light_node_unsupported", "轻量节点不提供批量系统维护动作"
	}
	if action == BatchActionRefresh {
		return "", ""
	}
	if host.IsLocal {
		if s.batchActions == nil {
			return "batch_action_backend_unavailable", "本机批量维护适配器不可用"
		}
		return "", ""
	}
	if host.FederationProtocol != FederationProtocolV2 {
		return "batch_protocol_upgrade_required", "旧版联邦连接只支持状态读取，需重新配对后执行批量维护"
	}
	if !ScopeAllowsBatchTasks(host.Scope) {
		return "batch_scope_required", "当前连接未授权批量系统维护，需在目标 KPanel 重新配对"
	}
	if _, ok := s.remoteV2.(remoteV2BatchTaskAPI); !ok {
		return "batch_protocol_upgrade_required", "当前中心端不支持批量维护协议"
	}
	return "", ""
}

func (s *Service) resumeBatchTasks() {
	s.batchRecoveryMu.Lock()
	defer s.batchRecoveryMu.Unlock()
	for _, item := range s.batchTasks.List() {
		if batchTaskTerminal(item.State) || s.batchTaskRunActive(item.ID) {
			continue
		}
		_, err := s.batchTasks.Update(item.ID, func(task *BatchTask) error {
			now := s.now().UTC()
			for index := range task.Targets {
				target := &task.Targets[index]
				switch {
				case target.State == BatchTargetSubmitting:
					target.State = BatchTargetNeedsAttention
					target.Stage = "submission_interrupted"
					target.Progress = 100
					target.ErrorCode = "batch_submission_outcome_unknown"
					target.Message = "中心端在目标主机确认接收前中断；为避免重复执行，未自动重放，请先核对目标状态"
					target.FinishedAt = &now
				case task.CancelRequested && target.State == BatchTargetQueued:
					target.State = BatchTargetCancelled
					target.Stage = "cancelled"
					target.Progress = 100
					target.Message = "任务在提交到目标主机前已取消"
					target.FinishedAt = &now
				case task.CancelRequested && target.State == BatchTargetRunning:
					target.State = BatchTargetNeedsAttention
					target.Stage = "tracking_cancelled"
					target.Progress = 100
					target.ErrorCode = "batch_target_continues"
					target.Message = "目标任务已经提交且不能由中心撤销，请在目标主机核对结果"
					target.FinishedAt = &now
				}
			}
			return nil
		})
		if err != nil {
			continue
		}
		current, err := s.batchTasks.Get(item.ID)
		if err == nil && !batchTaskTerminal(current.State) {
			s.launchBatchTask(item.ID)
		}
	}
}

func (s *Service) batchTaskRunActive(id string) bool {
	s.batchMu.Lock()
	defer s.batchMu.Unlock()
	_, exists := s.batchRuns[id]
	return exists
}

func (s *Service) launchBatchTask(id string) {
	s.mu.Lock()
	if !s.started || s.ctx == nil || s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	s.batchMu.Lock()
	if _, exists := s.batchRuns[id]; exists {
		s.batchMu.Unlock()
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.batchRuns[id] = cancel
	s.wg.Add(1)
	s.batchMu.Unlock()
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		defer func() {
			s.batchMu.Lock()
			delete(s.batchRuns, id)
			s.batchMu.Unlock()
			cancel()
		}()
		s.runBatchTask(ctx, id)
	}()
}

func (s *Service) runBatchTask(ctx context.Context, id string) {
	task, err := s.batchTasks.Update(id, func(task *BatchTask) error {
		if task.StartedAt == nil {
			now := s.now().UTC()
			task.StartedAt = &now
		}
		return nil
	})
	if err != nil || batchTaskTerminal(task.State) {
		return
	}
	workers := make(chan struct{}, task.Concurrency)
	var group sync.WaitGroup
	for _, target := range task.Targets {
		if target.State != BatchTargetQueued && target.State != BatchTargetRunning {
			continue
		}
		group.Add(1)
		go func(hostID string) {
			defer group.Done()
			select {
			case workers <- struct{}{}:
				defer func() { <-workers }()
			case <-ctx.Done():
				s.finalizeCancelledBatchTarget(id, hostID)
				return
			}
			select {
			case s.batchSem <- struct{}{}:
				defer func() { <-s.batchSem }()
			case <-ctx.Done():
				s.finalizeCancelledBatchTarget(id, hostID)
				return
			}
			s.runBatchTarget(ctx, id, hostID)
		}(target.HostID)
	}
	group.Wait()
}

func (s *Service) runBatchTarget(ctx context.Context, taskID, hostID string) {
	task, target, err := s.batchTaskTarget(taskID, hostID)
	if err != nil {
		return
	}
	if target.State == BatchTargetRunning && target.ExecutionID != "" {
		s.trackBatchTarget(ctx, task, target)
		return
	}
	if target.State != BatchTargetQueued {
		return
	}
	started := s.now().UTC()
	_, err = s.beginBatchTargetSubmission(taskID, hostID, started)
	if err != nil {
		return
	}
	requestTimeout := 30 * time.Second
	if remaining := time.Duration(task.TimeoutSeconds) * time.Second; remaining < requestTimeout {
		requestTimeout = remaining
	}
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	execution, submitErr := s.submitBatchTarget(requestCtx, task.Action, target)
	cancel()
	if submitErr != nil {
		if ctx.Err() != nil {
			s.finalizeCancelledBatchTarget(taskID, hostID)
			return
		}
		if task.Action != BatchActionRefresh {
			// A transport failure can happen after the target accepted a mutating
			// action but before the receipt reached us. Never label that as safely
			// retryable: doing so could execute the maintenance action twice.
			s.finishBatchTarget(taskID, hostID, BatchTargetNeedsAttention, "submission_unknown", "batch_submission_outcome_unknown", "提交结果无法确认；目标动作可能已经开始，请先到目标主机核对")
			return
		}
		s.finishBatchTarget(taskID, hostID, BatchTargetFailed, "submit_failed", remoteErrorCode(submitErr), remoteErrorMessage(remoteErrorCode(submitErr)))
		return
	}
	switch execution.State {
	case BatchTargetSucceeded:
		s.finishBatchTarget(taskID, hostID, BatchTargetSucceeded, cleanBatchStage(execution.Stage, "completed"), "", execution.Message)
		return
	case BatchTargetFailed:
		s.finishBatchTarget(taskID, hostID, BatchTargetFailed, cleanBatchStage(execution.Stage, "failed"), cleanBatchCode(execution.ErrorCode, "batch_target_failed"), execution.Message)
		return
	case BatchTargetNeedsAttention:
		s.finishBatchTarget(taskID, hostID, BatchTargetNeedsAttention, cleanBatchStage(execution.Stage, "status_unknown"), cleanBatchCode(execution.ErrorCode, "batch_target_status_unknown"), execution.Message)
		return
	}
	if execution.State != BatchTargetRunning || !batchExecutionIDPattern.MatchString(execution.ExecutionID) {
		s.finishBatchTarget(taskID, hostID, BatchTargetNeedsAttention, "invalid_receipt", "batch_target_receipt_invalid", "目标主机返回了无法追踪的任务凭据，请到目标主机核对")
		return
	}
	_, err = s.updateBatchTarget(taskID, hostID, func(target *BatchTaskTarget) {
		target.ExecutionID = execution.ExecutionID
		target.State = BatchTargetRunning
		target.Stage = cleanBatchStage(execution.Stage, "accepted")
		target.Progress = min(max(execution.Progress, 1), 99)
		target.Message = cleanDisplayText(execution.Message, 300)
	})
	if err != nil {
		return
	}
	task, target, err = s.batchTaskTarget(taskID, hostID)
	if err == nil {
		s.trackBatchTarget(ctx, task, target)
	}
}

func (s *Service) beginBatchTargetSubmission(taskID, hostID string, started time.Time) (BatchTask, error) {
	return s.batchTasks.Update(taskID, func(task *BatchTask) error {
		if task.CancelRequested {
			return ErrBatchTaskState
		}
		for index := range task.Targets {
			target := &task.Targets[index]
			if target.HostID != hostID {
				continue
			}
			if target.State != BatchTargetQueued {
				return ErrBatchTaskState
			}
			target.State = BatchTargetSubmitting
			target.Stage = "submitting"
			target.Progress = 2
			target.Message = "正在向目标主机提交固定动作"
			target.StartedAt = &started
			return nil
		}
		return ErrNotFound
	})
}

func (s *Service) trackBatchTarget(ctx context.Context, task BatchTask, target BatchTaskTarget) {
	deadline := task.CreatedAt.Add(time.Duration(task.TimeoutSeconds) * time.Second)
	if target.StartedAt != nil {
		deadline = target.StartedAt.Add(time.Duration(task.TimeoutSeconds) * time.Second)
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			s.finalizeCancelledBatchTarget(task.ID, target.HostID)
			return
		case <-timer.C:
		}
		if !s.now().UTC().Before(deadline) {
			s.finishBatchTarget(task.ID, target.HostID, BatchTargetNeedsAttention, "tracking_timeout", "batch_tracking_timeout", "跟踪已达到安全时限；目标任务可能仍在运行，请到目标主机核对")
			return
		}
		statusTimeout := deadline.Sub(s.now().UTC())
		if statusTimeout > 15*time.Second {
			statusTimeout = 15 * time.Second
		}
		if statusTimeout <= 0 {
			s.finishBatchTarget(task.ID, target.HostID, BatchTargetNeedsAttention, "tracking_timeout", "batch_tracking_timeout", "跟踪已达到安全时限；目标任务可能仍在运行，请到目标主机核对")
			return
		}
		statusCtx, cancel := context.WithTimeout(ctx, statusTimeout)
		execution, err := s.batchTargetStatus(statusCtx, task.Action, target)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				s.finalizeCancelledBatchTarget(task.ID, target.HostID)
				return
			}
			_, _ = s.updateBatchTargetIfChanged(task.ID, target.HostID, func(current *BatchTaskTarget) {
				current.Stage = "waiting_for_status"
				current.Message = "暂时无法读取目标任务状态，系统将继续重试"
			})
			timer.Reset(s.batchPollInterval)
			continue
		}
		if execution.ExecutionID != target.ExecutionID {
			s.finishBatchTarget(task.ID, target.HostID, BatchTargetNeedsAttention, "execution_mismatch", "batch_execution_mismatch", "目标主机返回的维护任务 ID 与批量任务不一致，请人工核对")
			return
		}
		switch execution.State {
		case BatchTargetRunning:
			_, _ = s.updateBatchTargetIfChanged(task.ID, target.HostID, func(current *BatchTaskTarget) {
				current.Stage = cleanBatchStage(execution.Stage, "running")
				current.Progress = min(max(execution.Progress, 1), 99)
				current.Message = cleanDisplayText(execution.Message, 300)
			})
			timer.Reset(s.batchPollInterval)
		case BatchTargetSucceeded:
			s.finishBatchTarget(task.ID, target.HostID, BatchTargetSucceeded, cleanBatchStage(execution.Stage, "completed"), "", execution.Message)
			return
		case BatchTargetFailed:
			s.finishBatchTarget(task.ID, target.HostID, BatchTargetFailed, cleanBatchStage(execution.Stage, "failed"), cleanBatchCode(execution.ErrorCode, "batch_target_failed"), execution.Message)
			return
		case BatchTargetNeedsAttention:
			s.finishBatchTarget(task.ID, target.HostID, BatchTargetNeedsAttention, cleanBatchStage(execution.Stage, "status_unknown"), cleanBatchCode(execution.ErrorCode, "batch_target_status_unknown"), execution.Message)
			return
		default:
			s.finishBatchTarget(task.ID, target.HostID, BatchTargetNeedsAttention, "status_unknown", "batch_target_status_unknown", "目标主机无法确认该任务的最终状态，请人工核对")
			return
		}
	}
}

func (s *Service) batchTaskTarget(taskID, hostID string) (BatchTask, BatchTaskTarget, error) {
	task, err := s.batchTasks.Get(taskID)
	if err != nil {
		return BatchTask{}, BatchTaskTarget{}, err
	}
	for _, target := range task.Targets {
		if target.HostID == hostID {
			return task, target, nil
		}
	}
	return BatchTask{}, BatchTaskTarget{}, ErrNotFound
}

func (s *Service) updateBatchTarget(taskID, hostID string, mutate func(*BatchTaskTarget)) (BatchTask, error) {
	return s.batchTasks.Update(taskID, func(task *BatchTask) error {
		for index := range task.Targets {
			if task.Targets[index].HostID == hostID {
				mutate(&task.Targets[index])
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Service) updateBatchTargetIfChanged(taskID, hostID string, mutate func(*BatchTaskTarget)) (BatchTask, error) {
	task, target, err := s.batchTaskTarget(taskID, hostID)
	if err != nil {
		return BatchTask{}, err
	}
	updated := target
	mutate(&updated)
	if updated == target {
		return task, nil
	}
	return s.updateBatchTarget(taskID, hostID, mutate)
}

func (s *Service) finishBatchTarget(taskID, hostID string, state BatchTargetState, stage, code, message string) {
	_, _ = s.updateBatchTarget(taskID, hostID, func(target *BatchTaskTarget) {
		now := s.now().UTC()
		target.State = state
		target.Stage = cleanBatchStage(stage, string(state))
		target.Progress = 100
		target.ErrorCode = cleanDisplayText(code, 80)
		target.Message = cleanDisplayText(message, 300)
		target.FinishedAt = &now
	})
}

func (s *Service) finalizeCancelledBatchTarget(taskID, hostID string) {
	task, target, err := s.batchTaskTarget(taskID, hostID)
	if err != nil || !task.CancelRequested || batchTargetTerminal(target.State) {
		return
	}
	if target.State == BatchTargetQueued || task.Action == BatchActionRefresh {
		s.finishBatchTarget(taskID, hostID, BatchTargetCancelled, "cancelled", "", "任务在完成前已取消")
		return
	}
	s.finishBatchTarget(taskID, hostID, BatchTargetNeedsAttention, "tracking_cancelled", "batch_target_continues", "目标动作可能已经提交且不能由中心撤销，请在目标主机核对结果")
}

func cleanBatchStage(value, fallback string) string {
	value = cleanDisplayText(value, 80)
	if value == "" {
		return fallback
	}
	return value
}

func cleanBatchCode(value, fallback string) string {
	value = cleanDisplayText(value, 80)
	if value == "" {
		return fallback
	}
	return value
}

func (s *Service) submitBatchTarget(ctx context.Context, action BatchAction, target BatchTaskTarget) (BatchTargetExecution, error) {
	if action == BatchActionRefresh {
		_, err := s.RefreshNow(ctx, target.HostID)
		if err != nil {
			return BatchTargetExecution{}, err
		}
		return BatchTargetExecution{State: BatchTargetSucceeded, Stage: "completed", Progress: 100, Message: "主机状态已重新采集"}, nil
	}
	invocation := BatchActionInvocation{Action: action, OperationID: target.OperationID, ControllerID: s.NodeID()}
	if target.HostID == LocalHostID {
		if s.batchActions == nil {
			return BatchTargetExecution{}, ErrBatchTaskUnsupported
		}
		return s.batchActions.Submit(ctx, invocation)
	}
	return s.submitRemoteBatchTarget(ctx, target.HostID, invocation)
}

func (s *Service) batchTargetStatus(ctx context.Context, action BatchAction, target BatchTaskTarget) (BatchTargetExecution, error) {
	invocation := BatchActionInvocation{Action: action, OperationID: target.OperationID, ControllerID: s.NodeID()}
	if target.HostID == LocalHostID {
		if s.batchActions == nil {
			return BatchTargetExecution{}, ErrBatchTaskUnsupported
		}
		return s.batchActions.Status(ctx, invocation, target.ExecutionID)
	}
	return s.remoteBatchTargetStatus(ctx, target.HostID, invocation, target.ExecutionID)
}

func (s *Service) batchTaskHostCredential(id string) (hostRecordV2, v2Credential, remoteV2BatchTaskAPI, error) {
	record, err := s.storeV2.Host(id)
	remote, ok := s.remoteV2.(remoteV2BatchTaskAPI)
	if err != nil || record.State != hostStateV2Active ||
		!ScopeAllowsBatchTasks(s.effectiveHostScopeV2(record)) || !ok {
		return hostRecordV2{}, v2Credential{}, nil, ErrBatchTaskUnsupported
	}
	credential, err := s.secretsV2.ReadCredential(record.CredentialFile)
	if err != nil {
		return hostRecordV2{}, v2Credential{}, nil, err
	}
	return record, credential, remote, nil
}

func (s *Service) submitRemoteBatchTarget(ctx context.Context, hostID string, invocation BatchActionInvocation) (BatchTargetExecution, error) {
	record, credential, remote, err := s.batchTaskHostCredential(hostID)
	if err != nil {
		return BatchTargetExecution{}, err
	}
	return remote.BatchTaskV2(
		ctx, record.Origin, record.ControllerID, record.RemoteNodeID,
		noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(),
		batchTaskV2Request{Operation: "submit", Action: invocation.Action, OperationID: invocation.OperationID},
	)
}

func (s *Service) remoteBatchTargetStatus(ctx context.Context, hostID string, invocation BatchActionInvocation, executionID string) (BatchTargetExecution, error) {
	record, credential, remote, err := s.batchTaskHostCredential(hostID)
	if err != nil {
		return BatchTargetExecution{}, err
	}
	return remote.BatchTaskV2(
		ctx, record.Origin, record.ControllerID, record.RemoteNodeID,
		noiseKeyV2(credential), credential.TargetPublic, s.now().UTC(),
		batchTaskV2Request{
			Operation: "status", Action: invocation.Action,
			OperationID: invocation.OperationID, ExecutionID: executionID,
		},
	)
}

func validateBatchTargetExecution(result BatchTargetExecution) error {
	if !validBatchTargetState(result.State) || result.Progress < 0 || result.Progress > 100 ||
		(result.ExecutionID != "" && !batchExecutionIDPattern.MatchString(result.ExecutionID)) ||
		cleanDisplayText(result.Stage, 80) != result.Stage ||
		cleanDisplayText(result.Message, 300) != result.Message ||
		cleanDisplayText(result.ErrorCode, 80) != result.ErrorCode {
		return ErrAuthentication
	}
	switch result.State {
	case BatchTargetRunning:
		if result.ExecutionID == "" || result.Progress >= 100 {
			return ErrAuthentication
		}
	case BatchTargetSucceeded, BatchTargetFailed, BatchTargetNeedsAttention:
		if result.Progress != 100 {
			return ErrAuthentication
		}
	default:
		return ErrAuthentication
	}
	return nil
}

func (s *Service) RefreshNow(ctx context.Context, id string) (Host, error) {
	if id == LocalHostID {
		s.invalidateLocalTelemetry()
		return s.localHostSummary(ctx), nil
	}
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if _, err := s.store.Host(id); err == nil {
		s.mu.Lock()
		current := s.runtime[id]
		current.inFlight = true
		s.runtime[id] = current
		s.mu.Unlock()
		s.poll(ctx, id)
		return s.Host(ctx, id)
	} else if !errors.Is(err, ErrNotFound) {
		return Host{}, err
	}
	if _, err := s.storeV2.Host(id); err != nil {
		return Host{}, err
	}
	s.mu.Lock()
	current := s.runtime[id]
	current.inFlight = true
	s.runtime[id] = current
	s.mu.Unlock()
	s.pollV2Locked(ctx, id)
	return s.Host(ctx, id)
}

func recalculateBatchTaskAt(task *BatchTask, now time.Time) {
	wasFinished := task.FinishedAt
	recalculateBatchTask(task)
	if task.FinishedAt != nil && wasFinished == nil {
		finished := now.UTC()
		task.FinishedAt = &finished
	}
}

func batchTaskProgress(task BatchTask) int {
	if task.Total == 0 {
		return 0
	}
	progress := 0
	for _, target := range task.Targets {
		progress += min(max(target.Progress, 0), 100)
	}
	return int(math.Round(float64(progress) / float64(task.Total)))
}

func (s *Service) BatchTaskJobs() []contract.Job {
	tasks := s.batchTasks.List()
	jobs := make([]contract.Job, 0, len(tasks))
	for _, task := range tasks {
		state := contract.JobRunning
		switch task.State {
		case BatchTaskQueued:
			state = contract.JobQueued
		case BatchTaskSucceeded:
			state = contract.JobSucceeded
		case BatchTaskCancelled:
			state = contract.JobCancelled
		case BatchTaskPartial, BatchTaskFailed, BatchTaskNeedsAttention:
			state = contract.JobFailedNeedsAttention
		}
		job := contract.Job{
			ID: "cluster-batch:" + task.ID, Action: "cluster.batch." + string(task.Action), Origin: contract.OriginWeb,
			State: state, Stage: string(task.State), Progress: batchTaskProgress(task),
			TargetKind: "cluster", TargetID: task.ID,
			TargetLabel: fmt.Sprintf("%d 台主机", task.Total), CreatedAt: task.CreatedAt,
			StartedAt: cloneTime(task.StartedAt), FinishedAt: cloneTime(task.FinishedAt),
		}
		if state == contract.JobFailedNeedsAttention {
			job.Error = &contract.Problem{Code: "cluster_batch_task_attention", Title: "集群批量任务需要检查", Retryable: true}
		}
		jobs = append(jobs, job)
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })
	return jobs
}
