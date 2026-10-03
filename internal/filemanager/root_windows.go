//go:build windows

package filemanager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unsafe"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"golang.org/x/sys/windows"
)

type windowsVolume struct {
	root   *os.Root
	handle windows.Handle
	base   string
}
type fileRoot struct {
	volumes map[string]*windowsVolume
	virtual bool
	mu      sync.RWMutex
	closed  bool
}

func (m *Manager) platformCanonical(value string) (string, error) {
	if value == "/" {
		return value, nil
	}
	file, err := m.rootFS.Open(rootName(value))
	if err != nil {
		return "", err
	}
	defer file.Close()
	var buffer [32768]uint16
	n, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", err
	}
	if n >= uint32(len(buffer)) {
		return "", ErrInvalidPath
	}
	physical := strings.TrimPrefix(windows.UTF16ToString(buffer[:n]), `\\?\`)
	key := ""
	if m.rootFS.virtual {
		parts := strings.Split(strings.TrimPrefix(value, "/"), "/")
		key = strings.ToUpper(parts[0])
	}
	v := m.rootFS.volumes[key]
	if v == nil {
		return "", ErrInvalidPath
	}
	// GetFinalPathNameByHandle expands 8.3 aliases and preserves the filesystem's
	// spelling. Compare protections using this canonical virtual pathname.
	base := v.base
	relative, err := filepath.Rel(base, physical)
	if err != nil || relative == ".." || strings.HasPrefix(relative, `..\`) {
		return "", ErrInvalidPath
	}
	prefix := ""
	if m.rootFS.virtual {
		prefix = "/" + key
	}
	if relative == "." {
		if prefix == "" {
			return "/", nil
		}
		return prefix, nil
	}
	return prefix + "/" + filepath.ToSlash(relative), nil
}

func platformVirtualRoot(root string) bool { return root == "/" }
func platformPathKey(value string) string  { return strings.ToLower(value) }
func platformNameValid(value string) bool {
	if strings.ContainsAny(value, `<>:"\|?*`) || strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(value, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return false
	}
	if len(base) >= 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) {
		suffix := []rune(base[3:])
		if len(suffix) == 1 && strings.ContainsRune("123456789¹²³", suffix[0]) {
			return false
		}
	}
	return value != "." && value != ".."
}

