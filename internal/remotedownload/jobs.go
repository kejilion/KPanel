package remotedownload

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	"unicode"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	jobSchemaVersion = 1
	MaxJobs          = 100
	MaxQueuedJobs    = 10
	MaxJobStateBytes = 256 << 10
	JobRetention     = 7 * 24 * time.Hour
)

var (
	ErrJobStoreUnavailable = errors.New("remote download job store is unavailable")
	ErrJobNotFound         = errors.New("remote download job not found")
	ErrJobActive           = errors.New("remote download job is active")
	jobIDPattern           = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

type persistedJobs struct {
	SchemaVersion int                              `json:"schemaVersion"`
	Jobs          []contract.FileRemoteDownloadJob `json:"jobs"`
}

type JobStore struct {
	mu          sync.RWMutex
	root        string
	path        string
	jobs        map[string]contract.FileRemoteDownloadJob
	available   bool
	now         func() time.Time
	writeAtomic func(string, string, []byte) error
}

func OpenJobStore(root string) (*JobStore, error) {
	return openJobStore(root, readPersistedJobs, writeAtomicPrivateFile)
}

// Keep the legacy journal intact for rollback. Migrate verified download
// history only when the new common transfer journal does not exist yet.
func OpenTransferJobStore(root, legacyRoot string) (*JobStore, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	legacyRoot = filepath.Clean(strings.TrimSpace(legacyRoot))
	if !filepath.IsAbs(root) || !filepath.IsAbs(legacyRoot) {
		return nil, errors.New("file transfer job roots must be absolute")
	}
	newPath := filepath.Join(root, "jobs.json")
	if _, err := os.Lstat(newPath); !errors.Is(err, os.ErrNotExist) {
		return OpenJobStore(root)
	}
	legacy, err := readPersistedJobs(filepath.Join(legacyRoot, "jobs.json"))
	if errors.Is(err, os.ErrNotExist) {
		return OpenJobStore(root)
	}
	if err != nil {
		return &JobStore{root: root, path: newPath, jobs: make(map[string]contract.FileRemoteDownloadJob)}, nil
	}
	if err := ensurePrivateDirectory(root); err != nil {
		return &JobStore{root: root, path: newPath, jobs: make(map[string]contract.FileRemoteDownloadJob)}, nil
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		return nil, err
	}
	if err := writeAtomicPrivateFile(root, newPath, payload); err != nil {
		return &JobStore{root: root, path: newPath, jobs: make(map[string]contract.FileRemoteDownloadJob)}, nil
	}
	return OpenJobStore(root)
}

func openJobStore(
	root string,
	readState func(string) (persistedJobs, error),
	writeAtomic func(string, string, []byte) error,
) (*JobStore, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || !filepath.IsAbs(root) {
		return nil, errors.New("remote download job root must be absolute")
	}
	store := &JobStore{
		root: root, path: filepath.Join(root, "jobs.json"), jobs: make(map[string]contract.FileRemoteDownloadJob),
		now: time.Now, writeAtomic: writeAtomic,
	}
	// The job index is an optional runtime facility. Filesystem and state
	// failures disable background downloads without taking down the Panel or
	// replacing the abnormal path; only an invalid caller-supplied root above is
	// a programming/configuration error.
	if err := ensurePrivateDirectory(root); err != nil {
		return store, nil
	}
	state, err := readState(store.path)
	switch {
	case err == nil:
		if err := os.Chmod(store.path, 0o600); err != nil {
			return store, nil
		}
		for _, job := range state.Jobs {
			store.jobs[job.ID] = cloneJob(job)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persistLocked(); err != nil {
			return store, nil
		}
	default:
		return store, nil
	}
	store.available = true
	changed := store.interruptActiveLocked(store.now().UTC())
	changed = store.pruneLocked(store.now().UTC()) || changed
	if changed {
		if err := store.persistLocked(); err != nil {
			store.available = false
		}
	}
	return store, nil
}

func (s *JobStore) Available() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.available
}

func (s *JobStore) Create(job contract.FileRemoteDownloadJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return ErrJobStoreUnavailable
	}
	if err := validateJob(job); err != nil {
		return err
	}
	if _, exists := s.jobs[job.ID]; exists {
		return errors.New("remote download job already exists")
	}
	previous := cloneJobs(s.jobs)
	s.pruneLocked(s.now().UTC())
	if len(s.jobs) >= MaxJobs {
		if !s.removeOldestTerminalLocked() {
			s.jobs = previous
			return errors.New("remote download job history limit reached")
		}
	}
	s.jobs[job.ID] = cloneJob(job)
	if err := s.persistLocked(); err != nil {
		s.jobs = previous
		return err
	}
	return nil
}

