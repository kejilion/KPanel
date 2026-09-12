package contract

import "testing"

func TestParseSiteAddress(t *testing.T) {
	for _, tc := range []struct {
		input, host string
		port        int
	}{
		{"Example.COM", "example.com", 0}, {"Example.COM:08443", "example.com", 8443},
		{"192.168.1.10:8080", "192.168.1.10", 8080}, {"example.com:80", "example.com", 80},
		{"example.com:65535", "example.com", 65535},
	} {
		host, port, err := ParseSiteAddress(tc.input)
		if err != nil || host != tc.host || port != tc.port {
			t.Fatalf("%s: %q %d %v", tc.input, host, port, err)
		}
	}
	for _, input := range []string{"", " example.com", "example.com ", "example.com:0", "example.com:65536", "example.com:-1", "example.com:+80", "example.com:", "example.com:80/path", "https://example.com:8443", "a@b.com:80", "foo;touch:80", "../foo:80", "example..com:80", "example.com.:80", "192.168.1.1", "[::1]:8080", "x.com:80\n", "x.com:1e3", "x.com:000080"} {
		if _, _, err := ParseSiteAddress(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
