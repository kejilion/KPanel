package cluster

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

func TestClusterAddressPolicyKeepsPrivateAllowlist(t *testing.T) {
	tests := []struct {
		address string
		cidrs   []string
		allowed bool
	}{
		{"8.8.8.8", nil, true},
		{"2606:4700:4700::1111", nil, true},
		{"::ffff:8.8.8.8", nil, true},
		{"192.31.196.1", nil, true},
		{"192.52.193.1", nil, true},
		{"192.175.48.1", nil, true},
		{"2001:3::1", nil, true},
		{"2001:4:112::1", nil, true},
		{"2620:4f:8000::1", nil, true},
		{"10.1.2.3", nil, false},
		{"10.1.2.3", []string{"10.1.0.0/16"}, true},
		{"10.2.2.3", []string{"10.1.0.0/16"}, false},
		{"192.168.1.5", []string{"192.168.1.0/24"}, true},
		{"fd00::5", []string{"fd00::/8"}, true},
		{"100.64.0.1", nil, false},
		{"100.64.0.1", []string{"100.64.0.0/10"}, true},
		{"::ffff:10.1.2.3", []string{"10.1.0.0/16"}, true},
		{"::ffff:100.64.0.1", []string{"100.64.0.0/10"}, true},
		{"127.0.0.1", []string{"0.0.0.0/0"}, false},
		{"::1", []string{"::/0"}, false},
		{"169.254.169.254", []string{"0.0.0.0/0"}, false},
		{"168.63.129.16", []string{"0.0.0.0/0"}, false},
		{"100.100.100.200", []string{"100.64.0.0/10"}, false},
		{"fd00:ec2::254", []string{"fd00::/8"}, false},
		{"::ffff:168.63.129.16", []string{"0.0.0.0/0"}, false},
		{"::ffff:100.100.100.200", []string{"100.64.0.0/10"}, false},
		{"203.0.113.10", []string{"0.0.0.0/0"}, false},
		{"64:ff9b::7f00:1", []string{"::/0"}, false},
		{"100:0:0:1::1", []string{"::/0"}, false},
		{"3fff::1", []string{"::/0"}, false},
		{"5f00::1", []string{"::/0"}, false},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			client, err := NewRemoteClient(RemoteClientConfig{
				PrivateCIDRs: test.cidrs,
				Resolver:     staticResolver{"panel.example.com": {net.ParseIP(test.address)}},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, host := range []string{test.address, "panel.example.com"} {
				addresses, err := client.resolve(context.Background(), host)
				if !test.allowed {
					if !errors.Is(err, ErrPrivateOrigin) {
						t.Fatalf("resolve(%s) = %v, %v; want ErrPrivateOrigin", host, addresses, err)
					}
				} else if err != nil || len(addresses) != 1 || addresses[0] != netip.MustParseAddr(test.address).Unmap() {
					t.Fatalf("resolve(%s) = %v, %v; want permitted canonical address", host, addresses, err)
				}
			}
		})
	}
}

func TestClusterMixedMetadataAnswerNeverReachesDialer(t *testing.T) {
	for _, address := range []string{"168.63.129.16", "100.100.100.200", "fd00:ec2::254", "3fff::1"} {
		t.Run(address, func(t *testing.T) {
			calls := 0
			client, err := NewRemoteClient(RemoteClientConfig{
				PrivateCIDRs: []string{"0.0.0.0/0", "::/0"},
				Resolver:     staticResolver{"panel.example.com": {net.ParseIP("8.8.8.8"), net.ParseIP(address)}},
				Dialer: func(context.Context, string, string) (net.Conn, error) {
					calls++
					return nil, errors.New("test dial stopped")
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.dialContext(context.Background(), "tcp", "panel.example.com:443"); !errors.Is(err, ErrPrivateOrigin) || calls != 0 {
				t.Fatalf("mixed answer reached dialer: calls=%d err=%v", calls, err)
			}
		})
	}
}