func (s *JobStore) Update(job contract.FileRemoteDownloadJob, persist bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return ErrJobStoreUnavailable
	}
	if err := validateJob(job); err != nil {
		return err
	}
	if _, exists := s.jobs[job.ID]; !exists {
		return ErrJobNotFound
	}
	s.jobs[job.ID] = cloneJob(job)
	if !persist {
		return nil
	}
	if err := s.persistLocked(); err != nil {
		return err
	}
	return nil
}

func (s *JobStore) Get(id string) (contract.FileRemoteDownloadJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.available {
		return contract.FileRemoteDownloadJob{}, ErrJobStoreUnavailable
	}
	job, exists := s.jobs[id]
	if !jobIDPattern.MatchString(id) || !exists {
		return contract.FileRemoteDownloadJob{}, ErrJobNotFound
	}
	return cloneJob(job), nil
}

func (s *JobStore) removeOldestTerminalLocked() bool {
	var oldest contract.FileRemoteDownloadJob
	found := false
	for _, job := range s.jobs {
		if activeJobState(job.State) || found && !job.CreatedAt.Before(oldest.CreatedAt) {
			continue
		}
		oldest = job
		found = true
	}
	if !found {
		return false
	}
	delete(s.jobs, oldest.ID)
	return true
}

func (s *JobStore) List() ([]contract.FileRemoteDownloadJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return nil, ErrJobStoreUnavailable
	}
	previous := cloneJobs(s.jobs)
	if s.pruneLocked(s.now().UTC()) {
		if err := s.persistLocked(); err != nil {
			s.jobs = previous
			return nil, err
		}
	}
	result := make([]contract.FileRemoteDownloadJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		result = append(result, cloneJob(job))
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].CreatedAt.After(result[right].CreatedAt)
	})
	return result, nil
}

func (s *JobStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return ErrJobStoreUnavailable
	}
	job, exists := s.jobs[id]
	if !jobIDPattern.MatchString(id) || !exists {
		return ErrJobNotFound
	}
	if activeJobState(job.State) {
		return ErrJobActive
	}
	delete(s.jobs, id)
	if err := s.persistLocked(); err != nil {
		s.jobs[id] = job
		return err
	}
	return nil
}

func (s *JobStore) interruptActiveLocked(now time.Time) bool {
	changed := false
	for id, job := range s.jobs {
		if !activeJobState(job.State) {
			continue
		}
		job.State = "interrupted"
		job.Code = "remote_download_interrupted"
		job.UpdatedAt = now
		job.FinishedAt = timePointer(now)
		s.jobs[id] = job
		changed = true
	}
	return changed
}

func (s *JobStore) pruneLocked(now time.Time) bool {
	changed := false
	for id, job := range s.jobs {
		if activeJobState(job.State) || job.FinishedAt == nil || now.Sub(*job.FinishedAt) <= JobRetention {
			continue
		}
		delete(s.jobs, id)
		changed = true
	}
	if len(s.jobs) <= MaxJobs {
		return changed
	}
	terminal := make([]contract.FileRemoteDownloadJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		if !activeJobState(job.State) {
			terminal = append(terminal, job)
		}
	}
	sort.Slice(terminal, func(left, right int) bool {
		return terminal[left].CreatedAt.Before(terminal[right].CreatedAt)
	})
	remove := len(s.jobs) - MaxJobs
	if remove > len(terminal) {
		remove = len(terminal)
	}
	for _, job := range terminal[:remove] {
		delete(s.jobs, job.ID)
		changed = true
	}
	return changed
}

func (s *JobStore) persistLocked() error {
	jobs := make([]contract.FileRemoteDownloadJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, cloneJob(job))
	}
	sort.Slice(jobs, func(left, right int) bool { return jobs[left].ID < jobs[right].ID })
	data, err := json.MarshalIndent(persistedJobs{SchemaVersion: jobSchemaVersion, Jobs: jobs}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode remote download jobs: %w", err)
	}
	data = append(data, '\n')
	if len(data) > MaxJobStateBytes {
		return errors.New("remote download job state exceeds 256 KiB")
	}
	if err := s.writeAtomic(s.root, s.path, data); err != nil {
		return fmt.Errorf("persist remote download jobs: %w", err)
	}
	return nil
}

