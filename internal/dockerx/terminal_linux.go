//go:build linux

package dockerx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
	"golang.org/x/sys/unix"
)

type containerTerminalRecord struct {
	ExecID      string `json:"execId"`
	ContainerID string `json:"containerId"`
	Token       string `json:"token"`
	PID         int    `json:"pid,omitempty"`
	StartTime   string `json:"startTime,omitempty"`
	BootID      string `json:"bootId,omitempty"`
}

type containerTerminalState struct {
	ID          string `json:"ID"`
	ContainerID string `json:"ContainerID"`
	Running     bool   `json:"Running"`
	PID         int    `json:"Pid"`
	ExitCode    int    `json:"ExitCode"`
}

type containerTerminalControl struct {
	pidfd int
	tty   *os.File
}

type containerTerminalProcess struct {
	client  *Client
	record  containerTerminalRecord
	stream  io.ReadWriteCloser
	cancel  context.CancelFunc
	control *containerTerminalControl
	mu      sync.Mutex
	stopped bool
}

func containerTerminalCapabilities() error {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return ErrContainerTerminalUnsupported
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			caps, parseErr := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "CapEff:")), 16, 64)
			if parseErr == nil && caps&(1<<unix.CAP_KILL) != 0 && caps&(1<<unix.CAP_SYS_ADMIN) != 0 {
				fd, openErr := unix.PidfdOpen(os.Getpid(), 0)
				if openErr == nil {
					signalErr := unix.PidfdSendSignal(fd, 0, nil, 0)
					_ = unix.Close(fd)
					if signalErr == nil {
						return nil
					}
				}
			}
		}
	}
	return ErrContainerTerminalUnsupported
}

func (c *Client) startContainerTerminal(ctx context.Context, id string, rows, columns uint16) (terminal.Process, error) {
	ctx, cancelOpen := context.WithTimeout(ctx, 10*time.Second)
	defer cancelOpen()
	if err := containerTerminalCapabilities(); err != nil {
		return nil, err
	}
	directory, err := c.containerTerminalDirectory(true)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	// Also bound records retained across Agent restarts, before Manager has
	// admitted any new sessions. In-process failures remain owned by Manager.
	if len(entries) >= terminal.DefaultMaxSessions {
		return nil, terminal.ErrLimit
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, err
	}
	identity := hex.EncodeToString(token[:])
	payload, _ := json.Marshal(map[string]any{
		"AttachStdin": true, "AttachStdout": true, "AttachStderr": true,
		"Tty": true, "Cmd": []string{"/bin/sh", "-c", "if [ -x /bin/bash ]; then exec /bin/bash -i; else exec /bin/sh -i; fi"},
		"Env": []string{"TERM=xterm-256color", "COLORTERM=truecolor", "KPANEL_TERMINAL_ID=" + identity},
	})
	data, _, err := c.nginxDockerRequest(ctx, http.MethodPost, "/containers/"+id+"/exec", payload, 4<<10)
	if err != nil {
		return nil, err
	}
	var created struct {
		ID string `json:"Id"`
	}
	if json.Unmarshal(data, &created) != nil || !dockerExecIDPattern.MatchString(created.ID) {
		return nil, ErrInvalidDockerExec
	}
	record := containerTerminalRecord{ExecID: created.ID, ContainerID: id, Token: identity}
	// Persist BEFORE start: a killed Agent must not lose a started exec.
	if err := c.writeContainerTerminalRecord(record); err != nil {
		return nil, err
	}
	process := &containerTerminalProcess{client: c, record: record}
	stream, cancel, startErr := c.attachContainerTerminal(ctx, record.ExecID)
	process.stream, process.cancel = stream, cancel
	if startErr == nil {
		startErr = process.pin(ctx)
	}
	if startErr == nil {
		startErr = process.Resize(rows, columns)
	}
	if startErr != nil {
		// Retain the record on ambiguous start/cleanup failure for recovery.
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if process.control == nil {
			_ = process.pin(cleanupCtx)
		}
		if err := process.Kill(); err != nil {
			// Return ownership of an ambiguously started exec to Manager, so
			// it retains admission capacity and retries cleanup. Keep pidfds.
			if process.cancel != nil {
				process.cancel()
			}
			if process.stream != nil {
				_ = process.stream.Close()
			}
			return process, startErr
		}
		_ = process.Close()
		return nil, startErr
	}
	return process, nil
}

