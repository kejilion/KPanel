package netguard

import (
	"net"
	"testing"
)

// TestBlocked is the whole contract in one table. Every row that differs
// between the two modes is a deliberate decision about what "let me reach my
// LAN" buys the operator, and every row that does not differ is an address
// the operator cannot consent to reaching.
func TestBlocked(t *testing.T) {
	testCases := []struct {
		ip          string
		strict      bool // blocked when allowPrivate = false
		allowingLAN bool // blocked when allowPrivate = true
		why         string
	}{
		// The bypasses the old IsPrivate()/IsGlobalUnicast() predicate missed.
		{"100.100.100.200", true, true, "Alibaba Cloud metadata"},
		{"169.254.169.254", true, true, "AWS/GCP/Azure metadata"},
		{"fd00:ec2::254", true, true, "AWS IPv6 metadata"},
		{"100.64.0.1", true, false, "CGNAT / Tailscale peer"},
		{"::ffff:127.0.0.1", true, false, "IPv4-mapped loopback"},
		{"64:ff9b::a00:1", true, false, "NAT64-wrapped 10.0.0.1"},

		// Registry prefixes that are not RFC1918 and were previously dialed.
		{"192.0.0.1", true, true, "IETF protocol assignments"},
		{"192.0.2.1", true, true, "TEST-NET-1"},
		{"198.18.0.1", true, true, "benchmarking"},
		{"198.51.100.1", true, true, "TEST-NET-2"},
		{"203.0.113.1", true, true, "TEST-NET-3"},
		{"192.88.99.1", true, true, "6to4 relay anycast"},
		{"240.0.0.1", true, true, "reserved"},
		{"2001:db8::1", true, true, "documentation"},
		{"2002:c0a8:0001::1", true, true, "6to4"},
		{"2001::1", true, true, "Teredo"},
		{"100::1", true, true, "discard-only"},

		// The classic private space: blocked strictly, reachable on LAN.
		{"10.0.0.1", true, false, "RFC1918"},
		{"172.16.0.1", true, false, "RFC1918"},
		{"172.31.255.254", true, false, "RFC1918 upper bound"},
		{"192.168.1.1", true, false, "RFC1918"},
		{"127.0.0.1", true, false, "loopback"},
		{"::1", true, false, "IPv6 loopback"},
		{"fc00::1", true, false, "unique local"},
		{"fd7a:115c:a1e0::1", true, false, "Tailscale ULA"},

		// Never reachable, LAN mode or not.
		{"0.0.0.0", true, true, "unspecified"},
		{"::", true, true, "IPv6 unspecified"},
		{"255.255.255.255", true, true, "broadcast"},
		{"224.0.0.1", true, true, "multicast"},
		{"ff02::1", true, true, "IPv6 multicast"},
		{"169.254.1.1", true, true, "link-local is not a LAN"},
		{"fe80::1", true, true, "IPv6 link-local"},

		// Must stay reachable, or the feature does nothing.
		{"8.8.8.8", false, false, "public DNS"},
		{"1.1.1.1", false, false, "public DNS"},
		{"93.184.216.34", false, false, "public web"},
		{"172.32.0.1", false, false, "just outside RFC1918"},
		{"100.128.0.1", false, false, "just outside CGNAT"},
		{"2001:4860:4860::8888", false, false, "public IPv6 DNS"},
		{"2606:4700::1111", false, false, "public IPv6"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.ip, func(t *testing.T) {
			ip := net.ParseIP(testCase.ip)
			if ip == nil {
				t.Fatalf("test table has an unparseable address %q", testCase.ip)
			}
			if got := Blocked(ip, false); got != testCase.strict {
				t.Errorf("Blocked(%s, false) = %v, want %v (%s)", testCase.ip, got, testCase.strict, testCase.why)
			}
			if got := Blocked(ip, true); got != testCase.allowingLAN {
				t.Errorf("Blocked(%s, true) = %v, want %v (%s)", testCase.ip, got, testCase.allowingLAN, testCase.why)
			}
		})
	}
}

// TestBlockedFailsClosed pins the nil/garbage path: callers reach Blocked
// after a DNS lookup or a literal parse, and an address they could not make
// sense of must not be dialed.
func TestBlockedFailsClosed(t *testing.T) {
	for _, allowPrivate := range []bool{false, true} {
		if !Blocked(nil, allowPrivate) {
			t.Errorf("Blocked(nil, %v) = false, want true", allowPrivate)
		}
		if !Blocked(net.IP{1, 2, 3}, allowPrivate) {
			t.Errorf("Blocked(3-byte IP, %v) = false, want true", allowPrivate)
		}
	}
}

// TestBlockedAcceptsBothIPv4Forms guards the 4-byte/16-byte split in net.IP:
// a lookup can hand back either representation of the same address, and the
// table walk must not depend on which one it got.
func TestBlockedAcceptsBothIPv4Forms(t *testing.T) {
	for _, raw := range []string{"10.0.0.1", "100.100.100.200", "8.8.8.8"} {
		ip := net.ParseIP(raw)
		if got, want := Blocked(ip.To4(), false), Blocked(ip.To16(), false); got != want {
			t.Errorf("Blocked(%s) disagrees between To4 (%v) and To16 (%v)", raw, got, want)
		}
	}
}
