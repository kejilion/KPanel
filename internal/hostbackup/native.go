package hostbackup

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/backup"
)

type Volume struct {
	Name       string
	Driver     string
	Mountpoint string
	Labels     map[string]string
	Options    map[string]string
}

var imageIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func networkConfiguration(raw json.RawMessage) json.RawMessage {
	var old map[string]json.RawMessage
	_ = json.Unmarshal(raw, &old)
	clean := map[string]json.RawMessage{}
	for _, key := range []string{"Name", "Driver", "Internal", "Attachable", "Ingress", "EnableIPv4", "EnableIPv6", "IPAM", "Options", "Labels"} {
		if v, ok := old[key]; ok {
			clean[key] = v
		}
	}
	var labels map[string]string
	if json.Unmarshal(clean["Labels"], &labels) == nil {
		delete(labels, "io.kpanel.restore.job")
		if len(labels) == 0 {
			clean["Labels"] = json.RawMessage(`{}`)
		} else {
			clean["Labels"], _ = json.Marshal(labels)
		}
	}
	out, _ := json.Marshal(clean)
	return out
}
func canonicalContainerReferences(c *Container, all []Container) {
	sort.Slice(c.Mounts, func(i, j int) bool { return c.Mounts[i].Destination < c.Mounts[j].Destination })
	var links []string
	if json.Unmarshal(c.HostConfig["Links"], &links) == nil {
		for n, link := range links {
			parts := strings.Split(link, ":")
			if len(parts) == 2 {
				links[n] = strings.TrimPrefix(parts[0], "/") + ":" + path.Base(parts[1])
			}
		}
		sort.Strings(links)
		c.HostConfig["Links"], _ = json.Marshal(links)
	}
	for _, key := range []string{"NetworkMode", "PidMode", "IpcMode"} {
		var value string
		_ = json.Unmarshal(c.HostConfig[key], &value)
		if strings.HasPrefix(value, "container:") {
			ref := strings.TrimPrefix(value, "container:")
			for _, peer := range all {
				if ref == peer.ID || ref == peer.Name {
					c.HostConfig[key], _ = json.Marshal("container:" + peer.Name)
				}
			}
		}
	}
	var mode string
	_ = json.Unmarshal(c.HostConfig["NetworkMode"], &mode)
	for name, raw := range c.Networks {
		var endpoint struct{ NetworkID string }
		if json.Unmarshal(raw, &endpoint) == nil && endpoint.NetworkID != "" && mode == endpoint.NetworkID {
			c.HostConfig["NetworkMode"], _ = json.Marshal(name)
		}
	}
	for name, raw := range c.Networks {
		var old map[string]json.RawMessage
		_ = json.Unmarshal(raw, &old)
		endpoint := map[string]json.RawMessage{}
		for _, key := range []string{"Aliases", "Links", "IPAMConfig", "DriverOpts", "GwPriority"} {
			if v, ok := old[key]; ok {
				endpoint[key] = v
			}
		}
		c.Networks[name], _ = json.Marshal(endpoint)
	}
}

