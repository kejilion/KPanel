// Package netguard decides whether an IP address may be reached by egress
// that a panel user controls the target of: the AI provider client
// (internal/ai) and the desktop browser's fetch/WebSocket relay
// (internal/agent). Both are SSRF sinks — the operator types a URL, paneld
// connects to it — so both need the same answer to one question, and used to
// answer it with two byte-identical private copies that could drift.
//
// The rule is the IANA IPv4/IPv6 Special-Purpose Address Registry: an address
// is blocked when its prefix is registered "Globally Reachable = False".
// That is deliberately not the same as net.IP's IsPrivate/IsGlobalUnicast
// combination the two copies used before. Go's own documentation says
// IsPrivate reports RFC1918/RFC4193 membership and nothing else, and the gap
// between "not RFC1918" and "not reachable from the internet" is where the
// interesting targets live: 100.64.0.0/10 (carrier NAT, and also every
// Tailscale node's address) and 100.100.100.200 (Alibaba Cloud's metadata
// service) both satisfied the old predicate's every clause and were dialed.
package netguard

import "net"

// lanPrefix marks the prefixes that stop being blocked when the operator has
// opted this host into LAN egress (store.AllowedHosts.BrowseAllowPrivateNetworks).
// Everything else in the tables below stays blocked in both modes: "let me
// reach my LAN" is a request for RFC1918/ULA/loopback and, on a host that
// joined a tailnet, its CGNAT-range peers — it is not a request for the
// benchmarking range, documentation ranges, or link-local.
type entry struct {
	cidr string
	lan  bool
	net  *net.IPNet
}

// blockedV4 is the IPv4 half of the registry. 224.0.0.0/4 and 240.0.0.0/4
// fold in multicast, the reserved space, and 255.255.255.255.
var blockedV4 = []entry{
	{cidr: "0.0.0.0/8"}, // "this network"
	{cidr: "10.0.0.0/8", lan: true},
	{cidr: "100.64.0.0/10", lan: true}, // CGNAT; also the Tailscale range
	{cidr: "127.0.0.0/8", lan: true},   // loopback
	{cidr: "169.254.0.0/16"},           // link-local — see metadataAddresses
	{cidr: "172.16.0.0/12", lan: true},
	{cidr: "192.0.0.0/24"},   // IETF protocol assignments
	{cidr: "192.0.2.0/24"},   // TEST-NET-1
	{cidr: "192.88.99.0/24"}, // 6to4 relay anycast (deprecated)
	{cidr: "192.168.0.0/16", lan: true},
	{cidr: "198.18.0.0/15"},   // benchmarking
	{cidr: "198.51.100.0/24"}, // TEST-NET-2
	{cidr: "203.0.113.0/24"},  // TEST-NET-3
	{cidr: "224.0.0.0/4"},     // multicast
	{cidr: "240.0.0.0/4"},     // reserved, incl. 255.255.255.255
}

// blockedV6 is the IPv6 half. Two prefixes are handled before this table
// rather than in it, because membership alone is not the interesting fact:
// ::ffff:0:0/96 and 64:ff9b::/96 carry an IPv4 address in their low 32 bits,
// so ::ffff:127.0.0.1 and 64:ff9b::a00:1 have to be judged by the IPv4 table.
// Go's net.IP already unwraps the first form; nothing unwraps the second,
// which is why a NAT64 gateway was a way around the old predicate entirely.
var blockedV6 = []entry{
	{cidr: "::/128"},              // unspecified
	{cidr: "::1/128", lan: true},  // loopback
	{cidr: "100::/64"},            // discard-only
	{cidr: "2001::/23"},           // IETF protocol assignments (incl. Teredo)
	{cidr: "2001:db8::/32"},       // documentation
	{cidr: "2002::/16"},           // 6to4 — blocked whole, not unwrapped
	{cidr: "64:ff9b:1::/48"},      // local-use NAT64
	{cidr: "fc00::/7", lan: true}, // unique local (incl. Tailscale's ULA)
	{cidr: "fe80::/10"},           // link-local
	{cidr: "ff00::/8"},            // multicast
}

