package scenepacks

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixturePack(id, version string) (Pack, map[string][]byte) {
	p := Pack{Schema: 1, ID: id, Version: version, Runtime: "kpanel-scene-pack@1", Name: Text{"zh-CN": "场景", "en-US": "Scene"}, Description: Text{"zh-CN": "测试场景", "en-US": "Test scene"}, Author: Author{Name: "KPanel"}, License: "MIT", Tags: []string{"space"}, Theme: Theme{"#112233", "#445566", "#778899"}, Cameras: []Camera{{ID: "wide", Name: Text{"zh-CN": "全景", "en-US": "Wide"}}}, Entry: "index.html", Poster: "poster.webp", Thumb: "thumb.webp", Path: id + "/dist"}
	data := map[string][]byte{"index.html": []byte(`<html><script src="./scene.js"></script></html>`), "manifest.json": []byte(`{"schema":1}`), "poster.webp": []byte("poster"), "thumb.webp": []byte("thumb"), "scene.js": []byte("console.log('" + version + "')")}
	for _, name := range []string{"index.html", "manifest.json", "poster.webp", "thumb.webp", "scene.js"} {
		body := data[name]
		p.Files = append(p.Files, File{Path: name, Size: int64(len(body)), SHA256: contentDigest(body)})
		p.SizeBytes += int64(len(body))
	}
	return p, data
}

func fixtureFetch(t *testing.T, p *Pack, files map[string][]byte) Fetch {
	t.Helper()
	return func(ctx context.Context, address string, limit int64) ([]byte, error) {
		if !strings.HasPrefix(address, officialRoot) && !strings.HasPrefix(address, mirrorRoot) {
			t.Errorf("untrusted source %s", address)
			return nil, ErrSource
		}
		if strings.HasSuffix(address, "/catalog.json") {
			return json.Marshal(Catalog{Schema: 1, Packs: []Pack{*p}})
		}
		for name, body := range files {
			if strings.HasSuffix(address, "/"+p.Path+"/"+name) {
				return body, nil
			}
		}
		return nil, ErrSource
	}
}

func listOne(t *testing.T, s *Store) View {
	t.Helper()
	list, err := s.List(context.Background())
	if err != nil || len(list.Packs) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	return list.Packs[0]
}

func TestInstallAtomicFailureRestartDeleteAndCapabilities(t *testing.T) {
	p, files := fixturePack("orbit", "1.0.0")
	root := t.TempDir()
	fetch := fixtureFetch(t, &p, files)
	s := Open(root, fetch)
	defer s.Close()
	initial := listOne(t, s)
	if _, err := s.Install(context.Background(), p.ID, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale install %v", err)
	}
	installed, err := s.Install(context.Background(), p.ID, initial.ResourceVersion)
	if err != nil || !installed.Installed {
		t.Fatalf("install %+v %v", installed, err)
	}
	token := s.state.Installed[p.ID].Token
	if data, _, err := s.File(p.ID, token, "scene.js"); err != nil || string(data) != string(files["scene.js"]) {
		t.Fatalf("file %q %v", data, err)
	}
	if _, _, err := s.File(p.ID, strings.Repeat("0", 32), "scene.js"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("forged capability %v", err)
	}
	if _, _, err := s.File(p.ID, token, "../state.json"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("traversal %v", err)
	}
	p2, files2 := fixturePack("orbit", "1.0.1")
	p = p2
	s.fetch = fixtureFetch(t, &p, files2)
	s.lastSuccess = time.Time{}
	s.lastAttempt = time.Time{}
	update := listOne(t, s)
	files2["scene.js"] = []byte("bad digest")
	if _, err := s.Install(context.Background(), p.ID, update.ResourceVersion); !errors.Is(err, ErrSource) {
		t.Fatalf("digest mismatch %v", err)
	}
	if s.state.Installed[p.ID].Token != token {
		t.Fatal("failed update replaced installed pack")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "objects"))
	if len(entries) != 1 {
		t.Fatalf("staging leaked: %d", len(entries))
	}
	restarted := Open(root, func(context.Context, string, int64) ([]byte, error) { return nil, ErrSource })
	defer restarted.Close()
	list, err := restarted.List(context.Background())
	if err != nil || list.Warning == "" || !list.Packs[0].Installed || list.Packs[0].Version != "1.0.0" {
		t.Fatalf("offline restart %+v %v", list, err)
	}
	if err := restarted.Delete(p.ID, initial.ResourceVersion); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale delete %v", err)
	}
	if err := restarted.Delete(p.ID, list.Packs[0].ResourceVersion); err != nil {
		t.Fatal(err)
	}
	if _, _, err := restarted.File(p.ID, token, "scene.js"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted capability remains usable %v", err)
	}
	entries, _ = os.ReadDir(filepath.Join(root, "objects"))
	if len(entries) != 0 {
		t.Fatal("deleted bytes retained")
	}
}