// nativeMountConfig validates the actual Docker mount inputs against inspect's
// resolved mounts. It pins anonymous volumes by name and resolves VolumesFrom
// into explicit mounts so no stale container IDs can redirect restored data.
func nativeMountConfig(c Container) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	for k, v := range c.HostConfig {
		out[k] = v
	}
	var binds []string
	var mounts []map[string]json.RawMessage
	if raw := out["Binds"]; len(raw) > 0 && json.Unmarshal(raw, &binds) != nil {
		return nil, backup.ErrInvalid
	}
	if raw := out["Mounts"]; len(raw) > 0 && json.Unmarshal(raw, &mounts) != nil {
		return nil, backup.ErrInvalid
	}
	known := map[string]Mount{}
	for _, m := range c.Mounts {
		if !path.IsAbs(m.Destination) || known[m.Destination].Destination != "" {
			return nil, backup.ErrInvalid
		}
		known[m.Destination] = m
	}
	seen := map[string]bool{}
	check := func(kind, source, target string) bool {
		m, ok := known[target]
		if !ok || seen[target] || m.Type != kind {
			return false
		}
		expected := m.Source
		if kind == "volume" {
			expected = m.Name
		}
		if expected != source {
			return false
		}
		seen[target] = true
		return true
	}
	for n, b := range binds {
		parts := strings.Split(b, ":")
		if len(parts) == 1 {
			m, ok := known[b]
			if !ok || m.Type != "volume" || m.Name == "" {
				return nil, backup.ErrInvalid
			}
			mode := "rw"
			if !m.RW {
				mode = "ro"
			}
			binds[n] = m.Name + ":" + b + ":" + mode
			parts = strings.Split(binds[n], ":")
		}
		if len(parts) < 2 || len(parts) > 3 {
			return nil, backup.ErrInvalid
		}
		source, target := parts[0], parts[1]
		kind := "volume"
		if path.IsAbs(source) {
			kind = "bind"
		}
		if !check(kind, source, target) {
			return nil, backup.ErrInvalid
		}
	}
	for _, m := range mounts {
		var kind, source, target string
		_ = json.Unmarshal(m["Type"], &kind)
		_ = json.Unmarshal(m["Source"], &source)
		_ = json.Unmarshal(m["Target"], &target)
		if kind == "tmpfs" {
			continue
		}
		if kind == "volume" && source == "" {
			resolved, ok := known[target]
			if !ok || resolved.Type != "volume" || resolved.Name == "" {
				return nil, backup.ErrInvalid
			}
			source = resolved.Name
			m["Source"], _ = json.Marshal(source)
		}
		if !check(kind, source, target) {
			return nil, backup.ErrInvalid
		}
	}
	for _, m := range c.Mounts {
		if seen[m.Destination] || m.Type == "tmpfs" {
			continue
		}
		source := m.Source
		if m.Type == "volume" {
			source = m.Name
		}
		raw, _ := json.Marshal(map[string]any{"Type": m.Type, "Source": source, "Target": m.Destination, "ReadOnly": !m.RW})
		var v map[string]json.RawMessage
		_ = json.Unmarshal(raw, &v)
		mounts = append(mounts, v)
	}
	out["Binds"], _ = json.Marshal(binds)
	out["Mounts"], _ = json.Marshal(mounts)
	delete(out, "VolumesFrom")
	return out, nil
}

func (e *Engine) prepareImages(ctx context.Context, containers []Container) error {
	seen := map[string]bool{}
	for _, c := range containers {
		if !imageIDPattern.MatchString(c.ImageID) {
			return backup.ErrInvalid
		}
		if seen[c.ImageID] {
			continue
		}
		seen[c.ImageID] = true
		var image struct {
			ID string `json:"Id"`
		}
		if err := e.docker(ctx, "GET", "/images/"+url.PathEscape(c.ImageID)+"/json", nil, &image); err == nil && image.ID == c.ImageID {
			continue
		}
		if !strings.Contains(c.ImageRef, "@sha256:") {
			return errors.New("exact local image is missing; load the source image before restore")
		}
		if err := e.docker(ctx, "POST", "/images/create?fromImage="+url.QueryEscape(c.ImageRef), nil, nil); err != nil {
			return err
		}
		if err := e.docker(ctx, "GET", "/images/"+url.PathEscape(c.ImageID)+"/json", nil, &image); err != nil || image.ID != c.ImageID {
			return errors.New("exact backup image could not be loaded")
		}
	}
	return nil
}

func (e *Engine) prepareVolumes(ctx context.Context, p *Payload) error {
	for name, old := range p.Volumes {
		if !validContainerName(name) || old.Name != name || old.Driver != "local" || len(old.Options) > 0 {
			return errors.New("volume driver needs an external snapshot adapter")
		}
		var v Volume
		if err := e.docker(ctx, "GET", "/volumes/"+url.PathEscape(name), nil, &v); err != nil {
			if err := e.docker(ctx, "POST", "/volumes/create", map[string]any{"Name": name, "Driver": old.Driver, "Labels": old.Labels}, &v); err != nil {
				return err
			}
		}
		if v.Name != name || v.Driver != old.Driver || len(v.Options) > 0 || !path.IsAbs(v.Mountpoint) || e.excluded(v.Mountpoint) {
			return backup.ErrInvalid
		}
		for n, r := range p.Roots {
			if r.Path == old.Mountpoint {
				p.Roots[n].Path = v.Mountpoint
			}
		}
		for n := range p.Containers {
			for k, m := range p.Containers[n].Mounts {
				if m.Type == "volume" && m.Name == name {
					p.Containers[n].Mounts[k].Source = v.Mountpoint
				}
			}
		}
	}
	return nil
}
