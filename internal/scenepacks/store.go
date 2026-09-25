package scenepacks

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

const officialRoot = "https://raw.githubusercontent.com/kejilion/KPanel/main/scene-packs/"
const mirrorRoot = "https://gh.kejilion.pro/" + officialRoot

type Fetch func(context.Context, string, int64) ([]byte, error)
type installedPack struct {
	Pack  Pack   `json:"pack"`
	Token string `json:"token"`
}
type state struct {
	Schema    int                      `json:"schema"`
	Source    string                   `json:"source"`
	Installed map[string]installedPack `json:"installed"`
}

type Store struct {
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	opMu        sync.Mutex
	root        string
	state       state
	unavailable bool
	fetch       Fetch
	catalog     Catalog
	lastAttempt time.Time
	lastSuccess time.Time
	// All network work, including thumbnails, is bounded independently of requests.
	network    chan struct{}
	writeState func(string, []byte) error
}

// Open degrades only optional artwork when its state is damaged. It never makes
// the administration UI unavailable or replaces a malformed state with empty data.
func Open(root string, fetch Fetch) *Store {
	client := remotedownload.NewClient(remotedownload.Config{ResponseHeaderTimeout: 8 * time.Second, IdleTimeout: 15 * time.Second})
	s := &Store{root: root, state: state{Schema: 1, Source: "auto", Installed: map[string]installedPack{}}, network: make(chan struct{}, 3), writeState: backup.AtomicFile}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.fetch = func(ctx context.Context, address string, limit int64) ([]byte, error) {
		response, err := client.Open(ctx, address)
		if err != nil {
			return nil, ErrSource
		}
		defer response.Body.Close()
		// Catalog trust does not extend to any redirect target, even a public host.
		if response.StatusCode != http.StatusOK || response.Request == nil || response.Request.URL.String() != address || response.ContentLength > limit {
			return nil, ErrSource
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
		if err != nil || int64(len(body)) > limit {
			return nil, ErrSource
		}
		return body, nil
	}
	if fetch != nil {
		s.fetch = fetch
	}
	if backup.PrivateDir(root) != nil || backup.PrivateDir(filepath.Join(root, "objects")) != nil {
		s.unavailable = true
		return s
	}
	data, err := backup.ReadFile(filepath.Join(root, "state.json"), MaxStateBytes)
	if err == nil {
		if json.Unmarshal(data, &s.state) != nil || !validState(s.state) {
			s.unavailable = true
			return s
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		s.unavailable = true
		return s
	}
	if err := s.pruneObjects(); err != nil {
		s.unavailable = true
	}
	return s
}

func validState(value state) bool {
	if value.Schema != 1 || !ValidSource(value.Source) || value.Installed == nil || len(value.Installed) > MaxInstalled {
		return false
	}
	var total int64
	tokens := map[string]bool{}
	for id, item := range value.Installed {
		if id != item.Pack.ID || validatePack(item.Pack) != nil || !ValidToken(item.Token) || tokens[item.Token] {
			return false
		}
		tokens[item.Token] = true
		total += item.Pack.SizeBytes
	}
	return total <= MaxTotalBytes
}

func (s *Store) pruneObjects() error {
	root := filepath.Join(s.root, "objects")
	if err := backup.NoLinkParents(root); err != nil {
		return err
	}
	dir, err := os.Open(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(MaxInstalled + 3)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > MaxInstalled+2 {
		return ErrQuota
	}
	keep := map[string]bool{}
	for _, item := range s.state.Installed {
		keep[item.Token] = true
	}
	for _, entry := range entries {
		if !ValidToken(entry.Name()) || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return ErrInvalid
		}
		if !keep[entry.Name()] {
			if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) save(next state) error {
	data, err := json.Marshal(next)
	if err != nil || !validState(next) || int64(len(data)) > MaxStateBytes {
		return ErrInvalid
	}
	file := filepath.Join(s.root, "state.json")
	if err := s.writeState(file, data); err != nil {
		// A directory fsync can fail after rename. Reconcile the actual committed
		// index before deciding whether the new object may be removed.
		actual, readErr := backup.ReadFile(file, MaxStateBytes)
		if readErr != nil || !bytes.Equal(actual, data) {
			return ErrUnavailable
		}
	}
	s.state = next
	return nil
}

func (s *Store) nextState() state {
	next := state{Schema: 1, Source: s.state.Source, Installed: map[string]installedPack{}}
	for id, item := range s.state.Installed {
		next.Installed[id] = item
	}
	return next
}

func (s *Store) download(ctx context.Context, source, relative string, limit int64, expected string) ([]byte, error) {
	ctx, cancelAll := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancelAll)
	defer func() { stop(); cancelAll() }()
	select {
	case s.network <- struct{}{}:
		defer func() { <-s.network }()
	default:
		return nil, ErrBusy
	}
	roots := []string{officialRoot, mirrorRoot}
	if source == "github" {
		roots = roots[:1]
	} else if source == "mirror" {
		roots = roots[1:]
	}
	for _, root := range roots {
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		body, err := s.fetch(requestCtx, root+relative, limit)
		cancel()
		if err == nil && int64(len(body)) <= limit && (expected == "" || (int64(len(body)) == limit && contentDigest(body) == expected)) {
			return body, nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, ErrSource
}

func (s *Store) Close() { s.cancel() }

// refresh is serialized with state mutations. Failed refreshes have a backoff;
// installed packs remain usable even when the remote catalog is unavailable.
func (s *Store) refresh(ctx context.Context) error {
	if !s.opMu.TryLock() {
		return ErrBusy
	}
	defer s.opMu.Unlock()
	s.mu.Lock()
	if s.unavailable {
		s.mu.Unlock()
		return ErrUnavailable
	}
	now := time.Now()
	if now.Sub(s.lastAttempt) < 30*time.Second || now.Sub(s.lastSuccess) < 5*time.Minute {
		err := error(nil)
		if s.lastSuccess.IsZero() || s.lastSuccess.Before(s.lastAttempt) {
			err = ErrSource
		}
		s.mu.Unlock()
		return err
	}
	s.lastAttempt = now
	source := s.state.Source
	s.mu.Unlock()
	body, err := s.download(ctx, source, "catalog.json", MaxCatalogBytes, "")
	if err != nil {
		return err
	}
	catalog, err := DecodeCatalog(body)
	if err != nil {
		return ErrSource
	}
	s.mu.Lock()
	s.catalog = catalog
	s.lastSuccess = time.Now()
	s.mu.Unlock()
	return nil
}

func (s *Store) view(p Pack) View {
	item, installed := s.state.Installed[p.ID]
	v := View{ID: p.ID, Version: p.Version, Name: p.Name, Description: p.Description, Author: p.Author, License: p.License, Tags: p.Tags, Theme: p.Theme, Cameras: p.Cameras, SizeBytes: p.SizeBytes, Installed: installed}
	if installed {
		version := item.Pack.Version
		base := FilePrefix + p.ID + "/files/" + item.Token + "/"
		v.InstalledVersion = &version
		v.FileBase = &base
	}
	identity, _ := json.Marshal(struct {
		Pack  Pack
		Token string
	}{p, item.Token})
	v.ResourceVersion = "sha256:" + contentDigest(identity)
	return v
}

func (s *Store) packs() []Pack {
	result := append([]Pack{}, s.catalog.Packs...)
	seen := map[string]bool{}
	for _, p := range result {
		seen[p.ID] = true
	}
	for id, item := range s.state.Installed {
		if !seen[id] {
			result = append(result, item.Pack)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (s *Store) List(ctx context.Context) (List, error) {
	err := s.refresh(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return List{}, ErrUnavailable
	}
	if err != nil && s.catalog.Packs == nil && len(s.state.Installed) == 0 {
		return List{}, err
	}
	result := List{Source: s.state.Source, Sources: []string{"auto", "github", "mirror"}, Packs: []View{}}
	if err != nil {
		result.Warning = "scene_pack_catalog_unavailable"
	}
	for _, p := range s.packs() {
		result.Packs = append(result.Packs, s.view(p))
	}
	return result, nil
}

func (s *Store) SetSource(source string) error {
	if !ValidSource(source) {
		return ErrInvalid
	}
	if !s.opMu.TryLock() {
		return ErrBusy
	}
	defer s.opMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return ErrUnavailable
	}
	next := s.nextState()
	next.Source = source
	if err := s.save(next); err != nil {
		return err
	}
	s.lastAttempt = time.Time{}
	s.lastSuccess = time.Time{}
	return nil
}

func (s *Store) Install(ctx context.Context, id, expected string) (View, error) {
	if !ValidID(id) {
		return View{}, ErrInvalid
	}
	if err := s.refresh(ctx); err != nil {
		return View{}, err
	}
	if !s.opMu.TryLock() {
		return View{}, ErrBusy
	}
	defer s.opMu.Unlock()
	s.mu.Lock()
	var pack Pack
	for _, p := range s.catalog.Packs {
		if p.ID == id {
			pack = p
			break
		}
	}
	if pack.ID == "" {
		s.mu.Unlock()
		return View{}, ErrNotFound
	}
	if expected == "" || s.view(pack).ResourceVersion != expected {
		s.mu.Unlock()
		return View{}, ErrConflict
	}
	var total int64
	for otherID, item := range s.state.Installed {
		if otherID != id {
			total += item.Pack.SizeBytes
		}
	}
	_, replacing := s.state.Installed[id]
	if total+pack.SizeBytes > MaxTotalBytes || (!replacing && len(s.state.Installed) >= MaxInstalled) {
		s.mu.Unlock()
		return View{}, ErrQuota
	}
	source := s.state.Source
	if err := s.pruneObjects(); err != nil {
		s.mu.Unlock()
		return View{}, ErrUnavailable
	}
	s.mu.Unlock()
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return View{}, ErrUnavailable
	}
	token := hex.EncodeToString(random[:])
	target := filepath.Join(s.root, "objects", token)
	if err := backup.PrivateDir(target); err != nil {
		return View{}, ErrUnavailable
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(target)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	for _, file := range pack.Files {
		body, err := s.download(ctx, source, pack.Path+"/"+file.Path, file.Size, file.SHA256)
		if err != nil {
			return View{}, err
		}
		name := filepath.Join(target, filepath.FromSlash(file.Path))
		if backup.PrivateDir(filepath.Dir(name)) != nil {
			return View{}, ErrUnavailable
		}
		if _, err := backup.CopyFile(name, bytes.NewReader(body), file.Size); err != nil {
			return View{}, ErrUnavailable
		}
	}
	if ctx.Err() != nil {
		return View{}, ErrSource
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.nextState()
	previous := next.Installed[id]
	next.Installed[id] = installedPack{Pack: pack, Token: token}
	if err := s.save(next); err != nil {
		return View{}, err
	}
	committed = true
	if previous.Token != "" {
		_ = os.RemoveAll(filepath.Join(s.root, "objects", previous.Token))
	}
	return s.view(pack), nil
}

func (s *Store) Delete(id, expected string) error {
	if !ValidID(id) {
		return ErrInvalid
	}
	if !s.opMu.TryLock() {
		return ErrBusy
	}
	defer s.opMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return ErrUnavailable
	}
	item, exists := s.state.Installed[id]
	if !exists {
		return ErrNotFound
	}
	p := item.Pack
	for _, candidate := range s.catalog.Packs {
		if candidate.ID == id {
			p = candidate
			break
		}
	}
	if expected == "" || s.view(p).ResourceVersion != expected {
		return ErrConflict
	}
	next := s.nextState()
	delete(next.Installed, id)
	if err := s.save(next); err != nil {
		return err
	}
	return s.pruneObjects()
}

// File requires an unguessable capability from the authenticated catalog view.
// Only catalog-listed bytes are served; even direct HTML navigation is sandboxed
// by the HTTP layer. Capabilities expose artwork, never Panel APIs or state.
func (s *Store) File(id, token, name string) ([]byte, string, error) {
	if !ValidID(id) || !ValidToken(token) || !ValidPath(name) {
		return nil, "", ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unavailable {
		return nil, "", ErrUnavailable
	}
	item, ok := s.state.Installed[id]
	if !ok || item.Token != token {
		return nil, "", ErrNotFound
	}
	var spec File
	for _, file := range item.Pack.Files {
		if file.Path == name {
			spec = file
			break
		}
	}
	if spec.Path == "" {
		return nil, "", ErrNotFound
	}
	objectPath := filepath.Join(s.root, "objects", token)
	if backup.NoLinkParents(objectPath) != nil {
		return nil, "", ErrInvalid
	}
	root, err := os.OpenRoot(objectPath)
	if err != nil {
		return nil, "", ErrNotFound
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, "", ErrNotFound
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != spec.Size {
		return nil, "", ErrInvalid
	}
	body, err := io.ReadAll(io.LimitReader(f, spec.Size+1))
	if err != nil || int64(len(body)) != spec.Size || contentDigest(body) != spec.SHA256 {
		return nil, "", ErrInvalid
	}
	return body, ContentType(name), nil
}

func (s *Store) Image(ctx context.Context, id, name string) ([]byte, error) {
	if !ValidID(id) || (name != "thumb.webp" && name != "poster.webp") {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	item, installed := s.state.Installed[id]
	s.mu.Unlock()
	if installed {
		body, _, err := s.File(id, item.Token, name)
		return body, err
	}
	if name == "poster.webp" {
		return nil, ErrNotFound
	}
	if err := s.refresh(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	source := s.state.Source
	var pack Pack
	for _, p := range s.catalog.Packs {
		if p.ID == id {
			pack = p
			break
		}
	}
	s.mu.Unlock()
	for _, file := range pack.Files {
		if file.Path == name {
			return s.download(ctx, source, pack.Path+"/"+name, file.Size, file.SHA256)
		}
	}
	return nil, ErrNotFound
}
