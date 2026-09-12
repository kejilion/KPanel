package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

var ErrBusy = errors.New("another backup operation is active")
var ErrNotFound = errors.New("backup record not found")

type Record struct {
	CompletedModules []string  `json:"completedModules,omitempty"`
	ID               string    `json:"id"`
	Action           string    `json:"action"`
	Status           string    `json:"status"`
	Stage            string    `json:"stage"`
	Modules          []string  `json:"modules"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	Size             int64     `json:"size"`
	Manifest         *Manifest `json:"manifest,omitempty"`
	ErrorCode        string    `json:"errorCode,omitempty"`
	TargetRevision   string    `json:"targetRevision,omitempty"`
	SourceID         string    `json:"sourceId,omitempty"`
	AgentID          string    `json:"agentId,omitempty"`
	AgentRevision    string    `json:"agentRevision,omitempty"`
}

// Manager persists intent before executing. Recovery never replays host writes.
// A failed terminal write keeps the manager closed to subsequent mutations.
type Manager struct {
	Root            string
	mu              sync.Mutex
	records         map[string]Record
	active          string
	storageFailed   bool
	closing         bool
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	maintenanceStop chan struct{}
	closeOnce       sync.Once
}

func OpenManager(root string) (*Manager, error) {
	if err := PrivateDir(root); err != nil {
		return nil, err
	}
	m := &Manager{Root: root, records: map[string]Record{}, maintenanceStop: make(chan struct{})}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !ValidID(entry.Name()) {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return nil, ErrInvalid
		}
		body, err := ReadFile(filepath.Join(root, entry.Name(), "record.json"), 32<<10)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || len(body) > 32<<10 {
			return nil, ErrInvalid
		}
		var record Record
		if Decode(body, &record) != nil || record.ID != entry.Name() {
			return nil, ErrInvalid
		}
		if len(m.records) >= 50 {
			return nil, errors.New("backup record limit exceeded")
		}
		if record.Status == "queued" || record.Status == "running" {
			record.Status = "failed"
			record.ErrorCode = "interrupted"
			record.Stage = "interrupted"
			record.UpdatedAt = time.Now().UTC()
			if err := WriteJSON(filepath.Join(root, entry.Name(), "record.json"), record); err != nil {
				return nil, err
			}
		}
		m.records[record.ID] = record
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-m.maintenanceStop:
				return
			case now := <-ticker.C:
				if err := m.ExpireQueued(now); err != nil {
					continue
				}
				_ = m.Sweep(now)
			}
		}
	}()
	return m, nil
}

func (m *Manager) Path(id, name string) (string, error) {
	if !ValidID(id) || filepath.Base(name) != name || name == "." || name == ".." || name == "" {
		return "", ErrInvalid
	}
	return filepath.Join(m.Root, id, name), nil
}
func (m *Manager) List() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Record, 0, len(m.records))
	for _, r := range m.records {
		items = append(items, r)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}
func (m *Manager) Get(id string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return r, ErrNotFound
	}
	return r, nil
}

func (m *Manager) saveLocked(record Record) error {
	record.UpdatedAt = time.Now().UTC()
	if err := WriteJSON(filepath.Join(m.Root, record.ID, "record.json"), record); err != nil {
		m.storageFailed = true
		return err
	}
	m.records[record.ID] = record
	return nil
}

func (m *Manager) Reserve(action string, modules []string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reserveLocked(action, modules)
}

func (m *Manager) ReserveFrom(sourceID string, modules []string) (Record, Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	source, ok := m.records[sourceID]
	if !ok || source.Action != "import" || source.Status != "ready" {
		return Record{}, source, ErrInvalid
	}
	record, err := m.reserveLocked("restore", modules)
	return record, source, err
}

func (m *Manager) reserveLocked(action string, modules []string) (Record, error) {
	if m.closing || m.active != "" || m.storageFailed {
		return Record{}, ErrBusy
	}
	if len(m.records) >= 50 {
		return Record{}, errors.New("delete an old backup before creating another")
	}
	r := Record{ID: NewID(), Action: action, Status: "queued", Stage: "queued", Modules: modules, CreatedAt: time.Now().UTC()}
	if err := PrivateDir(filepath.Join(m.Root, r.ID)); err != nil {
		return r, err
	}
	if err := m.saveLocked(r); err != nil {
		return r, err
	}
	m.active = r.ID
	return m.records[r.ID], nil
}

func (m *Manager) Update(id string, fn func(*Record)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return ErrNotFound
	}
	fn(&r)
	return m.saveLocked(r)
}

func (m *Manager) Abort(id, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return ErrNotFound
	}
	r.Status = "failed"
	r.Stage = "failed"
	r.ErrorCode = code
	if err := m.saveLocked(r); err != nil {
		return err
	}
	if m.active == id {
		m.active = ""
	}
	return nil
}

func (m *Manager) Run(id string, worker func(context.Context, string) error) error {
	m.mu.Lock()
	if m.active != id || m.closing {
		m.mu.Unlock()
		return ErrBusy
	}
	r := m.records[id]
	if r.Status != "queued" {
		m.mu.Unlock()
		return ErrBusy
	}
	r.Status = "running"
	r.Stage = "preparing"
	if err := m.saveLocked(r); err != nil {
		m.mu.Unlock()
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	m.cancel = cancel
	m.wg.Add(1)
	m.mu.Unlock()
	go func() {
		defer m.wg.Done()
		defer cancel()
		err := worker(ctx, id)
		m.mu.Lock()
		defer m.mu.Unlock()
		r := m.records[id]
		if err != nil {
			r.Status = "failed"
			r.Stage = "failed"
			r.ErrorCode = FailureCode(err)
			if len(r.CompletedModules) > 0 && r.ErrorCode != "cleanup_pending" && r.ErrorCode != "recovery_required" {
				r.ErrorCode = "partially_restored"
			}
			if ctx.Err() != nil && len(r.CompletedModules) == 0 {
				r.ErrorCode = "interrupted"
			}
		}
		if err == nil && r.Status != "restarting" {
			r.Status = "completed"
			r.Stage = "completed"
			if r.Action == "import" {
				r.Status = "ready"
				r.Stage = "ready"
			}
		}
		if saveErr := m.saveLocked(r); saveErr != nil {
			r.Status = "failed"
			r.ErrorCode = "persistence_pending"
			m.records[id] = r
			return
		}
		if r.Status != "restarting" {
			m.active = ""
		}
		m.cancel = nil
	}()
	return nil
}

// ExpireQueued releases abandoned uploads. Running workers own their own
// cancellation deadline and are never expired while they may still write.
func (m *Manager) ExpireQueued(now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == "" {
		return nil
	}
	r := m.records[m.active]
	if r.Status != "queued" || now.Sub(r.UpdatedAt) < 2*time.Hour {
		return nil
	}
	r.Status = "failed"
	r.Stage = "expired"
	r.ErrorCode = "upload_expired"
	if err := m.saveLocked(r); err != nil {
		return err
	}
	m.active = ""
	return nil
}

func (m *Manager) Delete(id string) error { return m.DeleteWith(id, nil) }

// Hold the same reservation lock through remote cleanup and local deletion.
func (m *Manager) DeleteWith(id string, before func(Record) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != "" || m.storageFailed {
		return ErrBusy
	}
	if _, ok := m.records[id]; !ok {
		return ErrNotFound
	}
	if !ValidID(id) {
		return ErrInvalid
	}
	path := filepath.Join(m.Root, id)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrInvalid
	}
	if before != nil {
		if err := before(m.records[id]); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	delete(m.records, id)
	return SyncDir(m.Root)
}

func (m *Manager) Close() {
	m.closeOnce.Do(func() { close(m.maintenanceStop) })
	m.mu.Lock()
	m.closing = true
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	m.wg.Wait()
}
func (m *Manager) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active != "" || m.storageFailed
}

// Sweep bounds temporary plaintext lifetime while retaining small task records.
// Recovery journals are retained until their owning adapter resolves them.
func (m *Manager) Sweep(now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active != "" || m.storageFailed {
		return nil
	}
	for id, r := range m.records {
		age := now.Sub(r.CreatedAt)
		ttl := 24 * time.Hour
		if r.Action == "export" {
			ttl = 7 * 24 * time.Hour
		}
		if age < ttl || r.Status == "restarting" {
			continue
		}
		dir := filepath.Join(m.Root, id)
		hasJournal := false
		for _, name := range []string{"restore-journal.json", "export-journal.json", "panel-restore.payload"} {
			if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
				if name == "restore-journal.json" {
					var j struct{ Phase string }
					body, readErr := ReadFile(filepath.Join(dir, name), 4<<20)
					if readErr == nil && json.Unmarshal(body, &j) == nil && (j.Phase == "completed" || j.Phase == "rolled_back") {
						continue
					}
				}
				hasJournal = true
			}
		}
		if hasJournal {
			continue
		}
		for _, name := range []string{"upload.kpb", "backup.kpb", "panel.payload", "apps.payload", "web.payload", "docker.payload", "container.tmp"} {
			path := filepath.Join(dir, name)
			if err := NoLinkParents(path); err != nil {
				return err
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		for _, name := range []string{"unpack-apps", "unpack-web", "unpack-docker", "check-apps", "check-web", "check-docker"} {
			path := filepath.Join(dir, name)
			if err := NoLinkParents(path); err != nil {
				return err
			}
			if err := os.RemoveAll(path); err != nil {
				return err
			}
		}
		if age > 30*24*time.Hour {
			if err := NoLinkParents(dir); err != nil {
				return err
			}
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
			delete(m.records, id)
			continue
		}
		if r.Status == "ready" || r.Action == "export" && r.Status == "completed" {
			r.Status = "expired"
			r.Stage = "expired"
			if err := m.saveLocked(r); err != nil {
				return err
			}
		}
	}
	return nil
}
