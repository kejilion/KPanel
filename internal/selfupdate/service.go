package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	stateSchemaVersion = 1
	stateFileName      = "state.json"
	lockFileName       = ".lock"
	maxStateBytes      = 64 << 10
	defaultHold        = 24 * time.Hour
)

var (
	ErrBusy     = errors.New("automatic update state is busy")
	ErrConflict = errors.New("automatic update state changed")
)

type Release struct {
	Version     string
	ImageDigest string
}

// ReleaseSource returns the current stable release and its immutable image
// digest. Implementations must reject drafts, prereleases and mutable image
// references.
type ReleaseSource interface {
	Latest(context.Context) (Release, error)
}

// Executor owns the privileged, host-specific part of an update. Recovery is
// deliberately called before policy evaluation so a disabled timer can still
// finish a transaction interrupted by a power loss.
type Executor interface {
	Recover(context.Context) error
	InstalledVersion(context.Context) (string, error)
	Update(context.Context, string, string) error
}

type Config struct {
	StateDir    string
	Source      ReleaseSource
	Now         func() time.Time
	Hold        time.Duration
	Schedule    string
	ChannelName string
}

type Service struct {
	stateDir string
	source   ReleaseSource
	now      func() time.Time
	hold     time.Duration
	schedule string
	channel  string
	mu       sync.Mutex
}

type persistedState struct {
	SchemaVersion        int        `json:"schemaVersion"`
	Revision             uint64     `json:"revision"`
	Enabled              bool       `json:"enabled"`
	State                string     `json:"state"`
	CurrentVersion       string     `json:"currentVersion,omitempty"`
	CandidateVersion     string     `json:"candidateVersion,omitempty"`
	CandidateImageDigest string     `json:"candidateImageDigest,omitempty"`
	CandidateFirstSeenAt *time.Time `json:"candidateFirstSeenAt,omitempty"`
	LastCheckedAt        *time.Time `json:"lastCheckedAt,omitempty"`
	LastAttemptAt        *time.Time `json:"lastAttemptAt,omitempty"`
	LastSuccessAt        *time.Time `json:"lastSuccessAt,omitempty"`
	LastErrorCode        string     `json:"lastErrorCode,omitempty"`
	LastError            string     `json:"lastError,omitempty"`
	FailedVersion        string     `json:"failedVersion,omitempty"`
	FailedImageDigest    string     `json:"failedImageDigest,omitempty"`
}

type Status struct {
	Available            bool       `json:"available"`
	Enabled              bool       `json:"enabled"`
	State                string     `json:"state"`
	Channel              string     `json:"channel"`
	Schedule             string     `json:"schedule"`
	ObservationHours     int        `json:"observationHours"`
	CurrentVersion       string     `json:"currentVersion,omitempty"`
	CandidateVersion     string     `json:"candidateVersion,omitempty"`
	CandidateImageDigest string     `json:"candidateImageDigest,omitempty"`
	CandidateFirstSeenAt *time.Time `json:"candidateFirstSeenAt,omitempty"`
	LastCheckedAt        *time.Time `json:"lastCheckedAt,omitempty"`
	LastAttemptAt        *time.Time `json:"lastAttemptAt,omitempty"`
	LastSuccessAt        *time.Time `json:"lastSuccessAt,omitempty"`
	LastErrorCode        string     `json:"lastErrorCode,omitempty"`
	LastError            string     `json:"lastError,omitempty"`
	FailedVersion        string     `json:"failedVersion,omitempty"`
	FailedImageDigest    string     `json:"failedImageDigest,omitempty"`
	ResourceVersion      string     `json:"resourceVersion"`
}

