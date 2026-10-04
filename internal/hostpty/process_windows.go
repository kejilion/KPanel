//go:build windows

package hostpty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowsAvailable performs feature detection without creating a process.
func WindowsAvailable() error {
	return windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find()
}

type windowsProcess struct {
	input, output         *os.File
	console, job, process windows.Handle
	mu                    sync.Mutex
	closeOnce             sync.Once
	closed                chan struct{}
	done                  chan struct{}
	chunks                chan []byte
	readMu                sync.Mutex
	pending               []byte
	waitErr               error
}

func startPlatform(command *exec.Cmd, rows, columns uint16) (Process, error) {
	if rows == 0 || columns == 0 || rows > 500 || columns > 1000 {
		return nil, errors.New("invalid terminal dimensions")
	}
	if err := WindowsAvailable(); err != nil {
		return nil, fmt.Errorf("ConPTY requires Windows 10 1809 / Server 2019: %w", err)
	}
	if command == nil || !strings.Contains(command.Path, `:\`) {
		return nil, errors.New("terminal executable must have an absolute path")
	}
	var inputRead, inputWrite, outputRead, outputWrite windows.Handle
	if err := windows.CreatePipe(&inputRead, &inputWrite, nil, 0); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(inputRead)
	if err := windows.CreatePipe(&outputRead, &outputWrite, nil, 0); err != nil {
		windows.CloseHandle(inputWrite)
		return nil, err
	}
	defer windows.CloseHandle(outputWrite)
	p := &windowsProcess{input: os.NewFile(uintptr(inputWrite), "conpty-input"), output: os.NewFile(uintptr(outputRead), "conpty-output"), closed: make(chan struct{}), done: make(chan struct{}), chunks: make(chan []byte, 64)}
	if err := windows.CreatePseudoConsole(windows.Coord{X: int16(columns), Y: int16(rows)}, inputRead, outputWrite, 0, &p.console); err != nil {
		p.input.Close()
		p.output.Close()
		return nil, err
	}
	// Drain output before spawning and throughout ClosePseudoConsole. Its close
	// is synchronous and can otherwise deadlock when the console flushes output.
	go p.pump()
	failed := true
	defer func() {
		if failed {
			_ = p.Close()
		}
	}()
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	p.job = job
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	// PSEUDOCONSOLE is the unusual attribute whose lpValue is the opaque handle
	// value itself, not an address. Keep it uintptr across the syscall boundary
	// instead of converting a non-pointer HANDLE into an unsafe.Pointer.
	if ok, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("UpdateProcThreadAttribute").Call(uintptr(unsafe.Pointer(attrs.List())), 0, windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(p.console), unsafe.Sizeof(p.console), 0, 0); ok == 0 {
		return nil, err
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	// Do not copy the parent's redirected standard handles into the child.
	// Explicit unavailable handles prevent duplication of redirected parent
	// handles while ConPTY supplies the child's console connection.
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput = windows.InvalidHandle
	si.StdOutput = windows.InvalidHandle
	si.StdErr = windows.InvalidHandle
	application, err := windows.UTF16PtrFromString(command.Path)
	if err != nil {
		return nil, err
	}
	args := make([]string, len(command.Args))
	for i, arg := range command.Args {
		args[i] = windows.EscapeArg(arg)
	}
	line, err := windows.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		return nil, err
	}
	directory, err := windows.UTF16PtrFromString(command.Dir)
	if err != nil {
		return nil, err
	}
	environment := append([]string(nil), command.Env...)
	if len(environment) == 0 {
		return nil, errors.New("terminal requires an explicit environment")
	}
	sort.Slice(environment, func(i, j int) bool { return strings.ToUpper(environment[i]) < strings.ToUpper(environment[j]) })
	// Encode each variable separately: UTF16FromString rejects interior NULs.
	var env []uint16
	for _, item := range environment {
		value, e := windows.UTF16FromString(item)
		if e != nil {
			return nil, e
		}
		env = append(env, value...)
	}
	env = append(env, 0)
	var pi windows.ProcessInformation
	err = windows.CreateProcess(application, line, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_SUSPENDED, &env[0], directory, &si.StartupInfo, &pi)
	if err != nil {
		return nil, err
	}
	p.process = pi.Process
	defer windows.CloseHandle(pi.Thread)
	// No user code may execute before containment: even very short-lived shells
	// must not be able to fork outside the kill-on-close Job Object.
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Process)
		p.process = 0
		return nil, err
	}
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		windows.TerminateJobObject(job, 1)
		windows.CloseHandle(pi.Process)
		p.process = 0
		return nil, err
	}
	go func() {
		_, err := windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		if err == nil {
			var code uint32
			err = windows.GetExitCodeProcess(pi.Process, &code)
			if err == nil && code != 0 {
				err = fmt.Errorf("terminal process exited with code %d", code)
			}
		}
		p.waitErr = err
		close(p.done)
		p.mu.Lock()
		// Natural shell exit also ends the process tree and the pseudoconsole,
		// allowing capture to observe EOF after draining the remaining bytes.
		if p.job != 0 {
			windows.TerminateJobObject(p.job, 1)
			windows.CloseHandle(p.job)
			p.job = 0
		}
		_ = p.input.Close()
		if p.console != 0 {
			windows.ClosePseudoConsole(p.console)
			p.console = 0
		}
		if p.process != 0 {
			windows.CloseHandle(p.process)
			p.process = 0
		}
		p.mu.Unlock()
	}()
	failed = false
	return p, nil
}

func (p *windowsProcess) pump() {
	defer close(p.chunks)
	defer p.output.Close()
	buffer := make([]byte, 4096)
	for {
		n, err := p.output.Read(buffer)
		if n > 0 {
			chunk := append([]byte(nil), buffer[:n]...)
			select {
			case p.chunks <- chunk:
			case <-p.closed:
			}
		}
		if err != nil {
			return
		}
	}
}
func (p *windowsProcess) Read(b []byte) (int, error) {
	p.readMu.Lock()
	defer p.readMu.Unlock()
	if len(b) == 0 {
		return 0, nil
	}
	if len(p.pending) == 0 {
		chunk, ok := <-p.chunks
		if !ok {
			return 0, io.EOF
		}
		p.pending = chunk
	}
	n := copy(b, p.pending)
	p.pending = p.pending[n:]
	return n, nil
}
func (p *windowsProcess) Write(b []byte) (int, error) { return p.input.Write(b) }
func (p *windowsProcess) Wait() error                 { <-p.done; return p.waitErr }
func (p *windowsProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return nil
	}
	return windows.TerminateJobObject(p.job, 1)
}
func (p *windowsProcess) Resize(rows, columns uint16) error {
	if rows == 0 || columns == 0 || rows > 500 || columns > 1000 {
		return errors.New("invalid terminal dimensions")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.console == 0 {
		return os.ErrClosed
	}
	return windows.ResizePseudoConsole(p.console, windows.Coord{X: int16(columns), Y: int16(rows)})
}
func (p *windowsProcess) Close() error {
	p.closeOnce.Do(func() {
		close(p.closed)
		p.mu.Lock()
		if p.job != 0 {
			windows.TerminateJobObject(p.job, 1)
			windows.CloseHandle(p.job)
			p.job = 0
		}
		_ = p.input.Close()
		if p.console != 0 {
			windows.ClosePseudoConsole(p.console)
			p.console = 0
		}
		_ = p.output.Close()
		if p.process != 0 {
			<-p.done
			windows.CloseHandle(p.process)
			p.process = 0
		}
		p.mu.Unlock()
	})
	return nil
}

// FIFO job terminals are Linux-only. Windows interactive terminals use ConPTY
// directly; unsupported FIFO requests must fail rather than claim success.
func createPlatformInput(string) error {
	return errors.New("FIFO job terminals are unavailable on Windows")
}
func openPlatformInput(string) (*os.File, error) {
	return nil, errors.New("FIFO job terminals are unavailable on Windows")
}
func writePlatformInput(string, []byte) error {
	return notWritten(errors.New("FIFO job terminals are unavailable on Windows"))
}
func removePlatformInput(string) error {
	return errors.New("FIFO job terminals are unavailable on Windows")
}
func isPlatformEnd(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, os.ErrClosed)
}
