package monitoring

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	CheckSchemaVersion  = 1
	MaxChecks           = 16
	MaxCheckNameRunes   = 48
	MaxCheckTargetBytes = 2048
	MaxCheckUpdateBytes = 48 << 10
	maxCheckFileBytes   = 64 << 10
)

var (
	ErrChecksConflict    = errors.New("monitoring checks changed")
	ErrChecksUnavailable = errors.New("monitoring checks unavailable")
	errChecksInvalidDisk = errors.New("monitoring checks data is invalid")
	checkIDPattern       = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)
	checkMetadataPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	checkResourcePattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type Check struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Target   string `json:"target"`
	Operator string `json:"operator,omitempty"`
	Region   string `json:"region,omitempty"`
}

type CheckSnapshot struct {
	SchemaVersion   int     `json:"schemaVersion"`
	ResourceVersion string  `json:"resourceVersion"`
	Available       bool    `json:"available"`
	Warning         string  `json:"warning,omitempty"`
	MaxItems        int     `json:"maxItems"`
	Items           []Check `json:"items"`
}

type ReplaceChecksInput struct {
	ExpectedResourceVersion string  `json:"expectedResourceVersion"`
	Items                   []Check `json:"items"`
}

type CheckValidationError struct {
	Field  string
	Detail string
}

func (e *CheckValidationError) Error() string { return e.Field + ": " + e.Detail }

type persistedChecks struct {
	SchemaVersion int     `json:"schemaVersion"`
	Items         []Check `json:"items"`
}

type checkStore struct {
	path      string
	available bool
	state     persistedChecks
}

func openCheckStore(root string) (*checkStore, error) {
	store := &checkStore{
		path:      filepath.Join(root, "checks.json"),
		available: true,
		state:     defaultCheckState(),
	}
	state, err := readChecks(store.path)
	switch {
	case err == nil:
		store.state = state
		if err := os.Chmod(store.path, 0o600); err != nil {
			return nil, fmt.Errorf("protect monitoring checks: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
		if err := store.persist(store.state); err != nil {
			return nil, err
		}
	case errors.Is(err, errChecksInvalidDisk):
		store.available = false
		store.state = persistedChecks{SchemaVersion: CheckSchemaVersion, Items: []Check{}}
	default:
		return nil, err
	}
	return store, nil
}

func DefaultChecks() []Check {
	return []Check{
		{ID: "telecom-beijing", Kind: "ping", Name: "电信 · 北京", Target: "219.141.136.10", Operator: "telecom", Region: "beijing"},
		{ID: "telecom-shanghai", Kind: "ping", Name: "电信 · 上海", Target: "202.96.209.5", Operator: "telecom", Region: "shanghai"},
		{ID: "telecom-guangzhou", Kind: "ping", Name: "电信 · 广州", Target: "202.96.128.86", Operator: "telecom", Region: "guangzhou"},
		{ID: "unicom-beijing", Kind: "ping", Name: "联通 · 北京", Target: "123.125.81.6", Operator: "unicom", Region: "beijing"},
		{ID: "unicom-shanghai", Kind: "ping", Name: "联通 · 上海", Target: "210.22.84.3", Operator: "unicom", Region: "shanghai"},
		{ID: "unicom-guangzhou", Kind: "ping", Name: "联通 · 广州", Target: "210.21.196.6", Operator: "unicom", Region: "guangzhou"},
		{ID: "mobile-beijing", Kind: "ping", Name: "移动 · 北京", Target: "221.179.155.161", Operator: "mobile", Region: "beijing"},
		{ID: "mobile-shanghai", Kind: "ping", Name: "移动 · 上海", Target: "211.136.112.50", Operator: "mobile", Region: "shanghai"},
		{ID: "mobile-guangzhou", Kind: "ping", Name: "移动 · 广州", Target: "211.136.192.6", Operator: "mobile", Region: "guangzhou"},
	}
}

func ValidCheckResourceVersion(value string) bool { return checkResourcePattern.MatchString(value) }

func (s *checkStore) snapshot() CheckSnapshot {
	state := s.state
	if !s.available {
		state = persistedChecks{SchemaVersion: CheckSchemaVersion, Items: []Check{}}
	}
	result := CheckSnapshot{
		SchemaVersion: CheckSchemaVersion, ResourceVersion: checkResourceVersion(state),
		Available: s.available, MaxItems: MaxChecks, Items: cloneChecks(state.Items),
	}
	if !s.available {
		result.Warning = "monitoring_checks_unavailable"
	}
	return result
}

func (s *checkStore) replace(input ReplaceChecksInput) (CheckSnapshot, error) {
	if !s.available {
		return CheckSnapshot{}, ErrChecksUnavailable
	}
	if !ValidCheckResourceVersion(input.ExpectedResourceVersion) {
		return CheckSnapshot{}, &CheckValidationError{Field: "expectedResourceVersion", Detail: "a valid resourceVersion is required"}
	}
	if input.ExpectedResourceVersion != checkResourceVersion(s.state) {
		return CheckSnapshot{}, ErrChecksConflict
	}
	next := persistedChecks{SchemaVersion: CheckSchemaVersion, Items: cloneChecks(input.Items)}
	if err := validateChecks(next); err != nil {
		return CheckSnapshot{}, err
	}
	if err := s.persist(next); err != nil {
		return CheckSnapshot{}, err
	}
	s.state = next
	return s.snapshot(), nil
}

func defaultCheckState() persistedChecks {
	return persistedChecks{SchemaVersion: CheckSchemaVersion, Items: DefaultChecks()}
}

func readChecks(path string) (persistedChecks, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return persistedChecks{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return persistedChecks{}, errors.New("monitoring checks must be a regular file")
	}
	if info.Size() <= 0 || info.Size() > maxCheckFileBytes {
		return persistedChecks{}, errChecksInvalidDisk
	}
	file, err := os.Open(path)
	if err != nil {
		return persistedChecks{}, fmt.Errorf("open monitoring checks: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCheckFileBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxCheckFileBytes {
		return persistedChecks{}, errChecksInvalidDisk
	}
	var state persistedChecks
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return persistedChecks{}, errChecksInvalidDisk
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return persistedChecks{}, errChecksInvalidDisk
	}
	if err := validateChecks(state); err != nil {
		return persistedChecks{}, errChecksInvalidDisk
	}
	state.Items = cloneChecks(state.Items)
	return state, nil
}

func (s *checkStore) persist(state persistedChecks) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode monitoring checks: %w", err)
	}
	data = append(data, '\n')
	if len(data) > maxCheckFileBytes {
		return &CheckValidationError{Field: "items", Detail: "monitoring checks exceed the storage limit"}
	}
	if err := writeChecksAtomic(filepath.Dir(s.path), s.path, data); err != nil {
		return fmt.Errorf("persist monitoring checks: %w", err)
	}
	return nil
}

