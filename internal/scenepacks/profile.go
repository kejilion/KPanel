package scenepacks

import "encoding/json"

// Profiles are fixed by the application, never supplied by a catalog or request.
// Share themes reuse the scene library's bounded, verified, atomic storage.
type profile struct {
	runtime, official, mirror, filePrefix string
	themes                                bool
}

var scenes = profile{"kpanel-scene-pack@1", officialRoot, mirrorRoot, FilePrefix, false}
var themes = profile{"kpanel-share-theme@2", "https://raw.githubusercontent.com/kejilion/KPanel/main/share-themes/", "https://gh.kejilion.pro/https://raw.githubusercontent.com/kejilion/KPanel/main/share-themes/", "/api/v1/cluster/share-themes/", true}

// Share themes speak protocol 2; protocol 1 packages stay installable because
// the host keeps answering them with the legacy schema-1 snapshot.
func (p profile) acceptsRuntime(runtime string) bool {
	return runtime == p.runtime || (p.themes && runtime == "kpanel-share-theme@1")
}

func OpenShareThemes(root string, fetch Fetch) *Store { return openProfile(root, fetch, themes) }

func DecodeShareThemeCatalog(data []byte) (Catalog, error) { return decodeCatalog(data, themes) }

func (s *Store) stateVersion() string {
	data, _ := json.Marshal(s.state)
	return "sha256:" + contentDigest(data)
}

// Select and deleting the active package commit selection in the same index as
// installed objects. A missing or damaged optional package falls back to default.
func (s *Store) Select(id, expected string) error {
	if !s.profile.themes || (id != "" && !ValidID(id)) {
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
	if expected == "" || expected != s.stateVersion() {
		return ErrConflict
	}
	if id != "" {
		if _, ok := s.state.Installed[id]; !ok {
			return ErrNotFound
		}
	}
	next := s.nextState()
	next.Selected = id
	return s.save(next)
}

type ActiveTheme struct {
	ID       string `json:"id"`
	Name     Text   `json:"name"`
	FileBase string `json:"fileBase"`
}

func (s *Store) ActiveTheme() *ActiveTheme {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.profile.themes || s.unavailable {
		return nil
	}
	item, ok := s.state.Installed[s.state.Selected]
	if !ok {
		return nil
	}
	return &ActiveTheme{item.Pack.ID, item.Pack.Name, s.profile.filePrefix + item.Pack.ID + "/files/" + item.Token + "/"}
}
