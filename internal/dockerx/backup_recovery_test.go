package dockerx

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDockerBackupBudgetBoundaries(t *testing.T) {
	var budget dockerBackupBudget
	for i := 0; i < maxDockerBackupEntries; i++ {
		if err := budget.add(0); err != nil {
			t.Fatal(err)
		}
	}
	if err := budget.add(0); err == nil {
		t.Fatal("accepted one entry over limit")
	}
	budget = dockerBackupBudget{}
	for i := 0; i < 5; i++ {
		if err := budget.add(maxDockerBackupFileBytes); err != nil {
			t.Fatal(err)
		}
	}
	if err := budget.add(1); err == nil {
		t.Fatal("accepted one byte over total limit")
	}
	budget = dockerBackupBudget{}
	if err := budget.add(maxDockerBackupFileBytes + 1); err == nil {
		t.Fatal("accepted one byte over file limit")
	}
}

func TestDockerBackupActualTarEntryLimit(t *testing.T) {
	for _, count := range []int{maxDockerBackupEntries, maxDockerBackupEntries + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			client := &Client{appRoot: t.TempDir(), stateRoot: t.TempDir(), now: time.Now}
			dir := filepath.Join(client.appRoot, "example")
			if err := os.Mkdir(dir, 0o750); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(dir)
			if err != nil {
				t.Fatal(err)
			}
			// Emit real tar directory headers without creating 100000 host files.
			path, err := client.createDockerBackupWithWalk(context.Background(), func(_ string, visit filepath.WalkFunc) error {
				for i := 0; i < count; i++ {
					if err := visit(dir, info, nil); err != nil {
						return err
					}
				}
				return nil
			})
			if count > maxDockerBackupEntries {
				if err == nil || path != "" {
					t.Fatalf("over-limit creation = %q, %v", path, err)
				}
				assertDockerBackupDirectoryEmpty(t, client)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := extractDockerBackup(context.Background(), path, t.TempDir()); err != nil {
				t.Fatal(err)
			}
			// Independently feed the restorer the extra entry rejected by creation.
			file, err := os.Create(filepath.Join(t.TempDir(), "over.tar.gz"))
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(file)
			tw := tar.NewWriter(gz)
			for i := 0; i <= maxDockerBackupEntries; i++ {
				if err := tw.WriteHeader(&tar.Header{Name: "docker/example", Typeflag: tar.TypeDir, Mode: 0o750}); err != nil {
					t.Fatal(err)
				}
			}
			if err := errors.Join(tw.Close(), gz.Close(), file.Close()); err != nil {
				t.Fatal(err)
			}
			if _, err := extractDockerBackup(context.Background(), file.Name(), t.TempDir()); err == nil {
				t.Fatal("restore accepted extra tar entry")
			}
		})
	}
}

func assertDockerBackupDirectoryEmpty(t *testing.T, client *Client) {
	t.Helper()
	entries, err := os.ReadDir(client.dockerBackupRoot())
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed creation left archive/temp: %v, %v", entries, err)
	}
}

func TestDockerBackupCreationFailureDoesNotPublish(t *testing.T) {
	for _, scenario := range []string{"cancel", "grow", "shrink", "replace", "unsafe-name", "empty"} {
		t.Run(scenario, func(t *testing.T) {
			client := &Client{appRoot: t.TempDir(), now: time.Now}
			path := filepath.Join(client.appRoot, "data")
			if scenario == "unsafe-name" {
				path = filepath.Join(client.appRoot, "invalid name")
			}
			if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			archive, err := client.createDockerBackupWithWalk(ctx, func(_ string, visit filepath.WalkFunc) error {
				switch scenario {
				case "cancel":
					cancel()
				case "grow":
					if err := os.WriteFile(path, []byte("longer than before"), 0o600); err != nil {
						return err
					}
				case "shrink":
					if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
						return err
					}
				case "replace":
					if err := os.Rename(path, path+".old"); err != nil {
						return err
					}
					if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
						return err
					}
				case "empty":
					return nil
				}
				return visit(path, info, nil)
			})
			if err == nil || archive != "" {
				t.Fatalf("failed source published archive: %q, %v", archive, err)
			}
			assertDockerBackupDirectoryEmpty(t, client)
		})
	}
}

