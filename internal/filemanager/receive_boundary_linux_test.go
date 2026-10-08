//go:build linux

package filemanager

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type receiveBoundaryReader struct {
	io.Reader
	once    bool
	replace func()
}

func (r *receiveBoundaryReader) Read(data []byte) (int, error) {
	if !r.once {
		r.once = true
		r.replace()
	}
	return r.Reader.Read(data)
}

func TestReceiveDirectoryPinsStageAndParentDuringExtraction(t *testing.T) {
	for _, replacement := range []string{"stage", "stage-directory", "parent"} {
		t.Run(replacement, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"home", "protected"} {
				if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			m, err := New(Config{Root: root, ProtectedVirtual: []string{"/protected"}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = m.Close() })
			input := contract.FileReceiveInput{Directory: "/home", Name: "result", Kind: "directory", SizeBytes: -1}
			parent, _, err := m.openReceiveDirectory("/home", false)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			var archive bytes.Buffer
			writer := tar.NewWriter(&archive)
			if err := writer.WriteHeader(&tar.Header{Name: "owned.txt", Mode: 0644, Size: 7, Typeflag: tar.TypeReg}); err != nil {
				t.Fatal(err)
			}
			_, _ = writer.Write([]byte("payload"))
			_ = writer.Close()
			content := &receiveBoundaryReader{Reader: bytes.NewReader(archive.Bytes()), replace: func() {
				if replacement == "stage" || replacement == "stage-directory" {
					if err := os.Rename(filepath.Join(root, "home", ".kpanel-extract-test"), filepath.Join(root, "home", "moved-stage")); err != nil {
						t.Fatal(err)
					}
					if replacement == "stage" {
						if err := os.Symlink(filepath.Join(root, "protected"), filepath.Join(root, "home", ".kpanel-extract-test")); err != nil {
							t.Fatal(err)
						}
					} else {
						if err := os.Mkdir(filepath.Join(root, "home", ".kpanel-extract-test"), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(root, "home", ".kpanel-extract-test", "unrelated.txt"), []byte("retain"), 0600); err != nil {
							t.Fatal(err)
						}
					}
				} else {
					if err := os.Rename(filepath.Join(root, "home"), filepath.Join(root, "moved-parent")); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(filepath.Join(root, "protected"), filepath.Join(root, "home")); err != nil {
						t.Fatal(err)
					}
				}
			}}
			_, err = m.importDirectory(context.Background(), input, content, parent, "/home/.kpanel-extract-test", func(os.FileInfo) error { return nil }, func(*fileRoot, os.FileInfo) error { return nil })
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("substituted extraction was published: %v", err)
			}
			entries, err := os.ReadDir(filepath.Join(root, "protected"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("protected target modified: %v %v", entries, err)
			}
			if replacement == "stage-directory" {
				for _, retained := range []string{".kpanel-extract-test/unrelated.txt", "moved-stage/result/owned.txt"} {
					if _, err := os.Stat(filepath.Join(root, "home", filepath.FromSlash(retained))); err != nil {
						t.Fatalf("unowned replacement or moved stage removed: %s: %v", retained, err)
					}
				}
			}
		})
	}
}

type receiveMutationContext struct {
	context.Context
	check func() error
}

func (c receiveMutationContext) Err() error { return c.check() }

func TestReceiveFileStagingSubstitutionNeverPublishesOrOverwrites(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		for _, replacement := range []string{"file", "symlink", "publication-directory"} {
			t.Run(fmt.Sprintf("overwrite=%t/%s", overwrite, replacement), func(t *testing.T) {
				root := t.TempDir()
				m := newReceiveManager(t, root)
				input := testReceiveInput(7)
				input.Overwrite = overwrite
				if overwrite {
					if err := os.WriteFile(filepath.Join(root, input.Name), []byte("original"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				session, err := m.BeginReceive(context.Background(), input)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.WriteReceiveChunk(context.Background(), session.ID, input.SourceKey, 0, strings.NewReader("payload"), receiveDigest([]byte("payload"))); err != nil {
					t.Fatal(err)
				}
				once := false
				ctx := receiveMutationContext{Context: context.Background(), check: func() error {
					record := m.receives.records[session.ID]
					if once || record.Session.State != "committing" || record.PublishIdentity == "" {
						return nil
					}
					once = true
					name := ".kpanel-upload-" + session.ID
					if replacement == "publication-directory" {
						name = ".kpanel-extract-" + session.ID
					}
					if err := os.Rename(filepath.Join(root, name), filepath.Join(root, "moved-owned")); err != nil {
						t.Fatal(err)
					}
					if replacement == "symlink" {
						if err := os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("unrelated"), 0600); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(filepath.Join(root, "unrelated.txt"), filepath.Join(root, name)); err != nil {
							t.Fatal(err)
						}
					} else if replacement == "publication-directory" {
						if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(root, name, "body"), []byte("unrelated"), 0600); err != nil {
							t.Fatal(err)
						}
					} else if err := os.WriteFile(filepath.Join(root, name), []byte("unrelated"), 0600); err != nil {
						t.Fatal(err)
					}
					return nil
				}}
				if _, err := m.CommitReceive(ctx, session.ID, input.SourceKey, 7, receiveDigest([]byte("payload"))); !errors.Is(err, ErrConflict) {
					t.Fatalf("substitution accepted: %v", err)
				}
				if !once {
					t.Fatal("publish window not exercised")
				}
				actual, err := os.ReadFile(filepath.Join(root, input.Name))
				if overwrite {
					if err != nil || string(actual) != "original" {
						t.Fatalf("original overwritten: %q %v", actual, err)
					}
				} else if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("unverified target published: %q %v", actual, err)
				}
				if _, err := os.Stat(filepath.Join(root, "moved-owned")); err != nil {
					t.Fatal("moved owned object removed", err)
				}
			})
		}
	}
}

