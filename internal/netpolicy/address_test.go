package netpolicy

import (
	"net/netip"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		address string
		want    AddressClass
	}{
		{"8.8.8.8", Public},
		{"2606:4700:4700::1111", Public},
		{"10.1.2.3", Private},
		{"172.16.0.1", Private},
		{"172.31.255.254", Private},
		{"192.168.1.5", Private},
		{"fc00::1", Private},
		{"fd00::5", Private},
		{"100.64.0.1", Shared},
		{"100.127.255.255", Shared},
		{"100.63.255.255", Public},
		{"100.128.0.0", Public},
		{"172.15.255.255", Public},
		{"172.32.0.1", Public},
		{"127.0.0.1", Loopback},
		{"127.255.255.254", Loopback},
		{"::1", Loopback},
		{"169.254.1.1", LinkLocal},
		{"fe80::1", LinkLocal},
		{"224.0.0.1", Multicast},
		{"ff02::1", Multicast},
		{"0.0.0.0", Unspecified},
		{"::", Unspecified},
		{"169.254.169.254", Metadata},
		{"100.100.100.200", Metadata},
		{"168.63.129.16", Metadata},
		{"fd00:ec2::254", Metadata},
		{"::7f00:1", Transition},
		{"64:ff9b::7f00:1", Transition},
		{"64:ff9b::808:808", Transition},
		{"64:ff9b:1::a9fe:a9fe", Transition},
		{"2002:7f00:1::", Transition},
		{"2001:0:4136:e378:8000:63bf:3fff:fdd2", Transition},
		{"0.0.0.1", Reserved},
		{"192.0.0.1", Reserved},
		{"192.0.2.1", Reserved},
		{"198.18.0.1", Reserved},
		{"198.19.255.255", Reserved},
		{"198.51.100.1", Reserved},
		{"203.0.113.10", Reserved},
		{"240.0.0.1", Reserved},
		{"255.255.255.255", Reserved},
		{"100::1", Reserved},
		{"100:0:0:1::1", Reserved},
		{"fec0::1", Reserved},
		{"2001:2::1", Reserved},
		{"2001:10::1", Reserved},
		{"2001:20::1", Reserved},
		{"2001:db8::1", Reserved},
		{"3fff::1", Reserved},
		{"3fff:fff::1", Reserved},
		{"5f00::1", Reserved},
		{"3fff:1000::1", Public},
		{"5f01::1", Public},
		{"64:ff9b:2::1", Public},
		// Special-purpose does not mean unavailable for ordinary public
		// clients. Preserve globally reachable service ranges that the
		// existing cluster/backup policies already accepted.
		{"192.31.196.1", Public},
		{"192.52.193.1", Public},
		{"192.175.48.1", Public},
		{"2001:3::1", Public},
		{"2001:4:112::1", Public},
		{"2620:4f:8000::1", Public},
		{"fe80::1%eth0", Invalid},
		{"2606:4700:4700::1111%eth0", Invalid},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			if got := Classify(netip.MustParseAddr(test.address)); got != test.want {
				t.Fatalf("Classify(%s) = %v, want %v", test.address, got, test.want)
			}
		})
	}
	if got := Classify(netip.Addr{}); got != Invalid {
		t.Fatalf("Classify(invalid) = %v", got)
	}
}

func TestClassifyIPv4MappedUsesEmbeddedAddress(t *testing.T) {
	for _, address := range []string{
		"8.8.8.8", "10.1.2.3", "192.168.1.5", "100.64.0.1",
		"127.0.0.1", "169.254.1.1", "224.0.0.1", "0.0.0.0",
		"169.254.169.254", "168.63.129.16", "100.100.100.200", "203.0.113.10",
	} {
		t.Run(address, func(t *testing.T) {
			native := netip.MustParseAddr(address)
			mapped := netip.MustParseAddr("::ffff:" + address)
			if got, want := Classify(mapped), Classify(native); got != want {
				t.Fatalf("mapped class = %v, native class = %v", got, want)
			}
		})
	}
}
