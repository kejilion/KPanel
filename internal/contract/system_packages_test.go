package contract

import "testing"

func TestValidateSystemPackagesAction(t *testing.T) {
	version := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	valid := SystemPackagesActionRequest{Action: "install", Items: []string{"curl", "htop"}, ExpectedResourceVersion: version}
	if field, detail := ValidateSystemPackagesAction(&valid); field != "" {
		t.Fatalf("valid request rejected: %s %s", field, detail)
	}
	duplicate := valid
	duplicate.Items = []string{"curl", "curl"}
	if field, _ := ValidateSystemPackagesAction(&duplicate); field != "items" {
		t.Fatalf("duplicate field = %q, want items", field)
	}
	unknown := valid
	unknown.Items = []string{"curl; reboot"}
	if field, _ := ValidateSystemPackagesAction(&unknown); field != "items" {
		t.Fatalf("unknown package field = %q, want items", field)
	}
	badAction := valid
	badAction.Action = "upgrade"
	if field, _ := ValidateSystemPackagesAction(&badAction); field != "action" {
		t.Fatalf("bad action field = %q, want action", field)
	}
}

func TestSystemPackageCatalogExcludesNoveltyAndGamePackages(t *testing.T) {
	if len(SystemPackageIDs) != 17 {
		t.Fatalf("catalog size = %d, want 17", len(SystemPackageIDs))
	}
	for _, item := range []string{"cmatrix", "sl", "bastet", "nsnake", "ninvaders"} {
		if IsSystemPackage(item) {
			t.Fatalf("novelty or game package %q remains supported", item)
		}
	}
}