func (c *Client) attachContainerTerminal(ctx context.Context, execID string) (io.ReadWriteCloser, context.CancelFunc, error) {
	// The HTTP handler ends immediately after returning the session. Detach
	// its context only for the upgraded stream, keeping the handshake bounded.
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stopCancellation := context.AfterFunc(ctx, cancel)
	timer := time.AfterFunc(10*time.Second, cancel)
	defer stopCancellation()
	defer timer.Stop()
	req, err := http.NewRequestWithContext(streamCtx, http.MethodPost, c.baseURL+"/exec/"+execID+"/start", strings.NewReader(`{"Detach":false,"Tty":true}`))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")
	client := *c.httpClient
	client.Timeout = 0
	response, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	stream, ok := response.Body.(io.ReadWriteCloser)
	if response.StatusCode != http.StatusSwitchingProtocols || !ok || streamCtx.Err() != nil {
		_ = response.Body.Close()
		cancel()
		return nil, nil, errors.New("Docker terminal upgrade failed")
	}
	// net/http's upgraded body preserves bytes buffered after the headers.
	return stream, cancel, nil
}

func (c *Client) containerTerminalState(ctx context.Context, execID string) (containerTerminalState, error) {
	var state containerTerminalState
	err := c.getJSON(ctx, "/exec/"+execID+"/json", &state)
	if err == nil && (state.ID != execID || !dockerExecIDPattern.MatchString(state.ContainerID)) {
		err = ErrInvalidDockerExec
	}
	return state, err
}

