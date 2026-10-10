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

func TestTransferJobStoreMigratesV1AndPersistsBTWithoutSecrets(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "file-transfers")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := contract.FileRemoteDownloadJob{ID: strings.Repeat("a", 32), State: "queued", Source: "https://example.com", TargetDirectory: "/home", CreatedAt: now, UpdatedAt: now}
	data, _ := json.Marshal(persistedJobs{SchemaVersion: 1, Jobs: []contract.FileRemoteDownloadJob{old}})
	previous := filepath.Join(root, "jobs-v1.json")
	if err := os.WriteFile(previous, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenTransferJobStore(root, filepath.Join(parent, "remote-downloads"))
	if err != nil || !store.Available() {
		t.Fatal(err)
	}
	got, err := store.Get(old.ID)
	if err != nil || got.State != "interrupted" {
		t.Fatal(got, err)
	}
	bt := contract.FileRemoteDownloadJob{ID: strings.Repeat("b", 32), State: "transferring", Source: "bittorrent", SourceKind: "bittorrent", TargetDirectory: "/home", Name: "bundle", CreatedAt: now, UpdatedAt: now, SourceBytes: 4096, TotalBytes: 8192, TransferMode: "bt-adaptive", Peers: 8}
	if err = store.Create(bt); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenTransferJobStore(root, filepath.Join(parent, "remote-downloads"))
	if err != nil {
		t.Fatal(err)
	}
	got, err = reopened.Get(bt.ID)
	if err != nil || got.State != "interrupted" || got.SourceBytes != 4096 {
		t.Fatal(got, err)
	}
	after, _ := os.ReadFile(previous)
	if !bytes.Equal(data, after) {
		t.Fatal("rollback journal mutated")
	}
	bt.Source = "magnet:?xt=urn:btih:0123456789012345678901234567890123456789&tr=https://example.com/secret"
	if err = store.Update(bt, true); err == nil {
		t.Fatal("full magnet retained in journal")
	}
}

func TestTransferJobStorePreservesHistoricalCrossHostJournalAndMigratesDownloads(t *testing.T) {
	for _, downloadHistory := range []bool{false, true} {
		t.Run(fmt.Sprint(downloadHistory), func(t *testing.T) {
			parent := t.TempDir()
			legacyRoot, currentRoot := filepath.Join(parent, "remote-downloads"), filepath.Join(parent, "file-transfers")
			legacy, err := OpenJobStore(legacyRoot)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			download := contract.FileRemoteDownloadJob{ID: strings.Repeat("a", 32), State: "transferring", Source: "https://downloads.example.com", TargetDirectory: "/home", CreatedAt: now, UpdatedAt: now}
			if downloadHistory {
				if err := legacy.Create(download); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(currentRoot, 0700); err != nil {
				t.Fatal(err)
			}
			// Historical cross-host copies persisted version/items, not schemaVersion.
			historical := []byte(`{"version":1,"jobs":[{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sourceNodeId":"cccccccccccccccccccccccccccccccc","targetHostId":"","targetDirectory":"/home","state":"complete","items":[{"path":"/source.bin","resourceVersion":"sha256:source","state":"complete","loadedBytes":7,"totalBytes":7,"entry":{"path":"/home/source.bin","name":"source.bin","kind":"file","sizeBytes":7,"resourceVersion":"sha256:target"},"retryable":false}],"createdAt":"2026-09-13T01:00:00Z","updatedAt":"2026-09-13T01:01:00Z"}]}`)
			oldCrossPath := filepath.Join(currentRoot, "jobs.json")
			if err := os.WriteFile(oldCrossPath, historical, 0600); err != nil {
				t.Fatal(err)
			}
			oldDownloadPath := filepath.Join(legacyRoot, "jobs.json")
			oldDownloads, _ := os.ReadFile(oldDownloadPath)
			current, err := OpenTransferJobStore(currentRoot, legacyRoot)
			if err != nil || !current.Available() {
				t.Fatalf("historical cross-host index disabled downloads: %v", err)
			}
			jobs, err := current.List()
			wantCount := 0
			if downloadHistory {
				wantCount = 1
			}
			if err != nil || len(jobs) != wantCount || downloadHistory && jobs[0].State != "interrupted" {
				t.Fatalf("download recovery: %#v %v", jobs, err)
			}
			cross := download
			cross.ID, cross.SourceKind, cross.Source = strings.Repeat("d", 32), "cross-host", "kpanel://"+strings.Repeat("e", 32)
			if err := current.Create(cross); err != nil {
				t.Fatal(err)
			}
			current, err = OpenTransferJobStore(currentRoot, legacyRoot)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := current.Get(cross.ID); err != nil || got.State != "interrupted" || got.SourceKind != "cross-host" {
				t.Fatalf("new common history did not survive restart: %#v %v", got, err)
			}
			for filename, before := range map[string][]byte{oldCrossPath: historical, oldDownloadPath: oldDownloads} {
				after, err := os.ReadFile(filename)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("rollback index changed: %s %v", filename, err)
				}
			}
		})
	}
}

