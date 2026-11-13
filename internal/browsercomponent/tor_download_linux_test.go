package browsercomponent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTorDownloadIsBoundedVerifiedAndAtomic(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("Agent private files require root")
	}
	target := filepath.Join(t.TempDir(), "archive")
	content := "official fixture"
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush() // Exercise the streaming bound without Content-Length.
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()
	call := func(limit int64) error {
		return downloadTorArchive(context.Background(), server.Client(), server.URL, target, expected, limit)
	}
	if err := call(4); err == nil {
		t.Fatal("oversized archive accepted")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("partial download published", err)
	}
	if err := call(64); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	content = "modified"
	if err := call(64); err != nil {
		t.Fatal("verified cache should be reusable", err)
	}
	if err := os.WriteFile(target, []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := call(64); err == nil {
		t.Fatal("bad checksum accepted")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "damaged" {
		t.Fatal("failed download replaced existing file")
	}
	entries, _ := os.ReadDir(filepath.Dir(target))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tor-download") {
			t.Fatal("partial archive retained")
		}
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("other", target); err != nil {
		t.Fatal(err)
	}
	if err := call(64); err != ErrInvalid {
		t.Fatal("symlink accepted", err)
	}
}
