//go:build windows

package filemanager

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func fileOwner(os.FileInfo) (string, string) {
	return "", ""
}

func preserveFileOwnership(file *os.File, _ os.FileInfo) error {
	// POSIX uid/gid have no Windows representation. An atomic replacement's
	// access rights are preserved from the open source by xattr_windows.go.
	// Never accept a newly created object with an absent/null DACL in the
	// meantime (that would grant unrestricted local access).
	sd, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if dacl == nil {
		return errors.New("new file has no access control")
	}
	return nil
}
