//go:build linux

package dockerx

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
	"golang.org/x/sys/unix"
)

func TestContainerTerminalEnvironmentWaitsForExecStartup(t *testing.T) {
	for _, test := range []struct {
		name        string
		environment []byte
	}{
		{"empty", nil},
		{"pre-exec", []byte("TERM=xterm\x00")},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			calls := 0
			err := verifyContainerTerminalEnvironment(ctx, "owned-nonce", func() ([]byte, error) {
				calls++
				if calls == 1 {
					return test.environment, nil
				}
				return []byte("TERM=xterm\x00KPANEL_TERMINAL_ID=owned-nonce\x00"), nil
			})
			if err != nil || calls != 2 {
				t.Fatalf("exec startup verification = %v, reads = %d", err, calls)
			}
		})
	}
}

func TestContainerTerminalEnvironmentRejectsUnverifiedIdentity(t *testing.T) {
	for _, test := range []struct {
		name        string
		environment []byte
		readError   error
	}{
		{"wrong-nonce", []byte("KPANEL_TERMINAL_ID=other-nonce\x00"), nil},
		{"nonce-prefix", []byte("KPANEL_TERMINAL_ID=owned-nonce-suffix\x00"), nil},
		{"empty-nonce", []byte("KPANEL_TERMINAL_ID=\x00"), nil},
		{"read-failure", nil, os.ErrPermission},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			calls := 0
			err := verifyContainerTerminalEnvironment(ctx, "owned-nonce", func() ([]byte, error) {
				calls++
				return test.environment, test.readError
			})
			if err == nil || calls != 1 {
				t.Fatalf("unverified identity = %v, reads = %d", err, calls)
			}
		})
	}
}

func TestContainerTerminalEnvironmentHonorsOpenDeadline(t *testing.T) {
	for _, test := range []struct {
		name        string
		environment []byte
	}{
		{"empty", nil},
		{"missing-nonce", []byte("TERM=xterm\x00")},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			err := verifyContainerTerminalEnvironment(ctx, "owned-nonce", func() ([]byte, error) {
				return test.environment, nil
			})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("unready environment deadline = %v", err)
			}
		})
	}
}

func TestContainerTerminalUpgradePreservesBufferedBytesAndHandlerIndependence(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		if r.Header.Get("Upgrade") != "tcp" {
			t.Error("missing upgrade")
		}
		conn, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = buffered.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\nREADY")
		_ = buffered.Flush()
		_, _ = io.Copy(conn, buffered)
		close(done)
	}))
	defer server.Close()
	c := testHTTPClient(server)
	c.httpClient.Timeout = 20 * time.Millisecond
	ctx, cancelHandler := context.WithCancel(context.Background())
	stream, cancelStream, err := c.attachContainerTerminal(ctx, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	defer cancelStream()
	cancelHandler()
	time.Sleep(40 * time.Millisecond)
	reader := bufio.NewReader(stream)
	data := make([]byte, 5)
	if _, err := io.ReadFull(reader, data); err != nil || string(data) != "READY" {
		t.Fatalf("buffered data = %q, %v", data, err)
	}
	if _, err := stream.Write([]byte("echo")); err != nil {
		t.Fatal(err)
	}
	data = make([]byte, 4)
	if _, err := io.ReadFull(reader, data); err != nil || string(data) != "echo" {
		t.Fatalf("interactive stream = %q, %v", data, err)
	}
	_ = stream.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not close")
	}
}

