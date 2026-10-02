//go:build linux

package sshlogin

import (
	"os"
	"syscall"
	"testing"
	"time"
)

type logExecutableInfo struct {
	mode os.FileMode
	uid  uint32
}

func (info logExecutableInfo) Name() string       { return "logread" }
func (info logExecutableInfo) Size() int64        { return 1 }
func (info logExecutableInfo) Mode() os.FileMode  { return info.mode }
func (info logExecutableInfo) ModTime() time.Time { return time.Time{} }
func (info logExecutableInfo) IsDir() bool        { return info.mode.IsDir() }
func (info logExecutableInfo) Sys() any           { return &syscall.Stat_t{Uid: info.uid} }

func TestLogExecutableRequiresRootAndProtectedMode(t *testing.T) {
	for _, sample := range []struct {
		mode os.FileMode
		uid  uint32
		want bool
	}{
		{0o755, 0, true}, {0o555, 0, true}, {0o775, 0, false}, {0o757, 0, false}, {0o755, 1000, false},
	} {
		if got := rootOwnedLogPath(logExecutableInfo{mode: sample.mode, uid: sample.uid}); got != sample.want {
			t.Fatalf("mode=%o uid=%d trust=%v", sample.mode, sample.uid, got)
		}
	}
}
