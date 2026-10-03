//go:build windows

package windowsnode

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Access int

const (
	SystemOnly Access = iota
	TelemetryRead
	LoginSnapshot
	ProgramRead
	StateDirectory
)

// Service SIDs are deterministic, including before SCM registration.
func ServiceSID(name string) string {
	units := utf16.Encode([]rune(strings.ToUpper(name)))
	data := make([]byte, len(units)*2)
	for i, value := range units {
		binary.LittleEndian.PutUint16(data[i*2:], value)
	}
	digest := sha1.Sum(data)
	result := "S-1-5-80"
	for i := 0; i < 5; i++ {
		result += fmt.Sprintf("-%d", binary.LittleEndian.Uint32(digest[i*4:]))
	}
	return result
}

func descriptor(access Access, directory bool) (*windows.SECURITY_DESCRIPTOR, error) {
	inheritance := ""
	if directory {
		inheritance = "OICI"
	}
	owner := "BA"
	if IsSystem() {
		owner = "SY"
	} else if access == LoginSnapshot {
		user, err := windows.GetCurrentProcessToken().GetTokenUser()
		if err != nil {
			return nil, err
		}
		if user.User.Sid.String() == ServiceSID(LoginService) {
			owner = user.User.Sid.String()
		}
	}
	sddl := "O:" + owner + "D:P(A;" + inheritance + ";FA;;;SY)"
	if access == StateDirectory || access == ProgramRead {
		sddl += "(A;" + inheritance + ";FA;;;BA)"
	}
	if access == TelemetryRead || access == LoginSnapshot || access == StateDirectory {
		sddl += "(A;" + inheritance + ";FRFX;;;" + ServiceSID(TelemetryService) + ")"
	}
	if access == LoginSnapshot {
		sddl += "(A;" + inheritance + ";FA;;;" + ServiceSID(LoginService) + ")"
	}
	if access == StateDirectory {
		sddl += "(A;;FRFX;;;" + ServiceSID(LoginService) + ")"
	}
	if access == ProgramRead {
		sddl += "(A;" + inheritance + ";FRFX;;;BU)(A;" + inheritance + ";FRFX;;;" + ServiceSID(TelemetryService) + ")(A;" + inheritance + ";FRFX;;;" + ServiceSID(LoginService) + ")"
	}
	return windows.SecurityDescriptorFromString(sddl)
}

// ValidateDescriptor rejects unknown ACE forms rather than assuming they cannot
// grant write access. Secret files allow no read access outside the narrow role.
func validateDescriptor(sd *windows.SECURITY_DESCRIPTOR, access Access, ancestor bool) error {
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return errors.New("file owner unavailable")
	}
	allowedOwner := owner.IsWellKnown(windows.WinLocalSystemSid) || owner.IsWellKnown(windows.WinBuiltinAdministratorsSid)
	if access == LoginSnapshot {
		allowedOwner = allowedOwner || owner.String() == ServiceSID(LoginService)
	}
	if ancestor && owner.String() == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464" {
		allowedOwner = true
	}
	if !allowedOwner {
		return errors.New("untrusted file owner")
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return errors.New("file has no restrictive DACL")
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return err
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return errors.New("unsupported file ACE")
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.IsWellKnown(windows.WinLocalSystemSid) || (ancestor && sid.String() == "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464") {
			continue
		}
		if sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) && (ancestor || access == StateDirectory || access == ProgramRead) {
			continue
		}
		if access == LoginSnapshot && sid.String() == ServiceSID(LoginService) {
			continue
		}
		mask := ace.Mask
		const mutation = windows.GENERIC_ALL | windows.GENERIC_WRITE | windows.WRITE_DAC | windows.WRITE_OWNER | windows.DELETE | windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.FILE_WRITE_EA | windows.FILE_WRITE_ATTRIBUTES | 0x40
		if ancestor {
			// ProgramData permits Users to create a new child; it must not permit them
			// to delete/replace an existing protected child or modify its security.
			if mask&(mutation&^(windows.FILE_WRITE_DATA|windows.FILE_APPEND_DATA)) != 0 {
				return errors.New("unsafe parent DACL")
			}
			continue
		}
		if mask&mutation != 0 {
			return errors.New("untrusted file write access")
		}
		if access == ProgramRead || access == StateDirectory {
			continue
		}
		if (access == TelemetryRead || access == LoginSnapshot) && sid.String() == ServiceSID(TelemetryService) {
			continue
		}
		if mask != 0 {
			return errors.New("untrusted secret read access")
		}
	}
	return nil
}

