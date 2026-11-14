package filemanager

import (
	"context"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func fileJobAction(action string) bool {
	return action == "compress" || action == "extract" || action == "copy" || action == "move" || action == "rename" || action == "trash_restore"
}

func storedFileJobAction(action string) bool { return fileJobAction(action) || action == "receive" }

func (m *Manager) awaitArchiveConflict(ctx context.Context, id string, conflict contract.FileConflict) (contract.FileConflictDecision, error) {
	a := &m.archiveJobs
	a.mu.Lock()
	record := a.records[id]
	if record == nil {
		a.mu.Unlock()
		return contract.FileConflictDecision{}, ErrConflict
	}
	record.Job.State = "waiting_input"
	record.Job.Conflict = &conflict
	record.Job.Revision++
	record.Decision = nil
	record.Job.UpdatedAt = m.now().UTC()
	err := m.persistArchiveJobsLocked()
	a.mu.Unlock()
	if err != nil {
		return contract.FileConflictDecision{}, err
	}
	timer := time.NewTimer(24 * time.Hour)
	defer timer.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return contract.FileConflictDecision{}, ctx.Err()
		case <-timer.C:
			return contract.FileConflictDecision{}, context.DeadlineExceeded
		case <-ticker.C:
			a.mu.Lock()
			record := a.records[id]
			if record != nil && record.Decision != nil {
				decision := *record.Decision
				record.Decision = nil
				record.Job.Conflict = nil
				record.Job.State = "running"
				record.Job.UpdatedAt = m.now().UTC()
				err := m.persistArchiveJobsLocked()
				a.mu.Unlock()
				return decision, err
			}
			a.mu.Unlock()
		}
	}
}

// Revision is compared while holding the same lock that persists the decision;
// two tabs can never both decide the same conflict.
func (m *Manager) DecideArchiveJob(id string, decision contract.FileConflictDecision) error {
	a := &m.archiveJobs
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := m.initializeArchiveJobsLocked(); err != nil {
		return err
	}
	record := a.records[id]
	if record == nil || record.Job.State != "waiting_input" || record.Job.Conflict == nil || record.Decision != nil || record.Job.Revision != decision.Revision {
		return ErrConflict
	}
	conflict := record.Job.Conflict
	directory := conflict.Source.Kind == "directory"
	if !validConflictPolicy(decision.Policy, directory) || decision.Policy == "" || decision.Policy == "ask" {
		return ErrAction
	}
	compatible := conflict.Target.Kind == "missing" || conflict.Source.Kind == conflict.Target.Kind && (conflict.Source.Path == "" || conflict.Source.Path != conflict.Target.Path)
	if !compatible && (decision.Policy == "overwrite" || decision.Policy == "merge") {
		return ErrAction
	}
	previous := cloneArchiveJob(record.Job)
	record.Decision = &decision
	record.Job.Revision++
	if decision.ApplyToRest && (compatible || conflict.Source.Kind == conflict.Target.Kind && (decision.Policy == "skip" || decision.Policy == "keep-both")) {
		if record.Job.ConflictPolicies == nil {
			record.Job.ConflictPolicies = &contract.FileConflictPolicies{}
		}
		if directory {
			record.Job.ConflictPolicies.Directory = decision.Policy
		} else {
			record.Job.ConflictPolicies.File = decision.Policy
		}
	}
	if err := m.persistArchiveJobsLocked(); err != nil {
		record.Job = previous
		record.Decision = nil
		return err
	}
	return nil
}