func TestDockerBackupRestoreFailurePreservesEveryOldProject(t *testing.T) {
	for _, scenario := range []string{"copy", "remove", "rename-back", "rename-old", "cancel", "cleanup"} {
		t.Run(scenario, func(t *testing.T) {
			client := &Client{appRoot: t.TempDir(), stateRoot: t.TempDir(), now: time.Now}
			for _, name := range []string{"a", "b", "c"} {
				if err := os.Mkdir(filepath.Join(client.appRoot, name), 0o750); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(client.appRoot, name, "data"), []byte("old-"+name), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(client.dockerBackupRoot(), 0o700); err != nil {
				t.Fatal(err)
			}
			id := "docker-20260906T000000Z-deadbeef.tar.gz"
			writeDockerBackupFixture(t, filepath.Join(client.dockerBackupRoot(), id), map[string]string{"docker/a/data": "new-a", "docker/b/data": "new-b", "docker/c/data": "new-c"})
			ops := defaultDockerRestoreOps()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ops.copyTree = func(ctx context.Context, source, target string) error {
				if err := copyRestoredDockerTreeContext(ctx, source, target); err != nil {
					return err
				}
				if filepath.Base(target) == "b" {
					if scenario == "cancel" {
						cancel()
						return nil
					}
					if scenario != "rename-old" && scenario != "cleanup" {
						return errors.New("injected copy failure")
					}
				}
				return nil
			}
			ops.removeAll = func(path string) error {
				if scenario == "remove" && path == filepath.Join(client.appRoot, "b") {
					return errors.New("injected remove failure")
				}
				if scenario == "cleanup" && strings.HasPrefix(filepath.Base(path), ".kpanel-restore-rollback-") {
					return errors.New("injected cleanup failure")
				}
				return os.RemoveAll(path)
			}
			ops.rename = func(from, to string) error {
				if scenario == "rename-back" && strings.Contains(from, ".kpanel-restore-rollback-") && filepath.Base(from) == "b" {
					return errors.New("injected rename-back failure")
				}
				if scenario == "rename-old" && from == filepath.Join(client.appRoot, "b") {
					return errors.New("injected rename-old failure")
				}
				return os.Rename(from, to)
			}
			err := client.restoreDockerBackupWithOps(ctx, id, ops)
			if err == nil {
				t.Fatal("injected failure reported success")
			}
			var recovery *dockerRestoreRecoveryError
			retained := errors.As(err, &recovery)
			wantRetained := scenario == "remove" || scenario == "rename-back" || scenario == "cleanup"
			if retained != wantRetained {
				t.Fatalf("recovery status = %v, error %v", retained, err)
			}
			for _, name := range []string{"a", "b", "c"} {
				path := filepath.Join(client.appRoot, name, "data")
				if retained && (name == "b" || scenario == "cleanup") {
					path = filepath.Join(recovery.root, name, "data")
				}
				data, readErr := os.ReadFile(path)
				if readErr != nil || string(data) != "old-"+name {
					t.Fatalf("old %s unavailable at %s: %q, %v", name, path, data, readErr)
				}
			}
			if retained {
				archive, err := client.createDockerBackup(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Open(archive)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				gz, err := gzip.NewReader(file)
				if err != nil {
					t.Fatal(err)
				}
				defer gz.Close()
				tr := tar.NewReader(gz)
				for {
					header, err := tr.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(header.Name, ".kpanel-restore-rollback-") {
						t.Fatal("backup collected retained recovery data")
					}
				}
			}
		})
	}
}

func TestDockerBackupRealFileRoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux ownership and modes")
	}
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "docker")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	client := &Client{appRoot: alias, stateRoot: t.TempDir(), now: time.Now}
	project := filepath.Join(root, "example")
	if err := os.Mkdir(project, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(project, 0o751); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(project, "data")
	if err := os.WriteFile(path, []byte("original bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o604); err != nil {
		t.Fatal(err)
	}
	uid, gid := currentNumericOwnership()
	if os.Geteuid() == 0 {
		uid, gid = 1234, 2345
		if err := os.Chown(path, uid, gid); err != nil {
			t.Fatal(err)
		}
		if err := os.Chown(project, uid, gid); err != nil {
			t.Fatal(err)
		}
	}
	archive, err := client.createDockerBackup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := client.restoreDockerBackup(context.Background(), filepath.Base(archive)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "original bytes" {
		t.Fatalf("round trip data %q, %v", data, err)
	}
	for path, mode := range map[string]os.FileMode{path: 0o604, project: 0o751} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		actualUID, actualGID, err := fileNumericOwnership(info)
		if err != nil || actualUID != uid || actualGID != gid || info.Mode().Perm() != mode {
			t.Fatalf("metadata at %s: %d:%d %o, err %v", path, actualUID, actualGID, info.Mode().Perm(), err)
		}
	}
}

func TestDockerBackupRejectsCorruptAndReservedArchives(t *testing.T) {
	for _, scenario := range []string{"checksum", "reserved", "traversal", "hardlink", "device", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.tar.gz")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(file)
			tw := tar.NewWriter(gz)
			header := &tar.Header{Name: "docker/example/data", Typeflag: tar.TypeReg, Mode: 0o600}
			switch scenario {
			case "reserved":
				header.Name = "docker/.kpanel-restore-rollback-1234/old"
			case "traversal":
				header.Name = "../../outside"
			case "hardlink":
				header.Typeflag = tar.TypeLink
				header.Linkname = "docker/example/other"
			case "device":
				header.Typeflag = tar.TypeChar
			}
			if err := tw.WriteHeader(header); err != nil {
				t.Fatal(err)
			}
			if err := errors.Join(tw.Close(), gz.Close(), file.Close()); err != nil {
				t.Fatal(err)
			}
			if scenario == "checksum" {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data[len(data)-8] ^= 0xff
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "cancel" {
				cancel()
			}
			if _, err := extractDockerBackup(ctx, path, t.TempDir()); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}

func TestDockerRestoreRecoveryLocationSurvivesJobReload(t *testing.T) {
	client := &Client{now: time.Now}
	stateDir := t.TempDir()
	if err := client.ConfigureJobs(stateDir); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), strings.Repeat("a", 180), strings.Repeat("b", 180), ".kpanel-restore-rollback-12345")
	failure := &dockerRestoreRecoveryError{root: root, cause: errors.New(strings.Repeat("secret-token ", 1000))}
	message := safeDockerJobMessage(errors.Join(errors.New(strings.Repeat("noise", 200)), failure))
	if !strings.Contains(message, root) || strings.Contains(message, "secret-token") || len(message) > len(root)+150 {
		t.Fatalf("unsafe/truncated recovery message %q", message)
	}
	now := time.Now().UTC()
	record := dockerJobRecord{MaintenanceJob: MaintenanceJob{ID: strings.Repeat("a", 32), Action: "backup_restore", Status: "failed", Stage: "failed", Message: message, Progress: 100, CreatedAt: now, FinishedAt: &now}}
	client.jobs.finish(record)
	restarted := &Client{now: time.Now}
	if err := restarted.ConfigureJobs(stateDir); err != nil {
		t.Fatal(err)
	}
	job, err := restarted.MaintenanceJob(record.ID)
	if err != nil || job.Status != "failed" || job.Stage != "failed" || job.Message != message || job.FinishedAt == nil {
		t.Fatalf("reloaded recovery job: %+v, %v", job, err)
	}
	if restarted.jobs.hasActive() {
		t.Fatal("terminal restore was replayed")
	}
}

type dockerBackupReadHookContext struct {
	context.Context
	checks int
	hook   func() error
}

func (ctx *dockerBackupReadHookContext) Err() error {
	ctx.checks++
	// For this small fixture, the second read checks EOF before the final stat.
	if ctx.checks == 2 {
		return ctx.hook()
	}
	return nil
}

func TestDockerBackupMutationAndCancellationDuringRead(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprint(cancel), func(t *testing.T) {
			client := &Client{appRoot: t.TempDir(), now: time.Now}
			path := filepath.Join(client.appRoot, "data")
			if err := os.WriteFile(path, []byte("before"), 0o600); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &dockerBackupReadHookContext{Context: context.Background(), hook: func() error {
				if cancel {
					return context.Canceled
				}
				if err := os.WriteFile(path, []byte("after!"), 0o600); err != nil {
					return err
				}
				return os.Chtimes(path, time.Now(), info.ModTime().Add(time.Second))
			}}
			archive, err := client.createDockerBackupWithWalk(ctx, func(_ string, visit filepath.WalkFunc) error { return visit(path, info, nil) })
			if err == nil || archive != "" {
				t.Fatalf("mid-read source failure published: %q, %v", archive, err)
			}
			assertDockerBackupDirectoryEmpty(t, client)
		})
	}
}

