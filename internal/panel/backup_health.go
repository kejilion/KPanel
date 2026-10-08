package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/backupremote"
	"github.com/kejilion/kejilion-panel/internal/notification"
)

type backupScheduleHealth struct {
	State         string     `json:"state"`
	LastSuccessAt *time.Time `json:"lastSuccessAt,omitempty"`
	LastRecordID  string     `json:"lastRecordId,omitempty"`
	ErrorCode     string     `json:"errorCode,omitempty"`
	eventID       string
}

func sameBackupModules(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// Health is derived from final task receipts, never from a claimed slot or a
// worker's return value. Keep the encrypted v1 schedule schema unchanged.
func scheduleHealth(settings backupremote.Settings, records []backup.Record) backupScheduleHealth {
	plan := settings.Schedule
	health := backupScheduleHealth{State: "idle"}
	if !plan.Enabled {
		health.State = "disabled"
	}
	modules := slices.Clone(plan.Modules)
	slices.Sort(modules)
	destination := ""
	for _, storage := range settings.Storages {
		if storage.ID == plan.StorageID {
			destination = storage.Fingerprint()
		}
	}
	var latest *backup.Record
	for _, record := range records {
		if !record.Automatic || record.Action != "export" {
			continue
		}
		selected := slices.Clone(record.Modules)
		slices.Sort(selected)
		if !slices.Equal(modules, selected) {
			continue
		}
		if plan.StorageID == "" {
			if record.Remote != nil {
				continue
			}
		} else if destination == "" || record.Remote == nil || record.Remote.StorageID != plan.StorageID || record.Remote.Destination != destination {
			continue
		}
		if latest == nil || record.CreatedAt.After(latest.CreatedAt) {
			copy := record
			latest = &copy
		}
		if record.Status == "completed" && record.Size > 0 && (record.Remote == nil || record.Remote.Status == "completed") &&
			(health.LastSuccessAt == nil || record.UpdatedAt.After(*health.LastSuccessAt)) {
			at := record.UpdatedAt
			health.LastSuccessAt = &at
		}
	}
	if !plan.Enabled {
		return health
	}
	if latest != nil {
		health.LastRecordID = latest.ID
		body, _ := json.Marshal([]any{latest.ID, latest.Status, latest.ErrorCode, latest.UpdatedAt})
		sum := sha256.Sum256(body)
		health.eventID = hex.EncodeToString(sum[:])
		switch latest.Status {
		case "queued", "running", "restarting":
			health.State = "running"
		case "failed":
			health.State, health.ErrorCode = "failed", latest.ErrorCode
			if health.ErrorCode == "" {
				health.ErrorCode = "failed"
			}
		case "completed":
			if latest.Size > 0 && (latest.Remote == nil || latest.Remote.Status == "completed") {
				health.State = "healthy"
				if latest.ErrorCode != "" {
					health.State, health.ErrorCode = "warning", latest.ErrorCode
				}
			}
		}
	}
	// Start failures and missed slots can have no task record. NextRun was
	// durably advanced by Claim and gives the occurrence a stable identity.
	if plan.LastError != "" && (plan.LastError == "schedule_missed" || latest == nil || latest.CreatedAt.Before(plan.LastRun)) {
		health.State, health.ErrorCode = "failed", plan.LastError
		if plan.LastError == "schedule_missed" {
			health.State = "missed"
		}
		body, _ := json.Marshal([]any{modules, destination, plan.NextRun, plan.LastError})
		sum := sha256.Sum256(body)
		health.eventID = hex.EncodeToString(sum[:])
	}
	return health
}

func (s *Server) backupHealth() backupScheduleHealth {
	if s.backupRemote == nil || s.backups == nil {
		return backupScheduleHealth{State: "unavailable"}
	}
	return scheduleHealth(s.backupRemote.Snapshot(), s.backups.List())
}

func (s *Server) backupSettingsSnapshot() any {
	settings := s.backupRemote.Snapshot()
	health := backupScheduleHealth{State: "unavailable"}
	if s.backups != nil {
		health = scheduleHealth(settings, s.backups.List())
	}
	return struct {
		backupremote.Settings
		Health backupScheduleHealth `json:"health"`
	}{settings, health}
}

func (s *Server) backupNotificationStatus() notification.BackupStatus {
	health := s.backupHealth()
	if health.State == "disabled" || health.State == "idle" || health.State == "running" || health.State == "unavailable" {
		return notification.BackupStatus{}
	}
	return notification.BackupStatus{ID: health.eventID, ErrorCode: health.ErrorCode, Completed: health.State == "healthy"}
}
