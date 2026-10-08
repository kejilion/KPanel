package panel

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/backupremote"
	"github.com/kejilion/kejilion-panel/internal/hostbackup"
)

type backupRemoteTransport interface {
	Upload(context.Context, string, *os.File, int64) error
	Download(context.Context, string, string) (int64, error)
	Delete(context.Context, string) error
	List(context.Context) ([]backupremote.Object, error)
	Test(context.Context) error
	Close()
}

func (s *Server) newBackupRemoteClient(storage backupremote.Storage) (backupRemoteTransport, error) {
	if s.backupRemoteClient != nil {
		return s.backupRemoteClient(storage)
	}
	return backupremote.NewClient(storage)
}

func decodeBackupSettings(r *http.Request, out any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return backupremote.ErrInvalid
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 16385))
	if err != nil || len(b) > 16384 || backup.Decode(b, out) != nil {
		return backupremote.ErrInvalid
	}
	return nil
}

func (s *Server) remoteBackupError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, backupremote.ErrConflict):
		s.writeProblem(w, r, 409, "backup_settings_changed", "备份设置已变更，请刷新后重试", "")
	case errors.Is(err, backupremote.ErrInvalid):
		s.writeProblem(w, r, 400, "invalid_backup_settings", "请检查远程存储、时间和备份密码", "")
	case errors.Is(err, backupremote.ErrLimit):
		s.writeProblem(w, r, 400, "backup_remote_limit", "远程存储或文件数量超过限制，请使用独立的备份目录", "")
	case errors.Is(err, backupremote.ErrUnavailable):
		s.writeProblem(w, r, 502, "backup_remote_unavailable", "远程连接失败，请检查地址、凭据和读写权限", "")
	default:
		s.backupError(w, r, err)
	}
}

func (s *Server) serveRemoteBackups(w http.ResponseWriter, r *http.Request, p []string) bool {
	if len(p) < 2 {
		return false
	}
	matched := p[1] == "settings" || p[1] == "storage" || p[1] == "schedule" || p[1] == "remote-import" || len(p) == 3 && p[2] == "upload"
	if !matched {
		return false
	}
	if s.backupRemote == nil {
		s.remoteBackupError(w, r, backup.ErrBusy)
		return true
	}
	var err error
	switch {
	case len(p) == 2 && p[1] == "settings" && r.Method == "GET":
		s.writeJSON(w, 200, s.backupSettingsSnapshot())
		return true
	case len(p) == 2 && p[1] == "storage" && r.Method == "PUT":
		var v struct {
			Revision string               `json:"revision"`
			Storage  backupremote.Storage `json:"storage"`
		}
		if err = decodeBackupSettings(r, &v); err == nil {
			err = s.backupRemote.PutStorage(v.Revision, v.Storage)
		}
	case len(p) == 3 && p[1] == "storage" && r.Method == "DELETE":
		var v struct {
			Revision string `json:"revision"`
		}
		if err = decodeBackupSettings(r, &v); err == nil {
			err = s.backupRemote.DeleteStorage(v.Revision, p[2])
		}
	case len(p) == 4 && p[1] == "storage" && (p[3] == "test" && r.Method == "POST" || p[3] == "files" && r.Method == "GET"):
		s.remoteBackupFiles(w, r, p[2], p[3] == "test")
		return true
	case len(p) == 2 && p[1] == "schedule" && r.Method == "PUT":
		var v struct {
			Revision string                `json:"revision"`
			Schedule backupremote.Schedule `json:"schedule"`
		}
		if err = decodeBackupSettings(r, &v); err == nil {
			err = s.backupRemote.PutSchedule(v.Revision, v.Schedule, time.Now())
		}
	case len(p) == 3 && p[1] == "schedule" && p[2] == "run" && r.Method == "POST":
		var record backup.Record
		plan, revision := s.backupRemote.PlanWithRevision()
		record, err = s.startScheduledBackup(plan, revision)
		if err == nil {
			s.writeJSON(w, 202, record)
			return true
		}
	case len(p) == 2 && p[1] == "remote-import" && r.Method == "POST":
		s.importRemoteBackup(w, r)
		return true
	case len(p) == 3 && p[2] == "upload" && r.Method == "POST" && backup.ValidID(p[1]):
		s.retryBackupUpload(w, r, p[1])
		return true
	default:
		err = backupremote.ErrInvalid
	}
	if err != nil {
		s.remoteBackupError(w, r, err)
	} else {
		s.writeJSON(w, 200, s.backupSettingsSnapshot())
	}
	return true
}

