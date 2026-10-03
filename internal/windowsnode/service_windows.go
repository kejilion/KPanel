//go:build windows

package windowsnode

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

type ServiceSpec struct {
	Name, Command, Capability string
	Privileged                bool
}

var Services = []ServiceSpec{
	{TelemetryService, "run", "monitoring", false},
	{TerminalService, "terminal-broker", "terminal", true},
	{FileService, "file-broker", "files", true},
	{LoginService, "login-broker", "login", false},
}

type serviceHandler struct{ run func(context.Context) error }

func (h serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status <- svc.Status{State: svc.StartPending}
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()
	current := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	status <- current
	for {
		select {
		case err := <-done:
			if err != nil {
				return true, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				status <- current
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 20000}
				cancel()
				select {
				case err := <-done:
					if err != nil {
						return true, 1
					}
					return false, 0
				case <-time.After(20 * time.Second):
					return true, 2
				}
			}
		}
	}
}
func RunService(name string, run func(context.Context) error) error {
	return svc.Run(name, serviceHandler{run})
}

func AcquireLifecycle() (func(), error) {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;SY)(A;;GA;;;BA)")
	if err != nil {
		return nil, err
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	name, _ := windows.UTF16PtrFromString(`Global\KejilionNodeLifecycle`)
	h, err := windows.CreateMutex(&sa, false, name)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil, err
	}
	result, err := windows.WaitForSingleObject(h, 0)
	if err != nil || (result != windows.WAIT_OBJECT_0 && result != windows.WAIT_ABANDONED) {
		windows.CloseHandle(h)
		return nil, errors.New("node lifecycle operation in progress; retry later")
	}
	path := filepath.Join(DataDir(), "lifecycle.lock")
	content := []byte(fmt.Sprintf("{\"pid\":%d,\"startedAt\":%q}\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano)))
	if err := WriteAtomic(path, content, StateDirectory); err != nil {
		windows.ReleaseMutex(h)
		windows.CloseHandle(h)
		return nil, err
	}
	return func() { _ = os.Remove(path); windows.ReleaseMutex(h); windows.CloseHandle(h) }, nil
}

func InitializeDirectories() error {
	for _, item := range []struct {
		path   string
		access Access
	}{
		{DataDir(), StateDirectory}, {InstallDir(), ProgramRead},
		{filepath.Join(DataDir(), "run"), StateDirectory},
		{filepath.Join(DataDir(), "run", "login"), LoginSnapshot},
		{filepath.Join(DataDir(), "state"), SystemOnly},
		{filepath.Join(InstallDir(), "staging"), ProgramRead},
	} {
		if err := EnsureDirectory(item.path, item.access); err != nil {
			return fmt.Errorf("initialize %s: %w", item.path, err)
		}
	}
	return nil
}

func InstallServices(capabilities []string) error {
	if !IsSystem() {
		return errors.New("service installation requires SYSTEM")
	}
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	created := []*mgr.Service{}
	rollback := func() {
		for _, s := range created {
			s.Control(svc.Stop)
			s.Delete()
			s.Close()
		}
		_ = runSystemTool("schtasks.exe", "/Delete", "/TN", UpdateTask, "/F")
		_ = eventReaderMembership(false)
	}
	for _, spec := range Services {
		enabled := slices.Contains(capabilities, spec.Capability) || (spec.Name == TerminalService && slices.Contains(capabilities, "desktop"))
		if !enabled {
			continue
		}
		cfg := mgr.Config{StartType: mgr.StartAutomatic, ErrorControl: mgr.ErrorNormal, DisplayName: spec.Name, SidType: windows.SERVICE_SID_TYPE_UNRESTRICTED, DelayedAutoStart: true}
		if !spec.Privileged {
			cfg.ServiceStartName = `NT SERVICE\` + spec.Name
			if spec.Name == TelemetryService {
				cfg.SidType = windows.SERVICE_SID_TYPE_RESTRICTED
			}
		}
		service, err := manager.CreateService(spec.Name, filepath.Join(InstallDir(), "kejilion-node.exe"), cfg, "service", "run", spec.Name)
		if err != nil {
			rollback()
			return err
		}
		created = append(created, service)
		if !spec.Privileged {
			privileges := append(windows.StringToUTF16("SeChangeNotifyPrivilege"), 0)
			info := struct{ Privileges *uint16 }{&privileges[0]}
			if err := windows.ChangeServiceConfig2(service.Handle, windows.SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO, (*byte)(unsafe.Pointer(&info))); err != nil {
				rollback()
				return err
			}
		}
		if err := service.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}, {Type: mgr.ServiceRestart, Delay: 30 * time.Second}, {Type: mgr.ServiceRestart, Delay: 60 * time.Second}}, 86400); err != nil {
			rollback()
			return err
		}
		if err := service.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
			rollback()
			return err
		}
	}
	if slices.Contains(capabilities, "login") {
		if err := eventReaderMembership(true); err != nil {
			rollback()
			return err
		}
	}
	if err := registerUpdateTask(); err != nil {
		rollback()
		return err
	}
	for _, s := range created {
		if err := s.Start(); err != nil {
			rollback()
			return err
		}
	}
	for _, s := range created {
		s.Close()
	}
	return nil
}

func StopServices() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	for i := len(Services) - 1; i >= 0; i-- {
		s, err := m.OpenService(Services[i].Name)
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			continue
		}
		if err != nil {
			return err
		}
		status, err := s.Query()
		if err != nil {
			s.Close()
			return err
		}
		if status.State != svc.Stopped {
			_, err = s.Control(svc.Stop)
			if err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
				s.Close()
				return err
			}
		}
		deadline := time.Now().Add(20 * time.Second)
		for status.State != svc.Stopped && time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			status, err = s.Query()
			if err != nil {
				break
			}
		}
		s.Close()
		if err != nil {
			return err
		}
		if status.State != svc.Stopped {
			return fmt.Errorf("service %s did not stop", Services[i].Name)
		}
	}
	return nil
}
func StartServices() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	for _, spec := range Services {
		s, err := m.OpenService(spec.Name)
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			continue
		}
		if err != nil {
			return err
		}
		err = s.Start()
		s.Close()
		if err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
			return err
		}
	}
	return nil
}
func RemoveServices() error {
	if err := StopServices(); err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	for _, spec := range Services {
		s, err := m.OpenService(spec.Name)
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			continue
		}
		if err != nil {
			return err
		}
		err = s.Delete()
		s.Close()
		if err != nil {
			return err
		}
	}
	if err := eventReaderMembership(false); err != nil {
		return err
	}
	return runSystemTool("schtasks.exe", "/Delete", "/TN", UpdateTask, "/F")
}

func RequestUpdate() error { return runSystemTool("schtasks.exe", "/Run", "/TN", UpdateTask) }

// RemoveInstallation only visits the two fixed known-folder roots. It rejects
// every reparse point before removing anything; in-use executables and their
// parent directory are queued in child-first order for deletion at next boot.
func RemoveInstallation() error {
	if !IsSystem() {
		return errors.New("installation cleanup requires SYSTEM")
	}
	roots := []string{DataDir(), InstallDir()}
	paths := []string{}
	for _, root := range roots {
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) && path == root {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			clean, err := filepath.Rel(root, path)
			if err != nil || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return errors.New("uninstall path left protected root")
			}
			ptr, err := windows.UTF16PtrFromString(path)
			if err != nil {
				return err
			}
			attributes, err := windows.GetFileAttributes(ptr)
			if err != nil {
				return err
			}
			if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
				return errors.New("uninstall refuses reparse points")
			}
			if err := ValidatePath(path, ProgramRead); err != nil {
				if err := ValidatePath(path, LoginSnapshot); err != nil {
					return fmt.Errorf("untrusted uninstall target: %s", path)
				}
			}
			paths = append(paths, path)
			return nil
		}); err != nil {
			return err
		}
	}
	for i := len(paths) - 1; i >= 0; i-- {
		path := paths[i]
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_DIR_NOT_EMPTY) {
				return err
			}
			ptr, _ := windows.UTF16PtrFromString(path)
			if err := windows.MoveFileEx(ptr, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT); err != nil {
				return err
			}
		}
	}
	return nil
}

func runSystemTool(name string, args ...string) error {
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(directory, name), args...)
	command.Stdout = nil
	command.Stderr = nil
	return command.Run()
}
func registerUpdateTask() error {
	executable := filepath.Join(InstallDir(), "kejilion-node-bootstrap.exe")
	escape := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	xml := `<?xml version="1.0" encoding="UTF-8"?><Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><Triggers><BootTrigger><Enabled>true</Enabled><Delay>PT20M</Delay></BootTrigger><TimeTrigger><StartBoundary>` + time.Now().UTC().Add(20*time.Minute).Format(time.RFC3339) + `</StartBoundary><Enabled>true</Enabled><Repetition><Interval>PT1H</Interval><StopAtDurationEnd>false</StopAtDurationEnd></Repetition></TimeTrigger></Triggers><Principals><Principal id="System"><UserId>S-1-5-18</UserId><RunLevel>HighestAvailable</RunLevel></Principal></Principals><Settings><MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy><StartWhenAvailable>true</StartWhenAvailable><DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries><StopIfGoingOnBatteries>false</StopIfGoingOnBatteries><ExecutionTimeLimit>PT10M</ExecutionTimeLimit></Settings><Actions Context="System"><Exec><Command>` + escape.Replace(executable) + `</Command><Arguments>update</Arguments></Exec></Actions></Task>`
	path := filepath.Join(DataDir(), "update-task.xml")
	if err := WriteAtomic(path, []byte(xml), SystemOnly); err != nil {
		return err
	}
	defer os.Remove(path)
	return runSystemTool("schtasks.exe", "/Create", "/TN", UpdateTask, "/XML", path, "/F")
}

func eventReaderMembership(add bool) error {
	groupSID, err := windows.CreateWellKnownSid(windows.WinBuiltinEventLogReadersGroup)
	if err != nil {
		return err
	}
	group, _, _, err := groupSID.LookupAccount("")
	if err != nil {
		return err
	}
	member, err := windows.StringToSid(ServiceSID(LoginService))
	if err != nil {
		return err
	}
	name, _ := windows.UTF16PtrFromString(group)
	entry := struct{ SID *windows.SID }{member}
	api := "NetLocalGroupDelMembers"
	if add {
		api = "NetLocalGroupAddMembers"
	}
	proc := windows.NewLazySystemDLL("netapi32.dll").NewProc(api)
	result, _, _ := proc.Call(0, uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&entry)), 1)
	if result == 0 || (add && result == 1378) || (!add && result == 1377) {
		return nil
	}
	return fmt.Errorf("event reader group membership: Windows error %d", result)
}
