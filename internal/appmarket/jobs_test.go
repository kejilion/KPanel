package appmarket

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeJobRunner struct {
	mu        sync.Mutex
	calls     [][]string
	unitState string
	unitErr   error
}

func (runner *fakeJobRunner) Run(_ context.Context, name string, arguments ...string) ([]byte, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	call := append([]string{name}, arguments...)
	runner.calls = append(runner.calls, call)
	if name == "systemctl" {
		if runner.unitErr != nil {
			return nil, runner.unitErr
		}
		state := runner.unitState
		if state == "" {
			state = "inactive"
		}
		return []byte(state + "\n"), nil
	}
	return nil, nil
}

func (*fakeJobRunner) LookPath(name string) (string, error) {
	if name != "systemd-run" {
		return "", errors.New("not found")
	}
	return "/usr/bin/systemd-run", nil
}

func TestDeclarativeInstallRunsAsPersistentBackgroundJob(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeJobRunner{}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}

	job, err := service.StartInstall(context.Background(), "builtin-28", InstallInput{
		HostPort:   18028,
		AccessMode: "domain_only",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "queued" || job.Progress != 0 {
		t.Fatalf("initial job = %#v", job)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		job, err = service.AppJob(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == "succeeded" {
			break
		}
		if job.Status == "failed" || time.Now().After(deadline) {
			record, _ := service.jobs.read(job.ID)
			t.Fatalf("background job did not succeed: public=%#v record=%#v launches=%#v", job, record, runner.calls)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.Progress != 100 || job.Stage != "completed" {
		t.Fatalf("completed job = %#v", job)
	}
	if len(service.AppJobs()) != 1 {
		t.Fatalf("job history count = %d", len(service.AppJobs()))
	}
}

func TestKejilionStandardAppsBecomeDirectlyInstallable(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeJobRunner{}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) {
		return "/usr/local/bin/k", nil
	}

	item, err := service.Find(context.Background(), "builtin-4")
	if err != nil {
		t.Fatal(err)
	}
	if item.Installer != "kejilion" || !item.Capabilities["install"].Enabled {
		t.Fatalf("standard app is not directly installable: %#v", item)
	}
	job, err := service.StartInstall(context.Background(), item.ID, InstallInput{HostPort: 18081})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.jobs.read(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Selector != "4" || record.Adapter != "kejilion" || record.HostPort != 18081 {
		t.Fatalf("unsafe or incomplete script job record: %#v", record)
	}
	if !record.Interactive {
		t.Fatal("kejilion.sh install was not routed through the interactive terminal")
	}
	if len(runner.calls) != 1 || runner.calls[0][0] != "systemd-run" ||
		!strings.Contains(strings.Join(runner.calls[0], " "), appJobUnitPrefix+job.ID) ||
		!strings.Contains(strings.Join(runner.calls[0], " "), "app-pty-run") ||
		!strings.Contains(
			strings.Join(runner.calls[0], " "),
			"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK",
		) {
		t.Fatalf("background launch = %#v", runner.calls)
	}
	if _, err := service.StartInstall(context.Background(), "builtin-28", InstallInput{}); !errors.Is(err, ErrTaskConflict) {
		t.Fatalf("parallel install error = %v, want conflict", err)
	}

	restarted, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	recovered, err := restarted.AppJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Status != "failed" || recovered.Stage != "interrupted" {
		t.Fatalf("interrupted background job was not recovered: %#v", recovered)
	}
}

func TestInteractiveApplicationJobCanBeEndedAndReleasesTaskLock(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeJobRunner{unitState: "active"}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	id := "0123456789abcdef0123456789abcdef"
	created := time.Now().Add(-time.Minute).UTC()
	record := appJobRecord{
		AppJob: AppJob{
			ID: id, AppID: "builtin-114", AppName: "OpenClaw",
			Action: "manage", Interactive: true, InputOpen: true,
			Status: "running", Stage: "interactive", Progress: 5, CreatedAt: created,
		},
		Selector: "114", Adapter: "kejilion",
	}
	if err := service.jobs.put(record); err != nil {
		t.Fatal(err)
	}
	if err := createTerminalInput(service.jobs.inputPath(id)); err != nil {
		t.Fatal(err)
	}

	job, err := service.CancelAppJob(id)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "running" || job.Stage != "cancelling" || job.InputOpen {
		t.Fatalf("cancellation request = %#v", job)
	}
	if !service.jobs.cancelRequested(id) {
		t.Fatal("cancellation marker was not persisted")
	}
	if _, err := service.StartInstall(context.Background(), "builtin-28", InstallInput{}); !errors.Is(err, ErrTaskConflict) {
		t.Fatalf("active cancellation did not retain task lock: %v", err)
	}
	runner.mu.Lock()
	calls := append([][]string(nil), runner.calls...)
	runner.mu.Unlock()
	foundStop := false
	for _, call := range calls {
		if strings.Join(call, " ") ==
			"systemctl stop --no-block "+appJobUnitPrefix+id+".service" {
			foundStop = true
			break
		}
	}
	if !foundStop {
		t.Fatalf("systemd stop call = %#v", calls)
	}

	runner.unitState = "inactive"
	job, err = service.AppJob(id)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "cancelled" || job.Stage != "cancelled" ||
		job.Progress != 100 || job.FinishedAt == nil {
		t.Fatalf("cancelled job = %#v", job)
	}
	if service.jobs.hasActive() {
		t.Fatal("cancelled task lock was not released")
	}
	if service.jobs.cancelRequested(id) {
		t.Fatal("cancellation marker was not removed")
	}
}

func TestNonInteractiveApplicationJobCannotBeEndedManually(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeJobRunner{unitState: "active"}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	id := "fedcba9876543210fedcba9876543210"
	if err := service.jobs.put(appJobRecord{
		AppJob: AppJob{
			ID: id, AppID: "thirdparty-test", AppName: "Test",
			Action: "install", Status: "running", Stage: "installing",
			Progress: 35, CreatedAt: time.Now().UTC(),
		},
		Adapter: "declarative",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := service.CancelAppJob(id); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-interactive cancellation error = %v", err)
	}
}

func TestKejilionGuidedAppsUseFixedInteractiveSelector(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeJobRunner{}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) {
		return "/usr/local/bin/k", nil
	}

	for _, appID := range []string{
		"builtin-1",
		"builtin-2",
		"builtin-3",
		"builtin-7",
		"builtin-9",
		"builtin-19",
		"builtin-38",
		"builtin-41",
		"builtin-51",
		"builtin-54",
		"builtin-55",
		"builtin-56",
		"builtin-66",
		"builtin-104",
		"builtin-114",
		"builtin-115",
		"builtin-116",
	} {
		item, findErr := service.Find(context.Background(), appID)
		if findErr != nil {
			t.Fatal(findErr)
		}
		if item.Installer != "kejilion" || !item.Capabilities["install"].Enabled {
			t.Fatalf("guided application %s is not interactive: %#v", appID, item)
		}
	}

	job, err := service.StartInstall(
		context.Background(),
		"builtin-7",
		InstallInput{},
	)
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.jobs.read(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Selector != "7" || record.Adapter != "kejilion" ||
		!record.Interactive {
		t.Fatalf("guided application escaped the fixed selector terminal: %#v", record)
	}
	if len(runner.calls) != 1 ||
		!strings.Contains(strings.Join(runner.calls[0], " "), "app-pty-run") {
		t.Fatalf("guided application launch = %#v", runner.calls)
	}
}

func TestKejilionInstallCapabilityRequiresInteractiveProtocol(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		&fakeJobRunner{},
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) {
		return "", errors.New("interactive protocol missing")
	}

	item, err := service.Find(context.Background(), "builtin-7")
	if err != nil {
		t.Fatal(err)
	}
	if item.Installer != "guided" || item.Capabilities["install"].Enabled {
		t.Fatalf("non-interactive script enabled a terminal install: %#v", item)
	}
}

func TestAllAuditedBuiltinAppsOfferSafeInstallPath(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		&fakeJobRunner{},
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) {
		return "/usr/local/bin/k", nil
	}

	inventory, err := service.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	builtinCount := 0
	for _, item := range inventory.Items {
		if item.Source != "builtin" {
			continue
		}
		builtinCount++
		if !item.Capabilities["install"].Enabled {
			t.Fatalf("builtin application %s has no install path: %#v", item.ID, item)
		}
		if item.Installer != "kejilion" && item.Installer != "declarative" {
			t.Fatalf("builtin application %s escaped trusted installers: %#v", item.ID, item)
		}
	}
	if builtinCount != 116 {
		t.Fatalf("audited builtin application count = %d, want 116", builtinCount)
	}
}

func TestInactiveScriptJobAutomaticallyReleasesTaskLock(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeJobRunner{}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }

	job, err := service.StartInstall(context.Background(), "builtin-4", InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.jobs.read(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	record.CreatedAt = service.now().Add(-appJobLaunchGrace - time.Second)
	if err := service.jobs.put(record); err != nil {
		t.Fatal(err)
	}

	history := service.AppJobs()
	if len(history) != 1 || history[0].Status != "failed" ||
		history[0].Stage != "interrupted" || history[0].InputOpen {
		t.Fatalf("stale task lock was not released: %#v", history)
	}
	if service.jobs.hasActive() {
		t.Fatal("stale task still blocked a new install")
	}
}

func TestActivatingScriptJobRemainsActivePastLaunchGrace(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeJobRunner{unitState: "activating"}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }

	job, err := service.StartInstall(context.Background(), "builtin-4", InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.jobs.read(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	record.CreatedAt = service.now().Add(-appJobLaunchGrace - time.Second)
	if err := service.jobs.put(record); err != nil {
		t.Fatal(err)
	}

	history := service.AppJobs()
	if len(history) != 1 ||
		(history[0].Status != "queued" && history[0].Status != "running") {
		t.Fatalf("activating systemd job was interrupted: %#v", history)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	lastCall := runner.calls[len(runner.calls)-1]
	if strings.Join(lastCall, " ") !=
		"systemctl show --property=ActiveState --value "+appJobUnitPrefix+job.ID+".service" {
		t.Fatalf("systemd state probe = %#v", lastCall)
	}
}

func TestUnknownScriptJobUnitStateDoesNotInterruptTask(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeJobRunner{}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }

	job, err := service.StartInstall(context.Background(), "builtin-4", InstallInput{})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.jobs.read(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	record.CreatedAt = service.now().Add(-appJobLaunchGrace - time.Second)
	if err := service.jobs.put(record); err != nil {
		t.Fatal(err)
	}
	runner.unitErr = errors.New("temporary systemd D-Bus failure")

	history := service.AppJobs()
	if len(history) != 1 ||
		(history[0].Status != "queued" && history[0].Status != "running") {
		t.Fatalf("unknown systemd state interrupted the task: %#v", history)
	}
}

func TestKejilionScriptCompatibilityRequiresExplicitLicenseAcceptance(t *testing.T) {
	base := []byte("KJ_APP_NONINTERACTIVE\nkpanel_run_docker_app_install\n")
	unaccepted := append(
		append([]byte{}, base...),
		[]byte("permission_granted=\"false\"\nsed -i 's/permission_granted=\"false\"/permission_granted=\"true\"/' /usr/local/bin/k\n")...,
	)
	if isKPanelCompatibleScript(unaccepted) {
		t.Fatal("unaccepted user license enabled background script execution")
	}
	accepted := append(append([]byte{}, base...), []byte("permission_granted=\"true\"\n")...)
	if !isKPanelCompatibleScript(accepted) {
		t.Fatal("accepted compatible script was rejected")
	}
}

func TestKejilionInteractiveCompatibilityIsExplicit(t *testing.T) {
	base := []byte(
		"KJ_APP_NONINTERACTIVE\nkpanel_run_docker_app_install\n" +
			"permission_granted=\"true\"\n",
	)
	if isKPanelInteractiveCompatibleScript(base) {
		t.Fatal("legacy script unexpectedly enabled interactive terminal jobs")
	}
	compatible := append(
		append([]byte{}, base...),
		[]byte("KJ_APP_INTERACTIVE\nkpanel_app_interactive_choice\n")...,
	)
	if !isKPanelInteractiveCompatibleScript(compatible) {
		t.Fatal("interactive script protocol was rejected")
	}
	if isKPanelInteractiveManageCompatibleScript(compatible) {
		t.Fatal("legacy interactive script unexpectedly enabled application management")
	}
	managed := append(
		append([]byte{}, compatible...),
		[]byte("kpanel_app_interactive_manage_choice\nKJ_APP_MARKER_RECOVERY\n")...,
	)
	if !isKPanelInteractiveManageCompatibleScript(managed) {
		t.Fatal("interactive application management protocol was rejected")
	}
}
