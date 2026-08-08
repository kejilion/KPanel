package ai

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"testing"
)

type stubResolver struct {
	addresses []net.IPAddr
	err       error
}

func (s stubResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return s.addresses, s.err
}

func ipAddr(ip string) net.IPAddr {
	return net.IPAddr{IP: net.ParseIP(ip)}
}

func TestIsFakeIPAddress(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"198.18.0.1", true},     // RFC 2544 benchmark（Clash 默认 fake-ip-range）
		{"198.19.255.255", true}, // 同上
		{"100.64.0.1", true},     // CGNAT
		{"100.127.255.255", true},
		{"10.199.0.101", true}, // RFC1918 私网（用户环境 fake-ip）
		{"192.168.1.1", true},  // RFC1918
		{"240.0.0.1", true},    // 保留段
		{"127.0.0.1", true},    // loopback 非公网
		{"1.2.3.4", false},     // 公网
		{"93.184.216.34", false},
		{"8.8.8.8", false},
	}
	for _, test := range tests {
		if got := isFakeIPAddress(net.ParseIP(test.ip)); got != test.want {
			t.Errorf("isFakeIPAddress(%s) = %v, want %v", test.ip, got, test.want)
		}
	}
}

func TestAnyResolvedAreFakeIP(t *testing.T) {
	if !anyResolvedAreFakeIP([]net.IPAddr{ipAddr("10.199.0.101"), ipAddr("198.18.0.1")}) {
		t.Fatal("all fake addresses must be detected")
	}
	if !anyResolvedAreFakeIP([]net.IPAddr{ipAddr("10.199.0.101"), ipAddr("2606:4700:4700::1111")}) {
		t.Fatal("any fake address must trigger fallback")
	}
	if anyResolvedAreFakeIP([]net.IPAddr{ipAddr("1.2.3.4"), ipAddr("2606:4700:4700::1111")}) {
		t.Fatal("public-only addresses must not trigger fallback")
	}
	if anyResolvedAreFakeIP(nil) {
		t.Fatal("empty result must not trigger fallback")
	}
}

func TestResolveProviderHostFallsBackToPublicDNS(t *testing.T) {
	client := &HTTPModelClient{
		resolver: stubResolver{addresses: []net.IPAddr{ipAddr("10.199.0.101")}},
		fallbackLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{ipAddr("93.184.216.34")}, nil
		},
	}
	addresses, err := client.resolveProviderHost(context.Background(), Provider{Protocol: ProtocolOpenAICompatible, EndpointScope: EndpointPublic}, "api.deepseek.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) != 1 || addresses[0].IP.String() != "93.184.216.34" {
		t.Fatalf("expected fallback address, got %#v", addresses)
	}
}

func TestResolveProviderHostFallsBackForBenchmarkAndCGNAT(t *testing.T) {
	for _, fake := range []string{"198.18.0.1", "100.64.0.1"} {
		client := &HTTPModelClient{
			resolver: stubResolver{addresses: []net.IPAddr{ipAddr(fake)}},
			fallbackLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
				return []net.IPAddr{ipAddr("93.184.216.34")}, nil
			},
		}
		addresses, err := client.resolveProviderHost(context.Background(), Provider{EndpointScope: EndpointPublic}, "api.example.com")
		if err != nil {
			t.Fatalf("%s: %v", fake, err)
		}
		if len(addresses) != 1 || addresses[0].IP.String() != "93.184.216.34" {
			t.Fatalf("%s: expected fallback, got %#v", fake, addresses)
		}
	}
}

func TestResolveProviderHostFallsBackWhenFakeIPMixedWithPublic(t *testing.T) {
	client := &HTTPModelClient{
		resolver: stubResolver{addresses: []net.IPAddr{ipAddr("10.199.0.101"), ipAddr("2606:4700:4700::1111")}},
		fallbackLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{ipAddr("93.184.216.34")}, nil
		},
	}
	addresses, err := client.resolveProviderHost(context.Background(), Provider{EndpointScope: EndpointPublic}, "api.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) != 1 || addresses[0].IP.String() != "93.184.216.34" {
		t.Fatalf("expected fallback address, got %#v", addresses)
	}
}

func TestResolveProviderHostKeepsPublicAddress(t *testing.T) {
	called := false
	client := &HTTPModelClient{
		resolver: stubResolver{addresses: []net.IPAddr{ipAddr("93.184.216.34")}},
		fallbackLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			called = true
			return []net.IPAddr{ipAddr("1.1.1.1")}, nil
		},
	}
	addresses, err := client.resolveProviderHost(context.Background(), Provider{EndpointScope: EndpointPublic}, "api.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("public address must not trigger fallback lookup")
	}
	if len(addresses) != 1 || addresses[0].IP.String() != "93.184.216.34" {
		t.Fatalf("expected original address, got %#v", addresses)
	}
}

func TestResolveProviderHostReturnsOriginalErrorWhenFallbackFails(t *testing.T) {
	client := &HTTPModelClient{
		resolver: stubResolver{addresses: []net.IPAddr{ipAddr("10.199.0.101")}},
		fallbackLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return nil, errors.New("public DNS unreachable")
		},
	}
	_, err := client.resolveProviderHost(context.Background(), Provider{EndpointScope: EndpointPublic}, "api.deepseek.com")
	if err == nil || !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("expected original validation error, got %v", err)
	}
}