func (s *Server) remoteBackupFiles(w http.ResponseWriter, r *http.Request, id string, test bool) {
	select {
	case s.backupRemoteGate <- struct{}{}:
		defer func() { <-s.backupRemoteGate }()
	default:
		s.backupError(w, r, backup.ErrBusy)
		return
	}
	storage, err := s.backupRemote.Storage(id)
	if err != nil {
		s.remoteBackupError(w, r, err)
		return
	}
	client, err := s.newBackupRemoteClient(storage)
	if err != nil {
		s.remoteBackupError(w, r, err)
		return
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if test {
		if err := client.Test(ctx); err != nil {
			s.remoteBackupError(w, r, err)
			return
		}
		s.writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	objects, err := client.List(ctx)
	if err != nil {
		s.remoteBackupError(w, r, err)
		return
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Modified.After(objects[j].Modified) })
	s.writeJSON(w, 200, map[string]any{"items": objects})
}

func remoteReceipt(storage backupremote.Storage, record backup.Record) *backup.RemoteCopy {
	key := "kpanel-" + record.CreatedAt.UTC().Format("20060102T150405Z") + "-" + record.ID + ".kpb"
	return &backup.RemoteCopy{StorageID: storage.ID, StorageName: storage.Name, Destination: storage.Fingerprint(), Key: key, Status: "pending"}
}

func (s *Server) startBackupExport(input backupRequest, keep int) (backup.Record, error) {
	automatic := keep > 0
	if backup.ValidatePassword(input.Password) != nil {
		return backup.Record{}, backup.ErrInvalid
	}
	modules, err := backup.Selection(input.Modules)
	if err != nil {
		return backup.Record{}, err
	}
	input.Modules = modules
	var storage backupremote.Storage
	if input.StorageID != "" {
		if s.backupRemote == nil {
			return backup.Record{}, backup.ErrInvalid
		}
		storage, err = s.backupRemote.Storage(input.StorageID)
		if err != nil {
			return backup.Record{}, err
		}
	}
	record, err := s.backups.Reserve("export", modules)
	if err != nil {
		return record, err
	}
	record.Automatic = automatic
	planSettings := backupremote.Settings{}
	currentPlan := false
	if automatic && s.backupRemote != nil {
		planSettings = s.backupRemote.Snapshot()
		currentPlan = (input.scheduleRevision == "" || input.scheduleRevision == planSettings.Revision) &&
			sameBackupModules(planSettings.Schedule.Modules, input.Modules) && planSettings.Schedule.StorageID == input.StorageID
	}
	if storage.ID != "" {
		record.Remote = remoteReceipt(storage, record)
	}
	if err = s.backups.Update(record.ID, func(r *backup.Record) { r.Automatic = automatic; r.Remote = record.Remote }); err != nil {
		_ = s.backups.Abort(record.ID, "start_failed")
		return record, err
	}
	if automatic && currentPlan {
		if err = s.backupRemote.MarkRunForRevision(planSettings.Revision, record.CreatedAt); errors.Is(err, backupremote.ErrConflict) {
			currentPlan = false
		} else if err != nil {
			_ = s.backups.Abort(record.ID, "start_failed")
			return record, err
		}
	}
	err = s.backups.Run(record.ID, func(ctx context.Context, id string) (result error) {
		defer func() {
			if automatic && currentPlan {
				code := ""
				if result != nil {
					code = backup.FailureCode(result)
				}
				_ = s.backupRemote.SetRunErrorForRevision(planSettings.Revision, code)
			}
		}()
		if automatic && len(hostBackupModules(input.Modules)) > 0 {
			var inventory hostbackup.Preview
			if err := s.backupAgentJSON(ctx, "GET", "/v1/backups/inventory", nil, &inventory); err != nil {
				return err
			}
			input.AgentRevision = inventory.Revision
		}
		if err := s.generateBackup(ctx, id, input); err != nil {
			return err
		}
		if storage.ID != "" {
			if err := s.uploadBackup(ctx, id, storage, record.Remote); err != nil {
				return err
			}
		}
		if automatic {
			if err := s.pruneAutomaticBackups(ctx, id, storage, keep); err != nil {
				return s.backups.Update(id, func(r *backup.Record) { r.ErrorCode = "retention_failed" })
			}
		}
		return nil
	})
	if err != nil {
		_ = s.backups.Abort(record.ID, "start_failed")
	}
	return record, err
}

func (s *Server) uploadBackup(ctx context.Context, id string, storage backupremote.Storage, receipt *backup.RemoteCopy) error {
	pending := *receipt
	pending.Status = "uploading"
	if err := s.backups.Update(id, func(r *backup.Record) { r.Stage = "uploading_remote"; r.Remote = &pending }); err != nil {
		return err
	}
	client, err := s.newBackupRemoteClient(storage)
	if err == nil {
		defer client.Close()
		var file io.ReadCloser
		f, e := backup.OpenRegular(filepath.Join(s.backups.Root, id, "backup.kpb"), backup.MaxEncryptedBytes)
		if e != nil {
			err = e
		} else {
			file = f
			defer file.Close()
			info, e := f.Stat()
			if e != nil {
				err = e
			} else {
				err = client.Upload(ctx, receipt.Key, f, info.Size())
			}
		}
	}
	done := *receipt
	done.Status = "completed"
	if err != nil {
		done.Status = "failed"
	}
	if saveErr := s.backups.Update(id, func(r *backup.Record) { r.Remote = &done }); saveErr != nil {
		return saveErr
	}
	if err != nil {
		return &backup.Failure{Code: "remote_upload_failed", Err: err}
	}
	return nil
}

func (s *Server) retryBackupUpload(w http.ResponseWriter, r *http.Request, id string) {
	var input struct {
		StorageID string `json:"storageId"`
	}
	if err := decodeBackupSettings(r, &input); err != nil {
		s.remoteBackupError(w, r, err)
		return
	}
	storage, err := s.backupRemote.Storage(input.StorageID)
	if err != nil {
		s.remoteBackupError(w, r, err)
		return
	}
	previous, err := s.backups.Get(id)
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	record, err := s.backups.ReserveUpload(id)
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	receipt := remoteReceipt(storage, record)
	// Retrying an automatic copy to its original destination preserves its
	// identity. Manual exports and copies to another target are not enrolled.
	automatic := record.Automatic && record.Remote != nil && record.Remote.StorageID == storage.ID && record.Remote.Destination == storage.Fingerprint()
	err = s.backups.Update(id, func(r *backup.Record) { r.Remote = receipt; r.Automatic = automatic })
	if err == nil {
		err = s.backups.Run(id, func(ctx context.Context, id string) error {
			if err := s.uploadBackup(ctx, id, storage, receipt); err != nil {
				return err
			}
			// Uploading an existing package does not retry retention. Preserve
			// that warning until a later automatic export actually prunes.
			if automatic && previous.ErrorCode == "retention_failed" {
				return s.backups.Update(id, func(r *backup.Record) { r.ErrorCode = "retention_failed" })
			}
			return nil
		})
	}
	if err != nil {
		_ = s.backups.Abort(id, "start_failed")
		s.backupError(w, r, err)
		return
	}
	s.writeJSON(w, 202, record)
}

func (s *Server) importRemoteBackup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		StorageID string `json:"storageId"`
		Key       string `json:"key"`
		Password  string `json:"password"`
	}
	if err := decodeBackupSettings(r, &input); err != nil || !backupremote.ValidObject(input.Key) || backup.ValidatePassword(input.Password) != nil {
		s.remoteBackupError(w, r, backupremote.ErrInvalid)
		return
	}
	storage, err := s.backupRemote.Storage(input.StorageID)
	if err != nil {
		s.remoteBackupError(w, r, err)
		return
	}
	record, err := s.backups.Reserve("import", nil)
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	err = s.backups.Run(record.ID, func(ctx context.Context, id string) error {
		password := []byte(input.Password)
		input.Password = ""
		defer clear(password)
		if err := s.backups.Update(id, func(r *backup.Record) { r.Stage = "downloading_remote" }); err != nil {
			return err
		}
		client, err := s.newBackupRemoteClient(storage)
		if err != nil {
			return err
		}
		defer client.Close()
		size, err := client.Download(ctx, input.Key, filepath.Join(s.backups.Root, id, "upload.kpb"))
		if err != nil {
			return &backup.Failure{Code: "remote_download_failed", Err: err}
		}
		return s.inspectBackup(ctx, id, password, size)
	})
	if err != nil {
		_ = s.backups.Abort(record.ID, "start_failed")
		s.backupError(w, r, err)
		return
	}
	s.writeJSON(w, 202, record)
}

