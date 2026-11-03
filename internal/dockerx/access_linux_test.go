//go:build linux

package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestContainerAccessUsesKejilionDockerUserRuleShape(t *testing.T) {
	id := strings.Repeat("e", 64)
	raw := managedInspect(id, "2026-01-01T00:00:00Z", 0)
	raw.NetworkSettings.Networks = map[string]dockerNetworkEndpoint{
		"app": {IPAddress: "172.30.0.9"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/containers/"+id+"/json" {
			http.NotFound(response, request)
			return
		}
		_ = json.NewEncoder(response).Encode(raw)
	}))
	defer server.Close()
	client := testHTTPClient(server)
	client.iptablesRulesPath = filepath.Join(t.TempDir(), "iptables", "rules.v4")
	var inserted [][]string
	client.hostCommand = func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		switch {
		case name == "iptables-save":
			return []byte("*filter\nCOMMIT\n"), nil
		case name == "iptables" && slices.Contains(arguments, "-S"):
			return []byte("-N DOCKER-USER\n"), nil
		case name == "iptables" && slices.Contains(arguments, "-C"):
			return nil, errors.New("rule absent")
		case name == "iptables" && slices.Contains(arguments, "-I"):
			inserted = append(inserted, append([]string(nil), arguments...))
			return nil, nil
		default:
			return nil, errors.New("unexpected command")
		}
	}
	version := client.summaryFromInspect(raw).ResourceVersion
	if err := client.updateContainerAccess(
		context.Background(),
		id,
		version,
		false,
		"203.0.113.10",
	); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, arguments := range inserted {
		joined += strings.Join(arguments, " ") + "\n"
	}
	expectedOrder := []string{
		"-p tcp -d 172.30.0.9 -j DROP",
		"-p tcp -s 203.0.113.10 -d 172.30.0.9 -j ACCEPT",
		"-p tcp -s 127.0.0.0/8 -d 172.30.0.9 -j ACCEPT",
		"-p udp -d 172.30.0.9 -j DROP",
		"-p udp -s 203.0.113.10 -d 172.30.0.9 -j ACCEPT",
		"-p udp -s 127.0.0.0/8 -d 172.30.0.9 -j ACCEPT",
		"-m state --state ESTABLISHED,RELATED -d 172.30.0.9 -j ACCEPT",
	}
	if len(inserted) != len(expectedOrder) {
		t.Fatalf("inserted rules = %d, want %d:\n%s", len(inserted), len(expectedOrder), joined)
	}
	for index, expected := range expectedOrder {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing Kejilion-compatible rule %q in:\n%s", expected, joined)
		}
		if actual := strings.Join(inserted[index], " "); !strings.HasSuffix(actual, expected) {
			t.Fatalf("rule %d = %q, want script insertion order suffix %q", index, actual, expected)
		}
	}
	data, err := os.ReadFile(client.iptablesRulesPath)
	if err != nil || string(data) != "*filter\nCOMMIT\n" {
		t.Fatalf("iptables persistence = %q, %v", data, err)
	}
}

func TestContainerAccessRollsBackKernelRulesWhenPersistenceFails(t *testing.T) {
	id := strings.Repeat("f", 64)
	raw := managedInspect(id, "2026-01-01T00:00:00Z", 0)
	raw.NetworkSettings.Networks = map[string]dockerNetworkEndpoint{
		"app": {IPAddress: "172.30.0.10"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(response).Encode(raw)
	}))
	defer server.Close()
	client := testHTTPClient(server)
	client.iptablesRulesPath = filepath.Join(t.TempDir(), "rules.v4")
	var inserted, deleted int
	client.hostCommand = func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		switch {
		case name == "iptables-save":
			return nil, errors.New("save failed")
		case name == "iptables" && slices.Contains(arguments, "-S"):
			return []byte("-N DOCKER-USER\n"), nil
		case name == "iptables" && slices.Contains(arguments, "-C"):
			return nil, errors.New("rule absent")
		case name == "iptables" && slices.Contains(arguments, "-I"):
			inserted++
			return nil, nil
		case name == "iptables" && slices.Contains(arguments, "-D"):
			deleted++
			return nil, nil
		default:
			return nil, errors.New("unexpected command")
		}
	}
	version := client.summaryFromInspect(raw).ResourceVersion
	err := client.updateContainerAccess(context.Background(), id, version, false, "")
	if err == nil || !strings.Contains(err.Error(), "save Docker iptables firewall rules") {
		t.Fatalf("persistence failure = %v", err)
	}
	if inserted == 0 || deleted != inserted {
		t.Fatalf("firewall rollback inserted=%d deleted=%d", inserted, deleted)
	}
}

func TestContainerAccessAppliesRulesToEveryDockerNetwork(t *testing.T) {
	id := strings.Repeat("a", 64)
	raw := managedInspect(id, "2026-01-01T00:00:00Z", 0)
	raw.NetworkSettings.Networks = map[string]dockerNetworkEndpoint{
		"frontend": {IPAddress: "172.30.0.11"},
		"backend":  {IPAddress: "172.31.0.12"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(response).Encode(raw)
	}))
	defer server.Close()
	client := testHTTPClient(server)
	client.iptablesRulesPath = filepath.Join(t.TempDir(), "rules.v4")
	var inserted [][]string
	client.hostCommand = func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		switch {
		case name == "iptables-save":
			return []byte("*filter\nCOMMIT\n"), nil
		case name == "iptables" && slices.Contains(arguments, "-S"):
			return []byte("-N DOCKER-USER\n"), nil
		case name == "iptables" && slices.Contains(arguments, "-C"):
			return nil, errors.New("rule absent")
		case name == "iptables" && slices.Contains(arguments, "-I"):
			inserted = append(inserted, append([]string(nil), arguments...))
			return nil, nil
		default:
			return nil, errors.New("unexpected command")
		}
	}
	version := client.summaryFromInspect(raw).ResourceVersion
	if err := client.updateContainerAccess(context.Background(), id, version, false, ""); err != nil {
		t.Fatal(err)
	}
	joined := ""
	for _, arguments := range inserted {
		joined += strings.Join(arguments, " ") + "\n"
	}
	for _, address := range []string{"172.30.0.11", "172.31.0.12"} {
		if !strings.Contains(joined, "-d "+address+" -j DROP") {
			t.Fatalf("network address %s was not covered:\n%s", address, joined)
		}
	}
}

