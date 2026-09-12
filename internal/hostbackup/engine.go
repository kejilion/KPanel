// Package hostbackup is the shared, fixed backup adapter used by kejilion.sh
// and the Agent. It preserves script directories and Docker's native config;
// it never executes code from a backup or accepts shell command text.
package hostbackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/backup"
)

const ProtocolVersion = 1

type Engine struct {
	Root     string
	StateDir string
	Client   *http.Client
}
type Mount struct {
	Type        string
	Name        string
	Source      string
	Destination string
	RW          bool
}
type Container struct {
	ImageID    string                     `json:"imageId"`
	ImageRef   string                     `json:"imageRef"`
	ID         string                     `json:"id"`
	Name       string                     `json:"name"`
	Module     string                     `json:"module"`
	Running    bool                       `json:"running"`
	Config     map[string]json.RawMessage `json:"config"`
	HostConfig map[string]json.RawMessage `json:"hostConfig"`
	Networks   map[string]json.RawMessage `json:"networks"`
	Mounts     []Mount                    `json:"mounts"`
}
type Root struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	Module    string `json:"module"`
	Directory bool   `json:"directory"`
	Bytes     int64  `json:"bytes"`
	Entries   int    `json:"entries"`
}
type Inventory struct {
	ProtectedContainers []Container                `json:"-"`
	ProtectedRoots      []string                   `json:"-"`
	Volumes             map[string]Volume          `json:"volumes"`
	Version             int                        `json:"version"`
	Revision            string                     `json:"revision"`
	Roots               []Root                     `json:"roots"`
	Containers          []Container                `json:"containers"`
	Networks            map[string]json.RawMessage `json:"networks"`
	Modules             []Module                   `json:"modules"`
}
type Module struct {
	Issue      string   `json:"issue,omitempty"`
	ID         string   `json:"id"`
	Bytes      int64    `json:"bytes"`
	Entries    int      `json:"entries"`
	Containers int      `json:"containers"`
	Requires   []string `json:"requires"`
}
type Payload struct {
	Volumes    map[string]Volume          `json:"volumes"`
	Version    int                        `json:"version"`
	Module     string                     `json:"module"`
	Roots      []Root                     `json:"roots"`
	Containers []Container                `json:"containers"`
	Networks   map[string]json.RawMessage `json:"networks"`
}

func New(stateDir string) *Engine {
	return NewWithSocket(stateDir, "/var/run/docker.sock")
}
func NewWithSocket(stateDir, socket string) *Engine {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{DisableCompression: true, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, "unix", socket)
	}}
	return &Engine{Root: "/", StateDir: stateDir, Client: &http.Client{Transport: transport, Timeout: 10 * time.Minute}}
}

