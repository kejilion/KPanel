package webbrowser

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestVisualDisplayRejectsInvalidViewport(t *testing.T) {
	for _, size := range [][2]int64{{0, 240}, {1601, 500}, {600, 1001}} {
		if _, err := startVisualDisplay(context.Background(), size[0], size[1]); err == nil {
			t.Fatalf("accepted invalid viewport %v", size)
		}
	}
}

func TestRealVisualDisplayIsolationAndResize(t *testing.T) {
	if os.Getenv("KPANEL_TEST_VNC") != "1" {
		t.Skip("set KPANEL_TEST_VNC=1 in the isolated VNC container")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d, err := startVisualDisplay(ctx, 640, 480)
	if err != nil {
		t.Fatal(err)
	}
	dir := d.dir
	defer d.Close()
	conn, err := net.DialTimeout("unix", d.socket, 2*time.Second)
	if err != nil {
		t.Fatalf("private VNC socket: %v", err)
	}
	conn.Close()
	for _, size := range [][2]int64{{960, 540}, {800, 600}, {960, 540}} {
		if err := d.resize(ctx, size[0], size[1]); err != nil {
			t.Fatalf("resize %v: %v", size, err)
		}
	}
	cmd := exec.CommandContext(ctx, "xrandr", "--query")
	cmd.Env = append(os.Environ(), "DISPLAY="+d.display, "XAUTHORITY="+filepath.Join(dir, "missing"))
	if cmd.Run() == nil {
		t.Fatal("X11 accepted a connection without the private authority file")
	}
	d.Close()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("display directory remained after close: %v", err)
	}
}
