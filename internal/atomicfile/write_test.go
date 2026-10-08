package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWritePrivateSeparatesFileSyncFailureFromCommittedDirectorySyncFailure(t *testing.T) {
	for _, stage := range []string{"file", "directory", "none"} {
		t.Run(stage, func(t *testing.T) {
			directory := t.TempDir()
			target := filepath.Join(directory, "state.json")
			if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected " + stage + " sync failure")
			directorySyncs := 0
			writer := Writer{
				SyncFile: func(file *os.File) error {
					if stage == "file" {
						return injected
					}
					return file.Sync()
				},
				SyncDirectory: func(path string) error {
					directorySyncs++
					if stage == "directory" {
						return injected
					}
					return syncParentDirectory(path)
				},
			}
			result, err := writer.WritePrivate(directory, target, ".state-*", []byte("new"))
			want := "new"
			if stage == "file" {
				want = "old"
				if !errors.Is(err, injected) || result.Committed || result.DurabilityErr != nil || directorySyncs != 0 {
					t.Fatalf("pre-commit result=%+v error=%v directorySyncs=%d", result, err, directorySyncs)
				}
			} else {
				if err != nil || !result.Committed || directorySyncs != 1 || result.CleanupErr != nil {
					t.Fatalf("committed result=%+v error=%v directorySyncs=%d", result, err, directorySyncs)
				}
				if (stage == "directory") != errors.Is(result.DurabilityErr, injected) {
					t.Fatalf("directory sync diagnosis=%v", result.DurabilityErr)
				}
			}
			data, readErr := os.ReadFile(target)
			if readErr != nil || string(data) != want {
				t.Fatalf("target=%q error=%v, want %q", data, readErr, want)
			}
			entries, readErr := os.ReadDir(directory)
			if readErr != nil || len(entries) != 1 {
				t.Fatalf("temporary file not reclaimed: entries=%v error=%v", entries, readErr)
			}
			if runtime.GOOS != "windows" {
				info, statErr := os.Stat(target)
				if statErr != nil || info.Mode().Perm() != 0o600 {
					t.Fatalf("private target=%v error=%v", info, statErr)
				}
			}
		})
	}
}

func TestReplaceWindowsFallbackPreservesCommitAndRollbackOutcomes(t *testing.T) {
	for _, stage := range []string{"success", "replacement", "cleanup", "restore", "old-backup"} {
		t.Run(stage, func(t *testing.T) {
			directory := t.TempDir()
			source, target := filepath.Join(directory, "temporary"), filepath.Join(directory, "state.json")
			backup := target + ".previous"
			for path, content := range map[string]string{source: "new", target: "old"} {
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			injected := errors.New("injected " + stage + " failure")
			initial := true
			rename := func(from, to string) error {
				if from == source && to == target {
					if initial {
						initial = false
						return os.ErrExist
					}
					if stage == "replacement" || stage == "restore" {
						return injected
					}
				}
				if stage == "restore" && from == backup {
					return injected
				}
				return os.Rename(from, to)
			}
			removes := 0
			remove := func(path string) error {
				removes++
				if stage == "old-backup" || stage == "cleanup" && removes == 2 {
					return injected
				}
				return os.Remove(path)
			}
			result, err := replace(source, target, true, rename, remove)
			committed := stage == "success" || stage == "cleanup"
			if result.Committed != committed || (err == nil) != committed {
				t.Fatalf("result=%+v error=%v, committed=%t", result, err, committed)
			}
			if !committed && !errors.Is(err, injected) {
				t.Fatalf("missing failure cause: %v", err)
			}
			if (stage == "cleanup") != errors.Is(result.CleanupErr, injected) {
				t.Fatalf("cleanup diagnosis=%v", result.CleanupErr)
			}
			want, readPath := "old", target
			if committed {
				want = "new"
			} else if stage == "restore" {
				readPath = backup
			}
			data, readErr := os.ReadFile(readPath)
			if readErr != nil || string(data) != want {
				t.Fatalf("recoverable content=%q error=%v, want %q", data, readErr, want)
			}
			if stage == "success" || stage == "replacement" {
				if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("backup remains after success/restore: %v", err)
				}
			}
		})
	}
}

func TestReplaceDoesNotFallbackOutsideWindowsOrForNonregularTarget(t *testing.T) {
	for _, kind := range []string{"non-windows", "directory"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			source, target := filepath.Join(directory, "temporary"), filepath.Join(directory, "target")
			if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
				t.Fatal(err)
			}
			if kind == "directory" {
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			injected := errors.New("rename refused")
			result, err := replace(source, target, kind == "directory", func(string, string) error {
				calls++
				return injected
			}, func(string) error {
				t.Fatal("unexpected backup removal")
				return nil
			})
			if !errors.Is(err, injected) || result.Committed || calls != 1 {
				t.Fatalf("result=%+v error=%v calls=%d", result, err, calls)
			}
			if _, err := os.Stat(target); err != nil {
				t.Fatalf("original target changed: %v", err)
			}
		})
	}
}
