package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSystemResourceActionRejectsActionInapplicableJSONFields(t *testing.T) {
	var request SystemResourceActionRequest
	err := json.Unmarshal([]byte(`{
		"action":"firewall-open-all",
		"port":0,
		"expectedResourceVersion":"`+strings.Repeat("a", 64)+`"
	}`), &request)
	if err != nil {
		t.Fatal(err)
	}
	field, _ := ValidateSystemResourceAction(&request)
	if field != "port" {
		t.Fatalf("inapplicable zero field was accepted: field=%q request=%#v", field, request)
	}

	if err := json.Unmarshal([]byte(`{"action":"firewall-open-all","unknown":true}`), &request); err == nil {
		t.Fatal("unknown JSON field was accepted")
	}
}

func TestValidateSystemResourceActionBoundsSensitiveInputs(t *testing.T) {
	version := strings.Repeat("a", 64)
	trueValue := true
	tests := []struct {
		name    string
		request SystemResourceActionRequest
		field   string
	}{
		{
			name: "hosts IPv6",
			request: SystemResourceActionRequest{
				Action: "hosts-add", Address: "2001:db8::1", Hostnames: []string{"host.example"},
				ExpectedResourceVersion: version,
			},
		},
		{
			name: "too many hostnames",
			request: SystemResourceActionRequest{
				Action: "hosts-add", Address: "192.0.2.1", Hostnames: make([]string, 17),
				ExpectedResourceVersion: version,
			},
			field: "hostnames",
		},
		{
			name: "underscore hostname",
			request: SystemResourceActionRequest{
				Action: "hosts-add", Address: "192.0.2.1", Hostnames: []string{"foo_bar"},
				ExpectedResourceVersion: version,
			},
			field: "hostnames",
		},
		{
			name: "empty hostname label",
			request: SystemResourceActionRequest{
				Action: "hosts-add", Address: "192.0.2.1", Hostnames: []string{"a..b"},
				ExpectedResourceVersion: version,
			},
			field: "hostnames",
		},
		{
			name: "quartz cron",
			request: SystemResourceActionRequest{
				Action: "cron-add", Expression: "0 0 ? * *", Command: "true",
				ExpectedResourceVersion: version,
			},
			field: "expression.dayOfMonth",
		},
		{
			name: "minute out of range",
			request: SystemResourceActionRequest{
				Action: "cron-add", Expression: "60 * * * *", Command: "true",
				ExpectedResourceVersion: version,
			},
			field: "expression.minute",
		},
		{
			name: "quartz day number",
			request: SystemResourceActionRequest{
				Action: "cron-add", Expression: "0 0 * * MON#2", Command: "true",
				ExpectedResourceVersion: version,
			},
			field: "expression.dayOfWeek",
		},
		{
			name: "quartz last day",
			request: SystemResourceActionRequest{
				Action: "cron-add", Expression: "0 0 L * *", Command: "true",
				ExpectedResourceVersion: version,
			},
			field: "expression.dayOfMonth",
		},
		{
			name: "quartz nearest weekday",
			request: SystemResourceActionRequest{
				Action: "cron-add", Expression: "0 0 15W * *", Command: "true",
				ExpectedResourceVersion: version,
			},
			field: "expression.dayOfMonth",
		},
		{
			name: "named month and weekday",
			request: SystemResourceActionRequest{
				Action: "cron-add", Expression: "*/15 0-23/2 * JAN,MAR MON-FRI", Command: "true",
				ExpectedResourceVersion: version,
			},
		},
		{
			name: "blank cron command",
			request: SystemResourceActionRequest{
				Action: "cron-add", Expression: "0 0 * * *", Command: "   ",
				ExpectedResourceVersion: version,
			},
			field: "command",
		},
		{
			name: "oversized cron command",
			request: SystemResourceActionRequest{
				Action: "cron-add", Expression: "0 0 * * *", Command: strings.Repeat("x", SystemResourceMaxCommand+1),
				ExpectedResourceVersion: version,
			},
			field: "command",
		},
		{
			name: "IPv6 firewall",
			request: SystemResourceActionRequest{
				Action: "firewall-block-ip", Address: "2001:db8::1", ExpectedResourceVersion: version,
			},
			field: "address",
		},
		{
			name: "IPv4 firewall CIDR",
			request: SystemResourceActionRequest{
				Action: "firewall-allow-ip", Address: "192.0.2.9/24", ExpectedResourceVersion: version,
			},
		},
		{
			name: "invalid country code",
			request: SystemResourceActionRequest{
				Action: "firewall-block-country", CountryCode: "USA", ExpectedResourceVersion: version,
			},
			field: "countryCode",
		},
		{
			name: "interface state",
			request: SystemResourceActionRequest{
				Action: "network-interface-state", InterfaceName: "lo", Enabled: &trueValue,
				ExpectedResourceVersion: version,
			},
		},
		{
			name: "uppercase version",
			request: SystemResourceActionRequest{
				Action: "firewall-open-all", ExpectedResourceVersion: strings.Repeat("A", 64),
			},
			field: "expectedResourceVersion",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, detail := ValidateSystemResourceAction(&test.request)
			if field != test.field {
				t.Fatalf("field=%q detail=%q want=%q", field, detail, test.field)
			}
		})
	}
}

func TestValidateSystemResourceActionNormalizesCountryCode(t *testing.T) {
	request := SystemResourceActionRequest{
		Action: "firewall-block-country", CountryCode: " us ", ExpectedResourceVersion: strings.Repeat("a", 64),
	}
	field, detail := ValidateSystemResourceAction(&request)
	if field != "" || detail != "" || request.CountryCode != "US" {
		t.Fatalf("field=%q detail=%q countryCode=%q", field, detail, request.CountryCode)
	}
}
