package cluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
)

const (
	batchTaskStateFileName = "cluster-batch-task-state.json"
	maxBatchTaskStateBytes = int64(4 << 20)
	maxBatchTasks          = 100
	maxActiveBatchTasks    = 4
	maxBatchTargets        = 50
	maxBatchConcurrency    = 4
	minBatchTimeoutSeconds = 60
	maxBatchTimeoutSeconds = 3600
	defaultBatchTimeout    = 3600
)

var batchExecutionIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,96}$`)

type batchTaskPersistedState struct {
	SchemaVersion            int         `json:"schemaVersion"`
	Tasks                    []BatchTask `json:"tasks"`
	MaintenanceHostIDs       []string    `json:"maintenanceHostIds"`
	MaintenanceControllerIDs []string    `json:"maintenanceControllerIds"`
}

type batchTaskStore struct {
	mu    sync.RWMutex
	path  string
	state batchTaskPersistedState
	ops   atomicFileOpsV2
}

func openBatchTaskStore(path string) (*batchTaskStore, error) {
	if !filepath.IsAbs(path) || filepath.Base(filepath.Clean(path)) != batchTaskStateFileName {
		return nil, errors.New("cluster batch task store path is invalid")
	}
	if err := protectDirectoryV2(filepath.Dir(path)); err != nil {
		return nil, err
	}
	ops := defaultAtomicFileOpsV2()
	if err := recoverAtomicTargetV2(path, ops); err != nil {
		return nil, fmt.Errorf("recover cluster batch task store: %w", err)
	}
	store := &batchTaskStore{
		path: path,
		state: batchTaskPersistedState{
			SchemaVersion: 1, Tasks: []BatchTask{},
			MaintenanceHostIDs: []string{}, MaintenanceControllerIDs: []string{},
		},
		ops: ops,
	}
	content, err := readRegularFileV2(path, maxBatchTaskStateBytes, true)
	switch {
	case err == nil:
		loadErr := decodeBatchTaskState(content, &store.state)
		if loadErr != nil {
			if restoreErr := restoreAtomicBackupV2(path, ops); restoreErr != nil {
				return nil, fmt.Errorf("decode cluster batch task store: %v (backup recovery: %w)", loadErr, restoreErr)
			}
			content, err = readRegularFileV2(path, maxBatchTaskStateBytes, true)
			if err != nil {
				return nil, fmt.Errorf("read recovered cluster batch task store: %w", err)
			}
			store.state = batchTaskPersistedState{}
			if err := decodeBatchTaskState(content, &store.state); err != nil {
				return nil, fmt.Errorf("decode recovered cluster batch task store: %w", err)
			}
		} else if err := discardAtomicBackupV2(path, ops); err != nil {
			return nil, fmt.Errorf("finalize cluster batch task store recovery: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persistLocked(); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("read cluster batch task store: %w", err)
	}
	return store, nil
}

func decodeBatchTaskState(content []byte, state *batchTaskPersistedState) error {
	*state = batchTaskPersistedState{}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(state); err != nil {
		return fmt.Errorf("decode cluster batch task store: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("cluster batch task store contains multiple JSON values")
	}
	if state.SchemaVersion != 1 || state.Tasks == nil || len(state.Tasks) > maxBatchTasks ||
		len(state.MaintenanceHostIDs) > MaxHosts || len(state.MaintenanceControllerIDs) > maxControllersV2 {
		return errors.New("cluster batch task store is invalid")
	}
	if state.MaintenanceHostIDs == nil {
		state.MaintenanceHostIDs = []string{}
	}
	if state.MaintenanceControllerIDs == nil {
		state.MaintenanceControllerIDs = []string{}
	}
	for _, grants := range [][]string{state.MaintenanceHostIDs, state.MaintenanceControllerIDs} {
		seen := make(map[string]struct{}, len(grants))
		for _, id := range grants {
			if !validID(id) {
				return errors.New("cluster batch task store contains an invalid maintenance grant")
			}
			if _, exists := seen[id]; exists {
				return errors.New("cluster batch task store contains a duplicate maintenance grant")
			}
			seen[id] = struct{}{}
		}
	}
	ids := make(map[string]struct{}, len(state.Tasks))
	activeHosts := make(map[string]struct{})
	active := 0
	for index := range state.Tasks {
		task := &state.Tasks[index]
		if err := validateStoredBatchTask(*task); err != nil {
			return err
		}
		if _, exists := ids[task.ID]; exists {
			return errors.New("cluster batch task store contains a duplicate task")
		}
		ids[task.ID] = struct{}{}
		if !batchTaskTerminal(task.State) {
			active++
			for _, target := range task.Targets {
				if _, exists := activeHosts[target.HostID]; exists {
					return errors.New("cluster batch task store contains overlapping active hosts")
				}
				activeHosts[target.HostID] = struct{}{}
			}
		}
	}
	if active > maxActiveBatchTasks {
		return errors.New("cluster batch task store contains too many active tasks")
	}
	return nil
}

func validateStoredBatchTask(task BatchTask) error {
	if !validID(task.ID) || !validBatchAction(task.Action) || !validBatchTaskState(task.State) ||
		task.Concurrency < 1 || task.Concurrency > maxBatchConcurrency ||
		task.TimeoutSeconds < minBatchTimeoutSeconds || task.TimeoutSeconds > maxBatchTimeoutSeconds ||
		len(task.Targets) < 1 || len(task.Targets) > maxBatchTargets || task.CreatedAt.IsZero() ||
		(task.ParentTaskID != "" && !validID(task.ParentTaskID)) {
		return errors.New("cluster batch task store contains an invalid task")
	}
	seen := make(map[string]struct{}, len(task.Targets))
	for _, target := range task.Targets {
		if (target.HostID != LocalHostID && !validID(target.HostID)) ||
			cleanDisplayText(target.HostName, 80) != target.HostName || target.HostName == "" ||
			(target.HostKind != HostKindPanel && target.HostKind != HostKindLightNode) ||
			!validID(target.OperationID) || !validBatchTargetState(target.State) ||
			target.Progress < 0 || target.Progress > 100 ||
			(target.ExecutionID != "" && !batchExecutionIDPattern.MatchString(target.ExecutionID)) ||
			cleanDisplayText(target.Stage, 80) != target.Stage ||
			cleanDisplayText(target.Message, 300) != target.Message ||
			cleanDisplayText(target.ErrorCode, 80) != target.ErrorCode {
			return errors.New("cluster batch task store contains an invalid target")
		}
		if (target.StartedAt != nil && target.StartedAt.Before(task.CreatedAt)) ||
			(target.FinishedAt != nil && target.FinishedAt.Before(task.CreatedAt)) ||
			(target.StartedAt != nil && target.FinishedAt != nil && target.FinishedAt.Before(*target.StartedAt)) {
			return errors.New("cluster batch task store contains invalid target timestamps")
		}
		switch target.State {
		case BatchTargetQueued:
			if target.StartedAt != nil || target.FinishedAt != nil || target.ExecutionID != "" || target.Progress != 0 {
				return errors.New("cluster batch task store contains an invalid queued target")
			}
		case BatchTargetSubmitting:
			if target.StartedAt == nil || target.FinishedAt != nil || target.ExecutionID != "" || target.Progress >= 100 {
				return errors.New("cluster batch task store contains an invalid submitting target")
			}
		case BatchTargetRunning:
			if target.StartedAt == nil || target.FinishedAt != nil || target.ExecutionID == "" || target.Progress < 1 || target.Progress >= 100 {
				return errors.New("cluster batch task store contains an invalid running target")
			}
		default:
			if target.FinishedAt == nil || target.Progress != 100 {
				return errors.New("cluster batch task store contains an invalid terminal target")
			}
		}
		if _, exists := seen[target.HostID]; exists {
			return errors.New("cluster batch task store contains a duplicate target")
		}
		seen[target.HostID] = struct{}{}
	}
	if (task.StartedAt != nil && task.StartedAt.Before(task.CreatedAt)) ||
		(task.FinishedAt != nil && task.FinishedAt.Before(task.CreatedAt)) ||
		(task.StartedAt != nil && task.FinishedAt != nil && task.FinishedAt.Before(*task.StartedAt)) {
		return errors.New("cluster batch task store contains invalid task timestamps")
	}
	expected := cloneBatchTask(task)
	recalculateBatchTask(&expected)
	if expected.State != task.State || expected.Total != task.Total || expected.Completed != task.Completed ||
		expected.Succeeded != task.Succeeded || expected.Failed != task.Failed ||
		expected.NeedsAttention != task.NeedsAttention || expected.Cancelled != task.Cancelled ||
		expected.Unsupported != task.Unsupported || (expected.FinishedAt == nil) != (task.FinishedAt == nil) {
		return errors.New("cluster batch task store contains inconsistent derived state")
	}
	return nil
}

func (s *batchTaskStore) Create(task BatchTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validateStoredBatchTask(task); err != nil || task.State != BatchTaskQueued {
		return ErrBatchTaskInvalid
	}
	if s.activeCountLocked() >= maxActiveBatchTasks {
		return ErrBatchTaskBusy
	}
	previous := cloneBatchTaskState(s.state)
	for _, existing := range s.state.Tasks {
		if existing.ID == task.ID {
			return ErrBatchTaskInvalid
		}
		if batchTaskTerminal(existing.State) {
			continue
		}
		for _, existingTarget := range existing.Targets {
			for _, target := range task.Targets {
				if target.HostID == existingTarget.HostID {
					return ErrBatchTaskBusy
				}
			}
		}
	}
	s.pruneTerminalLocked(maxBatchTasks - 1)
	if len(s.state.Tasks) >= maxBatchTasks {
		s.state = previous
		return ErrBatchTaskBusy
	}
	s.state.Tasks = append(s.state.Tasks, cloneBatchTask(task))
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *batchTaskStore) Get(id string) (BatchTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, task := range s.state.Tasks {
		if task.ID == id {
			return cloneBatchTask(task), nil
		}
	}
	return BatchTask{}, ErrNotFound
}

func (s *batchTaskStore) List() []BatchTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items := make([]BatchTask, 0, len(s.state.Tasks))
	for _, task := range s.state.Tasks {
		items = append(items, cloneBatchTask(task))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items
}

func (s *batchTaskStore) Update(id string, mutate func(*BatchTask) error) (BatchTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.state.Tasks {
		if s.state.Tasks[index].ID != id {
			continue
		}
		previous := cloneBatchTaskState(s.state)
		if err := mutate(&s.state.Tasks[index]); err != nil {
			return BatchTask{}, err
		}
		recalculateBatchTask(&s.state.Tasks[index])
		if err := validateStoredBatchTask(s.state.Tasks[index]); err != nil {
			s.state = previous
			return BatchTask{}, ErrBatchTaskInvalid
		}
		if err := s.persistLocked(); err != nil {
			s.state = previous
			return BatchTask{}, err
		}
		return cloneBatchTask(s.state.Tasks[index]), nil
	}
	return BatchTask{}, ErrNotFound
}

func (s *batchTaskStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, task := range s.state.Tasks {
		if task.ID != id {
			continue
		}
		if !batchTaskTerminal(task.State) {
			return ErrBatchTaskState
		}
		previous := cloneBatchTaskState(s.state)
		s.state.Tasks = append(s.state.Tasks[:index], s.state.Tasks[index+1:]...)
		if err := s.persistLocked(); err != nil {
			s.state = previous
			return err
		}
		return nil
	}
	return ErrNotFound
}

func (s *batchTaskStore) HostMaintenanceGranted(id string) bool {
	return s.maintenanceGranted(id, true)
}

func (s *batchTaskStore) ControllerMaintenanceGranted(id string) bool {
	return s.maintenanceGranted(id, false)
}

func (s *batchTaskStore) maintenanceGranted(id string, host bool) bool {
	if !validID(id) {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	grants := s.state.MaintenanceControllerIDs
	if host {
		grants = s.state.MaintenanceHostIDs
	}
	return stringSliceContains(grants, id)
}

func (s *batchTaskStore) GrantHostMaintenance(id string) error {
	return s.setMaintenanceGrant(id, true, true)
}

func (s *batchTaskStore) RevokeHostMaintenance(id string) error {
	return s.setMaintenanceGrant(id, true, false)
}

func (s *batchTaskStore) GrantControllerMaintenance(id string) error {
	return s.setMaintenanceGrant(id, false, true)
}

func (s *batchTaskStore) RevokeControllerMaintenance(id string) error {
	return s.setMaintenanceGrant(id, false, false)
}

func (s *batchTaskStore) setMaintenanceGrant(id string, host, granted bool) error {
	if !validID(id) {
		return ErrBatchTaskInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	grants := &s.state.MaintenanceControllerIDs
	maximum := maxControllersV2
	if host {
		grants = &s.state.MaintenanceHostIDs
		maximum = MaxHosts
	}
	index := -1
	for current, value := range *grants {
		if value == id {
			index = current
			break
		}
	}
	if granted && index >= 0 || !granted && index < 0 {
		return nil
	}
	previous := cloneBatchTaskState(s.state)
	if granted {
		if len(*grants) >= maximum {
			return ErrBatchTaskBusy
		}
		*grants = append(*grants, id)
		sort.Strings(*grants)
	} else {
		*grants = append((*grants)[:index], (*grants)[index+1:]...)
	}
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func (s *batchTaskStore) ReconcileMaintenanceGrants(hosts []hostRecordV2, controllers []controllerRecordV2) error {
	validHosts := make(map[string]struct{}, len(hosts))
	for _, record := range hosts {
		validHosts[record.ID] = struct{}{}
	}
	validControllers := make(map[string]struct{}, len(controllers))
	for _, record := range controllers {
		validControllers[record.ID] = struct{}{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hostGrants := filterMaintenanceGrants(s.state.MaintenanceHostIDs, validHosts)
	controllerGrants := filterMaintenanceGrants(s.state.MaintenanceControllerIDs, validControllers)
	if slicesEqual(hostGrants, s.state.MaintenanceHostIDs) &&
		slicesEqual(controllerGrants, s.state.MaintenanceControllerIDs) {
		return nil
	}
	previous := cloneBatchTaskState(s.state)
	s.state.MaintenanceHostIDs = hostGrants
	s.state.MaintenanceControllerIDs = controllerGrants
	if err := s.persistLocked(); err != nil {
		s.state = previous
		return err
	}
	return nil
}

func filterMaintenanceGrants(grants []string, valid map[string]struct{}) []string {
	result := make([]string, 0, len(grants))
	for _, id := range grants {
		if _, exists := valid[id]; exists {
			result = append(result, id)
		}
	}
	return result
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *batchTaskStore) activeCountLocked() int {
	active := 0
	for _, task := range s.state.Tasks {
		if !batchTaskTerminal(task.State) {
			active++
		}
	}
	return active
}

func (s *batchTaskStore) pruneTerminalLocked(maximum int) {
	for len(s.state.Tasks) > maximum {
		oldest := -1
		for index, task := range s.state.Tasks {
			if !batchTaskTerminal(task.State) {
				continue
			}
			if oldest < 0 || task.CreatedAt.Before(s.state.Tasks[oldest].CreatedAt) {
				oldest = index
			}
		}
		if oldest < 0 {
			return
		}
		s.state.Tasks = append(s.state.Tasks[:oldest], s.state.Tasks[oldest+1:]...)
	}
}

func (s *batchTaskStore) persistLocked() error {
	content, err := json.Marshal(s.state)
	if err != nil {
		return err
	}
	if int64(len(content)+1) > maxBatchTaskStateBytes {
		return errors.New("cluster batch task store exceeds its size limit")
	}
	return atomicWriteFileV2(s.path, append(content, '\n'), 0o600, true, s.ops)
}

func cloneBatchTaskState(state batchTaskPersistedState) batchTaskPersistedState {
	result := batchTaskPersistedState{
		SchemaVersion: state.SchemaVersion, Tasks: make([]BatchTask, len(state.Tasks)),
		MaintenanceHostIDs:       append([]string{}, state.MaintenanceHostIDs...),
		MaintenanceControllerIDs: append([]string{}, state.MaintenanceControllerIDs...),
	}
	for index, task := range state.Tasks {
		result.Tasks[index] = cloneBatchTask(task)
	}
	return result
}

func cloneBatchTask(task BatchTask) BatchTask {
	task.Targets = append([]BatchTaskTarget(nil), task.Targets...)
	for index := range task.Targets {
		task.Targets[index].StartedAt = cloneTime(task.Targets[index].StartedAt)
		task.Targets[index].FinishedAt = cloneTime(task.Targets[index].FinishedAt)
	}
	task.StartedAt = cloneTime(task.StartedAt)
	task.FinishedAt = cloneTime(task.FinishedAt)
	return task
}

func validBatchAction(action BatchAction) bool {
	switch action {
	case BatchActionRefresh, BatchActionSystemUpdate, BatchActionCleanupCache,
		BatchActionCleanupStandard, BatchActionLogsRetain7Days,
		BatchActionLogsRetain3Days, BatchActionLogsMax500MiB, BatchActionReboot:
		return true
	default:
		return false
	}
}

func validBatchTaskState(state BatchTaskState) bool {
	switch state {
	case BatchTaskQueued, BatchTaskRunning, BatchTaskCancelling, BatchTaskSucceeded,
		BatchTaskPartial, BatchTaskFailed, BatchTaskCancelled, BatchTaskNeedsAttention:
		return true
	default:
		return false
	}
}

func validBatchTargetState(state BatchTargetState) bool {
	switch state {
	case BatchTargetQueued, BatchTargetSubmitting, BatchTargetRunning, BatchTargetSucceeded,
		BatchTargetFailed, BatchTargetCancelled, BatchTargetUnsupported, BatchTargetNeedsAttention:
		return true
	default:
		return false
	}
}

func batchTaskTerminal(state BatchTaskState) bool {
	switch state {
	case BatchTaskSucceeded, BatchTaskPartial, BatchTaskFailed, BatchTaskCancelled, BatchTaskNeedsAttention:
		return true
	default:
		return false
	}
}

func batchTargetTerminal(state BatchTargetState) bool {
	switch state {
	case BatchTargetSucceeded, BatchTargetFailed, BatchTargetCancelled,
		BatchTargetUnsupported, BatchTargetNeedsAttention:
		return true
	default:
		return false
	}
}

func recalculateBatchTask(task *BatchTask) {
	task.Total = len(task.Targets)
	task.Completed, task.Succeeded, task.Failed = 0, 0, 0
	task.NeedsAttention, task.Cancelled, task.Unsupported = 0, 0, 0
	active := 0
	for _, target := range task.Targets {
		if batchTargetTerminal(target.State) {
			task.Completed++
		} else {
			active++
		}
		switch target.State {
		case BatchTargetSucceeded:
			task.Succeeded++
		case BatchTargetFailed:
			task.Failed++
		case BatchTargetNeedsAttention:
			task.NeedsAttention++
		case BatchTargetCancelled:
			task.Cancelled++
		case BatchTargetUnsupported:
			task.Unsupported++
		}
	}
	if active > 0 {
		if task.CancelRequested {
			task.State = BatchTaskCancelling
		} else if task.StartedAt == nil {
			task.State = BatchTaskQueued
		} else {
			task.State = BatchTaskRunning
		}
		task.FinishedAt = nil
		return
	}
	if task.FinishedAt == nil {
		finishedAt := task.CreatedAt
		for _, target := range task.Targets {
			if target.FinishedAt != nil && target.FinishedAt.After(finishedAt) {
				finishedAt = *target.FinishedAt
			}
		}
		task.FinishedAt = &finishedAt
	}
	switch {
	case task.NeedsAttention > 0:
		task.State = BatchTaskNeedsAttention
	case task.Succeeded == task.Total:
		task.State = BatchTaskSucceeded
	case task.Cancelled == task.Total:
		task.State = BatchTaskCancelled
	case task.Succeeded > 0:
		task.State = BatchTaskPartial
	default:
		task.State = BatchTaskFailed
	}
}