func (p *containerTerminalProcess) pin(ctx context.Context) error {
	var state containerTerminalState
	for {
		var err error
		state, err = p.client.containerTerminalState(ctx, p.record.ExecID)
		if err != nil {
			return err
		}
		if state.ContainerID != p.record.ContainerID {
			return ErrInvalidDockerExec
		}
		if state.Running && state.PID > 0 {
			break
		}
		if state.ExitCode != 0 {
			return errContainerShellStart
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	fd, err := unix.PidfdOpen(state.PID, 0)
	if err != nil {
		return err
	}
	// A per-exec nonce prevents a stale/recycled daemon PID from granting
	// control of a different host process, even if it owns another PTY.
	if err := verifyContainerTerminalEnvironment(ctx, p.record.Token, func() ([]byte, error) {
		return os.ReadFile(fmt.Sprintf("/proc/%d/environ", state.PID))
	}); err != nil {
		_ = unix.Close(fd)
		return err
	}
	p.control = &containerTerminalControl{pidfd: fd}
	start, ttyDevice, err := containerTerminalProcIdentity(state.PID)
	if err != nil {
		return err
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return err
	}
	// Hold the actual slave inode, never reopen a pts number during close.
	ttyFD, err := unix.Open(fmt.Sprintf("/proc/%d/fd/0", state.PID), unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	tty := os.NewFile(uintptr(ttyFD), "container-terminal")
	var stat unix.Stat_t
	if err := unix.Fstat(ttyFD, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFCHR ||
		uint64(unix.Major(uint64(stat.Rdev))) != (ttyDevice>>8)&0xfff || uint64(unix.Minor(uint64(stat.Rdev))) != (ttyDevice&0xff)|((ttyDevice>>12)&0xfff00) {
		_ = tty.Close()
		return errors.New("Docker terminal device identity changed")
	}
	p.control.tty = tty
	after, _, err := containerTerminalProcIdentity(state.PID)
	verified, verifyErr := p.client.containerTerminalState(ctx, p.record.ExecID)
	if err != nil || verifyErr != nil || after != start || !verified.Running || verified.PID != state.PID || verified.ContainerID != p.record.ContainerID || p.control.exited() {
		return errors.New("Docker terminal process identity changed")
	}
	// Only persist birth metadata after both daemon inspections and the
	// original pidfd agree, so recovery cannot trust a recycled PID's birth.
	p.record.PID, p.record.StartTime, p.record.BootID = state.PID, start, strings.TrimSpace(string(boot))
	return p.client.writeContainerTerminalRecord(p.record)
}

func verifyContainerTerminalEnvironment(ctx context.Context, token string, readEnvironment func() ([]byte, error)) error {
	for {
		environment, err := readEnvironment()
		if err != nil {
			return errors.New("Docker terminal process identity could not be verified")
		}
		if strings.Contains("\x00"+string(environment), "\x00KPANEL_TERMINAL_ID="+token+"\x00") {
			return nil
		}
		// Docker may report a running exec before execve installs its
		// environment. Wait for the nonce within the caller's deadline;
		// an explicitly different nonce remains a hard failure.
		if strings.Contains("\x00"+string(environment), "\x00KPANEL_TERMINAL_ID=") {
			return errors.New("Docker terminal process identity could not be verified")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func containerTerminalProcIdentity(pid int) (string, uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", 0, err
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return "", 0, ErrInvalidDockerExec
	}
	fields := strings.Fields(string(data)[end+1:])
	if len(fields) < 20 {
		return "", 0, ErrInvalidDockerExec
	}
	device, parseErr := strconv.ParseInt(fields[4], 10, 64)
	if parseErr != nil {
		return "", 0, ErrInvalidDockerExec
	}
	return fields[19], uint64(uint32(device)), nil
}

func (c *containerTerminalControl) exited() bool {
	fds := []unix.PollFd{{Fd: int32(c.pidfd), Events: unix.POLLIN}}
	_, err := unix.Poll(fds, 0)
	return err == nil && fds[0].Revents&unix.POLLIN != 0
}

func (c *containerTerminalControl) terminate() error {
	if c.exited() {
		return nil
	}
	if c.tty != nil {
		// Disconnect this session's tty (including its foreground job). Merely
		// closing Docker's attach stream would leave the exec shell running.
		_ = unix.IoctlSetInt(int(c.tty.Fd()), unix.TIOCVHANGUP, 0)
	}
	if err := unix.PidfdSendSignal(c.pidfd, unix.SIGKILL, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
		return err
	}
	fds := []unix.PollFd{{Fd: int32(c.pidfd), Events: unix.POLLIN}}
	if _, err := unix.Poll(fds, 2000); err != nil {
		return err
	}
	if fds[0].Revents&unix.POLLIN == 0 {
		return errors.New("container terminal stop was not confirmed")
	}
	return nil
}

func (p *containerTerminalProcess) Read(data []byte) (int, error) { return p.stream.Read(data) }
func (p *containerTerminalProcess) Write(data []byte) (int, error) {
	// A blocked Engine must not hold the terminal Manager input gate forever.
	timer := time.AfterFunc(3*time.Second, func() { _ = p.stream.Close() })
	defer timer.Stop()
	return p.stream.Write(data)
}
func (p *containerTerminalProcess) Resize(rows, columns uint16) error {
	if rows == 0 || columns == 0 || rows > 500 || columns > 1000 {
		return ErrInvalidDockerExec
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := p.client.nginxDockerRequest(ctx, http.MethodPost, fmt.Sprintf("/exec/%s/resize?h=%d&w=%d", p.record.ExecID, rows, columns), nil, 4<<10)
	return err
}
func (p *containerTerminalProcess) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return nil
	}
	if p.control == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		state, err := p.client.containerTerminalState(ctx, p.record.ExecID)
		if err != nil {
			return err
		}
		if !state.Running {
			if err := p.client.removeContainerTerminalRecord(p.record.ExecID); err != nil {
				return err
			}
			p.stopped = true
			return nil
		}
		if err := p.pin(ctx); err != nil {
			return err
		}
	}
	if err := p.control.terminate(); err != nil {
		return err
	}
	if err := p.client.removeContainerTerminalRecord(p.record.ExecID); err != nil {
		return err
	}
	p.stopped = true
	return nil
}
func (p *containerTerminalProcess) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
	}
	if p.stream != nil {
		_ = p.stream.Close()
	}
	if p.control != nil {
		if p.control.tty != nil {
			_ = p.control.tty.Close()
			p.control.tty = nil
		}
		_ = unix.Close(p.control.pidfd)
		p.control = nil
	}
	return nil
}
func (p *containerTerminalProcess) Wait() error {
	// EOF can be a dropped transport while the exec is still alive. Stop it
	// through the same confirmed cleanup path instead of waiting indefinitely.
	if err := p.Kill(); err != nil {
		return fmt.Errorf("%w: %v", terminal.ErrCleanupPending, err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	state, err := p.client.containerTerminalState(ctx, p.record.ExecID)
	if err == nil && state.ExitCode != 0 {
		return fmt.Errorf("container shell exited with status %d", state.ExitCode)
	}
	return nil
}

func (c *Client) containerTerminalDirectory(create bool) (string, error) {
	directory := filepath.Join(c.stateRoot, "docker-terminal-recovery")
	if create {
		if err := os.MkdirAll(c.stateRoot, 0700); err != nil {
			return "", err
		}
		if err := os.Mkdir(directory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("container terminal recovery directory is not private")
	}
	return directory, nil
}
func (c *Client) writeContainerTerminalRecord(record containerTerminalRecord) error {
	directory, err := c.containerTerminalDirectory(true)
	if err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".pending-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filepath.Join(directory, record.ExecID+".json")); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (c *Client) removeContainerTerminalRecord(execID string) error {
	err := os.Remove(filepath.Join(c.stateRoot, "docker-terminal-recovery", execID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (c *Client) recoverContainerTerminals(ctx context.Context) error {
	directory, err := c.containerTerminalDirectory(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	if len(entries) > terminal.DefaultMaxSessions*2 {
		return errors.New("too many container terminal recovery records")
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A pending file is never used to start an exec: only a synced,
		// published .json record can precede start. Interrupted updates retain
		// the previous published record.
		if strings.HasPrefix(entry.Name(), ".pending-") && entry.Type().IsRegular() {
			if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
				return err
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			return errors.New("incomplete container terminal recovery record")
		}
		execID := strings.TrimSuffix(entry.Name(), ".json")
		if !dockerExecIDPattern.MatchString(execID) {
			return ErrInvalidDockerExec
		}
		fd, err := unix.Open(filepath.Join(directory, entry.Name()), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(fd), entry.Name())
		var record containerTerminalRecord
		err = json.NewDecoder(io.LimitReader(file, 4096)).Decode(&record)
		_ = file.Close()
		if err != nil || record.ExecID != execID || !dockerExecIDPattern.MatchString(record.ContainerID) || len(record.Token) != 32 {
			return ErrInvalidDockerExec
		}
		process := &containerTerminalProcess{client: c, record: record}
		if record.PID > 0 && record.BootID != "" && record.StartTime != "" {
			// PID reuse and a reboot must never turn recovery into an unrelated
			// host-process kill. The pidfd is pinned before inspecting /proc.
			pidfd, pinErr := unix.PidfdOpen(record.PID, 0)
			if pinErr == nil {
				process.control = &containerTerminalControl{pidfd: pidfd}
			}
			start, _, statErr := containerTerminalProcIdentity(record.PID)
			if strings.TrimSpace(string(boot)) != record.BootID || errors.Is(pinErr, unix.ESRCH) || errors.Is(statErr, os.ErrNotExist) || (process.control != nil && process.control.exited()) || (statErr == nil && start != record.StartTime) {
				_ = process.Close()
				if err := c.removeContainerTerminalRecord(execID); err != nil {
					return err
				}
				continue
			}
			if pinErr != nil {
				return pinErr
			}
			if statErr != nil {
				_ = process.Close()
				return statErr
			}
		} else {
			state, inspectErr := c.containerTerminalState(ctx, execID)
			if inspectErr != nil {
				return inspectErr
			}
			if !state.Running {
				if err := c.removeContainerTerminalRecord(execID); err != nil {
					return err
				}
				continue
			}
			if err := process.pin(ctx); err != nil {
				// Once the nonce has fixed a pidfd, an incomplete PTY setup
				// must not prevent terminating that already verified process.
				if process.control != nil && process.Kill() == nil {
					_ = process.Close()
					continue
				}
				_ = process.Close()
				return err
			}
		}
		stopErr := process.Kill()
		_ = process.Close()
		if stopErr != nil {
			return stopErr
		}
	}
	return nil
}