func readPersistedJobs(statePath string) (persistedJobs, error) {
	info, err := os.Lstat(statePath)
	if err != nil {
		return persistedJobs{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > MaxJobStateBytes {
		return persistedJobs{}, ErrJobStoreUnavailable
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		return persistedJobs{}, err
	}
	var state persistedJobs
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return persistedJobs{}, ErrJobStoreUnavailable
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return persistedJobs{}, ErrJobStoreUnavailable
	}
	if state.SchemaVersion != jobSchemaVersion || len(state.Jobs) > MaxJobs {
		return persistedJobs{}, ErrJobStoreUnavailable
	}
	seen := make(map[string]bool, len(state.Jobs))
	for _, job := range state.Jobs {
		if seen[job.ID] || validateJob(job) != nil {
			return persistedJobs{}, ErrJobStoreUnavailable
		}
		seen[job.ID] = true
	}
	return state, nil
}

func validateJob(job contract.FileRemoteDownloadJob) error {
	validSourceKind := job.SourceKind == "" && validSource(job.Source) && job.TargetHostID == "" ||
		job.SourceKind == "cross-host" && strings.HasPrefix(job.Source, "kpanel://") && jobIDPattern.MatchString(strings.TrimPrefix(job.Source, "kpanel://")) &&
			(job.TargetHostID == "" || jobIDPattern.MatchString(job.TargetHostID))
	if !jobIDPattern.MatchString(job.ID) || !validJobState(job.State) || job.CreatedAt.IsZero() ||
		job.UpdatedAt.Before(job.CreatedAt) || job.LoadedBytes < 0 || job.TotalBytes < 0 ||
		job.LoadedBytes > contract.MaxFileTransferBytes+(32<<20) || job.TotalBytes > contract.MaxFileTransferBytes || !validSourceKind ||
		!validTargetDirectory(job.TargetDirectory) || (job.Name != "" && !validJobName(job.Name)) {
		return errors.New("invalid remote download job")
	}
	terminal := !activeJobState(job.State)
	if terminal != (job.FinishedAt != nil) || job.FinishedAt != nil && job.FinishedAt.Before(job.CreatedAt) {
		return errors.New("invalid remote download job timestamps")
	}
	if job.State == "complete" {
		if job.Code != "" || job.Name == "" || job.Entry == nil || (job.Entry.Kind != "file" && !(job.SourceKind == "cross-host" && job.Entry.Kind == "directory")) ||
			job.Entry.Name != job.Name || job.Entry.Path != path.Join(job.TargetDirectory, job.Name) ||
			(job.Entry.Kind == "file" && job.Entry.SizeBytes != job.LoadedBytes) || job.Entry.ResourceVersion == "" || len(job.Entry.ResourceVersion) > 256 {
			return errors.New("invalid completed remote download job")
		}
	} else if job.Entry != nil {
		return errors.New("invalid remote download job result")
	}
	return nil
}

func validJobState(state string) bool {
	switch state {
	case "queued", "connecting", "transferring", "confirming", "complete", "cancelled", "error", "interrupted":
		return true
	default:
		return false
	}
}

func activeJobState(state string) bool {
	return state == "queued" || state == "connecting" || state == "transferring" || state == "confirming"
}

func validSource(value string) bool {
	parsed, err := ValidateURL(value)
	return err == nil && parsed.Path == "" && parsed.RawPath == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func validTargetDirectory(value string) bool {
	return value != "" && value[0] == '/' && len(value) <= 4096 && path.Clean(value) == value &&
		!strings.Contains(value, `\`) && !hasControl(value)
}

func validJobName(value string) bool {
	if value == "" || value == "." || value == ".." || strings.TrimSpace(value) != value ||
		len(value) > 255 || !utf8.ValidString(value) || strings.ContainsAny(value, `/\`) || strings.HasPrefix(value, ".kpanel-") {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func cloneJob(job contract.FileRemoteDownloadJob) contract.FileRemoteDownloadJob {
	result := job
	if job.Entry != nil {
		entry := *job.Entry
		result.Entry = &entry
	}
	if job.FinishedAt != nil {
		finished := *job.FinishedAt
		result.FinishedAt = &finished
	}
	return result
}

func cloneJobs(source map[string]contract.FileRemoteDownloadJob) map[string]contract.FileRemoteDownloadJob {
	result := make(map[string]contract.FileRemoteDownloadJob, len(source))
	for id, job := range source {
		result[id] = cloneJob(job)
	}
	return result
}

func timePointer(value time.Time) *time.Time { return &value }

func ensurePrivateDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("remote download job root must be a non-symlink directory")
	}
	return os.Chmod(directory, 0o700)
}

func writeAtomicPrivateFile(directory, target string, data []byte) error {
	file, err := os.CreateTemp(directory, ".remote-download-jobs-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(target)
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		directoryFile, err := os.Open(directory)
		if err != nil {
			return err
		}
		defer directoryFile.Close()
		return directoryFile.Sync()
	}
	return nil
}
