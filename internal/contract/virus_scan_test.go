package contract

import "testing"

func TestValidateVirusScanAction(t *testing.T) {
	tests := []struct {
		name  string
		input VirusScanActionRequest
		valid bool
	}{
		{name: "full", input: VirusScanActionRequest{Mode: VirusScanModeFull}, valid: true},
		{name: "important", input: VirusScanActionRequest{Mode: VirusScanModeImportant}, valid: true},
		{name: "custom", input: VirusScanActionRequest{Mode: VirusScanModeCustom, Paths: []string{"/srv/sites", "/home"}}, valid: true},
		{name: "custom root", input: VirusScanActionRequest{Mode: VirusScanModeCustom, Paths: []string{"/"}}, valid: true},
		{name: "paths with full", input: VirusScanActionRequest{Mode: VirusScanModeFull, Paths: []string{"/home"}}},
		{name: "empty custom", input: VirusScanActionRequest{Mode: VirusScanModeCustom}},
		{name: "relative", input: VirusScanActionRequest{Mode: VirusScanModeCustom, Paths: []string{"home"}}},
		{name: "unclean", input: VirusScanActionRequest{Mode: VirusScanModeCustom, Paths: []string{"/home/../root"}}},
		{name: "comma", input: VirusScanActionRequest{Mode: VirusScanModeCustom, Paths: []string{"/srv/a,b"}}},
		{name: "control", input: VirusScanActionRequest{Mode: VirusScanModeCustom, Paths: []string{"/srv/a\tb"}}},
		{name: "duplicate", input: VirusScanActionRequest{Mode: VirusScanModeCustom, Paths: []string{"/home", "/home"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, _ := ValidateVirusScanAction(&test.input)
			if (field == "") != test.valid {
				t.Fatalf("valid=%v field=%q input=%#v", test.valid, field, test.input)
			}
		})
	}
}
