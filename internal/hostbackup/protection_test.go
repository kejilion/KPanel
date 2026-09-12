package hostbackup

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRuntimeProtection(t *testing.T) {
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	cases := []struct {
		name string
		c    Container
		want bool
		root string
	}{
		{"reserved-name", Container{Name: "kejilion-panel"}, true, ""},
		{"renamed-bind", Container{Name: "my-runtime", Mounts: []Mount{{Type: "bind", Source: "/home/custom-panel", Destination: "/var/lib/kejilion-panel"}}}, true, "/home/custom-panel"},
		{"named-volume", Container{Name: "custom-runtime", Mounts: []Mount{{Type: "volume", Name: "custom-panel", Source: "/opt/docker/volumes/custom-panel/_data", Destination: "/var/lib/kejilion-panel"}}}, true, "/opt/docker/volumes/custom-panel/_data"},
		{"custom-env-destination", Container{Name: "custom", Config: map[string]json.RawMessage{"Env": raw([]string{"KEJILION_PANEL_DATA_DIR=/data"})}, Mounts: []Mount{{Type: "bind", Source: "/home/renamed-data", Destination: "/data"}}}, true, "/home/renamed-data"},
		{"agent-secret", Container{Name: "custom", Mounts: []Mount{{Type: "bind", Source: "/etc/kejilion-panel/agent.token", Destination: "/different-secret-name"}}}, true, "/etc/kejilion-panel/agent.token"},
		{"runtime-marker", Container{Name: "custom", Config: map[string]json.RawMessage{"Entrypoint": raw([]string{"/paneld"})}}, true, ""},
		{"docker-socket-only", Container{Name: "portainer", Mounts: []Mount{{Type: "bind", Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock"}}}, false, ""},
		{"ordinary-data", Container{Name: "ordinary", Mounts: []Mount{{Type: "bind", Source: "/home/app", Destination: "/data"}}}, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, roots := runtimeProtection(tc.c)
			if got != tc.want {
				t.Fatalf("protected=%v want %v", got, tc.want)
			}
			if tc.root != "" && !protectionRootOverlap(roots, tc.root) {
				t.Fatalf("missing protected root %+v", roots)
			}
		})
	}
	for _, p := range []string{"/home/custom-panel", "/home", "/home/custom-panel/ai.db"} {
		if !protectionRootOverlap([]string{"/home/custom-panel"}, p) {
			t.Fatal("missed source/parent/child:", p)
		}
	}
	if protectionRootOverlap([]string{"/home/custom-panel"}, "/home/custom-panel-other") {
		t.Fatal("path prefix collision")
	}
}

func TestBackupBroadMountsAreNotPanelIdentity(t *testing.T) {
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	for _, m := range []Mount{{Type: "bind", Source: "/", Destination: "/rootfs"}, {Type: "bind", Source: "/run", Destination: "/run"}} {
		if protected, roots := runtimeProtection(Container{Name: "cadvisor", Mounts: []Mount{m}}); protected || len(roots) > 0 {
			t.Fatalf("ordinary broad mount identified as Panel: %+v roots=%v", m, roots)
		}
	}
	c := Container{Name: "renamed-runtime", Config: map[string]json.RawMessage{"Env": raw([]string{"KEJILION_PANEL_DATA_DIR=/data/panel"})}, Mounts: []Mount{{Type: "bind", Source: "/home/custom-parent", Destination: "/data"}}}
	if protected, roots := runtimeProtection(c); !protected || !protectionRootOverlap(roots, "/home/custom-parent") {
		t.Fatalf("identified Panel parent bind lost: %v %v", protected, roots)
	}
}
func TestBackupOrdinaryRootMountInventoryKeepsApplicationData(t *testing.T) {
	e, d, p := hostFixture(t)
	e.StateDir = e.host("/var/lib/kejilion-panel/agent")
	c := d.containers[p.Containers[0].ID]
	c.Name = "/cadvisor"
	c.HostConfig["Binds"] = json.RawMessage(`["/:/rootfs:ro","/run:/run:ro"]`)
	c.Mounts = []Mount{{Type: "bind", Source: "/", Destination: "/rootfs"}, {Type: "bind", Source: "/run", Destination: "/run"}}
	inv, err := e.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.ProtectedContainers) > 0 || len(inv.ProtectedRoots) > 0 {
		t.Fatalf("ordinary container masked host: %+v", inv.ProtectedRoots)
	}
	found := false
	for _, root := range inv.Roots {
		if root.Path == "/home/app" && root.Bytes == 4 {
			found = true
		}
	}
	if !found {
		t.Fatalf("ordinary broad mounts silently removed application data: %+v", inv.Roots)
	}
	if len(inv.Containers) != 1 {
		t.Fatal("ordinary container config omitted")
	}
	dir := t.TempDir()
	if err := e.Create(context.Background(), dir, []string{"apps", "web", "docker"}, inv.Revision); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(dir, "apps.payload"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	g, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	b, err := io.ReadAll(g)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("old!")) {
		t.Fatal("application data missing from successful export")
	}
}
