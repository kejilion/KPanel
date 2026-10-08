//go:build linux

package filemanager

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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
	for _, replacement := range []string{"stage", "parent"} {
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
				if replacement == "stage" {
					if err := os.Rename(filepath.Join(root, "home", ".kpanel-extract-test"), filepath.Join(root, "home", "moved-stage")); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(filepath.Join(root, "protected"), filepath.Join(root, "home", ".kpanel-extract-test")); err != nil {
						t.Fatal(err)
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
			_, err = m.importDirectory(context.Background(), input, content, parent, "/home/.kpanel-extract-test", func(*fileRoot, os.FileInfo) error { return nil })
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("substituted extraction was published: %v", err)
			}
			entries, err := os.ReadDir(filepath.Join(root, "protected"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("protected target modified: %v %v", entries, err)
			}
		})
	}
}