func TestResolveProviderHostRejectsPrivateFallbackResult(t *testing.T) {
	client := &HTTPModelClient{
		resolver: stubResolver{addresses: []net.IPAddr{ipAddr("10.199.0.101")}},
		fallbackLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			return []net.IPAddr{ipAddr("192.168.1.10")}, nil
		},
	}
	_, err := client.resolveProviderHost(context.Background(), Provider{EndpointScope: EndpointPublic}, "api.example.com")
	if err == nil || !strings.Contains(err.Error(), "non-public address") {
		t.Fatalf("fallback result must still be validated, got %v", err)
	}
}

func TestLookupViaServersQueriesSpecifiedServer(t *testing.T) {
	server := startDNSStub(t, net.ParseIP("93.184.216.34"))
	addresses, err := lookupViaServers(context.Background(), "resolve-test.example", []string{server})
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) != 1 || addresses[0].IP.String() != "93.184.216.34" {
		t.Fatalf("unexpected addresses: %#v", addresses)
	}
}

func TestPreferIPv4(t *testing.T) {
	addresses := preferIPv4([]net.IPAddr{
		ipAddr("2606:4700:4700::1111"),
		ipAddr("93.184.216.34"),
		ipAddr("1.2.3.4"),
		ipAddr("2606:4700:4700::2222"),
	})
	want := []string{"93.184.216.34", "1.2.3.4", "2606:4700:4700::1111", "2606:4700:4700::2222"}
	if len(addresses) != len(want) {
		t.Fatalf("length = %d, want %d", len(addresses), len(want))
	}
	for i, address := range addresses {
		if address.IP.String() != want[i] {
			t.Errorf("index %d = %s, want %s", i, address.IP, want[i])
		}
	}
}

func TestServerWithDefaultPort(t *testing.T) {
	tests := []struct {
		server string
		want   string
	}{
		{"8.8.8.8", "8.8.8.8:53"},
		{"2001:4860::8888", "[2001:4860::8888]:53"},
		{"127.0.0.1:59669", "127.0.0.1:59669"},
	}
	for _, test := range tests {
		if got := serverWithDefaultPort(test.server); got != test.want {
			t.Errorf("serverWithDefaultPort(%s) = %s, want %s", test.server, got, test.want)
		}
	}
}

func TestResolveProviderHostPrivateScopeUnchanged(t *testing.T) {
	called := false
	client := &HTTPModelClient{
		resolver: stubResolver{addresses: []net.IPAddr{ipAddr("10.199.0.101")}},
		fallbackLookup: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			called = true
			return []net.IPAddr{ipAddr("93.184.216.34")}, nil
		},
	}
	addresses, err := client.resolveProviderHost(context.Background(), Provider{EndpointScope: EndpointPrivate}, "host.docker.internal")
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("private scope must not trigger fallback lookup")
	}
	if len(addresses) != 1 || addresses[0].IP.String() != "10.199.0.101" {
		t.Fatalf("private scope must keep resolved address, got %#v", addresses)
	}
}

// startDNSStub 在本地回环启动一个最小 UDP DNS 服务器，
// 对 A 查询返回固定答案，对其他类型返回空响应。
func startDNSStub(t *testing.T, answer net.IP) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if response := buildDNSResponse(buf[:n], answer); response != nil {
				pc.WriteTo(response, addr)
			}
		}
	}()
	return pc.LocalAddr().String()
}

// buildDNSResponse 构造最小 DNS 响应：回显查询 ID 与问题，
// 仅对 A 查询附带一条 IPv4 答案。
func buildDNSResponse(query []byte, answer net.IP) []byte {
	if len(query) < 12 {
		return nil
	}
	offset := 12
	for offset < len(query) && query[offset] != 0 {
		offset += int(query[offset]) + 1
	}
	offset++
	if offset+4 > len(query) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(query[offset:])
	question := query[12 : offset+4]

	response := make([]byte, 0, 64)
	response = append(response, query[0], query[1]) // ID
	response = append(response, 0x81, 0x80)         // QR=1, RD=1, RA=1, RCODE=0
	response = append(response, 0x00, 0x01)         // QDCOUNT=1
	if qtype == 1 && answer != nil && answer.To4() != nil {
		response = append(response, 0x00, 0x01) // ANCOUNT=1
	} else {
		response = append(response, 0x00, 0x00) // ANCOUNT=0
	}
	response = append(response, 0x00, 0x00) // NSCOUNT=0
	response = append(response, 0x00, 0x00) // ARCOUNT=0
	response = append(response, question...)
	if qtype == 1 && answer != nil && answer.To4() != nil {
		response = append(response, 0xC0, 0x0C)             // NAME 指针指向问题名
		response = append(response, 0x00, 0x01)             // TYPE A
		response = append(response, 0x00, 0x01)             // CLASS IN
		response = append(response, 0x00, 0x00, 0x00, 0x3C) // TTL=60
		response = append(response, 0x00, 0x04)             // RDLENGTH=4
		response = append(response, answer.To4()...)
	}
	return response
}
