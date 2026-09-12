package panel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/hostbackup"
	"github.com/kejilion/kejilion-panel/internal/version"
)

type backupRequest struct {
	Password      string   `json:"password"`
	Modules       []string `json:"modules"`
	Revision      string   `json:"revision"`
	AgentRevision string   `json:"agentRevision"`
}
type backupStreamer interface {
	OpenStream(context.Context, string, string, string, string, io.Reader, http.Header, int64) (*http.Response, error)
}

func (s *Server) backupError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, title := http.StatusConflict, "backup_failed", "备份恢复未完成，请刷新状态后重试"
	if errors.Is(err, backup.ErrInvalid) {
		status, code, title = http.StatusBadRequest, "invalid_backup", "备份包、密码或所选内容无效"
	}
	if errors.Is(err, backup.ErrBusy) {
		code, title = "backup_busy", "已有备份恢复任务正在执行"
	}
	if errors.Is(err, backup.ErrNotFound) {
		status, code, title = 404, "backup_not_found", "备份记录不存在"
	}
	s.writeProblem(w, r, status, code, title, "")
}
func (s *Server) decodeBackupRequest(r *http.Request) (backupRequest, error) {
	var input backupRequest
	data, err := io.ReadAll(io.LimitReader(r.Body, 16385))
	if err != nil || len(data) > 16384 || backup.Decode(data, &input) != nil {
		return input, backup.ErrInvalid
	}
	return input, nil
}
func (s *Server) handleBackups(w http.ResponseWriter, r *http.Request) {
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method != "GET" && (!s.checkOrigin(w, r) || !s.checkCSRF(w, r, session)) {
		return
	}
	if s.backups == nil {
		s.backupError(w, r, backup.ErrBusy)
		return
	}
	if err := s.backups.Sweep(time.Now()); err != nil {
		s.backupError(w, r, err)
		return
	}
	if r.URL.RawQuery != "" || r.URL.RawPath != "" {
		s.backupError(w, r, backup.ErrInvalid)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/backups"), "/")
	if r.Method != "GET" {
		action := "backup.operation"
		if len(parts) > 1 {
			action = "backup." + parts[len(parts)-1]
		}
		if err := s.audit(r, session.User.ID, action, "backup", "", "intent", nil); err != nil {
			s.writeProblem(w, r, 503, "audit_unavailable", "无法记录备份操作，请稍后重试", "")
			return
		}
	}
	if len(parts) == 1 && parts[0] == "" && r.Method == "GET" {
		s.writeJSON(w, 200, map[string]any{"items": s.backups.List(), "maxBytes": backup.MaxBytes})
		return
	}
	if len(parts) == 2 {
		if parts[1] == "inventory" && r.Method == "GET" {
			s.backupInventory(w, r)
			return
		}
		if parts[1] == "export" && r.Method == "POST" {
			s.backupExport(w, r)
			return
		}
		if parts[1] == "import" && r.Method == "POST" {
			s.backupImport(w, r)
			return
		}
	}
	if len(parts) < 2 || !backup.ValidID(parts[1]) {
		s.backupError(w, r, backup.ErrNotFound)
		return
	}
	record, err := s.backups.Get(parts[1])
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	if len(parts) == 2 {
		if r.Method == "GET" {
			s.writeJSON(w, 200, record)
			return
		}
		if r.Method == "DELETE" {
			if record.Status == "restarting" {
				s.backupError(w, r, backup.ErrBusy)
				return
			}
			if err := s.backups.DeleteWith(record.ID, func(current backup.Record) error {
				if current.AgentID != "" {
					return s.backupAgentJSON(r.Context(), "DELETE", "/v1/backups/"+current.AgentID, nil, nil)
				}
				return nil
			}); err != nil {
				s.backupError(w, r, err)
				return
			}
			w.WriteHeader(204)
			return
		}
	}
	if len(parts) == 3 && parts[2] == "download" && r.Method == "GET" && record.Status == "completed" && record.Action == "export" {
		f, err := backup.OpenRegular(filepath.Join(s.backups.Root, record.ID, "backup.kpb"), backup.MaxEncryptedBytes)
		if err != nil {
			s.backupError(w, r, err)
			return
		}
		defer f.Close()
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Hour))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="kpanel-`+record.ID+`.kpb"`)
		http.ServeContent(w, r, "backup.kpb", time.Time{}, f)
		return
	}
	if len(parts) == 3 && parts[2] == "recover" && r.Method == "POST" && record.Status == "failed" && record.AgentID != "" {
		s.backupRecover(w, r, record)
		return
	}
	if len(parts) == 3 && parts[2] == "restore" && r.Method == "POST" {
		s.backupRestore(w, r, record)
		return
	}
	if len(parts) == 3 && parts[2] == "preview" && r.Method == "POST" && record.Status == "ready" {
		if s.backups.Busy() {
			s.backupError(w, r, backup.ErrBusy)
			return
		}
		revision := "host-only"
		if slices.Contains(record.Modules, "panel") {
			data, err := s.exportPanelBackup(r.Context())
			if err != nil {
				s.backupError(w, r, err)
				return
			}
			revision, err = panelBackupRevision(data)
			if err != nil {
				s.backupError(w, r, err)
				return
			}
		}
		agentRevision := record.AgentRevision
		if record.AgentID != "" {
			var checked backup.Record
			if err := s.backupAgentJSON(r.Context(), "POST", "/v1/backups/"+record.AgentID+"/preview", struct{}{}, &checked); err != nil {
				s.backupError(w, r, err)
				return
			}
			agentRevision = checked.TargetRevision
		}
		if err := s.backups.Update(record.ID, func(r *backup.Record) { r.TargetRevision = revision; r.AgentRevision = agentRevision }); err != nil {
			s.backupError(w, r, err)
			return
		}
		updated, _ := s.backups.Get(record.ID)
		s.writeJSON(w, 200, updated)
		return
	}
	s.backupError(w, r, backup.ErrInvalid)
}

func (s *Server) backupInventory(w http.ResponseWriter, r *http.Request) {
	data, err := s.exportPanelBackup(r.Context())
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	hash := sha256.Sum256(data)
	out := map[string]any{"revision": hex.EncodeToString(hash[:]), "panelBytes": len(data), "maxBytes": backup.MaxBytes, "hostAvailable": false}
	var host hostbackup.Preview
	if err := s.backupAgentJSON(r.Context(), "GET", "/v1/backups/inventory", nil, &host); err == nil {
		out["hostAvailable"] = true
		out["host"] = host
	}
	s.writeJSON(w, 200, out)
}
func (s *Server) backupAgentJSON(ctx context.Context, method, path string, input, output any) error {
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	response, err := s.agent.Do(ctx, method, path, "", "", body)
	if err != nil {
		return err
	}
	if method == "DELETE" && response.StatusCode == http.StatusNotFound {
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var problem struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(response.Body, &problem) == nil && (problem.Code == "backup_host_busy" || problem.Code == "backup_busy") {
			return &backup.Failure{Code: "host_busy", Err: backup.ErrBusy}
		}
		return errors.New("backup Agent request failed")
	}
	if output != nil {
		return backup.Decode(response.Body, output)
	}
	return nil
}
func (s *Server) backupAgentWait(ctx context.Context, id string) (backup.Record, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var record backup.Record
		if err := s.backupAgentJSON(ctx, "GET", "/v1/backups/"+id, nil, &record); err != nil {
			return record, err
		}
		if record.Status == "completed" || record.Status == "ready" {
			return record, nil
		}
		if record.Status == "failed" {
			return record, &backup.Failure{Code: record.ErrorCode, Err: errors.New("host backup operation failed")}
		}
		select {
		case <-ctx.Done():
			return record, ctx.Err()
		case <-ticker.C:
		}
	}
}
func (s *Server) backupAgentCopy(ctx context.Context, method, id, module string, file *os.File) error {
	client, ok := s.agent.(backupStreamer)
	if !ok {
		return errors.New("Agent streaming unavailable")
	}
	var reader io.Reader
	length := int64(0)
	if method == "PUT" {
		reader = file
		info, err := file.Stat()
		if err != nil {
			return err
		}
		length = info.Size()
	}
	response, err := client.OpenStream(ctx, method, "/v1/backups/"+id+"/"+module, "", "", reader, nil, length)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("Agent backup transfer failed")
	}
	if method == "GET" {
		n, err := io.Copy(file, io.LimitReader(response.Body, backup.MaxBytes+1))
		if err != nil {
			return err
		}
		if n > backup.MaxBytes {
			return backup.ErrInvalid
		}
		return file.Sync()
	}
	return nil
}
func hostBackupModules(modules []string) []string {
	out := []string{}
	for _, m := range modules {
		if m != "panel" {
			out = append(out, m)
		}
	}
	return out
}

func (s *Server) backupExport(w http.ResponseWriter, r *http.Request) {
	input, err := s.decodeBackupRequest(r)
	if err != nil || backup.ValidatePassword(input.Password) != nil {
		s.backupError(w, r, backup.ErrInvalid)
		return
	}
	modules, err := backup.Selection(input.Modules)
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	record, err := s.backups.Reserve("export", modules)
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	err = s.backups.Run(record.ID, func(ctx context.Context, id string) error {
		defer func() { input.Password = "" }()
		dir := filepath.Join(s.backups.Root, id)
		sources := []backup.Source{}
		defer func() {
			for _, m := range modules {
				_ = os.Remove(filepath.Join(dir, m+".payload"))
			}
		}()
		if slices.Contains(modules, "panel") {
			data, err := s.exportPanelBackup(ctx)
			if err != nil {
				return err
			}
			if err := backup.AtomicFile(filepath.Join(dir, "panel.payload"), data); err != nil {
				return err
			}
			sources = append(sources, backup.Source{Module: "panel", Path: filepath.Join(dir, "panel.payload")})
		}
		if host := hostBackupModules(modules); len(host) > 0 {
			var job backup.Record
			if err := s.backupAgentJSON(ctx, "POST", "/v1/backups", hostbackup.Request{Action: "export", Modules: host, Revision: input.AgentRevision}, &job); err != nil {
				return err
			}
			if err := s.backups.Update(id, func(r *backup.Record) { r.Stage = "backing_up_services"; r.AgentID = job.ID }); err != nil {
				return err
			}
			if _, err := s.backupAgentWait(ctx, job.ID); err != nil {
				return err
			}
			defer s.backupAgentJSON(context.Background(), "DELETE", "/v1/backups/"+job.ID, nil, nil)
			for _, module := range host {
				f, err := os.OpenFile(filepath.Join(dir, module+".payload"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
				if err != nil {
					return err
				}
				err = s.backupAgentCopy(ctx, "GET", job.ID, module, f)
				err = errors.Join(err, f.Close())
				if err != nil {
					return err
				}
				sources = append(sources, backup.Source{Module: module, Path: f.Name()})
			}
		}
		var bytes int64
		for _, source := range sources {
			info, err := os.Stat(source.Path)
			if err != nil {
				return err
			}
			bytes += info.Size()
		}
		if err := backup.RequireSpace(dir, bytes+(32<<20)); err != nil {
			return err
		}
		if err := s.backups.Update(id, func(r *backup.Record) { r.Stage = "encrypting" }); err != nil {
			return err
		}
		path := filepath.Join(dir, "backup.kpb")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		manifest, err := backup.WriteContext(ctx, f, input.Password, version.Version, sources)
		if err == nil {
			err = f.Sync()
		}
		info, statErr := f.Stat()
		err = errors.Join(err, statErr, f.Close())
		if err != nil {
			_ = os.Remove(path)
			return err
		}
		return s.backups.Update(id, func(r *backup.Record) { r.Manifest = &manifest; r.Size = info.Size() })
	})
	if err != nil {
		_ = s.backups.Abort(record.ID, "start_failed")
		s.backupError(w, r, err)
		return
	}
	s.writeJSON(w, 202, record)
}

func (s *Server) backupImport(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > backup.MaxEncryptedBytes+(16<<10) {
		s.backupError(w, r, backup.ErrInvalid)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, backup.MaxEncryptedBytes+(16<<10))
	mr, err := r.MultipartReader()
	if err != nil {
		s.backupError(w, r, backup.ErrInvalid)
		return
	}
	part, err := mr.NextPart()
	if err != nil || part.FormName() != "password" {
		s.backupError(w, r, backup.ErrInvalid)
		return
	}
	password, err := io.ReadAll(io.LimitReader(part, 257))
	if err != nil || backup.ValidatePassword(string(password)) != nil {
		s.backupError(w, r, backup.ErrInvalid)
		return
	}
	part, err = mr.NextPart()
	if err != nil || part.FormName() != "file" {
		clear(password)
		s.backupError(w, r, backup.ErrInvalid)
		return
	}
	record, err := s.backups.Reserve("import", nil)
	if err != nil {
		clear(password)
		s.backupError(w, r, err)
		return
	}
	dir := filepath.Join(s.backups.Root, record.ID)
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Hour))
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Hour))
	size, err := backup.CopyFile(filepath.Join(dir, "upload.kpb"), part, backup.MaxEncryptedBytes)
	if err == nil {
		_, err = mr.NextPart()
		if err == io.EOF {
			err = nil
		} else {
			err = backup.ErrInvalid
		}
	}
	if err != nil {
		clear(password)
		_ = s.backups.Abort(record.ID, "upload_failed")
		s.backupError(w, r, err)
		return
	}
	if err := backup.RequireSpace(dir, 2*size); err != nil {
		clear(password)
		_ = s.backups.Abort(record.ID, "insufficient_space")
		s.backupError(w, r, err)
		return
	}
	err = s.backups.Run(record.ID, func(ctx context.Context, id string) (operationErr error) {
		defer clear(password)
		defer func() {
			if operationErr != nil {
				for _, name := range []string{"upload.kpb", "panel.payload", "apps.payload", "web.payload", "docker.payload"} {
					_ = os.Remove(filepath.Join(dir, name))
				}
			}
		}()
		f, err := backup.OpenRegular(filepath.Join(dir, "upload.kpb"), backup.MaxEncryptedBytes)
		if err != nil {
			return err
		}
		defer f.Close()
		manifest, err := backup.ReadContext(ctx, f, string(password), dir)
		err = errors.Join(err, f.Close())
		if err != nil {
			return err
		}
		modules := []string{}
		for _, p := range manifest.Parts {
			modules = append(modules, p.Module)
		}
		if slices.Contains(modules, "panel") {
			data, err := backup.ReadFile(filepath.Join(dir, "panel.payload"), backup.MaxPanelBytes)
			if err != nil {
				return err
			}
			value, err := decodePanelBackup(data)
			if err != nil {
				return err
			}
			if err := s.validatePanelRestore(value, filepath.Join(dir, "validate-panel")); err != nil {
				return err
			}
		}
		revision := "host-only"
		if slices.Contains(modules, "panel") {
			current, err := s.exportPanelBackup(ctx)
			if err != nil {
				return err
			}
			revision, err = panelBackupRevision(current)
			if err != nil {
				return err
			}
		}
		if err := s.backups.Update(id, func(r *backup.Record) {
			r.Modules = modules
			r.Manifest = &manifest
			r.Size = size
			r.TargetRevision = revision
			r.Stage = "checking_services"
		}); err != nil {
			return err
		}
		if host := hostBackupModules(modules); len(host) > 0 {
			var job backup.Record
			if err := s.backupAgentJSON(ctx, "POST", "/v1/backups", hostbackup.Request{Action: "import", Modules: host}, &job); err != nil {
				return err
			}
			defer func() {
				if operationErr != nil {
					cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_ = s.backupAgentJSON(cleanup, "POST", "/v1/backups/"+job.ID+"/abort", struct{}{}, nil)
				}
			}()
			if err := s.backups.Update(id, func(r *backup.Record) { r.AgentID = job.ID }); err != nil {
				return err
			}
			for _, module := range host {
				f, err := backup.OpenRegular(filepath.Join(dir, module+".payload"), backup.MaxBytes)
				if err != nil {
					return err
				}
				err = s.backupAgentCopy(ctx, "PUT", job.ID, module, f)
				f.Close()
				if err != nil {
					return err
				}
			}
			if err := s.backupAgentJSON(ctx, "POST", "/v1/backups/"+job.ID+"/inspect", struct{}{}, nil); err != nil {
				return err
			}
			checked, err := s.backupAgentWait(ctx, job.ID)
			if err != nil {
				return err
			}
			if err := s.backups.Update(id, func(r *backup.Record) { r.AgentRevision = checked.TargetRevision }); err != nil {
				return err
			}
			for _, module := range host {
				_ = os.Remove(filepath.Join(dir, module+".payload"))
			}
		}
		return os.Remove(filepath.Join(dir, "upload.kpb"))
	})
	if err != nil {
		clear(password)
		_ = s.backups.Abort(record.ID, "start_failed")
		s.backupError(w, r, err)
		return
	}
	s.writeJSON(w, 202, record)
}

func (s *Server) backupRestore(w http.ResponseWriter, r *http.Request, source backup.Record) {
	input, err := s.decodeBackupRequest(r)
	if err != nil || source.Action != "import" || source.Status != "ready" || input.Revision == "" || input.Revision != source.TargetRevision {
		s.backupError(w, r, backup.ErrInvalid)
		return
	}
	modules, err := backup.Selection(input.Modules)
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	for _, m := range modules {
		if !slices.Contains(source.Modules, m) {
			s.backupError(w, r, backup.ErrInvalid)
			return
		}
	}
	record, pinned, err := s.backups.ReserveFrom(source.ID, modules)
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	source = pinned
	if source.TargetRevision != input.Revision {
		_ = s.backups.Abort(record.ID, "preview_changed")
		s.backupError(w, r, backup.ErrInvalid)
		return
	}
	err = s.backups.Run(record.ID, func(ctx context.Context, id string) error {
		var panelData []byte
		if slices.Contains(modules, "panel") {
			var err error
			panelData, err = backup.ReadFile(filepath.Join(s.backups.Root, source.ID, "panel.payload"), backup.MaxPanelBytes)
			if err != nil {
				return err
			}
			if _, err := decodePanelBackup(panelData); err != nil {
				return err
			}
			if err := backup.RequireSpace(s.backups.Root, 128<<20); err != nil {
				return err
			}
		}
		if slices.Contains(modules, "panel") {
			current, err := s.exportPanelBackup(ctx)
			if err != nil {
				return err
			}
			revision, err := panelBackupRevision(current)
			if err != nil {
				return err
			}
			if revision != input.Revision {
				return errors.New("panel configuration changed after preview")
			}
		}
		if err := s.backups.Update(id, func(r *backup.Record) { r.SourceID = source.ID; r.Stage = "restoring_services" }); err != nil {
			return err
		}
		if host := hostBackupModules(modules); len(host) > 0 {
			var job backup.Record
			if err := s.backupAgentJSON(ctx, "POST", "/v1/backups", hostbackup.Request{Action: "restore", Modules: host, SourceID: source.AgentID, Revision: source.AgentRevision}, &job); err != nil {
				return err
			}
			if err := s.backups.Update(id, func(r *backup.Record) { r.AgentID = job.ID }); err != nil {
				return err
			}
			result, waitErr := s.backupAgentWait(ctx, job.ID)
			if len(result.CompletedModules) > 0 {
				if err := s.backups.Update(id, func(r *backup.Record) { r.CompletedModules = result.CompletedModules }); err != nil {
					return err
				}
			}
			if waitErr != nil {
				return waitErr
			}
			if err := s.backups.Update(id, func(r *backup.Record) { r.CompletedModules = host }); err != nil {
				return err
			}
		}
		if slices.Contains(modules, "panel") {
			if err := s.backups.Update(id, func(r *backup.Record) { r.Status = "restarting"; r.Stage = "restarting" }); err != nil {
				return err
			}
			if err := stagePanelRestore(s.config, id, panelData); err != nil {
				return err
			}
			s.backupRestart <- struct{}{}
		}
		return nil
	})
	if err != nil {
		_ = s.backups.Abort(record.ID, "start_failed")
		s.backupError(w, r, err)
		return
	}
	s.writeJSON(w, 202, record)
}

func (s *Server) backupRecover(w http.ResponseWriter, r *http.Request, original backup.Record) {
	job, err := s.backups.Reserve("recover", original.Modules)
	if err != nil {
		s.backupError(w, r, err)
		return
	}
	err = s.backups.Run(job.ID, func(ctx context.Context, id string) error {
		var hostJob backup.Record
		if err := s.backupAgentJSON(ctx, "POST", "/v1/backups/"+original.AgentID+"/recover", struct{}{}, &hostJob); err != nil {
			return err
		}
		if err := s.backups.Update(id, func(r *backup.Record) { r.AgentID = hostJob.ID; r.SourceID = original.ID }); err != nil {
			return err
		}
		if _, err := s.backupAgentWait(ctx, hostJob.ID); err != nil {
			return err
		}
		var result backup.Record
		if err := s.backupAgentJSON(ctx, "GET", "/v1/backups/"+original.AgentID, nil, &result); err != nil {
			return err
		}
		return s.backups.Update(original.ID, func(r *backup.Record) {
			r.CompletedModules = result.CompletedModules
			r.Status = "failed"
			r.Stage = "rolled_back"
			r.ErrorCode = "rolled_back"
			if len(result.CompletedModules) > 0 {
				r.ErrorCode = "partially_restored"
				if !slices.Contains(r.Modules, "panel") {
					r.Status = "completed"
					r.Stage = "completed"
					r.ErrorCode = ""
				}
			}
		})
	})
	if err != nil {
		_ = s.backups.Abort(job.ID, "start_failed")
		s.backupError(w, r, err)
		return
	}
	s.writeJSON(w, 202, job)
}
