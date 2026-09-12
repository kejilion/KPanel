package hostbackup

import (
	"encoding/json"
	"path"
	"slices"
	"strings"
)

// Identity detection is one-way: a generic / or /run mount is not Panel.
// Once identity is established, intersect mounts both ways to find a custom
// parent bind which contains the configured Panel data/secret destination.
func runtimeProtection(c Container) (bool, []string) {
	protected := c.Name == "kejilion-panel" || c.Name == "kejilion-agent"
	sources := []string{"/var/lib/kejilion-panel", "/etc/kejilion-panel", "/home/docker/kpanel"}
	destinations := []string{"/var/lib/kejilion-panel", "/etc/kejilion-panel", "/run/kejilion-panel", "/run/secrets/agent-token"}
	var env []string
	_ = json.Unmarshal(c.Config["Env"], &env)
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch key {
		case "KEJILION_PANEL_DATA_DIR", "KEJILION_PANEL_AGENT_SOCKET", "KEJILION_PANEL_AGENT_TOKEN_FILE":
			protected = true
			if path.IsAbs(value) && path.Clean(value) == value {
				destinations = append(destinations, value)
			}
		}
	}
	var entrypoint, cmd []string
	_ = json.Unmarshal(c.Config["Entrypoint"], &entrypoint)
	_ = json.Unmarshal(c.Config["Cmd"], &cmd)
	for _, command := range [][]string{entrypoint, cmd} {
		if len(command) > 0 && (path.Base(command[0]) == "paneld" || path.Base(command[0]) == "kejilion-agent") {
			protected = true
		}
	}
	for _, m := range c.Mounts {
		if m.Type != "bind" && m.Type != "volume" {
			continue
		}
		for _, root := range sources {
			if within(root, m.Source) {
				protected = true
			}
		}
		for _, root := range destinations {
			if within(root, m.Destination) {
				protected = true
			}
		}
	}
	if !protected {
		return false, nil
	}
	paths := []string{}
	for _, m := range c.Mounts {
		if (m.Type != "bind" && m.Type != "volume") || !path.IsAbs(m.Source) || path.Clean(m.Source) != m.Source {
			continue
		}
		keep := m.Destination == "/paneld" || protectionOverlap("/app/web", m.Destination)
		for _, root := range sources {
			if within(root, m.Source) {
				keep = true
			}
		}
		for _, root := range destinations {
			if protectionOverlap(root, m.Destination) {
				keep = true
			}
		}
		if keep {
			paths = append(paths, m.Source)
		}
	}
	slices.Sort(paths)
	return true, slices.Compact(paths)
}
func protectionOverlap(a, b string) bool {
	return path.IsAbs(a) && path.IsAbs(b) && (within(a, b) || within(b, a))
}
func protectionRootOverlap(roots []string, candidate string) bool {
	for _, root := range roots {
		if protectionOverlap(root, candidate) {
			return true
		}
	}
	return false
}
