package agent

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const dockerPortUsageEnrichmentTimeout = 3 * time.Second

// enrichPortUsageWithDocker adds only an optional Docker owner to the existing
// host ss snapshot. Docker is deliberately best-effort: a Docker outage must
// never hide or change the host port records.
func (s *Server) enrichPortUsageWithDocker(ctx context.Context, snapshot contract.PortUsageSnapshot) contract.PortUsageSnapshot {
	if s.docker == nil {
		return snapshot
	}
	dockerContext, cancel := context.WithTimeout(ctx, dockerPortUsageEnrichmentTimeout)
	defer cancel()
	containers, err := s.docker.Containers(dockerContext)
	if err != nil {
		return snapshot
	}
	return attachDockerPortOwners(snapshot, containers)
}

func attachDockerPortOwners(snapshot contract.PortUsageSnapshot, containers []contract.ContainerSummary) contract.PortUsageSnapshot {
	for index := range snapshot.Entries {
		if owner, ok := matchDockerPortOwner(snapshot.Entries[index], containers); ok {
			snapshot.Entries[index].Container = &owner
		}
	}
	return snapshot
}

type dockerPortCandidate struct {
	containerKey string
	owner        contract.PortUsageContainer
	score        int
}

func matchDockerPortOwner(entry contract.PortUsageEntry, containers []contract.ContainerSummary) (contract.PortUsageContainer, bool) {
	publicPort, err := strconv.ParseUint(strings.TrimSpace(entry.LocalPort), 10, 16)
	if err != nil || publicPort == 0 {
		return contract.PortUsageContainer{}, false
	}
	protocol := normalizePortProtocol(entry.Protocol)
	if protocol == "" {
		return contract.PortUsageContainer{}, false
	}

	bestByContainer := make(map[string]dockerPortCandidate)
	for _, container := range containers {
		if strings.ToLower(strings.TrimSpace(container.State)) != "running" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(container.NetworkMode), "host") {
			// Host-network containers share the host namespace. Their ports are
			// intentionally presented as the host process from ss, not as a
			// Docker-published mapping.
			continue
		}
		name := strings.TrimSpace(container.Name)
		id := strings.TrimSpace(container.ID)
		if name == "" && id == "" {
			continue
		}
		containerKey := id
		if containerKey == "" {
			containerKey = name
		}
		for _, binding := range container.Ports {
			if binding.PublicPort != uint16(publicPort) || normalizePortProtocol(binding.Type) != protocol {
				continue
			}
			score, matches := dockerPortAddressScore(entry.LocalAddress, binding.IP)
			if !matches {
				continue
			}
			owner := contract.PortUsageContainer{
				ID: id, Name: name, Image: strings.TrimSpace(container.Image),
				ContainerPort:  binding.PrivatePort,
				ComposeProject: strings.TrimSpace(container.ComposeProject),
				ComposeService: strings.TrimSpace(container.ComposeService),
			}
			candidate, exists := bestByContainer[containerKey]
			if !exists || score > candidate.score {
				bestByContainer[containerKey] = dockerPortCandidate{containerKey: containerKey, owner: owner, score: score}
				continue
			}
			if score == candidate.score && candidate.owner.ContainerPort != owner.ContainerPort {
				// The same container exposes the host port more than once. Keep
				// the owner, but do not claim a specific internal port.
				candidate.owner.ContainerPort = 0
				bestByContainer[containerKey] = candidate
			}
		}
	}

	var best dockerPortCandidate
	bestScore := -1
	ambiguous := false
	for _, candidate := range bestByContainer {
		if candidate.score > bestScore {
			best, bestScore, ambiguous = candidate, candidate.score, false
			continue
		}
		if candidate.score == bestScore {
			ambiguous = true
		}
	}
	if bestScore < 0 || ambiguous {
		// Multiple equally valid Docker owners are possible when different host
		// addresses are used. Do not guess in the system-center view.
		return contract.PortUsageContainer{}, false
	}
	return best.owner, true
}

func normalizePortProtocol(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "tcp", "tcp4", "tcp6":
		return "tcp"
	case "udp", "udp4", "udp6":
		return "udp"
	case "sctp", "sctp4", "sctp6":
		return "sctp"
	default:
		return ""
	}
}

func dockerPortAddressScore(localAddress, dockerAddress string) (int, bool) {
	localAddress = normalizePortAddress(localAddress)
	dockerAddress = normalizePortAddress(dockerAddress)
	localIP, localFamily, localWildcard := parsedPortAddress(localAddress)
	dockerIP, dockerFamily, dockerWildcard := parsedPortAddress(dockerAddress)

	if localIP != nil && dockerIP != nil && localIP.Equal(dockerIP) {
		return 4, true
	}
	if localWildcard && dockerWildcard && localFamily != "" && localFamily == dockerFamily {
		return 4, true
	}
	if dockerWildcard && localIP != nil && localFamily == dockerFamily {
		return 2, true
	}
	if dockerAddress == "" {
		// Older Docker responses may omit HostIp. It is safe to use this only
		// when the host port/protocol has one unambiguous Docker owner.
		return 1, true
	}
	return 0, false
}

func normalizePortAddress(value string) string {
	return strings.Trim(strings.TrimSpace(value), "[]")
}

func parsedPortAddress(value string) (net.IP, string, bool) {
	switch value {
	case "", "*":
		return nil, "", true
	case "0.0.0.0":
		return nil, "ipv4", true
	case "::":
		return nil, "ipv6", true
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return nil, "", false
	}
	if ip.To4() != nil {
		return ip, "ipv4", false
	}
	return ip, "ipv6", false
}
