//go:build linux

package main

import (
	"encoding/json"
	"errors"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func publishNodeCheckStatus(summary contract.ServiceCheckSummary) error {
	if !contract.ValidServiceCheckSummary(&summary, time.Now()) {
		return errors.New("invalid check status")
	}
	account, err := user.LookupGroup("kejilion-node")
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid <= 0 {
		return errors.New("unsafe telemetry group")
	}
	return writeNodeCheckStatus(nodeCheckStatusPath, summary, gid)
}

// Only the root broker writes; the telemetry account has group read access.
func writeNodeCheckStatus(path string, summary contract.ServiceCheckSummary, gid int) error {
	data, err := json.Marshal(summary)
	if err != nil || len(data) > contract.MaxServiceCheckSummaryBytes {
		return errors.New("check status exceeds size limit")
	}
	return writeNodeRuntimeSnapshot(path, data, gid)
}

func publishProcdHealthSnapshot(snapshot contract.LightNodeHealth) error {
	if !contract.ValidLightNodeHealth(snapshot, time.Now()) {
		return errors.New("invalid procd health")
	}
	account, err := user.LookupGroup("kejilion-node")
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid <= 0 {
		return errors.New("unsafe telemetry group")
	}
	data, err := json.Marshal(snapshot)
	if err != nil || len(data) > 4096 {
		return errors.New("procd health exceeds size limit")
	}
	return writeNodeRuntimeSnapshot(nodeProcdHealthPath, data, gid)
}

func writeNodeRuntimeSnapshot(path string, data []byte, gid int) error {
	dir := filepath.Dir(path)
	if err := os.Mkdir(dir, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || !rootOwned(info) || info.Mode().Perm()&0o027 != 0 {
		return errors.New("unsafe check status directory")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || (int(stat.Gid) != gid && stat.Gid != 0) {
		return errors.New("unsafe check status group")
	}
	if err := os.Chown(dir, 0, gid); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".check-status-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chown(0, gid); err != nil {
		return err
	}
	if err := file.Chmod(0o640); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