func TestContainerTerminalRecoveryRetriesFailuresAndNeverKillsReusedIdentity(t *testing.T) {
	c := New("/unused", "/home/web", t.TempDir())
	directory, err := c.containerTerminalDirectory(true)
	if err != nil {
		t.Fatal(err)
	}
	invalid := filepath.Join(directory, "invalid.json")
	if err := os.WriteFile(invalid, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.RecoverContainerTerminals(context.Background()); err == nil {
		t.Fatal("invalid record accepted")
	}
	if err := os.Remove(invalid); err != nil {
		t.Fatal(err)
	}
	child := exec.Command("/bin/sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Fatal(err)
	}
	record := containerTerminalRecord{ExecID: strings.Repeat("a", 64), ContainerID: strings.Repeat("b", 64), Token: strings.Repeat("c", 32), PID: child.Process.Pid, StartTime: "wrong-birth", BootID: strings.TrimSpace(string(boot))}
	if err := c.writeContainerTerminalRecord(record); err != nil {
		t.Fatal(err)
	}
	if err := c.RecoverContainerTerminals(context.Background()); err != nil {
		t.Fatalf("transient failure was cached: %v", err)
	}
	if err := child.Process.Signal(unix.Signal(0)); err != nil {
		t.Fatalf("recovery killed a different process: %v", err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 0 {
		t.Fatalf("stale records retained: %v", entries)
	}
}

func TestContainerTerminalRecoveryRejectsSymlink(t *testing.T) {
	c := New("/unused", "/home/web", t.TempDir())
	directory, err := c.containerTerminalDirectory(true)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, strings.Repeat("a", 64)+".json")); err != nil {
		t.Fatal(err)
	}
	if err := c.RecoverContainerTerminals(context.Background()); err == nil {
		t.Fatal("symlink record accepted")
	}
	data, _ := os.ReadFile(outside)
	if string(data) != "unchanged" {
		t.Fatal("recovery modified symlink target")
	}
}

func TestContainerTerminalRecoveryStopsVerifiedProcessAfterPartialPinFailure(t *testing.T) {
	token := strings.Repeat("d", 32)
	child := exec.Command("/bin/sleep", "30")
	child.Env = append(os.Environ(), "KPANEL_TERMINAL_ID="+token)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	execID, containerID := strings.Repeat("a", 64), strings.Repeat("b", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/exec/"+execID+"/json" {
			t.Errorf("unexpected recovery request: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", 500)
			return
		}
		_ = json.NewEncoder(w).Encode(containerTerminalState{ID: execID, ContainerID: containerID, Running: true, PID: child.Process.Pid})
	}))
	defer server.Close()
	c := testHTTPClient(server)
	c.stateRoot = t.TempDir()
	record := containerTerminalRecord{ExecID: execID, ContainerID: containerID, Token: token}
	if err := c.writeContainerTerminalRecord(record); err != nil {
		t.Fatal(err)
	}
	// The child has the verified nonce but fd0 is /dev/null, so pin stops
	// after acquiring its pidfd. Recovery must still use that trusted handle.
	if err := c.RecoverContainerTerminals(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("verified child was not terminated")
	}
	if _, err := os.Stat(filepath.Join(c.stateRoot, "docker-terminal-recovery", execID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup retained record: %v", err)
	}
}

