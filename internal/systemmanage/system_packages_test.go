package systemmanage

import (
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestSystemPackageCatalogMatchesContractOrder(t *testing.T) {
	if len(systemPackageCatalog) != len(contract.SystemPackageIDs) {
		t.Fatalf("catalog size = %d, want %d", len(systemPackageCatalog), len(contract.SystemPackageIDs))
	}
	for index, definition := range systemPackageCatalog {
		if definition.id != contract.SystemPackageIDs[index] {
			t.Fatalf("catalog[%d] = %q, want %q", index, definition.id, contract.SystemPackageIDs[index])
		}
	}
}

func TestSystemPackagesPolicyRoundTrip(t *testing.T) {
	request := contract.SystemPackagesActionRequest{
		Action: "install", Items: []string{"curl", "htop"},
		ExpectedResourceVersion: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	parsed, ok := parseSystemPackagesPolicy(systemPackagesPolicy(request))
	if !ok || parsed.Action != request.Action || parsed.ExpectedResourceVersion != request.ExpectedResourceVersion || len(parsed.Items) != 2 || parsed.Items[1] != "htop" {
		t.Fatalf("parsed policy = %#v, ok=%t", parsed, ok)
	}
	if _, ok := parseSystemPackagesPolicy("install.bad.curl;reboot"); ok {
		t.Fatal("unsafe package policy was accepted")
	}
}

func TestSystemPackageArgumentsUseFixedNativeInvocations(t *testing.T) {
	tests := []struct {
		kind    packageManagerKind
		action  string
		wantArg string
	}{
		{packageManagerAPT, "install", "install"},
		{packageManagerAPT, "remove", "purge"},
		{packageManagerDNF, "remove", "remove"},
		{packageManagerAPK, "install", "add"},
		{packageManagerPacman, "install", "-S"},
		{packageManagerZypper, "remove", "remove"},
	}
	for _, test := range tests {
		arguments := systemPackageArguments(test.kind, test.action, "curl")
		found, packageCount := false, 0
		for _, argument := range arguments {
			if argument == test.wantArg {
				found = true
			}
			if argument == "curl" {
				packageCount++
			}
		}
		if !found || packageCount != 1 {
			t.Fatalf("kind=%s action=%s arguments=%#v", test.kind, test.action, arguments)
		}
	}
	if arguments := systemPackageArguments(packageManagerUnknown, "install", "curl"); arguments != nil {
		t.Fatalf("unknown manager arguments = %#v", arguments)
	}
}
