package remotedownload

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestJobStorePersistsOnlyRedactedSourceAndInterruptsActiveJobs(t *testing.T) {
	root := filepath.Join(t.TempDir(), "jobs")
	store, err := OpenJobStore(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	job := contract.FileRemoteDownloadJob{
		ID: strings.Repeat("a", 32), State: "transferring", Source: "https://downloads.example.com",
		TargetDirectory: "/home", Name: "release.zip", LoadedBytes: 7,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Create(job); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "token") || strings.Contains(string(data), "signature") {
		t.Fatalf("job state leaked URL material: %s", data)
	}
	reopened, err := OpenJobStore(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Get(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != "interrupted" || loaded.Code != "remote_download_interrupted" || loaded.FinishedAt == nil {
		t.Fatalf("recovered job = %#v", loaded)
	}
}

func TestTransferJobStoreMigratesWithoutChangingRollbackJournal(t *testing.T) {
	root := t.TempDir()
	legacyRoot, newRoot := filepath.Join(root, "remote-downloads"), filepath.Join(root, "file-transfers")
	legacy, err := OpenJobStore(legacyRoot)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := contract.FileRemoteDownloadJob{ID: strings.Repeat("a", 32), State: "transferring", Source: "https://downloads.example.com", TargetDirectory: "/home", CreatedAt: now, UpdatedAt: now}
	if err := legacy.Create(job); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(legacyRoot, "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := OpenTransferJobStore(newRoot, legacyRoot)
	if err != nil || !current.Available() {
		t.Fatalf("migration=%v", err)
	}
	recovered, err := current.Get(job.ID)
	if err != nil || recovered.State != "interrupted" {
		t.Fatalf("recovery=%#v %v", recovered, err)
	}
	after, _ := os.ReadFile(filepath.Join(legacyRoot, "jobs.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("rollback journal changed")
	}
	job.ID = strings.Repeat("b", 32)
	job.SourceKind = "cross-host"
	job.Source = "kpanel://" + strings.Repeat("c", 32)
	job.TargetHostID = strings.Repeat("d", 32)
	if err := current.Create(job); err != nil {
		t.Fatal(err)
	}
	current, err = OpenTransferJobStore(newRoot, legacyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if restarted, err := current.Get(job.ID); err != nil || restarted.State != "interrupted" || restarted.TargetHostID != job.TargetHostID {
		t.Fatalf("cross-host restart=%#v %v", restarted, err)
	}
	// A corrupt legacy index must be preserved and never silently discarded.
	brokenRoot := filepath.Join(root, "broken")
	_ = os.Mkdir(brokenRoot, 0700)
	_ = os.WriteFile(filepath.Join(brokenRoot, "jobs.json"), []byte("broken"), 0600)
	disabled, err := OpenTransferJobStore(filepath.Join(root, "disabled"), brokenRoot)
	if err != nil || disabled.Available() {
		t.Fatalf("corrupt migration enabled: %v", err)
	}
}

func TestJobStoreBoundsHistoryAndRefusesActiveDeletion(t *testing.T) {
	store, err := OpenJobStore(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := contract.FileRemoteDownloadJob{
		ID: strings.Repeat("b", 32), State: "queued", Source: "https://downloads.example.com",
		TargetDirectory: "/home", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Create(job); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(job.ID); err != ErrJobActive {
		t.Fatalf("delete active error = %v", err)
	}
	finished := now.Add(time.Second)
	job.State = "cancelled"
	job.Code = "remote_download_cancelled"
	job.UpdatedAt = finished
	job.FinishedAt = &finished
	if err := store.Update(job, true); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(job.ID); err != nil {
		t.Fatal(err)
	}
	jobs, err := store.List()
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%#v err=%v", jobs, err)
	}
}

func TestJobStoreEvictsOldestTerminalJobAtHistoryLimit(t *testing.T) {
	store, err := OpenJobStore(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index := range MaxJobs {
		finished := now.Add(time.Duration(index) * time.Second)
		job := contract.FileRemoteDownloadJob{
			ID: fmt.Sprintf("%032x", index+1), State: "cancelled", Source: "https://downloads.example.com",
			TargetDirectory: "/home", CreatedAt: finished, UpdatedAt: finished, FinishedAt: &finished,
		}
		store.jobs[job.ID] = job
	}
	newJob := contract.FileRemoteDownloadJob{
		ID: strings.Repeat("f", 32), State: "queued", Source: "https://downloads.example.com",
		TargetDirectory: "/home", CreatedAt: now.Add(time.Hour), UpdatedAt: now.Add(time.Hour),
	}
	if err := store.Create(newJob); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(fmt.Sprintf("%032x", 1)); err != ErrJobNotFound {
		t.Fatalf("oldest job error = %v", err)
	}
	jobs, err := store.List()
	if err != nil || len(jobs) != MaxJobs {
		t.Fatalf("jobs=%d err=%v", len(jobs), err)
	}
}

func TestJobStoreKeepsLatestInMemoryStateWhenPersistenceFails(t *testing.T) {
	store, err := OpenJobStore(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := contract.FileRemoteDownloadJob{
		ID: strings.Repeat("c", 32), State: "queued", Source: "https://downloads.example.com",
		TargetDirectory: "/home", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Create(job); err != nil {
		t.Fatal(err)
	}
	persistError := errors.New("disk full")
	store.writeAtomic = func(string, string, []byte) error { return persistError }
	finished := now.Add(time.Second)
	job.State = "error"
	job.Code = "remote_download_interrupted"
	job.UpdatedAt = finished
	job.FinishedAt = &finished
	if err := store.Update(job, true); !errors.Is(err, persistError) {
		t.Fatalf("update error = %v", err)
	}
	loaded, err := store.Get(job.ID)
	if err != nil || loaded.State != "error" || loaded.FinishedAt == nil {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestJobStorePrunesExpiredTerminalJobsWhenListed(t *testing.T) {
	store, err := OpenJobStore(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	finished := now.Add(-JobRetention - time.Minute)
	job := contract.FileRemoteDownloadJob{
		ID: strings.Repeat("d", 32), State: "cancelled", Source: "https://downloads.example.com",
		TargetDirectory: "/home", CreatedAt: finished, UpdatedAt: finished, FinishedAt: &finished,
	}
	if err := store.Create(job); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	jobs, err := store.List()
	if err != nil || len(jobs) != 0 {
		t.Fatalf("jobs=%#v err=%v", jobs, err)
	}
	reopened, err := OpenJobStore(store.root)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err = reopened.List()
	if err != nil || len(jobs) != 0 {
		t.Fatalf("reopened jobs=%#v err=%v", jobs, err)
	}
}

func TestOpenJobStoreRejectsRelativeRootAsConfigurationError(t *testing.T) {
	store, err := OpenJobStore("remote-downloads")
	if err == nil || store != nil {
		t.Fatalf("store=%#v err=%v, want configuration error", store, err)
	}
}

func TestOpenJobStoreDegradesWhenRootIsRegularFileWithoutChangingIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "remote-downloads")
	original := []byte("preserve this abnormal job index path")
	if err := os.WriteFile(root, original, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenJobStore(root)
	if err != nil || store == nil {
		t.Fatalf("store=%#v err=%v", store, err)
	}
	if store.Available() {
		t.Fatal("regular-file job root remained available")
	}
	if _, err := store.Get(strings.Repeat("a", 32)); err != ErrJobStoreUnavailable {
		t.Fatalf("Get error = %v, want ErrJobStoreUnavailable", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(root)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || !bytes.Equal(content, original) {
		t.Fatalf("abnormal root was changed: mode=%s content=%q", info.Mode(), content)
	}
}

func TestOpenJobStoreDegradesWhenInitialPersistenceFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "remote-downloads")
	persistError := errors.New("injected job index permission failure")
	writeCalls := 0
	store, err := openJobStore(
		root,
		readPersistedJobs,
		func(string, string, []byte) error {
			writeCalls++
			return persistError
		},
	)
	if err != nil || store == nil {
		t.Fatalf("store=%#v err=%v", store, err)
	}
	if store.Available() || writeCalls != 1 {
		t.Fatalf("available=%t writeCalls=%d", store.Available(), writeCalls)
	}
	if _, err := os.Stat(filepath.Join(root, "jobs.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed initial persistence left jobs.json: %v", err)
	}
}

func TestOpenJobStoreDegradesOnStateReadErrorWithoutChangingIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "remote-downloads")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, "jobs.json")
	original := []byte("preserve unreadable state")
	if err := os.WriteFile(statePath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	readError := errors.New("injected job index read failure")
	writeCalls := 0
	store, err := openJobStore(
		root,
		func(path string) (persistedJobs, error) {
			if path != statePath {
				t.Fatalf("read path = %q, want %q", path, statePath)
			}
			return persistedJobs{}, readError
		},
		func(string, string, []byte) error {
			writeCalls++
			return nil
		},
	)
	if err != nil || store == nil {
		t.Fatalf("store=%#v err=%v", store, err)
	}
	content, readErr := os.ReadFile(statePath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if store.Available() || writeCalls != 0 || !bytes.Equal(content, original) {
		t.Fatalf("available=%t writeCalls=%d content=%q", store.Available(), writeCalls, content)
	}
}

func TestJobStoreFailsClosedOnCorruptState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "jobs")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "jobs.json"), []byte(`{"schemaVersion":1,"jobs":[{"id":"secret"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenJobStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.Available() {
		t.Fatal("corrupt job store remained available")
	}
	if _, err := store.Get(strings.Repeat("a", 32)); err != ErrJobStoreUnavailable {
		t.Fatalf("get error = %v", err)
	}
	_, err = store.List()
	if err != ErrJobStoreUnavailable {
		t.Fatalf("list error = %v", err)
	}
	encoded, _ := json.Marshal(store)
	if strings.Contains(string(encoded), "secret") {
		t.Fatalf("store leaked corrupt content: %s", encoded)
	}
}

func TestJobStoreFailsClosedOnInconsistentCompletedState(t *testing.T) {
	root := filepath.Join(t.TempDir(), "jobs")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := persistedJobs{SchemaVersion: jobSchemaVersion, Jobs: []contract.FileRemoteDownloadJob{{
		ID: strings.Repeat("e", 32), State: "complete", Source: "https://downloads.example.com",
		TargetDirectory: "/home", Name: "artifact.bin", LoadedBytes: 7,
		Entry: &contract.FileEntry{
			Name: "different.bin", Path: "/home/artifact.bin", Kind: "file", SizeBytes: 7,
			ResourceVersion: "sha256:test",
		},
		CreatedAt: now, UpdatedAt: now, FinishedAt: &now,
	}}}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "jobs.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenJobStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.Available() {
		t.Fatal("inconsistent completed state remained available")
	}
}
