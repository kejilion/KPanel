package hostbackup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/backup"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fixtureContainer struct {
	ID              string
	Name            string
	Image           string
	Config          map[string]json.RawMessage
	HostConfig      map[string]json.RawMessage
	Mounts          []Mount
	State           struct{ Running bool }
	NetworkSettings struct{ Networks map[string]json.RawMessage }
}
type fixtureDocker struct {
	mu         sync.Mutex
	containers map[string]*fixtureContainer
	calls      []string
	failCreate bool
	failDelete bool
}

func (f *fixtureDocker) request(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := r.Method + " " + r.URL.Path
	f.calls = append(f.calls, key)
	status := 200
	var value any = map[string]any{}
	switch {
	case key == "GET /info":
		value = map[string]any{"SecurityOptions": []string{}}
	case key == "GET /containers/json":
		items := []any{}
		for _, c := range f.containers {
			state := "exited"
			if c.State.Running {
				state = "running"
			}
			items = append(items, map[string]any{"Id": c.ID, "Names": []string{c.Name}, "State": state})
		}
		value = items
	case strings.HasPrefix(r.URL.Path, "/images/"):
		value = map[string]any{"Id": "sha256:" + strings.Repeat("a", 64), "RepoDigests": []string{"example/app@sha256:" + strings.Repeat("b", 64)}}
	case key == "POST /containers/create":
		if f.failCreate {
			status = 500
			break
		}
		var input map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			return nil, err
		}
		id := strings.Repeat("c", 64)
		c := &fixtureContainer{ID: id, Name: "/" + r.URL.Query().Get("name"), Image: "sha256:" + strings.Repeat("a", 64), Config: input}
		_ = json.Unmarshal(input["HostConfig"], &c.HostConfig)
		f.containers[id] = c
		value = map[string]string{"Id": id}
	case strings.HasPrefix(r.URL.Path, "/containers/"):
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/containers/"), "/")
		var c *fixtureContainer
		for _, candidate := range f.containers {
			if candidate.ID == parts[0] || candidate.Name == "/"+parts[0] {
				c = candidate
			}
		}
		if c == nil {
			status = 404
			break
		}
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}
		switch {
		case r.Method == "GET" && action == "json":
			value = c
		case action == "stop":
			c.State.Running = false
		case action == "start":
			c.State.Running = true
		case action == "rename":
			c.Name = "/" + r.URL.Query().Get("name")
		case r.Method == "DELETE":
			if f.failDelete {
				status = 500
			} else {
				delete(f.containers, c.ID)
			}
		}
	default:
		return nil, errors.New("unexpected fixture Docker request: " + key)
	}
	body, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
}

func hostFixture(t *testing.T) (*Engine, *fixtureDocker, Payload) {
	t.Helper()
	e := &Engine{Root: t.TempDir(), StateDir: t.TempDir()}
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	c := Container{ID: strings.Repeat("d", 64), Name: "fixture-app", Module: "apps", ImageID: "sha256:" + strings.Repeat("a", 64), Config: map[string]json.RawMessage{"Image": raw("example/app:old"), "Labels": raw(map[string]string{}), "Env": raw([]string{"PRIVATE=test-secret"})}, HostConfig: map[string]json.RawMessage{"Binds": raw([]string{"/home/app:/data:rw"})}, Networks: map[string]json.RawMessage{}, Mounts: []Mount{{Type: "bind", Source: "/home/app", Destination: "/data", RW: true}}}
	hash := sha256.Sum256([]byte("/home/app"))
	root := Root{ID: hex.EncodeToString(hash[:16]), Path: "/home/app", Module: "apps", Directory: true, Bytes: 4, Entries: 2}
	if err := os.MkdirAll(e.host(root.Path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.host("/home/app/data"), []byte("old!"), 0640); err != nil {
		t.Fatal(err)
	}
	existing := &fixtureContainer{ID: c.ID, Name: "/" + c.Name, Image: c.ImageID, Config: c.Config, HostConfig: c.HostConfig, Mounts: c.Mounts}
	existing.NetworkSettings.Networks = c.Networks
	d := &fixtureDocker{containers: map[string]*fixtureContainer{c.ID: existing}}
	e.Client = &http.Client{Transport: fixtureTransport(d.request)}
	return e, d, Payload{Version: 1, Module: "apps", Roots: []Root{root}, Containers: []Container{c}, Networks: map[string]json.RawMessage{}, Volumes: map[string]Volume{}}
}

func TestBackupHostRestoreAndRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "restore", true: "rollback"}[fail], func(t *testing.T) {
			e, d, p := hostFixture(t)
			directory := t.TempDir()
			_ = os.WriteFile(e.host("/home/app/data"), []byte("new!"), 0640)
			if err := e.writePayload(context.Background(), filepath.Join(directory, "apps.payload"), p); err != nil {
				t.Fatal(err)
			}
			_ = os.WriteFile(e.host("/home/app/data"), []byte("old!"), 0640)
			d.failCreate = fail
			err := e.Restore(context.Background(), backup.NewID(), directory, []string{"apps"})
			if (err != nil) != fail {
				t.Fatal(err)
			}
			data, err := os.ReadFile(e.host("/home/app/data"))
			if err != nil {
				t.Fatal(err)
			}
			want := "new!"
			if fail {
				want = "old!"
			}
			if string(data) != want {
				t.Fatal("wrong restored data", string(data))
			}
			if len(d.containers) != 1 {
				t.Fatal("containers not cleaned or restored")
			}
		})
	}
}

