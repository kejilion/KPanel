package filemanager

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const maxArchiveJobs = 50
const maxActiveArchiveJobs = 4
const maxArchiveStateBytes = 2 << 20

var ErrArchiveJobsUnavailable = errors.New("归档任务状态不可用，请检查 Agent 状态目录")

type archiveJobRecord struct {
	Private        *receivePublication            `json:"private,omitempty"`
	Decision       *contract.FileConflictDecision `json:"decision,omitempty"`
	Job            contract.FileArchiveJob        `json:"job"`
	Temps          []string                       `json:"temps"`
	TempIdentities map[string]archiveTempIdentity `json:"tempIdentities,omitempty"`
}

type archiveTempIdentity struct {
	Parent    string `json:"parent"`
	Object    string `json:"object"`
	Directory string `json:"directory"`
	Workspace string `json:"workspace"`
}
type archiveJobs struct {
	mu          sync.Mutex
	initialized bool
	available   bool
	closed      bool
	records     map[string]*archiveJobRecord
	cancels     map[string]context.CancelFunc
	gate        chan struct{}
	store       *fileRoot
	wg          sync.WaitGroup
}

func archiveJobActive(state string) bool {
	return state == "queued" || state == "running" || state == "cancelling" || state == "waiting_input"
}
func (m *Manager) archiveStatePath() string {
	return path.Join(path.Dir(m.trashRoot), "archive-jobs-v2.json")
}

