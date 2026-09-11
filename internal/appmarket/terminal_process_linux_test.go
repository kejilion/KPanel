//go:build linux

package appmarket

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestFourLinuxTerminalsRemainIsolatedWhenOneIsKilled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	processes := make([]terminalProcess, 4)
	outputs := make([]chan string, 4)
	for index := range processes {
		command := exec.CommandContext(ctx, "/bin/bash", "-c", "IFS= read -r value; printf 'reply:%s\\n' \"$value\"; IFS= read -r done")
		process, err := startTerminalProcess(command, 24, 80)
		if err != nil {
			t.Fatal(err)
		}
		processes[index] = process
		t.Cleanup(func() { _ = process.Kill(); _ = process.Close() })
		outputs[index] = make(chan string, 1)
		go func() {
			data, _ := io.ReadAll(process)
			_ = process.Wait()
			outputs[index] <- string(data)
		}()
	}
	if err := processes[0].Kill(); err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(processes); index++ {
		if _, err := processes[index].Write([]byte(fmt.Sprintf("app-%d\nfinish\n", index))); err != nil {
			t.Fatal(err)
		}
	}
	for index, output := range outputs {
		select {
		case data := <-output:
			if index > 0 && !strings.Contains(data, fmt.Sprintf("reply:app-%d", index)) {
				t.Fatalf("terminal %d lost its response: %q", index, data)
			}
			for other := 1; other < len(processes); other++ {
				if other != index && strings.Contains(data, fmt.Sprintf("app-%d", other)) {
					t.Fatalf("terminal %d leaked terminal %d output", index, other)
				}
			}
		case <-ctx.Done():
			t.Fatal("terminal did not exit before timeout")
		}
	}
}

func TestLinuxTerminalProcessSupportsInteractiveRead(t *testing.T) {
	command := exec.Command(
		"/bin/bash",
		"-c",
		"printf 'name: '; IFS= read -r value; printf '\\r\\nhello %s\\r\\n' \"$value\"",
	)
	process, err := startTerminalProcess(command, 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Close()
	if _, err := process.Write([]byte("kpanel\n")); err != nil {
		t.Fatal(err)
	}
	output, readErr := io.ReadAll(process)
	if readErr != nil && !isTerminalEnd(readErr) {
		t.Fatal(readErr)
	}
	if err := process.Wait(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "hello kpanel") {
		t.Fatalf("terminal output = %q", output)
	}
}

func TestLinuxTerminalInputFIFOTransfersBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal.input")
	if err := createTerminalInput(path); err != nil {
		t.Fatal(err)
	}
	defer removeTerminalInput(path)
	reader, err := openTerminalInput(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	want := []byte(strings.Repeat("输入🙂", 180))
	if err := writeTerminalInput(path, want); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(reader, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("FIFO input = %q, want %q", got, want)
	}
}

func TestLinuxTerminalInputWaitsForBriefBackpressure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terminal.input")
	if err := createTerminalInput(path); err != nil {
		t.Fatal(err)
	}
	defer removeTerminalInput(path)
	reader, err := openTerminalInput(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	capacity, err := unix.FcntlInt(reader.Fd(), unix.F_GETPIPE_SZ, 0)
	if err != nil {
		t.Fatal(err)
	}
	fill := bytes.Repeat([]byte{'x'}, capacity)
	if written, err := reader.Write(fill); err != nil || written != len(fill) {
		t.Fatalf("fill FIFO: written=%d err=%v", written, err)
	}

	want := []byte("input-after-backpressure\r")
	result := make(chan error, 1)
	go func() {
		result <- writeTerminalInput(path, want)
	}()
	select {
	case err := <-result:
		t.Fatalf("FIFO write did not wait for available space: %v", err)
	case <-time.After(40 * time.Millisecond):
	}

	released := min(capacity, 4096)
	if _, err := io.ReadFull(reader, make([]byte, released)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO write did not resume after space became available")
	}

	tail := make([]byte, capacity-released+len(want))
	if _, err := io.ReadFull(reader, tail); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(tail, want) {
		t.Fatalf("FIFO tail does not contain the complete terminal input")
	}
}
