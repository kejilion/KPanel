package cluster

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type batchTelemetryStub struct{}

func (batchTelemetryStub) Telemetry(context.Context) (contract.HostTelemetry, error) {
	return contract.HostTelemetry{Hostname: "batch-test", CollectedAt: time.Now().UTC()}, nil
}

type batchBackendStub struct {
	mu             sync.Mutex
	submitResult   BatchTargetExecution
	submitErr      error
	statusResults  []BatchTargetExecution
	statusErr      error
	statusErrors   []error
	blockStatus    bool
	accepted       chan struct{}
	submitCalls    []BatchActionInvocation
	statusCalls    []BatchActionInvocation
	statusExecIDs  []string
	acceptedClosed bool
	onSubmit       func()
}

func (b *batchBackendStub) Submit(_ context.Context, input BatchActionInvocation) (BatchTargetExecution, error) {
	b.mu.Lock()
	b.submitCalls = append(b.submitCalls, input)
	if b.accepted != nil && !b.acceptedClosed {
		close(b.accepted)
		b.acceptedClosed = true
	}
	result, err, onSubmit := b.submitResult, b.submitErr, b.onSubmit
	b.mu.Unlock()
	if onSubmit != nil {
		onSubmit()
	}
	return result, err
}

func (b *batchBackendStub) Status(ctx context.Context, input BatchActionInvocation, executionID string) (BatchTargetExecution, error) {
	b.mu.Lock()
	b.statusCalls = append(b.statusCalls, input)
	b.statusExecIDs = append(b.statusExecIDs, executionID)
	block := b.blockStatus
	statusErr := b.statusErr
	if len(b.statusErrors) > 0 {
		statusErr = b.statusErrors[0]
		b.statusErrors = b.statusErrors[1:]
	}
	var result BatchTargetExecution
	if len(b.statusResults) > 0 {
		result = b.statusResults[0]
		if len(b.statusResults) > 1 {
			b.statusResults = b.statusResults[1:]
		}
	}
	b.mu.Unlock()
	if block {
		<-ctx.Done()
		return BatchTargetExecution{}, ctx.Err()
	}
	return result, statusErr
}

func (b *batchBackendStub) snapshot() (submits, statuses []BatchActionInvocation, executionIDs []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]BatchActionInvocation(nil), b.submitCalls...),
		append([]BatchActionInvocation(nil), b.statusCalls...),
		append([]string(nil), b.statusExecIDs...)
}

func newBatchTestService(t *testing.T, directory string, backend BatchActionBackend) *Service {
	t.Helper()
	service, err := NewService(ServiceConfig{
		DataDir: directory, PanelVersion: "test", Hostname: "batch-test",
		Telemetry: batchTelemetryStub{}, BatchActions: backend,
		BatchPollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func waitBatchTaskTerminal(t *testing.T, service *Service, id string) BatchTask {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		task, err := service.BatchTask(id)
		if err != nil {
			t.Fatal(err)
		}
		if batchTaskTerminal(task.State) {
			return task
		}
		if time.Now().After(deadline) {
			t.Fatalf("batch task %s did not finish: %#v", id, task)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestBatchTaskCatalogAndValidationExposeOnlyFixedActions(t *testing.T) {
	catalog := BatchTaskCatalogInfo()
	if len(catalog.Actions) != 8 || catalog.Limits.MaxTargets != 50 || catalog.Limits.MaxConcurrency != 4 {
		t.Fatalf("unexpected catalog: %#v", catalog)
	}
	for _, action := range catalog.Actions {
		if !validBatchAction(action.ID) || strings.Contains(string(action.ID), "shell") || strings.Contains(string(action.ID), "command") {
			t.Fatalf("catalog exposed an invalid action: %#v", action)
		}
	}
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), &batchBackendStub{})
	defer service.Close()
	if _, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: "shell", HostIDs: []string{LocalHostID},
	}); !errors.Is(err, ErrBatchTaskInvalid) {
		t.Fatalf("arbitrary action error = %v, want ErrBatchTaskInvalid", err)
	}
	if _, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionReboot, HostIDs: []string{LocalHostID},
	}); !errors.Is(err, ErrBatchTaskInvalid) {
		t.Fatalf("unconfirmed reboot error = %v, want ErrBatchTaskInvalid", err)
	}
}

