package webenv

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateActionUsesFixedEnumsAndResourceVersion(t *testing.T) {
	summary := Summary{ResourceVersion: "resource-a"}
	tests := []struct {
		name  string
		input ActionRequest
		want  []string
	}{
		{"install", ActionRequest{Action: "install", Profile: "full", ExpectedResourceVersion: "resource-a"}, []string{"install", "full"}},
		{"optimization", ActionRequest{Action: "optimization.apply", Operation: "high", ExpectedResourceVersion: "resource-a"}, []string{"optimize", "high"}},
		{"update", ActionRequest{Action: "update.component", Component: "mysql", Version: "8.4", BackupBeforeChange: true, ExpectedResourceVersion: "resource-a"}, []string{"update", "mysql", "8.4", "true"}},
		{"backup delete", ActionRequest{Action: "backup.delete", BackupID: "web_20260728112233.tar.gz", ExpectedResourceVersion: "resource-a"}, []string{"backup", "delete", "web_20260728112233.tar.gz"}},
		{"restore", ActionRequest{Action: "restore", BackupID: "web_20260728112233.tar.gz", ExpectedResourceVersion: "resource-a"}, []string{"restore", "web_20260728112233.tar.gz"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, err := validateAction(test.input, summary)
			if err != nil {
				t.Fatalf("validateAction() error = %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("validateAction() = %#v, want %#v", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("validateAction() = %#v, want %#v", got, test.want)
				}
			}
		})
	}
}

func TestValidateActionRejectsStaleUnsafeOrArbitraryInput(t *testing.T) {
	summary := Summary{ResourceVersion: "current"}
	for _, input := range []ActionRequest{
		{Action: "install", Profile: "full"},
		{Action: "install", Profile: "full", ExpectedResourceVersion: "stale"},
		{Action: "shell", Operation: "rm -rf /", ExpectedResourceVersion: "current"},
		{Action: "restore", BackupID: "../../etc/shadow", ExpectedResourceVersion: "current"},
		{Action: "update.component", Component: "mysql;id", Version: "latest", ExpectedResourceVersion: "current"},
	} {
		if _, _, err := validateAction(input, summary); err == nil {
			t.Fatalf("validateAction(%#v) unexpectedly succeeded", input)
		}
	}
}

