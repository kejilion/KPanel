//go:build windows

package hostbackup

import "os"

func restoreOwnership(path string, uid, gid int) error         { return nil }
func copyOwnership(path string, info os.FileInfo) error        { return nil }
func restoreSymlinkOwnership(path string, uid, gid int) error  { return nil }
func copySymlinkOwnership(path string, info os.FileInfo) error { return nil }