func TestBatchTaskStateDecoderRequiresExplicitSchemaAndTaskList(t *testing.T) {
	for _, content := range []string{`{}`, `{"schemaVersion":1}`} {
		state := batchTaskPersistedState{SchemaVersion: 1, Tasks: []BatchTask{}}
		if err := decodeBatchTaskState([]byte(content), &state); err == nil {
			t.Fatalf("incomplete persisted state %s was accepted as %#v", content, state)
		}
	}
	var compatible batchTaskPersistedState
	if err := decodeBatchTaskState([]byte(`{"schemaVersion":1,"tasks":[]}`), &compatible); err != nil ||
		compatible.MaintenanceHostIDs == nil || compatible.MaintenanceControllerIDs == nil {
		t.Fatalf("state without sidecar fields was not upgraded safely: %#v, %v", compatible, err)
	}
}

func TestBatchTaskMaintenanceGrantsPersistAndReconcile(t *testing.T) {
	path := filepath.Join(t.TempDir(), batchTaskStateFileName)
	store, err := openBatchTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	hostID := strings.Repeat("8", 32)
	controllerID := strings.Repeat("9", 32)
	if err := store.GrantHostMaintenance(hostID); err != nil {
		t.Fatal(err)
	}
	if err := store.GrantControllerMaintenance(controllerID); err != nil {
		t.Fatal(err)
	}
	reopened, err := openBatchTaskStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.HostMaintenanceGranted(hostID) || !reopened.ControllerMaintenanceGranted(controllerID) {
		t.Fatalf("maintenance grants did not persist: %#v", reopened.state)
	}
	if err := reopened.ReconcileMaintenanceGrants(
		[]hostRecordV2{{ID: hostID}}, nil,
	); err != nil {
		t.Fatal(err)
	}
	if !reopened.HostMaintenanceGranted(hostID) || reopened.ControllerMaintenanceGranted(controllerID) {
		t.Fatalf("maintenance grant reconciliation was not scoped: %#v", reopened.state)
	}
}

func TestBatchTaskLocalLifecyclePersistsAndFeedsJobs(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "cluster")
	backend := &batchBackendStub{
		submitResult:  BatchTargetExecution{ExecutionID: "maintenance-1", State: BatchTargetRunning, Stage: "accepted", Progress: 5},
		statusResults: []BatchTargetExecution{{ExecutionID: "maintenance-1", State: BatchTargetSucceeded, Stage: "completed", Progress: 100, Message: "updated"}},
	}
	service := newBatchTestService(t, directory, backend)
	service.Start(context.Background())
	task, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionSystemUpdate, HostIDs: []string{LocalHostID}, Concurrency: 1, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitBatchTaskTerminal(t, service, task.ID)
	if completed.State != BatchTaskSucceeded || completed.Succeeded != 1 || completed.Targets[0].Message != "updated" {
		t.Fatalf("unexpected completed task: %#v", completed)
	}
	submits, statuses, executionIDs := backend.snapshot()
	if len(submits) != 1 || submits[0].Action != BatchActionSystemUpdate || len(statuses) != 1 ||
		len(executionIDs) != 1 || executionIDs[0] != "maintenance-1" {
		t.Fatalf("unexpected backend calls: submits=%#v statuses=%#v ids=%#v", submits, statuses, executionIDs)
	}
	jobs := service.BatchTaskJobs()
	if len(jobs) != 1 || jobs[0].ID != "cluster-batch:"+task.ID || jobs[0].State != contract.JobSucceeded || jobs[0].Progress != 100 {
		t.Fatalf("unexpected batch job projection: %#v", jobs)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := newBatchTestService(t, directory, backend)
	defer reopened.Close()
	persisted, err := reopened.BatchTask(task.ID)
	if err != nil || persisted.State != BatchTaskSucceeded || persisted.FinishedAt == nil {
		t.Fatalf("persisted task = %#v, %v", persisted, err)
	}
}