func TestContainerAccessAppliesAndPersistsIPv6Rules(t *testing.T) {
	id := strings.Repeat("b", 64)
	raw := managedInspect(id, "2026-01-01T00:00:00Z", 0)
	raw.NetworkSettings.Networks = map[string]dockerNetworkEndpoint{
		"frontend": {IPAddress: "172.30.0.21", GlobalIPv6Address: "2001:db8::21"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(response).Encode(raw)
	}))
	defer server.Close()
	client := testHTTPClient(server)
	directory := t.TempDir()
	client.iptablesRulesPath = filepath.Join(directory, "rules.v4")
	client.ip6tablesRulesPath = filepath.Join(directory, "rules.v6")
	var v4Rules, v6Rules [][]string
	client.hostCommand = func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		switch {
		case name == "iptables-save":
			return []byte("v4-saved\n"), nil
		case name == "ip6tables-save":
			return []byte("v6-saved\n"), nil
		case (name == "iptables" || name == "ip6tables") && slices.Contains(arguments, "-S"):
			return []byte("-N DOCKER-USER\n"), nil
		case (name == "iptables" || name == "ip6tables") && slices.Contains(arguments, "-C"):
			return nil, errors.New("rule absent")
		case name == "iptables" && slices.Contains(arguments, "-I"):
			v4Rules = append(v4Rules, append([]string(nil), arguments...))
			return nil, nil
		case name == "ip6tables" && slices.Contains(arguments, "-I"):
			v6Rules = append(v6Rules, append([]string(nil), arguments...))
			return nil, nil
		default:
			return nil, errors.New("unexpected command")
		}
	}
	if err := client.updateContainerAccess(context.Background(), id, client.summaryFromInspect(raw).ResourceVersion, false, "203.0.113.10"); err != nil {
		t.Fatal(err)
	}
	joinedV6 := ""
	for _, arguments := range v6Rules {
		joinedV6 += strings.Join(arguments, " ") + "\n"
	}
	if !strings.Contains(joinedV6, "-p tcp -d 2001:db8::21 -j DROP") ||
		!strings.Contains(joinedV6, "-p tcp -s ::1/128 -d 2001:db8::21 -j ACCEPT") ||
		!strings.Contains(joinedV6, "-m state --state ESTABLISHED,RELATED -d 2001:db8::21 -j ACCEPT") ||
		strings.Contains(joinedV6, "203.0.113.10") {
		t.Fatalf("IPv6 rules do not enforce the access boundary:\n%s", joinedV6)
	}
	if len(v4Rules) == 0 {
		t.Fatal("IPv4 rules were not applied alongside IPv6 rules")
	}
	for path, want := range map[string]string{client.iptablesRulesPath: "v4-saved\n", client.ip6tablesRulesPath: "v6-saved\n"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("persisted rules %s = %q, %v; want %q", path, got, err, want)
		}
	}
}

func TestContainerAccessRollsBackIPv4WhenIPv6RulesAreUnavailable(t *testing.T) {
	id := strings.Repeat("c", 64)
	raw := managedInspect(id, "2026-01-01T00:00:00Z", 0)
	raw.NetworkSettings.Networks = map[string]dockerNetworkEndpoint{
		"frontend": {IPAddress: "172.30.0.22", GlobalIPv6Address: "2001:db8::22"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(response).Encode(raw)
	}))
	defer server.Close()
	client := testHTTPClient(server)
	client.iptablesRulesPath = filepath.Join(t.TempDir(), "rules.v4")
	var inserted, deleted int
	client.hostCommand = func(_ context.Context, name string, arguments ...string) ([]byte, error) {
		switch {
		case name == "iptables" && slices.Contains(arguments, "-S"):
			return []byte("-N DOCKER-USER\n"), nil
		case name == "iptables" && slices.Contains(arguments, "-C"):
			return nil, errors.New("rule absent")
		case name == "iptables" && slices.Contains(arguments, "-I"):
			inserted++
			return nil, nil
		case name == "iptables" && slices.Contains(arguments, "-D"):
			deleted++
			return nil, nil
		case name == "ip6tables" && slices.Contains(arguments, "-S"):
			return nil, errors.New("DOCKER-USER chain missing")
		default:
			return nil, errors.New("unexpected command")
		}
	}
	err := client.updateContainerAccess(context.Background(), id, client.summaryFromInspect(raw).ResourceVersion, false, "")
	if err == nil || !strings.Contains(err.Error(), "ip6tables DOCKER-USER") {
		t.Fatalf("missing IPv6 chain error = %v", err)
	}
	if inserted == 0 || deleted != inserted {
		t.Fatalf("IPv4 rollback inserted=%d deleted=%d", inserted, deleted)
	}
}
