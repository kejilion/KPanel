package filemanager

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func newTestManager(t testing.TB) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	manager, err := New(Config{
		Root: root,
		ProtectedVirtual: []string{
			"/docker/kpanel",
			"/.kpanel-trash",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close manager: %v", err)
		}
	})
	return manager, root
}

func TestListHidesProtectedDirectoryAndSortsDirectoriesFirst(t *testing.T) {
	manager, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "docker", "kpanel"))
	mustMkdirAll(t, filepath.Join(root, "website"))
	mustWrite(t, filepath.Join(root, "a.txt"), "hello")

	result, err := manager.List(context.Background(), "/", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 3 || result.Entries[0].Kind != "directory" {
		t.Fatalf("unexpected directory listing: %#v", result.Entries)
	}
	docker, err := manager.List(context.Background(), "/docker", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(docker.Entries) != 0 {
		t.Fatalf("protected directory leaked: %#v", docker.Entries)
	}
}

func TestRejectsTraversalProtectedPathsAndSymlinks(t *testing.T) {
	manager, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "docker", "kpanel"))
	for _, value := range []string{"/../etc", `/docker\kpanel`, "/docker/kpanel"} {
		if _, err := manager.List(context.Background(), value, 10); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.List(context.Background(), "/escape", 10); !errors.Is(err, ErrSymlink) {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	if _, err := manager.Upload(
		context.Background(), "/escape", "outside.txt",
		strings.NewReader("blocked"), 7, false,
	); !errors.Is(err, ErrSymlink) {
		t.Fatalf("expected upload through symlink to be rejected, got %v", err)
	}
}

func TestProtectedDirectoryCannotBeChangedThroughAnAncestor(t *testing.T) {
	manager, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "docker", "kpanel"))
	mustWrite(t, filepath.Join(root, "docker", "kpanel", "state.db"), "protected")

	for _, action := range []contract.FileActionRequest{
		{Action: "trash", Sources: []string{"/docker"}},
		{Action: "copy", Sources: []string{"/docker"}, Target: "/"},
		{Action: "chmod", Sources: []string{"/docker"}, Mode: "700"},
	} {
		result, err := manager.Action(context.Background(), action)
		if err != nil {
			t.Fatalf("%s returned top-level error: %v", action.Action, err)
		}
		if len(result.Failed) != 1 || !strings.Contains(result.Failed[0].Detail, ErrProtected.Error()) {
			t.Fatalf("%s should reject protected ancestor: %#v", action.Action, result)
		}
	}
	content, err := os.ReadFile(filepath.Join(root, "docker", "kpanel", "state.db"))
	if err != nil || string(content) != "protected" {
		t.Fatalf("protected content changed: %q, err=%v", content, err)
	}
}

func TestReadOnlyVirtualDirectoryCanBeListedButNotMutated(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "proc"))
	mustWrite(t, filepath.Join(root, "proc", "status"), "ok")
	manager, err := New(Config{
		Root: root, ReadOnlyVirtual: []string{"/proc"}, TrashVirtual: "/state/trash",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	listed, err := manager.List(context.Background(), "/proc", 10)
	if err != nil || len(listed.Entries) != 1 {
		t.Fatalf("read-only directory was not listable: %#v err=%v", listed, err)
	}
	if _, err := manager.Upload(context.Background(), "/proc", "new", strings.NewReader("x"), 1, false); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("read-only upload was not rejected: %v", err)
	}
	entry, err := manager.Stat("/proc/status")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.WriteText(context.Background(), "/proc/status", contract.FileWriteRequest{
		Content: "changed", ExpectedResourceVersion: entry.ResourceVersion,
	}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("read-only text edit was not rejected: %v", err)
	}
	result, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "trash", Sources: []string{"/proc/status"},
	})
	if err != nil || len(result.Failed) != 1 || !strings.Contains(result.Failed[0].Detail, ErrReadOnly.Error()) {
		t.Fatalf("read-only trash was not rejected: %#v err=%v", result, err)
	}
}