func TestStorageFailurePreservesPreviousAndPrunesInterruptedStaging(t *testing.T) {
	p, files := fixturePack("orbit", "1.0.0")
	root := t.TempDir()
	s := Open(root, fixtureFetch(t, &p, files))
	defer s.Close()
	v := listOne(t, s)
	installed, err := s.Install(context.Background(), p.ID, v.ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	token := s.state.Installed[p.ID].Token
	s.writeState = func(string, []byte) error { return errors.New("disk full") }
	if err := s.Delete(p.ID, installed.ResourceVersion); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("failed index commit %v", err)
	}
	if _, _, err := s.File(p.ID, token, "scene.js"); err != nil {
		t.Fatal("failed delete removed artwork", err)
	}
	staging := filepath.Join(root, "objects", strings.Repeat("a", 32))
	if err := os.Mkdir(staging, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "partial.js"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	restarted := Open(root, fixtureFetch(t, &p, files))
	defer restarted.Close()
	if restarted.unavailable {
		t.Fatal("valid restart unavailable")
	}
	if _, err := os.Stat(staging); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interrupted staging retained", err)
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	broken := Open(root, nil)
	defer broken.Close()
	if _, err := broken.List(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("corruption hidden %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "objects", token)); err != nil {
		t.Fatal("corrupt state destroyed original", err)
	}
}

func TestCatalogRejectsPathsCountsHashesAndOversizedEntries(t *testing.T) {
	p, _ := fixturePack("orbit", "1.0.0")
	cases := map[string]func(*Pack){
		"escape": func(p *Pack) { p.Path = "../other" }, "fileEscape": func(p *Pack) { p.Files[0].Path = "../index.html" },
		"absolute": func(p *Pack) { p.Files[0].Path = "/index.html" }, "encoded": func(p *Pack) { p.Files[0].Path = "a%2findex.html" },
		"hash": func(p *Pack) { p.Files[0].SHA256 = "bad" }, "oversized": func(p *Pack) { p.Files[0].Size = MaxFileBytes + 1 },
		"total": func(p *Pack) { p.SizeBytes++ }, "duplicate": func(p *Pack) { p.Files = append(p.Files, p.Files[0]) },
		"runtime": func(p *Pack) { p.Runtime = "other" }, "color": func(p *Pack) { p.Theme.Brand = "url(evil)" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			q := p
			q.Files = append([]File{}, p.Files...)
			change(&q)
			data, _ := json.Marshal(Catalog{1, []Pack{q}})
			if _, err := DecodeCatalog(data); err == nil {
				t.Fatal("accepted invalid catalog")
			}
		})
	}
	data, _ := json.Marshal(Catalog{1, []Pack{p, p}})
	if _, err := DecodeCatalog(data); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
	actual, err := os.ReadFile("../../scene-packs/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := DecodeCatalog(actual)
	if err != nil || len(catalog.Packs) != 3 {
		t.Fatalf("published three-pack catalog %d %v", len(catalog.Packs), err)
	}
}

func TestConcurrentInstallCloseAndSourcePersistence(t *testing.T) {
	p, files := fixturePack("orbit", "1.0.0")
	s := Open(t.TempDir(), fixtureFetch(t, &p, files))
	defer s.Close()
	v := listOne(t, s)
	base := s.fetch
	entered := make(chan struct{})
	var blocked atomic.Bool
	s.fetch = func(ctx context.Context, address string, limit int64) ([]byte, error) {
		if strings.HasSuffix(address, "/index.html") {
			if blocked.CompareAndSwap(false, true) {
				close(entered)
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return base(ctx, address, limit)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Install(context.Background(), p.ID, v.ResourceVersion); done <- err }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("download never started")
	}
	if err := s.SetSource("mirror"); !errors.Is(err, ErrBusy) {
		t.Fatalf("concurrent mutation %v", err)
	}
	s.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled install succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel download")
	}
	restarted := Open(s.root, base)
	defer restarted.Close()
	if err := restarted.SetSource("mirror"); err != nil {
		t.Fatal(err)
	}
	again := Open(s.root, base)
	defer again.Close()
	if again.state.Source != "mirror" {
		t.Fatal("source was not persisted")
	}
}

func TestQuotaAndSymlinkDoNotEscapeObjectRoot(t *testing.T) {
	p, files := fixturePack("orbit", "1.0.0")
	s := Open(t.TempDir(), fixtureFetch(t, &p, files))
	defer s.Close()
	v := listOne(t, s)
	for i := 0; i < MaxInstalled; i++ {
		s.state.Installed[string(rune('a'+i))] = installedPack{Pack: p, Token: strings.Repeat("b", 32)}
	}
	if _, err := s.Install(context.Background(), p.ID, v.ResourceVersion); !errors.Is(err, ErrQuota) {
		t.Fatalf("quota %v", err)
	}
	s.state.Installed = map[string]installedPack{}
	v = listOne(t, s)
	if _, err := s.Install(context.Background(), p.ID, v.ResourceVersion); err != nil {
		t.Fatal(err)
	}
	token := s.state.Installed[p.ID].Token
	target := filepath.Join(s.root, "objects", token, "scene.js")
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, files["scene.js"], 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Skip("symlink unavailable", err)
	}
	if _, _, err := s.File(p.ID, token, "scene.js"); err == nil {
		t.Fatal("followed outside symlink")
	}
}
