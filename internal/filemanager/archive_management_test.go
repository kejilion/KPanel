package filemanager

import (
	"archive/tar"
	"archive/zip"
	"context"
	"encoding/binary"
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

func makeArchiveFixture(t *testing.T, m *Manager, root, format string) contract.FileEntry {
	t.Helper()
	mustWrite(t, filepath.Join(root, "project", "assets", "app.js"), "script")
	mustWrite(t, filepath.Join(root, "project", "index.html"), "hello")
	source, err := m.Stat("/project")
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Action(context.Background(), contract.FileActionRequest{Action: "compress", Sources: []string{source.Path}, Target: "/", Name: "project." + format, Format: format, ExpectedResourceVersions: map[string]string{source.Path: source.ResourceVersion}})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := m.Stat(result.Succeeded[0].Destination)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestArchiveBrowseAndSelectedExtraction(t *testing.T) {
	for _, format := range []string{"zip", "tar", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			m, root := newTestManager(t)
			source := makeArchiveFixture(t, m, root, format)
			query := contract.FileArchiveQuery{Path: source.Path, ResourceVersion: source.ResourceVersion}
			listing, err := m.ArchiveContents(context.Background(), query)
			if err != nil || len(listing.Entries) != 2 || listing.Entries[0].Path != "assets" {
				t.Fatalf("listing=%+v err=%v", listing, err)
			}
			query.Directory = ""
			listing, err = m.ArchiveContents(context.Background(), query)
			if err != nil || len(listing.Entries) != 2 || listing.Entries[0].Kind != "directory" {
				t.Fatalf("children=%+v err=%v", listing, err)
			}
			query.Search = "APP.JS"
			listing, err = m.ArchiveContents(context.Background(), query)
			if err != nil || len(listing.Entries) != 1 || listing.Entries[0].Path != "assets/app.js" {
				t.Fatalf("search=%+v err=%v", listing, err)
			}
			input := contract.FileActionRequest{Action: "extract", Sources: []string{source.Path}, Target: "/", Name: "selected", Format: format, ExpectedResourceVersion: source.ResourceVersion, ArchiveEntries: []string{"assets"}}
			if _, err := m.Action(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			content, err := os.ReadFile(filepath.Join(root, "selected", "assets", "app.js"))
			if err != nil || string(content) != "script" {
				t.Fatalf("selected content=%q err=%v", content, err)
			}
			if _, err := os.Stat(filepath.Join(root, "selected", "index.html")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unselected file published: %v", err)
			}
			input.Name = "missing"
			input.ArchiveEntries = []string{"absent"}
			if _, err := m.Action(context.Background(), input); !errors.Is(err, ErrConflict) {
				t.Fatalf("missing selection: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("missing selection published")
			}
			mustWrite(t, filepath.Join(root, strings.TrimPrefix(source.Path, "/")), "changed")
			if _, err := m.ArchiveContents(context.Background(), query); !errors.Is(err, ErrConflict) {
				t.Fatalf("stale cached contents: %v", err)
			}
		})
	}
}

func TestArchiveBrowsePaginationAndBounds(t *testing.T) {
	m, root := newTestManager(t)
	file, err := os.Create(filepath.Join(root, "many.zip"))
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(file)
	for i := 0; i < 501; i++ {
		if _, err := w.Create(fmt.Sprintf("%03d.txt", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	source, _ := m.Stat("/many.zip")
	query := contract.FileArchiveQuery{Path: source.Path, ResourceVersion: source.ResourceVersion}
	listing, err := m.ArchiveContents(context.Background(), query)
	if err != nil || len(listing.Entries) != 500 || !listing.Truncated || listing.NextOffset != 500 || listing.Total != 501 {
		t.Fatalf("page=%+v err=%v", listing, err)
	}
	query.Offset = 500
	listing, err = m.ArchiveContents(context.Background(), query)
	if err != nil || len(listing.Entries) != 1 || listing.Truncated {
		t.Fatalf("last page=%+v err=%v", listing, err)
	}
	m.archiveIndex.key = ""
	m.maxCopyEntries = 500
	query.Offset = 0
	if _, err = m.ArchiveContents(context.Background(), query); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("entry cap=%v", err)
	}
}

func TestArchiveSelectionStillValidatesUnsafeUnselectedEntries(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "ok/../../escape", "ok/hidden"} {
		t.Run(name, func(t *testing.T) {
			m, root := newTestManager(t)
			mode := os.FileMode(0644)
			if name == "ok/hidden" {
				mode = os.ModeSymlink | 0777
			}
			writeZIPFixture(t, filepath.Join(root, "bad.zip"), name, mode, "unsafe")
			source, _ := m.Stat("/bad.zip")
			if _, err := m.ArchiveContents(context.Background(), contract.FileArchiveQuery{Path: source.Path, ResourceVersion: source.ResourceVersion}); !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("unsafe browse=%v", err)
			}
			_, err := m.Action(context.Background(), contract.FileActionRequest{Action: "extract", Sources: []string{source.Path}, Target: "/", Name: "out", Format: "zip", ArchiveEntries: []string{"safe"}, ExpectedResourceVersion: source.ResourceVersion})
			if !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("unsafe selection=%v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "out")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unsafe output published")
			}
		})
	}
}

func TestArchiveStandardTARRootIsBounded(t *testing.T) {
	m, root := newTestManager(t)
	file, err := os.Create(filepath.Join(root, "root.tar"))
	if err != nil {
		t.Fatal(err)
	}
	w := tar.NewWriter(file)
	for i := 0; i < 3; i++ {
		if err := w.WriteHeader(&tar.Header{Name: "./", Mode: 0755, Typeflag: tar.TypeDir}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.WriteHeader(&tar.Header{Name: "./hello.txt", Size: 5, Mode: 0644}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	source, _ := m.Stat("/root.tar")
	query := contract.FileArchiveQuery{Path: source.Path, ResourceVersion: source.ResourceVersion}
	listing, err := m.ArchiveContents(context.Background(), query)
	if err != nil || len(listing.Entries) != 1 || listing.Entries[0].Path != "hello.txt" {
		t.Fatalf("standard tar=%+v %v", listing, err)
	}
	m.archiveIndex.key = ""
	m.maxCopyEntries = 2
	if _, err := m.ArchiveContents(context.Background(), query); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("root headers bypassed cap: %v", err)
	}
}

func waitArchiveJob(t *testing.T, m *Manager, id string) contract.FileArchiveJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		items, err := m.ArchiveJobs()
		if err != nil {
			t.Fatal(err)
		}
		for _, job := range items {
			if job.ID == id && !archiveJobActive(job.State) {
				return job
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("archive job did not finish")
	return contract.FileArchiveJob{}
}

func TestArchiveJobsPersistPartialResultsAndNeverOverwrite(t *testing.T) {
	m, root := newTestManager(t)
	writeZIPFixture(t, filepath.Join(root, "one.zip"), "a.txt", 0644, "hello")
	writeZIPFixture(t, filepath.Join(root, "two.zip"), "b.txt", 0644, "world")
	mustWrite(t, filepath.Join(root, "two", "keep.txt"), "keep")
	one, _ := m.Stat("/one.zip")
	two, _ := m.Stat("/two.zip")
	job, err := m.StartArchiveJob(contract.FileActionRequest{Action: "extract", Sources: []string{one.Path, two.Path}, Target: "/", Format: "tar.gz", ExpectedResourceVersions: map[string]string{one.Path: one.ResourceVersion, two.Path: two.ResourceVersion}})
	if err != nil {
		t.Fatal(err)
	}
	if job.Format != "" {
		t.Fatalf("batch extract persisted unused format %q", job.Format)
	}
	done := waitArchiveJob(t, m, job.ID)
	if done.State != "partial" || len(done.Result.Succeeded) != 1 || len(done.Result.Failed) != 1 || done.Result.Failed[0].Path != two.Path {
		t.Fatalf("partial result=%+v", done)
	}
	if content, err := os.ReadFile(filepath.Join(root, "two", "keep.txt")); err != nil || string(content) != "keep" {
		t.Fatalf("existing file changed: %q %v", content, err)
	}
	m.StopArchiveJobs()
	recovered, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	items, err := recovered.ArchiveJobs()
	if err != nil || len(items) != 1 || items[0].State != "partial" {
		t.Fatalf("reload=%+v %v", items, err)
	}
}

func TestArchiveJobCancelWhileMutationIsBusy(t *testing.T) {
	m, root := newTestManager(t)
	source := makeArchiveFixture(t, m, root, "zip")
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	job, err := m.StartArchiveJob(contract.FileActionRequest{Action: "extract", Sources: []string{source.Path}, Target: "/", Name: "cancelled", Format: "tar.gz", ExpectedResourceVersion: source.ResourceVersion})
	if err != nil {
		t.Fatal(err)
	}
	if job.Format != "zip" {
		t.Fatalf("single extract format=%q, want zip", job.Format)
	}
	if err := m.ChangeArchiveJob(job.ID, "cancel"); err != nil {
		t.Fatal(err)
	}
	done := waitArchiveJob(t, m, job.ID)
	if done.State != "cancelled" {
		t.Fatalf("cancelled=%+v", done)
	}
	if _, err := os.Stat(filepath.Join(root, "cancelled")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled job published output")
	}
	if err := m.ChangeArchiveJob(job.ID, "cancel"); !errors.Is(err, ErrConflict) {
		t.Fatalf("terminal task cancel=%v", err)
	}
}

func TestArchiveBatchSharesByteBudget(t *testing.T) {
	m, root := newTestManager(t)
	m.maxCopyBytes = 7
	versions := map[string]string{}
	for _, name := range []string{"one", "two"} {
		writeZIPFixture(t, filepath.Join(root, name+".zip"), "file.txt", 0644, "hello")
		source, _ := m.Stat("/" + name + ".zip")
		versions[source.Path] = source.ResourceVersion
	}
	job, err := m.StartArchiveJob(contract.FileActionRequest{Action: "extract", Sources: []string{"/one.zip", "/two.zip"}, Target: "/", ExpectedResourceVersions: versions})
	if err != nil {
		t.Fatal(err)
	}
	done := waitArchiveJob(t, m, job.ID)
	if done.State != "partial" || len(done.Result.Failed) != 1 {
		t.Fatalf("batch bypassed shared cap=%+v", done)
	}
	if _, err := os.Stat(filepath.Join(root, "two")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("over-budget output published")
	}
}

func TestArchiveRecoveryOnlyCleansJournalOwnedTemps(t *testing.T) {
	m, root := newTestManager(t)
	id := strings.Repeat("a", 32)
	temp := archiveJobTemp("/", "extract", id, 0)
	mustWrite(t, filepath.Join(root, strings.TrimPrefix(temp, "/"), "partial"), "temporary")
	mustWrite(t, filepath.Join(root, ".kpanel-extract-unrelated", "keep"), "keep")
	mustWrite(t, filepath.Join(root, "result", "keep"), "committed")
	record := archiveJobRecord{Job: contract.FileArchiveJob{ID: id, Action: "extract", State: "running", Target: "/", Sources: []string{"/source.zip"}, Name: "result", CreatedAt: time.Now(), UpdatedAt: time.Now()}, Temps: []string{temp}}
	data, err := json.Marshal(map[string]any{"version": 1, "records": []archiveJobRecord{record}})
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, strings.TrimPrefix(m.archiveStatePath(), "/")), string(data))
	items, err := m.ArchiveJobs()
	if err != nil || len(items) != 1 || items[0].State != "interrupted" {
		t.Fatalf("recovery=%+v %v", items, err)
	}
	if _, err := os.Stat(filepath.Join(root, strings.TrimPrefix(temp, "/"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned temp remains")
	}
	for _, name := range []string{".kpanel-extract-unrelated/keep", "result/keep"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("non-temp removed: %s %v", name, err)
		}
	}
}

func TestArchiveStateCorruptionAndSymlinkFailClosed(t *testing.T) {
	for _, mode := range []string{"invalid-json", "symlink", "forged-temp"} {
		t.Run(mode, func(t *testing.T) {
			m, root := newTestManager(t)
			state := filepath.Join(root, strings.TrimPrefix(m.archiveStatePath(), "/"))
			mustWrite(t, state, "invalid")
			if mode == "symlink" {
				if err := os.Remove(state); err != nil {
					t.Fatal(err)
				}
				mustWrite(t, filepath.Join(root, "victim"), `{"version":1,"records":[]}`)
				if err := os.Symlink(filepath.Join(root, "victim"), state); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "forged-temp" {
				j := contract.FileArchiveJob{ID: strings.Repeat("a", 32), Action: "extract", State: "running", Target: "/", Sources: []string{"/source.zip"}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
				payload, _ := json.Marshal(map[string]any{"version": 1, "records": []archiveJobRecord{{Job: j, Temps: []string{"/victim"}}}})
				mustWrite(t, state, string(payload))
				mustWrite(t, filepath.Join(root, "victim"), "keep")
			}
			if _, err := m.ArchiveJobs(); !errors.Is(err, ErrArchiveJobsUnavailable) {
				t.Fatalf("corrupt state=%v", err)
			}
			if mode != "invalid-json" {
				if _, err := os.Stat(filepath.Join(root, "victim")); err != nil {
					t.Fatal("victim removed")
				}
			}
		})
	}
}

func TestZIPDirectoryCannotLieAboutSizeOrEntryCount(t *testing.T) {
	for _, field := range []int{8, 12, 16} {
		t.Run(fmt.Sprint(field), func(t *testing.T) {
			m, root := newTestManager(t)
			writeZIPFixture(t, filepath.Join(root, "bad.zip"), "hello.txt", 0644, "hello")
			data, err := os.ReadFile(filepath.Join(root, "bad.zip"))
			if err != nil {
				t.Fatal(err)
			}
			end := len(data) - 22
			if field == 8 {
				binary.LittleEndian.PutUint16(data[end+8:], 0)
				binary.LittleEndian.PutUint16(data[end+10:], 0)
			} else {
				binary.LittleEndian.PutUint32(data[end+field:], 1)
			}
			if err := os.WriteFile(filepath.Join(root, "bad.zip"), data, 0600); err != nil {
				t.Fatal(err)
			}
			source, _ := m.Stat("/bad.zip")
			if _, err := m.ArchiveContents(context.Background(), contract.FileArchiveQuery{Path: source.Path, ResourceVersion: source.ResourceVersion}); !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("forged directory accepted: %v", err)
			}
		})
	}
}

func TestArchiveJobsRejectNonCanonicalSourcesBeforePersistence(t *testing.T) {
	m, root := newTestManager(t)
	source := makeArchiveFixture(t, m, root, "zip")
	_, err := m.StartArchiveJob(contract.FileActionRequest{Action: "extract", Sources: []string{"//project.zip"}, Target: "/", Name: "out", ExpectedResourceVersion: source.ResourceVersion})
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("noncanonical source=%v", err)
	}
	if _, err := m.ArchiveJobs(); err != nil {
		t.Fatalf("invalid request poisoned journal: %v", err)
	}
}

func TestZIPDirectoryOf65535BytesIsAccepted(t *testing.T) {
	m, root := newTestManager(t)
	file, err := os.Create(filepath.Join(root, "large-directory.zip"))
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(file)
	// A classic ZIP central directory may legitimately be exactly 65,535 bytes.
	// A ZIP64-looking byte sequence inside its comment must remain ordinary data.
	comment := make([]byte, 0xffff-46-len("file"))
	copy(comment[len(comment)-20:], []byte{'P', 'K', 6, 7})
	entry, err := w.CreateHeader(&zip.FileHeader{Name: "file", Method: zip.Store, Comment: string(comment)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("test")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "large-directory.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if size := binary.LittleEndian.Uint32(data[len(data)-22+12:]); size != 0xffff {
		t.Fatalf("fixture size=%d", size)
	}
	source, _ := m.Stat("/large-directory.zip")
	listing, err := m.ArchiveContents(context.Background(), contract.FileArchiveQuery{Path: source.Path, ResourceVersion: source.ResourceVersion})
	if err != nil || len(listing.Entries) != 1 || listing.Entries[0].Path != "file" {
		t.Fatalf("browse large classic directory=%+v err=%v", listing, err)
	}
	if _, err := m.Action(context.Background(), contract.FileActionRequest{
		Action: "extract", Sources: []string{source.Path}, Target: "/", Name: "large-directory",
		Format: "zip", ExpectedResourceVersion: source.ResourceVersion,
	}); err != nil {
		t.Fatalf("extract large classic directory: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(root, "large-directory", "file"))
	if err != nil || string(content) != "test" {
		t.Fatalf("extracted content=%q err=%v", content, err)
	}
}

func TestZIP64EndRecordSentinelsAreRejected(t *testing.T) {
	for _, field := range []string{"records", "size", "offset"} {
		t.Run(field, func(t *testing.T) {
			m, root := newTestManager(t)
			writeZIPFixture(t, filepath.Join(root, "zip64.zip"), "hello.txt", 0644, "hello")
			data, err := os.ReadFile(filepath.Join(root, "zip64.zip"))
			if err != nil {
				t.Fatal(err)
			}
			end := len(data) - 22
			switch field {
			case "records":
				binary.LittleEndian.PutUint16(data[end+8:], 0xffff)
				binary.LittleEndian.PutUint16(data[end+10:], 0xffff)
			case "size":
				binary.LittleEndian.PutUint32(data[end+12:], 0xffffffff)
			case "offset":
				binary.LittleEndian.PutUint32(data[end+16:], 0xffffffff)
			}
			if err := os.WriteFile(filepath.Join(root, "zip64.zip"), data, 0600); err != nil {
				t.Fatal(err)
			}
			source, _ := m.Stat("/zip64.zip")
			if _, err := m.ArchiveContents(context.Background(), contract.FileArchiveQuery{Path: source.Path, ResourceVersion: source.ResourceVersion}); !errors.Is(err, ErrInvalidArchive) {
				t.Fatalf("ZIP64 %s sentinel accepted: %v", field, err)
			}
		})
	}
}

func TestZIPCommentCannotSelectAnUnvalidatedDirectory(t *testing.T) {
	m, root := newTestManager(t)
	file, err := os.Create(filepath.Join(root, "comment.zip"))
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(file)
	entry, err := w.Create("hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	// Standard archive/zip accepts a later EOCD with trailing bytes. This one
	// claims an empty archive and must not override the verified real directory.
	comment := make([]byte, 23)
	copy(comment, []byte{'P', 'K', 5, 6})
	comment[22] = 'x'
	if err := w.SetComment(string(comment)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	source, _ := m.Stat("/comment.zip")
	listing, err := m.ArchiveContents(context.Background(), contract.FileArchiveQuery{Path: source.Path, ResourceVersion: source.ResourceVersion})
	if err != nil || len(listing.Entries) != 1 || listing.Entries[0].Path != "hello.txt" {
		t.Fatalf("comment changed validated directory: %+v %v", listing, err)
	}
}
