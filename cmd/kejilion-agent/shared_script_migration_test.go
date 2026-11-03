package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSharedScriptMigrationRetriesOnlyLifecycleBusy(t *testing.T) {
	attempts := 0
	m := &sharedScriptMigration{interval: time.Millisecond, execute: func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errSharedScriptMigrationBusy
		}
		return nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	m.Run(ctx)
	if !m.Ready() || attempts != 3 {
		t.Fatalf("ready=%v attempts=%d", m.Ready(), attempts)
	}
	m.Run(ctx)
	if attempts != 3 {
		t.Fatal("completed migration executed again")
	}
}

func TestSharedScriptMigrationFailureAndCancellationStayClosed(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		attempts := 0
		m := &sharedScriptMigration{interval: time.Millisecond, execute: func(context.Context) error {
			attempts++
			return errors.New("fixture failure")
		}}
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		m.Run(ctx)
		cancel()
		if m.Ready() || (!cancelled && attempts != 1) || (cancelled && attempts != 0) {
			t.Fatalf("cancelled=%v ready=%v attempts=%d", cancelled, m.Ready(), attempts)
		}
	}
	m := newSharedScriptMigration(false)
	if !m.Ready() {
		t.Fatal("read-only Agent must not start a host mutation")
	}
}

func TestSharedScriptMigrationCommandRejectsArguments(t *testing.T) {
	for _, args := range [][]string{{"host-script-migrate", "/tmp/other"}, {"host-script-migrate", "--script", "/tmp/other"}} {
		if err := run(args); err == nil {
			t.Fatal("migration accepted a caller-selected argument")
		}
	}
}