func New(config Config) (*Service, error) {
	config.StateDir = filepath.Clean(strings.TrimSpace(config.StateDir))
	if config.StateDir == "." || !filepath.IsAbs(config.StateDir) {
		return nil, errors.New("automatic update state directory must be an absolute path")
	}
	if config.Source == nil {
		return nil, errors.New("automatic update release source is required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.Hold == 0 {
		config.Hold = defaultHold
	}
	if config.Hold < 0 || config.Hold > 7*24*time.Hour {
		return nil, errors.New("automatic update observation period is invalid")
	}
	if config.Schedule == "" {
		config.Schedule = "daily-04:00-local"
	}
	if config.ChannelName == "" {
		config.ChannelName = "stable"
	}
	if err := ensurePrivateDirectory(config.StateDir); err != nil {
		return nil, fmt.Errorf("prepare automatic update state: %w", err)
	}
	service := &Service{
		stateDir: config.StateDir, source: config.Source, now: config.Now,
		hold: config.Hold, schedule: config.Schedule, channel: config.ChannelName,
	}
	if _, err := service.load(); err != nil {
		return nil, fmt.Errorf("load automatic update state: %w", err)
	}
	return service, nil
}

func (s *Service) Status(currentVersion string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.load()
	if err != nil {
		return Status{}, err
	}
	result := s.snapshot(state)
	if currentVersion = normalizeCurrentVersion(currentVersion); currentVersion != "" {
		result.CurrentVersion = currentVersion
	}
	return result, nil
}

func (s *Service) SetEnabled(currentVersion, expectedResourceVersion string, enabled bool) (Status, error) {
	var result Status
	err := s.exclusive(func() error {
		state, err := s.load()
		if err != nil {
			return err
		}
		if expectedResourceVersion == "" || expectedResourceVersion != resourceVersion(state) {
			return ErrConflict
		}
		state.Enabled = enabled
		state.CurrentVersion = normalizeCurrentVersion(currentVersion)
		state.LastError = ""
		state.LastErrorCode = ""
		if enabled {
			if state.State == "disabled" || state.State == "" {
				state.State = "idle"
			}
		} else {
			state.State = "disabled"
		}
		if err := s.save(&state); err != nil {
			return err
		}
		result = s.snapshot(state)
		return nil
	})
	return result, err
}

// Check refreshes stable release metadata but never installs an update.
func (s *Service) Check(ctx context.Context, currentVersion string) (Status, error) {
	var result Status
	err := s.exclusive(func() error {
		state, err := s.load()
		if err != nil {
			return err
		}
		state.CurrentVersion = normalizeCurrentVersion(currentVersion)
		if state.CurrentVersion == "" {
			return errors.New("current KPanel version is invalid")
		}
		checkErr := s.checkLocked(ctx, &state)
		if saveErr := s.save(&state); saveErr != nil {
			return saveErr
		}
		result = s.snapshot(state)
		return checkErr
	})
	return result, err
}

// Run is the systemd timer entry point. It serializes with settings writes and
// manual checks, recovers stale transactions, observes a stable candidate for
// the configured hold period, and invokes the exact-version executor once.
func (s *Service) Run(ctx context.Context, executor Executor) (Status, error) {
	if executor == nil {
		return Status{}, errors.New("automatic update executor is required")
	}
	var result Status
	err := s.exclusive(func() error {
		state, err := s.load()
		if err != nil {
			return err
		}
		wasUpdating := state.State == "updating" && state.CandidateVersion != ""
		if err := executor.Recover(ctx); err != nil {
			now := s.utcNow()
			state.State = "failed"
			state.LastAttemptAt = &now
			state.LastErrorCode = "recovery_failed"
			state.LastError = boundedError(err)
			_ = s.save(&state)
			result = s.snapshot(state)
			return fmt.Errorf("recover interrupted automatic update: %w", err)
		}
		installed, err := executor.InstalledVersion(ctx)
		if err != nil {
			return fmt.Errorf("read installed KPanel version: %w", err)
		}
		state.CurrentVersion = normalizeCurrentVersion(installed)
		if state.CurrentVersion == "" {
			return errors.New("installed KPanel version is invalid")
		}
		if wasUpdating {
			s.reconcileInterrupted(&state)
			if err := s.save(&state); err != nil {
				return err
			}
		}
		if !state.Enabled {
			state.State = "disabled"
			if err := s.save(&state); err != nil {
				return err
			}
			result = s.snapshot(state)
			return nil
		}
		if err := s.checkLocked(ctx, &state); err != nil {
			if saveErr := s.save(&state); saveErr != nil {
				return saveErr
			}
			result = s.snapshot(state)
			return err
		}
		if state.State != "available" || state.CandidateVersion == "" {
			if err := s.save(&state); err != nil {
				return err
			}
			result = s.snapshot(state)
			return nil
		}
		target := state.CandidateVersion
		digest := state.CandidateImageDigest
		now := s.utcNow()
		state.State = "updating"
		state.LastAttemptAt = &now
		state.LastErrorCode = ""
		state.LastError = ""
		if err := s.save(&state); err != nil {
			return err
		}
		if err := executor.Update(ctx, target, digest); err != nil {
			state.State = "failed"
			state.FailedVersion = target
			state.FailedImageDigest = digest
			state.LastErrorCode = "update_failed"
			state.LastError = boundedError(err)
			if saveErr := s.save(&state); saveErr != nil {
				return errors.Join(err, saveErr)
			}
			result = s.snapshot(state)
			return fmt.Errorf("automatic update to %s failed: %w", target, err)
		}
		now = s.utcNow()
		state.State = "succeeded"
		state.CurrentVersion = target
		state.CandidateVersion = ""
		state.CandidateImageDigest = ""
		state.CandidateFirstSeenAt = nil
		state.FailedVersion = ""
		state.FailedImageDigest = ""
		state.LastErrorCode = ""
		state.LastError = ""
		state.LastSuccessAt = &now
		if err := s.save(&state); err != nil {
			return err
		}
		result = s.snapshot(state)
		return nil
	})
	return result, err
}

func (s *Service) reconcileInterrupted(state *persistedState) {
	now := s.utcNow()
	if compareVersions(state.CurrentVersion, state.CandidateVersion) >= 0 {
		state.State = "succeeded"
		state.LastSuccessAt = &now
		state.CandidateVersion = ""
		state.CandidateImageDigest = ""
		state.CandidateFirstSeenAt = nil
		state.FailedVersion = ""
		state.FailedImageDigest = ""
		state.LastErrorCode = ""
		state.LastError = ""
		return
	}
	state.State = "failed"
	state.FailedVersion = state.CandidateVersion
	state.FailedImageDigest = state.CandidateImageDigest
	state.LastErrorCode = "update_interrupted"
	state.LastError = "the previous automatic update was interrupted and recovery restored the prior version"
}

func (s *Service) checkLocked(ctx context.Context, state *persistedState) error {
	now := s.utcNow()
	latest, err := s.source.Latest(ctx)
	state.LastCheckedAt = &now
	if err != nil {
		state.State = "check_failed"
		state.LastErrorCode = "release_check_failed"
		state.LastError = boundedError(err)
		return fmt.Errorf("check stable KPanel release: %w", err)
	}
	latest.Version = normalizeStableVersion(latest.Version)
	latest.ImageDigest = normalizeImageDigest(latest.ImageDigest)
	if latest.Version == "" || latest.ImageDigest == "" {
		state.State = "check_failed"
		state.LastErrorCode = "invalid_release"
		state.LastError = "the stable release endpoint returned an invalid version or image digest"
		return errors.New("stable release endpoint returned invalid metadata")
	}
	state.LastErrorCode = ""
	state.LastError = ""
	if compareVersions(latest.Version, state.CurrentVersion) <= 0 {
		state.State = "idle"
		state.CandidateVersion = ""
		state.CandidateImageDigest = ""
		state.CandidateFirstSeenAt = nil
		if compareVersions(state.FailedVersion, state.CurrentVersion) <= 0 {
			state.FailedVersion = ""
			state.FailedImageDigest = ""
		}
		return nil
	}
	if state.CandidateVersion != latest.Version || state.CandidateImageDigest != latest.ImageDigest {
		state.CandidateVersion = latest.Version
		state.CandidateImageDigest = latest.ImageDigest
		state.CandidateFirstSeenAt = &now
		if state.FailedVersion != latest.Version || state.FailedImageDigest != latest.ImageDigest {
			state.FailedVersion = ""
			state.FailedImageDigest = ""
		}
		state.State = "waiting"
		return nil
	}
	if state.FailedVersion == latest.Version && state.FailedImageDigest == latest.ImageDigest {
		state.State = "blocked"
		return nil
	}
	if state.CandidateFirstSeenAt == nil {
		state.CandidateFirstSeenAt = &now
		state.State = "waiting"
		return nil
	}
	if now.Sub(state.CandidateFirstSeenAt.UTC()) < s.hold {
		state.State = "waiting"
		return nil
	}
	state.State = "available"
	return nil
}

func (s *Service) exclusive(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := acquireFileLock(filepath.Join(s.stateDir, lockFileName))
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

func (s *Service) snapshot(state persistedState) Status {
	return Status{
		Available: true, Enabled: state.Enabled, State: state.State,
		Channel: s.channel, Schedule: s.schedule,
		ObservationHours: int(s.hold / time.Hour), CurrentVersion: state.CurrentVersion,
		CandidateVersion: state.CandidateVersion, CandidateImageDigest: state.CandidateImageDigest,
		CandidateFirstSeenAt: state.CandidateFirstSeenAt,
		LastCheckedAt:        state.LastCheckedAt, LastAttemptAt: state.LastAttemptAt,
		LastSuccessAt: state.LastSuccessAt, LastErrorCode: state.LastErrorCode,
		LastError: state.LastError, FailedVersion: state.FailedVersion,
		FailedImageDigest: state.FailedImageDigest,
		ResourceVersion:   resourceVersion(state),
	}
}

func (s *Service) load() (persistedState, error) {
	state := persistedState{SchemaVersion: stateSchemaVersion, State: "disabled"}
	path := filepath.Join(s.stateDir, stateFileName)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		!trustedStateFile(info) || info.Size() > maxStateBytes {
		return state, errors.New("automatic update state file is unsafe")
	}
	file, err := os.Open(path)
	if err != nil {
		return state, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return persistedState{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return persistedState{}, errors.New("automatic update state contains multiple values")
		}
		return persistedState{}, err
	}
	if state.SchemaVersion != stateSchemaVersion || !validPersistedState(state) {
		return persistedState{}, errors.New("automatic update state is invalid")
	}
	return state, nil
}

func (s *Service) save(state *persistedState) error {
	state.SchemaVersion = stateSchemaVersion
	state.Revision++
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxStateBytes {
		return errors.New("automatic update state exceeds the size limit")
	}
	temporary, err := os.CreateTemp(s.stateDir, ".state-*.tmp")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := replaceFile(temporaryName, filepath.Join(s.stateDir, stateFileName)); err != nil {
		return err
	}
	return syncDirectory(s.stateDir)
}

func ensurePrivateDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !trustedFileOwner(info) {
			return errors.New("state path is not a real directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || !trustedStateDirectory(info) {
		return errors.New("state path is not private")
	}
	return nil
}

func replaceFile(source, target string) error {
	if err := os.Rename(source, target); err == nil {
		return nil
	}
	backup := target + ".previous"
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	return os.Remove(backup)
}

func validPersistedState(state persistedState) bool {
	validStates := map[string]bool{
		"disabled": true, "idle": true, "waiting": true, "available": true,
		"updating": true, "succeeded": true, "failed": true, "blocked": true,
		"check_failed": true,
	}
	if !validStates[state.State] || len(state.LastError) > 512 || len(state.LastErrorCode) > 64 {
		return false
	}
	for _, candidate := range []string{state.CurrentVersion, state.CandidateVersion, state.FailedVersion} {
		if candidate != "" && normalizeStableVersion(candidate) == "" {
			return false
		}
	}
	for _, digest := range []string{state.CandidateImageDigest, state.FailedImageDigest} {
		if digest != "" && normalizeImageDigest(digest) == "" {
			return false
		}
	}
	if (state.CandidateVersion == "") != (state.CandidateImageDigest == "") ||
		(state.FailedVersion == "") != (state.FailedImageDigest == "") {
		return false
	}
	return true
}

func resourceVersion(state persistedState) string {
	data, _ := json.Marshal(state)
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (s *Service) utcNow() time.Time { return s.now().UTC() }

func boundedError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}
