//go:build windows

package filemanager

import (
	"errors"
	"os"
)

func (*Manager) chmodNoFollow(string, os.FileMode, os.FileInfo) error {
	return errors.New("Windows 文件权限由 ACL 管理，不支持 POSIX chmod")
}
