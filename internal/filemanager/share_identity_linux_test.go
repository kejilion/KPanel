//go:build linux

package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestShareVersionDetectsSameMetadataRewriteAndReplacement(t *testing.T) {
	manager, root := newTestManager(t)
	filePath := filepath.Join(root, "shared.txt")
	mustWrite(t, filePath, "before")
	before, err := manager.ShareEntry(context.Background(), "/shared.txt")
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filePath, []byte("after!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filePath, before.ModifiedAt, before.ModifiedAt); err != nil {
		t.Fatal(err)
	}
	afterRewrite, err := manager.ShareEntry(context.Background(), "/shared.txt")
	if err != nil {
		t.Fatal(err)
	}
	if afterRewrite.ResourceVersion != before.ResourceVersion {
		t.Fatalf("fixture did not preserve the ordinary metadata version: before=%q after=%q", before.ResourceVersion, afterRewrite.ResourceVersion)
	}
	if afterRewrite.ShareVersion == before.ShareVersion {
		t.Fatal("share version did not detect an in-place rewrite with restored mtime")
	}

	replacementPath := filepath.Join(root, "replacement.txt")
	if err := os.WriteFile(replacementPath, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacementPath, before.ModifiedAt, before.ModifiedAt); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacementPath, filePath); err != nil {
		t.Fatal(err)
	}
	afterReplacement, err := manager.ShareEntry(context.Background(), "/shared.txt")
	if err != nil {
		t.Fatal(err)
	}
	if afterReplacement.ResourceVersion != before.ResourceVersion {
		t.Fatalf("replacement fixture did not preserve ordinary metadata version: before=%q after=%q", before.ResourceVersion, afterReplacement.ResourceVersion)
	}
	if afterReplacement.ShareVersion == before.ShareVersion {
		t.Fatal("share version did not detect a same-metadata inode replacement")
	}
}
