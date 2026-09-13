// Package filetransfer owns bounded, durable cross-host copy jobs. File contents
// and credentials remain with the existing file/cluster services.
package filetransfer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	MaxItems      = 64
	MaxJobs       = 32
	MaxPending    = 8
	maxStateBytes = 8 << 20
	retention     = 7 * 24 * time.Hour
)

var (
	ErrUnavailable = errors.New("file transfer jobs unavailable")
	ErrFull        = errors.New("file transfer queue full")
	ErrNotFound    = errors.New("file transfer job not found")
	ErrActive      = errors.New("file transfer job is active")
	ErrInvalid     = errors.New("invalid file transfer job")
	idPattern      = regexp.MustCompile(`^[a-f0-9]{32}$`)
	errClosing     = errors.New("file transfer server closing")
)

type Source struct {
	Path            string `json:"path"`
	ResourceVersion string `json:"resourceVersion"`
}

type Request struct {
	ID              string   `json:"id"`
	RetryOf         string   `json:"retryOf,omitempty"`
	SourceNodeID    string   `json:"sourceNodeId"`
	TargetHostID    string   `json:"targetHostId"`
	TargetDirectory string   `json:"targetDirectory"`
	Items           []Source `json:"items"`
}

type Item struct {
	Source
	State       string              `json:"state"`
	LoadedBytes int64               `json:"loadedBytes"`
	TotalBytes  int64               `json:"totalBytes"`
	Entry       *contract.FileEntry `json:"entry,omitempty"`
	Detail      string              `json:"detail,omitempty"`
	Retryable   bool                `json:"retryable"`
}

