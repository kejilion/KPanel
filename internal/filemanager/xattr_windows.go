//go:build windows

package filemanager

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func preserveFileExtendedAttributes(target, source *os.File) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(source.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if dacl == nil {
		return errors.New("source file has no explicit access control")
	}
	// A replacement must never fall back to its temporary file's inherited
	// permissions. Copy the effective DACL and protect it against re-inheritance.
	return windows.SetSecurityInfo(windows.Handle(target.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