// Only successful automatic exports with a matching destination receipt may
// be removed. Files discovered by remote listing are never retention inputs.
func (s *Server) pruneAutomaticBackups(ctx context.Context, owner string, storage backupremote.Storage, keep int) error {
	if keep < 1 {
		return backupremote.ErrInvalid
	}
	items := []backup.Record{}
	for _, r := range s.backups.List() {
		if r.ID == owner || !r.Automatic || r.Action != "export" || r.Status != "completed" {
			continue
		}
		if storage.ID == "" && r.Remote == nil || storage.ID != "" && r.Remote != nil && r.Remote.Status == "completed" && r.Remote.Destination == storage.Fingerprint() {
			items = append(items, r)
		}
	}
	if len(items) < keep {
		return nil
	}
	var client backupRemoteTransport
	if storage.ID != "" {
		var err error
		client, err = s.newBackupRemoteClient(storage)
		if err != nil {
			return err
		}
		defer client.Close()
	}
	for _, r := range items[keep-1:] {
		if client != nil {
			if err := client.Delete(ctx, r.Remote.Key); err != nil {
				return err
			}
		}
		if err := s.backups.PruneExport(owner, r.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) startScheduledBackup(plan backupremote.Schedule, revision string) (backup.Record, error) {
	if plan.Validate() != nil || backup.ValidatePassword(plan.Password) != nil {
		return backup.Record{}, backupremote.ErrInvalid
	}
	return s.startBackupExport(backupRequest{Modules: plan.Modules, StorageID: plan.StorageID, Password: plan.Password, scheduleRevision: revision}, plan.Keep)
}
func (s *Server) startBackupSchedule(parent context.Context) {
	s.backupScheduleMu.Lock()
	defer s.backupScheduleMu.Unlock()
	if s.backupRemote == nil || s.backupScheduleCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.backupScheduleCancel = cancel
	s.backupScheduleWG.Add(1)
	go func() {
		defer s.backupScheduleWG.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				plan, revision, run, err := s.backupRemote.ClaimWithRevision(now, s.backups.Busy())
				if err == nil && run {
					if _, err = s.startScheduledBackup(plan, revision); err != nil {
						_ = s.backupRemote.SetRunErrorForRevision(revision, backup.FailureCode(err))
					}
				}
			}
		}
	}()
}
func (s *Server) stopBackupSchedule() {
	s.backupScheduleMu.Lock()
	if s.backupScheduleCancel != nil {
		s.backupScheduleCancel()
	}
	s.backupScheduleMu.Unlock()
	s.backupScheduleWG.Wait()
}