func openChecked(path string, access Access, ancestor bool, rights uint32) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(p, rights|windows.READ_CONTROL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return 0, err
	}
	fail := func(err error) (windows.Handle, error) { windows.CloseHandle(h); return 0, err }
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return fail(err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fail(errors.New("reparse point rejected"))
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fail(err)
	}
	if err := validateDescriptor(sd, access, ancestor); err != nil {
		return fail(err)
	}
	return h, nil
}

func ValidateFile(file *os.File, access Access) error {
	var info windows.ByHandleFileInformation
	h := windows.Handle(file.Fd())
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return errors.New("reparse point rejected")
	}
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	return validateDescriptor(sd, access, false)
}

func ValidatePath(path string, access Access) error {
	h, err := openChecked(path, access, false, 0)
	if err == nil {
		windows.CloseHandle(h)
	}
	return err
}

func SecureProgramFile(path string) error {
	handles, err := lockParents(path)
	if err != nil {
		return err
	}
	defer closeHandles(handles)
	h, err := openChecked(path, ProgramRead, false, windows.WRITE_DAC|windows.WRITE_OWNER)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	sd, err := descriptor(ProgramRead, false)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil)
}

// LockParents keeps ancestor handles open without FILE_SHARE_DELETE throughout
// each write, closing the rename/junction window between validation and use.
func lockParents(path string) ([]windows.Handle, error) {
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
		return nil, errors.New("local absolute path required")
	}
	parents := []string{}
	for p := filepath.Dir(path); ; p = filepath.Dir(p) {
		parents = append(parents, p)
		if p == filepath.Dir(p) {
			break
		}
	}
	handles := []windows.Handle{}
	for i := len(parents) - 1; i >= 0; i-- {
		h, err := openChecked(parents[i], StateDirectory, true, 0)
		if err != nil {
			closeHandles(handles)
			return nil, fmt.Errorf("unsafe parent %s: %w", parents[i], err)
		}
		handles = append(handles, h)
	}
	return handles, nil
}
func closeHandles(handles []windows.Handle) {
	for _, h := range handles {
		windows.CloseHandle(h)
	}
}

func EnsureDirectory(path string, access Access) error {
	handles, err := lockParents(path)
	if err != nil {
		return err
	}
	defer closeHandles(handles)
	sd, err := descriptor(access, true)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	if err := windows.CreateDirectory(p, &sa); err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return err
	}
	h, err := openChecked(path, access, false, windows.WRITE_DAC|windows.WRITE_OWNER)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, owner, nil, acl, nil)
}

func WriteAtomic(path string, content []byte, access Access) error {
	handles, err := lockParents(path)
	if err != nil {
		return err
	}
	defer closeHandles(handles)
	if _, err := os.Lstat(path); err == nil {
		if err := ValidatePath(path, access); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	sd, err := descriptor(access, false)
	if err != nil {
		return err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	// CREATE_NEW plus a protected descriptor is atomic: no interval with inherited
	// broad read access exists, including for the enrollment and Noise private keys.
	name := path + fmt.Sprintf(".tmp-%d-%d", os.Getpid(), time.Now().UnixNano())
	p, _ := windows.UTF16PtrFromString(name)
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE, 0, &sa, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), name)
	defer os.Remove(name)
	if _, err = f.Write(content); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	target, _ := windows.UTF16PtrFromString(path)
	for i := 0; i < 5; i++ {
		err = windows.MoveFileEx(p, target, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil {
			return nil
		}
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return err
		}
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return err
}

func ReadFile(path string, limit int64, access Access) ([]byte, error) {
	handles, err := lockParents(path)
	if err != nil {
		return nil, err
	}
	defer closeHandles(handles)
	h, err := openChecked(path, access, false, windows.GENERIC_READ)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("invalid protected file")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("protected file exceeds limit")
	}
	return data, nil
}