type Job struct {
	ID              string    `json:"id"`
	RetryOf         string    `json:"retryOf,omitempty"`
	SourceNodeID    string    `json:"sourceNodeId"`
	TargetHostID    string    `json:"targetHostId"`
	TargetDirectory string    `json:"targetDirectory"`
	State           string    `json:"state"`
	Items           []Item    `json:"items"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type Execute func(context.Context, Source, func(contract.FileTransferEvent))

type Manager struct {
	mu        sync.Mutex
	root      string
	jobs      map[string]Job
	cancels   map[string]context.CancelCauseFunc
	gate      chan struct{}
	wg        sync.WaitGroup
	closed    bool
	available bool
	write     func([]byte) error
}

func Open(root string) *Manager {
	m := &Manager{root: root, jobs: map[string]Job{}, cancels: map[string]context.CancelCauseFunc{}, gate: make(chan struct{}, 2)}
	m.write = m.writeAtomic
	if !filepath.IsAbs(root) {
		return m
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return m
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || os.Chmod(root, 0700) != nil {
		return m
	}
	filename := filepath.Join(root, "jobs.json")
	if info, err = os.Lstat(filename); err == nil {
		if !info.Mode().IsRegular() || info.Size() > maxStateBytes || os.Chmod(filename, 0600) != nil {
			return m
		}
		f, err := os.Open(filename)
		if err != nil {
			return m
		}
		data, err := io.ReadAll(io.LimitReader(f, maxStateBytes+1))
		f.Close()
		if err != nil || len(data) > maxStateBytes {
			return m
		}
		var state struct {
			Version int   `json:"version"`
			Jobs    []Job `json:"jobs"`
		}
		d := json.NewDecoder(bytes.NewReader(data))
		d.DisallowUnknownFields()
		var extra any
		if d.Decode(&state) != nil || d.Decode(&extra) != io.EOF || state.Version != 1 || len(state.Jobs) > MaxJobs {
			return m
		}
		for _, job := range state.Jobs {
			if _, exists := m.jobs[job.ID]; exists || validateJob(job) != nil {
				return m
			}
			if Active(job.State) {
				job.State = "interrupted"
				job.UpdatedAt = time.Now().UTC()
				for i := range job.Items {
					if Active(job.Items[i].State) {
						job.Items[i].Retryable = job.Items[i].State == "queued"
						job.Items[i].State = "interrupted"
						job.Items[i].Detail = "面板已重启；请检查目标目录后决定是否重新复制。"
					}
				}
			}
			m.jobs[job.ID] = job
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return m
	}
	m.prune()
	m.available = m.persist() == nil
	return m
}

func validPath(p string) bool {
	return len(p) > 0 && len(p) <= 4096 && strings.HasPrefix(p, "/") && path.Clean(p) == p && !strings.ContainsAny(p, "\\\x00\r\n")
}

func Validate(r Request) error {
	if r.RetryOf != "" && (!idPattern.MatchString(r.RetryOf) || r.RetryOf == r.ID) {
		return ErrInvalid
	}
	if !idPattern.MatchString(r.ID) || !idPattern.MatchString(r.SourceNodeID) || (r.TargetHostID != "" && !idPattern.MatchString(r.TargetHostID)) || !validPath(r.TargetDirectory) || len(r.Items) == 0 || len(r.Items) > MaxItems {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, item := range r.Items {
		if !validPath(item.Path) || item.Path == "/" || item.ResourceVersion == "" || len(item.ResourceVersion) > 256 || seen[item.Path] {
			return ErrInvalid
		}
		seen[item.Path] = true
	}
	return nil
}

func Active(state string) bool {
	return state == "queued" || state == "running" || state == "connecting" || state == "transferring" || state == "committing"
}

func validateJob(j Job) error {
	r := Request{ID: j.ID, RetryOf: j.RetryOf, SourceNodeID: j.SourceNodeID, TargetHostID: j.TargetHostID, TargetDirectory: j.TargetDirectory}
	for _, item := range j.Items {
		r.Items = append(r.Items, item.Source)
		if !validState(item.State) || item.LoadedBytes < 0 || item.LoadedBytes > contract.MaxFileTransferArchiveBytes || item.TotalBytes < 0 || item.TotalBytes > contract.MaxFileTransferBytes || len(item.Detail) > 4096 {
			return ErrInvalid
		}
		if item.State == "complete" && (item.Entry == nil || item.Entry.ResourceVersion == "" || path.Dir(item.Entry.Path) != j.TargetDirectory) {
			return ErrInvalid
		}
	}
	if !validState(j.State) || j.CreatedAt.IsZero() || j.UpdatedAt.Before(j.CreatedAt) {
		return ErrInvalid
	}
	return Validate(r)
}

func validState(s string) bool {
	return Active(s) || s == "complete" || s == "partial" || s == "error" || s == "cancelled" || s == "interrupted"
}

func clone(j Job) Job {
	j.Items = append([]Item(nil), j.Items...)
	for i := range j.Items {
		if j.Items[i].Entry != nil {
			e := *j.Items[i].Entry
			j.Items[i].Entry = &e
		}
	}
	return j
}

func (m *Manager) List() ([]Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.available {
		return nil, ErrUnavailable
	}
	items := make([]Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		if Active(job.State) || time.Since(job.UpdatedAt) <= retention {
			items = append(items, clone(job))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

// Start is idempotent for an exact request ID and payload. No already-started
// filesystem operation is automatically replayed, including after a restart.
func (m *Manager) Start(r Request, execute Execute) (Job, error) {
	if Validate(r) != nil {
		return Job{}, ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.available || m.closed {
		return Job{}, ErrUnavailable
	}
	if existing, ok := m.jobs[r.ID]; ok {
		if existing.RetryOf != r.RetryOf || existing.SourceNodeID != r.SourceNodeID || existing.TargetHostID != r.TargetHostID || existing.TargetDirectory != r.TargetDirectory || len(existing.Items) != len(r.Items) {
			return Job{}, ErrInvalid
		}
		for i := range r.Items {
			if existing.Items[i].Source != r.Items[i] {
				return Job{}, ErrInvalid
			}
		}
		return clone(existing), nil
	}
	// Consuming a parent's retryable items and creating its child is one durable
	// update. Reopening the page or retrying a lost response cannot copy twice.
	var parent Job
	if r.RetryOf != "" {
		for _, existing := range m.jobs {
			if existing.RetryOf != r.RetryOf {
				continue
			}
			if existing.SourceNodeID != r.SourceNodeID || existing.TargetHostID != r.TargetHostID || existing.TargetDirectory != r.TargetDirectory || len(existing.Items) != len(r.Items) {
				return Job{}, ErrInvalid
			}
			for i := range r.Items {
				if existing.Items[i].Source != r.Items[i] {
					return Job{}, ErrInvalid
				}
			}
			return clone(existing), nil
		}
		var ok bool
		parent, ok = m.jobs[r.RetryOf]
		if !ok || Active(parent.State) || parent.SourceNodeID != r.SourceNodeID || parent.TargetHostID != r.TargetHostID || parent.TargetDirectory != r.TargetDirectory {
			return Job{}, ErrInvalid
		}
		var sources []Source
		for _, item := range parent.Items {
			if item.Retryable && item.State != "complete" {
				sources = append(sources, item.Source)
			}
		}
		if len(sources) == 0 || len(sources) != len(r.Items) {
			return Job{}, ErrInvalid
		}
		for i := range sources {
			if sources[i] != r.Items[i] {
				return Job{}, ErrInvalid
			}
		}
	}
	if len(m.cancels) >= MaxPending {
		return Job{}, ErrFull
	}
	previous := make(map[string]Job, len(m.jobs))
	for id, j := range m.jobs {
		previous[id] = j
	}
	m.prune()
	if len(m.jobs) >= MaxJobs {
		var oldest *Job
		for _, j := range m.jobs {
			if j.ID != r.RetryOf && !Active(j.State) && (oldest == nil || j.CreatedAt.Before(oldest.CreatedAt)) {
				value := j
				oldest = &value
			}
		}
		if oldest == nil {
			return Job{}, ErrFull
		}
		delete(m.jobs, oldest.ID)
	}
	now := time.Now().UTC()
	j := Job{ID: r.ID, RetryOf: r.RetryOf, SourceNodeID: r.SourceNodeID, TargetHostID: r.TargetHostID, TargetDirectory: r.TargetDirectory, State: "queued", CreatedAt: now, UpdatedAt: now}
	for _, source := range r.Items {
		j.Items = append(j.Items, Item{Source: source, State: "queued", Retryable: true})
	}
	m.jobs[j.ID] = j
	if parent.ID != "" {
		parent = clone(parent)
		for i := range parent.Items {
			parent.Items[i].Retryable = false
		}
		m.jobs[parent.ID] = parent
	}
	if err := m.persist(); err != nil {
		m.jobs = previous
		return Job{}, ErrUnavailable
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	m.cancels[j.ID] = cancel
	m.wg.Add(1)
	go m.run(ctx, cancel, clone(j), execute)
	return clone(j), nil
}

func (m *Manager) run(ctx context.Context, cancel context.CancelCauseFunc, job Job, execute Execute) {
	defer m.wg.Done()
	defer cancel(context.Canceled)
	select {
	case m.gate <- struct{}{}:
		defer func() { <-m.gate }()
	case <-ctx.Done():
	}
	ctx, timeout := context.WithTimeout(ctx, 2*time.Hour)
	defer timeout()
	lastPersist := time.Now()
	for i := range job.Items {
		if ctx.Err() != nil {
			break
		}
		job.State = "running"
		job.Items[i].State = "connecting"
		job.Items[i].Retryable = false
		if !m.update(job, true) {
			cancel(ErrUnavailable)
			break
		}
		func() {
			defer func() {
				if recover() != nil {
					job.Items[i].State = "interrupted"
					job.Items[i].Detail = "传输意外中断，请检查目标目录。"
				}
			}()
			execute(ctx, job.Items[i].Source, func(event contract.FileTransferEvent) {
				item := &job.Items[i]
				item.State = event.State
				item.LoadedBytes = event.LoadedBytes
				item.TotalBytes = event.TotalBytes
				item.Entry = event.Entry
				item.Detail = event.Detail
				if len(item.Detail) > 4096 {
					item.Detail = "传输失败，请检查目标目录。"
				}
				item.Retryable = event.Code == "source_unavailable" || event.Code == "source_changed" || event.Code == "target_unavailable" || event.Code == "transfer_too_large"
				persist := !Active(event.State) || time.Since(lastPersist) >= 5*time.Second
				if !m.update(job, persist) {
					cancel(ErrUnavailable)
				}
				if persist {
					lastPersist = time.Now()
				}
			})
		}()
		if Active(job.Items[i].State) {
			job.Items[i].State = "interrupted"
			job.Items[i].Detail = "未收到完整结果，请检查目标目录。"
		}
		if !m.update(job, true) {
			cancel(ErrUnavailable)
			break
		}
	}
	completed := 0
	for i := range job.Items {
		if job.Items[i].State == "complete" {
			completed++
			continue
		}
		if Active(job.Items[i].State) {
			job.Items[i].Retryable = job.Items[i].State == "queued"
			job.Items[i].State = "cancelled"
			if errors.Is(context.Cause(ctx), errClosing) {
				job.Items[i].State = "interrupted"
			}
		}
	}
	switch {
	case completed == len(job.Items):
		job.State = "complete"
	case completed > 0:
		job.State = "partial"
	case errors.Is(context.Cause(ctx), errClosing):
		job.State = "interrupted"
	case ctx.Err() != nil:
		job.State = "cancelled"
	default:
		job.State = "error"
	}
	m.update(job, true)
}

func (m *Manager) update(j Job, persist bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	j.UpdatedAt = time.Now().UTC()
	if validateJob(j) != nil {
		return false
	}
	m.jobs[j.ID] = clone(j)
	if !Active(j.State) {
		delete(m.cancels, j.ID)
	}
	if persist && m.persist() != nil {
		m.available = false
		return false
	}
	return true
}

func (m *Manager) Change(id, operation string) error {
	if !idPattern.MatchString(id) {
		return ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.available {
		return ErrUnavailable
	}
	j, ok := m.jobs[id]
	if !ok {
		return ErrNotFound
	}
	if operation == "cancel" {
		if cancel := m.cancels[id]; cancel != nil {
			cancel(context.Canceled)
		}
		return nil
	}
	if operation != "clear" {
		return ErrInvalid
	}
	if Active(j.State) || m.cancels[id] != nil {
		return ErrActive
	}
	delete(m.jobs, id)
	if m.persist() != nil {
		m.jobs[id] = j
		return ErrUnavailable
	}
	return nil
}

func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	for _, cancel := range m.cancels {
		cancel(errClosing)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

func (m *Manager) prune() {
	for id, j := range m.jobs {
		if !Active(j.State) && time.Since(j.UpdatedAt) > retention {
			delete(m.jobs, id)
		}
	}
}

func (m *Manager) persist() error {
	jobs := make([]Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	data, err := json.Marshal(struct {
		Version int   `json:"version"`
		Jobs    []Job `json:"jobs"`
	}{1, jobs})
	if err != nil || len(data) > maxStateBytes {
		return ErrUnavailable
	}
	return m.write(data)
}

func (m *Manager) writeAtomic(data []byte) error {
	f, err := os.CreateTemp(m.root, ".jobs-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(m.root, "jobs.json")); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		dir, err := os.Open(m.root)
		if err != nil {
			return err
		}
		defer dir.Close()
		return dir.Sync()
	}
	return nil
}