func TestTransferJobStorePrefersRC13HistoryAndKeepsItForRollback(t *testing.T) {
	parent := t.TempDir()
	legacyRoot, currentRoot := filepath.Join(parent, "remote-downloads"), filepath.Join(parent, "file-transfers")
	now := time.Now().UTC()
	job := contract.FileRemoteDownloadJob{ID: strings.Repeat("a", 32), State: "queued", Source: "https://downloads.example.com", TargetDirectory: "/home", CreatedAt: now, UpdatedAt: now}
	before := make(map[string][]byte)
	for _, root := range []string{legacyRoot, currentRoot} {
		store, err := OpenJobStore(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Create(job); err != nil {
			t.Fatal(err)
		}
		filename := filepath.Join(root, "jobs.json")
		before[filename], err = os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		job.ID = strings.Repeat("b", 32)
		job.SourceKind, job.Source = "cross-host", "kpanel://"+strings.Repeat("c", 32)
	}
	current, err := OpenTransferJobStore(currentRoot, legacyRoot)
	if err != nil || !current.Available() {
		t.Fatalf("rc.13 migration failed: %v", err)
	}
	if jobs, err := current.List(); err != nil || len(jobs) != 1 || jobs[0].ID != job.ID || jobs[0].State != "interrupted" {
		t.Fatalf("wrong authoritative history: %#v %v", jobs, err)
	}
	if err := current.Delete(job.ID); err != nil {
		t.Fatal(err)
	}
	current, err = OpenTransferJobStore(currentRoot, legacyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if jobs, err := current.List(); err != nil || len(jobs) != 0 {
		t.Fatalf("old history reimported on restart: %#v %v", jobs, err)
	}
	for filename, original := range before {
		after, err := os.ReadFile(filename)
		if err != nil || !bytes.Equal(original, after) {
			t.Fatalf("rollback index changed: %s %v", filename, err)
		}
	}
}

func TestTransferJobStoreRefusesUnknownAndDamagedIndexes(t *testing.T) {
	for _, payload := range []string{
		`broken`, `{"schemaVersion":2,"jobs":[]}`, `{"version":2,"jobs":[]}`,
		`{"version":1,"jobs":null}`, `{"version":1,"jobs":{}}`,
		`{"version":1,"jobs":[],"schemaVersion":1}`, `{"version":1,"jobs":[]} {}`,
		`{"version":1,"jobs":[` + strings.Repeat(`{},`, 32) + `{}]}`,
		`{"version":1,"jobs":[],"extra":"` + strings.Repeat("x", 8<<20) + `"}`,
	} {
		t.Run(fmt.Sprintf("bytes-%d-%.30s", len(payload), payload), func(t *testing.T) {
			parent := t.TempDir()
			legacyRoot, currentRoot := filepath.Join(parent, "remote-downloads"), filepath.Join(parent, "file-transfers")
			if _, err := OpenJobStore(legacyRoot); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(currentRoot, 0700); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(currentRoot, "jobs.json")
			if err := os.WriteFile(filename, []byte(payload), 0600); err != nil {
				t.Fatal(err)
			}
			current, err := OpenTransferJobStore(currentRoot, legacyRoot)
			if err != nil || current.Available() {
				t.Fatalf("unknown/damaged history accepted: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(currentRoot, "jobs-v2.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("created another history: %v", err)
			}
			after, err := os.ReadFile(filename)
			if err != nil || string(after) != payload {
				t.Fatalf("abnormal history replaced: %v", err)
			}
		})
	}
}

func TestTransferJobStoreNeverReplacesDamagedVersionedIndex(t *testing.T) {
	parent := t.TempDir()
	legacyRoot, currentRoot := filepath.Join(parent, "remote-downloads"), filepath.Join(parent, "file-transfers")
	for _, root := range []string{legacyRoot, currentRoot} {
		if _, err := OpenJobStore(root); err != nil {
			t.Fatal(err)
		}
	}
	filename := filepath.Join(currentRoot, "jobs-v2.json")
	before := []byte("damaged authoritative history")
	if err := os.WriteFile(filename, before, 0600); err != nil {
		t.Fatal(err)
	}
	current, err := OpenTransferJobStore(currentRoot, legacyRoot)
	if err != nil || current.Available() {
		t.Fatalf("damaged current history bypassed: %v", err)
	}
	after, err := os.ReadFile(filename)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("damaged history replaced: %v", err)
	}
}

func TestTransferJobStoreRecognizesBoundedHistoricalIndexesWithoutDownloadHistory(t *testing.T) {
	for _, historical := range []string{
		`{"version":1,"jobs":[]}`,
		`{"version":1,"jobs":[{"sourceNodeId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","items":[{"detail":"` + strings.Repeat("x", MaxJobStateBytes) + `"}]}]}`,
	} {
		t.Run(fmt.Sprintf("bytes-%d", len(historical)), func(t *testing.T) {
			parent := t.TempDir()
			currentRoot := filepath.Join(parent, "file-transfers")
			if err := os.Mkdir(currentRoot, 0700); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(currentRoot, "jobs.json")
			if err := os.WriteFile(filename, []byte(historical), 0600); err != nil {
				t.Fatal(err)
			}
			current, err := OpenTransferJobStore(currentRoot, filepath.Join(parent, "remote-downloads"))
			if err != nil || !current.Available() {
				t.Fatalf("historical bounds rejected: %v", err)
			}
			if jobs, err := current.List(); err != nil || len(jobs) != 0 {
				t.Fatalf("historical content imported: %#v %v", jobs, err)
			}
			after, err := os.ReadFile(filename)
			if err != nil || string(after) != historical {
				t.Fatalf("historical content changed: %v", err)
			}
		})
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

func TestCrossHostJobsPreserveOrdinaryFileNames(t *testing.T) {
	store, err := OpenJobStore(filepath.Join(t.TempDir(), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i, name := range []string{" report.txt", "report.txt ", ".kpanel-user.txt"} {
		job := contract.FileRemoteDownloadJob{ID: fmt.Sprintf("%032x", i+1), State: "queued", Source: "kpanel://" + strings.Repeat("b", 32), SourceKind: "cross-host", TargetDirectory: "/home", Name: name, CreatedAt: now, UpdatedAt: now}
		if err := store.Create(job); err != nil {
			t.Fatalf("ordinary name %q rejected: %v", name, err)
		}
		loaded, _ := store.Get(job.ID)
		if loaded.Name != name {
			t.Fatalf("name was normalized: %q", loaded.Name)
		}
	}
	for _, name := range []string{"../outside", ".kpanel-upload-owned", ".kpanel-extract-owned", "name\x00"} {
		if validCrossTransferName(name) {
			t.Fatalf("internal/invalid name accepted: %q", name)
		}
	}
	if validJobName(" report.txt") || validJobName(".kpanel-user.txt") {
		t.Fatal("URL suggested-name policy weakened")
	}
}