func (m *Manager) initializeArchiveJobsLocked() error {
	a := &m.archiveJobs
	if a.closed {
		return ErrArchiveJobsUnavailable
	}
	if a.initialized {
		if !a.available {
			return ErrArchiveJobsUnavailable
		}
		return nil
	}
	a.initialized = true
	a.records = make(map[string]*archiveJobRecord)
	a.cancels = make(map[string]context.CancelFunc)
	a.gate = make(chan struct{}, 1)
	// Pin the private state directory. Do not follow a replaced state file or
	// allow a symlink in its ancestry to redirect recovery/persistence.
	current := "/"
	for _, part := range strings.Split(strings.TrimPrefix(path.Dir(m.archiveStatePath()), "/"), "/") {
		current = joinVirtual(current, part)
		info, err := m.rootFS.Lstat(rootName(current))
		// A Windows volume root already exists, but attempting to create it can
		// return access denied rather than ErrExist. Only create missing parts.
		if errors.Is(err, os.ErrNotExist) {
			if err := m.rootFS.Mkdir(rootName(current), 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return ErrArchiveJobsUnavailable
			}
			info, err = m.rootFS.Lstat(rootName(current))
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrArchiveJobsUnavailable
		}
	}
	var err error
	a.store, err = m.rootFS.OpenRoot(rootName(path.Dir(m.archiveStatePath())))
	if err != nil {
		return ErrArchiveJobsUnavailable
	}
	stateName := "archive-jobs-v2.json"
	if _, err := a.store.Lstat(stateName); errors.Is(err, os.ErrNotExist) {
		if _, oldErr := a.store.Lstat("archive-jobs.json"); oldErr == nil {
			stateName = "archive-jobs.json"
		}
	}
	storedInfo, statErr := a.store.Lstat(stateName)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return ErrArchiveJobsUnavailable
	}
	if statErr == nil && !storedInfo.Mode().IsRegular() {
		return ErrArchiveJobsUnavailable
	}
	file, err := a.store.Open(stateName)
	if err == nil {
		defer file.Close()
		info, statErr := file.Stat()
		if statErr != nil || storedInfo == nil || !os.SameFile(storedInfo, info) || !info.Mode().IsRegular() || info.Size() > maxArchiveStateBytes {
			return ErrArchiveJobsUnavailable
		}
		payload, err := io.ReadAll(io.LimitReader(file, maxArchiveStateBytes+1))
		if err != nil || len(payload) > maxArchiveStateBytes {
			return ErrArchiveJobsUnavailable
		}
		var state struct {
			Version int                `json:"version"`
			Records []archiveJobRecord `json:"records"`
		}
		decoder := json.NewDecoder(bytes.NewReader(payload))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&state) != nil || state.Version != 1 || len(state.Records) > maxArchiveJobs {
			return ErrArchiveJobsUnavailable
		}
		var tail any
		if decoder.Decode(&tail) != io.EOF {
			return ErrArchiveJobsUnavailable
		}
		for _, record := range state.Records {
			j := record.Job
			id, idErr := hex.DecodeString(j.ID)
			target, pathErr := normalizeVirtual(j.Target)
			if idErr != nil || len(id) != 16 || pathErr != nil || target != j.Target || !storedFileJobAction(j.Action) || len(record.Temps) > MaxBatchItems || j.CreatedAt.IsZero() {
				return ErrArchiveJobsUnavailable
			}
			if !ValidFileConflictPolicies(j.ConflictPolicies) || j.State == "waiting_input" && (j.Conflict == nil || j.Revision == 0) || j.ID != hex.EncodeToString(id) || len(j.Sources) == 0 || len(j.Sources) > MaxBatchItems || validateArchiveSelection(j.ArchiveEntries) != nil || len(j.Result.Succeeded) > MaxBatchItems || len(j.Result.Failed) > MaxBatchItems || len(j.Name) > maxPathBytes || j.UpdatedAt.IsZero() {
				return ErrArchiveJobsUnavailable
			}
			for _, source := range j.Sources {
				if j.Action != "trash_restore" {
					if normalized, err := normalizeVirtual(source); err != nil || normalized != source {
						return ErrArchiveJobsUnavailable
					}
				}
			}
			if _, exists := a.records[j.ID]; exists {
				return ErrArchiveJobsUnavailable
			}
			for i, temp := range record.Temps {
				if temp != archiveJobTemp(j.Target, j.Action, j.ID, i) {
					return ErrArchiveJobsUnavailable
				}
			}
			switch j.State {
			case "queued", "running", "cancelling", "waiting_input", "complete", "partial", "error", "cancelled", "interrupted", "skipped":
			default:
				return ErrArchiveJobsUnavailable
			}
			if j.Action == "receive" {
				if record.Private == nil || !strings.HasPrefix(record.Private.StageName, ".kpanel-extract-"+j.ID) || len(record.Private.StageName) != len(".kpanel-extract-")+64 || record.Private.StageIdentity == "" || record.Private.WorkspaceIdentity == "" {
					return ErrArchiveJobsUnavailable
				}
				if _, err := hex.DecodeString(strings.TrimPrefix(record.Private.StageName, ".kpanel-extract-")); err != nil {
					return ErrArchiveJobsUnavailable
				}
				if canonical, err := normalizeVirtual(record.Private.Workspace); err != nil || canonical != record.Private.Workspace {
					return ErrArchiveJobsUnavailable
				}
				if record.Private.TargetIdentity == "" {
					return ErrArchiveJobsUnavailable
				}
			} else if record.Private != nil {
				return ErrArchiveJobsUnavailable
			}
			copy := record
			a.records[j.ID] = &copy
		}
		// A name alone cannot prove ownership after a restart or a local rename.
		for _, record := range a.records {
			if !archiveJobActive(record.Job.State) {
				continue
			}
			record.Job.State = "interrupted"
			if record.Private != nil {
				_ = m.cleanupReceivePublication(record.Private)
			}
			record.Job.Detail = "Agent 已重启，请核对目标目录后重试"
			record.Job.UpdatedAt = m.now().UTC()
			for _, temp := range record.Temps {
				if err := m.cleanupArchiveTemp(record, temp); err != nil {
					record.Job.Detail = "Agent 已重启，暂存清理未完成，请核对目标目录"
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrArchiveJobsUnavailable
	}
	a.available = true
	if err := m.persistArchiveJobsLocked(); err != nil {
		a.available = false
		return err
	}
	return nil
}

func (m *Manager) cleanupArchiveTemp(record *archiveJobRecord, virtual string) error {
	if _, _, err := m.resolveExisting(record.Job.Target); err != nil {
		return err
	}
	parent, info, err := m.openReceiveDirectory(record.Job.Target, false)
	if err != nil {
		return err
	}
	defer parent.Close()
	identity := record.TempIdentities[virtual]
	if identity.Parent == "" || identity.Object == "" || identity.Parent != stableRootReceiveIdentity(parent, ".", info) || identity.Directory == "" || identity.Directory != record.Job.Target && !isWithin(record.Job.Target, identity.Directory) {
		return ErrConflict
	}
	workspace, workspaceInfo, err := m.openReceiveDirectory(identity.Directory, false)
	if err != nil {
		return err
	}
	defer workspace.Close()
	if identity.Workspace != stableRootReceiveIdentity(workspace, ".", workspaceInfo) || !archiveWorkspaceProtected(workspaceInfo, workspace) || !archiveSameFilesystem(info, workspaceInfo) {
		return ErrConflict
	}
	visible, err := workspace.Lstat(path.Base(virtual))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return removeOwnedReceiveDirectory(workspace, path.Base(virtual), visible, nil, identity.Object)
}

func (m *Manager) persistArchiveJobsLocked() error {
	a := &m.archiveJobs
	records := make([]archiveJobRecord, 0, len(a.records))
	for _, record := range a.records {
		records = append(records, *record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Job.CreatedAt.Before(records[j].Job.CreatedAt) })
	for _, record := range records {
		if !archiveJobActive(record.Job.State) && m.now().Sub(record.Job.UpdatedAt) > 7*24*time.Hour {
			delete(a.records, record.Job.ID)
		}
	}
	records = records[:0]
	for _, record := range a.records {
		records = append(records, *record)
	}
	payload, err := json.Marshal(struct {
		Version int                `json:"version"`
		Records []archiveJobRecord `json:"records"`
	}{1, records})
	if err != nil || len(payload) > maxArchiveStateBytes {
		return ErrArchiveJobsUnavailable
	}
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return ErrArchiveJobsUnavailable
	}
	tempPath := ".kpanel-archive-state-" + hex.EncodeToString(bytes)
	temp, err := a.store.OpenFile(tempPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ErrArchiveJobsUnavailable
	}
	defer func() { _ = temp.Close(); _ = a.store.Remove(tempPath) }()
	if _, err = temp.Write(payload); err == nil {
		err = temp.Sync()
	}
	if err == nil {
		err = temp.Close()
	}
	if err == nil {
		err = a.store.Rename(tempPath, "archive-jobs-v2.json")
	}
	if err == nil {
		err = syncRootDirectory(a.store, ".")
	}
	if err != nil {
		return ErrArchiveJobsUnavailable
	}
	return nil
}

