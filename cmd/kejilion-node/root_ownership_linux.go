//go:build linux

package main

import (
	"os"
	"syscall"
)

func rootOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	// The enrolled node configuration is intentionally root:kejilion-node so
	// the non-root telemetry process can read it. Root UID plus no write bits is
	// the trust boundary; requiring GID 0 would skip every normal installation.
	return ok && stat.Uid == 0
}
