package panel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSiteCreateHTTPPortValidation(t *testing.T) {
	for _, address := range []string{"example.com", "example.com:8443", "192.168.1.10:8080"} {
		var input siteWriteInput
		data, _ := json.Marshal(map[string]any{"primaryDomain": address, "type": "static"})
		if err := json.Unmarshal(data, &input); err != nil {
			t.Fatal(err)
		}
		if field, detail := validateSiteWriteInput(&input, true); field != "" {
			t.Fatalf("%s: %s %s", address, field, detail)
		}
		if input.PrimaryDomain.Value != address {
			t.Fatalf("address lost: %s", input.PrimaryDomain.Value)
		}
	}
	for _, raw := range []string{
		`{"primaryDomain":"example.com:0","type":"static"}`,
		`{"primaryDomain":"example.com:65536","type":"static"}`,
		`{"primaryDomain":"example.com:8443","type":"static","certificate":"pem","privateKey":"pem"}`,
		`{"primaryDomain":"example.com:80;id","type":"static"}`,
	} {
		var input siteWriteInput
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			t.Fatal(err)
		}
		if field, _ := validateSiteWriteInput(&input, true); field == "" {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestSiteUpdateIPv4IdentityValidation(t *testing.T) {
	for _, tc := range []struct {
		host  string
		valid bool
	}{
		{"192.168.1.10", true}, {"example.com", true}, {"192.168.1.10:8443", false}, {"192.168.1.10/path", false},
	} {
		var input siteWriteInput
		data, _ := json.Marshal(map[string]any{"primaryDomain": tc.host, "type": "proxy", "upstream": "http://127.0.0.1:3001", "expectedResourceVersion": "sha256:" + strings.Repeat("b", 64)})
		if err := json.Unmarshal(data, &input); err != nil {
			t.Fatal(err)
		}
		if field, detail := validateSiteWriteInput(&input, false); (field == "") != tc.valid {
			t.Fatalf("%s: %s %s", tc.host, field, detail)
		}
	}
}