func TestDockerBackupFailedJobDoesNotReplayAfterRestart(t *testing.T) {
	client := &Client{appRoot: t.TempDir(), stateRoot: t.TempDir(), now: time.Now}
	jobs := t.TempDir()
	if err := client.ConfigureJobs(jobs); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(client.dockerBackupRoot(), 0o700); err != nil {
		t.Fatal(err)
	}
	id := "docker-20260906T000000Z-deadbeef.tar.gz"
	archive := filepath.Join(client.dockerBackupRoot(), id)
	if err := os.WriteFile(archive, []byte("invalid archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	job, err := client.StartMaintenance(context.Background(), MaintenanceInput{Action: "backup_restore", BackupID: id})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForDockerJob(t, client, job.ID)
	if job.Status != "failed" {
		t.Fatalf("bad archive job: %+v", job)
	}
	// Make replay observable: the same ID now contains an applicable archive.
	if err := os.Remove(archive); err != nil {
		t.Fatal(err)
	}
	writeDockerBackupFixture(t, archive, map[string]string{"docker/data": "must not replay"})
	restarted := &Client{appRoot: client.appRoot, stateRoot: client.stateRoot, now: time.Now}
	if err := restarted.ConfigureJobs(jobs); err != nil {
		t.Fatal(err)
	}
	reloaded, err := restarted.MaintenanceJob(job.ID)
	if err != nil || reloaded.Status != "failed" || reloaded.Message != job.Message {
		t.Fatalf("lost failure: %+v, %v", reloaded, err)
	}
	if _, err := os.Stat(filepath.Join(client.appRoot, "data")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore replayed: %v", err)
	}
	if entries, err := os.ReadDir(client.stateRoot); err != nil || len(entries) != 0 {
		t.Fatalf("extraction stage leaked: %v, %v", entries, err)
	}
}

func TestDockerRestoreCompletedMessageDescribesReplacement(t *testing.T) {
	message := dockerActionCompleted("backup_restore")
	if !strings.Contains(message, "已替换") || strings.Contains(message, "未被覆盖") {
		t.Fatalf("restore completion misrepresents replacement: %q", message)
	}
}
