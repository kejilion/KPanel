package backup

import (
	"errors"
	"golang.org/x/sys/windows"
)

func RequireSpace(path string, bytes int64) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(p, &available, &total, &free); err != nil {
		return err
	}
	if bytes < 0 || uint64(bytes)+(64<<20) > available {
		return errors.New("insufficient backup staging space")
	}
	return nil
}