func archiveJobTemp(target, action, id string, index int) string {
	prefix := ".kpanel-extract-"
	if action == "compress" {
		prefix = ".kpanel-archive-"
	}
	return joinVirtual(target, fmt.Sprintf("%s%s-%d", prefix, id, index))
}

func cloneArchiveJob(job contract.FileArchiveJob) contract.FileArchiveJob {
	if job.Entry != nil {
		entry := *job.Entry
		job.Entry = &entry
	}
	job.Sources = append([]string{}, job.Sources...)
	job.ArchiveEntries = append([]string{}, job.ArchiveEntries...)
	job.Result.Succeeded = append([]contract.FileActionItem{}, job.Result.Succeeded...)
	job.Result.Failed = append([]contract.FileActionFailure{}, job.Result.Failed...)
	job.Result.Skipped = append([]string{}, job.Result.Skipped...)
	if job.ConflictPolicies != nil {
		p := *job.ConflictPolicies
		job.ConflictPolicies = &p
	}
	if job.Conflict != nil {
		c := *job.Conflict
		job.Conflict = &c
	}
	return job
}

func (m *Manager) ArchiveJobs() ([]contract.FileArchiveJob, error) {
	a := &m.archiveJobs
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := m.initializeArchiveJobsLocked(); err != nil {
		return nil, err
	}
	items := make([]contract.FileArchiveJob, 0, len(a.records))
	for _, record := range a.records {
		if archiveJobActive(record.Job.State) || m.now().Sub(record.Job.UpdatedAt) <= 7*24*time.Hour {
			items = append(items, cloneArchiveJob(record.Job))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

func (m *Manager) StartArchiveJob(input contract.FileActionRequest) (contract.FileArchiveJob, error) {
	var empty contract.FileArchiveJob
	if !fileJobAction(input.Action) || !ValidFileConflictPolicies(input.ConflictPolicies) || len(input.Sources) == 0 || len(input.Sources) > MaxBatchItems {
		return empty, ErrAction
	}
	deduplicateConflictSources(&input)
	if err := validateArchiveSelection(input.ArchiveEntries); err != nil {
		return empty, err
	}
	if len(input.ArchiveEntries) > 0 && (input.Action != "extract" || len(input.Sources) != 1) {
		return empty, ErrAction
	}
	_, target, err := m.resolveExisting(input.Target)
	if err != nil {
		return empty, err
	}
	input.Target = target
	info, err := m.rootFS.Lstat(rootName(target))
	if err != nil || !info.IsDir() {
		return empty, ErrNotDirectory
	}
	if input.Action == "extract" && len(input.Sources) == 1 || input.Action == "compress" {
		if err := validateName(input.Name); err != nil {
			return empty, err
		}
	}
	if input.Action == "compress" {
		if err := validateArchiveName(input.Name, input.Format); err != nil {
			return empty, err
		}
	}
	input.Sources = append([]string{}, input.Sources...)
	input.ArchiveEntries = append([]string{}, input.ArchiveEntries...)
	versions := make(map[string]string, len(input.Sources))
	seen := make(map[string]bool)
	for _, source := range input.Sources {
		if input.Action != "trash_restore" {
			if canonical, err := normalizeVirtual(source); err != nil || canonical != source {
				return empty, ErrInvalidPath
			}
		}
		if seen[source] {
			return empty, ErrAction
		}
		seen[source] = true
		version := input.ExpectedResourceVersions[source]
		if len(input.Sources) == 1 && version == "" {
			version = input.ExpectedResourceVersion
		}
		if version == "" {
			return empty, ErrConflict
		}
		checkSource := source
		if input.Action == "trash_restore" {
			if _, err := m.readTrashMetadata(source); err != nil {
				return empty, err
			}
			checkSource = joinVirtual(m.trashRoot, source)
			info, err := m.rootFS.Lstat(rootName(checkSource))
			if err != nil {
				return empty, err
			}
			if resourceVersion(checkSource, info) != version {
				return empty, ErrConflict
			}
		} else if err := m.checkExpectedVersion(source, version); err != nil {
			return empty, err
		}
		if input.Action == "extract" && archiveFormatForName(source) == "" {
			return empty, ErrInvalidArchive
		}
		versions[source] = version
	}
	input.ExpectedResourceVersions = versions
	a := &m.archiveJobs
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := m.initializeArchiveJobsLocked(); err != nil {
		return empty, err
	}
	if len(a.cancels) >= maxActiveArchiveJobs {
		return empty, ErrBusy
	}
	if len(a.records) >= maxArchiveJobs {
		var oldest *archiveJobRecord
		for _, record := range a.records {
			if !archiveJobActive(record.Job.State) && (oldest == nil || record.Job.CreatedAt.Before(oldest.Job.CreatedAt)) {
				oldest = record
			}
		}
		if oldest == nil {
			return empty, ErrBusy
		}
		delete(a.records, oldest.Job.ID)
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return empty, err
	}
	id := hex.EncodeToString(idBytes)
	name := input.Name
	if name == "" {
		name = path.Base(input.Sources[0])
	}
	if input.ConflictPolicies != nil {
		p := *input.ConflictPolicies
		input.ConflictPolicies = &p
	}
	jobFormat := input.Format
	if input.Action == "extract" && len(input.Sources) > 1 {
		name = fmt.Sprintf("%d 个压缩包", len(input.Sources))
		jobFormat = ""
	} else if input.Action == "extract" {
		jobFormat = archiveFormatForName(input.Sources[0])
	}
	job := contract.FileArchiveJob{ConflictPolicies: input.ConflictPolicies, ID: id, Action: input.Action, Name: name, Target: target, Sources: input.Sources, ArchiveEntries: input.ArchiveEntries, Format: jobFormat, State: "queued", CreatedAt: m.now().UTC(), UpdatedAt: m.now().UTC(), Result: contract.FileActionResult{Action: input.Action, Succeeded: []contract.FileActionItem{}, Failed: []contract.FileActionFailure{}}}
	record := &archiveJobRecord{Job: job}
	count := len(input.Sources)
	if input.Action != "extract" {
		count = 1
	}
	if input.Action == "compress" || input.Action == "extract" {
		for i := 0; i < count; i++ {
			record.Temps = append(record.Temps, archiveJobTemp(target, input.Action, id, i))
		}
	}
	a.records[id] = record
	if err := m.persistArchiveJobsLocked(); err != nil {
		delete(a.records, id)
		return empty, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	a.cancels[id] = cancel
	a.wg.Add(1)
	go m.runArchiveJob(ctx, cancel, id, input)
	return cloneArchiveJob(job), nil
}

func (m *Manager) runArchiveJob(ctx context.Context, cancel context.CancelFunc, id string, input contract.FileActionRequest) {
	a := &m.archiveJobs
	defer a.wg.Done()
	defer cancel()
	defer func() {
		failure := recover()
		a.mu.Lock()
		defer a.mu.Unlock()
		if failure != nil {
			if record := a.records[id]; record != nil {
				record.Job.State = "interrupted"
				record.Job.Detail = "任务意外停止，请核对目标目录后重试"
				record.Job.UpdatedAt = m.now().UTC()
				_ = m.persistArchiveJobsLocked()
			}
		}
		delete(a.cancels, id)
	}()
	acquired := false
	select {
	case a.gate <- struct{}{}:
		acquired = true
	case <-ctx.Done():
	}
	if acquired {
		defer func() {
			if acquired {
				<-a.gate
			}
		}()
	}
	update := func(fn func(*contract.FileArchiveJob)) bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		record := a.records[id]
		fn(&record.Job)
		record.Job.UpdatedAt = m.now().UTC()
		if err := m.persistArchiveJobsLocked(); err != nil {
			record.Job.State = "interrupted"
			record.Job.Detail = "任务状态保存失败，请核对目标目录"
			cancel()
			return false
		}
		return true
	}
	budget := &copyBudget{maxEntries: m.maxCopyEntries, maxBytes: m.maxCopyBytes}
	if ctx.Err() == nil && update(func(job *contract.FileArchiveJob) { job.State = "running" }) {
		count := len(input.Sources)
		if input.Action != "extract" {
			count = 1
		}
		for index := 0; index < count && ctx.Err() == nil; index++ {
			item := input
			if item.Action == "trash_restore" {
				item.TrashIDs = item.Sources
				item.Sources = nil
			}
			source := input.Sources[index]
			if input.Action == "rename" {
				item.ExpectedResourceVersion = input.ExpectedResourceVersions[source]
			}
			if input.Action == "extract" {
				item.Sources = []string{source}
				item.Format = archiveFormatForName(source)
				item.ExpectedResourceVersion = input.ExpectedResourceVersions[source]
				if count > 1 {
					item.Name = archiveBaseName(path.Base(source))
				}
			}
			operation := &archiveOperation{budget: budget, tempSuffix: fmt.Sprintf("%s-%d", id, index), tempReady: func(identity archiveTempIdentity) error {
				a.mu.Lock()
				defer a.mu.Unlock()
				record := a.records[id]
				if record.TempIdentities == nil {
					record.TempIdentities = make(map[string]archiveTempIdentity)
				}
				record.TempIdentities[record.Temps[index]] = identity
				return m.persistArchiveJobsLocked()
			}, progress: func(value *copyBudget) {
				a.mu.Lock()
				a.records[id].Job.Entries = value.entries
				a.records[id].Job.ProcessedBytes = value.bytes
				a.mu.Unlock()
			}}
			if item.Action == "rename" {
				item.Target = joinVirtual(item.Target, item.Name)
			}
			operationContext := context.WithValue(ctx, archiveContextKey{}, operation)
			if input.ConflictPolicies != nil {
				session := &ConflictSession{Policies: *input.ConflictPolicies}
				session.Prompt = func(promptContext context.Context, conflict contract.FileConflict) (contract.FileConflictDecision, error) {
					// Action owns writeMu. Release it and the archive work slot
					// while the durable task waits for its administrator.
					m.writeMu.Unlock()
					if acquired {
						<-a.gate
						acquired = false
					}
					decision, err := m.awaitArchiveConflict(promptContext, id, conflict)
					if err == nil {
						select {
						case a.gate <- struct{}{}:
							acquired = true
						case <-promptContext.Done():
							err = promptContext.Err()
						}
					}
					m.writeMu.Lock()
					return decision, err
				}
				operationContext = WithFileConflicts(operationContext, session)
			}
			result, err := m.Action(operationContext, item)
			if !update(func(job *contract.FileArchiveJob) {
				job.Result.Succeeded = append(job.Result.Succeeded, result.Succeeded...)
				job.Result.Failed = append(job.Result.Failed, result.Failed...)
				job.Result.Skipped = append(job.Result.Skipped, result.Skipped...)
				if len(job.Result.Skipped) > 64 {
					job.Result.Skipped = job.Result.Skipped[:64]
				}
				job.Result.CompletedCount += result.CompletedCount
				job.Result.SkippedCount += result.SkippedCount
				if job.ConflictPolicies != nil {
					*input.ConflictPolicies = *job.ConflictPolicies
				}
				if err != nil {
					job.Result.Failed = append(job.Result.Failed, contract.FileActionFailure{Path: source, Detail: archiveFailureDetail(err)})
				}
			}) {
				break
			}
		}
	}
	update(func(job *contract.FileArchiveJob) {
		if job.State == "interrupted" {
			return
		}
		switch {
		case ctx.Err() != nil:
			job.State = "cancelled"
			job.Detail = "任务已停止，请核对目标目录中的已完成结果"
		case len(job.Result.Skipped) > 0 && len(job.Result.Succeeded)+job.Result.CompletedCount > 0:
			job.State = "partial"
		case len(job.Result.Skipped) > 0 && len(job.Result.Failed) == 0:
			job.State = "skipped"
		case len(job.Result.Failed) > 0 && len(job.Result.Succeeded)+job.Result.CompletedCount > 0:
			job.State = "partial"
		case len(job.Result.Failed) > 0:
			job.State = "error"
		default:
			job.State = "complete"
		}
	})
}

func archiveBaseName(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".tar.gz", ".tgz", ".zip", ".tar"} {
		if strings.HasSuffix(lower, suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}

func (m *Manager) ChangeArchiveJob(id, operation string) error {
	a := &m.archiveJobs
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := m.initializeArchiveJobsLocked(); err != nil {
		return err
	}
	record, ok := a.records[id]
	if !ok {
		return os.ErrNotExist
	}
	switch operation {
	case "cancel":
		if cancel := a.cancels[id]; cancel != nil && archiveJobActive(record.Job.State) {
			record.Job.State = "cancelling"
			cancel()
		} else {
			return ErrConflict
		}
	case "clear":
		if _, active := a.cancels[id]; active {
			return ErrBusy
		}
		delete(a.records, id)
	default:
		return ErrAction
	}
	return m.persistArchiveJobsLocked()
}

func (m *Manager) closeArchiveJobs() {
	a := &m.archiveJobs
	a.mu.Lock()
	a.closed = true
	for _, cancel := range a.cancels {
		cancel()
	}
	a.mu.Unlock()
	a.wg.Wait()
	a.mu.Lock()
	if a.store != nil {
		_ = a.store.Close()
		a.store = nil
	}
	a.mu.Unlock()
}

func (m *Manager) StopArchiveJobs() { m.closeArchiveJobs() }
