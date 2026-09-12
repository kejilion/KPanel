//go:build linux

package hostbackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/kejilion/kejilion-panel/internal/backup"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

var niTask = os.Getenv("KPB_NATIVE_TASK")

func niEngine(t *testing.T) *Engine {
	t.Helper()
	if os.Getenv("KPB_NATIVE_INTEGRATION") != "1" {
		t.Skip("isolated native Docker only")
	}
	niIsolation(t)
	e := NewWithSocket(niTask+"/jobs", niTask+"/docker.sock")
	e.Root = niTask + "/fs"
	e.Client.Transport = niTrace{t, e.Client.Transport}
	return e
}

// niIsolation prohibits accidental interaction with an existing Docker daemon.
// The shell harness must create both namespaces and the marker before enabling tests.
func niIsolation(t *testing.T) {
	t.Helper()
	if !filepath.IsAbs(niTask) || filepath.Clean(niTask) != niTask || !strings.HasPrefix(niTask, "/var/tmp/kpanel-native-review.") {
		t.Fatal("KPB_NATIVE_TASK must be an isolated /var/tmp/kpanel-native-review.* directory")
	}
	if os.Geteuid() != 0 {
		t.Fatal("native fixture requires root inside its isolated namespace")
	}
	for _, p := range []string{niTask, niTask + "/isolation.json", niTask + "/docker.sock"} {
		st, err := os.Lstat(p)
		if err != nil || st.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("invalid fixture path %s: %v", p, err)
		}
	}
	var marker struct {
		Task    string
		MountNS string
		NetNS   string
	}
	raw, err := os.ReadFile(niTask + "/isolation.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &marker); err != nil {
		t.Fatal(err)
	}
	mount, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	net, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	initMount, err := os.Readlink("/proc/1/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	initNet, err := os.Readlink("/proc/1/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	if marker.Task != niTask || marker.MountNS != mount || marker.NetNS != net || mount == initMount || net == initNet {
		t.Fatal("fixture marker does not match a private mount/network namespace")
	}
	for _, name := range []string{"home", "opt"} {
		a, err := os.Stat("/" + name)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.Stat(niTask + "/fs/" + name)
		if err != nil || !os.SameFile(a, b) {
			t.Fatalf("/%s is not fixture-owned", name)
		}
	}
	socket, err := os.Lstat(niTask + "/docker.sock")
	if err != nil || socket.Mode()&os.ModeSocket == 0 {
		t.Fatal("fixture Docker socket missing")
	}
}

type niTrace struct {
	t    *testing.T
	base http.RoundTripper
}

