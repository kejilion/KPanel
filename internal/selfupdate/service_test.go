package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	if err != nil && runtime.GOOS == "linux" && strings.Contains(err.Error(), "state path is not a real directory") {
		t.Skip("automatic update state tests require a root-owned state directory on Linux")
	}
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

func TestPreviewPolicyPersistsAndReturningStableNeverDowngrades(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	stateDir := t.TempDir()
	stableSource := &testReleaseSource{version: "1.9.0"}
	previewSource := &testReleaseSource{version: "2.0.0-rc.2", digest: "sha256:" + strings.Repeat("b", 64)}
	service, err := New(Config{
		StateDir: stateDir, StableSource: stableSource, PreviewSource: previewSource,
		Now: func() time.Time { return now }, Hold: 24 * time.Hour,
	})
	if err != nil && runtime.GOOS == "linux" && strings.Contains(err.Error(), "state path is not a real directory") {
		t.Skip("automatic update state tests require a root-owned state directory on Linux")
	}
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.Status("1.9.0")
	if err != nil {
		t.Fatal(err)
	}
	status, err = service.SetPolicy("1.9.0", status.ResourceVersion, false, ChannelPreview)
	if err != nil || status.Channel != ChannelPreview || status.Enabled {
		t.Fatalf("preview policy status=%#v err=%v", status, err)
	}
	status, err = service.Check(context.Background(), "1.9.0")
	if err != nil || status.CandidateVersion != "2.0.0-rc.2" || !status.CanInstall {
		t.Fatalf("preview check status=%#v err=%v", status, err)
	}

	reloaded, err := New(Config{
		StateDir: stateDir, StableSource: stableSource, PreviewSource: previewSource,
		Now: func() time.Time { return now }, Hold: 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err = reloaded.Status("2.0.0-rc.2")
	if err != nil || status.Channel != ChannelPreview {
		t.Fatalf("reloaded status=%#v err=%v", status, err)
	}
	status, err = reloaded.SetPolicy("2.0.0-rc.2", status.ResourceVersion, false, ChannelStable)
	if err != nil {
		t.Fatal(err)
	}
	status, err = reloaded.Check(context.Background(), "2.0.0-rc.2")
	if err != nil || status.CandidateVersion != "" || status.Channel != ChannelStable || status.State != "disabled" {
		t.Fatalf("stable return status=%#v err=%v", status, err)
	}
}

func TestQueuedInstallBypassesHoldWithoutEnablingFutureUpdates(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	service := newTestService(t, &testReleaseSource{version: "1.2.0"}, &now, 24*time.Hour)
	status, err := service.Check(context.Background(), "1.1.0")
	if err != nil || !status.CanInstall || status.Enabled {
		t.Fatalf("checked status=%#v err=%v", status, err)
	}
	status, err = service.QueueInstall("1.1.0", status.ResourceVersion)
	if err != nil || status.State != "queued" || !status.InstallRequested || status.CanInstall {
		t.Fatalf("queued status=%#v err=%v", status, err)
	}
	executor := &testExecutor{installed: "1.1.0"}
	status, err = service.Run(context.Background(), executor)
	if err != nil || status.State != "succeeded" || status.Enabled || status.InstallRequested ||
		len(executor.targets) != 1 || executor.targets[0] != "1.2.0" {
		t.Fatalf("manual run status=%#v targets=%v err=%v", status, executor.targets, err)
	}
	status, err = service.Run(context.Background(), executor)
	if err != nil || status.State != "disabled" || len(executor.targets) != 1 {
		t.Fatalf("future run status=%#v targets=%v err=%v", status, executor.targets, err)
	}
}

func TestDisablingAutomaticInstallPreservesManualCandidate(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	service := newTestService(t, &testReleaseSource{version: "1.2.0"}, &now, 24*time.Hour)
	status, err := service.Check(context.Background(), "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	status, err = service.SetPolicy("1.1.0", status.ResourceVersion, true, ChannelStable)
	if err != nil || !status.Enabled || !status.CanInstall {
		t.Fatalf("enabled status=%#v err=%v", status, err)
	}
	status, err = service.SetPolicy("1.1.0", status.ResourceVersion, false, ChannelStable)
	if err != nil || status.Enabled || status.State != "waiting" || !status.CanInstall ||
		status.CandidateVersion != "1.2.0" {
		t.Fatalf("disabled status=%#v err=%v", status, err)
	}
}

func TestCancelQueuedInstallRecordsDispatchFailureWithoutStickyRequest(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	service := newTestService(t, &testReleaseSource{version: "1.2.0"}, &now, time.Hour)
	status, err := service.Check(context.Background(), "1.1.0")
	if err != nil {
		t.Fatal(err)
	}
	status, err = service.QueueInstall("1.1.0", status.ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	status, err = service.CancelQueuedInstall(status.CandidateVersion, status.CandidateImageDigest, errors.New("systemd offline"))
	if err != nil || status.InstallRequested || status.State != "waiting" ||
		status.LastErrorCode != "install_start_failed" || !status.CanInstall {
		t.Fatalf("cancelled status=%#v err=%v", status, err)
	}
}

func TestStateSchemaOneMigratesToStableChannel(t *testing.T) {
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	service := newTestService(t, &testReleaseSource{version: "1.2.0"}, &now, time.Hour)
	legacy := `{"schemaVersion":1,"revision":7,"enabled":true,"state":"idle","currentVersion":"1.1.0"}`
	path := filepath.Join(service.stateDir, stateFileName)
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	status, err := service.Status("1.1.0")
	if err != nil || status.Channel != ChannelStable || !status.Enabled || status.State != "idle" {
		t.Fatalf("migrated status=%#v err=%v", status, err)
	}
	status, err = service.SetPolicy("1.1.0", status.ResourceVersion, true, ChannelStable)
	if err != nil || status.Channel != ChannelStable {
		t.Fatalf("saved migrated status=%#v err=%v", status, err)
	}
	persisted, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(persisted), `"schemaVersion": 2`) ||
		!strings.Contains(string(persisted), `"channel": "stable"`) {
		t.Fatalf("persisted migration=%s err=%v", persisted, err)
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
	for _, valid := range []string{"1.2.3", "v1.2.3", "1.2.3-rc.1", "v2.0.0-rc.12"} {
		if normalizeReleaseVersion(valid) == "" {
			t.Errorf("valid release version %q was rejected", valid)
		}
	}
	for _, invalid := range []string{"1.2.3-rc.0", "1.2.3-rc.01", "1.2.3-beta.1", "1.2.3-rc.1-extra"} {
		if normalizeReleaseVersion(invalid) != "" {
			t.Errorf("invalid release version %q was accepted", invalid)
		}
	}
	if compareVersions("2.0.0", "2.0.0-rc.9") <= 0 ||
		compareVersions("2.0.0-rc.10", "2.0.0-rc.2") <= 0 ||
		compareVersions("2.0.0-rc.1", "1.99.99") <= 0 {
		t.Fatal("release candidate ordering failed")
	}
	if normalizeImageDigest("sha256:"+strings.Repeat("a", 64)) == "" ||
		normalizeImageDigest("sha256:"+strings.Repeat("A", 64)) != "" {
		t.Fatal("image digest validation failed")
	}
}
