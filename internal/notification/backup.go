package notification

import (
	"fmt"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

const backupRuleKey = "backup"

// BackupStatus contains only a stable receipt/slot identity and a failure
// code. It carries no archive content, password or remote storage credential.
type BackupStatus struct {
	ID        string
	ErrorCode string
	Completed bool
}

func (s *Service) SetBackupSource(source func() BackupStatus) {
	s.mu.Lock()
	s.backupSource = source
	s.mu.Unlock()
}

func (s *Service) handleBackup(host cluster.Host, now time.Time, locale string, send func(cluster.Host, string, string, string) (bool, bool)) bool {
	s.mu.Lock()
	source := s.backupSource
	s.mu.Unlock()
	if source == nil {
		return false
	}
	status := source()
	if status.ID == "" || !validDisplayText(status.ID, 160) || (status.ErrorCode == "" && !status.Completed) {
		return false
	}
	key := host.ID + ":" + backupRuleKey
	state, tracked := s.reserveAlertState(key)
	if !tracked || state.LastEventID == status.ID {
		return false
	}
	if status.Completed && !state.Active {
		state.LastEventID = status.ID
		s.setAlertState(key, state)
		return true
	}
	kind := "alert"
	if status.Completed {
		kind = "recovery"
	}
	accepted, _ := send(host, backupRuleKey, kind, backupStatusMessage(host, status, now, locale))
	if !accepted {
		return false
	}
	state.LastEventID = status.ID
	state.Active = !status.Completed
	state.LastNotifiedAt = now
	s.setAlertState(key, state)
	return true
}

func backupStatusMessage(host cluster.Host, status BackupStatus, now time.Time, locale string) string {
	labels := [3]string{"自动备份失败，请查看备份中心。", "自動備份失敗，請查看備份中心。", "Automatic backup failed. Check the backup center."}
	if status.ErrorCode == "schedule_missed" {
		labels = [3]string{"自动备份错过执行窗口，请检查面板运行状态。", "自動備份錯過執行時段，請檢查面板執行狀態。", "Automatic backup missed its window. Check that the panel is running."}
	} else if status.ErrorCode == "retention_failed" {
		labels = [3]string{"自动备份已完成，但旧副本清理失败。", "自動備份已完成，但舊副本清理失敗。", "Automatic backup completed, but old copies could not be pruned."}
	} else if status.Completed {
		labels = [3]string{"自动备份已恢复成功。", "自動備份已恢復成功。", "Automatic backups have recovered."}
	}
	index := 0
	if locale == "zh-TW" {
		index = 1
	}
	if locale == "en-US" {
		index = 2
	}
	return fmt.Sprintf("%s\n%s\n%s", safeMessageText(host.Name), labels[index], formatNotificationTime(now))
}
