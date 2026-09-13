package filetransfer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func testRequest(id int) Request {
	return Request{ID: fmt.Sprintf("%032x", id), SourceNodeID: strings.Repeat("a", 32), TargetDirectory: "/target", Items: []Source{{Path: "/one", ResourceVersion: "version-1"}, {Path: "/two", ResourceVersion: "version-2"}}}
}

func waitJob(t *testing.T, m *Manager, id string) Job {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		jobs, err := m.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, j := range jobs {
			if j.ID == id && !Active(j.State) {
				return j
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job did not finish")
	return Job{}
}

func TestJobsPersistPartialResultsAndDoNotReplayDuplicateCreation(t *testing.T) {
	root := t.TempDir()
	m := Open(root)
	defer m.Close()
	r := testRequest(1)
	var calls atomic.Int32
	execute := func(ctx context.Context, source Source, emit func(contract.FileTransferEvent)) {
		calls.Add(1)
		if source.Path == "/one" {
			emit(contract.FileTransferEvent{State: "error", Code: "source_changed", Detail: "changed"})
			return
		}
		emit(contract.FileTransferEvent{State: "complete", LoadedBytes: 3, TotalBytes: 3, Entry: &contract.FileEntry{Path: "/target/two", Name: "two", Kind: "file", ResourceVersion: "target-version"}})
	}
	if _, err := m.Start(r, execute); err != nil {
		t.Fatal(err)
	}
	j := waitJob(t, m, r.ID)
	if j.State != "partial" || !j.Items[0].Retryable || j.Items[1].Entry == nil {
		t.Fatalf("job=%+v", j)
	}
	if _, err := m.Start(r, execute); err != nil || calls.Load() != 2 {
		t.Fatalf("duplicate replay: %v calls=%d", err, calls.Load())
	}
	r.Items[0].Path = "/changed"
	if _, err := m.Start(r, execute); !errors.Is(err, ErrInvalid) {
		t.Fatalf("different request=%v", err)
	}
	m.Close()
	reopened := Open(root)
	defer reopened.Close()
	items, err := reopened.List()
	if err != nil || len(items) != 1 || items[0].State != "partial" {
		t.Fatalf("reopen=%+v %v", items, err)
	}
	if err := reopened.Change(j.ID, "clear"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "jobs.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCancellationAndCloseKeepUnstartedItemsRecoverable(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprint(shutdown), func(t *testing.T) {
			m := Open(t.TempDir())
			defer m.Close()
			r := testRequest(1)
			started := make(chan struct{})
			if _, err := m.Start(r, func(ctx context.Context, _ Source, emit func(contract.FileTransferEvent)) {
				close(started)
				emit(contract.FileTransferEvent{State: "transferring", LoadedBytes: 2, TotalBytes: 3})
				<-ctx.Done()
			}); err != nil {
				t.Fatal(err)
			}
			<-started
			if err := m.Change(r.ID, "clear"); !errors.Is(err, ErrActive) {
				t.Fatal(err)
			}
			if shutdown {
				m.Close()
			} else if err := m.Change(r.ID, "cancel"); err != nil {
				t.Fatal(err)
			}
			j := waitJob(t, m, r.ID)
			if j.Items[0].Retryable || !j.Items[1].Retryable || j.Items[0].State == "complete" {
				t.Fatalf("unsafe retry=%+v", j)
			}
		})
	}
}

func TestRestartMarksRunningItemsUncertainWithoutExecuting(t *testing.T) {
	root := t.TempDir()
	m := Open(root)
	r := testRequest(1)
	now := time.Now().UTC()
	m.jobs[r.ID] = Job{ID: r.ID, SourceNodeID: r.SourceNodeID, TargetDirectory: r.TargetDirectory, State: "running", CreatedAt: now, UpdatedAt: now, Items: []Item{{Source: r.Items[0], State: "committing"}, {Source: r.Items[1], State: "queued", Retryable: true}}}
	if err := m.persist(); err != nil {
		t.Fatal(err)
	}
	reopened := Open(root)
	defer reopened.Close()
	j := waitJob(t, reopened, r.ID)
	if j.State != "interrupted" || j.Items[0].Retryable || !j.Items[1].Retryable {
		t.Fatalf("restart=%+v", j)
	}
}

