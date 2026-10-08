package notification

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBackupNotificationsDedupePersistAndRecover(t *testing.T) {
	service, _, dir := newPanelLoginTestService(t, &notificationTestRobot{}, panelLoginTestHost())
	rules := DefaultRules()
	rules.CPUEnabled, rules.MemoryEnabled, rules.DiskEnabled, rules.SSHLoginEnabled, rules.HostOfflineEnabled = false, false, false, false, false
	rules.BackupEnabled = true
	if _, err := service.Configure(context.Background(), UpdateInput{Rules: rules, ExpectedResourceVersion: service.Snapshot().ResourceVersion}); err != nil {
		t.Fatal(err)
	}
	status := BackupStatus{ID: "slot-1", ErrorCode: "schedule_missed"}
	service.SetBackupSource(func() BackupStatus { return status })
	for range 2 {
		if err := service.evaluate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	history, err := service.history.snapshot()
	if err != nil || len(history.Events) != 1 || history.Events[0].Delivery != "local_only" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	// Reload the same persisted alert cursor, without sending through a real
	// channel or relying on an in-memory dedupe set.
	reopened, err := NewService(Config{DataDir: dir, Hosts: service.hosts, Telegram: &notificationTestTelegram{}, Robots: &notificationTestRobot{}})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	reopened.SetBackupSource(func() BackupStatus { return status })
	if err := reopened.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	history, _ = reopened.history.snapshot()
	if len(history.Events) != 1 {
		t.Fatal("restart repeated the same missed slot")
	}
	status = BackupStatus{ID: "job-2", ErrorCode: "retention_failed"}
	if err := reopened.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	status = BackupStatus{ID: "job-3", Completed: true}
	for range 2 {
		if err := reopened.evaluate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	history, _ = reopened.history.snapshot()
	if len(history.Events) != 3 || history.Events[2].Kind != "recovery" || history.Events[2].RelatedEventID != history.Events[1].ID {
		t.Fatalf("recovery=%+v", history.Events)
	}
}

func TestBackupRuleOffIsCompatibleAndDoesNotRecord(t *testing.T) {
	rules := DefaultRules()
	encoded, _ := json.Marshal(rules)
	if strings.Contains(string(encoded), "backupEnabled") {
		t.Fatal("off rule changed legacy state")
	}
	service, _, _ := newPanelLoginTestService(t, &notificationTestRobot{}, panelLoginTestHost())
	service.SetBackupSource(func() BackupStatus { return BackupStatus{ID: "failed", ErrorCode: "failed"} })
	if err := service.evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	history, _ := service.history.snapshot()
	if len(history.Events) != 0 {
		t.Fatalf("off rule emitted events: %+v", history.Events)
	}
}