func TestBatchTaskStatusRejectsExecutionIDMismatch(t *testing.T) {
	backend := &batchBackendStub{
		submitResult: BatchTargetExecution{
			ExecutionID: "maintenance-expected", State: BatchTargetRunning, Stage: "accepted", Progress: 5,
		},
		statusResults: []BatchTargetExecution{{
			ExecutionID: "maintenance-other", State: BatchTargetSucceeded, Stage: "completed", Progress: 100,
		}},
	}
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), backend)
	defer service.Close()
	service.Start(context.Background())
	task, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionSystemUpdate, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitBatchTaskTerminal(t, service, task.ID)
	if completed.State != BatchTaskNeedsAttention || completed.Targets[0].ErrorCode != "batch_execution_mismatch" {
		t.Fatalf("execution ID mismatch was not rejected: %#v", completed)
	}
}

func TestBatchTaskRetriesTransientStatusReadWithoutResubmitting(t *testing.T) {
	backend := &batchBackendStub{
		submitResult: BatchTargetExecution{ExecutionID: "maintenance-status-retry", State: BatchTargetRunning, Stage: "accepted", Progress: 5},
		statusResults: []BatchTargetExecution{
			{},
			{ExecutionID: "maintenance-status-retry", State: BatchTargetSucceeded, Stage: "completed", Progress: 100},
		},
		statusErrors: []error{errors.New("temporary status transport failure"), nil},
	}
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), backend)
	defer service.Close()
	service.Start(context.Background())
	task, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionCleanupCache, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitBatchTaskTerminal(t, service, task.ID)
	if completed.State != BatchTaskSucceeded {
		t.Fatalf("transient status read ended the task: %#v", completed)
	}
	submits, statuses, _ := backend.snapshot()
	if len(submits) != 1 || len(statuses) != 2 {
		t.Fatalf("status retry calls = submits %#v statuses %#v", submits, statuses)
	}
}

func TestBatchTaskStatusRequestHonorsRemainingTrackingTimeout(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "cluster")
	backend := &batchBackendStub{blockStatus: true}
	service := newBatchTestService(t, directory, backend)
	defer service.Close()
	now := time.Now().UTC()
	startedAt := now.Add(-59*time.Second - 900*time.Millisecond)
	task := BatchTask{
		ID: strings.Repeat("6", 32), Action: BatchActionSystemUpdate,
		Concurrency: 1, TimeoutSeconds: 60, CreatedAt: startedAt,
		Targets: []BatchTaskTarget{{
			HostID: LocalHostID, HostName: "batch-test", HostKind: HostKindPanel,
			OperationID: strings.Repeat("7", 32), State: BatchTargetQueued, Stage: "queued",
		}},
	}
	recalculateBatchTask(&task)
	if err := service.batchTasks.Create(task); err != nil {
		t.Fatal(err)
	}
	if _, err := service.batchTasks.Update(task.ID, func(task *BatchTask) error {
		task.StartedAt = &startedAt
		task.Targets[0].ExecutionID = "near-timeout-maintenance"
		task.Targets[0].State = BatchTargetRunning
		task.Targets[0].Stage = "running"
		task.Targets[0].Progress = 25
		task.Targets[0].StartedAt = &startedAt
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	service.Start(context.Background())
	completed := waitBatchTaskTerminal(t, service, task.ID)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("remaining 100ms tracking timeout took %s", elapsed)
	}
	if completed.State != BatchTaskNeedsAttention || completed.Targets[0].ErrorCode != "batch_tracking_timeout" {
		t.Fatalf("near-deadline task did not time out safely: %#v", completed)
	}
}

