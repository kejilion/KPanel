package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPolicyPrecommitFailurePreservesStateAndCommittedSyncWarningReturnsSavedStatus(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	source := &testReleaseSource{version: "1.2.0"}
	service := newTestService(t, source, &now, time.Hour)
	before := enableTestService(t, service, "1.1.0")
	path := filepath.Join(service.stateDir, stateFileName)
	beforeData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected state sync failure")
	service.writer.SyncFile = func(*os.File) error { return injected }
	failed, err := service.SetEnabled("1.1.0", before.ResourceVersion, false)
	if !errors.Is(err, injected) || failed.Available {
		t.Fatalf("pre-commit result=%+v error=%v", failed, err)
	}
	afterData, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(beforeData, afterData) {
		t.Fatalf("state changed before commit: error=%v", err)
	}
	persisted, err := service.Status("1.1.0")
	if err != nil || !reflect.DeepEqual(before, persisted) {
		t.Fatalf("pre-commit status changed: %+v error=%v", persisted, err)
	}
	state, err := service.load()
	if err != nil {
		t.Fatal(err)
	}
	originalRevision := state.Revision
	if err := service.save(&state); !errors.Is(err, injected) || state.Revision != originalRevision {
		t.Fatalf("failed save advanced revision=%d error=%v", state.Revision, err)
	}
	service.writer.SyncFile = nil
	service.writer.SyncDirectory = func(string) error { return injected }
	var diagnostic bytes.Buffer
	service.logger = slog.New(slog.NewTextHandler(&diagnostic, nil))
	committed, err := service.SetEnabled("1.1.0", before.ResourceVersion, false)
	if err != nil || committed.Enabled || !committed.Available || committed.ResourceVersion == before.ResourceVersion {
		t.Fatalf("committed policy=%+v error=%v", committed, err)
	}
	persisted, err = service.Status("1.1.0")
	if err != nil || !reflect.DeepEqual(committed, persisted) {
		t.Fatalf("committed response differs from state: %+v error=%v", persisted, err)
	}
	if !strings.Contains(diagnostic.String(), "committed=true") || !strings.Contains(diagnostic.String(), injected.Error()) {
		t.Fatalf("missing committed sync diagnosis: %s", diagnostic.String())
	}
	if _, err := service.SetEnabled("1.1.0", before.ResourceVersion, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("repeat stale write error=%v", err)
	}
	restarted := restartTestService(t, service, source, &now)
	reloaded, err := restarted.Status("1.1.0")
	if err != nil || !reflect.DeepEqual(committed, reloaded) {
		t.Fatalf("restart lost committed policy: %+v error=%v", reloaded, err)
	}
}

func TestQueueAndCancelCommittedSyncWarningsPreserveRequestsAndDoNotRepeatWrites(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	source := &testReleaseSource{version: "1.2.0"}
	service := newTestService(t, source, &now, time.Hour)
	observed, err := service.Check(context.Background(), "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected queue directory sync failure")
	service.writer.SyncDirectory = func(string) error { return injected }
	queued, err := service.QueueInstall("1.1.0", observed.ResourceVersion)
	if err != nil || queued.State != "queued" || !queued.InstallRequested {
		t.Fatalf("queue=%+v error=%v", queued, err)
	}
	if _, err := service.QueueInstall("1.1.0", observed.ResourceVersion); !errors.Is(err, ErrConflict) {
		t.Fatalf("repeat queue error=%v", err)
	}
	restarted := restartTestService(t, service, source, &now)
	reloaded, err := restarted.Status("1.1.0")
	if err != nil || !reflect.DeepEqual(queued, reloaded) {
		t.Fatalf("restart lost queue: %+v error=%v", reloaded, err)
	}
	canceled, err := service.CancelQueuedInstall(queued.CandidateVersion, queued.CandidateImageDigest, errors.New("dispatch failed"))
	if err != nil || canceled.InstallRequested || canceled.State != "waiting" || canceled.LastErrorCode != "install_start_failed" {
		t.Fatalf("cancel=%+v error=%v", canceled, err)
	}
	repeated, err := service.CancelQueuedInstall(queued.CandidateVersion, queued.CandidateImageDigest, errors.New("dispatch failed"))
	if err != nil || !reflect.DeepEqual(canceled, repeated) {
		t.Fatalf("repeat cancel changed committed state: %+v error=%v", repeated, err)
	}
}

func TestRunRequiresDurableIntentWithoutDiscardingItsCommittedState(t *testing.T) {
	service, source, now, executor := matureTestCandidate(t)
	injected := errors.New("injected intent directory sync failure")
	service.writer.SyncDirectory = func(path string) error {
		state, err := service.load()
		if err != nil {
			return err
		}
		if state.State == "updating" {
			return injected
		}
		return syncDirectory(path)
	}
	status, err := service.Run(context.Background(), executor)
	if !errors.Is(err, injected) || status.State != "updating" || status.InstallRequested || len(executor.targets) != 0 ||
		!strings.Contains(err.Error(), "intent was committed") {
		t.Fatalf("intent status=%+v error=%v targets=%v", status, err, executor.targets)
	}
	persisted, err := service.Status("1.1.0")
	if err != nil || !reflect.DeepEqual(status, persisted) {
		t.Fatalf("committed intent response differs from state: %+v error=%v", persisted, err)
	}
	// Recovery keeps its existing actual-version reconciliation and quarantine
	// policy. Re-running never replays an update after the uncertain checkpoint.
	restarted := restartTestService(t, service, source, now)
	reconciled, err := restarted.Run(context.Background(), executor)
	if err != nil || reconciled.State != "blocked" || reconciled.FailedVersion != "1.2.0" || len(executor.targets) != 0 {
		t.Fatalf("intent recovery=%+v error=%v targets=%v", reconciled, err, executor.targets)
	}
}