// Opt-in integration tests only create and remove their own disposable
// containers. Ordinary repository tests never contact a Docker daemon.
func TestContainerTerminalDockerIntegration(t *testing.T) {
	if os.Getenv("KPANEL_DOCKER_TERMINAL_TEST") != "1" {
		t.Skip("set KPANEL_DOCKER_TERMINAL_TEST=1 for disposable local Docker tests")
	}
	if err := containerTerminalCapabilities(); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"", "1000:1000"} {
		t.Run("user="+user, func(t *testing.T) {
			c, id, version := containerTerminalFixture(t, user)
			m := terminal.New(terminal.Config{})
			defer m.CloseAll()
			var process *containerTerminalProcess
			session, err := m.OpenWithStarter("integration-owner", 30, 120, func(rows, cols uint16) (terminal.Process, error) {
				opened, err := c.OpenContainerTerminal(context.Background(), id, version, rows, cols)
				if opened != nil {
					process = opened.(*containerTerminalProcess)
				}
				return opened, err
			})
			if err != nil {
				t.Fatal(err)
			}
			offset := int64(0)
			run := func(command, expected string) {
				t.Helper()
				if err := m.Input("integration-owner", session.ID, []byte(command)); err != nil {
					t.Fatal(err)
				}
				containerTerminalAwait(t, m, session.ID, &offset, expected)
			}
			run("cd /tmp\r", "")
			run("pwd\r", "\r\n/tmp\r\n")
			run("printf 'COMPLETION_OK\\n' > /tmp/kpanel-terminal-completion-target\r", "")
			run("cat /tmp/kpanel-terminal-completion-tar\t\r", "\r\nCOMPLETION_OK\r\n")
			run("sleep 30\r", "sleep 30")
			containerTerminalAwaitChild(t, process, true)
			if err := m.Input("integration-owner", session.ID, []byte("\x03")); err != nil {
				t.Fatal(err)
			}
			containerTerminalAwaitChild(t, process, false)
			run("printf 'INTERRUPT_OK\\n'\r", "\r\nINTERRUPT_OK\r\n")
			if err := m.Resize("integration-owner", session.ID, 42, 99); err != nil {
				t.Fatal(err)
			}
			run("stty size\r", "\r\n42 99\r\n")
			// Ignoring HUP still cannot keep the owned shell alive after close.
			run("trap '' HUP; printf 'CLOSE_READY\\n'\r", "\r\nCLOSE_READY\r\n")
			if err := m.Close("integration-owner", session.ID); err != nil {
				t.Fatal(err)
			}
			containerTerminalAssertStopped(t, c, process.record.ExecID, id)
			process.mu.Lock()
			released := process.control == nil
			process.mu.Unlock()
			if !released {
				t.Fatal("close leaked pinned process handles")
			}
		})
	}
	t.Run("disconnect-and-natural-exit", func(t *testing.T) {
		c, id, version := containerTerminalFixture(t, "")
		for _, disconnect := range []bool{true, false} {
			opened, err := c.OpenContainerTerminal(context.Background(), id, version, 24, 80)
			if err != nil {
				t.Fatal(err)
			}
			p := opened.(*containerTerminalProcess)
			if disconnect {
				_ = p.stream.Close()
			} else {
				_, _ = p.Write([]byte("exit\r"))
				_, _ = io.Copy(io.Discard, p)
			}
			_ = p.Wait()
			containerTerminalAssertStopped(t, c, p.record.ExecID, id)
			p.mu.Lock()
			released := p.control == nil
			p.mu.Unlock()
			if !released {
				t.Fatal("Wait leaked pinned process handles")
			}
		}
	})
	t.Run("agent-process-exit-recovery", func(t *testing.T) {
		c, id, version := containerTerminalFixture(t, "")
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		command := exec.Command(executable, "-test.run=^TestContainerTerminalCrashHelper$")
		command.Env = append(os.Environ(), "KPANEL_TERMINAL_CRASH_HELPER=1", "KPANEL_TERMINAL_CONTAINER="+id, "KPANEL_TERMINAL_VERSION="+version, "KPANEL_TERMINAL_STATE="+c.stateRoot)
		output, err := command.Output()
		if err != nil {
			t.Fatalf("crash helper: %v", err)
		}
		execID := strings.TrimSpace(string(output))
		if !dockerExecIDPattern.MatchString(execID) {
			t.Fatalf("helper output = %q", output)
		}
		// A new Client represents the next Agent process. It reads only its
		// private records, including when the daemon is temporarily offline.
		restarted := New("/var/run/docker.sock", "/home/web", c.stateRoot)
		restarted.ConfigureDaemonAccess("/run/docker.pid", true)
		if err := restarted.RecoverContainerTerminals(context.Background()); err != nil {
			t.Fatal(err)
		}
		containerTerminalAssertStopped(t, restarted, execID, id)
	})
}