func TestWriteTextIsAtomicAndRequiresResourceVersion(t *testing.T) {
	manager, root := newTestManager(t)
	mustWrite(t, filepath.Join(root, "config.json"), `{"enabled":false}`)
	entry, err := manager.Stat("/config.json")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := manager.WriteText(context.Background(), "/config.json", contract.FileWriteRequest{
		Content: `{"enabled":true}`, ExpectedResourceVersion: entry.ResourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ResourceVersion == entry.ResourceVersion {
		t.Fatal("resource version did not change")
	}
	content, err := os.ReadFile(filepath.Join(root, "config.json"))
	if err != nil || string(content) != `{"enabled":true}` {
		t.Fatalf("unexpected content %q, err=%v", content, err)
	}
	if _, err := manager.WriteText(context.Background(), "/config.json", contract.FileWriteRequest{
		Content: "stale", ExpectedResourceVersion: entry.ResourceVersion,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected stale write conflict, got %v", err)
	}
}

func TestActiveSVGContentUsesTheTextEditor(t *testing.T) {
	manager, root := newTestManager(t)
	mustWrite(t, filepath.Join(root, "icon.svg"), `<svg xmlns="http://www.w3.org/2000/svg"></svg>`)
	entry, err := manager.Stat("/icon.svg")
	if err != nil {
		t.Fatal(err)
	}
	if !entry.Editable || !entry.Previewable {
		t.Fatalf("SVG viewer policy = %#v", entry)
	}
}

func TestUploadCopyMoveChmodAndTrash(t *testing.T) {
	manager, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "source"))
	mustMkdirAll(t, filepath.Join(root, "target"))
	entry, err := manager.Upload(
		context.Background(), "/source", "hello.txt",
		strings.NewReader("hello"), 5, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if entry.SizeBytes != 5 {
		t.Fatalf("unexpected upload entry: %#v", entry)
	}
	if _, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "copy", Sources: []string{"/source/hello.txt"}, Target: "/target",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "hello.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "chmod", Sources: []string{"/target/hello.txt"}, Mode: "640",
	}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(root, "target", "hello.txt"))
		if info.Mode().Perm() != 0640 {
			t.Fatalf("unexpected mode %o", info.Mode().Perm())
		}
	}
	if _, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "trash", Sources: []string{"/target/hello.txt"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "hello.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file was not moved to trash: %v", err)
	}
	trashEntries, err := os.ReadDir(filepath.Join(root, ".kpanel-trash", "files"))
	if err != nil || len(trashEntries) != 1 {
		t.Fatalf("unexpected trash: %#v, err=%v", trashEntries, err)
	}
}

func TestCopyAndMoveHonorExpectedResourceVersions(t *testing.T) {
	manager, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "source"))
	mustMkdirAll(t, filepath.Join(root, "target"))
	for _, name := range []string{"copy.txt", "move.txt"} {
		mustWrite(t, filepath.Join(root, "source", name), "current")
	}

	for _, action := range []string{"copy", "move"} {
		source := "/source/" + action + ".txt"
		result, err := manager.Action(context.Background(), contract.FileActionRequest{
			Action: action, Sources: []string{source}, Target: "/target",
			ExpectedResourceVersions: map[string]string{source: "sha256:stale"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Succeeded) != 0 || len(result.Failed) != 1 ||
			!strings.Contains(result.Failed[0].Detail, ErrConflict.Error()) {
			t.Fatalf("%s stale result: %#v", action, result)
		}
		if _, statErr := os.Stat(filepath.Join(root, "source", action+".txt")); statErr != nil {
			t.Fatalf("%s changed stale source: %v", action, statErr)
		}
		if _, statErr := os.Stat(filepath.Join(root, "target", action+".txt")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("%s created stale destination: %v", action, statErr)
		}
	}
}

func TestTrashActionProcessesEverySource(t *testing.T) {
	manager, root := newTestManager(t)
	sources := []string{"/first.txt", "/second.txt"}
	expectedVersions := make(map[string]string, len(sources))
	for _, source := range sources {
		mustWrite(t, filepath.Join(root, strings.TrimPrefix(source, "/")), source)
		entry, err := manager.Stat(source)
		if err != nil {
			t.Fatal(err)
		}
		expectedVersions[source] = entry.ResourceVersion
	}

	result, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action:                   "trash",
		Sources:                  sources,
		ExpectedResourceVersions: expectedVersions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Succeeded) != len(sources) || len(result.Failed) != 0 {
		t.Fatalf("batch trash result: %#v", result)
	}
	for _, source := range sources {
		_, err := os.Stat(filepath.Join(root, strings.TrimPrefix(source, "/")))
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s was not moved to trash: %v", source, err)
		}
	}
	trash, err := manager.ListTrash(context.Background())
	if err != nil || trash.Total != len(sources) {
		t.Fatalf("unexpected trash after batch action: %#v err=%v", trash, err)
	}
}

