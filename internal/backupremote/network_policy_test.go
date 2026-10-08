package backupremote

import (
	"net/netip"
	"testing"
)

func TestBackupAddressPolicyKeepsPrivateNAS(t *testing.T) {
	tests := []struct {
		address string
		allowed bool
	}{
		{"8.8.8.8", true},
		{"2606:4700:4700::1111", true},
		{"192.168.1.5", true},
		{"10.1.2.3", true},
		{"172.16.0.1", true},
		{"fd00::5", true},
		{"::ffff:8.8.8.8", true},
		{"::ffff:192.168.1.5", true},
		{"192.31.196.1", true},
		{"192.52.193.1", true},
		{"192.175.48.1", true},
		{"2001:3::1", true},
		{"2001:4:112::1", true},
		{"2620:4f:8000::1", true},
		{"127.0.0.1", false},
		{"::1", false},
		{"100.64.0.1", false},
		{"100.100.100.200", false},
		{"169.254.169.254", false},
		{"168.63.129.16", false},
		{"fd00:ec2::254", false},
		{"0.0.0.0", false},
		{"224.0.0.1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:100.64.0.1", false},
		{"::ffff:168.63.129.16", false},
		{"0.0.0.1", false},
		{"192.0.0.1", false},
		{"192.0.2.1", false},
		{"198.18.0.1", false},
		{"198.51.100.1", false},
		{"203.0.113.10", false},
		{"240.0.0.1", false},
		{"::ffff:203.0.113.10", false},
		{"100::1", false},
		{"100:0:0:1::1", false},
		{"fec0::1", false},
		{"2001:2::1", false},
		{"2001:10::1", false},
		{"2001:20::1", false},
		{"2001:db8::1", false},
		{"3fff::1", false},
		{"5f00::1", false},
		{"::7f00:1", false},
		{"64:ff9b::7f00:1", false},
		{"64:ff9b:1::a9fe:a9fe", false},
		{"2002:7f00:1::", false},
		{"2001:0:4136:e378:8000:63bf:3fff:fdd2", false},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			if got := allowedIP(netip.MustParseAddr(test.address)); got != test.allowed {
				t.Fatalf("allowedIP(%s) = %v, want %v", test.address, got, test.allowed)
			}
		})
	}
	if allowedIP(netip.Addr{}) {
		t.Fatal("invalid address allowed")
	}
}

func TestBackupPrivateNASURLConfigurationRemainsValid(t *testing.T) {
	for _, kind := range []string{"webdav", "s3"} {
		for _, endpoint := range []string{"http://192.168.1.5:9000", "https://10.1.2.3:9443", "http://[fd00::5]:9000"} {
			t.Run(kind+" "+endpoint, func(t *testing.T) {
				storage := testStorage()
				storage.Kind, storage.Endpoint = kind, endpoint
				if kind == "s3" {
					storage.Bucket, storage.AccessKey, storage.PathStyle = "bucket", "fixture", true
				}
				client, err := NewClient(storage)
				if err != nil {
					t.Fatalf("private NAS configuration rejected: %v", err)
				}
				client.Close()
			})
		}
	}
}