// This uses real file writes and flock only inside an explicitly disposable
// rootfs. Ordinary go test on a workstation cannot enter this branch.
func TestSharedScriptMigrationLegacyRootfs(t *testing.T) {
	if os.Getenv("KPANEL_MIGRATION_ROOTFS_TEST") != "1" {
		t.Skip("explicit disposable rootfs only")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil || os.Geteuid() != 0 {
		t.Fatal("refusing migration test outside disposable root container")
	}
	root := os.Getenv("KPANEL_TEST_SOURCE_ROOT")
	if root == "" {
		t.Fatal("source root required")
	}
	life, err := os.ReadFile(filepath.Join(root, "packaging/kejilion-app/kpanel.conf"))
	if err != nil {
		t.Fatal(err)
	}
	const app = "/home/docker/kpanel"
	for _, dir := range []string{app + "/bin", app + "/update-state", "/run/lock", "/usr/local/bin"} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, data string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	old := "#!/bin/sh\ncanshu=\"V6\"\npermission_granted=\"true\"\nENABLE_STATS=\"true\"\n# old shared bytes\n"
	// The old updater prefers its private script. Its extracted payload can
	// therefore have different inherited preferences from the current SSH k.
	next := "#!/bin/sh\ncanshu=\"CN\"\npermission_granted=\"true\"\nENABLE_STATS=\"false\"\n# new paired bytes\n"
	write(app+"/.managed-by-kejilion-app", "", 0600)
	write(app+"/bin/kpanel.conf", string(life), 0600)
	write(app+"/bin/kejilion.sh", next, 0700)
	write("/root/kejilion.sh", old, 0700)
	write("/usr/local/bin/k", old, 0755)
	if err := os.Symlink("/usr/local/bin/k", "/usr/bin/k"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	lock := exec.CommandContext(ctx, "/bin/bash", "-c", "exec 9>/run/lock/kejilion-kpanel-lifecycle.lock; flock -x 9; printf locked; read -r release")
	input, _ := lock.StdinPipe()
	output, _ := lock.StdoutPipe()
	if err := lock.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close(); _ = lock.Wait() }()
	buf := make([]byte, 6)
	if _, err := io.ReadFull(output, buf); err != nil || string(buf) != "locked" {
		t.Fatalf("lock handshake: %q %v", buf, err)
	}
	if err := migrateSharedScript(ctx); !errors.Is(err, errSharedScriptMigrationBusy) {
		t.Fatalf("old updater lock was ignored: %v", err)
	}
	got, _ := os.ReadFile("/usr/local/bin/k")
	if string(got) != old {
		t.Fatal("migration changed SSH bytes before old updater committed")
	}
	_, _ = input.Write([]byte("done\n"))
	_ = input.Close()
	if err := lock.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(app+"/update-state/transaction", 0700); err != nil {
		t.Fatal(err)
	}
	if err := migrateSharedScript(ctx); !errors.Is(err, errSharedScriptMigrationBusy) {
		t.Fatalf("unfinished transaction was ignored: %v", err)
	}
	if err := os.Remove(app + "/update-state/transaction"); err != nil {
		t.Fatal(err)
	}
	backup := app + "/bin/shared-script.rollback"
	if err := os.Mkdir(backup, 0700); err != nil {
		t.Fatal(err)
	}
	write(backup+"/1", old, 0700)
	write(backup+"/.building", "", 0600)
	if err := migrateSharedScript(ctx); !errors.Is(err, errSharedScriptMigrationBusy) {
		t.Fatalf("incomplete snapshot recovery was not scheduled for retry: %v", err)
	}
	if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("incomplete snapshot was not safely retired: %v", err)
	}
	for _, path := range []string{"/root/kejilion.sh", "/usr/local/bin/k"} {
		got, _ := os.ReadFile(path)
		if string(got) != old {
			t.Fatalf("recovering an incomplete snapshot changed %s", path)
		}
	}
	if err := os.Mkdir(backup, 0700); err != nil {
		t.Fatal(err)
	}
	write(backup+"/2.absent", "", 0600)
	write(backup+"/.building", "", 0600)
	restoreIncomplete := exec.CommandContext(ctx, "/bin/bash", "-ec", `docker_app_plus() { :; }
entry() {
. /home/docker/kpanel/bin/kpanel.conf
kpanel_restore_shared_script
}
entry`)
	restoreIncomplete.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	if output, err := restoreIncomplete.CombinedOutput(); err != nil {
		t.Fatalf("restoring an incomplete snapshot: %v %s", err, output)
	}
	if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore left an incomplete snapshot: %v", err)
	}
	if err := os.Chmod(app+"/bin/kejilion.sh", 0777); err != nil {
		t.Fatal(err)
	}
	if err := migrateSharedScript(ctx); err == nil || errors.Is(err, errSharedScriptMigrationBusy) {
		t.Fatalf("unsafe script mode admitted: %v", err)
	}
	if err := os.Chmod(app+"/bin/kejilion.sh", 0700); err != nil {
		t.Fatal(err)
	}
	// Fail only the final wrapper rename, after all shared destinations changed.
	// The same production entry must put the original SSH files back.
	write("/usr/local/bin/mv", "#!/bin/sh\nfor last; do :; done\n[ \"$last\" != /home/docker/kpanel/bin/kejilion.sh ] || exit 1\nexec /bin/mv \"$@\"\n", 0700)
	if err := migrateSharedScript(ctx); err == nil {
		t.Fatal("failed wrapper publication was reported as success")
	}
	if err := os.Remove("/usr/local/bin/mv"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/root/kejilion.sh", "/usr/local/bin/k"} {
		got, _ := os.ReadFile(path)
		if string(got) != old {
			t.Fatalf("partial migration was not rolled back: %s", path)
		}
	}
	if info, _ := os.Stat("/root/kejilion.sh"); info == nil || info.Mode().Perm() != 0700 {
		t.Fatal("root script original mode was not restored")
	}
	if target, _ := os.Readlink("/usr/bin/k"); target != "/usr/local/bin/k" {
		t.Fatal("original link was not restored")
	}
	// A committed wrapper followed by cleanup failure is also a possible
	// restart state. It must stay pending until its old snapshot is retired.
	write("/usr/local/bin/rm", "#!/bin/sh\nfor arg; do\n[ \"$arg\" != /home/docker/kpanel/bin/shared-script.rollback ] || exit 1\ndone\nexec /bin/rm \"$@\"\n", 0700)
	if err := migrateSharedScript(ctx); err == nil {
		t.Fatal("committed migration with stale rollback was reported as ready")
	}
	if _, err := os.Stat(app + "/bin/shared-script.rollback/ready"); err != nil {
		t.Fatal("cleanup failure fixture did not retain the old snapshot")
	}
	if err := os.Remove("/usr/local/bin/rm"); err != nil {
		t.Fatal(err)
	}
	if err := migrateSharedScript(ctx); err != nil {
		t.Fatalf("restart after committed wrapper: %v", err)
	}
	if _, err := os.Lstat(app + "/bin/shared-script.rollback"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("committed migration left a stale snapshot: %v", err)
	}
	got, _ = os.ReadFile("/usr/local/bin/k")
	if !strings.Contains(string(got), "# new paired bytes") || !strings.Contains(string(got), "canshu=\"V6\"") || !strings.Contains(string(got), "ENABLE_STATS=\"true\"") {
		t.Fatalf("paired bytes/preferences not synchronized: %s", got)
	}
	rootScript, _ := os.ReadFile("/root/kejilion.sh")
	if string(rootScript) != string(got) {
		t.Fatal("root and k diverged")
	}
	shim, _ := os.ReadFile(app + "/bin/kejilion.sh")
	if !strings.Contains(string(shim), "# KPanel shared script compatibility wrapper: v1") || !strings.Contains(string(shim), "exec /usr/local/bin/k") || len(shim) > 256 {
		t.Fatal("legacy update entry did not become a small forwarding file")
	}
	if err := migrateSharedScript(ctx); err != nil {
		t.Fatalf("idempotent restart: %v", err)
	}
	// A later failed update must restore the newly committed shared bytes,
	// never the pre-migration version left in the earlier snapshot.
	write(app+"/bin/future-script", strings.ReplaceAll(next, "new paired bytes", "future paired bytes"), 0700)
	future := exec.CommandContext(ctx, "/bin/bash", "-ec", `docker_app_plus() { :; }
entry() {
. /home/docker/kpanel/bin/kpanel.conf
kpanel_prepare_shared_script /home/docker/kpanel/bin/future-script
kpanel_restore_shared_script
}
entry`)
	future.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	if output, err := future.CombinedOutput(); err != nil {
		t.Fatalf("following failed-update fixture: %v %s", err, output)
	}
	for _, path := range []string{"/root/kejilion.sh", "/usr/local/bin/k"} {
		restored, _ := os.ReadFile(path)
		if string(restored) != string(got) {
			t.Fatalf("following update restored pre-migration bytes: %s", path)
		}
	}
	if err := os.Remove(app + "/bin/kejilion.sh"); err != nil {
		t.Fatal(err)
	}
	if m := newSharedScriptMigration(true); !m.Ready() {
		t.Fatal("new lifecycle with no legacy private entry must be ready")
	}
	// Complete the synthetic failed-update fixture before independent cases.
	if err := os.RemoveAll(app + "/bin/shared-script.rollback"); err != nil {
		t.Fatal(err)
	}
	write(app+"/bin/kejilion.sh", old, 0700)
	current := "#!/bin/sh\ncanshu=\"default\"\npermission_granted=\"true\"\nENABLE_STATS=\"false\"\n# current SSH defaults\n"
	write("/usr/local/bin/k", current, 0755)
	write("/root/kejilion.sh", current, 0755)
	if err := migrateSharedScript(ctx); err != nil {
		t.Fatalf("nondefault to current default preferences: %v", err)
	}
	got, _ = os.ReadFile("/usr/local/bin/k")
	if !strings.Contains(string(got), "canshu=\"default\"") || !strings.Contains(string(got), "ENABLE_STATS=\"false\"") {
		t.Fatalf("current default/opt-out preferences not applied: %s", got)
	}
	for _, path := range []string{"/root/kejilion.sh", "/usr/local/bin/k", "/usr/bin/k"} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	write(app+"/bin/kejilion.sh", next, 0700)
	if err := migrateSharedScript(ctx); err != nil {
		t.Fatalf("legacy-only trusted preferences: %v", err)
	}
	got, _ = os.ReadFile("/usr/local/bin/k")
	if !strings.Contains(string(got), "canshu=\"CN\"") || !strings.Contains(string(got), "ENABLE_STATS=\"false\"") {
		t.Fatalf("trusted private fallback lost preferences: %s", got)
	}
}
