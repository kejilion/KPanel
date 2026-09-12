package hostbackup

import (
	"errors"
	"github.com/kejilion/kejilion-panel/internal/backup"
	"os"
	"path/filepath"
	"time"
)

// SweepCLI removes only bounded, private staging directories created by the CLI.
func SweepCLI(root string, now time.Time) error {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !backup.ValidID(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if now.Sub(info.ModTime()) < 24*time.Hour {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if err := removeDataTree(path); err != nil {
			return err
		}
	}
	return nil
}
