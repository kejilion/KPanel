package desktopworkspace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/webp"
)

const (
	SchemaVersion        = 2
	MaxWorkspaceBytes    = 256 << 10
	MaxIconBytes         = 256 << 10
	MaxIconTotalBytes    = 16 << 20
	MaxShortcuts         = 64
	MaxPositions         = 512
	MaxHiddenEntryKeys   = 512
	MaxShortcutNameRunes = 48
	MaxDescriptionRunes  = 160
	MaxURLBytes          = 2048
	MaxPathBytes         = 4096
	MaxIconDimension     = 1024
	MaxIconPixels        = 1_000_000
)

type ShortcutTargetType string

const (
	ShortcutTargetURL       ShortcutTargetType = "url"
	ShortcutTargetFile      ShortcutTargetType = "file"
	ShortcutTargetDirectory ShortcutTargetType = "directory"
)

const unavailableWarning = "desktop_workspace_unavailable"

var (
	ErrConflict       = errors.New("desktop workspace changed")
	ErrUnavailable    = errors.New("desktop workspace is unavailable")
	ErrNotFound       = errors.New("desktop shortcut not found")
	ErrIconInvalid    = errors.New("desktop shortcut icon is invalid")
	ErrIconQuota      = errors.New("desktop shortcut icon quota exceeded")
	errInvalidOnDisk  = errors.New("desktop workspace data is invalid")
	shortcutIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	resourcePattern   = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	navKeyPattern     = regexp.MustCompile(`^nav:/[a-z][a-z0-9-]{0,63}$`)
	appIDPattern      = regexp.MustCompile(`^(?:builtin|thirdparty)-[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	siteIDPattern     = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type ShortcutInput struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	TargetType  ShortcutTargetType `json:"targetType,omitempty"`
	URL         string             `json:"url,omitempty"`
	Path        string             `json:"path,omitempty"`
}

type Shortcut struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	TargetType  ShortcutTargetType `json:"targetType"`
	URL         string             `json:"url,omitempty"`
	Path        string             `json:"path,omitempty"`
	IconVersion string             `json:"iconVersion,omitempty"`
	IconURL     string             `json:"iconURL,omitempty"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

type Workspace struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	ResourceVersion string              `json:"resourceVersion"`
	Available       bool                `json:"available"`
	Warning         string              `json:"warning,omitempty"`
	HiddenEntryKeys []string            `json:"hiddenEntryKeys"`
	Positions       map[string]Position `json:"positions"`
	Labels          map[string]string   `json:"labels"`
	Shortcuts       []Shortcut          `json:"shortcuts"`
}

type ReplaceInput struct {
	ExpectedResourceVersion string              `json:"expectedResourceVersion"`
	HiddenEntryKeys         []string            `json:"hiddenEntryKeys"`
	Positions               map[string]Position `json:"positions"`
	Labels                  map[string]string   `json:"labels"`
	Shortcuts               []ShortcutInput     `json:"shortcuts"`
}

type Icon struct {
	ContentType string
	Data        []byte
	Version     string
}

type ValidationError struct {
	Field  string
	Detail string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Detail
}

type shortcutRecord struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	TargetType  ShortcutTargetType `json:"targetType,omitempty"`
	URL         string             `json:"url,omitempty"`
	Path        string             `json:"path,omitempty"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

type persistedWorkspace struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	HiddenEntryKeys []string            `json:"hiddenEntryKeys"`
	Positions       map[string]Position `json:"positions"`
	Labels          map[string]string   `json:"labels"`
	Shortcuts       []shortcutRecord    `json:"shortcuts"`
}

type atomicWriter func(directory, target string, data []byte) error

type Store struct {
	mu            sync.RWMutex
	root          string
	workspacePath string
	iconsDir      string
	state         persistedWorkspace
	available     bool
	iconVersions  map[string]string
	now           func() time.Time
	writeAtomic   atomicWriter
}

func Open(root string) (*Store, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "." || !filepath.IsAbs(root) {
		return nil, errors.New("desktop workspace root must be absolute")
	}
	iconsDir := filepath.Join(root, "icons")
	if err := ensurePrivateDirectory(root); err != nil {
		return nil, fmt.Errorf("initialize desktop workspace directory: %w", err)
	}
	if err := ensurePrivateDirectory(iconsDir); err != nil {
		return nil, fmt.Errorf("initialize desktop icon directory: %w", err)
	}
	store := &Store{
		root:          root,
		workspacePath: filepath.Join(root, "workspace.json"),
		iconsDir:      iconsDir,
		state:         emptyPersistedWorkspace(),
		available:     true,
		iconVersions:  make(map[string]string),
		now:           time.Now,
		writeAtomic:   writeAtomicPrivateFile,
	}
	state, err := readPersistedWorkspace(store.workspacePath)
	switch {
	case err == nil:
		store.state = state
		if chmodErr := os.Chmod(store.workspacePath, 0o600); chmodErr != nil {
			return nil, fmt.Errorf("protect desktop workspace: %w", chmodErr)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persistLocked(store.state); err != nil {
			return nil, err
		}
	case errors.Is(err, errInvalidOnDisk):
		store.available = false
	default:
		return nil, err
	}
	if store.available {
		if err := store.gcIconsLocked(); err != nil {
			return nil, fmt.Errorf("inspect desktop icons: %w", err)
		}
	}
	return store, nil
}

func ValidShortcutID(id string) bool {
	return shortcutIDPattern.MatchString(id)
}

func ValidResourceVersion(value string) bool {
	return resourcePattern.MatchString(value)
}

func (s *Store) Workspace() Workspace {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workspaceLocked()
}

func (s *Store) Replace(input ReplaceInput) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return Workspace{}, ErrUnavailable
	}
	if !ValidResourceVersion(input.ExpectedResourceVersion) {
		return Workspace{}, &ValidationError{
			Field: "expectedResourceVersion", Detail: "a valid resourceVersion is required",
		}
	}
	if input.ExpectedResourceVersion != resourceVersion(s.state) {
		return Workspace{}, ErrConflict
	}
	next, err := buildPersistedWorkspace(input, s.state, s.now().UTC())
	if err != nil {
		return Workspace{}, err
	}
	if err := s.persistLocked(next); err != nil {
		return Workspace{}, err
	}
	s.state = next
	// The metadata rename is the commit point. Icon cleanup is deliberately
	// best-effort so an orphan cannot turn a committed workspace update into an
	// ambiguous failure.
	_ = s.gcIconsLocked()
	return s.workspaceLocked(), nil
}

func (s *Store) Icon(id string) (Icon, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.available {
		return Icon{}, ErrUnavailable
	}
	if !ValidShortcutID(id) || !s.shortcutExistsLocked(id) {
		return Icon{}, ErrNotFound
	}
	return readIcon(s.iconPath(id))
}

func (s *Store) PutIcon(id, contentType string, data []byte) (Icon, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return Icon{}, ErrUnavailable
	}
	if !ValidShortcutID(id) || !s.shortcutExistsLocked(id) {
		return Icon{}, ErrNotFound
	}
	icon, err := validateIcon(contentType, data)
	if err != nil {
		return Icon{}, err
	}
	total, oldSize, err := s.iconBytesLocked(id)
	if err != nil {
		return Icon{}, err
	}
	if total-oldSize+int64(len(data)) > MaxIconTotalBytes {
		return Icon{}, ErrIconQuota
	}
	if err := s.writeAtomic(s.iconsDir, s.iconPath(id), data); err != nil {
		return Icon{}, fmt.Errorf("persist desktop shortcut icon: %w", err)
	}
	s.iconVersions[id] = icon.Version
	return icon, nil
}

func (s *Store) DeleteIcon(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.available {
		return ErrUnavailable
	}
	if !ValidShortcutID(id) || !s.shortcutExistsLocked(id) {
		return ErrNotFound
	}
	path := s.iconPath(id)
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		delete(s.iconVersions, id)
		return nil
	case err != nil:
		return fmt.Errorf("inspect desktop shortcut icon: %w", err)
	case !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0:
		return errors.New("desktop shortcut icon must be a regular file")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove desktop shortcut icon: %w", err)
	}
	delete(s.iconVersions, id)
	_ = syncDirectory(s.iconsDir)
	return nil
}

func (s *Store) workspaceLocked() Workspace {
	state := s.state
	if !s.available {
		state = emptyPersistedWorkspace()
	}
	shortcuts := make([]Shortcut, 0, len(state.Shortcuts))
	for _, item := range state.Shortcuts {
		shortcuts = append(shortcuts, Shortcut{
			ID: item.ID, Name: item.Name, Description: item.Description,
			TargetType: item.TargetType, URL: item.URL, Path: item.Path,
			IconVersion: s.iconVersions[item.ID], CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	result := Workspace{
		SchemaVersion: SchemaVersion, ResourceVersion: resourceVersion(state),
		Available: s.available, HiddenEntryKeys: append([]string{}, state.HiddenEntryKeys...),
		Positions: clonePositions(state.Positions), Labels: cloneLabels(state.Labels),
		Shortcuts: shortcuts,
	}
	if !s.available {
		result.Warning = unavailableWarning
	}
	return result
}

func (s *Store) shortcutExistsLocked(id string) bool {
	index := sort.Search(len(s.state.Shortcuts), func(index int) bool {
		return s.state.Shortcuts[index].ID >= id
	})
	return index < len(s.state.Shortcuts) && s.state.Shortcuts[index].ID == id
}

func (s *Store) persistLocked(state persistedWorkspace) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode desktop workspace: %w", err)
	}
	data = append(data, '\n')
	if len(data) > MaxWorkspaceBytes {
		return &ValidationError{Field: "workspace", Detail: "desktop workspace exceeds 256 KiB"}
	}
	if err := s.writeAtomic(s.root, s.workspacePath, data); err != nil {
		return fmt.Errorf("persist desktop workspace: %w", err)
	}
	return nil
}

func (s *Store) iconPath(id string) string {
	return filepath.Join(s.iconsDir, id+".icon")
}

func (s *Store) iconBytesLocked(replacingID string) (total, replaced int64, err error) {
	entries, err := os.ReadDir(s.iconsDir)
	if err != nil {
		return 0, 0, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".icon") {
			continue
		}
		id := strings.TrimSuffix(name, ".icon")
		if !ValidShortcutID(id) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return 0, 0, infoErr
		}
		if !info.Mode().IsRegular() || entry.Type()&os.ModeSymlink != 0 {
			return 0, 0, errors.New("desktop shortcut icon must be a regular file")
		}
		total += info.Size()
		if id == replacingID {
			replaced = info.Size()
		}
	}
	return total, replaced, nil
}

func (s *Store) gcIconsLocked() error {
	referenced := make(map[string]bool, len(s.state.Shortcuts))
	for _, shortcut := range s.state.Shortcuts {
		referenced[shortcut.ID] = true
	}
	entries, err := os.ReadDir(s.iconsDir)
	if err != nil {
		return err
	}
	s.iconVersions = make(map[string]string, len(referenced))
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(s.iconsDir, name)
		if strings.HasPrefix(name, ".desktop-workspace-") {
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				_ = os.Remove(path)
			}
			continue
		}
		if !strings.HasSuffix(name, ".icon") {
			continue
		}
		id := strings.TrimSuffix(name, ".icon")
		if !ValidShortcutID(id) || !referenced[id] {
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				_ = os.Remove(path)
			}
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return err
		}
		icon, readErr := readIcon(path)
		if readErr == nil {
			s.iconVersions[id] = icon.Version
		}
	}
	_ = syncDirectory(s.iconsDir)
	return nil
}

func emptyPersistedWorkspace() persistedWorkspace {
	return persistedWorkspace{
		SchemaVersion: SchemaVersion, HiddenEntryKeys: []string{},
		Positions: map[string]Position{}, Labels: map[string]string{}, Shortcuts: []shortcutRecord{},
	}
}

func readPersistedWorkspace(path string) (persistedWorkspace, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return persistedWorkspace{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return persistedWorkspace{}, errors.New("desktop workspace must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > MaxWorkspaceBytes {
		return persistedWorkspace{}, errInvalidOnDisk
	}
	file, err := os.Open(path)
	if err != nil {
		return persistedWorkspace{}, fmt.Errorf("open desktop workspace: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxWorkspaceBytes+1))
	if err != nil {
		return persistedWorkspace{}, fmt.Errorf("read desktop workspace: %w", err)
	}
	if len(data) == 0 || len(data) > MaxWorkspaceBytes {
		return persistedWorkspace{}, errInvalidOnDisk
	}
	var state persistedWorkspace
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return persistedWorkspace{}, errInvalidOnDisk
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return persistedWorkspace{}, errInvalidOnDisk
	}
	switch state.SchemaVersion {
	case 1:
		for index := range state.Shortcuts {
			item := &state.Shortcuts[index]
			if item.TargetType != "" || item.Path != "" {
				return persistedWorkspace{}, errInvalidOnDisk
			}
			item.TargetType = ShortcutTargetURL
		}
		state.SchemaVersion = SchemaVersion
	case SchemaVersion:
	default:
		return persistedWorkspace{}, errInvalidOnDisk
	}
	if err := validatePersistedWorkspace(state); err != nil {
		return persistedWorkspace{}, errInvalidOnDisk
	}
	return canonicalizePersistedWorkspace(state), nil
}

func buildPersistedWorkspace(input ReplaceInput, current persistedWorkspace, now time.Time) (persistedWorkspace, error) {
	if !ValidResourceVersion(input.ExpectedResourceVersion) {
		return persistedWorkspace{}, &ValidationError{
			Field: "expectedResourceVersion", Detail: "a valid resourceVersion is required",
		}
	}
	state := persistedWorkspace{
		SchemaVersion:   SchemaVersion,
		HiddenEntryKeys: append([]string(nil), input.HiddenEntryKeys...),
		Positions:       clonePositions(input.Positions), Labels: cloneLabels(input.Labels),
		Shortcuts: make([]shortcutRecord, 0, len(input.Shortcuts)),
	}
	currentByID := make(map[string]shortcutRecord, len(current.Shortcuts))
	for _, item := range current.Shortcuts {
		currentByID[item.ID] = item
	}
	for _, item := range input.Shortcuts {
		targetType, targetURL, targetPath, err := normalizedShortcutTarget(item.TargetType, item.URL, item.Path)
		if err != nil {
			return persistedWorkspace{}, err
		}
		record := shortcutRecord{
			ID: item.ID, Name: item.Name, Description: item.Description,
			TargetType: targetType, URL: targetURL, Path: targetPath,
			CreatedAt: now, UpdatedAt: now,
		}
		if previous, ok := currentByID[item.ID]; ok {
			record.CreatedAt = previous.CreatedAt
			if previous.Name == item.Name && previous.Description == item.Description &&
				previous.TargetType == targetType && previous.URL == targetURL && previous.Path == targetPath {
				record.UpdatedAt = previous.UpdatedAt
			}
		}
		state.Shortcuts = append(state.Shortcuts, record)
	}
	if err := validatePersistedWorkspace(state); err != nil {
		return persistedWorkspace{}, err
	}
	state = canonicalizePersistedWorkspace(state)
	encoded, err := json.Marshal(state)
	if err != nil {
		return persistedWorkspace{}, err
	}
	if len(encoded)+1 > MaxWorkspaceBytes {
		return persistedWorkspace{}, &ValidationError{Field: "workspace", Detail: "desktop workspace exceeds 256 KiB"}
	}
	return state, nil
}

func validatePersistedWorkspace(state persistedWorkspace) error {
	if state.SchemaVersion != SchemaVersion {
		return &ValidationError{Field: "schemaVersion", Detail: "unsupported desktop workspace schema"}
	}
	if len(state.Shortcuts) > MaxShortcuts {
		return &ValidationError{Field: "shortcuts", Detail: "at most 64 shortcuts are allowed"}
	}
	if len(state.HiddenEntryKeys) > MaxHiddenEntryKeys {
		return &ValidationError{Field: "hiddenEntryKeys", Detail: "at most 512 hidden entries are allowed"}
	}
	if len(state.Positions) > MaxPositions {
		return &ValidationError{Field: "positions", Detail: "at most 512 positions are allowed"}
	}
	shortcutIDs := make(map[string]bool, len(state.Shortcuts))
	for _, item := range state.Shortcuts {
		if !ValidShortcutID(item.ID) || shortcutIDs[item.ID] {
			return &ValidationError{Field: "shortcuts", Detail: "shortcut IDs must be unique 32-character lowercase hexadecimal values"}
		}
		shortcutIDs[item.ID] = true
		if strings.TrimSpace(item.Name) != item.Name || item.Name == "" ||
			!utf8.ValidString(item.Name) || utf8.RuneCountInString(item.Name) > MaxShortcutNameRunes ||
			hasControl(item.Name) {
			return &ValidationError{Field: "shortcuts.name", Detail: "shortcut name must contain 1 to 48 characters without controls or outer whitespace"}
		}
		if !utf8.ValidString(item.Description) || utf8.RuneCountInString(item.Description) > MaxDescriptionRunes || hasControl(item.Description) {
			return &ValidationError{Field: "shortcuts.description", Detail: "shortcut description must contain at most 160 characters without controls"}
		}
		if _, _, _, err := normalizedShortcutTarget(item.TargetType, item.URL, item.Path); err != nil {
			return err
		}
		if item.CreatedAt.IsZero() || item.UpdatedAt.IsZero() || item.UpdatedAt.Before(item.CreatedAt) {
			return &ValidationError{Field: "shortcuts", Detail: "shortcut timestamps are invalid"}
		}
	}

	hidden := make(map[string]bool, len(state.HiddenEntryKeys))
	for _, key := range state.HiddenEntryKeys {
		if !validHideableKey(key) || hidden[key] {
			return &ValidationError{Field: "hiddenEntryKeys", Detail: "hidden entry keys must be unique app: or site: keys"}
		}
		hidden[key] = true
	}
	for key, position := range state.Positions {
		if !validPositionKey(key) || math.IsNaN(position.X) || math.IsInf(position.X, 0) ||
			math.IsNaN(position.Y) || math.IsInf(position.Y, 0) ||
			position.X < 0 || position.X > 1 || position.Y < 0 || position.Y > MaxPositions {
			return &ValidationError{
				Field: "positions",
				Detail: fmt.Sprintf(
					"positions require a stable entry key, x between 0 and 1, and paged y between 0 and %d",
					MaxPositions,
				),
			}
		}
		if strings.HasPrefix(key, "shortcut:") && !shortcutIDs[strings.TrimPrefix(key, "shortcut:")] {
			return &ValidationError{Field: "positions", Detail: "shortcut positions must reference an existing shortcut"}
		}
	}
	for key, label := range state.Labels {
		if !validSiteKey(key) || strings.TrimSpace(label) != label || label == "" ||
			!utf8.ValidString(label) || utf8.RuneCountInString(label) > MaxShortcutNameRunes || hasControl(label) {
			return &ValidationError{Field: "labels", Detail: "labels require a site key and 1 to 48 visible characters"}
		}
	}
	return nil
}

func validateShortcutURL(value string) error {
	if value == "" || len(value) > MaxURLBytes || strings.TrimSpace(value) != value || hasControl(value) {
		return &ValidationError{Field: "shortcuts.url", Detail: "shortcut URL must be an absolute HTTP(S) URL up to 2048 bytes"}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" || parsed.Hostname() == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return &ValidationError{Field: "shortcuts.url", Detail: "shortcut URL must be an absolute HTTP(S) URL without credentials"}
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return &ValidationError{Field: "shortcuts.url", Detail: "shortcut URL port is invalid"}
		}
	}
	return nil
}

func normalizedShortcutTarget(targetType ShortcutTargetType, targetURL, targetPath string) (ShortcutTargetType, string, string, error) {
	// Treat the omitted type as URL for compatibility with a cached v1 client.
	if targetType == "" && targetURL != "" && targetPath == "" {
		targetType = ShortcutTargetURL
	}
	switch targetType {
	case ShortcutTargetURL:
		if targetPath != "" {
			return "", "", "", &ValidationError{Field: "shortcuts.path", Detail: "URL shortcuts cannot include a file path"}
		}
		if err := validateShortcutURL(targetURL); err != nil {
			return "", "", "", err
		}
		return targetType, targetURL, "", nil
	case ShortcutTargetFile, ShortcutTargetDirectory:
		if targetURL != "" {
			return "", "", "", &ValidationError{Field: "shortcuts.url", Detail: "file shortcuts cannot include a URL"}
		}
		if err := validateShortcutPath(targetPath, targetType == ShortcutTargetDirectory); err != nil {
			return "", "", "", err
		}
		return targetType, "", targetPath, nil
	default:
		return "", "", "", &ValidationError{Field: "shortcuts.targetType", Detail: "shortcut targetType must be url, file, or directory"}
	}
}

func validateShortcutPath(value string, directory bool) error {
	if value == "" || len(value) > MaxPathBytes || !utf8.ValidString(value) ||
		!strings.HasPrefix(value, "/") || strings.Contains(value, `\`) || hasControl(value) ||
		path.Clean(value) != value || (!directory && value == "/") {
		return &ValidationError{Field: "shortcuts.path", Detail: "shortcut path must be a canonical absolute POSIX path up to 4096 bytes"}
	}
	for _, component := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if component == "" && value != "/" || component == "." || component == ".." {
			return &ValidationError{Field: "shortcuts.path", Detail: "shortcut path cannot contain empty, dot, or parent components"}
		}
	}
	return nil
}

func validHideableKey(key string) bool {
	return validAppKey(key) || validSiteKey(key)
}

func validPositionKey(key string) bool {
	return navKeyPattern.MatchString(key) || validAppKey(key) || validSiteKey(key) || validShortcutKey(key)
}

func validAppKey(key string) bool {
	return strings.HasPrefix(key, "app:") && appIDPattern.MatchString(strings.TrimPrefix(key, "app:"))
}

func validSiteKey(key string) bool {
	return strings.HasPrefix(key, "site:") && siteIDPattern.MatchString(strings.TrimPrefix(key, "site:"))
}

func validShortcutKey(key string) bool {
	return strings.HasPrefix(key, "shortcut:") && ValidShortcutID(strings.TrimPrefix(key, "shortcut:"))
}

func hasControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func canonicalizePersistedWorkspace(state persistedWorkspace) persistedWorkspace {
	result := persistedWorkspace{
		SchemaVersion:   SchemaVersion,
		HiddenEntryKeys: append([]string(nil), state.HiddenEntryKeys...),
		Positions:       clonePositions(state.Positions), Labels: cloneLabels(state.Labels),
		Shortcuts: append([]shortcutRecord(nil), state.Shortcuts...),
	}
	if result.HiddenEntryKeys == nil {
		result.HiddenEntryKeys = []string{}
	}
	if result.Positions == nil {
		result.Positions = map[string]Position{}
	}
	if result.Labels == nil {
		result.Labels = map[string]string{}
	}
	if result.Shortcuts == nil {
		result.Shortcuts = []shortcutRecord{}
	}
	sort.Strings(result.HiddenEntryKeys)
	sort.Slice(result.Shortcuts, func(left, right int) bool {
		return result.Shortcuts[left].ID < result.Shortcuts[right].ID
	})
	return result
}

func clonePositions(source map[string]Position) map[string]Position {
	result := make(map[string]Position, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneLabels(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func resourceVersion(state persistedWorkspace) string {
	canonical := canonicalizePersistedWorkspace(state)
	data, _ := json.Marshal(canonical)
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validateIcon(contentType string, data []byte) (Icon, error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return Icon{}, ErrIconInvalid
	}
	if len(data) == 0 || len(data) > MaxIconBytes {
		return Icon{}, ErrIconInvalid
	}
	normalized := ""
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		normalized = "image/png"
		if mediaType != normalized || validateDecodedImage(data, "png") != nil {
			return Icon{}, ErrIconInvalid
		}
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		normalized = "image/jpeg"
		if mediaType != normalized || validateDecodedImage(data, "jpeg") != nil {
			return Icon{}, ErrIconInvalid
		}
	case len(data) >= 16 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		normalized = "image/webp"
		if mediaType != normalized || validateDecodedWebP(data) != nil {
			return Icon{}, ErrIconInvalid
		}
	default:
		return Icon{}, ErrIconInvalid
	}
	digest := sha256.Sum256(data)
	return Icon{
		ContentType: normalized, Data: append([]byte(nil), data...), Version: hex.EncodeToString(digest[:]),
	}, nil
}

func readIcon(path string) (Icon, error) {
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return Icon{}, ErrNotFound
	case err != nil:
		return Icon{}, err
	case !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0:
		return Icon{}, errors.New("desktop shortcut icon must be a regular file")
	case info.Size() <= 0 || info.Size() > MaxIconBytes:
		return Icon{}, ErrIconInvalid
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Icon{}, err
	}
	contentType := ""
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		contentType = "image/png"
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		contentType = "image/jpeg"
	case len(data) >= 16 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		contentType = "image/webp"
	default:
		return Icon{}, ErrIconInvalid
	}
	return validateIcon(contentType, data)
}

func validateDecodedImage(data []byte, expectedFormat string) error {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != expectedFormat {
		return ErrIconInvalid
	}
	if err := validateIconDimensions(config.Width, config.Height); err != nil {
		return err
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || format != expectedFormat {
		return ErrIconInvalid
	}
	return validateDecodedBounds(decoded.Bounds(), config.Width, config.Height)
}

func validateIconDimensions(width, height int) error {
	if width < 1 || height < 1 || width > MaxIconDimension || height > MaxIconDimension ||
		int64(width)*int64(height) > MaxIconPixels {
		return ErrIconInvalid
	}
	return nil
}

func validateDecodedWebP(data []byte) error {
	config, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ErrIconInvalid
	}
	if err := validateIconDimensions(config.Width, config.Height); err != nil {
		return err
	}
	decoded, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return ErrIconInvalid
	}
	return validateDecodedBounds(decoded.Bounds(), config.Width, config.Height)
}

func validateDecodedBounds(bounds image.Rectangle, expectedWidth, expectedHeight int) error {
	width, height := bounds.Dx(), bounds.Dy()
	if width != expectedWidth || height != expectedHeight {
		return ErrIconInvalid
	}
	return validateIconDimensions(width, height)
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path must be a non-symlink directory")
	}
	return os.Chmod(path, 0o700)
}

func writeAtomicPrivateFile(directory, target string, data []byte) error {
	file, err := os.CreateTemp(directory, ".desktop-workspace-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	renameErr := os.Rename(temporary, target)
	if renameErr == nil {
		_ = syncDirectory(directory)
		return nil
	}
	if runtime.GOOS != "windows" {
		return renameErr
	}

	// Production Linux replaces atomically above. This fallback preserves the
	// same logical commit behavior for local Windows development.
	backup := target + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	_ = os.Remove(backup)
	_ = syncDirectory(directory)
	return nil
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