func (e *Engine) excluded(virtual string) bool {
	if excluded(virtual) {
		return true
	}
	actual := filepath.ToSlash(filepath.Clean(e.host(virtual)))
	state := filepath.ToSlash(filepath.Clean(e.StateDir))
	return within(state, actual) || within(actual, state)
}
func (e *Engine) host(path string) string {
	return filepath.Join(e.Root, filepath.FromSlash(strings.TrimPrefix(path, "/")))
}
func (e *Engine) docker(ctx context.Context, method, path string, input, output any) error {
	var reader io.Reader
	if input != nil {
		body, err := json.Marshal(input)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := e.Client.Do(req)
	if err != nil {
		return errors.New("Docker Engine is unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		if method == "DELETE" {
			return nil
		}
		return os.ErrNotExist
	}
	if response.StatusCode == http.StatusNotModified && method == "POST" && (strings.HasSuffix(path, "/start") || strings.HasSuffix(path, "/stop") || strings.Contains(path, "/stop?")) {
		return nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Docker %s returned HTTP %d", method, response.StatusCode)
	}
	if output == nil {
		_, err := io.Copy(io.Discard, io.LimitReader(response.Body, 32<<20))
		return err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil || len(body) > 16<<20 {
		return backup.ErrInvalid
	}
	return json.Unmarshal(body, output)
}

func within(parent, path string) bool {
	return path == parent || strings.HasPrefix(path, strings.TrimSuffix(parent, "/")+"/")
}
func excluded(path string) bool {
	for _, root := range []string{"/proc", "/sys", "/dev", "/run", "/var/run", "/etc/kejilion-panel", "/var/lib/kejilion-panel", "/home/docker/kpanel"} {
		if within(root, path) || within(path, root) {
			return true
		}
	}
	for _, part := range strings.Split(path, "/") {
		if part == ".kpanel-backups" || strings.Contains(part, ".kpanel-restore-") || strings.Contains(part, ".kpanel-next-") || strings.HasPrefix(part, ".kpanel-backup-") {
			return true
		}
	}
	return false
}
func owner(path string) string {
	if within("/home/web", path) {
		return "web"
	}
	if within("/home", path) {
		return "apps"
	}
	return "docker"
}

func (e *Engine) Inventory(ctx context.Context) (Inventory, error) {
	i := Inventory{Version: 1, Roots: []Root{}, Containers: []Container{}, Networks: map[string]json.RawMessage{}, Volumes: map[string]Volume{}}
	issues := map[string]string{}
	var info struct{ SecurityOptions []string }
	if err := e.docker(ctx, "GET", "/info", nil, &info); err != nil {
		return i, err
	}
	for _, option := range info.SecurityOptions {
		if strings.Contains(option, "name=userns") || strings.Contains(option, "name=rootless") {
			for _, m := range []string{"apps", "web", "docker"} {
				issues[m] = "user_namespace_requires_adapter"
			}
		}
	}
	candidates := map[string]string{}
	home, err := os.ReadDir(e.host("/home"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return i, err
	}
	for _, entry := range home {
		path := "/home/" + entry.Name()
		if entry.Name() == "docker" {
			sub, err := os.ReadDir(e.host(path))
			if err != nil {
				return i, err
			}
			for _, child := range sub {
				p := path + "/" + child.Name()
				if !e.excluded(p) {
					candidates[p] = "apps"
				}
			}
			continue
		}
		if strings.HasPrefix(entry.Name(), "web_") && strings.Contains(entry.Name(), ".tar.gz") {
			continue
		}
		if !e.excluded(path) {
			candidates[path] = owner(path)
		}
	}
	var listed []struct {
		ID string `json:"Id"`
	}
	if err := e.docker(ctx, "GET", "/containers/json?all=1", nil, &listed); err != nil {
		return i, err
	}
	if len(listed) > 512 {
		return i, errors.New("backup supports at most 512 containers")
	}
	for _, item := range listed {
		var raw struct {
			ID         string `json:"Id"`
			Image      string
			Name       string
			Config     map[string]json.RawMessage
			HostConfig map[string]json.RawMessage
			State      struct {
				Running    bool
				Paused     bool
				Restarting bool
			}
			Mounts          []Mount
			NetworkSettings struct{ Networks map[string]json.RawMessage }
		}
		if err := e.docker(ctx, "GET", "/containers/"+url.PathEscape(item.ID)+"/json", nil, &raw); err != nil {
			return i, err
		}
		var labels map[string]string
		json.Unmarshal(raw.Config["Labels"], &labels)
		if labels["io.kpanel.backup.previous"] != "" || strings.HasPrefix(strings.TrimPrefix(raw.Name, "/"), "kpanel-previous-") {
			continue
		}
		c := Container{ID: raw.ID, Name: strings.TrimPrefix(raw.Name, "/"), Module: "docker", Running: raw.State.Running, Config: raw.Config, HostConfig: raw.HostConfig, Networks: raw.NetworkSettings.Networks, Mounts: raw.Mounts}
		c.ImageID = raw.Image
		if protected, roots := runtimeProtection(c); protected {
			i.ProtectedContainers = append(i.ProtectedContainers, c)
			i.ProtectedRoots = append(i.ProtectedRoots, roots...)
			continue
		}
		for _, mount := range c.Mounts {
			if mount.Type == "tmpfs" {
				continue
			}
			if mount.Type != "bind" && mount.Type != "volume" {
				return i, fmt.Errorf("container %s uses an unsupported mount type", c.Name)
			}
			if e.excluded(mount.Source) {
				continue // Preserve runtime bindings without archiving their live data.
			}
			if owner(mount.Source) == "web" {
				c.Module = "web"
			} else if owner(mount.Source) == "apps" && c.Module != "web" {
				c.Module = "apps"
			}
			candidates[mount.Source] = owner(mount.Source)
		}
		if work := labels["com.docker.compose.project.working_dir"]; work != "" {
			if !path.IsAbs(work) || e.excluded(work) {
				return i, fmt.Errorf("invalid Compose directory for %s", c.Name)
			}
			candidates[work] = owner(work)
			if owner(work) == "web" {
				c.Module = "web"
			} else if owner(work) == "apps" && c.Module != "web" {
				c.Module = "apps"
			}
			for _, file := range strings.Split(labels["com.docker.compose.project.config_files"], ",") {
				if file != "" && !within(work, file) {
					if e.excluded(file) || !path.IsAbs(file) {
						return i, backup.ErrInvalid
					}
					candidates[file] = owner(file)
				}
			}
		}
		var autoRemove bool
		_ = json.Unmarshal(c.HostConfig["AutoRemove"], &autoRemove)
		if autoRemove && c.Running {
			issues[c.Module] = "auto_remove_requires_stop"
		}
		if raw.State.Paused || raw.State.Restarting {
			issues[c.Module] = "container_not_stable"
		}
		for _, mount := range c.Mounts {
			if mount.Type == "volume" {
				var v Volume
				if err := e.docker(ctx, "GET", "/volumes/"+url.PathEscape(mount.Name), nil, &v); err != nil {
					return i, err
				}
				i.Volumes[v.Name] = v
				if v.Driver != "local" || len(v.Options) > 0 {
					issues[c.Module] = "volume_driver_requires_external_backup"
				}
				if candidates[mount.Source] == "docker" {
					candidates[mount.Source] = c.Module
				}
			}
		}
		var img struct{ RepoDigests []string }
		if err := e.docker(ctx, "GET", "/images/"+url.PathEscape(c.ImageID)+"/json", nil, &img); err != nil {
			return i, err
		}
		if len(img.RepoDigests) > 0 {
			c.ImageRef = img.RepoDigests[0]
		}
		i.Containers = append(i.Containers, c)
		for network := range c.Networks {
			if _, ok := i.Networks[network]; ok {
				continue
			}
			var value json.RawMessage
			if err := e.docker(ctx, "GET", "/networks/"+url.PathEscape(network), nil, &value); err != nil {
				return i, err
			}
			i.Networks[network] = networkConfiguration(value)
		}
	}
	// Protected data can be a custom /home directory or a named volume. Filter
	// after all containers are inspected so enumeration order cannot leak it.
	for candidate, module := range candidates {
		for _, root := range i.ProtectedRoots {
			if !protectionOverlap(root, candidate) {
				continue
			}
			delete(candidates, candidate)
			if candidate != root && within(candidate, root) {
				issues[module] = "protected_panel_data"
			}
		}
	}
	for _, c := range i.Containers {
		for _, mount := range c.Mounts {
			if protectionRootOverlap(i.ProtectedRoots, mount.Source) {
				issues[c.Module] = "protected_panel_data"
			}
		}
	}
	paths := make([]string, 0, len(candidates))
	for path := range candidates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, rootPath := range paths {
		path := rootPath
		covered := false
		for _, root := range i.Roots {
			if within(root.Path, path) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		if !strings.HasPrefix(path, "/") || filepath.ToSlash(filepath.Clean(path)) != path || e.excluded(path) {
			return i, backup.ErrInvalid
		}
		info, err := os.Lstat(e.host(path))
		if err != nil {
			return i, err
		}
		hash := sha256.Sum256([]byte(path))
		r := Root{ID: hex.EncodeToString(hash[:16]), Path: path, Module: candidates[path], Directory: info.IsDir()}
		if err := e.measure(ctx, &r); err != nil {
			issues[r.Module] = "data_path_cannot_be_archived"
		}
		i.Roots = append(i.Roots, r)
	}
	for n := range i.Containers {
		canonicalContainerReferences(&i.Containers[n], i.Containers)
	}
	for _, module := range []string{"apps", "web", "docker"} {
		m := Module{ID: module, Requires: []string{}, Issue: issues[module]}
		for _, root := range i.Roots {
			if root.Module == module {
				m.Bytes += root.Bytes
				m.Entries += root.Entries
			}
		}
		for _, c := range i.Containers {
			// Selecting data also selects every container that can write it.
			for _, mount := range c.Mounts {
				for _, r := range i.Roots {
					if r.Module == module && (within(r.Path, mount.Source) || within(mount.Source, r.Path)) && c.Module != module && !slices.Contains(m.Requires, c.Module) {
						m.Requires = append(m.Requires, c.Module)
					}
				}
			}
			if c.Module != module {
				continue
			}
			m.Containers++
			for _, mount := range c.Mounts {
				for _, r := range i.Roots {
					if (within(r.Path, mount.Source) || within(mount.Source, r.Path)) && r.Module != module && !slices.Contains(m.Requires, r.Module) {
						m.Requires = append(m.Requires, r.Module)
					}
				}
			}
		}
		for _, c := range i.Containers {
			if c.Module != module {
				continue
			}
			for _, ref := range containerDependencies(c) {
				found := false
				for _, peer := range i.Containers {
					if peer.Name == ref || peer.ID == ref {
						found = true
						if peer.Module != module && !slices.Contains(m.Requires, peer.Module) {
							m.Requires = append(m.Requires, peer.Module)
						}
					}
				}
				if !found {
					m.Issue = "container_dependency_missing"
				}
			}
		}
		i.Modules = append(i.Modules, m)
	}
	// Content naturally changes while services run. The preview revision binds
	// configuration and identities, not log sizes or runtime endpoint addresses.
	for n := range i.Containers {
		canonicalContainerReferences(&i.Containers[n], i.Containers)
	}
	sort.Slice(i.Containers, func(a, b int) bool { return i.Containers[a].Name < i.Containers[b].Name })
	topology := i
	topology.Modules = nil
	topology.Roots = append([]Root(nil), i.Roots...)
	for n := range topology.Roots {
		topology.Roots[n].Bytes = 0
		topology.Roots[n].Entries = 0
	}
	body, _ := json.Marshal(topology)
	digest := sha256.Sum256(body)
	i.Revision = hex.EncodeToString(digest[:])
	return i, nil
}

func (e *Engine) measure(ctx context.Context, root *Root) error {
	if err := noNestedMounts(e.host(root.Path)); err != nil {
		return err
	}
	return filepath.WalkDir(e.host(root.Path), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("backup contains a link or special file: %s", root.Path)
		}
		root.Entries++
		if info.Mode().IsRegular() {
			root.Bytes += info.Size()
		}
		if root.Entries > 100000 || root.Bytes > backup.MaxBytes || info.Mode().IsRegular() && info.Size() > 10<<30 {
			return errors.New("backup file or archive limit exceeded")
		}
		return nil
	})
}

func validateSelection(i Inventory, modules []string) error {
	if _, err := backup.Selection(modules); err != nil {
		return err
	}
	for _, m := range i.Modules {
		if !slices.Contains(modules, m.ID) {
			continue
		}
		if m.Issue != "" {
			return fmt.Errorf("%s: %s", m.ID, m.Issue)
		}
		for _, required := range m.Requires {
			if !slices.Contains(modules, required) {
				return fmt.Errorf("%s shares data with %s; select both modules", m.ID, required)
			}
		}
	}
	return nil
}
