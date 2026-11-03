package cluster

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const fileRelayV1SpoolDirectory = "cluster-file-relay-v1-tmp"

func prepareFileRelayV1SpoolDirectory(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" || !filepath.IsAbs(dataDir) {
		return "", errors.New("file relay spool parent must be absolute")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("create file relay spool parent: %w", err)
	}
	directory := filepath.Join(dataDir, fileRelayV1SpoolDirectory)
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("create file relay spool directory: %w", err)
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("file relay spool path is not a private directory")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return "", fmt.Errorf("protect file relay spool directory: %w", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("inspect file relay spool directory: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "kpanel-file-relay-v1-") || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("inspect stale file relay spool: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("stale file relay spool is not a regular file")
		}
		if err := os.Remove(path); err != nil {
			return "", fmt.Errorf("remove stale file relay spool: %w", err)
		}
	}
	return directory, nil
}
