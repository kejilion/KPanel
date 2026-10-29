//go:build windows

package filemanager

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"time"
	"unsafe"
)

// A checkpoint must identify the opened object across process restarts. Names,
// size and timestamps cannot distinguish a replacement sibling.
func stableReceiveIdentity(info os.FileInfo, files ...*os.File) string {
	if len(files) != 1 || files[0] == nil {
		return ""
	}
	file := files[0]
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return ""
	}
	var id struct {
		Volume uint64
		ID     [16]byte
	}
	handle := windows.Handle(file.Fd())
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileIdInfo, (*byte)(unsafe.Pointer(&id)), uint32(unsafe.Sizeof(id))); err != nil {
		return ""
	}
	var basic windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &basic); err != nil {
		return ""
	}
	return fmt.Sprintf("winfile:%016x:%x:%08x%08x", id.Volume, id.ID, basic.CreationTime.HighDateTime, basic.CreationTime.LowDateTime)
}
func archiveStagePrivate(info os.FileInfo, roots ...*fileRoot) bool {
	return trustedWindowsDirectory(info, roots, true)
}
func archiveWorkspaceProtected(info os.FileInfo, roots ...*fileRoot) bool {
	return trustedWindowsDirectory(info, roots, false)
}
func publicationParentProtected(info os.FileInfo, roots ...*fileRoot) bool {
	return trustedWindowsDirectory(info, roots, false)
}
func archiveSameFilesystem(a, b os.FileInfo) bool                     { return true }
func inheritArchiveDestinationDefaults(stage, target *fileRoot) error { return nil }
func openOrdinaryCopyRoot(stage, target *fileRoot) (*fileRoot, error) {
	const name = ".copy-payload"
	if err := stage.Mkdir(name, 0700); err != nil {
		return nil, err
	}
	content, err := stage.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	closeOnError := func(err error) (*fileRoot, error) {
		_ = content.Close()
		return nil, err
	}
	targetDirectory, err := target.Open(".")
	if err != nil {
		return closeOnError(err)
	}
	defer targetDirectory.Close()
	security, err := windows.GetSecurityInfo(windows.Handle(targetDirectory.Fd()), windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return closeOnError(err)
	}
	dacl, _, err := security.DACL()
	if err != nil || dacl == nil {
		if err == nil {
			err = errors.New("destination directory has no DACL")
		}
		return closeOnError(err)
	}
	contentDirectory, err := openAccessTarget(content, ".")
	if err != nil {
		return closeOnError(err)
	}
	defer contentDirectory.Close()
	if err := windows.SetSecurityInfo(windows.Handle(contentDirectory.Fd()), windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return closeOnError(err)
	}
	return content, nil
}
func receiveFileTimes(_ *fileRoot, _ string, file *os.File, when time.Time) error {
	stamp := windows.NsecToFiletime(when.UnixNano())
	return windows.SetFileTime(windows.Handle(file.Fd()), nil, &stamp, &stamp)
}
func stageReceiveFile(source *os.File, stage *fileRoot, name string) (os.FileInfo, error) {
	info, err := source.Stat()
	if err != nil {
		return nil, err
	}
	file, err := createFileWithSourceAccess(stage, name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm(), source)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if _, err = source.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if _, err = io.Copy(file, source); err != nil {
		return nil, err
	}
	if err = applySourceAccess(file, source, true); err != nil {
		return nil, err
	}
	if err = file.Sync(); err != nil {
		return nil, err
	}
	if err = receiveFileTimes(stage, name, file, info.ModTime()); err != nil {
		return nil, err
	}
	return file.Stat()
}
func publishReceiveObject(source *fileRoot, sourceName string, target *fileRoot, targetName string, replace bool) error {
	return retryWindowsMutation(func() error {
		from, leaf, volume, err := source.parent(sourceName)
		if err != nil {
			return err
		}
		defer closeParent(from, volume)
		to, newLeaf, targetVolume, err := target.parent(targetName)
		if err != nil {
			return err
		}
		defer closeParent(to, targetVolume)
		if leaf == "." || newLeaf == "." {
			return ErrRootOperation
		}
		handle, err := ntOpen(from, leaf, windows.DELETE|windows.FILE_READ_ATTRIBUTES, windows.FILE_OPEN, 0)
		if err != nil {
			return err
		}
		defer windows.CloseHandle(handle)
		err = renameWindowsHandle(handle, to, newLeaf, replace)
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			return os.ErrExist
		}
		return err
	})
}
