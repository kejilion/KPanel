package systemmanage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (m *Manager) openRCRuntimeActive() bool {
	info, err := os.Stat(filepath.Join(m.runRoot, "openrc"))
	return err == nil && info.IsDir()
}

func (m *Manager) hostnameWriteAvailable() error {
	if !m.openRCRuntimeActive() {
		if _, err := m.runner.LookPath("hostnamectl"); err == nil {
			return nil
		}
	}
	if _, err := m.runner.LookPath("rc-service"); err != nil {
		return errors.New("OpenRC is unavailable")
	}
	if _, err := m.runner.LookPath("hostname"); err != nil {
		return errors.New("hostname is unavailable")
	}
	return nil
}

func (m *Manager) setRuntimeHostname(ctx context.Context, value string) error {
	if !m.openRCRuntimeActive() {
		if _, err := m.runner.LookPath("hostnamectl"); err == nil {
			_, err = m.runner.Run(ctx, "hostnamectl", "set-hostname", value)
			return err
		}
	}
	if _, err := m.runner.LookPath("rc-service"); err != nil {
		return errors.New("OpenRC is unavailable")
	}
	if _, err := m.runner.LookPath("hostname"); err != nil {
		return errors.New("hostname is unavailable")
	}
	_, err := m.runner.Run(ctx, "hostname", value)
	return err
}

func (m *Manager) timezoneWriteAvailable() error {
	if !m.openRCRuntimeActive() {
		if _, err := m.runner.LookPath("timedatectl"); err == nil {
			return nil
		}
	}
	if _, err := m.runner.LookPath("rc-service"); err != nil {
		return errors.New("OpenRC is unavailable")
	}
	zoneRoot := "/usr/share/zoneinfo"
	if m.etcRoot != "/etc" {
		zoneRoot = filepath.Join(m.etcRoot, "..", "usr", "share", "zoneinfo")
	}
	info, err := os.Stat(filepath.Clean(zoneRoot))
	if err != nil || !info.IsDir() {
		return errors.New("IANA timezone database is unavailable")
	}
	return nil
}

type timezoneFileSnapshot struct {
	existed bool
	mode    os.FileMode
	data    []byte
	link    string
}

func snapshotTimezoneFile(path string) (timezoneFileSnapshot, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return timezoneFileSnapshot{}, nil
	}
	if err != nil {
		return timezoneFileSnapshot{}, err
	}
	result := timezoneFileSnapshot{existed: true, mode: info.Mode().Perm()}
	if info.Mode()&os.ModeSymlink != 0 {
		result.link, err = os.Readlink(path)
		return result, err
	}
	if !info.Mode().IsRegular() {
		return timezoneFileSnapshot{}, errors.New("timezone path is not a regular file or symlink")
	}
	result.data, err = os.ReadFile(path)
	return result, err
}

func restoreTimezoneFile(path string, snapshot timezoneFileSnapshot) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !snapshot.existed {
		return nil
	}
	if snapshot.link != "" {
		return os.Symlink(snapshot.link, path)
	}
	mode := snapshot.mode
	if mode == 0 {
		mode = 0o644
	}
	return writeAtomic(path, snapshot.data, mode)
}

func (m *Manager) setOpenRCTimezone(
	candidate string,
	zone string,
) (bool, string, error) {
	if _, err := m.runner.LookPath("rc-service"); err != nil {
		return false, "", fmt.Errorf("%w: OpenRC is unavailable", ErrUnsupported)
	}
	localtimePath := filepath.Join(m.etcRoot, "localtime")
	timezonePath := filepath.Join(m.etcRoot, "timezone")
	if current, err := filepath.EvalSymlinks(localtimePath); err == nil &&
		filepath.Clean(current) == filepath.Clean(candidate) {
		return false, "系统时区没有变化", nil
	}
	localtimeSnapshot, err := snapshotTimezoneFile(localtimePath)
	if err != nil {
		return false, "", fmt.Errorf("%w: snapshot /etc/localtime: %v", ErrUnsupported, err)
	}
	timezoneSnapshot, err := snapshotTimezoneFile(timezonePath)
	if err != nil {
		return false, "", fmt.Errorf("%w: snapshot /etc/timezone: %v", ErrUnsupported, err)
	}
	temporary := localtimePath + ".kpanel.tmp"
	_ = os.Remove(temporary)
	relativeTarget, err := filepath.Rel(filepath.Dir(localtimePath), candidate)
	if err != nil || strings.HasPrefix(relativeTarget, ".."+string(filepath.Separator)+"..") {
		return false, "", fmt.Errorf("%w: resolve timezone link: %v", ErrUnsupported, err)
	}
	if err := os.Symlink(relativeTarget, temporary); err != nil {
		return false, "", fmt.Errorf("%w: create timezone link: %v", ErrRolledBack, err)
	}
	if err := os.Rename(temporary, localtimePath); err != nil {
		_ = os.Remove(temporary)
		return false, "", fmt.Errorf("%w: install timezone link: %v", ErrRolledBack, err)
	}
	if err := writeAtomic(timezonePath, []byte(zone+"\n"), 0o644); err != nil {
		rollbackErr := restoreTimezoneFile(localtimePath, localtimeSnapshot)
		if rollbackErr != nil {
			return false, "", fmt.Errorf("%w: timezone metadata write failed and rollback failed", ErrNeedsAttention)
		}
		return false, "", fmt.Errorf("%w: timezone metadata write failed: %v", ErrRolledBack, err)
	}
	current, verifyErr := filepath.EvalSymlinks(localtimePath)
	if verifyErr != nil || filepath.Clean(current) != filepath.Clean(candidate) ||
		strings.TrimSpace(readLimited(timezonePath)) != zone {
		localtimeRollback := restoreTimezoneFile(localtimePath, localtimeSnapshot)
		timezoneRollback := restoreTimezoneFile(timezonePath, timezoneSnapshot)
		if localtimeRollback != nil || timezoneRollback != nil {
			return false, "", fmt.Errorf("%w: timezone verification and rollback failed", ErrNeedsAttention)
		}
		return false, "", fmt.Errorf("%w: timezone verification failed", ErrRolledBack)
	}
	return true, "系统时区已更新并回读验证", nil
}
