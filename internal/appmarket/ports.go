package appmarket

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type PortConflict struct {
	Source    string `json:"source"`
	Protocol  string `json:"protocol"`
	Container string `json:"container,omitempty"`
}

type InstallPortStatus struct {
	Port      uint16         `json:"port"`
	Available bool           `json:"available"`
	Conflicts []PortConflict `json:"conflicts"`
	CheckedAt time.Time      `json:"checkedAt"`
}

func (s *Service) CheckInstallPort(
	ctx context.Context,
	id string,
	port uint16,
) (InstallPortStatus, error) {
	item, err := s.Find(ctx, id)
	if err != nil {
		return InstallPortStatus{}, err
	}
	if !item.InstallPortConfigurable {
		return InstallPortStatus{}, fmt.Errorf(
			"%w: this application does not expose a single configurable install port",
			ErrUnsupported,
		)
	}
	if port == 0 {
		if item.DefaultPort < 1 || item.DefaultPort > 65535 {
			return InstallPortStatus{}, fmt.Errorf("%w: application port is invalid", ErrForbidden)
		}
		port = uint16(item.DefaultPort)
	}
	return s.inspectInstallPort(ctx, port)
}

func (s *Service) inspectInstallPort(
	ctx context.Context,
	port uint16,
) (InstallPortStatus, error) {
	if port == 0 {
		return InstallPortStatus{}, fmt.Errorf("%w: application port is invalid", ErrForbidden)
	}
	containers, err := s.docker.Containers(ctx)
	if err != nil {
		return InstallPortStatus{}, fmt.Errorf("inspect Docker port bindings: %w", err)
	}
	conflicts := make([]PortConflict, 0)
	seen := make(map[string]bool)
	for _, container := range containers {
		for _, binding := range container.Ports {
			if binding.PublicPort != port {
				continue
			}
			protocol := strings.ToLower(strings.TrimSpace(binding.Type))
			if protocol == "" {
				protocol = "tcp"
			}
			key := "docker\x00" + protocol + "\x00" + container.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			conflicts = append(conflicts, PortConflict{
				Source: "docker", Protocol: protocol, Container: container.Name,
			})
		}
	}
	if s.listeningPorts == nil {
		return InstallPortStatus{}, errors.New("host port inspection is unavailable")
	}
	listeners, err := s.listeningPorts(ctx)
	if err != nil {
		return InstallPortStatus{}, fmt.Errorf("inspect host listeners: %w", err)
	}
	for _, protocol := range listeners[port] {
		key := "listener\x00" + protocol
		if seen[key] {
			continue
		}
		seen[key] = true
		conflicts = append(conflicts, PortConflict{
			Source: "listener", Protocol: protocol,
		})
	}
	sort.Slice(conflicts, func(i, j int) bool {
		left := conflicts[i].Source + "\x00" + conflicts[i].Protocol + "\x00" + conflicts[i].Container
		right := conflicts[j].Source + "\x00" + conflicts[j].Protocol + "\x00" + conflicts[j].Container
		return left < right
	})
	return InstallPortStatus{
		Port: port, Available: len(conflicts) == 0, Conflicts: conflicts,
		CheckedAt: time.Now().UTC(),
	}, nil
}

func systemListeningPorts(ctx context.Context) (map[uint16][]string, error) {
	result := make(map[uint16][]string)
	if runtime.GOOS != "linux" {
		return result, nil
	}
	files := []struct {
		path     string
		protocol string
		tcp      bool
	}{
		{"/proc/net/tcp", "tcp", true},
		{"/proc/net/tcp6", "tcp6", true},
		{"/proc/net/udp", "udp", false},
		{"/proc/net/udp6", "udp6", false},
	}
	opened := 0
	for _, candidate := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		file, err := os.Open(candidate.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		opened++
		scanner := bufio.NewScanner(file)
		first := true
		for scanner.Scan() {
			if first {
				first = false
				continue
			}
			port, ok := listeningSocketPort(scanner.Text(), candidate.tcp)
			if !ok {
				continue
			}
			if !stringSliceContains(result[port], candidate.protocol) {
				result[port] = append(result[port], candidate.protocol)
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			return nil, scanErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	if opened == 0 {
		return nil, errors.New("Linux socket tables are unavailable")
	}
	return result, nil
}

// listeningSocketPort reads one /proc/net/{tcp,udp}{,6} row. TCP rows count
// only in LISTEN (0A). UDP has no listen state; a bound server socket stays
// unconnected (07), while connected client sockets (01, e.g. an outbound DNS
// query) hold only an ephemeral port and must not block an install.
func listeningSocketPort(line string, tcp bool) (uint16, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return 0, false
	}
	if tcp && fields[3] != "0A" || !tcp && fields[3] != "07" {
		return 0, false
	}
	_, rawPort, ok := strings.Cut(fields[1], ":")
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseUint(rawPort, 16, 16)
	if err != nil || value == 0 {
		return 0, false
	}
	return uint16(value), true
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