func TestBatchCopyMoveAndChmodProcessEverySource(t *testing.T) {
	manager, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "copy-target"))
	mustMkdirAll(t, filepath.Join(root, "move-target"))
	copySources := []string{"/copy-first.txt", "/copy-second.txt"}
	moveSources := []string{"/move-first.txt", "/move-second.txt"}
	for _, source := range append(copySources, moveSources...) {
		mustWrite(t, filepath.Join(root, strings.TrimPrefix(source, "/")), source)
	}

	result, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "copy", Sources: copySources, Target: "/copy-target",
	})
	if err != nil || len(result.Succeeded) != len(copySources) || len(result.Failed) != 0 {
		t.Fatalf("batch copy result: %#v err=%v", result, err)
	}
	result, err = manager.Action(context.Background(), contract.FileActionRequest{
		Action: "move", Sources: moveSources, Target: "/move-target",
	})
	if err != nil || len(result.Succeeded) != len(moveSources) || len(result.Failed) != 0 {
		t.Fatalf("batch move result: %#v err=%v", result, err)
	}

	chmodSources := make([]string, 0, len(copySources))
	expectedVersions := make(map[string]string, len(copySources))
	for _, source := range copySources {
		target := "/copy-target/" + path.Base(source)
		entry, statErr := manager.Stat(target)
		if statErr != nil {
			t.Fatal(statErr)
		}
		chmodSources = append(chmodSources, target)
		expectedVersions[target] = entry.ResourceVersion
	}
	result, err = manager.Action(context.Background(), contract.FileActionRequest{
		Action: "chmod", Sources: chmodSources, Mode: "640",
		ExpectedResourceVersions: expectedVersions,
	})
	if err != nil || len(result.Succeeded) != len(chmodSources) || len(result.Failed) != 0 {
		t.Fatalf("batch chmod result: %#v err=%v", result, err)
	}

	for _, source := range copySources {
		if _, statErr := os.Stat(filepath.Join(root, "copy-target", path.Base(source))); statErr != nil {
			t.Fatal(statErr)
		}
	}
	for _, source := range moveSources {
		if _, statErr := os.Stat(filepath.Join(root, strings.TrimPrefix(source, "/"))); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("move source %s still exists: %v", source, statErr)
		}
		if _, statErr := os.Stat(filepath.Join(root, "move-target", path.Base(source))); statErr != nil {
			t.Fatal(statErr)
		}
	}
}

func TestBatchTrashRestoreAndDeleteProcessEveryID(t *testing.T) {
	manager, root := newTestManager(t)
	sources := []string{"/restore-first.txt", "/restore-second.txt", "/delete-first.txt", "/delete-second.txt"}
	expectedVersions := make(map[string]string, len(sources))
	for _, source := range sources {
		mustWrite(t, filepath.Join(root, strings.TrimPrefix(source, "/")), source)
		entry, err := manager.Stat(source)
		if err != nil {
			t.Fatal(err)
		}
		expectedVersions[source] = entry.ResourceVersion
	}
	result, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "trash", Sources: sources, ExpectedResourceVersions: expectedVersions,
	})
	if err != nil || len(result.Succeeded) != len(sources) || len(result.Failed) != 0 {
		t.Fatalf("prepare batch trash result: %#v err=%v", result, err)
	}

	trash, err := manager.ListTrash(context.Background())
	if err != nil || trash.Total != len(sources) {
		t.Fatalf("unexpected trash list: %#v err=%v", trash, err)
	}
	restoreIDs := make([]string, 0, 2)
	deleteIDs := make([]string, 0, 2)
	trashVersions := make(map[string]string, len(trash.Entries))
	for _, entry := range trash.Entries {
		trashVersions[entry.ID] = entry.ResourceVersion
		if strings.HasPrefix(entry.Name, "restore-") {
			restoreIDs = append(restoreIDs, entry.ID)
		} else {
			deleteIDs = append(deleteIDs, entry.ID)
		}
	}
	result, err = manager.Action(context.Background(), contract.FileActionRequest{
		Action: "trash_restore", TrashIDs: restoreIDs, ExpectedResourceVersions: trashVersions,
	})
	if err != nil || len(result.Succeeded) != len(restoreIDs) || len(result.Failed) != 0 {
		t.Fatalf("batch restore result: %#v err=%v", result, err)
	}
	result, err = manager.Action(context.Background(), contract.FileActionRequest{
		Action: "trash_delete", TrashIDs: deleteIDs, ExpectedResourceVersions: trashVersions,
	})
	if err != nil || len(result.Succeeded) != len(deleteIDs) || len(result.Failed) != 0 {
		t.Fatalf("batch delete result: %#v err=%v", result, err)
	}

	for _, source := range sources[:2] {
		if _, statErr := os.Stat(filepath.Join(root, strings.TrimPrefix(source, "/"))); statErr != nil {
			t.Fatalf("restored source %s is missing: %v", source, statErr)
		}
	}
	trash, err = manager.ListTrash(context.Background())
	if err != nil || trash.Total != 0 {
		t.Fatalf("trash not empty after batch restore/delete: %#v err=%v", trash, err)
	}
}

