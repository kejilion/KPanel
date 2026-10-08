package panel

import (
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/backupremote"
)

func TestBackupHealthUsesFinalMatchingReceipts(t *testing.T) {
	now := time.Date(2026, 10, 8, 3, 0, 0, 0, time.UTC)
	settings := backupremote.Settings{Schedule: backupremote.Schedule{Enabled: true, Modules: []string{"panel", "apps"}, LastRun: now.Add(-time.Hour), NextRun: now.Add(time.Hour)}}
	success := backup.Record{ID: "success", Automatic: true, Action: "export", Status: "completed", Modules: []string{"apps", "panel"}, Size: 123, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Hour)}
	latest := success
	latest.ID, latest.CreatedAt, latest.UpdatedAt = "new", now, now.Add(time.Minute)
	for _, test := range []struct{ status, errorCode, state string }{
		{"queued", "", "running"}, {"running", "", "running"}, {"failed", "persistence_pending", "failed"}, {"failed", "remote_upload_failed", "failed"}, {"completed", "retention_failed", "warning"},
	} {
		latest.Status, latest.ErrorCode = test.status, test.errorCode
		health := scheduleHealth(settings, []backup.Record{latest, success})
		if health.State != test.state || health.LastSuccessAt == nil {
			t.Fatalf("health=%+v", health)
		}
		wantSuccess := success.UpdatedAt
		if test.status == "completed" {
			wantSuccess = latest.UpdatedAt
		}
		if !health.LastSuccessAt.Equal(wantSuccess) {
			t.Fatalf("non-final result reported success: %+v", health)
		}
	}
	// A worker error before Manager.Run persists its final result must not
	// create an early success/failure receipt or a second notification.
	latest.Status, latest.ErrorCode = "running", ""
	settings.Schedule.LastError = "remote_upload_failed"
	if health := scheduleHealth(settings, []backup.Record{latest, success}); health.State != "running" {
		t.Fatalf("early worker outcome: %+v", health)
	}
	settings.Schedule.LastError = "schedule_missed"
	missed := scheduleHealth(settings, []backup.Record{success})
	if missed.State != "missed" || missed.LastSuccessAt == nil || missed.eventID == "" {
		t.Fatalf("missed=%+v", missed)
	}
	if next := scheduleHealth(settings, []backup.Record{success}); next.eventID != missed.eventID {
		t.Fatal("missed-slot identity is unstable")
	}
	settings.Schedule.LastError = ""
	latest.Status, latest.ErrorCode = "failed", "remote_upload_failed"
	failed := scheduleHealth(settings, []backup.Record{latest})
	latest.Status, latest.ErrorCode = "completed", ""
	latest.UpdatedAt = latest.UpdatedAt.Add(time.Minute)
	if completed := scheduleHealth(settings, []backup.Record{latest}); completed.eventID == failed.eventID {
		t.Fatal("same-record upload recovery lost its identity")
	}
	manual := success
	manual.Automatic = false
	otherModules := success
	otherModules.Modules = []string{"docker"}
	if health := scheduleHealth(settings, []backup.Record{manual, otherModules}); health.LastSuccessAt != nil {
		t.Fatalf("unrelated success=%+v", health)
	}
}

func TestBackupHealthBindsRemoteDestination(t *testing.T) {
	storage := backupremote.Storage{ID: "remote", Kind: "webdav", Endpoint: "https://nas.example", Prefix: "current"}
	settings := backupremote.Settings{Storages: []backupremote.Storage{storage}, Schedule: backupremote.Schedule{Enabled: true, Modules: []string{"panel"}, StorageID: storage.ID}}
	record := backup.Record{Automatic: true, Action: "export", Status: "completed", Modules: []string{"panel"}, Size: 100, UpdatedAt: time.Now(), Remote: &backup.RemoteCopy{StorageID: storage.ID, Destination: storage.Fingerprint(), Status: "uploading"}}
	if h := scheduleHealth(settings, []backup.Record{record}); h.LastSuccessAt != nil {
		t.Fatal("incomplete remote upload counted as success")
	}
	record.Remote.Status = "completed"
	if h := scheduleHealth(settings, []backup.Record{record}); h.LastSuccessAt == nil {
		t.Fatal("complete receipt missed")
	}
	settings.Storages[0].Prefix = "changed"
	if h := scheduleHealth(settings, []backup.Record{record}); h.LastSuccessAt != nil {
		t.Fatal("receipt from an old destination counted")
	}
}
