//go:build windows

package hostpty

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

type conPTYTranscript struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (transcript *conPTYTranscript) Write(data []byte) (int, error) {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	if remaining := (64 << 10) - transcript.buffer.Len(); remaining > 0 {
		_, _ = transcript.buffer.Write(data[:min(len(data), remaining)])
	}
	return len(data), nil
}

func (transcript *conPTYTranscript) snapshot() string {
	transcript.mu.Lock()
	defer transcript.mu.Unlock()
	return transcript.buffer.String()
}

func conPTYCommand(t *testing.T, script string) *exec.Cmd {
	t.Helper()
	if err := WindowsAvailable(); err != nil {
		t.Skipf("ConPTY unavailable: %v", err)
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	// Load the fixed inbox cmdlets directly rather than discovering unrelated
	// runner modules via the machine's optional analysis cache.
	moduleRoot := filepath.Join(system, `WindowsPowerShell\v1.0\Modules`)
	var setup strings.Builder
	for _, name := range []string{"Microsoft.PowerShell.Utility", "Microsoft.PowerShell.Management"} {
		manifest := strings.ReplaceAll(filepath.Join(moduleRoot, name, name+".psd1"), "'", "''")
		setup.WriteString("Import-Module -ErrorAction Stop '" + manifest + "'; ")
	}
	setup.WriteString("[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new(); ")
	units := utf16.Encode([]rune(setup.String() + script))
	encoded := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(encoded[i*2:], u)
	}
	command := exec.Command(filepath.Join(system, `WindowsPowerShell\v1.0\powershell.exe`), "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(encoded))
	command.Dir = t.TempDir()
	command.Env = []string{"SystemRoot=" + filepath.Dir(system), "WINDIR=" + filepath.Dir(system), "PATH=" + system, "TEMP=" + command.Dir, "TMP=" + command.Dir}
	return command
}

func TestWindowsConPTYUnicodeResizeAndNaturalExit(t *testing.T) {
	p, err := Start(conPTYCommand(t, `Write-Output '中文终端-OK'; exit 0`), 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	if err := p.Resize(40, 120); err != nil {
		t.Fatal(err)
	}
	if err := p.Resize(0, 120); err == nil {
		t.Fatal("accepted zero dimensions")
	}
	done := make(chan struct{})
	var transcript conPTYTranscript
	var readErr, waitErr error
	go func() { _, readErr = io.Copy(&transcript, p); waitErr = p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("ConPTY exit did not produce EOF; output=%q", transcript.snapshot())
	}
	output := []byte(transcript.snapshot())
	if readErr != nil || waitErr != nil {
		t.Fatalf("read=%v wait=%v output=%q", readErr, waitErr, output)
	}
	if !bytes.Contains(output, []byte("中文终端-OK")) {
		t.Fatalf("UTF-8 output lost: %q", output)
	}
}

// These controls distinguish inbox-shell initialization from ConPTY transport
// failures while keeping the same pinned executable and minimal environment.
func TestWindowsInboxShellExplicitEnvironment(t *testing.T) {
	fixture := conPTYCommand(t, `Write-Output 'DIRECT_READY'; exit 0`)
	systemFolders := append([]string(nil), fixture.Env...)
	for _, name := range []string{"ComSpec", "SystemDrive", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "ProgramData", "ProgramFiles", "ProgramFiles(x86)", "ProgramW6432"} {
		if value := os.Getenv(name); value != "" {
			systemFolders = append(systemFolders, name+"="+value)
		}
	}
	for _, control := range []struct {
		name string
		env  []string
	}{{"minimal", fixture.Env}, {"system-folders", systemFolders}, {"runner-environment", nil}} {
		t.Run(control.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, fixture.Path, fixture.Args[1:]...)
			command.Env, command.Dir = control.env, fixture.Dir
			command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			output, err := command.CombinedOutput()
			if err != nil || !bytes.Contains(output, []byte("DIRECT_READY")) {
				t.Fatalf("direct inbox shell failed: err=%v output=%q", err, output)
			}
		})
	}
}

