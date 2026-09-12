package hostbackup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
)

// Service is a fixed local Agent protocol. Authentication belongs to the
// enclosing Unix-socket server; no caller can supply a filesystem path.
type Service struct {
	Engine      *Engine
	Jobs        *backup.Manager
	mu          sync.Mutex
	stop        chan struct{}
	closeOnce   sync.Once
	maintenance sync.WaitGroup
}
type Request struct {
	Action   string   `json:"action"`
	Modules  []string `json:"modules"`
	Revision string   `json:"revision"`
	SourceID string   `json:"sourceId"`
}
type Preview struct {
	Version  int      `json:"version"`
	Revision string   `json:"revision"`
	Modules  []Module `json:"modules"`
}

func NewService(engine *Engine) (*Service, error) {
	jobs, err := backup.OpenManager(filepath.Join(engine.StateDir, "backup-center"))
	if err != nil {
		return nil, err
	}
	s := &Service{Engine: engine, Jobs: jobs, stop: make(chan struct{})}
	for _, record := range jobs.List() {
		directory := filepath.Join(jobs.Root, record.ID)
		if !recoveryPending(directory) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), recoveryTimeout)
		err := engine.Recover(ctx, record.ID, directory)
		cancel()
		if saveErr := s.recoveryResult(record.ID, directory, err); saveErr != nil {
			jobs.Close()
			return nil, saveErr
		}
	}
	s.maintenance.Add(1)
	go func() {
		defer s.maintenance.Done()
		timer := time.NewTicker(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-s.stop:
				return
			case now := <-timer.C:
				_ = SweepCLI(filepath.Join(engine.StateDir, "backup-cli"), now)
			}
		}
	}()
	return s, nil
}
func (s *Service) Close() {
	s.closeOnce.Do(func() { close(s.stop) })
	s.Jobs.Close()
	s.maintenance.Wait()
}
func backupJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func backupProblem(w http.ResponseWriter, err error) {
	status := http.StatusConflict
	if errors.Is(err, backup.ErrInvalid) {
		status = http.StatusBadRequest
	}
	if errors.Is(err, backup.ErrNotFound) {
		status = http.StatusNotFound
	}
	backupJSON(w, status, map[string]string{"code": "backup_unavailable", "title": "Backup operation failed"})
}
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		s.mu.Lock()
		defer s.mu.Unlock()
	}
	if err := s.Jobs.Sweep(time.Now()); err != nil {
		backupProblem(w, err)
		return
	}
	if err := s.Jobs.ExpireQueued(time.Now()); err != nil {
		backupProblem(w, err)
		return
	}
	if r.URL.RawQuery != "" || r.URL.RawPath != "" {
		backupProblem(w, backup.ErrInvalid)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/backups"), "/")
	if len(parts) == 1 && parts[0] == "" {
		if r.Method == "GET" {
			backupJSON(w, 200, map[string]any{"items": s.Jobs.List()})
			return
		}
		if r.Method == "POST" {
			s.create(w, r)
			return
		}
	}
	if len(parts) == 2 && parts[1] == "inventory" && r.Method == "GET" {
		i, err := s.Engine.Inventory(r.Context())
		if err != nil {
			backupProblem(w, err)
			return
		}
		backupJSON(w, 200, Preview{ProtocolVersion, i.Revision, i.Modules})
		return
	}
	if len(parts) < 2 || !backup.ValidID(parts[1]) {
		backupProblem(w, backup.ErrNotFound)
		return
	}
	id := parts[1]
	record, err := s.Jobs.Get(id)
	if err != nil {
		backupProblem(w, err)
		return
	}
	dir := filepath.Join(s.Jobs.Root, id)
	if len(parts) == 2 {
		if r.Method == "GET" {
			backupJSON(w, 200, record)
			return
		}
		if r.Method == "DELETE" {
			if recoveryPending(dir) {
				backupProblem(w, backup.ErrBusy)
				return
			}
			if err := s.Jobs.Delete(id); err != nil {
				backupProblem(w, err)
				return
			}
			w.WriteHeader(204)
			return
		}
	}
	if len(parts) != 3 {
		backupProblem(w, backup.ErrInvalid)
		return
	}
	name := parts[2]
	if name == "recover" && r.Method == "POST" && record.Status == "failed" {
		job, err := s.Jobs.Reserve("recover", record.Modules)
		if err != nil {
			backupProblem(w, err)
			return
		}
		err = s.Jobs.Run(job.ID, func(ctx context.Context, _ string) error {
			err := s.Engine.Recover(ctx, id, dir)
			saveErr := s.recoveryResult(id, dir, err)
			return errors.Join(err, saveErr)
		})
		if err != nil {
			_ = s.Jobs.Abort(job.ID, "start_failed")
			backupProblem(w, err)
			return
		}
		backupJSON(w, 202, job)
		return
	}
	if name == "abort" && r.Method == "POST" && record.Status == "queued" {
		if err := s.Jobs.Abort(id, "upload_cancelled"); err != nil {
			backupProblem(w, err)
			return
		}
		w.WriteHeader(204)
		return
	}
	if name == "preview" && r.Method == "POST" && record.Status == "ready" {
		if s.Jobs.Busy() {
			backupProblem(w, backup.ErrBusy)
			return
		}
		i, err := s.Engine.Inventory(r.Context())
		if err != nil {
			backupProblem(w, err)
			return
		}
		if err := s.Jobs.Update(id, func(r *backup.Record) { r.TargetRevision = i.Revision }); err != nil {
			backupProblem(w, err)
			return
		}
		record, _ = s.Jobs.Get(id)
		backupJSON(w, 200, record)
		return
	}
	if name == "inspect" && r.Method == "POST" && record.Action == "import" && record.Status == "queued" {
		err := s.Jobs.Run(id, func(ctx context.Context, id string) error {
			for _, module := range record.Modules {
				destination := filepath.Join(dir, "check-"+module)
				p, err := s.Engine.ReadPayload(ctx, filepath.Join(dir, module+".payload"), destination)
				cleanupErr := os.RemoveAll(destination)
				if err != nil {
					return err
				}
				if cleanupErr != nil {
					return cleanupErr
				}
				if p.Module != module {
					return backup.ErrInvalid
				}
			}
			i, err := s.Engine.Inventory(ctx)
			if err != nil {
				return err
			}
			return s.Jobs.Update(id, func(r *backup.Record) { r.TargetRevision = i.Revision })
		})
		if err != nil {
			backupProblem(w, err)
			return
		}
		backupJSON(w, 202, record)
		return
	}
	if !slices.Contains(record.Modules, name) || !slices.Contains([]string{"apps", "web", "docker"}, name) {
		backupProblem(w, backup.ErrInvalid)
		return
	}
	filename := filepath.Join(dir, name+".payload")
	if r.Method == "GET" && record.Action == "export" && record.Status == "completed" {
		f, err := backup.OpenRegular(filename, backup.MaxBytes)
		if err != nil {
			backupProblem(w, err)
			return
		}
		defer f.Close()
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Hour))
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, name+".payload", time.Time{}, f)
		return
	}
	if r.Method == "PUT" && record.Action == "import" && record.Status == "queued" {
		if r.ContentLength > backup.MaxBytes {
			backupProblem(w, backup.ErrInvalid)
			return
		}
		var used int64
		for _, module := range record.Modules {
			if info, err := os.Lstat(filepath.Join(dir, module+".payload")); err == nil {
				used += info.Size()
			}
		}
		if used >= backup.MaxBytes {
			backupProblem(w, backup.ErrInvalid)
			return
		}
		knownLength := r.ContentLength
		if knownLength < 0 {
			knownLength = 1 << 20
		}
		if err := backup.RequireSpace(dir, knownLength); err != nil {
			backupProblem(w, err)
			return
		}
		_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(2 * time.Hour))
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Hour))
		_, err := backup.CopyFile(filename, r.Body, backup.MaxBytes-used)
		if err != nil {
			_ = s.Jobs.Abort(id, "upload_failed")
			backupProblem(w, err)
			return
		}
		w.WriteHeader(204)
		return
	}
	backupProblem(w, backup.ErrInvalid)
}