func TestTrashCanBeListedRestoredAndPermanentlyDeleted(t *testing.T) {
	manager, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "documents"))
	mustWrite(t, filepath.Join(root, "documents", "restore.txt"), "restore me")
	mustWrite(t, filepath.Join(root, "delete.txt"), "delete me")

	for _, source := range []string{"/documents/restore.txt", "/delete.txt"} {
		entry, err := manager.Stat(source)
		if err != nil {
			t.Fatal(err)
		}
		result, err := manager.Action(context.Background(), contract.FileActionRequest{
			Action: "trash", Sources: []string{source},
			ExpectedResourceVersions: map[string]string{source: entry.ResourceVersion},
		})
		if err != nil || len(result.Succeeded) != 1 {
			t.Fatalf("trash %s: %#v err=%v", source, result, err)
		}
	}

	trash, err := manager.ListTrash(context.Background())
	if err != nil || trash.Total != 2 {
		t.Fatalf("unexpected trash list: %#v err=%v", trash, err)
	}
	var restoreEntry, deleteEntry contract.FileTrashEntry
	for _, entry := range trash.Entries {
		switch entry.OriginalPath {
		case "/documents/restore.txt":
			restoreEntry = entry
		case "/delete.txt":
			deleteEntry = entry
		}
	}
	if !restoreEntry.Restorable || !deleteEntry.Restorable {
		t.Fatalf("trash metadata missing: %#v", trash.Entries)
	}
	result, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "trash_restore", TrashIDs: []string{restoreEntry.ID},
		ExpectedResourceVersions: map[string]string{restoreEntry.ID: restoreEntry.ResourceVersion},
	})
	if err != nil || len(result.Succeeded) != 1 {
		t.Fatalf("restore: %#v err=%v", result, err)
	}
	content, err := os.ReadFile(filepath.Join(root, "documents", "restore.txt"))
	if err != nil || string(content) != "restore me" {
		t.Fatalf("restored content = %q err=%v", content, err)
	}
	result, err = manager.Action(context.Background(), contract.FileActionRequest{
		Action: "trash_delete", TrashIDs: []string{deleteEntry.ID},
		ExpectedResourceVersions: map[string]string{deleteEntry.ID: deleteEntry.ResourceVersion},
	})
	if err != nil || len(result.Succeeded) != 1 {
		t.Fatalf("trash delete: %#v err=%v", result, err)
	}
	trash, err = manager.ListTrash(context.Background())
	if err != nil || trash.Total != 0 {
		t.Fatalf("trash not empty: %#v err=%v", trash, err)
	}
}

func TestPermanentDeleteIsOnlyAvailableInsideTrash(t *testing.T) {
	manager, root := newTestManager(t)
	mustWrite(t, filepath.Join(root, "delete.txt"), "delete me")
	result, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "delete", Sources: []string{"/delete.txt"},
	})
	if !errors.Is(err, ErrAction) || len(result.Succeeded) != 0 {
		t.Fatalf("ordinary permanent delete result: %#v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "delete.txt")); err != nil {
		t.Fatalf("ordinary file must remain: %v", err)
	}
}

