package appmarket

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func newLocalAppTestService(t *testing.T) (*Service, string, string) {
	t.Helper()
	root := t.TempDir()
	official := filepath.Join(root, "apps")
	local := filepath.Join(root, "apps-local")
	for _, dir := range []string{official, local} {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	service.scriptAppRoot = official
	service.scriptLocalAppRoot = local
	service.fileOwnerTrusted = func(os.FileInfo) bool { return true }
	return service, official, local
}

func writeLocalAppConfig(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocalAppsJoinInventoryWithScriptMetadata(t *testing.T) {
	service, _, local := newLocalAppTestService(t)
	writeLocalAppConfig(t, local, "myapp.conf", `local app_name="我的应用"
local app_text='自建服务'
local app_url="https://example.com/myapp"
local docker_name="myapp"
local docker_port="18080"
docker_app_plus
`)
	before := len(service.catalog.Apps)

	inventory, err := service.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(service.catalog.Apps) != before {
		t.Fatal("local apps leaked into the shared catalog")
	}
	if inventory.Categories[len(inventory.Categories)-1].Key != localAppCategory {
		t.Fatalf("local category missing: %#v", inventory.Categories)
	}
	item, err := service.Find(context.Background(), "local-myapp")
	if err != nil {
		t.Fatal(err)
	}
	if item.Source != localAppSource || item.NameZH != "我的应用" || item.Description != "自建服务" ||
		item.Website != "https://example.com/myapp" || item.Icon != genericThirdPartyIcon ||
		item.Category != localAppCategory {
		t.Fatalf("unexpected local app metadata: %#v", item.App)
	}
	if item.DefaultPort != 18080 || !item.InstallPortConfigurable {
		t.Fatalf("local app spec was not read: %#v", item)
	}
	if selector, ok := service.scriptSelectorFor(item); !ok || selector != "myapp" {
		t.Fatalf("local app selector = %q, %v", selector, ok)
	}
}

func TestLocalAppsYieldToOfficialAndCatalogNames(t *testing.T) {
	service, official, local := newLocalAppTestService(t)
	writeLocalAppConfig(t, official, "shadowed.conf", "docker_name=a\ndocker_app_plus\n")
	writeLocalAppConfig(t, local, "shadowed.conf", "docker_name=b\ndocker_app_plus\n")
	taken := service.catalog.Apps[0].Token
	writeLocalAppConfig(t, local, taken+".conf", "docker_name=c\ndocker_app_plus\n")
	writeLocalAppConfig(t, local, "bad.name.conf", "docker_name=d\ndocker_app_plus\n")
	writeLocalAppConfig(t, local, "notes.txt", "docker_name=e\n")
	if err := os.Symlink(filepath.Join(local, "shadowed.conf"), filepath.Join(local, "linked.conf")); err != nil {
		t.Logf("symlink unsupported: %v", err)
	}

	catalog := service.withLocalApps(service.catalog)
	if len(catalog.Apps) != len(service.catalog.Apps) {
		for _, app := range catalog.Apps[len(service.catalog.Apps):] {
			t.Errorf("unexpected local app %q", app.ID)
		}
	}
	if len(catalog.Categories) != len(service.catalog.Categories) {
		t.Fatal("local category added without local apps")
	}
}

func TestLocalAppsRequireTrustedConfig(t *testing.T) {
	service, _, local := newLocalAppTestService(t)
	writeLocalAppConfig(t, local, "untrusted.conf", "docker_name=u\ndocker_app_plus\n")
	service.fileOwnerTrusted = func(os.FileInfo) bool { return false }
	if _, err := service.Find(context.Background(), "local-untrusted"); err != ErrNotFound {
		t.Fatalf("untrusted local config was listed: %v", err)
	}
}

func TestLocalAppsMissingDirectoryKeepsCatalog(t *testing.T) {
	service, _, _ := newLocalAppTestService(t)
	service.scriptLocalAppRoot = filepath.Join(t.TempDir(), "missing")
	catalog := service.withLocalApps(service.catalog)
	if len(catalog.Apps) != len(service.catalog.Apps) {
		t.Fatal("missing local directory changed the catalog")
	}
}

func TestLocalAppScriptActionsRequireLocalAppProtocol(t *testing.T) {
	service, _, local := newLocalAppTestService(t)
	writeLocalAppConfig(t, local, "myapp.conf", "docker_name=myapp\ndocker_app_plus\n")
	root := t.TempDir()
	if err := service.configureJobs(
		filepath.Join(root, "jobs"), filepath.Join(root, "kejilion-agent"), &fakeJobRunner{},
	); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "k")
	service.scriptInteractiveFinder = func() (string, error) { return script, nil }
	service.scriptInteractiveManageFinder = service.scriptInteractiveFinder
	service.scriptManageFinder = service.scriptInteractiveFinder

	for _, sample := range []struct {
		content string
		enabled bool
	}{
		{"#!/bin/bash\nKPANEL_APP_CONCURRENCY_PROTOCOL_VERSION=\"1\"\n", false},
		{"#!/bin/bash\nKPANEL_LOCAL_APPS_PROTOCOL_VERSION=\"1\"\r\n", true},
	} {
		if err := os.WriteFile(script, []byte(sample.content), 0o755); err != nil {
			t.Fatal(err)
		}
		item, err := service.Find(context.Background(), "local-myapp")
		if err != nil {
			t.Fatal(err)
		}
		install := item.Capabilities["install"]
		if install.Enabled != sample.enabled ||
			(!sample.enabled && install.Reason != localAppScriptOutdated) {
			t.Fatalf("script %q: install capability = %#v", sample.content, install)
		}
	}
}
