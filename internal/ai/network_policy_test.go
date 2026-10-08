package ai

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderPublicAddressPolicy(t *testing.T) {
	tests := []struct {
		address string
		public  bool
	}{
		{"8.8.8.8", true},
		{"2606:4700:4700::1111", true},
		{"::ffff:8.8.8.8", true},
		{"192.31.196.1", true},
		{"192.52.193.1", true},
		{"192.175.48.1", true},
		{"2001:3::1", true},
		{"2001:4:112::1", true},
		{"2620:4f:8000::1", true},
		{"127.0.0.1", false},
		{"::1", false},
		{"10.1.2.3", false},
		{"192.168.1.5", false},
		{"fd00::5", false},
		{"100.64.0.1", false},
		{"100.100.100.200", false},
		{"169.254.169.254", false},
		{"168.63.129.16", false},
		{"fd00:ec2::254", false},
		{"0.0.0.0", false},
		{"224.0.0.1", false},
		{"203.0.113.10", false},
		{"198.18.0.1", false},
		{"2001:db8::1", false},
		{"3fff::1", false},
		{"::ffff:127.0.0.1", false},
		{"::ffff:100.64.0.1", false},
		{"::ffff:168.63.129.16", false},
		{"::ffff:203.0.113.10", false},
		{"::7f00:1", false},
		{"64:ff9b::7f00:1", false},
		{"64:ff9b:1::a9fe:a9fe", false},
		{"2002:7f00:1::", false},
		{"2001:0:4136:e378:8000:63bf:3fff:fdd2", false},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			host := test.address
			if strings.Contains(host, ":") {
				host = "[" + host + "]"
			}
			for _, scope := range []EndpointScope{EndpointPublic, EndpointPrivate} {
				// Private scope remains the explicit authorization for local
				// providers; it does not adopt the public destination policy.
				want := test.public || scope == EndpointPrivate
				if _, err := ValidateProviderURL("https://"+host+"/v1", scope); (err == nil) != want {
					t.Fatalf("literal URL scope=%s: err=%v, want allowed=%v", scope, err, want)
				}
				if err := ValidateResolvedAddresses(scope, []net.IPAddr{{IP: net.ParseIP(test.address)}}); (err == nil) != want {
					t.Fatalf("DNS address scope=%s: err=%v, want allowed=%v", scope, err, want)
				}
			}
		})
	}
}

func TestProviderPublicDNSRejectsEntireMixedAnswer(t *testing.T) {
	public := net.IPAddr{IP: net.ParseIP("8.8.8.8")}
	for _, blocked := range []string{"100.64.0.1", "168.63.129.16", "203.0.113.10", "64:ff9b::7f00:1"} {
		addresses := []net.IPAddr{public, {IP: net.ParseIP(blocked)}}
		if err := ValidateResolvedAddresses(EndpointPublic, addresses); err == nil {
			t.Errorf("mixed DNS answer allowed: %s", blocked)
		}
		if err := ValidateResolvedAddresses(EndpointPrivate, addresses); err != nil {
			t.Errorf("explicit private policy changed: %v", err)
		}
	}
	for _, addresses := range [][]net.IPAddr{nil, {{IP: nil}}, {{IP: net.IP{1, 2, 3}}}} {
		if err := ValidateResolvedAddresses(EndpointPublic, addresses); err == nil {
			t.Errorf("invalid public DNS answer allowed: %v", addresses)
		}
	}
}

func TestConfirmedPrivateProviderKeepsLocalEndpoints(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenStore(filepath.Join(dir, "ai.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	box, err := OpenSecretBox(filepath.Join(dir, "ai-secrets.key"), false)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewProviderService(store, box)
	if err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{
		"http://127.0.0.1:11434/v1", "http://192.168.1.5:11434/v1",
		"http://[fd00::5]:11434/v1", "http://100.64.0.1:11434/v1",
	} {
		provider, err := service.Save(context.Background(), "", ProviderInput{
			Name: "self-hosted", Protocol: ProtocolOpenAICompatible, BaseURL: endpoint,
			EndpointScope: EndpointPrivate, PrivateConfirmed: true, Enabled: true,
		})
		if err != nil || provider.BaseURL != endpoint || provider.EndpointScope != EndpointPrivate {
			t.Fatalf("private endpoint %s changed: provider=%#v err=%v", endpoint, provider, err)
		}
	}
}
