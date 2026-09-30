package scenepacks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func themeFixture(t *testing.T) (Pack, Fetch) {
	t.Helper()
	data, err := os.ReadFile("../../share-themes/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := DecodeShareThemeCatalog(data)
	if err != nil || len(catalog.Packs) < 2 {
		t.Fatalf("repository catalog: %v", err)
	}
	for _, p := range catalog.Packs {
		for _, file := range p.Files {
			body, err := os.ReadFile(filepath.Join("../../share-themes", p.Path, file.Path))
			if err != nil || int64(len(body)) != file.Size || contentDigest(body) != file.SHA256 {
				t.Fatalf("repository integrity %s/%s: %v", p.ID, file.Path, err)
			}
			if file.Path != "manifest.json" {
				source, err := os.ReadFile(filepath.Join("../../share-themes", p.ID, "src", file.Path))
				if err != nil || !bytes.Equal(source, body) {
					t.Fatalf("stale theme build %s/%s", p.ID, file.Path)
				}
			}
		}
	}
	return catalog.Packs[0], func(_ context.Context, address string, limit int64) ([]byte, error) {
		if !strings.HasPrefix(address, themes.official) {
			return nil, ErrSource
		}
		relative := strings.TrimPrefix(address, themes.official)
		if !filepath.IsLocal(relative) || strings.Contains(relative, "..") {
			t.Fatal("unsafe source", address)
		}
		return os.ReadFile(filepath.Join("../../share-themes", relative))
	}
}

func TestShareThemeProfileLifecycleAndIsolation(t *testing.T) {
	pack, fetch := themeFixture(t)
	data, _ := json.Marshal(Catalog{Schema: 1, Packs: []Pack{pack}})
	if _, err := DecodeCatalog(data); err == nil {
		t.Fatal("scene profile accepted theme")
	}
	bad := pack
	bad.SizeBytes = 5<<20 + 1
	if validatePackProfile(bad, themes) == nil {
		t.Fatal("theme quota not enforced")
	}
	root := t.TempDir()
	s := OpenShareThemes(root, fetch)
	defer s.Close()
	list, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Select(pack.ID, list.ResourceVersion); !errors.Is(err, ErrNotFound) {
		t.Fatal("uninstalled selection", err)
	}
	v, err := s.Install(context.Background(), pack.ID, list.Packs[0].ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	if s.ActiveTheme() != nil {
		t.Fatal("download applied automatically")
	}
	if err := s.Select(pack.ID, list.ResourceVersion); !errors.Is(err, ErrConflict) {
		t.Fatal("stale selection", err)
	}
	list, _ = s.List(context.Background())
	if err := s.Select(pack.ID, list.ResourceVersion); err != nil {
		t.Fatal(err)
	}
	active := s.ActiveTheme()
	if active == nil || !strings.HasPrefix(active.FileBase, themes.filePrefix) {
		t.Fatal("missing active theme")
	}
	s.Close()
	s = OpenShareThemes(root, fetch)
	defer s.Close()
	if got := s.ActiveTheme(); got == nil || got.ID == "" {
		t.Fatal("selection did not survive restart")
	}
	list, _ = s.List(context.Background())
	s.writeState = func(string, []byte) error { return errors.New("full disk") }
	if err := s.Select("", list.ResourceVersion); err == nil || s.ActiveTheme() == nil {
		t.Fatal("failed selection lost prior state")
	}
	s.Close()
	s = OpenShareThemes(root, fetch)
	defer s.Close()
	if err := s.Delete(pack.ID, v.ResourceVersion); err != nil {
		t.Fatal(err)
	}
	if s.ActiveTheme() != nil {
		t.Fatal("deleted active theme did not reset")
	}
	parts := strings.Split(strings.TrimSuffix(active.FileBase, "/"), "/")
	if _, _, err := s.File(pack.ID, parts[len(parts)-1], "index.html"); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted capability survived", err)
	}
}

func TestThemeInstallDigestFailurePreservesSelection(t *testing.T) {
	p, fetch := themeFixture(t)
	corrupt := false
	s := OpenShareThemes(t.TempDir(), func(ctx context.Context, address string, limit int64) ([]byte, error) {
		if corrupt && strings.HasSuffix(address, "/theme.js") {
			return []byte("bad"), nil
		}
		return fetch(ctx, address, limit)
	})
	defer s.Close()
	list, _ := s.List(context.Background())
	v, err := s.Install(context.Background(), p.ID, list.Packs[0].ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	list, _ = s.List(context.Background())
	if err := s.Select(p.ID, list.ResourceVersion); err != nil {
		t.Fatal(err)
	}
	before := s.ActiveTheme().FileBase
	corrupt = true
	if _, err := s.Install(context.Background(), p.ID, v.ResourceVersion); !errors.Is(err, ErrSource) {
		t.Fatal("corruption accepted", err)
	}
	if s.ActiveTheme().FileBase != before {
		t.Fatal("failed install changed active bytes")
	}
	list, _ = s.List(context.Background())
	if !list.Packs[0].Installed {
		t.Fatal("failed update removed installed copy")
	}
}

func TestShareThemeRuntimeCompatibility(t *testing.T) {
	pack, _ := themeFixture(t)
	for _, runtime := range []string{"kpanel-share-theme@1", "kpanel-share-theme@2"} {
		pack.Runtime = runtime
		if err := validatePackProfile(pack, themes); err != nil {
			t.Fatalf("%s must stay installable: %v", runtime, err)
		}
	}
	for _, runtime := range []string{"", "kpanel-share-theme@3", "kpanel-scene-pack@1"} {
		pack.Runtime = runtime
		if err := validatePackProfile(pack, themes); err == nil {
			t.Fatalf("%q must be rejected", runtime)
		}
	}
	// Scene packs never gain the share-theme protocols.
	pack.Runtime = "kpanel-share-theme@2"
	if scenes.acceptsRuntime(pack.Runtime) {
		t.Fatal("scene profile accepted a share-theme runtime")
	}
}