func TestQueueAndStorageFailuresAreBounded(t *testing.T) {
	m := Open(t.TempDir())
	defer m.Close()
	var concurrent, max atomic.Int32
	execute := func(ctx context.Context, _ Source, _ func(contract.FileTransferEvent)) {
		n := concurrent.Add(1)
		defer concurrent.Add(-1)
		for {
			old := max.Load()
			if old >= n || max.CompareAndSwap(old, n) {
				break
			}
		}
		<-ctx.Done()
	}
	for i := 1; i <= MaxPending; i++ {
		if _, err := m.Start(testRequest(i), execute); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Start(testRequest(99), execute); !errors.Is(err, ErrFull) {
		t.Fatal(err)
	}
	m.Close()
	if max.Load() > 2 {
		t.Fatalf("workers=%d", max.Load())
	}
	broken := Open(t.TempDir())
	defer broken.Close()
	broken.write = func([]byte) error { return errors.New("disk full") }
	if _, err := broken.Start(testRequest(1), execute); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if len(broken.cancels) != 0 || len(broken.jobs) != 0 {
		t.Fatal("failed persistence started work")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "jobs.json"), []byte("invalid state"), 0600); err != nil {
		t.Fatal(err)
	}
	corrupt := Open(root)
	defer corrupt.Close()
	if _, err := corrupt.List(); !errors.Is(err, ErrUnavailable) {
		t.Fatal("corrupt state replaced")
	}
}

func TestRequestValidationAndReturnedSnapshots(t *testing.T) {
	r := testRequest(1)
	for _, bad := range []string{"relative", "/a/../b", "/a//b", "/a\\b"} {
		r.TargetDirectory = bad
		if Validate(r) == nil {
			t.Fatal(bad)
		}
	}
	r = testRequest(1)
	r.Items = append(r.Items, r.Items[0])
	if Validate(r) == nil {
		t.Fatal("duplicate source")
	}
	r = testRequest(1)
	r.Items = make([]Source, MaxItems+1)
	if Validate(r) == nil {
		t.Fatal("oversize")
	}
	m := Open(t.TempDir())
	defer m.Close()
	r = testRequest(1)
	if _, err := m.Start(r, func(_ context.Context, source Source, emit func(contract.FileTransferEvent)) {
		emit(contract.FileTransferEvent{State: "complete", Entry: &contract.FileEntry{Path: "/target" + source.Path, ResourceVersion: "target"}})
	}); err != nil {
		t.Fatal(err)
	}
	first := waitJob(t, m, r.ID)
	first.Items[0].Path = "/changed"
	first.Items[0].Entry.Path = "/changed"
	second := waitJob(t, m, r.ID)
	if second.Items[0].Path != "/one" || second.Items[0].Entry.Path != "/target/one" {
		t.Fatal("mutable snapshot escaped")
	}
}

func TestRetryConsumesOnlyRecoverableItemsAtomicallyAndOnlyOnce(t *testing.T) {
	root := t.TempDir()
	m := Open(root)
	defer m.Close()
	r := testRequest(1)
	if _, err := m.Start(r, func(_ context.Context, source Source, emit func(contract.FileTransferEvent)) {
		if source.Path == "/one" {
			emit(contract.FileTransferEvent{State: "error", Code: "source_unavailable"})
		} else {
			emit(contract.FileTransferEvent{State: "error", Detail: "response lost after commit"})
		}
	}); err != nil {
		t.Fatal(err)
	}
	waitJob(t, m, r.ID)
	retry := testRequest(2)
	retry.RetryOf = r.ID
	retry.Items = retry.Items[:1]
	originalWrite := m.write
	m.write = func([]byte) error { return errors.New("disk full") }
	if _, err := m.Start(retry, nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if !waitJob(t, m, r.ID).Items[0].Retryable {
		t.Fatal("failed persistence consumed retry")
	}
	m.write = originalWrite
	var calls atomic.Int32
	execute := func(_ context.Context, source Source, emit func(contract.FileTransferEvent)) {
		calls.Add(1)
		emit(contract.FileTransferEvent{State: "complete", Entry: &contract.FileEntry{Path: "/target/one", ResourceVersion: "v2"}})
	}
	if _, err := m.Start(retry, execute); err != nil {
		t.Fatal(err)
	}
	waitJob(t, m, retry.ID)
	if waitJob(t, m, r.ID).Items[0].Retryable {
		t.Fatal("retry not consumed")
	}
	duplicate := retry
	duplicate.ID = testRequest(3).ID
	if j, err := m.Start(duplicate, execute); err != nil || j.ID != retry.ID || calls.Load() != 1 {
		t.Fatalf("duplicate=%+v %v calls=%d", j, err, calls.Load())
	}
	m.Close()
	reopened := Open(root)
	defer reopened.Close()
	if j, err := reopened.Start(duplicate, execute); err != nil || j.ID != retry.ID || calls.Load() != 1 {
		t.Fatal("restart replayed retry")
	}
	if err := reopened.Change(retry.ID, "clear"); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Start(duplicate, execute); !errors.Is(err, ErrInvalid) {
		t.Fatal("clearing child revived consumed items")
	}
}
