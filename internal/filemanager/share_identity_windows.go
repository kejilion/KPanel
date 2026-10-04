//go:build windows

package filemanager

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

func shareFileIdentity(_ os.FileInfo, file shareVersionFile) (string, bool) {
	handle, ok := file.(interface{ Fd() uintptr })
	if !ok {
		return "", false
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(handle.Fd()), &info); err != nil {
		return "", false
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow, info.CreationTime.HighDateTime, info.CreationTime.LowDateTime), true
}
