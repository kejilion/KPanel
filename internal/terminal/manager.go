package terminal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/hostpty"
)

const (
	DefaultBufferBytes      = 1 << 20
	DefaultMaxSessions      = 16
	DefaultMaxOwnerSessions = 4
	DefaultIdleTimeout      = 30 * time.Minute
	DefaultLifetime         = 8 * time.Hour
	MaxInputBytes           = 16 << 10
	MaxOutputBytes          = 64 << 10
)

var (
	ErrNotFound = errors.New("terminal session not found")
	ErrLimit    = errors.New("terminal session limit reached")
	ErrClosed   = errors.New("terminal session is closed")
	ErrOffset   = errors.New("terminal output offset is invalid")
)

type Process interface {
	io.ReadWriteCloser
	Wait() error
	Kill() error
	Resize(rows, columns uint16) error
}

type Starter func(rows, columns uint16) (Process, error)

type Config struct {
	Starter          Starter
	ParentUnit       string
	BufferBytes      int
	MaxSessions      int
	MaxOwnerSessions int
	IdleTimeout      time.Duration
	Lifetime         time.Duration
	Now              func() time.Time
}

type Manager struct {
	mu       sync.RWMutex
	config   Config
	sessions map[string]*session
	stop     chan struct{}
	stopOnce sync.Once
	closed   bool
}

type session struct {
	mu        sync.Mutex
	id        string
	owner     string
	process   Process
	buffer    []byte
	base      int64
	next      int64
	notify    chan struct{}
	createdAt time.Time
	updatedAt time.Time
	exitedAt  *time.Time
	exitError string
	closed    bool
}

