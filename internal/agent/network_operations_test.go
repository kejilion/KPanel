package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestNetworkOperationsRoutesKeepStrictURLAndBodyBoundaries(t *testing.T) {
	server := testServer(t)
	token := "Bearer " + strings.Repeat("x", 32)

	request := httptest.NewRequest(http.MethodGet, "/v1/system/port-usage?raw=true", nil)
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_system_resource_url") {
		t.Fatalf("query status=%d body=%s", response.Code, response.Body.String())
	}

	version := strings.Repeat("a", 64)
	tests := []struct {
		name   string
		body   string
		status int
		code   string
	}{
		{name: "unknown field", body: `{"action":"disable","expectedResourceVersion":"` + version + `","command":"shutdown"}`, status: http.StatusBadRequest, code: "invalid_request"},
		{name: "disable fields", body: `{"action":"disable","expectedResourceVersion":"` + version + `","rxThresholdGiB":100}`, status: http.StatusUnprocessableEntity, code: "invalid_system_resource_action"},
		{name: "disabled write", body: `{"action":"disable","expectedResourceVersion":"` + version + `"}`, status: http.StatusForbidden, code: "system_resource_write_disabled"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/system/traffic-shutdown/actions", strings.NewReader(test.body))
			request.Header.Set("Authorization", token)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAttachDockerPortOwnersPreservesHostRecordsAndMatchesPublishedPorts(t *testing.T) {
	snapshot := contract.PortUsageSnapshot{Entries: []contract.PortUsageEntry{
		{Protocol: "tcp", LocalAddress: "0.0.0.0", LocalPort: "8080", Process: "docker-proxy", PID: 100, Raw: "tcp fixture 8080"},
		{Protocol: "tcp6", LocalAddress: "::1", LocalPort: "8081", Process: "docker-proxy", PID: 101, Raw: "tcp6 fixture 8081"},
		{Protocol: "udp", LocalAddress: "127.0.0.1", LocalPort: "5353", Process: "dns-proxy", PID: 102, Raw: "udp fixture 5353"},
		{Protocol: "tcp", LocalAddress: "0.0.0.0", LocalPort: "8082", Process: "host-service", PID: 103, Raw: "tcp fixture 8082"},
		{Protocol: "tcp", LocalAddress: "0.0.0.0", LocalPort: "8083", Process: "host-service", PID: 104, Raw: "tcp fixture 8083"},
		{Protocol: "tcp", LocalAddress: "0.0.0.0", LocalPort: "8084", Process: "host-service", PID: 105, Raw: "tcp fixture 8084"},
	}}
	containers := []contract.ContainerSummary{
		{
			ID: "container-web", Name: "web-nginx", Image: "nginx:alpine", State: "running",
			Ports: []contract.PortBinding{{PrivatePort: 80, PublicPort: 8080, IP: "0.0.0.0", Type: "tcp"}},
		},
		{
			ID: "container-admin", Name: "admin-ui", Image: "admin:latest", State: "running",
			Ports: []contract.PortBinding{{PrivatePort: 3000, PublicPort: 8081, IP: "::1", Type: "tcp"}},
		},
		{
			ID: "container-dns", Name: "dns", Image: "dns:latest", State: "running",
			Ports: []contract.PortBinding{{PrivatePort: 53, PublicPort: 5353, IP: "127.0.0.1", Type: "udp"}},
		},
		{
			ID: "container-stopped", Name: "stopped", Image: "old:latest", State: "exited",
			Ports: []contract.PortBinding{{PrivatePort: 80, PublicPort: 8082, IP: "0.0.0.0", Type: "tcp"}},
		},
		{
			ID: "container-internal", Name: "internal", Image: "internal:latest", State: "running",
			Ports: []contract.PortBinding{{PrivatePort: 8083, Type: "tcp"}},
		},
		{
			ID: "container-a", Name: "same-port-a", State: "running",
			Ports: []contract.PortBinding{{PrivatePort: 80, PublicPort: 8084, IP: "0.0.0.0", Type: "tcp"}},
		},
		{
			ID: "container-b", Name: "same-port-b", State: "running",
			Ports: []contract.PortBinding{{PrivatePort: 80, PublicPort: 8084, IP: "0.0.0.0", Type: "tcp"}},
		},
	}

	got := attachDockerPortOwners(snapshot, containers)
	if got.Entries[0].Container == nil || got.Entries[0].Container.Name != "web-nginx" || got.Entries[0].Container.ContainerPort != 80 {
		t.Fatalf("published Docker port was not matched: %#v", got.Entries[0].Container)
	}
	if got.Entries[1].Container == nil || got.Entries[1].Container.Name != "admin-ui" || got.Entries[1].Container.ContainerPort != 3000 {
		t.Fatalf("tcp6 Docker port was not matched: %#v", got.Entries[1].Container)
	}
	if got.Entries[2].Container == nil || got.Entries[2].Container.Name != "dns" || got.Entries[2].Container.ContainerPort != 53 {
		t.Fatalf("udp Docker port was not matched: %#v", got.Entries[2].Container)
	}
	for _, index := range []int{3, 4, 5} {
		if got.Entries[index].Container != nil {
			t.Fatalf("entry %d was incorrectly associated with Docker: %#v", index, got.Entries[index].Container)
		}
	}
	if got.Entries[0].Process != snapshot.Entries[0].Process || got.Entries[0].PID != snapshot.Entries[0].PID || got.Entries[0].Raw != snapshot.Entries[0].Raw {
		t.Fatalf("host record was changed while adding Docker metadata: %#v", got.Entries[0])
	}
}

func TestMatchDockerPortOwnerSkipsHostNetworkContainers(t *testing.T) {
	entry := contract.PortUsageEntry{Protocol: "tcp", LocalAddress: "0.0.0.0", LocalPort: "80", Process: "nginx"}
	owner, ok := matchDockerPortOwner(entry, []contract.ContainerSummary{{
		ID: "host-nginx", Name: "nginx", Image: "nginx:alpine", State: "running", NetworkMode: "host",
		Ports: []contract.PortBinding{{PrivatePort: 80, PublicPort: 80, IP: "0.0.0.0", Type: "tcp"}},
	}})
	if ok {
		t.Fatalf("host-network container was exposed as Docker owner: %#v", owner)
	}
}

func TestMatchDockerPortOwnerPrefersExactAddressAndDoesNotGuessTies(t *testing.T) {
	entry := contract.PortUsageEntry{Protocol: "tcp", LocalAddress: "127.0.0.1", LocalPort: "8088"}
	containers := []contract.ContainerSummary{
		{
			ID: "wildcard", Name: "wildcard", State: "running",
			Ports: []contract.PortBinding{{PrivatePort: 80, PublicPort: 8088, IP: "0.0.0.0", Type: "tcp"}},
		},
		{
			ID: "loopback", Name: "loopback", State: "running",
			Ports: []contract.PortBinding{{PrivatePort: 8080, PublicPort: 8088, IP: "127.0.0.1", Type: "tcp"}},
		},
	}
	owner, ok := matchDockerPortOwner(entry, containers)
	if !ok || owner.Name != "loopback" || owner.ContainerPort != 8080 {
		t.Fatalf("exact Docker address did not win: ok=%v owner=%#v", ok, owner)
	}

	containers[1].Ports[0].IP = "0.0.0.0"
	if _, ok := matchDockerPortOwner(entry, containers); ok {
		t.Fatal("ambiguous Docker owners were guessed")
	}
}