func TestBatchTaskMutatingSubmissionErrorNeedsAttentionAndRetryCreatesChild(t *testing.T) {
	backend := &batchBackendStub{submitErr: errors.New("transport stopped")}
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), backend)
	defer service.Close()
	service.Start(context.Background())
	task, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionCleanupCache, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitBatchTaskTerminal(t, service, task.ID)
	if completed.State != BatchTaskNeedsAttention || completed.Targets[0].ErrorCode != "batch_submission_outcome_unknown" {
		t.Fatalf("ambiguous submission was not preserved: %#v", completed)
	}
	retry, err := service.RetryBatchTask(context.Background(), task.ID, RetryBatchTaskInput{})
	if err != nil || retry.ParentTaskID != task.ID || retry.ID == task.ID {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	retried := waitBatchTaskTerminal(t, service, retry.ID)
	if retried.State != BatchTaskNeedsAttention {
		t.Fatalf("unexpected retry state: %#v", retried)
	}
	submits, _, _ := backend.snapshot()
	if len(submits) != 2 || submits[0].OperationID == submits[1].OperationID {
		t.Fatalf("retry did not create an independent invocation: %#v", submits)
	}
}

func TestBatchTaskRebootRetryRequiresFreshDisruptiveConfirmation(t *testing.T) {
	backend := &batchBackendStub{submitResult: BatchTargetExecution{
		State: BatchTargetFailed, Stage: "failed", Progress: 100, ErrorCode: "reboot_rejected",
	}}
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), backend)
	defer service.Close()
	service.Start(context.Background())
	task, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionReboot, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
		ConfirmDisruptive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitBatchTaskTerminal(t, service, task.ID)
	if failed.State != BatchTaskFailed {
		t.Fatalf("reboot fixture did not fail: %#v", failed)
	}
	if _, err := service.RetryBatchTask(context.Background(), task.ID, RetryBatchTaskInput{}); !errors.Is(err, ErrBatchTaskInvalid) {
		t.Fatalf("unconfirmed reboot retry error = %v, want ErrBatchTaskInvalid", err)
	}
	if _, err := service.RetryBatchTask(context.Background(), task.ID, RetryBatchTaskInput{ConfirmDisruptive: true}); err != nil {
		t.Fatalf("confirmed reboot retry failed: %v", err)
	}
}

func TestBatchTaskCancellationDoesNotClaimAcceptedActionWasStopped(t *testing.T) {
	accepted := make(chan struct{})
	backend := &batchBackendStub{
		submitResult: BatchTargetExecution{ExecutionID: "maintenance-cancel", State: BatchTargetRunning, Stage: "accepted", Progress: 5},
		blockStatus:  true, accepted: accepted,
	}
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), backend)
	defer service.Close()
	service.Start(context.Background())
	task, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionLogsRetain7Days, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("target action was not accepted")
	}
	if _, err := service.CancelBatchTask(task.ID); err != nil {
		t.Fatal(err)
	}
	completed := waitBatchTaskTerminal(t, service, task.ID)
	if completed.State != BatchTaskNeedsAttention || !completed.CancelRequested ||
		completed.Targets[0].ErrorCode != "batch_target_continues" {
		t.Fatalf("accepted cancellation was reported dishonestly: %#v", completed)
	}
}