func TestCloudflareSecretsUseBoundedDedicatedPayload(t *testing.T) {
	input := ActionRequest{
		Action: "protection.configure", Operation: "cloudflare-shield",
		CloudflareAccount: "admin@example.com", CloudflareToken: "secret-token",
		CloudflareZoneID: "zone-id",
	}
	secret, err := actionSecret(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(secret) != "admin@example.com\nsecret-token\nzone-id\n" {
		t.Fatalf("secret payload = %q", secret)
	}
	input.CloudflareToken = "bad\nvalue"
	if _, err := actionSecret(input); err == nil {
		t.Fatal("newline-bearing secret unexpectedly accepted")
	}
	input = ActionRequest{Action: "backup.create", CloudflareToken: "must-not-be-accepted"}
	if _, err := actionSecret(input); err == nil {
		t.Fatal("secret on unrelated action unexpectedly accepted")
	}
}

func TestStoredArgumentsRemainFixedEnums(t *testing.T) {
	valid := []struct {
		action string
		args   []string
	}{
		{"install", []string{"install", "full"}},
		{"protection.configure", []string{"protect", "waf-on"}},
		{"optimization.apply", []string{"optimize", "brotli-off"}},
		{"update.component", []string{"update", "mysql", "8.4", "true"}},
		{"backup.create", []string{"backup"}},
		{"backup.delete", []string{"backup", "delete", "web_20260728112233.tar.gz"}},
		{"restore", []string{"restore", "web_20260728112233.tar.gz"}},
		{"uninstall", []string{"uninstall", "false"}},
	}
	for _, test := range valid {
		if !storedArgumentsAllowed(test.action, test.args) {
			t.Fatalf("storedArgumentsAllowed(%q, %#v) = false", test.action, test.args)
		}
	}
	for _, test := range []struct {
		action string
		args   []string
	}{
		{"shell", []string{"bash", "-c", "id"}},
		{"install", []string{"install", "full;id"}},
		{"update.component", []string{"update", "mysql", "latest", "true", "id"}},
		{"restore", []string{"restore", "../../etc/shadow"}},
	} {
		if storedArgumentsAllowed(test.action, test.args) {
			t.Fatalf("storedArgumentsAllowed(%q, %#v) accepted unsafe input", test.action, test.args)
		}
	}
}

func TestRefreshConsumesProgressAndAtomicReceipt(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC()
	job := Job{ID: "0123456789abcdef0123456789abcdef", Action: "backup.create", Status: "running", CreatedAt: created}
	if err := service.writeJob(job); err != nil {
		t.Fatal(err)
	}
	log := "normal output\nKPANEL_LDNMP_EVENT {\"stage\":\"backup_archive\",\"progress\":45,\"message\":\"archiving\"}\n"
	if err := os.WriteFile(service.logPath(job.ID), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := service.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stage != "backup_archive" || got.Progress != 45 || got.Message != "archiving" {
		t.Fatalf("progress = %#v", got)
	}
	if err := os.WriteFile(service.receiptPath(job.ID), []byte(`{"status":"succeeded","message":"done"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = service.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "succeeded" || got.Progress != 100 || got.FinishedAt == nil {
		t.Fatalf("completed job = %#v", got)
	}
}

func TestTerminalPreservesANSIBytesAndOffset(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "fedcba9876543210fedcba9876543210", Action: "install", Status: "succeeded", CreatedAt: time.Now().UTC()}
	if err := service.writeJob(job); err != nil {
		t.Fatal(err)
	}
	content := []byte("\x1b[32mgreen\x1b[0m\n")
	if err := os.WriteFile(service.logPath(job.ID), content, 0o600); err != nil {
		t.Fatal(err)
	}
	chunk, err := service.Terminal(job.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(chunk.DataBase64)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(content) || chunk.NextOffset != int64(len(content)) || !chunk.Finished {
		t.Fatalf("terminal chunk = %#v, decoded = %q", chunk, decoded)
	}
}

func TestTerminalInputRejectsInvalidOrClosedJobs(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	job := Job{
		ID: "1234567890abcdef1234567890abcdef", Action: "install",
		Status: "running", CreatedAt: time.Now().UTC(),
	}
	if err := service.writeJob(job); err != nil {
		t.Fatal(err)
	}
	if err := service.WriteInput(job.ID, "\x00"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("NUL input error = %v, want ErrInvalid", err)
	}
	if err := service.WriteInput(job.ID, strings.Repeat("x", maxTerminalInput+1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized input error = %v, want ErrInvalid", err)
	}
	if err := service.WriteInput(job.ID, "yes\n"); !errors.Is(err, ErrConflict) {
		t.Fatalf("closed terminal error = %v, want ErrConflict", err)
	}
}

func TestBackupAndJobIdentifiersCannotEscape(t *testing.T) {
	service, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Job(filepath.Join("..", "outside")); err == nil {
		t.Fatal("unsafe job id unexpectedly accepted")
	}
	if _, _, err := service.OpenBackup("../../etc/passwd"); err == nil {
		t.Fatal("unsafe backup id unexpectedly accepted")
	}
}

func TestBackupsInEmptyDirectoryReturnsEmptySlice(t *testing.T) {
	backups, err := backupsIn(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if backups == nil {
		t.Fatal("empty backup directory returned nil; JSON response would become null")
	}
	if len(backups) != 0 {
		t.Fatalf("empty backup directory returned %d items", len(backups))
	}
}

func TestCappedOutputBoundsStatusCommandOutput(t *testing.T) {
	captured := &cappedOutput{limit: 8}
	for _, chunk := range []string{"abc", "defgh", "ijk"} {
		if n, err := captured.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if string(captured.data) != "abcdefgh" || !captured.overflow {
		t.Fatalf("captured %q overflow=%v", captured.data, captured.overflow)
	}
}