func TestWindowsConPTYCommandInterpreter(t *testing.T) {
	command := conPTYCommand(t, "")
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	command.Path = filepath.Join(system, "cmd.exe")
	command.Args = []string{command.Path, "/d", "/c", "echo CMD_READY"}
	p, err := Start(command, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	var transcript conPTYTranscript
	done := make(chan struct{})
	var readErr, waitErr error
	go func() { _, readErr = io.Copy(&transcript, p); waitErr = p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("command interpreter did not exit; output=%q", transcript.snapshot())
	}
	if readErr != nil || waitErr != nil || !strings.Contains(transcript.snapshot(), "CMD_READY") {
		t.Fatalf("command interpreter failed: read=%v wait=%v output=%q", readErr, waitErr, transcript.snapshot())
	}
}

func TestWindowsConPTYInteractiveInput(t *testing.T) {
	command := conPTYCommand(t, "")
	command.Args = []string{command.Path, "-NoLogo", "-NoProfile", "-NoExit", "-Command",
		`[Console]::InputEncoding = [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new(); [Console]::WriteLine('PTY_READY')`}
	p, err := Start(command, 24, 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	var transcript conPTYTranscript
	done := make(chan struct{})
	var readErr, waitErr error
	go func() { _, readErr = io.Copy(&transcript, p); waitErr = p.Wait(); close(done) }()
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(transcript.snapshot(), "PTY_READY") {
		if time.Now().After(deadline) {
			t.Fatalf("interactive shell did not become ready; output=%q", transcript.snapshot())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// The expected answer is absent from the input, so an echoed command cannot
	// make the assertion pass without PowerShell actually executing it.
	if _, err := io.WriteString(p, `$reply = 6 * 7; [Console]::WriteLine('PTY_REPLY=' + $reply); exit 0`+"\r"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("interactive command did not exit; output=%q", transcript.snapshot())
	}
	if readErr != nil || waitErr != nil || !strings.Contains(transcript.snapshot(), "PTY_REPLY=42") {
		t.Fatalf("interactive execution failed: read=%v wait=%v output=%q", readErr, waitErr, transcript.snapshot())
	}
}

func TestWindowsConPTYCloseKillsDescendants(t *testing.T) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	shell := strings.ReplaceAll(filepath.Join(system, `WindowsPowerShell\v1.0\powershell.exe`), "'", "''")
	script := `$child = Start-Process -WindowStyle Hidden -PassThru '` + shell + `' -ArgumentList '-NoProfile','-NonInteractive','-Command','Start-Sleep 120'; Write-Output ('CHILD_PID=' + $child.Id + ';'); Start-Sleep 120`
	p, err := Start(conPTYCommand(t, script), 24, 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	pidReady := make(chan uint32, 1)
	var transcript conPTYTranscript
	go func() {
		var output []byte
		buffer := make([]byte, 4096)
		pattern := regexp.MustCompile(`CHILD_PID=(\d+);`)
		for {
			n, e := p.Read(buffer)
			_, _ = transcript.Write(buffer[:n])
			output = append(output, buffer[:n]...)
			if match := pattern.FindSubmatch(output); match != nil {
				pid, _ := strconv.ParseUint(string(match[1]), 10, 32)
				pidReady <- uint32(pid)
				return
			}
			if e != nil {
				return
			}
		}
	}()
	var pid uint32
	select {
	case pid = <-pidReady:
	case <-time.After(15 * time.Second):
		t.Fatalf("child PID not reported; output=%q", transcript.snapshot())
	}
	child, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(child)
	closed := make(chan error, 1)
	go func() { closed <- p.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close deadlocked")
	}
	state, err := windows.WaitForSingleObject(child, 5000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatalf("child survived Job close: state=%d err=%v", state, err)
	}
}

func TestWindowsConPTYCloseDrainsUnreadOutput(t *testing.T) {
	process, err := Start(conPTYCommand(t, `$line = 'x' * 4096; for ($i = 0; $i -lt 256; $i++) { [Console]::WriteLine($line) }; Start-Sleep 30`), 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	p := process.(*windowsProcess)
	t.Cleanup(func() { _ = p.Close() })
	deadline := time.After(15 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for len(p.chunks) < cap(p.chunks) {
		select {
		case <-deadline:
			t.Fatal("test did not fill bounded output queue")
		case <-ticker.C:
		}
	}
	done := make(chan error, 1)
	go func() { done <- p.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close blocked on unread output")
	}
}
