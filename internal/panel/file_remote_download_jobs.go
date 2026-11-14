package panel

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
	"github.com/kejilion/kejilion-panel/internal/store"
)

const (
	remoteDownloadProgressPersistInterval = 5 * time.Second
	remoteDownloadProgressPersistBytes    = 8 << 20
)

var errRemoteDownloadServerClosing = errors.New("remote download server is closing")

type fileRemoteDownloadTask struct {
	accessLifetime      context.Context
	hostAccessLifetimes []context.Context
	job                 contract.FileRemoteDownloadJob
	input               contract.FileRemoteDownloadRequest
	actorID             string
	sourceIP            string
	requestID           string
	execute             func(context.Context, func(contract.FileTransferEvent) bool) contract.FileTransferEvent
}

func (s *Server) handleFileRemoteDownloadJobs(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "离线下载参数无效", "")
		return
	}
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	jobs, err := s.remoteDownloadJobs.List()
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "remote_download_jobs_unavailable", "离线下载任务暂不可用", "")
		return
	}
	s.writeJSON(w, http.StatusOK, contract.FileRemoteDownloadJobList{Items: jobs})
}

func (s *Server) handleFileRemoteDownloadJob(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "离线下载参数无效", "")
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/files/remote-downloads/")
	parts := strings.Split(suffix, "/")
	if len(parts) == 0 || len(parts) > 2 || parts[0] == "" || len(parts) == 2 && parts[1] != "cancel" && parts[1] != "decide" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodGet && len(parts) == 1 {
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		job, err := s.remoteDownloadJobs.Get(parts[0])
		if errors.Is(err, remotedownload.ErrJobNotFound) {
			s.writeProblem(w, r, http.StatusNotFound, "remote_download_job_not_found", "离线下载任务不存在", "")
			return
		}
		if err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "remote_download_jobs_unavailable", "离线下载任务暂不可用", "")
			return
		}
		s.writeJSON(w, http.StatusOK, job)
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, POST, DELETE")
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	job, err := s.remoteDownloadJobs.Get(parts[0])
	if errors.Is(err, remotedownload.ErrJobNotFound) {
		s.writeProblem(w, r, http.StatusNotFound, "remote_download_job_not_found", "离线下载任务不存在", "")
		return
	}
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "remote_download_jobs_unavailable", "离线下载任务暂不可用", "")
		return
	}
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "decide" {
		var decision contract.FileConflictDecision
		if s.decodeJSON(w, r, &decision) != nil {
			return
		}
		if err := s.audit(r, session.User.ID, "file.transfer.decide", "file-transfer-job", job.ID, "intent", map[string]any{"policy": decision.Policy, "revision": decision.Revision}); err != nil {
			s.writeProblem(w, r, 503, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		if err := s.remoteDownloadJobs.Decide(job.ID, decision); err != nil {
			s.writeProblem(w, r, 409, "file_conflict_stale", "同名项的任务状态已变化，请刷新后重试", "")
			return
		}
		_ = s.audit(r, session.User.ID, "file.transfer.decide", "file-transfer-job", job.ID, "success", map[string]any{"policy": decision.Policy, "revision": decision.Revision})
		s.writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if r.Method == http.MethodPost && len(parts) == 2 {
		if err := s.audit(r, session.User.ID, "file.remote_download.cancel", "file-download-job", job.ID, "intent", nil); err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		if !s.cancelRemoteDownloadJob(job.ID) {
			s.writeProblem(w, r, http.StatusConflict, "remote_download_job_finished", "该离线下载任务已经结束", "")
			return
		}
		_ = s.audit(r, session.User.ID, "file.remote_download.cancel", "file-download-job", job.ID, "success", nil)
		s.writeJSON(w, http.StatusAccepted, job)
		return
	}
	if r.Method == http.MethodDelete && len(parts) == 1 {
		if err := s.audit(r, session.User.ID, "file.remote_download.delete", "file-download-job", job.ID, "intent", nil); err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		err := s.remoteDownloadJobs.Delete(job.ID)
		if errors.Is(err, remotedownload.ErrJobActive) {
			s.writeProblem(w, r, http.StatusConflict, "remote_download_job_active", "请先停止离线下载任务", "")
			return
		}
		if err != nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "remote_download_jobs_unavailable", "离线下载任务暂不可用", "")
			return
		}
		_ = s.audit(r, session.User.ID, "file.remote_download.delete", "file-download-job", job.ID, "success", nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) startFileRemoteDownloadJob(
	w http.ResponseWriter,
	r *http.Request,
	input contract.FileRemoteDownloadRequest,
	source string,
	actorID string,
) {
	if s.remoteDownloadJobs == nil || !s.remoteDownloadJobs.Available() {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "remote_download_jobs_unavailable", "离线下载任务暂不可用", "")
		return
	}
	now := time.Now().UTC()
	job := contract.FileRemoteDownloadJob{
		ConflictPolicies: input.ConflictPolicies, ID: newRequestID(), State: "queued", Source: source, TargetDirectory: input.TargetDirectory,
		Name: input.Name, CreatedAt: now, UpdatedAt: now,
	}
	if input.SourceKind == "torrent" {
		job.SourceKind = "bittorrent"
	}
	jobContext, cancel, ok := s.reserveRemoteDownloadJob(job.ID)
	if !ok {
		w.Header().Set("Retry-After", "1")
		s.writeProblem(w, r, http.StatusTooManyRequests, "remote_download_queue_full", "离线下载队列已满，请稍后重试", "")
		return
	}
	release := true
	defer func() {
		if release {
			s.releaseRemoteDownloadReservation(job.ID, cancel)
			s.remoteDownloadWG.Done()
		}
	}()
	change := map[string]any{"source": source, "targetDirectory": input.TargetDirectory, "jobId": job.ID}
	if input.Name != "" {
		change["requestedName"] = input.Name
	}
	if err := s.audit(r, actorID, "file.remote_download", "directory", input.TargetDirectory, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.remoteDownloadJobs.Create(job); err != nil {
		_ = s.audit(r, actorID, "file.remote_download", "directory", input.TargetDirectory, "failure", map[string]any{
			"source": source, "targetDirectory": input.TargetDirectory, "jobId": job.ID,
			"errorCode": "remote_download_jobs_unavailable",
		})
		s.writeProblem(w, r, http.StatusServiceUnavailable, "remote_download_jobs_unavailable", "离线下载任务暂不可用", "")
		return
	}
	task := fileRemoteDownloadTask{
		job: job, input: input, actorID: actorID, sourceIP: s.remoteIP(r), requestID: requestID(r),
	}
	release = false
	go s.runFileRemoteDownloadJob(jobContext, cancel, task)
	s.writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) reserveRemoteDownloadJob(id string) (context.Context, context.CancelCauseFunc, bool) {
	s.remoteDownloadMu.Lock()
	defer s.remoteDownloadMu.Unlock()
	if s.remoteDownloadClosing || s.remoteDownloadPending >= remotedownload.MaxQueuedJobs {
		return nil, nil, false
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	s.remoteDownloadPending++
	s.remoteDownloadCancels[id] = cancel
	s.remoteDownloadWG.Add(1)
	return ctx, cancel, true
}

func (s *Server) releaseRemoteDownloadReservation(id string, cancel context.CancelCauseFunc) {
	cancel(context.Canceled)
	s.remoteDownloadMu.Lock()
	delete(s.remoteDownloadCancels, id)
	if s.remoteDownloadPending > 0 {
		s.remoteDownloadPending--
	}
	s.remoteDownloadMu.Unlock()
}

func (s *Server) runFileRemoteDownloadJob(ctx context.Context, cancel context.CancelCauseFunc, task fileRemoteDownloadTask) {
	defer s.remoteDownloadWG.Done()
	defer s.releaseRemoteDownloadReservation(task.job.ID, cancel)
	if task.accessLifetime != nil {
		stop := context.AfterFunc(task.accessLifetime, func() { cancel(errors.New("remote files disabled by this center")) })
		defer stop()
		if task.accessLifetime.Err() != nil {
			cancel(errors.New("remote files disabled by this center"))
		}
	}
	for _, lifetime := range task.hostAccessLifetimes {
		stop := context.AfterFunc(lifetime, func() { cancel(errors.New("remote files disabled for this host")) })
		defer stop()
		if lifetime.Err() != nil {
			cancel(errors.New("remote files disabled for this host"))
		}
	}
	// BT resource admission precedes the common transfer slot, leaving a slot
	// available for HTTP/cross-host work while another BT job is still queued.
	if task.input.SourceKind == "torrent" && s.btGate != nil {
		select {
		case s.btGate <- struct{}{}:
			defer func() { <-s.btGate }()
		case <-ctx.Done():
			s.finishRemoteDownloadJob(task, remoteDownloadJobCancellationResult(ctx, contract.FileTransferEvent{State: "error", Code: "remote_download_cancelled"}))
			return
		}
	}
	select {
	case s.remoteDownloadGate <- struct{}{}:
	case <-ctx.Done():
		result := contract.FileTransferEvent{State: "error", Code: "remote_download_cancelled"}
		result = remoteDownloadJobCancellationResult(ctx, result)
		s.finishRemoteDownloadJob(task, result)
		return
	}
	gateHeld := true
	defer func() {
		if gateHeld {
			<-s.remoteDownloadGate
		}
	}()
	transferContext, transferCancel := context.WithTimeout(ctx, panelFileTransferMaxDuration)
	defer transferCancel()
	job := task.job
	lastPersistedAt := time.Now()
	lastPersistedBytes := int64(0)
	persistFailed := false
	update := func(event contract.FileTransferEvent) bool {
		if event.State == "committing" {
			event.State = "confirming"
		}
		event = remoteDownloadJobCancellationResult(ctx, event)
		now := time.Now().UTC()
		job.State = event.State
		job.Conflict = event.Conflict
		if event.Conflict != nil {
			job.Revision++
			job.Decision = nil
		} else {
			job.Decision = nil
		}
		job.TransferMode, job.Peers, job.SpeedBytes = event.TransferMode, event.Peers, event.SpeedBytes
		job.SourceBytes = event.SourceBytes
		if event.State == "error" {
			if event.Code == "remote_download_cancelled" {
				job.State = "cancelled"
			} else if event.Code == "remote_download_interrupted" {
				job.State = "interrupted"
			} else {
				job.State = "error"
			}
			job.Code = event.Code
			job.FinishedAt = timePointer(now)
		}
		if event.State == "skipped" {
			job.FinishedAt = timePointer(now)
			job.Code = ""
		}
		if event.State == "partial" {
			job.FinishedAt = timePointer(now)
			job.Entry = event.Entry
			job.Result = event.Result
			job.Code = ""
		}
		if event.State == "complete" {
			job.State = "complete"
			job.Code = ""
			job.Entry = event.Entry
			job.FinishedAt = timePointer(now)
		}
		if event.Name != "" {
			job.Name = event.Name
		}
		if event.LoadedBytes >= 0 {
			job.LoadedBytes = event.LoadedBytes
		}
		if event.TotalBytes > 0 {
			job.TotalBytes = event.TotalBytes
		}
		job.UpdatedAt = now
		terminal := job.FinishedAt != nil
		persist := terminal || event.State != "transferring" ||
			time.Since(lastPersistedAt) >= remoteDownloadProgressPersistInterval ||
			!persistFailed && job.LoadedBytes-lastPersistedBytes >= remoteDownloadProgressPersistBytes
		persistErr := s.remoteDownloadJobs.Update(job, persist)
		if persist {
			lastPersistedAt = time.Now()
			lastPersistedBytes = job.LoadedBytes
			persistFailed = persistErr != nil
		}
		return persistErr == nil
	}
	if job.ConflictPolicies != nil {
		session := &filemanager.ConflictSession{Policies: *job.ConflictPolicies}
		session.Prompt = func(ctx context.Context, conflict contract.FileConflict) (contract.FileConflictDecision, error) {
			return s.awaitTransferConflict(ctx, &job, conflict, update, func() {
				if gateHeld {
					<-s.remoteDownloadGate
					gateHeld = false
				}
			}, func() error {
				select {
				case s.remoteDownloadGate <- struct{}{}:
					gateHeld = true
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}
		transferContext = filemanager.WithFileConflicts(transferContext, session)
	}
	var result contract.FileTransferEvent
	if task.execute != nil {
		result = task.execute(transferContext, update)
	} else {
		result = s.executeFileRemoteDownload(transferContext, task.input, task.requestID, update)
	}
	result = remoteDownloadJobCancellationResult(ctx, result)
	if result.State != "complete" && result.State != "error" && result.State != "skipped" && result.State != "partial" {
		result = contract.FileTransferEvent{State: "error", LoadedBytes: job.LoadedBytes, TotalBytes: job.TotalBytes, Name: job.Name, Code: "remote_download_interrupted"}
		update(result)
	}
	s.appendRemoteDownloadJobAudit(task, result)
}

func (s *Server) finishRemoteDownloadJob(task fileRemoteDownloadTask, result contract.FileTransferEvent) {
	now := time.Now().UTC()
	job := task.job
	job.State = "cancelled"
	if result.Code == "remote_download_interrupted" {
		job.State = "interrupted"
	}
	job.Code = result.Code
	job.UpdatedAt = now
	job.FinishedAt = timePointer(now)
	_ = s.remoteDownloadJobs.Update(job, true)
	s.appendRemoteDownloadJobAudit(task, result)
}

func remoteDownloadJobCancellationResult(ctx context.Context, result contract.FileTransferEvent) contract.FileTransferEvent {
	if result.State != "error" || ctx.Err() == nil {
		return result
	}
	if errors.Is(context.Cause(ctx), errRemoteDownloadServerClosing) {
		result.Code = "remote_download_interrupted"
	} else {
		result.Code = "remote_download_cancelled"
	}
	return result
}

func (s *Server) appendRemoteDownloadJobAudit(task fileRemoteDownloadTask, result contract.FileTransferEvent) {
	change := map[string]any{
		"source": task.job.Source, "targetDirectory": task.job.TargetDirectory,
		"jobId": task.job.ID, "bytes": result.LoadedBytes,
	}
	if result.Name != "" {
		change["targetName"] = result.Name
	}
	if result.Code != "" {
		change["errorCode"] = result.Code
	}
	resultName := "failure"
	targetKind := "directory"
	targetID := task.job.TargetDirectory
	if result.State == "complete" && result.Entry != nil {
		resultName = "success"
		targetKind = "file"
		targetID = result.Entry.Path
	}
	_ = s.store.AppendAudit(store.AuditEvent{
		ID: newRequestID(), OccurredAt: time.Now().UTC(), ActorType: actorType(task.actorID),
		ActorID: task.actorID, SourceIP: task.sourceIP, Action: fileTransferJobAuditAction(task.job),
		TargetKind: targetKind, TargetID: targetID, Result: resultName,
		RequestID: task.requestID, Change: change,
	}, store.MaxAuditEntries)
}

func fileTransferJobAuditAction(job contract.FileRemoteDownloadJob) string {
	if job.SourceKind == "cross-host" {
		return "file.transfer.copy"
	}
	return "file.remote_download"
}

func (s *Server) cancelRemoteDownloadJob(id string) bool {
	s.remoteDownloadMu.Lock()
	cancel, exists := s.remoteDownloadCancels[id]
	s.remoteDownloadMu.Unlock()
	if !exists {
		return false
	}
	cancel(context.Canceled)
	return true
}

func (s *Server) closeRemoteDownloadJobs() {
	s.remoteDownloadMu.Lock()
	if s.remoteDownloadClosing {
		s.remoteDownloadMu.Unlock()
		return
	}
	s.remoteDownloadClosing = true
	cancels := make([]context.CancelCauseFunc, 0, len(s.remoteDownloadCancels))
	for _, cancel := range s.remoteDownloadCancels {
		cancels = append(cancels, cancel)
	}
	s.remoteDownloadMu.Unlock()
	for _, cancel := range cancels {
		cancel(errRemoteDownloadServerClosing)
	}
	s.remoteDownloadWG.Wait()
}

func timePointer(value time.Time) *time.Time { return &value }