func TestBatchTaskRestartRecoveryDoesNotReplaySubmittingAction(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "cluster")
	backend := &batchBackendStub{}
	service := newBatchTestService(t, directory, backend)
	createdAt := time.Now().UTC()
	task := BatchTask{
		ID: strings.Repeat("a", 32), Action: BatchActionCleanupStandard, State: BatchTaskQueued,
		Concurrency: 1, TimeoutSeconds: 60, CreatedAt: createdAt,
		Targets: []BatchTaskTarget{{
			HostID: LocalHostID, HostName: "batch-test", HostKind: HostKindPanel,
			OperationID: strings.Repeat("b", 32), State: BatchTargetQueued, Stage: "queued",
		}},
	}
	recalculateBatchTask(&task)
	if err := service.batchTasks.Create(task); err != nil {
		t.Fatal(err)
	}
	_, err := service.batchTasks.Update(task.ID, func(task *BatchTask) error {
		task.StartedAt = &createdAt
		task.Targets[0].State = BatchTargetSubmitting
		task.Targets[0].Stage = "submitting"
		task.Targets[0].Progress = 2
		task.Targets[0].StartedAt = &createdAt
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := newBatchTestService(t, directory, backend)
	defer restarted.Close()
	restarted.Start(context.Background())
	recovered := waitBatchTaskTerminal(t, restarted, task.ID)
	if recovered.State != BatchTaskNeedsAttention || recovered.Targets[0].ErrorCode != "batch_submission_outcome_unknown" {
		t.Fatalf("unexpected recovered task: %#v", recovered)
	}
	submits, _, _ := backend.snapshot()
	if len(submits) != 0 {
		t.Fatalf("interrupted submission was replayed: %#v", submits)
	}
}

func TestBatchTaskStoreRejectsOverlappingActiveHost(t *testing.T) {
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), &batchBackendStub{})
	defer service.Close()
	first, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionRefresh, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
	})
	if err != nil || first.State != BatchTaskQueued {
		t.Fatalf("first task = %#v, %v", first, err)
	}
	if _, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionRefresh, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
	}); !errors.Is(err, ErrBatchTaskBusy) {
		t.Fatalf("overlapping task error = %v, want ErrBatchTaskBusy", err)
	}
}

func TestBatchTaskStoreKeepsHostReservedUntilItsWholeTaskFinishes(t *testing.T) {
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), &batchBackendStub{})
	defer service.Close()
	now := time.Now().UTC()
	remoteID := strings.Repeat("a", 32)
	first := BatchTask{
		ID: strings.Repeat("1", 32), Action: BatchActionRefresh, State: BatchTaskQueued,
		Concurrency: 1, TimeoutSeconds: 60, CreatedAt: now,
		Targets: []BatchTaskTarget{
			{HostID: LocalHostID, HostName: "local", HostKind: HostKindPanel, OperationID: strings.Repeat("2", 32), State: BatchTargetQueued, Stage: "queued"},
			{HostID: remoteID, HostName: "remote", HostKind: HostKindPanel, OperationID: strings.Repeat("3", 32), State: BatchTargetQueued, Stage: "queued"},
		},
	}
	recalculateBatchTask(&first)
	if err := service.batchTasks.Create(first); err != nil {
		t.Fatal(err)
	}
	if _, err := service.batchTasks.Update(first.ID, func(task *BatchTask) error {
		task.StartedAt = &now
		task.Targets[0].State = BatchTargetSucceeded
		task.Targets[0].Stage = "completed"
		task.Targets[0].Progress = 100
		task.Targets[0].StartedAt = &now
		task.Targets[0].FinishedAt = &now
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	second := BatchTask{
		ID: strings.Repeat("4", 32), Action: BatchActionRefresh, State: BatchTaskQueued,
		Concurrency: 1, TimeoutSeconds: 60, CreatedAt: now.Add(time.Second),
		Targets: []BatchTaskTarget{{
			HostID: LocalHostID, HostName: "local", HostKind: HostKindPanel,
			OperationID: strings.Repeat("5", 32), State: BatchTargetQueued, Stage: "queued",
		}},
	}
	recalculateBatchTask(&second)
	if err := service.batchTasks.Create(second); !errors.Is(err, ErrBatchTaskBusy) {
		t.Fatalf("completed host in an active task was not reserved: %v", err)
	}
}

func TestBatchTaskCancelledTargetCannotBeResurrectedForSubmission(t *testing.T) {
	backend := &batchBackendStub{}
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), backend)
	defer service.Close()
	task, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionCleanupStandard, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CancelBatchTask(task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.beginBatchTargetSubmission(task.ID, LocalHostID, time.Now().UTC()); !errors.Is(err, ErrBatchTaskState) {
		t.Fatalf("cancelled target submission error = %v, want ErrBatchTaskState", err)
	}
	stored, err := service.BatchTask(task.ID)
	if err != nil || stored.Targets[0].State != BatchTargetCancelled {
		t.Fatalf("cancelled target was resurrected: %#v, %v", stored, err)
	}
	if submits, _, _ := backend.snapshot(); len(submits) != 0 {
		t.Fatalf("cancelled target reached backend: %#v", submits)
	}
}

