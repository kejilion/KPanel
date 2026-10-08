// Package netpolicy classifies outbound IP addresses. Callers retain their own
// authorization rules for local services, private networks and shared networks.
package netpolicy

import "net/netip"

type AddressClass uint8

const (
	Invalid AddressClass = iota
	Public
	Private
	Shared
	Loopback
	LinkLocal
	Multicast
	Unspecified
	Metadata
	Transition
	Reserved
)

// Classify normalizes IPv4-mapped addresses before applying the same rules as
// native IPv4. Public identifies an ordinary service destination under this
// policy; it does not guarantee routability. This is not a blanket exclusion of
// the IANA special-purpose registries: globally reachable service ranges remain
// available unless an existing adapter policy already excluded them.
func Classify(address netip.Addr) AddressClass {
	if !address.IsValid() || address.Zone() != "" {
		return Invalid
	}
	address = address.Unmap()
	if address.IsUnspecified() {
		return Unspecified
	}
	if address.IsLoopback() {
		return Loopback
	}
	if address.IsMulticast() {
		return Multicast
	}
	for _, candidate := range metadataAddresses {
		if address == candidate {
			return Metadata
		}
	}
	if address.IsLinkLocalUnicast() {
		return LinkLocal
	}
	for _, prefix := range transitionPrefixes {
		if prefix.Contains(address) {
			return Transition
		}
	}
	if !address.IsGlobalUnicast() {
		return Reserved
	}
	for _, prefix := range reservedPrefixes {
		if prefix.Contains(address) {
			return Reserved
		}
	}
	// Metadata must be classified before the private/CGNAT exceptions so an
	// adapter's network allowlist cannot enable host-node services.
	if address.IsPrivate() {
		return Private
	}
	if sharedPrefix.Contains(address) {
		return Shared
	}
	return Public
}

var (
	sharedPrefix      = netip.MustParsePrefix("100.64.0.0/10")
	metadataAddresses = [...]netip.Addr{
		netip.MustParseAddr("169.254.169.254"),
		netip.MustParseAddr("100.100.100.200"),
		// Azure WireServer uses a public address for host-node services.
		netip.MustParseAddr("168.63.129.16"),
		netip.MustParseAddr("fd00:ec2::254"),
	}
	transitionPrefixes = [...]netip.Prefix{
		netip.MustParsePrefix("::/96"),
		netip.MustParsePrefix("64:ff9b::/96"),
		netip.MustParsePrefix("64:ff9b:1::/48"),
		netip.MustParsePrefix("2001::/32"),
		netip.MustParsePrefix("2002::/16"),
	}
	reservedPrefixes = [...]netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("100::/64"),
		netip.MustParsePrefix("100:0:0:1::/64"),
		netip.MustParsePrefix("fec0::/10"),
		netip.MustParsePrefix("2001:2::/48"),
		netip.MustParsePrefix("2001:10::/28"),
		netip.MustParsePrefix("2001:20::/28"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("3fff::/20"),
		netip.MustParsePrefix("5f00::/16"),
	}
)
