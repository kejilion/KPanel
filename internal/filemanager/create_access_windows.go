//go:build windows

package filemanager

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// Capture the effective DACL from the pinned source, and prevent the new
// directory's inheritance from widening it. Supplying this at NtCreateFile is
// essential: tightening access after creation cannot revoke handles already
// opened by another local user. Owners/SACLs are not copied by this operation.
func sourceCreationSecurity(source *os.File) (*windows.SECURITY_DESCRIPTOR, error) {
	sd, err := windows.GetSecurityInfo(windows.Handle(source.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return nil, err
	}
	if acl == nil {
		return nil, errors.New("source file has no explicit access control")
	}
	sd, err = sd.ToAbsolute()
	if err != nil {
		return nil, err
	}
	if err := sd.SetControl(windows.SE_DACL_PROTECTED, windows.SE_DACL_PROTECTED); err != nil {
		return nil, err
	}
	return sd, nil
}

func createFileWithSourceAccess(root *fileRoot, name string, flags int, mode os.FileMode, source *os.File) (*os.File, error) {
	if source == nil {
		return root.OpenFile(name, flags, mode)
	}
	sd, err := sourceCreationSecurity(source)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.FILE_GENERIC_WRITE | windows.READ_CONTROL | windows.WRITE_DAC)
	if flags&os.O_RDWR != 0 {
		access |= windows.FILE_GENERIC_READ
	}
	return root.openWithSecurity(name, access,
		windows.FILE_CREATE, windows.FILE_NON_DIRECTORY_FILE, sd)
}

func mkdirWithSourceAccess(root *fileRoot, name string, mode os.FileMode, source *os.File) error {
	if source == nil {
		return root.Mkdir(name, mode)
	}
	sd, err := sourceCreationSecurity(source)
	if err != nil {
		return err
	}
	file, err := root.openWithSecurity(name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE,
		windows.FILE_CREATE, windows.FILE_DIRECTORY_FILE, sd)
	if err != nil {
		return err
	}
	return file.Close()
}
