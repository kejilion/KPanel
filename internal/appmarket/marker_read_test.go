package appmarket

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkerReadSupportsRootLinkButRejectsLeafLinks(t *testing.T) {
	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real")
	if err := os.Mkdir(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "appno.txt"), []byte("28\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rootLink := filepath.Join(parent, "link")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if markers, warning := (&Service{appRoot: rootLink}).readMarkers(); warning != "" || !markers["28"] {
		t.Fatalf("symlinked root was not read: %v %q", markers, warning)
	}
	leafRoot := filepath.Join(parent, "leaf")
	if err := os.Mkdir(leafRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(realRoot, "appno.txt"), filepath.Join(leafRoot, "appno.txt")); err != nil {
		t.Fatal(err)
	}
	if markers, warning := (&Service{appRoot: leafRoot}).readMarkers(); warning == "" || len(markers) != 0 {
		t.Fatalf("symlinked marker was accepted: %v %q", markers, warning)
	}
	internalRoot := filepath.Join(parent, "internal")
	if err := os.Mkdir(internalRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(internalRoot, "real.txt"), []byte("28\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.txt", filepath.Join(internalRoot, "appno.txt")); err != nil {
		t.Fatal(err)
	}
	if markers, warning := (&Service{appRoot: internalRoot}).readMarkers(); warning == "" || len(markers) != 0 {
		t.Fatalf("internal symlinked marker was accepted: %v %q", markers, warning)
	}
}

func TestMarkerReadLimitsTotalBytes(t *testing.T) {
	if _, err := readMarkerContent(strings.NewReader(strings.Repeat("28\n", maxAppMarkersBytes))); err == nil {
		t.Fatal("oversized marker stream accepted")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "appno.txt"), []byte("28\n64\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	markers, warning := (&Service{appRoot: root}).readMarkers()
	if warning != "" || !markers["28"] || !markers["64"] {
		t.Fatalf("ordinary markers: %v %q", markers, warning)
	}
}
