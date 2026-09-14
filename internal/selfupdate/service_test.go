package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testReleaseSource struct {
	version string
	digest  string
	err     error
}

func (s *testReleaseSource) Latest(context.Context) (Release, error) {
	digest := s.digest
	if digest == "" {
		digest = "sha256:" + strings.Repeat("a", 64)
	}
	return Release{Version: s.version, ImageDigest: digest}, s.err
}

type testExecutor struct {
	installed string
	recover   error
	update    error
	targets   []string
	digests   []string
}

func (e *testExecutor) Recover(context.Context) error { return e.recover }
func (e *testExecutor) InstalledVersion(context.Context) (string, error) {
	return e.installed, nil
}
func (e *testExecutor) Update(_ context.Context, target, digest string) error {
	e.targets = append(e.targets, target)
	e.digests = append(e.digests, digest)
	if e.update == nil {
		e.installed = target
	}
	return e.update
}

func newTestService(t *testing.T, source *testReleaseSource, now *time.Time, hold time.Duration) *Service {
	t.Helper()
	service, err := New(Config{
		StateDir: t.TempDir(), Source: source, Hold: hold,
		Now: func() time.Time { return *now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func enableTestService(t *testing.T, service *Service, current string) Status {
	t.Helper()
	status, err := service.Status(current)
	if err != nil {
		t.Fatal(err)
	}
	status, err = service.SetEnabled(current, status.ResourceVersion, true)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func TestServiceDefaultsDisabledAndUsesOptimisticPolicyWrites(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	service := newTestService(t, &testReleaseSource{version: "1.1.0"}, &now, time.Hour)
	status, err := service.Status("1.0.0-dev")
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.State != "disabled" || status.CurrentVersion != "1.0.0" {
		t.Fatalf("unexpected default status: %#v", status)
	}
	updated, err := service.SetEnabled("1.0.0", status.ResourceVersion, true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled || updated.State != "idle" || updated.ResourceVersion == status.ResourceVersion {
		t.Fatalf("unexpected enabled status: %#v", updated)
	}
	if _, err := service.SetEnabled("1.0.0", status.ResourceVersion, false); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale policy write error = %v, want conflict", err)
	}
}

func TestRunObservesStableCandidateBeforeExactUpdate(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	source := &testReleaseSource{version: "1.2.0"}
	service := newTestService(t, source, &now, 24*time.Hour)
	enableTestService(t, service, "1.1.0")
	executor := &testExecutor{installed: "1.1.0"}

	status, err := service.Run(context.Background(), executor)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "waiting" || status.CandidateVersion != "1.2.0" || len(executor.targets) != 0 {
		t.Fatalf("first observation = %#v, targets=%v", status, executor.targets)
	}
	now = now.Add(23*time.Hour + 59*time.Minute)
	status, err = service.Run(context.Background(), executor)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "waiting" || len(executor.targets) != 0 {
		t.Fatalf("early observation = %#v, targets=%v", status, executor.targets)
	}
	now = now.Add(time.Minute)
	status, err = service.Run(context.Background(), executor)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "succeeded" || status.CurrentVersion != "1.2.0" || len(executor.targets) != 1 || executor.targets[0] != "1.2.0" ||
		len(executor.digests) != 1 || executor.digests[0] != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("mature candidate = %#v, targets=%v digests=%v", status, executor.targets, executor.digests)
	}
}

func TestFailedVersionIsQuarantinedUntilStableReleaseChanges(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	source := &testReleaseSource{version: "1.2.0"}
	service := newTestService(t, source, &now, time.Hour)
	enableTestService(t, service, "1.1.0")
	executor := &testExecutor{installed: "1.1.0", update: errors.New("injected failure")}
	if _, err := service.Run(context.Background(), executor); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	status, err := service.Run(context.Background(), executor)
	if err == nil || status.State != "failed" || status.FailedVersion != "1.2.0" {
		t.Fatalf("failure status=%#v err=%v", status, err)
	}
	now = now.Add(time.Hour)
	status, err = service.Run(context.Background(), executor)
	if err != nil || status.State != "blocked" || len(executor.targets) != 1 {
		t.Fatalf("quarantine status=%#v targets=%v err=%v", status, executor.targets, err)
	}
	source.digest = "sha256:" + strings.Repeat("b", 64)
	status, err = service.Run(context.Background(), executor)
	if err != nil || status.State != "waiting" || status.CandidateVersion != "1.2.0" ||
		status.CandidateImageDigest != source.digest || status.FailedVersion != "" {
		t.Fatalf("replacement digest status=%#v err=%v", status, err)
	}
	source.version = "1.2.1"
	status, err = service.Run(context.Background(), executor)
	if err != nil || status.State != "waiting" || status.CandidateVersion != "1.2.1" || status.FailedVersion != "" {
		t.Fatalf("new release status=%#v err=%v", status, err)
	}
}

func TestCheckFailureIsPersistedWithoutInstalling(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	source := &testReleaseSource{err: errors.New("offline")}
	service := newTestService(t, source, &now, time.Hour)
	status, err := service.Check(context.Background(), "1.0.0")
	if err == nil || status.State != "check_failed" || status.LastErrorCode != "release_check_failed" {
		t.Fatalf("check status=%#v err=%v", status, err)
	}
	persisted, loadErr := service.Status("1.0.0")
	if loadErr != nil || persisted.State != "check_failed" || persisted.LastCheckedAt == nil {
		t.Fatalf("persisted status=%#v err=%v", persisted, loadErr)
	}
}

func TestRunReconcilesInterruptedTransaction(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	service := newTestService(t, &testReleaseSource{version: "1.2.0"}, &now, time.Hour)
	enableTestService(t, service, "1.1.0")
	state, err := service.load()
	if err != nil {
		t.Fatal(err)
	}
	state.State = "updating"
	state.CandidateVersion = "1.2.0"
	state.CandidateImageDigest = "sha256:" + strings.Repeat("a", 64)
	if err := service.save(&state); err != nil {
		t.Fatal(err)
	}
	executor := &testExecutor{installed: "1.1.0"}
	status, err := service.Run(context.Background(), executor)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "blocked" || status.FailedVersion != "1.2.0" || status.LastErrorCode != "" {
		// A successful metadata check deliberately clears the transient error text,
		// while the failed version remains quarantined.
		t.Fatalf("interrupted status=%#v", status)
	}
}

func TestNewRejectsSymlinkedStateDirectory(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows symlink creation requires optional privileges")
	}
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{StateDir: link, Source: &testReleaseSource{}}); err == nil {
		t.Fatal("symlinked state directory was accepted")
	}
}

func TestStableVersionValidationAndOrdering(t *testing.T) {
	for _, invalid := range []string{"", "1.2", "1.2.3-rc.1", "01.2.3", "1.2.3/extra", "latest"} {
		if normalizeStableVersion(invalid) != "" {
			t.Errorf("invalid version %q was accepted", invalid)
		}
	}
	if normalizeStableVersion("v1.20.3") != "1.20.3" || compareVersions("1.10.0", "1.9.9") <= 0 {
		t.Fatal("stable version normalization or ordering failed")
	}
	if normalizeImageDigest("sha256:"+strings.Repeat("a", 64)) == "" ||
		normalizeImageDigest("sha256:"+strings.Repeat("A", 64)) != "" {
		t.Fatal("image digest validation failed")
	}
}
