package hostbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/backup"
)

func TestExportLocalVolumesRoundTrip(t *testing.T) {
	for _, module := range []string{"apps", "web", "docker"} {
		t.Run(module, func(t *testing.T) {
			e, docker, _ := hostFixture(t)
			var c *fixtureContainer
			for _, item := range docker.containers {
				c = item
			}
			bind := "/home/app"
			if module == "web" {
				bind = "/home/web"
			}
			if module == "docker" {
				bind = "/srv/app"
			}
			if err := os.MkdirAll(e.host(bind), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(e.host(bind), "data"), []byte("snapshot-bind"), 0o640); err != nil {
				t.Fatal(err)
			}
			c.Mounts = []Mount{{Type: "bind", Source: bind, Destination: "/data", RW: true}}
			bindings := []string{bind + ":/data:rw"}
			docker.volumes = map[string]Volume{}
			for n, name := range []string{"app-named", strings.Repeat("e", 64)} {
				v := Volume{Name: name, Driver: "local", Mountpoint: "/var/lib/docker/volumes/" + name + "/_data"}
				docker.volumes[name] = v
				destination := []string{"/named", "/anonymous"}[n]
				bindings = append(bindings, name+":"+destination+":rw")
				c.Mounts = append(c.Mounts, Mount{Type: "volume", Name: name, Source: v.Mountpoint, Destination: destination, RW: true})
				if err := os.MkdirAll(e.host(v.Mountpoint), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(e.host(v.Mountpoint), "value"), []byte("snapshot-"+name), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			c.HostConfig["Binds"], _ = json.Marshal(bindings)
			i, err := e.Inventory(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := e.Create(context.Background(), dir, []string{module}, i.Revision); err != nil {
				t.Fatal(err)
			}
			var encrypted bytes.Buffer
			if _, err := backup.Write(&encrypted, "round-trip-password", "fixture", []backup.Source{{Module: module, Path: filepath.Join(dir, module+".payload")}}); err != nil {
				t.Fatal(err)
			}
			restore := t.TempDir()
			if _, err := backup.Read(bytes.NewReader(encrypted.Bytes()), "round-trip-password", restore); err != nil {
				t.Fatal(err)
			}
			payload, err := e.ReadPayload(context.Background(), filepath.Join(restore, module+".payload"), t.TempDir())
			if err != nil {
				t.Fatalf("export cannot be imported: %v", err)
			}
			if len(payload.Volumes) != 2 {
				t.Fatalf("volumes=%+v", payload.Volumes)
			}
			for _, v := range docker.volumes {
				if err := os.WriteFile(filepath.Join(e.host(v.Mountpoint), "value"), []byte("changed"), 0o640); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.Restore(context.Background(), backup.NewID(), restore, []string{module}); err != nil {
				t.Fatal(err)
			}
			for _, v := range docker.volumes {
				got, err := os.ReadFile(filepath.Join(e.host(v.Mountpoint), "value"))
				if err != nil || string(got) != "snapshot-"+v.Name {
					t.Fatalf("restored volume=%q err=%v", got, err)
				}
			}
		})
	}
}

func TestLocalVolumeRootRequiresNativeDeclaration(t *testing.T) {
	e, _, p := hostFixture(t)
	volumePath := "/var/lib/docker/volumes/app-named/_data"
	sum := sha256.Sum256([]byte(volumePath))
	hash := hex.EncodeToString(sum[:16])
	p.Roots = []Root{{ID: hash, Path: volumePath, Module: "apps", Directory: true}}
	p.Containers[0].Mounts = []Mount{{Type: "volume", Name: "app-named", Source: volumePath, Destination: "/data"}}
	p.Containers[0].HostConfig["Binds"] = json.RawMessage(`["app-named:/data"]`)
	p.Volumes = map[string]Volume{"app-named": {Name: "app-named", Driver: "local", Mountpoint: volumePath}}
	if err := e.validatePayload(p); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Payload){
		func(p *Payload) { p.Volumes = nil },
		func(p *Payload) { p.Containers = nil },
		func(p *Payload) { p.Containers[0].Module = "docker" },
		func(p *Payload) {
			v := p.Volumes["app-named"]
			v.Mountpoint = "/root/foreign"
			p.Volumes["app-named"] = v
		},
		func(p *Payload) { v := p.Volumes["app-named"]; v.Driver = "nfs"; p.Volumes["app-named"] = v },
		func(p *Payload) {
			v := p.Volumes["app-named"]
			v.Options = map[string]string{"device": "/root"}
			p.Volumes["app-named"] = v
		},
	} {
		body, _ := json.Marshal(p)
		var bad Payload
		_ = json.Unmarshal(body, &bad)
		mutate(&bad)
		if err := e.validatePayload(bad); err == nil {
			t.Fatal("undeclared or unsupported volume root accepted")
		}
	}
}