func openFileRoot(root string, bindings map[string]string) (*fileRoot, error) {
	r := &fileRoot{volumes: make(map[string]*windowsVolume), virtual: root == "/"}
	autoDiscover := r.virtual && len(bindings) == 0
	if !r.virtual {
		bindings = map[string]string{"": root}
	} else if len(bindings) == 0 {
		bindings = make(map[string]string)
		mask, err := windows.GetLogicalDrives()
		if err != nil {
			return nil, err
		}
		for n := uint32(0); n < 26; n++ {
			if mask&(1<<n) == 0 {
				continue
			}
			letter := string(rune('A' + n))
			drive := letter + `:\`
			ptr, _ := windows.UTF16PtrFromString(drive)
			if windows.GetDriveType(ptr) == windows.DRIVE_FIXED {
				bindings[letter] = drive
			}
		}
	}
	for letter, path := range bindings {
		if r.virtual && (len(letter) != 1 || strings.ToUpper(letter) < "A" || strings.ToUpper(letter) > "Z") {
			r.Close()
			return nil, ErrInvalidPath
		}
		letter = strings.ToUpper(letter)
		if _, exists := r.volumes[letter]; exists {
			r.Close()
			return nil, ErrInvalidPath
		}
		clean := filepath.Clean(path)
		if !filepath.IsAbs(clean) || strings.HasPrefix(clean, `\\`) {
			r.Close()
			return nil, ErrInvalidPath
		}
		// Walk from the drive handle instead of following a user-writable
		// ancestor. Every component is opened with FILE_OPEN_REPARSE_POINT.
		base := filepath.VolumeName(clean) + `\`
		ptr, err := windows.UTF16PtrFromString(base)
		if err != nil {
			r.Close()
			return nil, err
		}
		h, err := windows.CreateFile(ptr, windows.FILE_GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			if autoDiscover {
				continue
			}
			r.Close()
			return nil, err
		}
		rest := strings.TrimPrefix(clean, base)
		if rest != "" {
			for _, part := range strings.Split(rest, `\`) {
				child, e := ntOpen(h, part, windows.FILE_GENERIC_READ, windows.FILE_OPEN, windows.FILE_DIRECTORY_FILE)
				windows.CloseHandle(h)
				if e != nil {
					r.Close()
					return nil, e
				}
				h = child
			}
		}
		osRoot, err := os.OpenRoot(clean)
		if err != nil {
			windows.CloseHandle(h)
			if autoDiscover {
				continue
			}
			r.Close()
			return nil, err
		}
		var canonical [32768]uint16
		n, err := windows.GetFinalPathNameByHandle(h, &canonical[0], uint32(len(canonical)), 0)
		if err != nil || n >= uint32(len(canonical)) {
			osRoot.Close()
			windows.CloseHandle(h)
			if autoDiscover {
				continue
			}
			r.Close()
			if err == nil {
				err = ErrInvalidPath
			}
			return nil, err
		}
		r.volumes[letter] = &windowsVolume{root: osRoot, handle: h, base: strings.TrimPrefix(windows.UTF16ToString(canonical[:n]), `\\?\`)}
	}
	if len(r.volumes) == 0 {
		return nil, errors.New("no accessible local fixed volumes")
	}
	return r, nil
}

func ntOpen(parent windows.Handle, name string, access, disposition, options uint32) (windows.Handle, error) {
	if name == "." {
		name = ""
	}
	name16, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: parent, ObjectName: name16, Attributes: windows.OBJ_CASE_INSENSITIVE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h windows.Handle
	err = windows.NtCreateFile(&h, access|windows.SYNCHRONIZE|windows.FILE_READ_ATTRIBUTES, &oa, &windows.IO_STATUS_BLOCK{}, nil, windows.FILE_ATTRIBUTE_NORMAL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, disposition, options|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT, 0, 0)
	if err != nil {
		return 0, ntError(err)
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		windows.CloseHandle(h)
		return 0, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		windows.CloseHandle(h)
		return 0, ErrSymlink
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 && info.NumberOfLinks > 1 {
		windows.CloseHandle(h)
		return 0, errors.New("不允许通过硬链接访问文件")
	}
	return h, nil
}
func ntError(err error) error {
	var status windows.NTStatus
	if errors.As(err, &status) {
		err = status.Errno()
	}
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return fmt.Errorf("文件被占用，请关闭占用程序后重试: %w", err)
	}
	return err
}

func retryWindowsMutation(run func() error) error {
	for attempt := 0; ; attempt++ {
		err := run()
		if attempt == 3 || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return err
		}
		time.Sleep(time.Duration(50*(1<<attempt)) * time.Millisecond)
	}
}

// Each step is relative to an already-open directory handle. A concurrent
// junction replacement cannot redirect this operation into another subtree.
func (r *fileRoot) parent(name string) (windows.Handle, string, *windowsVolume, error) {
	name = filepath.ToSlash(name)
	parts := strings.Split(name, "/")
	key := ""
	if r.virtual {
		if len(parts) == 0 || len(parts[0]) != 1 {
			return 0, "", nil, ErrRootOperation
		}
		key = strings.ToUpper(parts[0])
		parts = parts[1:]
	}
	v := r.volumes[key]
	if v == nil {
		return 0, "", nil, os.ErrNotExist
	}
	if len(parts) == 0 || len(parts) == 1 && parts[0] == "." {
		return v.handle, ".", v, nil
	}
	parent := v.handle
	for _, part := range parts[:len(parts)-1] {
		if !platformNameValid(part) || part == "" {
			if parent != v.handle {
				windows.CloseHandle(parent)
			}
			return 0, "", nil, ErrInvalidPath
		}
		child, err := ntOpen(parent, part, windows.FILE_GENERIC_READ, windows.FILE_OPEN, windows.FILE_DIRECTORY_FILE)
		if parent != v.handle {
			windows.CloseHandle(parent)
		}
		if err != nil {
			return 0, "", nil, err
		}
		parent = child
	}
	leaf := parts[len(parts)-1]
	if !platformNameValid(leaf) || leaf == "" {
		if parent != v.handle {
			windows.CloseHandle(parent)
		}
		return 0, "", nil, ErrInvalidPath
	}
	return parent, leaf, v, nil
}
func closeParent(h windows.Handle, v *windowsVolume) {
	if h != v.handle {
		windows.CloseHandle(h)
	}
}
func (r *fileRoot) open(name string, access, disposition, options uint32) (*os.File, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, os.ErrClosed
	}
	parent, leaf, v, err := r.parent(name)
	if err != nil {
		return nil, err
	}
	defer closeParent(parent, v)
	h, err := ntOpen(parent, leaf, access, disposition, options)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	relative := filepath.ToSlash(name)
	if r.virtual {
		_, relative, _ = strings.Cut(relative, "/")
	}
	return os.NewFile(uintptr(h), filepath.Join(v.base, filepath.FromSlash(relative))), nil
}
func (r *fileRoot) Open(name string) (*os.File, error) {
	return r.open(name, windows.FILE_GENERIC_READ, windows.FILE_OPEN, 0)
}
func (r *fileRoot) OpenFile(name string, flag int, _ os.FileMode) (*os.File, error) {
	access := uint32(windows.FILE_GENERIC_READ)
	if flag&os.O_WRONLY != 0 {
		access = windows.FILE_GENERIC_WRITE | windows.READ_CONTROL | windows.WRITE_DAC
	}
	if flag&os.O_RDWR != 0 {
		access = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.READ_CONTROL | windows.WRITE_DAC
	}
	disposition := uint32(windows.FILE_OPEN)
	if flag&os.O_CREATE != 0 {
		disposition = windows.FILE_OPEN_IF
		if flag&os.O_EXCL != 0 {
			disposition = windows.FILE_CREATE
		}
	}
	file, err := r.open(name, access, disposition, windows.FILE_NON_DIRECTORY_FILE)
	if err != nil {
		return nil, err
	}
	if flag&os.O_TRUNC != 0 {
		if err := file.Truncate(0); err != nil {
			file.Close()
			return nil, err
		}
	}
	if flag&os.O_APPEND != 0 {
		if _, err := file.Seek(0, io.SeekEnd); err != nil {
			file.Close()
			return nil, err
		}
	}
	return file, nil
}
func (r *fileRoot) Stat(name string) (os.FileInfo, error) {
	if name == "." && r.virtual {
		r.mu.RLock()
		defer r.mu.RUnlock()
		if r.closed {
			return nil, os.ErrClosed
		}
		return virtualRootInfo{}, nil
	}
	file, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}
func (r *fileRoot) Lstat(name string) (os.FileInfo, error) { return r.Stat(name) }
func (r *fileRoot) ReadFile(name string) ([]byte, error) {
	file, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}
func (r *fileRoot) Mkdir(name string, _ os.FileMode) error {
	file, err := r.open(name, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE, windows.FILE_CREATE, windows.FILE_DIRECTORY_FILE)
	if err != nil {
		return err
	}
	return file.Close()
}
func (r *fileRoot) MkdirAll(name string, mode os.FileMode) error {
	parts := strings.Split(filepath.ToSlash(name), "/")
	start := 0
	if r.virtual {
		start = 1
	}
	for i := start; i < len(parts); i++ {
		current := strings.Join(parts[:i+1], "/")
		if err := r.Mkdir(current, mode); err != nil {
			info, e := r.Stat(current)
			if e != nil || !info.IsDir() {
				return err
			}
		}
	}
	return nil
}
func (r *fileRoot) Chmod(name string, mode os.FileMode) error {
	file, err := r.open(name, windows.FILE_GENERIC_READ|windows.FILE_WRITE_ATTRIBUTES, windows.FILE_OPEN, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Chmod(mode)
}
func (r *fileRoot) Chtimes(name string, atime, mtime time.Time) error {
	file, err := r.open(name, windows.FILE_WRITE_ATTRIBUTES, windows.FILE_OPEN, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	a, m := windows.NsecToFiletime(atime.UnixNano()), windows.NsecToFiletime(mtime.UnixNano())
	return windows.SetFileTime(windows.Handle(file.Fd()), nil, &a, &m)
}
func (r *fileRoot) OpenRoot(name string) (*os.Root, error) {
	// Archive state lives beneath the protected broker state directory. Pin and
	// validate it before handing the existing private store its rooted handle.
	file, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var path [32768]uint16
	n, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &path[0], uint32(len(path)), 0)
	if err != nil {
		return nil, err
	}
	if n >= uint32(len(path)) {
		return nil, ErrInvalidPath
	}
	return os.OpenRoot(windows.UTF16ToString(path[:n]))
}
func (r *fileRoot) rename(oldName, newName string, replace bool) error {
	return retryWindowsMutation(func() error { return r.renameOnce(oldName, newName, replace) })
}

func (r *fileRoot) renameOnce(oldName, newName string, replace bool) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return os.ErrClosed
	}
	oldParent, oldLeaf, oldVolume, err := r.parent(oldName)
	if err != nil {
		return err
	}
	defer closeParent(oldParent, oldVolume)
	newParent, newLeaf, newVolume, err := r.parent(newName)
	if err != nil {
		return err
	}
	defer closeParent(newParent, newVolume)
	if oldLeaf == "." || newLeaf == "." {
		return ErrRootOperation
	}
	if oldVolume != newVolume {
		return windows.ERROR_NOT_SAME_DEVICE
	}
	h, err := ntOpen(oldParent, oldLeaf, windows.DELETE|windows.FILE_READ_ATTRIBUTES, windows.FILE_OPEN, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	name, err := windows.UTF16FromString(newLeaf)
	if err != nil {
		return err
	}
	name = name[:len(name)-1]
	type renameInfo struct {
		Replace uint32
		Root    windows.Handle
		Length  uint32
		Name    [1]uint16
	}
	var header renameInfo
	size := int(unsafe.Offsetof(header.Name)) + len(name)*2
	buffer := make([]byte, size)
	info := (*renameInfo)(unsafe.Pointer(&buffer[0]))
	if replace {
		info.Replace = 1
	}
	info.Root = newParent
	info.Length = uint32(len(name) * 2)
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&info.Name[0])), len(name)), name)
	if replace {
		// POSIX replacement permits an already-open reader to finish using the
		// old object, matching the existing atomic text-save contract.
		info.Replace = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
		err := windows.NtSetInformationFile(h, &windows.IO_STATUS_BLOCK{}, &buffer[0], uint32(len(buffer)), 65)
		if err == nil {
			return nil
		}
		if !errors.Is(err, windows.STATUS_INVALID_INFO_CLASS) && !errors.Is(err, windows.STATUS_NOT_SUPPORTED) && !errors.Is(err, windows.STATUS_INVALID_PARAMETER) {
			return ntError(err)
		}
		info.Replace = 1
	}
	return ntError(windows.NtSetInformationFile(h, &windows.IO_STATUS_BLOCK{}, &buffer[0], uint32(len(buffer)), windows.FileRenameInformation))
}
func (r *fileRoot) Rename(oldName, newName string) error { return r.rename(oldName, newName, true) }
func (r *fileRoot) Remove(name string) error {
	return retryWindowsMutation(func() error { return r.removeOnce(name) })
}

func (r *fileRoot) removeOnce(name string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return os.ErrClosed
	}
	parent, leaf, v, err := r.parent(name)
	if err != nil {
		return err
	}
	defer closeParent(parent, v)
	if leaf == "." {
		return ErrRootOperation
	}
	h, err := ntOpen(parent, leaf, windows.DELETE|windows.FILE_READ_ATTRIBUTES, windows.FILE_OPEN, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	flag := byte(1)
	return ntError(windows.NtSetInformationFile(h, &windows.IO_STATUS_BLOCK{}, &flag, 1, windows.FileDispositionInformation))
}
func (r *fileRoot) RemoveAll(name string) error {
	file, err := r.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	if info.IsDir() {
		for {
			entries, readErr := file.ReadDir(128)
			for _, entry := range entries {
				if err := r.RemoveAll(filepath.Join(name, entry.Name())); err != nil {
					file.Close()
					return err
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				file.Close()
				return readErr
			}
		}
	}
	file.Close()
	return r.Remove(name)
}
func (r *fileRoot) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	var errs []error
	for _, v := range r.volumes {
		errs = append(errs, v.root.Close(), windows.CloseHandle(v.handle))
	}
	return errors.Join(errs...)
}

type virtualRootInfo struct{}

func (virtualRootInfo) Name() string       { return "/" }
func (virtualRootInfo) Size() int64        { return 0 }
func (virtualRootInfo) Mode() os.FileMode  { return os.ModeDir | 0555 }
func (virtualRootInfo) ModTime() time.Time { return time.Time{} }
func (virtualRootInfo) IsDir() bool        { return true }
func (virtualRootInfo) Sys() any           { return nil }
func (m *Manager) listPlatformRoot(ctx context.Context, virtual string, options ListOptions) (contract.FileDirectory, error, bool) {
	if !m.rootFS.virtual || virtual != "/" && virtual != "" {
		return contract.FileDirectory{}, nil, false
	}
	m.rootFS.mu.RLock()
	defer m.rootFS.mu.RUnlock()
	if m.rootFS.closed {
		return contract.FileDirectory{}, os.ErrClosed, true
	}
	if err := ctx.Err(); err != nil {
		return contract.FileDirectory{}, err, true
	}
	if options.Offset < 0 || len(options.Search) > MaxSearchBytes {
		return contract.FileDirectory{}, ErrInvalidPath, true
	}
	var keys []string
	for key := range m.rootFS.volumes {
		if strings.Contains(strings.ToLower(key), strings.ToLower(strings.TrimSpace(options.Search))) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	entries := make([]contract.FileEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, contract.FileEntry{Name: key + ":", Path: "/" + key, Kind: "directory", Mode: "dr-xr-xr-x", ResourceVersion: "volume:" + key})
	}
	limit := options.Limit
	if limit <= 0 || limit > MaxDirectoryEntries {
		limit = MaxDirectoryEntries
	}
	start := min(options.Offset, len(entries))
	end := min(start+limit, len(entries))
	next := 0
	if end < len(entries) {
		next = end
	}
	return contract.FileDirectory{Path: "/", Entries: entries[start:end], Total: len(entries), TotalKnown: true, Offset: options.Offset, NextOffset: next, Truncated: next > 0, ReadAt: m.now().UTC()}, nil, true
}
