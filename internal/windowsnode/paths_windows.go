//go:build windows

// Package windowsnode contains the native Windows service and storage boundary.
package windowsnode

import (
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"unsafe"
)

const TelemetryService = "KejilionNode"
const TerminalService = "KejilionNodeTerminal"
const FileService = "KejilionNodeFile"
const LoginService = "KejilionNodeLogin"
const BootstrapService = "KejilionNodeBootstrap"
const UpdateTask = `\KPanel\KejilionNodeUpdate`

func DataDir() string    { return knownPath(windows.FOLDERID_ProgramData) }
func InstallDir() string { return knownPath(windows.FOLDERID_ProgramFiles) }
func knownPath(id *windows.KNOWNFOLDERID) string {
	p, err := windows.KnownFolderPath(id, windows.KF_FLAG_DEFAULT)
	if err != nil {
		panic(fmt.Sprintf("Windows known folder unavailable: %v", err))
	}
	return filepath.Join(p, "KejilionNode")
}
func ConfigPath() string             { return filepath.Join(DataDir(), "node.json") }
func TerminalConfigPath() string     { return filepath.Join(DataDir(), "terminal.json") }
func RuntimePath(name string) string { return filepath.Join(DataDir(), "run", name) }
func IsSystem() bool {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	return err == nil && user.User.Sid.IsWellKnown(windows.WinLocalSystemSid)
}
func DomainJoined() (bool, error) {
	var name *uint16
	var kind uint32
	if err := windows.NetGetJoinInformation(nil, &name, &kind); err != nil {
		return false, err
	}
	defer windows.NetApiBufferFree((*byte)(unsafe.Pointer(name)))
	return kind == windows.NetSetupDomainName, nil
}
