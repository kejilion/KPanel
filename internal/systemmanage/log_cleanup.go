package systemmanage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	maxRotatedSystemLogFiles = 10_000
	maxRotatedSystemLogBytes = int64(500 << 20)
)

var rotatedSystemLogPattern = regexp.MustCompile(
	`(?i)(?:\.(?:[0-9]+|old)(?:\.(?:gz|bz2|xz|zst))?|-[0-9]{8}(?:\.(?:gz|bz2|xz|zst))?|\.(?:gz|bz2|xz|zst))$`,
)

type rotatedSystemLog struct {
	path    string
	info    os.FileInfo
	modTime time.Time
	size    int64
}

// pruneRotatedSystemLogs is the non-journald cleanup backend used by OpenRC
// hosts. It only removes top-level, already-rotated regular files under the
// configured log root. Current logs, directories, sockets, and symlinks are
// never candidates.
func (m *Manager) pruneRotatedSystemLogs(ctx context.Context, policy string) error {
	root := filepath.Clean(m.logRoot)
	if !filepath.IsAbs(root) || root == string(filepath.Separator) {
		return errors.New("system log root is not a dedicated absolute directory")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("system log root is unavailable or unsafe")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read system log root: %w", err)
	}
	if len(entries) > maxRotatedSystemLogFiles {
		return errors.New("system log root exceeds the safe entry limit")
	}
	allowedBases := make(map[string]struct{}, 2)
	if _, path, pathErr := m.fixedSystemLog(); pathErr == nil && path != "" {
		allowedBases[filepath.Base(path)] = struct{}{}
	}
	if _, path, pathErr := m.fixedAuthLog(); pathErr == nil && path != "" {
		allowedBases[filepath.Base(path)] = struct{}{}
	}
	if len(allowedBases) == 0 {
		return errors.New("fixed system log files are unavailable")
	}
	archives := make([]rotatedSystemLog, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		match := rotatedSystemLogPattern.FindStringIndex(name)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) ||
			match == nil || match[0] == 0 {
			continue
		}
		if _, allowed := allowedBases[name[:match[0]]]; !allowed {
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("inspect rotated system log: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() < 0 {
			continue
		}
		archives = append(archives, rotatedSystemLog{
			path: path, info: info, modTime: info.ModTime(), size: info.Size(),
		})
	}

	sort.Slice(archives, func(left, right int) bool {
		if archives[left].modTime.Equal(archives[right].modTime) {
			return archives[left].path < archives[right].path
		}
		return archives[left].modTime.Before(archives[right].modTime)
	})
	switch policy {
	case "retain-7d", "retain-3d":
		days := 7
		if policy == "retain-3d" {
			days = 3
		}
		cutoff := m.now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
		for _, archive := range archives {
			if archive.modTime.After(cutoff) {
				continue
			}
			if err := removeRotatedSystemLog(archive); err != nil {
				return err
			}
		}
	case "max-500m":
		var total int64
		for _, archive := range archives {
			if archive.size > math.MaxInt64-total {
				total = math.MaxInt64
			} else {
				total += archive.size
			}
		}
		for _, archive := range archives {
			if total <= maxRotatedSystemLogBytes {
				break
			}
			if err := removeRotatedSystemLog(archive); err != nil {
				return err
			}
			total -= archive.size
		}
	default:
		return errors.New("unknown syslog cleanup policy")
	}
	return nil
}

func removeRotatedSystemLog(archive rotatedSystemLog) error {
	current, err := os.Lstat(archive.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("recheck rotated system log: %w", err)
	}
	if !current.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(archive.info, current) {
		return errors.New("rotated system log changed during cleanup")
	}
	if err := os.Remove(archive.path); err != nil {
		return fmt.Errorf("remove rotated system log: %w", err)
	}
	return nil
}
