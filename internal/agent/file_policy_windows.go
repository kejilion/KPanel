//go:build windows

package agent

import (
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
)

func platformFileManagerConfig(stateDirectory string) (filemanager.Config, bool) {
	program, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return filemanager.Config{}, true
	}
	data, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return filemanager.Config{}, true
	}
	virtual := func(value string) string {
		value = filepath.Clean(value)
		volume := filepath.VolumeName(value)
		if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(value) {
			return ""
		}
		return "/" + strings.ToUpper(volume[:1]) + "/" + strings.TrimPrefix(filepath.ToSlash(value[len(volume):]), "/")
	}
	if stateDirectory == "" {
		stateDirectory = filepath.Join(data, "KejilionNode", "state")
	}
	state := virtual(stateDirectory)
	if state == "" {
		return filemanager.Config{}, true
	}
	return filemanager.Config{Root: "/", TrashVirtual: state + "/file-trash", ProtectedVirtual: []string{virtual(filepath.Join(program, "KejilionNode")), virtual(filepath.Join(data, "KejilionNode")), state}}, true
}