// metadataAddresses are blocked in *both* modes, including when the operator
// turned LAN egress on. These addresses are never a legitimate browsing or
// provider target and are always a credential source: each one answers
// unauthenticated HTTP with the host's cloud IAM role. An operator asking for
// LAN access is asking to reach their own machines, and cannot meaningfully
// consent to handing the panel's instance role to a page they browse.
var metadataAddresses = []string{
	"169.254.169.254", // AWS/GCP/Azure/Oracle/OpenStack/DigitalOcean IMDS
	"100.100.100.200", // Alibaba Cloud
	"fd00:ec2::254",   // AWS IMDS over IPv6
}

var (
	nat64Prefix *net.IPNet
	metadataIPs []net.IP
)

func init() {
	for i := range blockedV4 {
		_, blockedV4[i].net = mustCIDR(blockedV4[i].cidr)
	}
	for i := range blockedV6 {
		_, blockedV6[i].net = mustCIDR(blockedV6[i].cidr)
	}
	_, nat64Prefix = mustCIDR("64:ff9b::/96")
	for _, raw := range metadataAddresses {
		metadataIPs = append(metadataIPs, net.ParseIP(raw))
	}
}

func mustCIDR(cidr string) (net.IP, *net.IPNet) {
	ip, network, err := net.ParseCIDR(cidr)
	if err != nil {
		// Unreachable: every literal above is a compile-time constant in this
		// file. Panicking at init beats returning a nil *net.IPNet that would
		// silently match nothing and open the guard.
		panic("netguard: invalid CIDR " + cidr + ": " + err.Error())
	}
	return ip, network
}

// Blocked reports whether ip must not be dialed. allowPrivate relaxes exactly
// the prefixes an operator means by "my LAN" (see entry.lan); it never
// relaxes metadataAddresses.
//
// A nil or malformed IP is blocked: callers reach this after a DNS lookup or
// a literal parse, and "we could not tell what this is" must fail closed.
func Blocked(ip net.IP, allowPrivate bool) bool {
	if ip == nil || (ip.To4() == nil && len(ip) != net.IPv6len) {
		return true
	}
	for _, metadata := range metadataIPs {
		if ip.Equal(metadata) {
			return true
		}
	}
	if v4 := embeddedV4(ip); v4 != nil {
		return blockedIn(blockedV4, v4, allowPrivate)
	}
	if v4 := ip.To4(); v4 != nil {
		return blockedIn(blockedV4, v4, allowPrivate)
	}
	if blockedIn(blockedV6, ip, allowPrivate) {
		return true
	}
	// Defense in depth for a v6 prefix the table above has not caught up to:
	// in strict mode anything Go does not consider global unicast is refused.
	// This cannot be applied in LAN mode — IsGlobalUnicast is already false
	// for loopback, which LAN mode exists to permit.
	return !allowPrivate && !ip.IsGlobalUnicast()
}

// embeddedV4 extracts the IPv4 address carried inside an IPv4-mapped
// (::ffff:0:0/96) or NAT64 (64:ff9b::/96) IPv6 address, so it can be judged
// against the IPv4 table. Returns nil for every other address.
func embeddedV4(ip net.IP) net.IP {
	if len(ip) != net.IPv6len {
		return nil
	}
	if ip.To4() != nil {
		// net.IP keeps IPv4-mapped addresses in 16-byte form and To4()
		// already unwraps them; handled by the caller's To4 branch.
		return nil
	}
	if nat64Prefix.Contains(ip) {
		return net.IPv4(ip[12], ip[13], ip[14], ip[15]).To4()
	}
	return nil
}

func blockedIn(table []entry, ip net.IP, allowPrivate bool) bool {
	for _, candidate := range table {
		if candidate.net.Contains(ip) {
			return !(allowPrivate && candidate.lan)
		}
	}
	return false
}