func TestBatchTaskStoreRollsBackMemoryWhenAtomicPersistenceFails(t *testing.T) {
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), &batchBackendStub{})
	defer service.Close()
	task, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionRefresh, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	originalSync := service.batchTasks.ops.syncDir
	service.batchTasks.ops.syncDir = func(string) error { return errors.New("disk sync failed") }
	if _, err := service.CancelBatchTask(task.ID); err == nil {
		t.Fatal("persistence failure was reported as success")
	}
	service.batchTasks.ops.syncDir = originalSync
	persisted, err := service.BatchTask(task.ID)
	if err != nil || persisted.CancelRequested || persisted.State != BatchTaskQueued || persisted.Targets[0].State != BatchTargetQueued {
		t.Fatalf("failed persistence mutated live state: %#v, %v", persisted, err)
	}
}

func TestBatchTaskRecoversInProcessAfterPersistenceFailureWithoutReplayingSubmission(t *testing.T) {
	backend := &batchBackendStub{
		submitResult: BatchTargetExecution{
			ExecutionID: "maintenance-persist-failure", State: BatchTargetRunning,
			Stage: "accepted", Progress: 5,
		},
	}
	var service *Service
	service = newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), backend)
	defer service.Close()
	backend.onSubmit = func() {
		service.batchTasks.mu.Lock()
		originalSync := service.batchTasks.ops.syncDir
		service.batchTasks.ops.syncDir = func(string) error { return errors.New("temporary disk sync failure") }
		service.batchTasks.mu.Unlock()
		go func() {
			time.Sleep(40 * time.Millisecond)
			service.batchTasks.mu.Lock()
			service.batchTasks.ops.syncDir = originalSync
			service.batchTasks.mu.Unlock()
		}()
	}
	service.Start(context.Background())
	task, err := service.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionSystemUpdate, HostIDs: []string{LocalHostID}, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	recovered := waitBatchTaskTerminal(t, service, task.ID)
	if recovered.State != BatchTaskNeedsAttention ||
		recovered.Targets[0].ErrorCode != "batch_submission_outcome_unknown" {
		t.Fatalf("ambiguous persisted submission was not recovered safely: %#v", recovered)
	}
	submits, _, _ := backend.snapshot()
	if len(submits) != 1 {
		t.Fatalf("submission was replayed after persistence recovery: %#v", submits)
	}
}