func TestRenameDoesNotReplaceExistingTarget(t *testing.T) {
	manager, root := newTestManager(t)
	mustWrite(t, filepath.Join(root, "source.txt"), "source")
	mustWrite(t, filepath.Join(root, "target.txt"), "target")

	source, err := manager.Stat("/source.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "rename", Sources: []string{"/source.txt"}, Target: "/target.txt",
		ExpectedResourceVersion: source.ResourceVersion,
	}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected existing-target rejection, got %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, "target.txt"))
	if err != nil || string(content) != "target" {
		t.Fatalf("target was modified: %q, err=%v", content, err)
	}
}

func TestLimitsUploadsAndBatchOperations(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Upload(
		context.Background(), "/", "large.bin",
		bytes.NewReader(nil), MaxUploadBytes+1, false,
	); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected upload limit, got %v", err)
	}
	sources := make([]string, MaxBatchItems+1)
	if _, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "trash", Sources: sources,
	}); !errors.Is(err, ErrBatchTooLarge) {
		t.Fatalf("expected batch limit, got %v", err)
	}
}

func TestDownloadGateRejectsExcessConcurrentReads(t *testing.T) {
	manager, root := newTestManager(t)
	mustWrite(t, filepath.Join(root, "download.txt"), "content")
	opened := make([]io.Closer, 0, 4)
	for range 4 {
		file, _, err := manager.Open(context.Background(), "/download.txt")
		if err != nil {
			t.Fatal(err)
		}
		opened = append(opened, file)
	}
	if _, _, err := manager.Open(context.Background(), "/download.txt"); !errors.Is(err, ErrBusy) {
		t.Fatalf("expected busy download gate, got %v", err)
	}
	for _, file := range opened {
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if file, _, err := manager.Open(context.Background(), "/download.txt"); err != nil {
		t.Fatalf("gate did not recover: %v", err)
	} else {
		_ = file.Close()
	}
}

func TestBatchCopyUsesOneCumulativeBudget(t *testing.T) {
	root := t.TempDir()
	manager, err := New(Config{Root: root, MaxCopyBytes: 12})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	mustMkdirAll(t, filepath.Join(root, "target"))
	mustWrite(t, filepath.Join(root, "first.txt"), "12345678")
	mustWrite(t, filepath.Join(root, "second.txt"), "abcdefgh")

	result, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "copy", Sources: []string{"/first.txt", "/second.txt"}, Target: "/target",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Succeeded) != 1 || len(result.Failed) != 1 {
		t.Fatalf("unexpected cumulative result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "first.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "second.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second copy should be rejected, got %v", err)
	}
}

func TestListPageFindsEntriesBeyondFirstPage(t *testing.T) {
	manager, root := newTestManager(t)
	for index := range 620 {
		mustWrite(t, filepath.Join(root, fmt.Sprintf("item-%03d.txt", index)), "x")
	}
	mustWrite(t, filepath.Join(root, "wanted-result.txt"), "x")

	first, err := manager.ListPage(context.Background(), "/", ListOptions{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 500 || first.NextOffset != 500 || first.TotalKnown {
		t.Fatalf("unexpected first page: %#v", first)
	}
	second, err := manager.ListPage(
		context.Background(),
		"/",
		ListOptions{Limit: 500, Offset: first.NextOffset},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Entries) != 121 || second.NextOffset != 0 ||
		!second.TotalKnown || second.Total != 621 {
		t.Fatalf("unexpected second page: %#v", second)
	}
	search, err := manager.ListPage(
		context.Background(),
		"/",
		ListOptions{Limit: 500, Search: "wanted"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Entries) != 1 || search.Entries[0].Name != "wanted-result.txt" {
		t.Fatalf("unexpected search result: %#v", search)
	}
}

func TestBatchActionReportsPartialResultWithoutLosingSuccesses(t *testing.T) {
	manager, root := newTestManager(t)
	mustWrite(t, filepath.Join(root, "keep.txt"), "keep")

	result, err := manager.Action(context.Background(), contract.FileActionRequest{
		Action: "trash", Sources: []string{"/keep.txt", "/missing.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Succeeded) != 1 || result.Succeeded[0].Path != "/keep.txt" {
		t.Fatalf("unexpected successes: %#v", result.Succeeded)
	}
	if len(result.Failed) != 1 || result.Failed[0].Path != "/missing.txt" {
		t.Fatalf("unexpected failures: %#v", result.Failed)
	}
	if _, err := os.Stat(filepath.Join(root, "keep.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful item was not moved: %v", err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