func TestContainerTerminalCrashHelper(t *testing.T) {
	if os.Getenv("KPANEL_TERMINAL_CRASH_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	c := New("/var/run/docker.sock", "/home/web", os.Getenv("KPANEL_TERMINAL_STATE"))
	c.ConfigureDaemonAccess("/run/docker.pid", true)
	opened, err := c.OpenContainerTerminal(context.Background(), os.Getenv("KPANEL_TERMINAL_CONTAINER"), os.Getenv("KPANEL_TERMINAL_VERSION"), 24, 80)
	if err != nil {
		t.Fatal(err)
	}
	p := opened.(*containerTerminalProcess)
	_, _ = p.Write([]byte("trap '' HUP; exec env -i /bin/sh -i\r"))
	// Wait for the exec to discard the nonce before crashing. Recovery must
	// rely on the already verified process birth, not mutable environment.
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, _ := os.ReadFile(fmt.Sprintf("/proc/%d/environ", p.record.PID))
		if !strings.Contains(string(data), "KPANEL_TERMINAL_ID=") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not exec env -i")
		}
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Print(p.record.ExecID)
	os.Exit(0) // Deliberately bypass all Go cleanup, like an Agent crash.
}

func containerTerminalFixture(t *testing.T, user string) (*Client, string, string) {
	t.Helper()
	c := New("/var/run/docker.sock", "/home/web", t.TempDir())
	c.ConfigureDaemonAccess("/run/docker.pid", true)
	payload, _ := json.Marshal(map[string]any{"Image": "alpine:3.23", "Cmd": []string{"sleep", "600"}, "User": user, "Labels": map[string]string{"io.kpanel.test": "docker-interactive-terminal"}})
	data, _, err := c.nginxDockerRequest(context.Background(), http.MethodPost, "/containers/create", payload, 4096)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(data, &created); err != nil || !dockerExecIDPattern.MatchString(created.ID) {
		t.Fatalf("create fixture: %s %v", data, err)
	}
	t.Cleanup(func() {
		_, _, err := c.nginxDockerRequest(context.Background(), http.MethodDelete, "/containers/"+created.ID+"?force=1&v=0", nil, 4096)
		if err != nil {
			t.Errorf("remove owned fixture %s: %v", created.ID, err)
		}
	})
	if _, _, err := c.nginxDockerRequest(context.Background(), http.MethodPost, "/containers/"+created.ID+"/start", nil, 4096); err != nil {
		t.Fatal(err)
	}
	inspect, err := c.inspect(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c, created.ID, c.summaryFromInspect(inspect).ResourceVersion
}

func containerTerminalAwait(t *testing.T, m *terminal.Manager, id string, offset *int64, expected string) {
	t.Helper()
	if expected == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var received strings.Builder
	for !strings.Contains(received.String(), expected) {
		out, err := m.Output(ctx, "integration-owner", id, *offset, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("waiting for %q: %v; received %q", expected, err, received.String())
		}
		*offset = out.NextOffset
		received.Write(out.Data)
		if out.ExitedAt != nil || out.Closed {
			t.Fatalf("terminal exited waiting for %q: %q (%s)", expected, received.String(), out.ExitError)
		}
	}
}

func containerTerminalAssertStopped(t *testing.T, c *Client, execID, containerID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		state, err := c.containerTerminalState(context.Background(), execID)
		if err != nil {
			t.Fatal(err)
		}
		if !state.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned exec survived cleanup")
		}
		time.Sleep(20 * time.Millisecond)
	}
	inspect, err := c.inspect(context.Background(), containerID)
	if err != nil || !inspect.State.Running {
		t.Fatalf("terminal cleanup stopped container: %v", err)
	}
	directory, err := c.containerTerminalDirectory(false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, execID+".json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup retained recovery record: %v", err)
	}
}

func containerTerminalAwaitChild(t *testing.T, p *containerTerminalProcess, running bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/task/%d/children", p.record.PID, p.record.PID))
		if err != nil {
			t.Fatal(err)
		}
		if (strings.TrimSpace(string(data)) != "") == running {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("foreground child running=%t did not become %t", !running, running)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