func TestReceiveRecoveryProvesPrivatePublicationBeforeRemovingSpool(t *testing.T) {
	for _, kind := range []string{"file", "directory"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			m := newReceiveManager(t, root)
			data := []byte("payload")
			if kind == "directory" {
				var archive bytes.Buffer
				writer := tar.NewWriter(&archive)
				_ = writer.WriteHeader(&tar.Header{Name: "child.txt", Mode: 0644, Size: 7, Typeflag: tar.TypeReg})
				_, _ = writer.Write(data)
				_ = writer.Close()
				data = archive.Bytes()
			}
			input := testReceiveInput(int64(len(data)))
			input.Kind = kind
			session, err := m.BeginReceive(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.WriteReceiveChunk(context.Background(), session.ID, input.SourceKey, 0, bytes.NewReader(data), receiveDigest(data)); err != nil {
				t.Fatal(err)
			}
			once := false
			ctx := receiveMutationContext{Context: context.Background(), check: func() error {
				record := m.receives.records[session.ID]
				if once || record.Session.State != "committing" || record.PublishIdentity == "" {
					return nil
				}
				once = true
				child := "body"
				if kind == "directory" {
					child = "result"
				}
				if err := os.Rename(filepath.Join(root, ".kpanel-extract-"+session.ID, child), filepath.Join(root, input.Name)); err != nil {
					t.Fatal(err)
				}
				return context.Canceled
			}}
			if _, err := m.CommitReceive(ctx, session.ID, input.SourceKey, int64(len(data)), receiveDigest(data)); !errors.Is(err, context.Canceled) {
				t.Fatalf("crash window not hit: %v", err)
			}
			_ = m.Close()
			m = newReceiveManager(t, root)
			resumed, err := m.ReceiveStatus(context.Background(), session.ID, input.SourceKey)
			if err != nil || resumed.State != "complete" || resumed.Entry == nil {
				t.Fatalf("published receive lost: %#v %v", resumed, err)
			}
		})
	}
}

func TestReceiveCleanupRetainsUnownedObjectsAfterRestart(t *testing.T) {
	for _, staging := range []string{"spool", "extract"} {
		t.Run(staging, func(t *testing.T) {
			root := t.TempDir()
			m := newReceiveManager(t, root)
			input := testReceiveInput(0)
			session, err := m.BeginReceive(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			record := m.receives.records[session.ID]
			name := ".kpanel-upload-" + session.ID
			if staging == "extract" {
				name = ".kpanel-extract-" + session.ID
				if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(filepath.Join(root, name))
				if err != nil {
					t.Fatal(err)
				}
				record.ExtractIdentity = stableReceiveIdentity(info)
				if err := m.persistReceivesLocked(); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Rename(filepath.Join(root, name), filepath.Join(root, "moved-owned")); err != nil {
				t.Fatal(err)
			}
			retained := filepath.Join(root, name)
			if staging == "extract" {
				if err := os.Mkdir(retained, 0700); err != nil {
					t.Fatal(err)
				}
				retained = filepath.Join(retained, "unrelated.txt")
			}
			if err := os.WriteFile(retained, []byte("retain"), 0600); err != nil {
				t.Fatal(err)
			}
			_ = m.Close()
			m = newReceiveManager(t, root)
			if err := m.AbortReceive(context.Background(), session.ID, input.SourceKey); !errors.Is(err, ErrConflict) {
				t.Fatalf("unowned cleanup accepted: %v", err)
			}
			actual, err := os.ReadFile(retained)
			if err != nil || string(actual) != "retain" {
				t.Fatalf("unowned data removed: %q %v", actual, err)
			}
			if _, err := os.Stat(filepath.Join(root, "moved-owned")); err != nil {
				t.Fatal("moved object removed", err)
			}
		})
	}
}
