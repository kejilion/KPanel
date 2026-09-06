package dockerx

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRestoreDockerBackupReplacesExistingArtifactsFromBackup(t *testing.T) {
	client := &Client{
		stateRoot: t.TempDir(),
		appRoot:   t.TempDir(),
		now:       time.Now,
	}
	if err := os.WriteFile(filepath.Join(client.appRoot, "appno.txt"), []byte("kpanel\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	id := "docker-20260727T120000Z-deadbeef.tar.gz"
	backupRoot := client.dockerBackupRoot()
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDockerBackupFixture(t, filepath.Join(backupRoot, id), map[string]string{
		"docker/appno.txt":                  "101\n102\n",
		"docker/example/docker-compose.yml": "services:\n  app:\n    image: nginx:alpine\n",
		"docker/kpanel/data/state":          "restored",
	})
	if err := client.restoreDockerBackup(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	markers, err := os.ReadFile(filepath.Join(client.appRoot, "appno.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(markers) != "101\n102\n" {
		t.Fatalf("restored app markers = %q", markers)
	}
	compose, err := os.ReadFile(filepath.Join(client.appRoot, "example", "docker-compose.yml"))
	if err != nil || !strings.Contains(string(compose), "nginx:alpine") {
		t.Fatalf("restored Compose project = %q, %v", compose, err)
	}
	info, err := os.Stat(filepath.Join(client.appRoot, "example", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	uid, gid, err := fileNumericOwnership(info)
	if err != nil {
		t.Fatal(err)
	}
	expectedUID, expectedGID := currentNumericOwnership()
	if uid != expectedUID || gid != expectedGID {
		t.Fatalf("restored numeric ownership = %d:%d, want %d:%d", uid, gid, expectedUID, expectedGID)
	}
	if data, err := os.ReadFile(filepath.Join(client.appRoot, "kpanel", "data", "state")); err != nil ||
		string(data) != "restored" {
		t.Fatalf("KPanel ecosystem artifact was not restored: %q, %v", data, err)
	}
}

func TestRestoreDockerBackupOverwritesExistingProjectAndContinues(t *testing.T) {
	client := &Client{stateRoot: t.TempDir(), appRoot: t.TempDir(), now: time.Now}
	if err := os.Mkdir(filepath.Join(client.appRoot, "example"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(client.appRoot, "example", "existing"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	id := "docker-20260727T120001Z-cafebabe.tar.gz"
	backupRoot := client.dockerBackupRoot()
	if err := os.MkdirAll(backupRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writeDockerBackupFixture(t, filepath.Join(backupRoot, id), map[string]string{
		"docker/example/new": "restored",
		"docker/other/file":  "restored too",
	})
	if err := client.restoreDockerBackup(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(client.appRoot, "other", "file")); statErr != nil {
		t.Fatalf("restore did not continue after replacing a project: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(client.appRoot, "example", "existing")); !os.IsNotExist(statErr) {
		t.Fatalf("stale project artifact remained after restore: %v", statErr)
	}
	data, readErr := os.ReadFile(filepath.Join(client.appRoot, "example", "new"))
	if readErr != nil || string(data) != "restored" {
		t.Fatalf("project was not restored: %q, %v", data, readErr)
	}
}

func TestExtractDockerBackupRejectsLinks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "unsafe.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "docker/example/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd", Mode: 0o777,
	}); err != nil {
		t.Fatal(err)
	}
	_ = tarWriter.Close()
	_ = gzipWriter.Close()
	_ = file.Close()
	if _, err := extractDockerBackup(context.Background(), path, t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "links") {
		t.Fatalf("unsafe link error = %v", err)
	}
}

func TestDockerRollbackRetainsOldBytesWhenRenameFails(t *testing.T) {
	appRoot := t.TempDir()
	rollbackRoot, err := os.MkdirTemp(appRoot, ".kpanel-restore-rollback-*")
	if err != nil {
		t.Fatal(err)
	}
	previous := filepath.Join(rollbackRoot, "example")
	if err := os.WriteFile(previous, []byte("old bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The target's parent disappeared: renaming the old copy back must fail.
	rollbackDockerRestore([]dockerRestoreReplacement{{target: filepath.Join(appRoot, "missing", "example"), previous: previous}}, rollbackRoot, appRoot)
	data, err := os.ReadFile(previous)
	if err != nil || string(data) != "old bytes" {
		t.Fatalf("old bytes lost after failed rollback: %q, %v", data, err)
	}
}

func writeDockerBackupFixture(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	uid, gid := currentNumericOwnership()
	for name, content := range entries {
		data := []byte(content)
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: name, Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(data)),
			Uid: uid, Gid: gid,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
