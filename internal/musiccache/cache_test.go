package musiccache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func ref() Ref { return Ref{Path: "/music/song.mp3", ResourceVersion: "v1", SizeBytes: 1234} }
func openTest(t *testing.T) *Cache {
	t.Helper()
	c, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestDurableMergeRestartVersionAndCorruption(t *testing.T) {
	c := openTest(t)
	r := ref()
	no := false
	if err := c.Put(r, Info{Tags: &Tags{Title: "歌曲", Artist: "歌手", Album: "专辑"}, EmbeddedAssets: &no}); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(r, Info{Quality: &Quality{Bitrate: 320, BitrateKind: "frame", VBR: true}}); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	items, err := restarted.Lookup([]Ref{r})
	if err != nil || len(items) != 1 {
		t.Fatalf("lookup=%v %v", items, err)
	}
	got := items[0]
	if got.Info.Tags.Title != "歌曲" || got.Info.Quality.Bitrate != 320 || *got.Info.EmbeddedAssets || got.Source != "file-header" || got.Schema != 1 || got.DetectedAt.IsZero() {
		t.Fatalf("record=%+v", got)
	}
	for _, other := range []Ref{{Path: r.Path, ResourceVersion: "v2", SizeBytes: r.SizeBytes}, {HostID: "other", Path: r.Path, ResourceVersion: r.ResourceVersion, SizeBytes: r.SizeBytes}, {Path: r.Path, ResourceVersion: r.ResourceVersion, SizeBytes: 4321}} {
		items, err := restarted.Lookup([]Ref{other})
		if err != nil || len(items) != 0 {
			t.Fatalf("stale cache=%v %v", items, err)
		}
	}
	if err := os.WriteFile(filepath.Join(c.dir, key(r)), []byte(`{broken`), 0600); err != nil {
		t.Fatal(err)
	}
	items, err = restarted.Lookup([]Ref{r})
	if err != nil || len(items) != 0 {
		t.Fatalf("corruption=%v %v", items, err)
	}
	if err := restarted.Put(r, Info{Quality: &Quality{Bitrate: 192}}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(c.dir, key(r)))
		root, _ := os.Stat(c.dir)
		if st.Mode().Perm() != 0600 || root.Mode().Perm() != 0700 {
			t.Fatal("private modes")
		}
	}
	restarted.now = func() time.Time { return time.Now().Add(Retention + time.Hour) }
	items, err = restarted.Lookup([]Ref{r})
	if err != nil || len(items) != 0 {
		t.Fatalf("expiry=%v %v", items, err)
	}
}
func TestBoundsAndWriteFailure(t *testing.T) {
	c := openTest(t)
	r := ref()
	for _, bad := range []Info{{}, {Tags: &Tags{Title: strings.Repeat("x", 1025)}}, {Quality: &Quality{Bitrate: -1}}, {Quality: &Quality{BitrateKind: "invented"}}} {
		if c.Put(r, bad) == nil {
			t.Fatal("accepted invalid observation")
		}
	}
	r.ResourceVersion = ""
	if c.Put(r, Info{Tags: &Tags{}}) == nil {
		t.Fatal("accepted unversioned file")
	}
	r = ref()
	c.writer.SyncFile = func(*os.File) error { return errors.New("disk full") }
	if c.Put(r, Info{Tags: &Tags{Title: "failed"}}) == nil {
		t.Fatal("missing write error")
	}
	if len(c.files) != 0 || c.bytes != 0 {
		t.Fatal("failed write became cached")
	}
	files, _ := os.ReadDir(c.dir)
	if len(files) != 0 {
		t.Fatal("temporary leak")
	}
}
func TestEvictionAndConcurrentAccess(t *testing.T) {
	c := openTest(t)
	for i := 0; i < MaxRecords; i++ {
		r := ref()
		r.Path = fmt.Sprintf("/music/%d.mp3", i)
		name := key(r)
		if err := os.WriteFile(filepath.Join(c.dir, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		c.files[name] = diskEntry{2, time.Now().Add(-time.Hour)}
		c.bytes += 2
	}
	if err := c.Put(ref(), Info{Tags: &Tags{Title: "new"}}); err != nil {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(c.dir)
	if len(files) != MaxRecords || len(c.files) != MaxRecords || c.bytes > MaxBytes {
		t.Fatal("count quota")
	}
	// The index is reconstructed from disk after a restart; no unbounded in-memory catalogue.
	restarted, err := Open(c.dir)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				if _, err := restarted.Lookup([]Ref{ref()}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}
func TestRejectSymlinksAndReplacedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux filesystem boundary")
	}
	c := openTest(t)
	r := ref()
	outside := filepath.Join(t.TempDir(), "private")
	os.WriteFile(outside, []byte("secret"), 0600)
	if err := os.Symlink(outside, filepath.Join(c.dir, key(r))); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(c.dir); err == nil {
		t.Fatal("symlink accepted")
	}
	os.Remove(filepath.Join(c.dir, key(r)))
	os.Rename(c.dir, c.dir+"-previous")
	os.Symlink(filepath.Dir(outside), c.dir)
	if err := c.Put(r, Info{Tags: &Tags{}}); err == nil {
		t.Fatal("replaced directory accepted")
	}
	b, _ := os.ReadFile(outside)
	if string(b) != "secret" {
		t.Fatal("outside file changed")
	}
}
