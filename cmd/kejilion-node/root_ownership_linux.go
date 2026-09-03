//go:build linux

package main

import (
	"os"
	"syscall"
)

func rootOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	// The staged release and migration targets must be root-owned. The group
	// may intentionally be kejilion-node for the low-privilege relay.
	return ok && stat.Uid == 0
}