type Snapshot struct {
	ID        string     `json:"id"`
	Offset    int64      `json:"offset"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	ExitedAt  *time.Time `json:"exitedAt,omitempty"`
	ExitError string     `json:"exitError,omitempty"`
	Closed    bool       `json:"closed"`
}

type Output struct {
	Data       []byte     `json:"data"`
	Offset     int64      `json:"offset"`
	NextOffset int64      `json:"nextOffset"`
	Truncated  bool       `json:"truncated"`
	ExitedAt   *time.Time `json:"exitedAt,omitempty"`
	ExitError  string     `json:"exitError,omitempty"`
	Closed     bool       `json:"closed"`
}

func New(config Config) *Manager {
	if config.Starter == nil {
		parentUnit := config.ParentUnit
		config.Starter = func(rows, columns uint16) (Process, error) {
			return starterWithParent(rows, columns, parentUnit)
		}
	}
	if config.BufferBytes <= 0 {
		config.BufferBytes = DefaultBufferBytes
	}
	if config.MaxSessions <= 0 {
		config.MaxSessions = DefaultMaxSessions
	}
	if config.MaxOwnerSessions <= 0 {
		config.MaxOwnerSessions = DefaultMaxOwnerSessions
	}
	if config.IdleTimeout <= 0 {
		config.IdleTimeout = DefaultIdleTimeout
	}
	if config.Lifetime <= 0 {
		config.Lifetime = DefaultLifetime
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	manager := &Manager{config: config, sessions: make(map[string]*session), stop: make(chan struct{})}
	go manager.reapLoop()
	return manager
}

func defaultStarter(rows, columns uint16) (Process, error) {
	return starterWithParent(rows, columns, "kejilion-agent.service")
}

func starterWithParent(rows, columns uint16, parentUnit string) (Process, error) {
	shell := "/bin/bash"
	if _, err := os.Stat(shell); err != nil {
		shell = "/bin/sh"
	}
	directory := "/"
	if root, err := os.Open("/root"); err == nil {
		if info, statErr := root.Stat(); statErr == nil && info.IsDir() {
			directory = "/root"
		}
		_ = root.Close()
	}
	environment := terminalEnvironment(shell)
	if os.Getenv("INVOCATION_ID") != "" {
		systemdRun, runErr := trustedTerminalExecutable("systemd-run")
		systemctl, controlErr := trustedTerminalExecutable("systemctl")
		if runErr == nil && controlErr == nil {
			identity, err := randomID()
			if err != nil {
				return nil, err
			}
			unit := "kpanel-terminal-" + identity
			command := transientTerminalCommandWithParent(systemdRun, shell, directory, unit, environment, parentUnit)
			process, err := hostpty.Start(command, rows, columns)
			if err != nil {
				return nil, err
			}
			return &transientTerminalProcess{Process: process, systemctl: systemctl, unit: unit}, nil
		}
	}
	command := exec.Command(shell, "-l")
	command.Dir = directory
	command.Env = environment
	return hostpty.Start(command, rows, columns)
}

func terminalEnvironment(shell string) []string {
	return []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"HOME=/root",
		"USER=root",
		"LOGNAME=root",
		"SHELL=" + shell,
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8",
	}
}

func transientTerminalCommand(systemdRun, shell, directory, unit string, environment []string) *exec.Cmd {
	return transientTerminalCommandWithParent(systemdRun, shell, directory, unit, environment, "kejilion-agent.service")
}

func transientTerminalCommandWithParent(systemdRun, shell, directory, unit string, environment []string, parentUnit string) *exec.Cmd {
	arguments := []string{
		"--quiet",
		"--wait",
		"--collect",
		"--pty",
		"--unit=" + unit,
		"--property=Type=exec",
		"--property=User=root",
		"--property=Group=root",
		"--property=WorkingDirectory=" + directory,
		"--property=UMask=0077",
		"--property=NoNewPrivileges=no",
		"--property=ProtectSystem=no",
		"--property=ProtectHome=no",
		"--property=KillMode=mixed",
		"--property=SendSIGHUP=yes",
		"--property=RuntimeMaxSec=8h",
	}
	if strings.TrimSpace(parentUnit) != "" {
		arguments = append(arguments,
			"--property=PartOf="+strings.TrimSpace(parentUnit),
			"--property=After="+strings.TrimSpace(parentUnit),
		)
	}
	for _, value := range environment {
		arguments = append(arguments, "--setenv="+value)
	}
	arguments = append(arguments, "--", shell, "-l")
	command := exec.Command(systemdRun, arguments...)
	command.Dir = directory
	command.Env = environment
	return command
}

func trustedTerminalExecutable(name string) (string, error) {
	for _, directory := range []string{"/usr/bin", "/bin"} {
		candidate := directory + "/" + name
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o022 == 0 && terminalExecutableOwnerTrusted(info) {
			return candidate, nil
		}
	}
	return "", errors.New("trusted " + name + " was not found")
}

type transientTerminalProcess struct {
	Process
	systemctl string
	unit      string
	stopOnce  sync.Once
}

func (process *transientTerminalProcess) stopUnit() {
	process.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = exec.CommandContext(ctx, process.systemctl, "stop", process.unit).Run()
	})
}

func (process *transientTerminalProcess) Close() error {
	err := process.Process.Close()
	process.stopUnit()
	return err
}

func (process *transientTerminalProcess) Kill() error {
	process.stopUnit()
	return process.Process.Kill()
}

func (m *Manager) Open(owner string, rows, columns uint16) (Snapshot, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" || rows == 0 || columns == 0 || rows > 500 || columns > 1000 {
		return Snapshot{}, errors.New("invalid terminal session request")
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Snapshot{}, ErrClosed
	}
	ownerCount := 0
	activeCount := 0
	for _, item := range m.sessions {
		if item.isActive() {
			activeCount++
		}
		if item.owner == owner && item.isActive() {
			ownerCount++
		}
	}
	if activeCount >= m.config.MaxSessions || ownerCount >= m.config.MaxOwnerSessions {
		m.mu.Unlock()
		return Snapshot{}, ErrLimit
	}
	process, err := m.config.Starter(rows, columns)
	if err != nil {
		m.mu.Unlock()
		return Snapshot{}, err
	}
	id, err := randomID()
	if err != nil {
		_ = process.Close()
		_ = process.Kill()
		m.mu.Unlock()
		return Snapshot{}, err
	}
	now := m.config.Now().UTC()
	item := &session{id: id, owner: owner, process: process, notify: make(chan struct{}), createdAt: now, updatedAt: now}
	m.sessions[id] = item
	m.mu.Unlock()
	go m.capture(item)
	return item.snapshot(), nil
}

func (m *Manager) capture(item *session) {
	buffer := make([]byte, 32<<10)
	for {
		read, err := item.process.Read(buffer)
		if read > 0 {
			item.append(buffer[:read], m.config.BufferBytes, m.config.Now().UTC())
		}
		if err != nil {
			if !hostpty.IsEnd(err) && !errors.Is(err, os.ErrClosed) {
				item.setExit(err, m.config.Now().UTC())
			}
			break
		}
	}
	err := item.process.Wait()
	item.setExit(err, m.config.Now().UTC())
}

func (m *Manager) Output(ctx context.Context, owner, id string, offset int64, wait time.Duration) (Output, error) {
	item, err := m.lookup(owner, id)
	if err != nil {
		return Output{}, err
	}
	if wait < 0 || wait > 1500*time.Millisecond {
		return Output{}, errors.New("invalid terminal wait duration")
	}
	for {
		output, notify, ready, err := item.output(offset, MaxOutputBytes, m.config.Now().UTC())
		if err != nil || ready || wait == 0 {
			return output, err
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return Output{}, ctx.Err()
		case <-notify:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			return item.currentOutput(offset, MaxOutputBytes, m.config.Now().UTC())
		}
	}
}

func (m *Manager) Input(owner, id string, data []byte) error {
	if len(data) == 0 || len(data) > MaxInputBytes {
		return errors.New("invalid terminal input size")
	}
	item, err := m.lookup(owner, id)
	if err != nil {
		return err
	}
	item.mu.Lock()
	defer item.mu.Unlock()
	if item.closed || item.exitedAt != nil {
		return ErrClosed
	}
	_, err = item.process.Write(data)
	if err == nil {
		item.updatedAt = m.config.Now().UTC()
	}
	return err
}

func (m *Manager) Resize(owner, id string, rows, columns uint16) error {
	if rows == 0 || columns == 0 || rows > 500 || columns > 1000 {
		return errors.New("invalid terminal dimensions")
	}
	item, err := m.lookup(owner, id)
	if err != nil {
		return err
	}
	item.mu.Lock()
	defer item.mu.Unlock()
	if item.closed || item.exitedAt != nil {
		return ErrClosed
	}
	if err := item.process.Resize(rows, columns); err != nil {
		return err
	}
	item.updatedAt = m.config.Now().UTC()
	return nil
}

func (m *Manager) Close(owner, id string) error {
	item, err := m.lookup(owner, id)
	if err != nil {
		return err
	}
	item.mu.Lock()
	if item.closed {
		item.mu.Unlock()
		return nil
	}
	item.closed = true
	item.updatedAt = m.config.Now().UTC()
	close(item.notify)
	item.notify = make(chan struct{})
	item.mu.Unlock()
	_ = item.process.Close()
	if err := item.process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	m.closed = true
	items := make([]*session, 0, len(m.sessions))
	for _, item := range m.sessions {
		items = append(items, item)
	}
	m.mu.Unlock()
	m.stopOnce.Do(func() { close(m.stop) })
	for _, item := range items {
		_ = m.Close(item.owner, item.id)
	}
}

func (m *Manager) reapLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.reap(m.config.Now().UTC())
		case <-m.stop:
			return
		}
	}
}

func (m *Manager) reap(now time.Time) {
	type staleSession struct{ owner, id string }
	stale := make([]staleSession, 0)
	m.mu.Lock()
	for id, item := range m.sessions {
		item.mu.Lock()
		inactive := now.Sub(item.updatedAt) >= m.config.IdleTimeout || now.Sub(item.createdAt) >= m.config.Lifetime
		finished := item.closed || item.exitedAt != nil
		if finished && now.Sub(item.updatedAt) >= 5*time.Minute {
			delete(m.sessions, id)
		} else if inactive && !finished {
			stale = append(stale, staleSession{owner: item.owner, id: id})
		}
		item.mu.Unlock()
	}
	m.mu.Unlock()
	for _, item := range stale {
		_ = m.Close(item.owner, item.id)
	}
}

func (m *Manager) lookup(owner, id string) (*session, error) {
	m.mu.RLock()
	item := m.sessions[id]
	m.mu.RUnlock()
	if item == nil || item.owner != strings.TrimSpace(owner) {
		return nil, ErrNotFound
	}
	return item, nil
}

func (item *session) isActive() bool {
	item.mu.Lock()
	defer item.mu.Unlock()
	return !item.closed && item.exitedAt == nil
}

func (item *session) snapshot() Snapshot {
	item.mu.Lock()
	defer item.mu.Unlock()
	return Snapshot{ID: item.id, Offset: item.next, CreatedAt: item.createdAt, UpdatedAt: item.updatedAt, ExitedAt: item.exitedAt, ExitError: item.exitError, Closed: item.closed}
}

func (item *session) append(data []byte, limit int, now time.Time) {
	item.mu.Lock()
	defer item.mu.Unlock()
	item.buffer = append(item.buffer, data...)
	item.next += int64(len(data))
	if len(item.buffer) > limit {
		drop := len(item.buffer) - limit
		copy(item.buffer, item.buffer[drop:])
		item.buffer = item.buffer[:limit]
		item.base += int64(drop)
	}
	item.updatedAt = now
	close(item.notify)
	item.notify = make(chan struct{})
}

func (item *session) setExit(err error, now time.Time) {
	item.mu.Lock()
	defer item.mu.Unlock()
	if item.exitedAt != nil {
		return
	}
	item.exitedAt = &now
	item.updatedAt = now
	if err != nil {
		item.exitError = err.Error()
	}
	close(item.notify)
	item.notify = make(chan struct{})
}

func (item *session) output(offset int64, limit int, now time.Time) (Output, <-chan struct{}, bool, error) {
	item.mu.Lock()
	defer item.mu.Unlock()
	if offset < 0 || offset > item.next {
		return Output{}, nil, false, ErrOffset
	}
	truncated := offset < item.base
	if truncated {
		offset = item.base
	}
	start := int(offset - item.base)
	available := len(item.buffer) - start
	if available > limit {
		available = limit
	}
	data := append([]byte(nil), item.buffer[start:start+available]...)
	item.updatedAt = now
	result := Output{Data: data, Offset: offset, NextOffset: offset + int64(len(data)), Truncated: truncated, ExitedAt: item.exitedAt, ExitError: item.exitError, Closed: item.closed}
	return result, item.notify, len(data) > 0 || item.closed || item.exitedAt != nil, nil
}

func (item *session) currentOutput(offset int64, limit int, now time.Time) (Output, error) {
	output, _, _, err := item.output(offset, limit, now)
	return output, err
}

func randomID() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