func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16385))
	var input Request
	if err != nil || len(body) > 16384 || backup.Decode(body, &input) != nil {
		backupProblem(w, backup.ErrInvalid)
		return
	}
	modules, err := backup.Selection(input.Modules)
	if err != nil || slices.Contains(modules, "panel") {
		backupProblem(w, backup.ErrInvalid)
		return
	}
	if input.Action != "export" && input.Action != "import" && input.Action != "restore" {
		backupProblem(w, backup.ErrInvalid)
		return
	}
	var source backup.Record
	if input.Action == "restore" {
		source, err = s.Jobs.Get(input.SourceID)
		if err != nil || source.Status != "ready" || source.Action != "import" || input.Revision == "" || source.TargetRevision != input.Revision {
			backupProblem(w, backup.ErrInvalid)
			return
		}
		for _, module := range modules {
			if !slices.Contains(source.Modules, module) {
				backupProblem(w, backup.ErrInvalid)
				return
			}
		}
	}
	for _, r := range s.Jobs.List() {
		if recoveryPending(filepath.Join(s.Jobs.Root, r.ID)) {
			backupProblem(w, errors.New("previous backup needs recovery"))
			return
		}
	}
	var record backup.Record
	if input.Action == "restore" {
		record, source, err = s.Jobs.ReserveFrom(input.SourceID, modules)
	} else {
		record, err = s.Jobs.Reserve(input.Action, modules)
	}
	if err != nil {
		backupProblem(w, err)
		return
	}
	if input.Action == "import" {
		backupJSON(w, 202, record)
		return
	}
	err = s.Jobs.Update(record.ID, func(r *backup.Record) { r.SourceID = input.SourceID; r.TargetRevision = input.Revision })
	if err == nil {
		err = s.Jobs.Run(record.ID, func(ctx context.Context, id string) error {
			dir := filepath.Join(s.Jobs.Root, id)
			if input.Action == "export" {
				return s.Engine.Create(ctx, dir, modules, input.Revision)
			}
			i, err := s.Engine.Inventory(ctx)
			if err != nil {
				return err
			}
			if i.Revision != input.Revision {
				return errors.New("host configuration changed after preview")
			}
			for _, module := range modules {
				original := filepath.Join(s.Jobs.Root, source.ID, module+".payload")
				if err := os.Link(original, filepath.Join(dir, module+".payload")); err != nil {
					return err
				}
			}
			err = s.Engine.Restore(ctx, id, dir, modules)
			if err == nil || backup.FailureCode(err) == "cleanup_pending" {
				if saveErr := s.Jobs.Update(id, func(r *backup.Record) { r.CompletedModules = modules }); saveErr != nil {
					return errors.Join(err, saveErr)
				}
			}
			return err
		})
	}
	if err != nil {
		_ = s.Jobs.Abort(record.ID, "start_failed")
		backupProblem(w, err)
		return
	}
	backupJSON(w, 202, record)
}

func (s *Service) recoveryResult(id, directory string, recoveryErr error) error {
	status, code := "failed", "interrupted_rolled_back"
	if recoveryErr != nil {
		code = "recovery_required"
	} else if data, err := backup.ReadFile(filepath.Join(directory, "restore-journal.json"), 4<<20); err == nil {
		var journal restoreJournal
		if backup.Decode(data, &journal) == nil && journal.Phase == "completed" {
			status = "completed"
			code = ""
		}
	}
	return s.Jobs.Update(id, func(r *backup.Record) {
		r.Status = status
		r.ErrorCode = code
		r.Stage = code
		if status == "completed" {
			r.Stage = "completed"
			r.CompletedModules = r.Modules
		}
	})
}
