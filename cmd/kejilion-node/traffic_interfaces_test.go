package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

func trafficInterfacesFixture(t *testing.T) (procRoot, path string) {
	t.Helper()
	procRoot = t.TempDir()
	if err := os.MkdirAll(filepath.Join(procRoot, "net"), 0o755); err != nil {
		t.Fatal(err)
	}
	dev := "Inter-| Receive | Transmit\n" +
		" eth0: 3221225472 0 0 0 0 0 0 0 2048 0 0 0 0 0 0 0\n" +
		" warp: 10 0 0 0 0 0 0 0 20 0 0 0 0 0 0 0\n"
	route := "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n" +
		"warp 00000000 0100000A 0003 0 0 100 00000000 0 0 0\n"
	for name, content := range map[string]string{"dev": dev, "route": route} {
		if err := os.WriteFile(filepath.Join(procRoot, "net", name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return procRoot, filepath.Join(t.TempDir(), "traffic-interfaces.json")
}

func TestTrafficInterfacesCommandShowsAndChangesTheSelection(t *testing.T) {
	procRoot, path := trafficInterfacesFixture(t)
	var out bytes.Buffer
	if err := applyTrafficInterfaces(nil, procRoot, path, -1, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "automatic") || !strings.Contains(out.String(), "warp") ||
		!strings.Contains(out.String(), "default-route") || !strings.Contains(out.String(), "3.0 GiB") {
		t.Fatalf("show output:\n%s", out.String())
	}

	out.Reset()
	if err := applyTrafficInterfaces([]string{"include", "eth0", "eth0"}, procRoot, path, -1, &out); err != nil {
		t.Fatal(err)
	}
	selection, _, err := systeminfo.LoadTrafficSelection(path)
	if err != nil || strings.Join(selection.Include, ",") != "eth0" || len(selection.Exclude) != 0 {
		t.Fatalf("stored selection = %#v, %v", selection, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("selection file mode = %v, %v", info, err)
	}
	if !strings.Contains(out.String(), "Counted interfaces: eth0") || !strings.Contains(out.String(), "selected") {
		t.Fatalf("include output:\n%s", out.String())
	}

	out.Reset()
	if err := applyTrafficInterfaces([]string{"exclude", "warp"}, procRoot, path, -1, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "automatic, excluding warp") {
		t.Fatalf("exclude output:\n%s", out.String())
	}

	if err := applyTrafficInterfaces([]string{"auto"}, procRoot, path, -1, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("auto left the selection file: %v", err)
	}
}

func TestTrafficInterfacesCommandRejectsInvalidArguments(t *testing.T) {
	procRoot, path := trafficInterfacesFixture(t)
	for _, arguments := range [][]string{
		{"include"}, {"exclude"}, {"auto", "eth0"}, {"show", "eth0"}, {"remove", "eth0"},
		{"include", "lo"}, {"include", "eth0/1"}, {"include", strings.Repeat("e", 16)},
	} {
		if err := applyTrafficInterfaces(arguments, procRoot, path, -1, &bytes.Buffer{}); err == nil {
			t.Fatalf("applyTrafficInterfaces(%q) accepted invalid arguments", arguments)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected arguments wrote a selection: %v", err)
	}
}