func TestBatchTaskRestartResumesTrackingWithoutResubmitting(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "cluster")
	createdAt := time.Now().UTC()
	service := newBatchTestService(t, directory, &batchBackendStub{})
	task := BatchTask{
		ID: strings.Repeat("1", 32), Action: BatchActionSystemUpdate, State: BatchTaskQueued,
		Concurrency: 1, TimeoutSeconds: 60, CreatedAt: createdAt,
		Targets: []BatchTaskTarget{{
			HostID: LocalHostID, HostName: "batch-test", HostKind: HostKindPanel,
			OperationID: strings.Repeat("2", 32), State: BatchTargetQueued, Stage: "queued",
		}},
	}
	recalculateBatchTask(&task)
	if err := service.batchTasks.Create(task); err != nil {
		t.Fatal(err)
	}
	_, err := service.batchTasks.Update(task.ID, func(task *BatchTask) error {
		task.StartedAt = &createdAt
		task.Targets[0].State = BatchTargetRunning
		task.Targets[0].Stage = "accepted"
		task.Targets[0].Progress = 5
		task.Targets[0].ExecutionID = "existing-maintenance"
		task.Targets[0].StartedAt = &createdAt
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	backend := &batchBackendStub{statusResults: []BatchTargetExecution{{
		ExecutionID: "existing-maintenance", State: BatchTargetSucceeded, Stage: "completed", Progress: 100,
	}}}
	restarted := newBatchTestService(t, directory, backend)
	defer restarted.Close()
	restarted.Start(context.Background())
	recovered := waitBatchTaskTerminal(t, restarted, task.ID)
	if recovered.State != BatchTaskSucceeded {
		t.Fatalf("running task was not resumed: %#v", recovered)
	}
	submits, statuses, executionIDs := backend.snapshot()
	if len(submits) != 0 || len(statuses) != 1 || len(executionIDs) != 1 || executionIDs[0] != "existing-maintenance" {
		t.Fatalf("recovery calls = submits %#v statuses %#v ids %#v", submits, statuses, executionIDs)
	}
}

func TestBatchTaskV2PairingAndExecutionUseAuthenticatedEncryptedRoute(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	clock := &serviceTestClock{now: now}
	targetBackend := &batchBackendStub{
		submitResult:  BatchTargetExecution{ExecutionID: "remote-maintenance", State: BatchTargetRunning, Stage: "accepted", Progress: 5},
		statusResults: []BatchTargetExecution{{ExecutionID: "remote-maintenance", State: BatchTargetSucceeded, Stage: "completed", Progress: 100}},
	}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "test", Hostname: "target",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target"},
		Remote:    targetRemote, Now: clock.Now, BatchActions: targetBackend,
		BatchPollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "test", Hostname: "center",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"},
		Remote:    centerRemote, Now: clock.Now, BatchPollInterval: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer center.Close()
	code, err := target.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	if code.Scope != SummaryTerminalFilesTasksScope {
		t.Fatalf("pairing code advertised scope %q", code.Scope)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Name: "target", Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
	})
	if err != nil {
		t.Fatal(err)
	}
	if host.Scope != SummaryTerminalFilesTasksScope || !host.BatchTaskAvailable {
		t.Fatalf("maintenance scope was not negotiated: %#v", host)
	}
	centerRecord, err := center.storeV2.Host(host.ID)
	if err != nil || centerRecord.Scope != SummaryTerminalFilesScope ||
		!center.batchTasks.HostMaintenanceGranted(host.ID) {
		t.Fatalf("center did not persist a downgrade-compatible grant: %#v, %v", centerRecord, err)
	}
	targetRecord, err := target.storeV2.Controller(centerRecord.ControllerID)
	if err != nil || targetRecord.Scope != SummaryTerminalFilesScope ||
		!target.batchTasks.ControllerMaintenanceGranted(targetRecord.ID) {
		t.Fatalf("target did not persist a downgrade-compatible grant: %#v, %v", targetRecord, err)
	}
	primaryState, err := readRegularFileV2(center.storeV2.path, maxClusterStoreV2Bytes, false)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(primaryState, []byte("cluster.system.maintenance")) {
		t.Fatalf("maintenance grant leaked into downgrade-sensitive v2 state: %s", primaryState)
	}
	center.Start(context.Background())
	task, err := center.CreateBatchTask(context.Background(), CreateBatchTaskInput{
		Action: BatchActionLogsMax500MiB, HostIDs: []string{host.ID}, TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	completed := waitBatchTaskTerminal(t, center, task.ID)
	if completed.State != BatchTaskSucceeded || completed.Targets[0].ExecutionID != "remote-maintenance" {
		t.Fatalf("unexpected remote task: %#v", completed)
	}
	for _, body := range route.requestBodies() {
		if bytes.Contains(body, []byte(string(BatchActionLogsMax500MiB))) ||
			bytes.Contains(body, []byte(completed.Targets[0].OperationID)) {
			t.Fatalf("batch action leaked outside Noise envelope: %s", body)
		}
	}
	submits, statuses, _ := targetBackend.snapshot()
	controllers := target.Controllers()
	if len(controllers) != 1 || controllers[0].Scope != SummaryTerminalFilesTasksScope ||
		len(submits) != 1 || submits[0].ControllerID != controllers[0].ID || len(statuses) != 1 {
		t.Fatalf("unexpected remote backend calls: submits=%#v statuses=%#v", submits, statuses)
	}
}

func TestPairingCodeAdvertisesOnlyLoadedMaintenanceCapability(t *testing.T) {
	service := newBatchTestService(t, filepath.Join(t.TempDir(), "cluster"), nil)
	defer service.Close()
	code, err := service.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	if code.Scope != SummaryTerminalFilesScope {
		t.Fatalf("pairing code advertised unavailable maintenance scope %q", code.Scope)
	}
}

func TestLegacyPairPathCannotGainMaintenanceScopeFromHeader(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	backend := &batchBackendStub{}
	remote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "test", Hostname: "target",
		Telemetry: batchTelemetryStub{}, Remote: remote, Now: func() time.Time { return now }, BatchActions: backend,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	code, err := target.CreatePairingCodeV2()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := parseV2PairingCode(code.Code, now)
	if err != nil {
		t.Fatal(err)
	}
	controllerKey, err := v2NoiseSuite.GenerateKeypair(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	controllerID := strings.Repeat("c", 32)
	transactionID := strings.Repeat("d", 32)
	payload, err := json.Marshal(v2PairPayload{ControllerName: "legacy", TransactionID: transactionID})
	if err != nil {
		t.Fatal(err)
	}
	envelope, handshake, err := sealV2Request(http.MethodPost, v2PairPath, v2Envelope{
		Protocol: FederationProtocolV2, ControllerID: controllerID, TargetID: descriptor.NodeID,
		CodeID: descriptor.CodeID, Timestamp: now.Unix(), RequestID: strings.Repeat("e", 32),
	}, controllerKey, descriptor.TargetPublicKey, descriptor.PairingKey, payload)
	if err != nil {
		t.Fatal(err)
	}
	response, err := target.HandleFederationV2(context.Background(), "198.51.100.20", v2PairPath, "batch-system-maintenance-v1", envelope)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := openV2Response(envelope, response, handshake)
	if err != nil {
		t.Fatal(err)
	}
	var result v2PairResult
	if err := decodeV2Payload(plaintext, &result); err != nil {
		t.Fatal(err)
	}
	if result.Scope != SummaryTerminalFilesScope {
		t.Fatalf("unauthenticated capability header granted scope %q", result.Scope)
	}

	// A capability header alone did not grant anything. A separate Noise-bound
	// maintenance route is an explicit grant request and can complete the same
	// still-provisional pairing transaction without touching the legacy scope
	// stored in cluster-state-v2.json.
	retryEnvelope, retryHandshake, err := sealV2Request(http.MethodPost, v2PairTasksPath, v2Envelope{
		Protocol: FederationProtocolV2, ControllerID: controllerID, TargetID: descriptor.NodeID,
		CodeID: descriptor.CodeID, Timestamp: now.Unix(), RequestID: strings.Repeat("f", 32),
	}, controllerKey, descriptor.TargetPublicKey, descriptor.PairingKey, payload)
	if err != nil {
		t.Fatal(err)
	}
	retryResponse, err := target.HandleFederationV2(context.Background(), "198.51.100.20", v2PairTasksPath, "", retryEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	retryPlaintext, err := openV2Response(retryEnvelope, retryResponse, retryHandshake)
	if err != nil {
		t.Fatal(err)
	}
	var retryResult v2PairResult
	if err := decodeV2Payload(retryPlaintext, &retryResult); err != nil {
		t.Fatal(err)
	}
	if retryResult.Scope != SummaryTerminalFilesTasksScope {
		t.Fatalf("authenticated maintenance pairing scope = %q", retryResult.Scope)
	}
	storedController, err := target.storeV2.Controller(controllerID)
	if err != nil || storedController.Scope != SummaryTerminalFilesScope ||
		!target.batchTasks.ControllerMaintenanceGranted(controllerID) {
		t.Fatalf("maintenance sidecar grant = %#v, %v", storedController, err)
	}
}
