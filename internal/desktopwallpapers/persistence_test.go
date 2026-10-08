package desktopwallpapers

import (
	"bytes"
	"errors"
	"image/color"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/atomicfile"
)

func TestAddPrecommitFailuresPreserveIndexAndExistingImages(t *testing.T) {
	for failAt, stage := range []string{"image", "thumbnail", "index"} {
		t.Run(stage, func(t *testing.T) {
			store, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			prepared := mustPrepare(t, testMeta(), webPFixture(t), jpegFixture(t, 32, 32, color.Black))
			first, err := store.Add(prepared)
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(store.indexPath())
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			injected := errors.New("injected pre-commit " + stage + " sync failure")
			store.writer.SyncFile = func(file *os.File) error {
				calls++
				if calls == failAt+1 {
					return injected
				}
				return file.Sync()
			}
			added, err := store.Add(prepared)
			if !errors.Is(err, injected) || added.ID != "" {
				t.Fatalf("added=%+v error=%v", added, err)
			}
			after, err := os.ReadFile(store.indexPath())
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("index changed before commit: error=%v", err)
			}
			list, usage := store.List()
			entries, readErr := os.ReadDir(store.filesDir)
			if len(list) != 1 || list[0].ID != first.ID || usage.Count != 1 || !store.filesPresent(first) ||
				readErr != nil || len(entries) != 2 {
				t.Fatalf("old collection changed: list=%+v usage=%+v files=%v error=%v", list, usage, entries, readErr)
			}
		})
	}
}

func TestAddCommittedSyncWarningsKeepFilesAndAvoidErrorDrivenRetry(t *testing.T) {
	for failAt, stage := range []string{"image", "thumbnail", "index"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			store, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			var diagnostic bytes.Buffer
			store.logger = slog.New(slog.NewTextHandler(&diagnostic, nil))
			calls := 0
			injected := errors.New("injected post-commit " + stage + " sync failure")
			store.writer.SyncDirectory = func(path string) error {
				calls++
				if calls == failAt+1 {
					return injected
				}
				return syncDirectory(path)
			}
			prepared := mustPrepare(t, testMeta(), webPFixture(t), jpegFixture(t, 32, 32, color.Black))
			var added Wallpaper
			attempts := 0
			for range 2 {
				attempts++
				added, err = store.Add(prepared)
				if err == nil {
					break
				}
			}
			if err != nil || attempts != 1 || calls != 3 || !store.filesPresent(added) {
				t.Fatalf("added=%+v error=%v attempts=%d syncs=%d", added, err, attempts, calls)
			}
			list, usage := store.List()
			index, readErr := store.readIndex()
			if readErr != nil || len(list) != 1 || usage.Count != 1 || !reflect.DeepEqual(list, index.Wallpapers) {
				t.Fatalf("memory/index disagree: list=%+v index=%+v error=%v", list, index, readErr)
			}
			if !strings.Contains(diagnostic.String(), "committed=true") || !strings.Contains(diagnostic.String(), injected.Error()) {
				t.Fatalf("missing committed sync diagnostic: %s", diagnostic.String())
			}
			restarted, err := Open(root)
			if err != nil {
				t.Fatal(err)
			}
			reloaded, _ := restarted.List()
			if !reflect.DeepEqual(list, reloaded) || !restarted.filesPresent(added) {
				t.Fatalf("restart lost committed wallpaper: %+v", reloaded)
			}
		})
	}
}

func TestDeleteSeparatesPrecommitFailureFromCommittedSyncWarning(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	added, err := store.Add(mustPrepare(t, testMeta(), webPFixture(t), jpegFixture(t, 32, 32, color.Black)))
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(store.indexPath())
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected index sync failure")
	store.writer.SyncFile = func(*os.File) error { return injected }
	if err := store.Delete(added.ID); !errors.Is(err, injected) {
		t.Fatalf("pre-commit delete error=%v", err)
	}
	after, err := os.ReadFile(store.indexPath())
	list, _ := store.List()
	if err != nil || !bytes.Equal(before, after) || len(list) != 1 || !store.filesPresent(added) {
		t.Fatalf("pre-commit delete changed old collection: list=%+v error=%v", list, err)
	}
	store.writer.SyncFile = nil
	store.writer.SyncDirectory = func(string) error { return injected }
	var diagnostic bytes.Buffer
	store.logger = slog.New(slog.NewTextHandler(&diagnostic, nil))
	if err := store.Delete(added.ID); err != nil {
		t.Fatalf("committed delete error=%v", err)
	}
	if err := store.Delete(added.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("repeat delete=%v", err)
	}
	list, usage := store.List()
	index, err := store.readIndex()
	if err != nil || len(list) != 0 || usage.Count != 0 || len(index.Wallpapers) != 0 ||
		!strings.Contains(diagnostic.String(), injected.Error()) {
		t.Fatalf("committed delete disagrees: list=%+v index=%+v log=%s error=%v", list, index, diagnostic.String(), err)
	}
	imagePath, thumbPath := store.paths(added.ID)
	for _, path := range []string{imagePath, thumbPath} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("deleted file remains: %s: %v", path, err)
		}
	}
	restarted, err := Open(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := restarted.List(); len(list) != 0 {
		t.Fatalf("restart resurrected deleted wallpaper: %+v", list)
	}
}

func TestOpenContinuesRepairAfterCommittedIndexSyncWarning(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	prepared := mustPrepare(t, testMeta(), webPFixture(t), jpegFixture(t, 32, 32, color.Black))
	kept, err := store.Add(prepared)
	if err != nil {
		t.Fatal(err)
	}
	lost, err := store.Add(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store.filesDir, lost.ID+".thumb")); err != nil {
		t.Fatal(err)
	}
	repaired, err := open(root, atomicfile.Writer{SyncDirectory: func(string) error {
		return errors.New("injected repair directory sync failure")
	}})
	if err != nil {
		t.Fatalf("committed repair blocked startup: %v", err)
	}
	list, _ := repaired.List()
	index, err := repaired.readIndex()
	if err != nil || len(list) != 1 || list[0].ID != kept.ID || !reflect.DeepEqual(list, index.Wallpapers) {
		t.Fatalf("repair disagrees with index: list=%+v index=%+v error=%v", list, index, err)
	}
	if _, err := os.Stat(filepath.Join(store.filesDir, lost.ID+".image")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repair skipped stray cleanup: %v", err)
	}
}
