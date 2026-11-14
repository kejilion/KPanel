package filemanager

import (
	"context"
	"errors"
	"os"
	"path"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

var errPublicationQueued = errors.New("directory publication queued")

type receivePublication struct {
	TargetIdentity    string `json:"targetIdentity"`
	Workspace         string `json:"workspace"`
	WorkspaceIdentity string `json:"workspaceIdentity"`
	StageName         string `json:"stageName"`
	StageIdentity     string `json:"stageIdentity"`
}

// Called only after the entire TAR, checksum and private stage have been
// validated. No request can nominate an arbitrary private source.
func (m *Manager) queueReceivePublicationLocked(record *receiveRecord, workspace *fileRoot, info os.FileInfo) error {
	a := &m.archiveJobs
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := m.initializeArchiveJobsLocked(); err != nil {
		return err
	}
	if len(a.cancels) >= maxActiveArchiveJobs || len(a.records) >= maxArchiveJobs {
		return ErrBusy
	}
	workspaceInfo, err := workspace.Stat(".")
	if err != nil {
		return err
	}
	id := record.Session.ID[:32]
	if _, exists := a.records[id]; exists {
		return ErrConflict
	}
	private := &receivePublication{Workspace: record.ExtractDirectory, WorkspaceIdentity: stableRootReceiveIdentity(workspace, ".", workspaceInfo), StageName: path.Base(receiveExtract(record)), StageIdentity: record.ExtractIdentity}
	private.TargetIdentity = record.ParentIdentity
	if private.StageIdentity == "" || !archiveWorkspaceProtected(workspaceInfo, workspace) {
		return ErrConflict
	}
	job := contract.FileArchiveJob{ID: id, Action: "receive", Name: record.Input.Name, Target: record.Input.Directory, Sources: []string{joinVirtual(record.Input.Directory, record.Input.Name)}, ConflictPolicies: record.Input.ConflictPolicies, State: "queued", CreatedAt: m.now().UTC(), UpdatedAt: m.now().UTC(), Result: contract.FileActionResult{Action: "receive", Succeeded: []contract.FileActionItem{}, Failed: []contract.FileActionFailure{}}}
	record.Session.PublishJobID = id
	// Persist the link first. A crash before the job is admitted is an
	// interruption, never permission to replay a partially merged directory.
	if err := m.persistReceivesLocked(); err != nil {
		record.Session.PublishJobID = ""
		return err
	}
	a.records[id] = &archiveJobRecord{Job: job, Private: private}
	if err := m.persistArchiveJobsLocked(); err != nil {
		delete(a.records, id)
		record.Session.PublishJobID = ""
		_ = m.persistReceivesLocked()
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	a.cancels[id] = cancel
	a.wg.Add(1)
	go m.runReceivePublication(ctx, cancel, id)
	return errPublicationQueued
}

func (m *Manager) openReceivePublication(private *receivePublication) (*fileRoot, *fileRoot, error) {
	workspace, info, err := m.openReceiveDirectory(private.Workspace, false)
	if err != nil {
		return nil, nil, err
	}
	if !archiveWorkspaceProtected(info, workspace) || stableRootReceiveIdentity(workspace, ".", info) != private.WorkspaceIdentity {
		workspace.Close()
		return nil, nil, ErrConflict
	}
	visible, err := workspace.Lstat(private.StageName)
	if err != nil || !visible.IsDir() || stableRootReceiveIdentity(workspace, private.StageName, visible) != private.StageIdentity {
		workspace.Close()
		return nil, nil, ErrConflict
	}
	stage, err := workspace.OpenRoot(private.StageName)
	if err != nil {
		workspace.Close()
		return nil, nil, err
	}
	opened, err := stage.Stat(".")
	if err != nil || !os.SameFile(visible, opened) || !archiveStagePrivate(opened, stage) {
		stage.Close()
		workspace.Close()
		return nil, nil, ErrConflict
	}
	return workspace, stage, nil
}

func (m *Manager) cleanupReceivePublication(private *receivePublication) error {
	workspace, stage, err := m.openReceivePublication(private)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer workspace.Close()
	stage.Close()
	info, err := workspace.Lstat(private.StageName)
	if err != nil {
		return err
	}
	return removeOwnedReceiveDirectory(workspace, private.StageName, info, nil, private.StageIdentity)
}

func (m *Manager) runReceivePublication(ctx context.Context, cancel context.CancelFunc, id string) {
	a := &m.archiveJobs
	defer a.wg.Done()
	defer cancel()
	// Registered before resource cleanup: reconcile only after stage handles,
	// the write lock and worker slot have all been released.
	defer m.finalizeReceivePublication(id)
	gateHeld := false
	writeHeld := false
	finishAfterCleanup := func() {}
	defer func() {
		if writeHeld {
			m.writeMu.Unlock()
		}
		if gateHeld {
			<-a.gate
		}
		finishAfterCleanup()
		a.mu.Lock()
		delete(a.cancels, id)
		a.mu.Unlock()
	}()
	select {
	case a.gate <- struct{}{}:
		gateHeld = true
	case <-ctx.Done():
	}
	a.mu.Lock()
	record := a.records[id]
	record.Job.State = "running"
	record.Job.UpdatedAt = m.now().UTC()
	persistErr := m.persistArchiveJobsLocked()
	job := cloneArchiveJob(record.Job)
	private := *record.Private
	a.mu.Unlock()
	result := job.Result
	finish := func(state string, entry *contract.FileEntry, err error) {
		finishAfterCleanup = func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			// Publish terminal state and resource-release ownership together.
			// Readers must not observe cancelled/complete while cleanup is pending.
			delete(a.cancels, id)
			record := a.records[id]
			record.Job.State = state
			record.Job.Entry = entry
			record.Job.Conflict = nil
			record.Decision = nil
			record.Job.Result = result
			record.Job.UpdatedAt = m.now().UTC()
			if err != nil {
				record.Job.Detail = archiveFailureDetail(err)
				record.Job.Result.Failed = append(record.Job.Result.Failed, contract.FileActionFailure{Path: job.Name, Detail: record.Job.Detail, Code: conflictErrorCode(err)})
			}
			if err := m.persistArchiveJobsLocked(); err != nil {
				record.Job.State = "interrupted"
				record.Job.Detail = "任务状态保存失败，请核对目标目录"
			}
		}
	}
	if persistErr != nil {
		finish("interrupted", nil, persistErr)
		return
	}
	if ctx.Err() != nil {
		finish("cancelled", nil, ctx.Err())
		return
	}
	workspace, stage, err := m.openReceivePublication(&private)
	if err != nil {
		finish("error", nil, err)
		return
	}
	defer func() { _ = m.cleanupReceivePublication(&private) }()
	defer workspace.Close()
	defer stage.Close()
	if err := m.writeMu.LockContext(ctx); err != nil {
		finish("cancelled", nil, err)
		return
	}
	writeHeld = true
	session := &ConflictSession{Policies: *job.ConflictPolicies, Result: &result}
	session.Prompt = func(ctx context.Context, conflict contract.FileConflict) (contract.FileConflictDecision, error) {
		m.writeMu.Unlock()
		writeHeld = false
		if gateHeld {
			<-a.gate
			gateHeld = false
		}
		decision, err := m.awaitArchiveConflict(ctx, id, conflict)
		if err == nil {
			select {
			case a.gate <- struct{}{}:
				gateHeld = true
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		m.writeMu.Lock()
		writeHeld = true
		return decision, err
	}
	validate := func() error {
		parent, info, err := m.openReceiveDirectory(job.Target, false)
		if err != nil {
			return err
		}
		identity := stableRootReceiveIdentity(parent, ".", info)
		parent.Close()
		if identity != private.TargetIdentity {
			return ErrConflict
		}
		w, s, err := m.openReceivePublication(&private)
		if err == nil {
			s.Close()
			w.Close()
		}
		return err
	}
	entry, err := m.publishConflictTree(WithFileConflicts(ctx, session), stage, "result", job.Target, job.Name, joinVirtual(joinVirtual(private.Workspace, private.StageName), "result"), validate, 0)
	if errors.Is(err, ErrFileSkipped) {
		if current, statErr := m.Stat(joinVirtual(job.Target, job.Name)); statErr == nil {
			entry = current
		}
		if len(result.Succeeded)+result.CompletedCount > 0 {
			finish("partial", &entry, nil)
		} else {
			finish("skipped", nil, nil)
		}
		return
	}
	if err != nil {
		state := "error"
		if ctx.Err() != nil {
			state = "cancelled"
		}
		finish(state, nil, err)
		return
	}
	result.Succeeded = append(result.Succeeded, actionItem(job.Name, entry, entry.Path))
	finish("complete", &entry, nil)
}

// The receive journal lock is held. Workers acquire it only after releasing
// their archive lock and publication resources. Lock order is receive -> archive.
func (m *Manager) receivePublicationStatusLocked(record *receiveRecord) (contract.FileReceiveSession, error) {
	a := &m.archiveJobs
	a.mu.Lock()
	if err := m.initializeArchiveJobsLocked(); err != nil {
		a.mu.Unlock()
		return record.Session, err
	}
	jobRecord := a.records[record.Session.PublishJobID]
	if jobRecord == nil {
		a.mu.Unlock()
		return record.Session, ErrConflict
	}
	job := cloneArchiveJob(jobRecord.Job)
	_, workerActive := a.cancels[job.ID]
	if job.Action != "receive" || job.Target != record.Input.Directory || jobRecord.Private == nil || jobRecord.Private.StageName != ".kpanel-extract-"+record.Session.ID {
		a.mu.Unlock()
		return record.Session, ErrConflict
	}
	a.mu.Unlock()
	session := record.Session
	session.Conflict = job.Conflict
	session.Revision = job.Revision
	if archiveJobActive(job.State) || workerActive {
		session.State = "committing"
		if job.State == "waiting_input" {
			session.State = "waiting_input"
		}
		return session, nil
	}
	if job.State != "complete" && job.State != "partial" && job.State != "skipped" {
		// Terminal workers can no longer publish. Free the reservation even if
		// a substituted parent prevents identity-safe removal of its old spool.
		_ = m.removeReceiveTemp(record)
		delete(m.receives.records, record.Session.ID)
		if err := m.persistReceivesLocked(); err != nil {
			return session, err
		}
		return session, os.ErrNotExist
	}
	session.State = "complete"
	session.Entry = job.Entry
	session.Result = &job.Result
	session.Conflict = nil
	record.Session = session
	if session.Entry != nil {
		record.Input.Name = session.Entry.Name
	}
	if err := m.persistReceivesLocked(); err != nil {
		return session, err
	}
	_ = m.removeReceiveTemp(record)
	return session, nil
}

func (m *Manager) finalizeReceivePublication(jobID string) {
	m.receives.mu.Lock()
	defer m.receives.mu.Unlock()
	for _, record := range m.receives.records {
		if record.Session.PublishJobID == jobID {
			_, _ = m.receivePublicationStatusLocked(record)
			return
		}
	}
}

func (m *Manager) DecideReceive(ctx context.Context, id, key string, decision contract.FileConflictDecision) (contract.FileReceiveSession, error) {
	if err := m.receives.mu.LockContext(ctx); err != nil {
		return contract.FileReceiveSession{}, err
	}
	defer m.receives.mu.Unlock()
	record, err := m.receiveLocked(id, key)
	if err != nil {
		return contract.FileReceiveSession{}, err
	}
	if record.Session.PublishJobID == "" {
		return contract.FileReceiveSession{}, ErrAction
	}
	if err := m.DecideArchiveJob(record.Session.PublishJobID, decision); err != nil {
		return contract.FileReceiveSession{}, err
	}
	return m.receivePublicationStatusLocked(record)
}