func TestBackupHostNativeMountBindingAndRevision(t *testing.T) {
	e, _, p := hostFixture(t)
	before, err := e.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(e.host("/home/app/log"), []byte("new log data"), 0600)
	after, err := e.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if before.Revision != after.Revision {
		t.Fatal("ordinary data growth invalidates preview")
	}
	p.Containers[0].Mounts = nil
	if err := e.validatePayload(p); err == nil {
		t.Fatal("actual mount bypassed validation")
	}
}

func TestBackupHostRejectsOversizedGzipTail(t *testing.T) {
	e, _, p := hostFixture(t)
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	metadata, _ := json.Marshal(p)
	_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Typeflag: tar.TypeReg, Mode: 0600, Size: int64(len(metadata))})
	_, _ = tw.Write(metadata)
	_ = tw.Close()
	_, _ = gz.Write(make([]byte, (1<<20)+1))
	_ = gz.Close()
	path := filepath.Join(t.TempDir(), "bad.payload")
	_ = os.WriteFile(path, out.Bytes(), 0600)
	if _, err := e.ReadPayload(context.Background(), path, t.TempDir()); err == nil {
		t.Fatal("accepted unbounded compressed padding")
	}
}

func TestBackupHostRollbackDoesNotTouchLiveWriterData(t *testing.T) {
	e, d, _ := hostFixture(t)
	d.failDelete = true
	target := e.host("/home/app")
	previous := target + ".kpanel-restore-" + backup.NewID()
	_ = os.Mkdir(previous, 0700)
	j := restoreJournal{Containers: []restoreContainer{{NewID: strings.Repeat("d", 64)}}, Roots: []restoreRoot{{Target: target, Previous: previous, HadPrevious: true, Applied: true}}}
	if err := e.rollback(context.Background(), &j); err == nil {
		t.Fatal("delete failure ignored")
	}
	if data, err := os.ReadFile(filepath.Join(target, "data")); err != nil || string(data) != "old!" {
		t.Fatal("live data was replaced")
	}
}

func TestBackupNativeAnonymousVolumesAndDependencies(t *testing.T) {
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	for _, host := range []map[string]json.RawMessage{{"Binds": raw([]string{"/data"})}, {"Mounts": raw([]map[string]any{{"Type": "volume", "Target": "/data"}})}} {
		c := Container{Mounts: []Mount{{Type: "volume", Name: "resolved-anonymous", Source: "/var/lib/docker/volumes/resolved-anonymous/_data", Destination: "/data", RW: true}}, HostConfig: host}
		cfg, err := nativeMountConfig(c)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(cfg)
		if !bytes.Contains(body, []byte("resolved-anonymous")) {
			t.Fatal("anonymous volume was not pinned")
		}
	}
	db := Container{Name: "db", ID: strings.Repeat("b", 64), HostConfig: map[string]json.RawMessage{}}
	sidecar := Container{Name: "sidecar", HostConfig: map[string]json.RawMessage{"NetworkMode": raw("container:" + db.ID)}}
	chat := Container{Name: "chat", HostConfig: map[string]json.RawMessage{"Links": raw([]string{"/db:/chat/db"})}}
	all := []Container{sidecar, chat, db}
	for n := range all {
		canonicalContainerReferences(&all[n], all)
	}
	p := Payload{Containers: all}
	if err := orderContainers(&p); err != nil {
		t.Fatal(err)
	}
	if p.Containers[0].Name != "db" {
		t.Fatal("dependency not first")
	}
	p.Containers = p.Containers[1:]
	if err := orderContainers(&p); err == nil {
		t.Fatal("missing dependency accepted")
	}
	if !excluded("/var/run/docker.sock") {
		t.Fatal("runtime socket treated as data")
	}
}
func TestBackupHostInterruptedJournalRecovery(t *testing.T) {
	e, _, _ := hostFixture(t)
	id := backup.NewID()
	dir := filepath.Join(t.TempDir(), id)
	if err := backup.PrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	target := e.host("/home/app")
	previous := target + ".kpanel-restore-" + id
	next := target + ".kpanel-next-" + id
	if err := os.Rename(target, previous); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(target, "data"), []byte("uncommitted"), 0600)
	j := restoreJournal{ID: id, Phase: "applying", Roots: []restoreRoot{{Target: target, Previous: previous, Next: next, HadPrevious: true, Applied: true}}}
	if err := backup.WriteJSON(filepath.Join(dir, "restore-journal.json"), j); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 2; n++ {
		if err := e.Recover(context.Background(), id, dir); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(target, "data"))
	if err != nil || string(data) != "old!" {
		t.Fatal("interrupted transaction changed original data", err)
	}
}
