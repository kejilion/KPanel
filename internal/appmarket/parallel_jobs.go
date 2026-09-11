package appmarket

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
)

const maxConcurrentAppJobs = 4

// Fixed additional host bindings in the paired native script. Reserve them
// before docker run, while a new installation is still pulling its image.
var appFixedHostPorts = map[string][]uint16{
	"builtin-4":   {80, 443},
	"builtin-8":   {56881},
	"builtin-17":  {53},
	"builtin-69":  {22022},
	"builtin-70":  {6195, 6196, 6199, 11451},
	"builtin-86":  {7359},
	"builtin-88":  {1935},
	"builtin-100": {22000, 21027},
}

var ErrTaskLimit = errors.New("application concurrency limit reached")
var parallelAppProtocol = regexp.MustCompile(`(?m)^KPANEL_APP_CONCURRENCY_PROTOCOL_VERSION="1"\r?$`)

// Compatibility is recorded before admission and checked again by the PTY
// worker. Jobs started with an older script remain exclusive until they exit.
func (s *Service) parallelAppScript(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 ||
		(runtime.GOOS == "linux" && (!s.fileOwnerTrusted(info) || info.Mode().Perm()&0o022 != 0)) {
		return false
	}
	content, err := io.ReadAll(io.LimitReader(file, (4<<20)+1))
	return err == nil && len(content) <= 4<<20 && parallelAppProtocol.Match(content)
}

func (s *Service) appJobResourceKeys(item Summary) []string {
	keys := []string{"app:" + item.ID}
	for _, port := range appFixedHostPorts[item.ID] {
		keys = append(keys, fmt.Sprintf("port:%d", port))
	}
	legacy := s.legacy[item.Num]
	container, service := legacy.Container, legacy.Service
	if item.Source == "thirdparty" {
		if spec, err := s.readThirdPartyScriptSpec(item.Token); err == nil {
			container, service = spec.Container, spec.Service
		}
	}
	for _, port := range item.Runtime.Ports {
		if port.PublicPort != 0 {
			keys = append(keys, fmt.Sprintf("port:%d", port.PublicPort))
		}
	}
	for _, name := range []string{container, service, item.Runtime.ContainerName} {
		if name != "" {
			keys = append(keys, "container:"+name)
		}
	}
	// These native script applications share data despite distinct selectors,
	// container names and ports. Keep their existing directory layouts intact.
	switch item.ID {
	case "builtin-1", "builtin-2":
		keys = append(keys, "data:bt-panel")
	case "builtin-6", "builtin-24":
		keys = append(keys, "data:webtop")
	case "builtin-55", "builtin-56":
		keys = append(keys, "data:frp")
	}
	return keys
}

// Callers hold Service.actions through admission, persistence and launch so
// simultaneous HTTP requests cannot consume the same slot or reserved port.
func (registry *appJobRegistry) canStart(candidate appJobRecord) error {
	registry.mu.Lock()
	ids := make([]string, 0, len(registry.jobs))
	for id := range registry.jobs {
		ids = append(ids, id)
	}
	registry.mu.Unlock()
	active := 0
	for _, id := range ids {
		record, err := registry.read(id)
		if err != nil {
			return fmt.Errorf("%w: application task state is unavailable", ErrNeedsAttention)
		}
		if record.Status != "queued" && record.Status != "running" {
			continue
		}
		active++
		if !candidate.ParallelSafe || !record.ParallelSafe ||
			candidate.AppID == record.AppID ||
			(candidate.Selector != "" && candidate.Selector == record.Selector) ||
			(candidate.ExpectedContainerID != "" && candidate.ExpectedContainerID == record.ExpectedContainerID) {
			return ErrTaskConflict
		}
		for _, key := range candidate.ResourceKeys {
			if slices.Contains(record.ResourceKeys, key) {
				if strings.HasPrefix(key, "port:") {
					return ErrPortConflict
				}
				return ErrTaskConflict
			}
		}
		if candidate.HostPort != 0 && candidate.HostPort == record.HostPort {
			return ErrPortConflict
		}
	}
	if active >= maxConcurrentAppJobs {
		return ErrTaskLimit
	}
	return nil
}
