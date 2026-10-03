//go:build windows

package terminal

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"unsafe"

	"github.com/kejilion/kejilion-panel/internal/hostpty"
	"golang.org/x/sys/windows"
)

func windowsShell() (string, string, error) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return "", "", err
	}
	shell := filepath.Join(system, `WindowsPowerShell\v1.0\powershell.exe`)
	// Resolve only the system-owned inbox executable. No PATH search, profile,
	// user-supplied SystemRoot, PSModulePath or current directory is inherited.
	for current := shell; len(current) > len(filepath.VolumeName(current))+1; current = filepath.Dir(current) {
		if err := trustedWindowsShellPath(current); err != nil {
			return "", "", err
		}
	}
	return shell, system, nil
}

func trustedWindowsShellPath(path string) error {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(pointer, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("shell path contains a reparse point")
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	trusted := func(sid *windows.SID) bool {
		return sid.String() == "S-1-5-18" || sid.String() == "S-1-5-32-544" || sid.String() == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
	}
	if !trusted(owner) {
		return errors.New("shell path owner is untrusted")
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if dacl == nil {
		return errors.New("shell path has no DACL")
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var header *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, (**windows.ACCESS_ALLOWED_ACE)(unsafe.Pointer(&header))); err != nil {
			return err
		}
		if header.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if header.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE && header.Header.AceType != windows.ACCESS_DENIED_ACE_TYPE {
			return errors.New("unsupported shell DACL entry")
		}
		if header.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&header.SidStart))
		const writeMask = windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES | 0x40 /* FILE_DELETE_CHILD */ | windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER | windows.GENERIC_WRITE | windows.GENERIC_ALL
		if header.Mask&writeMask != 0 && !trusted(sid) {
			return fmt.Errorf("shell path is writable by %s", sid.String())
		}
	}
	return nil
}

func Available() error {
	if err := hostpty.WindowsAvailable(); err != nil {
		return err
	}
	_, _, err := windowsShell()
	return err
}

func platformStarter(rows, columns uint16) (Process, error, bool) {
	shell, system, err := windowsShell()
	if err != nil {
		return nil, err, true
	}
	command := exec.Command(shell, "-NoLogo", "-NoProfile", "-NoExit", "-Command", `[Console]::InputEncoding = [Console]::OutputEncoding = $OutputEncoding = New-Object System.Text.UTF8Encoding`)
	command.Dir = filepath.Dir(system)
	command.Env = []string{
		"SystemRoot=" + filepath.Dir(system), "WINDIR=" + filepath.Dir(system), "ComSpec=" + filepath.Join(system, "cmd.exe"),
		"PATH=" + system + ";" + filepath.Dir(system) + ";" + filepath.Dir(shell),
		"TEMP=" + filepath.Join(filepath.Dir(system), "Temp"), "TMP=" + filepath.Join(filepath.Dir(system), "Temp"),
		"TERM=xterm-256color", "COLORTERM=truecolor", "POWERSHELL_TELEMETRY_OPTOUT=1",
	}
	process, err := hostpty.Start(command, rows, columns)
	return process, err, true
}
