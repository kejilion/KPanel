package webbrowser

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// visualDisplay owns a private, view-only TigerVNC framebuffer for one browser
// workspace. It never exposes a TCP listener or accepts VNC keyboard, pointer,
// clipboard, or resize commands from clients.
type visualDisplay struct {
	dir, auth, socket, display string
	cmd                        *exec.Cmd
	done                       chan struct{}
	wm                         *exec.Cmd
	wmDone                     chan struct{}
	width, height              int64
	mode                       string
	once                       sync.Once
}

var displayNumber = regexp.MustCompile(`^[0-9]{1,4}$`)

func startVisualDisplay(ctx context.Context, width, height int64) (*visualDisplay, error) {
	if width < 320 || width > 1600 || height < 240 || height > 1000 {
		return nil, errors.New("invalid_viewport")
	}
	dir, err := os.MkdirTemp("", "kpanel-vnc-")
	if err != nil {
		return nil, errors.New("browser_display_failed")
	}
	d := &visualDisplay{dir: dir, auth: filepath.Join(dir, "xauth"), socket: filepath.Join(dir, "vnc.sock"), width: width, height: height}
	defer func() {
		if err != nil {
			d.Close()
		}
	}()
	if err = os.WriteFile(d.auth, nil, 0600); err != nil {
		return nil, errors.New("browser_display_failed")
	}
	read, write, err := os.Pipe()
	if err != nil {
		return nil, errors.New("browser_display_failed")
	}
	defer read.Close()
	// Input comes from the local pointer, so the display server's stationary
	// pointer must not be included in the remote framebuffer.
	d.cmd = exec.CommandContext(ctx, "Xvnc", "-displayfd", "3", "-geometry", fmt.Sprintf("%dx%d", width, height), "-depth", "24", "-nocursor", "-auth", d.auth,
		"-rfbunixpath", d.socket, "-rfbunixmode", "0600", "-rfbport", "-1", "-SecurityTypes", "None", "-NeverShared",
		"-AcceptKeyEvents=0", "-AcceptPointerEvents=0", "-AcceptCutText=0", "-SendCutText=0", "-AcceptSetDesktopSize=0", "-AllowOverride=", "-nolisten", "tcp")
	d.cmd.ExtraFiles = []*os.File{write}
	d.cmd.Stdout, d.cmd.Stderr = io.Discard, io.Discard
	if err = d.cmd.Start(); err != nil {
		write.Close()
		return nil, errors.New("browser_display_failed")
	}
	write.Close()
	d.done = make(chan struct{})
	go func() { _ = d.cmd.Wait(); close(d.done) }()
	numbers := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(io.LimitReader(read, 16)).ReadString('\n')
		numbers <- strings.TrimSpace(line)
	}()
	select {
	case number := <-numbers:
		if !displayNumber.MatchString(number) {
			err = errors.New("browser_display_failed")
			return nil, err
		}
		d.display = ":" + number
	case <-d.done:
		err = errors.New("browser_display_failed")
		return nil, err
	case <-ctx.Done():
		err = ctx.Err()
		return nil, errors.New("browser_display_failed")
	case <-time.After(8 * time.Second):
		err = errors.New("browser_display_failed")
		return nil, err
	}
	var cookie [16]byte
	if _, err = rand.Read(cookie[:]); err != nil {
		return nil, errors.New("browser_display_failed")
	}
	authCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = exec.CommandContext(authCtx, "xauth", "-f", d.auth, "add", d.display, "MIT-MAGIC-COOKIE-1", hex.EncodeToString(cookie[:])).Run()
	cancel()
	if err != nil {
		return nil, errors.New("browser_display_failed")
	}
	info, statErr := os.Lstat(d.socket)
	if statErr != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm()&0077 != 0 {
		err = errors.New("browser_display_failed")
		return nil, err
	}
	// Kiosk fullscreen needs an X11 window manager; without one Chromium's
	// toolbar remains visible and page input coordinates no longer match.
	d.wm = exec.CommandContext(ctx, "openbox", "--sm-disable")
	d.wm.Env = d.environment()
	d.wm.Stdout, d.wm.Stderr = io.Discard, io.Discard
	if err = d.wm.Start(); err != nil {
		return nil, errors.New("browser_display_failed")
	}
	d.wmDone = make(chan struct{})
	go func() { _ = d.wm.Wait(); close(d.wmDone) }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		probeCtx, stop := context.WithTimeout(ctx, 500*time.Millisecond)
		probe := exec.CommandContext(probeCtx, "xprop", "-root", "_NET_SUPPORTING_WM_CHECK")
		probe.Env = d.environment()
		output, probeErr := probe.Output()
		stop()
		if probeErr == nil && strings.Contains(string(output), "window id # 0x") {
			break
		}
		select {
		case <-d.wmDone:
			err = errors.New("browser_display_failed")
			return nil, err
		case <-ctx.Done():
			err = errors.New("browser_display_failed")
			return nil, err
		case <-deadline.C:
			err = errors.New("browser_display_failed")
			return nil, err
		case <-time.After(50 * time.Millisecond):
		}
	}
	return d, nil
}

func (d *visualDisplay) environment() []string {
	return append(os.Environ(), "DISPLAY="+d.display, "XAUTHORITY="+d.auth)
}

func (d *visualDisplay) resize(ctx context.Context, width, height int64) error {
	if width < 320 || width > 1600 || height < 240 || height > 1000 {
		return errors.New("invalid_viewport")
	}
	if d.width == width && d.height == height {
		return nil
	}
	name := fmt.Sprintf("kpanel-%dx%d", width, height)
	run := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "xrandr", args...)
		cmd.Env = d.environment()
		return cmd.Run()
	}
	n := func(v int64) string { return strconv.FormatInt(v, 10) }
	if err := run("--newmode", name, "40.00", n(width), n(width+8), n(width+16), n(width+40), n(height), n(height+4), n(height+8), n(height+15)); err != nil {
		return errors.New("browser_display_failed")
	}
	if err := run("--addmode", "VNC-0", name); err != nil {
		_ = run("--rmmode", name)
		return errors.New("browser_display_failed")
	}
	if err := run("--output", "VNC-0", "--mode", name); err != nil {
		_ = run("--delmode", "VNC-0", name)
		_ = run("--rmmode", name)
		return errors.New("browser_display_failed")
	}
	old := d.mode
	d.width, d.height, d.mode = width, height, name
	if old != "" {
		if run("--delmode", "VNC-0", old) != nil || run("--rmmode", old) != nil {
			return errors.New("browser_display_failed")
		}
	}
	return nil
}

func (d *visualDisplay) Close() {
	d.once.Do(func() {
		if d.wm != nil && d.wm.Process != nil {
			_ = d.wm.Process.Kill()
			if d.wmDone != nil {
				select {
				case <-d.wmDone:
				case <-time.After(3 * time.Second):
				}
			}
		}
		if d.cmd != nil && d.cmd.Process != nil {
			_ = d.cmd.Process.Kill()
			if d.done != nil {
				select {
				case <-d.done:
				case <-time.After(3 * time.Second):
				}
			}
		}
		if d.dir != "" {
			_ = os.RemoveAll(d.dir)
		}
	})
}
