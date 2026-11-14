package remotedownload

import (
	"errors"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

var ErrDecisionConflict = errors.New("file conflict decision is stale")

func (s *JobStore) Decide(id string, decision contract.FileConflictDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return ErrJobStoreUnavailable
	}
	job, ok := s.jobs[id]
	if !ok {
		return ErrJobNotFound
	}
	if job.State != "waiting_input" || job.Conflict == nil || job.Decision != nil || job.Revision != decision.Revision {
		return ErrDecisionConflict
	}
	c := job.Conflict
	directory := c.Source.Kind == "directory"
	valid := decision.Policy == "skip" || decision.Policy == "keep-both" || decision.Policy == "overwrite" && !directory || decision.Policy == "merge" && directory
	compatible := c.Target.Kind == "missing" || c.Source.Kind == c.Target.Kind && (c.Source.Path == "" || c.Source.Path != c.Target.Path)
	if !valid || !compatible && (decision.Policy == "overwrite" || decision.Policy == "merge") {
		return ErrDecisionConflict
	}
	old := cloneJob(job)
	job = cloneJob(job)
	job.Decision = &decision
	job.Revision++
	if decision.ApplyToRest && (compatible || c.Source.Kind == c.Target.Kind && (decision.Policy == "skip" || decision.Policy == "keep-both")) {
		if job.ConflictPolicies == nil {
			job.ConflictPolicies = &contract.FileConflictPolicies{}
		}
		if directory {
			job.ConflictPolicies.Directory = decision.Policy
		} else {
			job.ConflictPolicies.File = decision.Policy
		}
	}
	s.jobs[id] = job
	if err := s.persistLocked(); err != nil {
		s.jobs[id] = old
		return err
	}
	return nil
}

// A selected cross-host batch can span several bounded per-source jobs. Its
// policy is inherited only from the same explicit batch and destination.
func (s *JobStore) BatchPolicies(batchID, targetHost, directory string) *contract.FileConflictPolicies {
	if batchID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest *contract.FileRemoteDownloadJob
	for _, job := range s.jobs {
		if job.BatchID == batchID && job.TargetHostID == targetHost && job.TargetDirectory == directory && job.ConflictPolicies != nil && (latest == nil || job.CreatedAt.After(latest.CreatedAt)) {
			copy := cloneJob(job)
			latest = &copy
		}
	}
	if latest == nil {
		return nil
	}
	return latest.ConflictPolicies
}
