package systemmanage

import (
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestVirusScanPolicyRoundTrip(t *testing.T) {
	input := contract.VirusScanActionRequest{Mode: contract.VirusScanModeCustom, Paths: []string{"/srv/sites", "/home"}}
	policy, err := encodeVirusScanPolicy(input)
	if err != nil {
		t.Fatal(err)
	}
	mode, paths, ok := parseVirusScanPolicy(policy)
	if !ok || mode != input.Mode || strings.Join(paths, ",") != strings.Join(input.Paths, ",") {
		t.Fatalf("round trip = %q %#v %v", mode, paths, ok)
	}
	if _, _, ok := parseVirusScanPolicy("custom.not-base64!"); ok {
		t.Fatal("invalid custom policy was accepted")
	}
}

func TestParseVirusScanReceipt(t *testing.T) {
	receipt, err := parseVirusScanReceipt([]byte("business log\nKPANEL_VIRUS_SCAN_PROTOCOL 1\nKPANEL_VIRUS_SCAN_STATUS=infected\nKPANEL_VIRUS_SCAN_MODE=important\nKPANEL_VIRUS_SCAN_SCANNED=42\nKPANEL_VIRUS_SCAN_INFECTED=2\nKPANEL_VIRUS_SCAN_ERRORS=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "infected" || receipt.Mode != "important" || receipt.ScannedFiles != 42 || receipt.InfectedFiles != 2 || receipt.Errors != 1 {
		t.Fatalf("receipt = %#v", receipt)
	}
	if _, err := parseVirusScanReceipt([]byte("KPANEL_VIRUS_SCAN_STATUS=clean\n")); err == nil {
		t.Fatal("receipt without protocol header was accepted")
	}
}

func TestParseVirusScanReportMapsBoundedFindings(t *testing.T) {
	snapshot := contract.VirusScanSnapshot{Paths: []string{"/srv/sites"}, Findings: []string{}}
	parseVirusScanReport(&snapshot, "/mnt/scan/0/site/eicar.txt: Win.Test FOUND\nScanned files: 17\nInfected files: 1\nTotal errors: 0\n")
	if snapshot.Status != "infected" || snapshot.ScannedFiles != 17 || snapshot.InfectedFiles != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if len(snapshot.Findings) != 1 || snapshot.Findings[0] != "/srv/sites/site/eicar.txt: Win.Test FOUND" {
		t.Fatalf("findings = %#v", snapshot.Findings)
	}
}

func TestTrustedVirusScanRequiresProtocolMarkers(t *testing.T) {
	content := []byte("permission_granted=\"true\"\nKPANEL_SYSTEM_RESOURCE_PROTOCOL_VERSION=\"4\"\nKJ_SYSTEM_RESOURCE_NONINTERACTIVE\nkpanel_system_resource_dispatch\nKPANEL_SYSTEM_RESOURCE_STATUS\nKPANEL_SYSTEM_RESOURCE_VERSION\nKPANEL_VIRUS_SCAN_PROTOCOL_VERSION=\"1\"\nKJ_VIRUS_SCAN_NONINTERACTIVE\nkpanel_virus_scan_dispatch\n# " + strings.Repeat("x", 1200))
	if !trustedKejilionVirusScanContent(content) {
		t.Fatal("current virus scan protocol markers were rejected")
	}
	if trustedKejilionVirusScanContent([]byte(strings.ReplaceAll(string(content), "kpanel_virus_scan_dispatch", "missing"))) {
		t.Fatal("script without virus scan dispatcher was accepted")
	}
}

func TestVirusScanMaintenancePlanUsesFixedOperations(t *testing.T) {
	manager, _, _, _ := testManager(t, &fakeRunner{})
	policy, err := encodeVirusScanPolicy(contract.VirusScanActionRequest{Mode: contract.VirusScanModeCustom, Paths: []string{"/srv/sites"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.writeMaintenance(contract.SystemMaintenanceSummary{ID: "scan-test", State: "running", Action: "virus-scan", Policy: policy}); err != nil {
		t.Fatal(err)
	}
	action, returnedPolicy, steps, err := manager.maintenanceSteps("virus-scan")
	if err != nil {
		t.Fatal(err)
	}
	if action != "virus-scan" || returnedPolicy != policy || len(steps) != 2 || steps[0].operation != maintenanceOperationVirusDB || steps[1].operation != maintenanceOperationVirusScan {
		t.Fatalf("plan = %q %q %#v", action, returnedPolicy, steps)
	}
}