func (n niTrace) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := n.base.RoundTrip(r)
	if err == nil && resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 16384))
		resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(body))
		n.t.Logf("DOCKER-ERROR %s %s status=%d body=%s", r.Method, r.URL.Path, resp.StatusCode, body)
		if r.GetBody != nil && r.URL.Path == "/containers/create" {
			rd, _ := r.GetBody()
			request, _ := io.ReadAll(rd)
			rd.Close()
			n.t.Logf("CREATE-INPUT %s", request)
		}
	}
	return resp, err
}
func niRaw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func niCall(t *testing.T, e *Engine, method, path string, in, out any) {
	t.Helper()
	if err := e.docker(context.Background(), method, path, in, out); err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
}
func niInspect(t *testing.T, e *Engine, name string) Container {
	t.Helper()
	var r struct {
		ID                 string `json:"Id"`
		Image              string
		Name               string
		Config, HostConfig map[string]json.RawMessage
		Mounts             []Mount
		State              struct{ Running bool }
		NetworkSettings    struct{ Networks map[string]json.RawMessage }
	}
	niCall(t, e, "GET", "/containers/"+name+"/json", nil, &r)
	return Container{ID: r.ID, ImageID: r.Image, Name: strings.TrimPrefix(r.Name, "/"), Config: r.Config, HostConfig: r.HostConfig, Mounts: r.Mounts, Networks: r.NetworkSettings.Networks, Running: r.State.Running}
}
func niCreate(t *testing.T, e *Engine, name string, h map[string]any) string {
	t.Helper()
	var r struct {
		ID string `json:"Id"`
	}
	niCall(t, e, "POST", "/containers/create?name="+name, map[string]any{"Image": "kpb-native-fixture:v1", "HostConfig": h}, &r)
	niCall(t, e, "POST", "/containers/"+r.ID+"/start", nil, nil)
	return r.ID
}
func niDir(t *testing.T) string {
	t.Helper()
	d := filepath.Join(niTask, "jobs", backup.NewID())
	if err := backup.PrivateDir(d); err != nil {
		t.Fatal(err)
	}
	return d
}
func niCleanup(t *testing.T, e *Engine) {
	t.Helper()
	t.Cleanup(func() {
		var items []struct {
			ID    string `json:"Id"`
			Names []string
		}
		if err := e.docker(context.Background(), "GET", "/containers/json?all=1", nil, &items); err != nil {
			t.Error(err)
			return
		}
		for _, c := range items {
			safe := false
			for _, n := range c.Names {
				if strings.HasPrefix(n, "/native-") || strings.HasPrefix(n, "/kpanel-previous-") {
					safe = true
				}
			}
			if !safe {
				t.Errorf("unexpected isolated container: %+v", c)
				continue
			}
			if err := e.docker(context.Background(), "DELETE", "/containers/"+c.ID+"?force=1", nil, nil); err != nil {
				t.Error(err)
			}
		}
	})
}
func niWrite(t *testing.T, p, v string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(v), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(p, 12345, 23456); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0640); err != nil {
		t.Fatal(err)
	}
}
func niCheckFile(t *testing.T, p, want string) {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil || string(b) != want {
		t.Fatalf("file %s=%q want %q err=%v", p, b, want, err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	x := st.Sys().(*syscall.Stat_t)
	if x.Uid != 12345 || x.Gid != 23456 || st.Mode().Perm() != 0640 {
		t.Fatalf("metadata %s uid/gid=%d/%d mode=%o", p, x.Uid, x.Gid, st.Mode().Perm())
	}
}
func niArchive(t *testing.T, e *Engine, modules []string) string {
	t.Helper()
	i, err := e.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 5; n++ {
		next, err := e.Inventory(context.Background())
		if err != nil || next.Revision != i.Revision {
			t.Fatalf("unstable inventory rev %s/%s err=%v", i.Revision, next.Revision, err)
		}
	}
	d := niDir(t)
	if err := e.Create(context.Background(), d, modules, i.Revision); err != nil {
		t.Fatalf("export: %v", err)
	}
	return d
}
func niEncryptDecode(t *testing.T, dir string, modules []string) string {
	t.Helper()
	sources := []backup.Source{}
	for _, m := range modules {
		sources = append(sources, backup.Source{Module: m, Path: filepath.Join(dir, m+".payload")})
	}
	var encrypted bytes.Buffer
	if _, err := backup.Write(&encrypted, "test-only-password-29", "integration", sources); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "encrypted.kpb"), encrypted.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	bad := append([]byte(nil), encrypted.Bytes()...)
	bad[len(bad)-1] ^= 1
	if _, err := backup.Read(bytes.NewReader(bad), "test-only-password-29", niDir(t)); err == nil {
		t.Fatal("corrupt encrypted archive accepted")
	}
	if _, err := backup.Read(bytes.NewReader(encrypted.Bytes()), "wrong-password", niDir(t)); err == nil {
		t.Fatal("wrong password accepted")
	}
	dst := niDir(t)
	if _, err := backup.Read(bytes.NewReader(encrypted.Bytes()), "test-only-password-29", dst); err != nil {
		t.Fatal(err)
	}
	return dst
}
func niNoOldContainers(t *testing.T, e *Engine) {
	t.Helper()
	var list []struct {
		ID    string
		Names []string
	}
	niCall(t, e, "GET", "/containers/json?all=1", nil, &list)
	for _, c := range list {
		for _, n := range c.Names {
			if strings.Contains(n, "kpanel-previous") {
				t.Fatalf("old container retained: %s", n)
			}
		}
	}
	if err := filepath.WalkDir(e.Root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.Contains(d.Name(), ".kpanel-restore-") || strings.Contains(d.Name(), ".kpanel-next-") {
			return fmt.Errorf("retained data: %s", p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
func niModify(t *testing.T, in, out string, fn func(*Payload)) {
	t.Helper()
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	o, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	g := gzip.NewWriter(o)
	tw := tar.NewWriter(g)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if h.Name == "manifest.json" {
			var p Payload
			if err := json.Unmarshal(b, &p); err != nil {
				t.Fatal(err)
			}
			fn(&p)
			b, err = json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			h.Size = int64(len(b))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := o.Sync(); err != nil {
		t.Fatal(err)
	}
}
func TestNativeIntegration(t *testing.T) {
	t.Run("EncryptedBindNamedAnonymousRoundTrip", func(t *testing.T) {
		e := niEngine(t)
		niCleanup(t, e)
		bind := e.host("/home/native-roundtrip/value")
		niWrite(t, bind, "snapshot-bind")
		var v Volume
		niCall(t, e, "POST", "/volumes/create", map[string]any{"Name": "native-named-volume"}, &v)
		id := niCreate(t, e, "native-roundtrip", map[string]any{"Binds": []string{"/home/native-roundtrip:/bind:rw", "native-named-volume:/named:rw"}, "Mounts": []map[string]any{{"Type": "volume", "Target": "/anon"}}})
		c := niInspect(t, e, id)
		paths := map[string]string{"/bind": bind}
		for _, m := range c.Mounts {
			if m.Type == "volume" {
				p := e.host(m.Source) + "/value"
				paths[m.Destination] = p
				niWrite(t, p, "snapshot-"+m.Destination)
			}
		}
		if len(paths) != 3 {
			t.Fatalf("missing mounts: %+v", c.Mounts)
		}
		exp := niArchive(t, e, []string{"apps"})
		if !niInspect(t, e, id).Running {
			t.Fatal("source not restarted")
		}
		dst := niEncryptDecode(t, exp, []string{"apps"})
		for _, p := range paths {
			niWrite(t, p, "target-old")
		}
		if err := e.Restore(context.Background(), filepath.Base(dst), dst, []string{"apps"}); err != nil {
			t.Fatal(err)
		}
		restored := niInspect(t, e, "native-roundtrip")
		if !restored.Running || restored.ID == id {
			t.Fatalf("container not recreated/running: %+v", restored)
		}
		if len(restored.Mounts) != 3 {
			t.Fatalf("restored mount count=%d", len(restored.Mounts))
		}
		for _, mount := range restored.Mounts {
			p := e.host(mount.Source) + "/value"
			if expected, ok := paths[mount.Destination]; !ok || p != expected {
				t.Fatalf("restored mount mismatch: %+v paths=%v", mount, paths)
			}
		}
		niCheckFile(t, bind, "snapshot-bind")
		for target, p := range paths {
			if target != "/bind" {
				niCheckFile(t, p, "snapshot-"+target)
			}
		}
		niNoOldContainers(t, e)
		t.Logf("PASS encrypted=%s sourceID=%s restoredID=%s mounts=%d", filepath.Join(exp, "encrypted.kpb"), id, restored.ID, len(paths))
	})
	for _, kind := range []string{"MissingImage", "CorruptPayload", "CreateFailure", "HealthFailure"} {
		t.Run(kind, func(t *testing.T) {
			e := niEngine(t)
			niCleanup(t, e)
			name := "native-" + strings.ToLower(kind)
			path := "/home/" + name
			file := e.host(path) + "/value"
			niWrite(t, file, "snapshot")
			id := niCreate(t, e, name, map[string]any{"Binds": []string{path + ":/data:rw"}})
			exp := niArchive(t, e, []string{"apps"})
			dst := niDir(t)
			out := filepath.Join(dst, "apps.payload")
			if kind == "CorruptPayload" {
				b, err := os.ReadFile(filepath.Join(exp, "apps.payload"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(out, b[:len(b)-6], 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				niModify(t, filepath.Join(exp, "apps.payload"), out, func(p *Payload) {
					for n := range p.Containers {
						if p.Containers[n].Name != name {
							continue
						}
						switch kind {
						case "MissingImage":
							p.Containers[n].ImageID = "sha256:" + strings.Repeat("f", 64)
							p.Containers[n].ImageRef = ""
						case "CreateFailure":
							p.Containers[n].HostConfig["Runtime"] = niRaw("kpb-missing-runtime")
						case "HealthFailure":
							p.Containers[n].Config["Healthcheck"] = niRaw(map[string]any{"Test": []string{"CMD", "/fixture", "health-fail"}, "Interval": int64(time.Second), "Timeout": int64(time.Second), "Retries": 1})
						}
					}
				})
			}
			niWrite(t, file, "target-before-failure")
			err := e.Restore(context.Background(), filepath.Base(dst), dst, []string{"apps"})
			if err == nil {
				t.Fatal("expected restore failure")
			}
			if kind == "CreateFailure" || kind == "HealthFailure" {
				var failure *backup.Failure
				if !errors.As(err, &failure) || failure.Code != "rolled_back" {
					t.Fatalf("failure did not complete rollback: %T %v", err, err)
				}
			}
			niCheckFile(t, file, "target-before-failure")
			c := niInspect(t, e, name)
			if c.ID != id || !c.Running {
				t.Fatalf("old container not preserved/restarted: %s running=%v", c.ID, c.Running)
			}
			niNoOldContainers(t, e)
			t.Logf("PASS failure=%s err=%v evidence=%s", kind, err, dst)
		})
	}
	t.Run("NamespaceLegacyLinkAndNetworkID", func(t *testing.T) {
		e := niEngine(t)
		niCleanup(t, e)
		donor := niCreate(t, e, "native-donor", map[string]any{})
		niCreate(t, e, "native-sidecar", map[string]any{"NetworkMode": "container:" + donor})
		niCreate(t, e, "native-linked", map[string]any{"Links": []string{"native-donor:db"}})
		var networks []struct{ Name string }
		niCall(t, e, "GET", "/networks", nil, &networks)
		for _, network := range networks {
			if network.Name == "native-custom-net" {
				niCall(t, e, "DELETE", "/networks/native-custom-net", nil, nil)
			}
		}
		var network struct {
			ID string `json:"Id"`
		}
		niCall(t, e, "POST", "/networks/create", map[string]any{"Name": "native-custom-net", "Driver": "bridge"}, &network)
		niCreate(t, e, "native-networkid", map[string]any{"NetworkMode": network.ID})
		modules := []string{"apps", "web", "docker"}
		exp := niArchive(t, e, modules)
		for _, name := range []string{"native-donor", "native-sidecar", "native-linked", "native-networkid"} {
			if !niInspect(t, e, name).Running {
				t.Fatal("source not restarted:", name)
			}
		}
		dst := niEncryptDecode(t, exp, modules)
		if err := e.Restore(context.Background(), filepath.Base(dst), dst, modules); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"native-donor", "native-sidecar", "native-linked", "native-networkid"} {
			if !niInspect(t, e, name).Running {
				t.Fatal("restored container not running:", name)
			}
		}
		cmd := exec.Command("docker", "-H", "unix://"+niTask+"/docker.sock", "exec", "native-linked", "/fixture", "read", "/etc/hosts")
		out, err := cmd.CombinedOutput()
		aliasFound := false
		for _, field := range strings.Fields(string(out)) {
			if field == "db" {
				aliasFound = true
			}
		}
		if err != nil || !aliasFound {
			t.Fatalf("link hosts missing: %s err=%v", out, err)
		}
		newDonor := niInspect(t, e, "native-donor")
		sidecar := niInspect(t, e, "native-sidecar")
		var namespace string
		if json.Unmarshal(sidecar.HostConfig["NetworkMode"], &namespace) != nil || namespace != "container:"+newDonor.ID {
			t.Fatalf("namespace points to old/wrong donor: %s", namespace)
		}
		networked := niInspect(t, e, "native-networkid")
		var networkMode string
		if json.Unmarshal(networked.HostConfig["NetworkMode"], &networkMode) != nil || networkMode != "native-custom-net" {
			t.Fatalf("old network ID was replayed: %s", networkMode)
		}
		niNoOldContainers(t, e)
		t.Logf("PASS namespace, legacy links, network ID; archive=%s", exp)
	})
}

type niCancelAfterStop struct {
	base   http.RoundTripper
	cancel context.CancelFunc
	fired  bool
}

func (n *niCancelAfterStop) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := n.base.RoundTrip(r)
	if err == nil && resp.StatusCode < 400 && r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/stop") && !n.fired {
		n.fired = true
		n.cancel()
	}
	return resp, err
}
func TestNativeInterruption(t *testing.T) {
	t.Run("CanceledExportRestartsAndClearsJournal", func(t *testing.T) {
		e := niEngine(t)
		niCleanup(t, e)
		path := "/home/native-canceled-export"
		niWrite(t, e.host(path)+"/value", "source-unchanged")
		id := niCreate(t, e, "native-canceled-export", map[string]any{"Binds": []string{path + ":/data:rw"}})
		inv, err := e.Inventory(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		tr := &niCancelAfterStop{base: e.Client.Transport, cancel: cancel}
		e.Client.Transport = tr
		dir := niDir(t)
		err = e.Create(ctx, dir, []string{"apps"}, inv.Revision)
		if err == nil || !tr.fired {
			t.Fatalf("export unexpectedly succeeded or stop not observed: %v fired=%v", err, tr.fired)
		}
		if !niInspect(t, e, id).Running {
			t.Fatal("source stopped after canceled export")
		}
		if _, err := os.Stat(filepath.Join(dir, "export-journal.json")); !os.IsNotExist(err) {
			t.Fatalf("export journal not removed after successful restart: %v", err)
		}
		niCheckFile(t, e.host(path)+"/value", "source-unchanged")
		t.Logf("PASS cancellation after real stop err=%v evidence=%s", err, dir)
	})
	t.Run("DurableExportJournalRestartsInDependencyOrder", func(t *testing.T) {
		e := niEngine(t)
		niCleanup(t, e)
		id := niCreate(t, e, "native-recover-donor", map[string]any{})
		niCreate(t, e, "native-recover-sidecar", map[string]any{"NetworkMode": "container:" + id})
		inv, err := e.Inventory(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		p := Payload{Containers: inv.Containers}
		if err := orderContainers(&p); err != nil {
			t.Fatal(err)
		}
		running := []Container{}
		for _, c := range p.Containers {
			if c.Running {
				running = append(running, Container{ID: c.ID, Name: c.Name})
			}
		}
		if len(running) != 2 {
			t.Fatalf("expected two isolated containers: %+v", running)
		}
		dir := niDir(t)
		path := filepath.Join(dir, "export-journal.json")
		if err := backup.WriteJSON(path, running); err != nil {
			t.Fatal(err)
		}
		for n := len(running) - 1; n >= 0; n-- {
			niCall(t, e, "POST", "/containers/"+running[n].ID+"/stop", nil, nil)
		}
		for _, c := range running {
			if niInspect(t, e, c.ID).Running {
				t.Fatal("interruption fixture did not stop:", c.Name)
			}
		}
		if err := e.Recover(context.Background(), filepath.Base(dir), dir); err != nil {
			t.Fatal(err)
		}
		for _, c := range running {
			if !niInspect(t, e, c.ID).Running {
				t.Fatal("recovery failed restart:", c.Name)
			}
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("export recovery journal remains: %v", err)
		}
		if err := e.Recover(context.Background(), filepath.Base(dir), dir); err != nil {
			t.Fatalf("idempotent recovery: %v", err)
		}
		t.Logf("PASS durable interruption fixture and idempotent recovery evidence=%s", dir)
	})
}
func TestNativeDataRootMigration(t *testing.T) {
	source := niEngine(t)
	niCleanup(t, source)
	target := NewWithSocket(niTask+"/jobs", niTask+"/docker-migrate.sock")
	target.Root = source.Root
	target.Client.Transport = niTrace{t, target.Client.Transport}
	niCleanup(t, target)
	var info struct{ DockerRootDir string }
	niCall(t, target, "GET", "/info", nil, &info)
	if info.DockerRootDir != "/opt/docker-migrate" {
		t.Fatalf("unexpected migration root: %s", info.DockerRootDir)
	}
	path := "/home/native-migrate"
	file := source.host(path) + "/value"
	niWrite(t, file, "snapshot-bind")
	niCall(t, source, "POST", "/volumes/create", map[string]any{"Name": "native-migrate-named"}, nil)
	id := niCreate(t, source, "native-migrate", map[string]any{"Binds": []string{path + ":/bind:rw", "native-migrate-named:/named:rw"}, "Mounts": []map[string]any{{"Type": "volume", "Target": "/anon"}}})
	old := niInspect(t, source, id)
	oldPaths := map[string]string{}
	for _, m := range old.Mounts {
		if m.Type == "volume" {
			p := source.host(m.Source) + "/value"
			oldPaths[m.Destination] = p
			niWrite(t, p, "snapshot-"+m.Destination)
		}
	}
	if len(oldPaths) != 2 {
		t.Fatalf("named/anonymous volume missing: %+v", old.Mounts)
	}
	exp := niArchive(t, source, []string{"apps"})
	dst := niEncryptDecode(t, exp, []string{"apps"})
	niCall(t, source, "DELETE", "/containers/"+id+"?force=1", nil, nil)
	niWrite(t, file, "target-old-bind")
	for _, p := range oldPaths {
		niWrite(t, p, "source-volume-unchanged")
	}
	if err := target.Restore(context.Background(), filepath.Base(dst), dst, []string{"apps"}); err != nil {
		t.Fatal(err)
	}
	c := niInspect(t, target, "native-migrate")
	if len(c.Mounts) != 3 {
		t.Fatalf("migrated mount count=%d", len(c.Mounts))
	}
	if !c.Running {
		t.Fatal("migrated container not running")
	}
	niCheckFile(t, file, "snapshot-bind")
	for _, m := range c.Mounts {
		if m.Type == "volume" {
			if !strings.HasPrefix(m.Source, "/opt/docker-migrate/volumes/") {
				t.Fatalf("source data root replayed: %+v", m)
			}
			niCheckFile(t, target.host(m.Source)+"/value", "snapshot-"+m.Destination)
		}
	}
	for _, p := range oldPaths {
		niCheckFile(t, p, "source-volume-unchanged")
	}
	niNoOldContainers(t, target)
	t.Logf("PASS distinct data-root=%s archive=%s mounts=%+v", info.DockerRootDir, exp, c.Mounts)
}

type niWatchStops struct {
	base    http.RoundTripper
	watched map[string]bool
	stopped map[string]bool
}

func (n *niWatchStops) RoundTrip(r *http.Request) (*http.Response, error) {
	parts := strings.Split(r.URL.Path, "/")
	if r.Method == "POST" && len(parts) == 4 && parts[1] == "containers" && parts[3] == "stop" && n.watched[parts[2]] {
		n.stopped[parts[2]] = true
	}
	return n.base.RoundTrip(r)
}
func TestNativeProtection(t *testing.T) {
	t.Run("RenamedCustomBindAndNamedVolumeAreExcluded", func(t *testing.T) {
		e := niEngine(t)
		niCleanup(t, e)
		bind := "/home/native-protected-bind"
		niWrite(t, e.host(bind)+"/ai-history", "DO-NOT-EXPORT-BIND-HISTORY")
		first := niCreate(t, e, "native-panel-original", map[string]any{"Binds": []string{bind + ":/var/lib/kejilion-panel:rw"}})
		niCall(t, e, "POST", "/containers/"+first+"/rename?name=native-renamed-panel", nil, nil)
		niCall(t, e, "POST", "/volumes/create", map[string]any{"Name": "native-protected-volume"}, nil)
		second := niCreate(t, e, "native-volume-runtime", map[string]any{"Binds": []string{"native-protected-volume:/var/lib/kejilion-panel:rw"}})
		volume := niInspect(t, e, second)
		var volumeRoot string
		for _, m := range volume.Mounts {
			if m.Type == "volume" {
				volumeRoot = m.Source
			}
		}
		if volumeRoot == "" {
			t.Fatal("no protected named volume")
		}
		niWrite(t, e.host(volumeRoot)+"/ai-history", "DO-NOT-EXPORT-VOLUME-HISTORY")
		inv, err := e.Inventory(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(inv.ProtectedContainers) != 2 || !protectionRootOverlap(inv.ProtectedRoots, bind) || !protectionRootOverlap(inv.ProtectedRoots, volumeRoot) {
			t.Fatalf("protection inventory incomplete: %+v", inv.ProtectedRoots)
		}
		for _, c := range inv.Containers {
			if c.ID == first || c.ID == second {
				t.Fatal("runtime container exported")
			}
		}
		for _, root := range inv.Roots {
			if protectionRootOverlap(inv.ProtectedRoots, root.Path) {
				t.Fatal("runtime data root exported:", root.Path)
			}
		}
		watch := &niWatchStops{base: e.Client.Transport, watched: map[string]bool{first: true, second: true}, stopped: map[string]bool{}}
		e.Client.Transport = watch
		dir := niDir(t)
		modules := []string{"apps", "web", "docker"}
		if err := e.Create(context.Background(), dir, modules, inv.Revision); err != nil {
			t.Fatal(err)
		}
		if len(watch.stopped) > 0 {
			t.Fatal("protected container was stopped:", watch.stopped)
		}
		for _, m := range modules {
			f, err := os.Open(filepath.Join(dir, m+".payload"))
			if err != nil {
				t.Fatal(err)
			}
			g, err := gzip.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(g)
			g.Close()
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(raw, []byte("DO-NOT-EXPORT-")) {
				t.Fatal("protected AI history included:", m)
			}
		}
		niCheckFile(t, e.host(bind)+"/ai-history", "DO-NOT-EXPORT-BIND-HISTORY")
		niCheckFile(t, e.host(volumeRoot)+"/ai-history", "DO-NOT-EXPORT-VOLUME-HISTORY")
		if !niInspect(t, e, first).Running || !niInspect(t, e, second).Running {
			t.Fatal("runtime no longer running")
		}
		t.Logf("PASS renamed bind/named volume excluded, no stop requests; evidence=%s", dir)
	})
	t.Run("SameNameCannotReplaceRuntime", func(t *testing.T) {
		e := niEngine(t)
		niCleanup(t, e)
		name := "native-runtime-collision"
		sourcePath := "/home/native-collision-app"
		niWrite(t, e.host(sourcePath)+"/value", "source")
		sourceID := niCreate(t, e, name, map[string]any{"Binds": []string{sourcePath + ":/data:rw"}})
		exp := niArchive(t, e, []string{"apps"})
		dst := niEncryptDecode(t, exp, []string{"apps"})
		niCall(t, e, "DELETE", "/containers/"+sourceID+"?force=1", nil, nil)
		runtimePath := "/home/native-destination-runtime"
		niWrite(t, e.host(runtimePath)+"/value", "runtime-untouched")
		runtimeID := niCreate(t, e, name, map[string]any{"Binds": []string{runtimePath + ":/var/lib/kejilion-panel:rw"}})
		watch := &niWatchStops{base: e.Client.Transport, watched: map[string]bool{runtimeID: true}, stopped: map[string]bool{}}
		e.Client.Transport = watch
		err := e.Restore(context.Background(), filepath.Base(dst), dst, []string{"apps"})
		if err == nil {
			t.Fatal("runtime same-name replacement accepted")
		}
		current := niInspect(t, e, name)
		if current.ID != runtimeID || !current.Running || len(watch.stopped) > 0 {
			t.Fatal("runtime touched")
		}
		niCheckFile(t, e.host(runtimePath)+"/value", "runtime-untouched")
		niNoOldContainers(t, e)
		t.Logf("PASS same-name ordinary source blocked before target stop: %v", err)
	})
	t.Run("RuntimePayloadRejected", func(t *testing.T) {
		e := niEngine(t)
		niCleanup(t, e)
		name := "native-marked-payload"
		path := "/home/native-marked-payload"
		niWrite(t, e.host(path)+"/value", "target")
		id := niCreate(t, e, name, map[string]any{"Binds": []string{path + ":/data:rw"}})
		exp := niArchive(t, e, []string{"apps"})
		dst := niDir(t)
		niModify(t, filepath.Join(exp, "apps.payload"), filepath.Join(dst, "apps.payload"), func(p *Payload) {
			for n := range p.Containers {
				p.Containers[n].Config["Env"] = niRaw([]string{"KEJILION_PANEL_DATA_DIR=/data"})
			}
		})
		err := e.Restore(context.Background(), filepath.Base(dst), dst, []string{"apps"})
		if err == nil {
			t.Fatal("Panel-marked payload accepted")
		}
		current := niInspect(t, e, name)
		if current.ID != id || !current.Running {
			t.Fatal("ordinary target changed")
		}
		niCheckFile(t, e.host(path)+"/value", "target")
		t.Logf("PASS package runtime marker rejected: %v", err)
	})
	t.Run("AncestorAndSharedRootAreNotSilentlySkipped", func(t *testing.T) {
		e := niEngine(t)
		niCleanup(t, e)
		root := "/home/native-parent-protection"
		path := root + "/panel"
		niWrite(t, e.host(path)+"/value", "runtime")
		niWrite(t, e.host(root)+"/other-app", "sibling")
		id := niCreate(t, e, "native-parent-runtime", map[string]any{"Binds": []string{path + ":/var/lib/kejilion-panel:rw"}})
		niCreate(t, e, "native-shared-data", map[string]any{"Binds": []string{path + ":/data:rw"}})
		inv, err := e.Inventory(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		issue := false
		for _, m := range inv.Modules {
			if m.Issue == "protected_panel_data" {
				issue = true
			}
		}
		if !issue {
			t.Fatal("ancestor/shared data silently accepted")
		}
		err = e.Create(context.Background(), niDir(t), []string{"apps", "web", "docker"}, inv.Revision)
		if err == nil {
			t.Fatal("unsafe selection exported")
		}
		if !niInspect(t, e, id).Running {
			t.Fatal("runtime stopped")
		}
		niCheckFile(t, e.host(path)+"/value", "runtime")
		niCheckFile(t, e.host(root)+"/other-app", "sibling")
		t.Logf("PASS ancestor/shared roots explicitly blocked: %v", err)
	})
}