func validateChecks(state persistedChecks) error {
	if state.SchemaVersion != CheckSchemaVersion {
		return &CheckValidationError{Field: "schemaVersion", Detail: "unsupported monitoring checks schema"}
	}
	if len(state.Items) > MaxChecks {
		return &CheckValidationError{Field: "items", Detail: "at most 16 monitoring checks are allowed"}
	}
	seen := make(map[string]bool, len(state.Items))
	for _, item := range state.Items {
		if !checkIDPattern.MatchString(item.ID) || seen[item.ID] {
			return &CheckValidationError{Field: "items.id", Detail: "IDs must be unique lowercase letters, numbers, or hyphens"}
		}
		seen[item.ID] = true
		if (item.Operator != "" && !checkMetadataPattern.MatchString(item.Operator)) ||
			(item.Region != "" && !checkMetadataPattern.MatchString(item.Region)) {
			return &CheckValidationError{Field: "items", Detail: "operator and region metadata are invalid"}
		}
		if strings.TrimSpace(item.Name) != item.Name || item.Name == "" || !utf8.ValidString(item.Name) ||
			utf8.RuneCountInString(item.Name) > MaxCheckNameRunes || hasCheckControl(item.Name) {
			return &CheckValidationError{Field: "items.name", Detail: "name must contain 1 to 48 characters without controls or outer whitespace"}
		}
		if strings.TrimSpace(item.Target) != item.Target || item.Target == "" || len(item.Target) > MaxCheckTargetBytes || hasCheckControl(item.Target) {
			return &CheckValidationError{Field: "items.target", Detail: "target is invalid"}
		}
		switch item.Kind {
		case "ping":
			if ip := net.ParseIP(item.Target); ip == nil || ip.To4() == nil {
				return &CheckValidationError{Field: "items.target", Detail: "Ping target must be an IPv4 address"}
			}
		case "tcp":
			host, port, err := net.SplitHostPort(item.Target)
			value, parseErr := strconv.Atoi(port)
			if err != nil || strings.TrimSpace(host) == "" || strings.ContainsAny(host, " \t\r\n") || parseErr != nil || value < 1 || value > 65535 {
				return &CheckValidationError{Field: "items.target", Detail: "TCP target must use host:port"}
			}
		case "http":
			parsed, err := url.ParseRequestURI(item.Target)
			if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				return &CheckValidationError{Field: "items.target", Detail: "HTTP target must be an absolute http or https URL without credentials"}
			}
		default:
			return &CheckValidationError{Field: "items.kind", Detail: "kind must be ping, tcp, or http"}
		}
	}
	return nil
}

func hasCheckControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func cloneChecks(items []Check) []Check {
	cloned := make([]Check, len(items))
	copy(cloned, items)
	return cloned
}

func checkResourceVersion(state persistedChecks) string {
	canonical := persistedChecks{SchemaVersion: CheckSchemaVersion, Items: cloneChecks(state.Items)}
	data, _ := json.Marshal(canonical)
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func writeChecksAtomic(directory, target string, data []byte) error {
	file, err := os.CreateTemp(directory, ".monitoring-checks-*")
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
	if err := os.Rename(temporary, target); err == nil {
		_ = syncCheckDirectory(directory)
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
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
	_ = syncCheckDirectory(directory)
	return nil
}

func syncCheckDirectory(path string) error {
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