func TestRunPrecommitIntentFailureDoesNotExecuteAndCanRetrySafely(t *testing.T) {
	service, _, _, executor := matureTestCandidate(t)
	before, err := os.ReadFile(filepath.Join(service.stateDir, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected intent file sync failure")
	service.writer.SyncFile = func(file *os.File) error {
		data, err := os.ReadFile(file.Name())
		if err != nil {
			return err
		}
		var state persistedState
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
		if state.State == "updating" {
			return injected
		}
		return file.Sync()
	}
	status, err := service.Run(context.Background(), executor)
	if !errors.Is(err, injected) || status.Available || len(executor.targets) != 0 {
		t.Fatalf("pre-commit intent status=%+v error=%v targets=%v", status, err, executor.targets)
	}
	after, err := os.ReadFile(filepath.Join(service.stateDir, stateFileName))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("pre-commit intent changed stored state: error=%v", err)
	}
	service.writer.SyncFile = nil
	status, err = service.Run(context.Background(), executor)
	if err != nil || status.State != "succeeded" || len(executor.targets) != 1 {
		t.Fatalf("safe retry status=%+v error=%v targets=%v", status, err, executor.targets)
	}
}

func TestRunCommittedTerminalSyncWarningPreservesSuccessAndFailureWithoutReplay(t *testing.T) {
	for _, outcome := range []string{"succeeded", "failed"} {
		t.Run(outcome, func(t *testing.T) {
			service, source, now, executor := matureTestCandidate(t)
			updateFailure := errors.New("injected executor failure")
			if outcome == "failed" {
				executor.update = updateFailure
			}
			injected := errors.New("injected terminal directory sync failure")
			service.writer.SyncDirectory = func(path string) error {
				state, err := service.load()
				if err != nil {
					return err
				}
				if state.State == outcome {
					return injected
				}
				return syncDirectory(path)
			}
			var diagnostic bytes.Buffer
			service.logger = slog.New(slog.NewTextHandler(&diagnostic, nil))
			status, err := service.Run(context.Background(), executor)
			if status.State != outcome || len(executor.targets) != 1 || (outcome == "succeeded" && err != nil) ||
				(outcome == "failed" && !errors.Is(err, updateFailure)) || errors.Is(err, injected) {
				t.Fatalf("terminal status=%+v error=%v targets=%v", status, err, executor.targets)
			}
			if !strings.Contains(diagnostic.String(), "committed=true") || !strings.Contains(diagnostic.String(), injected.Error()) {
				t.Fatalf("missing terminal warning: %s", diagnostic.String())
			}
			restarted := restartTestService(t, service, source, now)
			reloaded, err := restarted.Status(executor.installed)
			if err != nil || !reflect.DeepEqual(status, reloaded) {
				t.Fatalf("restart lost terminal state: %+v error=%v", reloaded, err)
			}
			if _, err := restarted.Run(context.Background(), executor); err != nil || len(executor.targets) != 1 {
				t.Fatalf("terminal retry replayed update: error=%v targets=%v", err, executor.targets)
			}
		})
	}
}

func TestRecoveryFailureDoesNotReportUncommittedFailureState(t *testing.T) {
	service, _, _, executor := matureTestCandidate(t)
	recoveryFailure := errors.New("injected recovery failure")
	writeFailure := errors.New("injected recovery state sync failure")
	executor.recover = recoveryFailure
	before, err := service.Status("1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	service.writer.SyncFile = func(*os.File) error { return writeFailure }
	status, err := service.Run(context.Background(), executor)
	if !errors.Is(err, recoveryFailure) || !errors.Is(err, writeFailure) || status.Available || len(executor.targets) != 0 {
		t.Fatalf("uncommitted recovery result=%+v error=%v targets=%v", status, err, executor.targets)
	}
	after, err := service.Status("1.1.0")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("uncommitted recovery changed state: %+v error=%v", after, err)
	}
}

func matureTestCandidate(t *testing.T) (*Service, *testReleaseSource, *time.Time, *testExecutor) {
	t.Helper()
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	source := &testReleaseSource{version: "1.2.0"}
	service := newTestService(t, source, &now, time.Hour)
	enableTestService(t, service, "1.1.0")
	if _, err := service.Check(context.Background(), "1.1.0"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	return service, source, &now, &testExecutor{installed: "1.1.0"}
}

func restartTestService(t *testing.T, service *Service, source *testReleaseSource, now *time.Time) *Service {
	t.Helper()
	restarted, err := New(Config{
		StateDir: service.stateDir, Source: source, Hold: service.hold,
		Now: func() time.Time { return *now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return restarted
}
