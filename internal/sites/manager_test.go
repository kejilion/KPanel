//go:build linux

package sites

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type fakeNginxController struct {
	mu         sync.Mutex
	readyErr   error
	testErrs   []error
	reloadErrs []error
	tests      int
	reloads    int
	dropDomain string
	dropResult bool
	dropErr    error
}

type fakeSiteScriptDeleter struct {
	root    string
	domain  string
	outcome scriptDeleteOutcome
	err     error
}

func (f *fakeSiteScriptDeleter) Delete(
	_ context.Context,
	domain string,
) (scriptDeleteOutcome, error) {
	f.domain = domain
	if f.err != nil {
		return scriptDeleteOutcome{}, f.err
	}
	for _, path := range []string{
		filepath.Join(f.root, "conf.d", domain+".conf"),
		filepath.Join(f.root, "html", domain),
		filepath.Join(f.root, "certs", domain+"_cert.pem"),
		filepath.Join(f.root, "certs", domain+"_key.pem"),
	} {
		if err := os.RemoveAll(path); err != nil {
			return scriptDeleteOutcome{}, err
		}
	}
	return f.outcome, nil
}

func (f *fakeNginxController) NginxReady(context.Context) error {
	return f.readyErr
}

func (f *fakeNginxController) NginxTest(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := f.tests
	f.tests++
	if index < len(f.testErrs) {
		return f.testErrs[index]
	}
	return nil
}

func (f *fakeNginxController) NginxReload(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	index := f.reloads
	f.reloads++
	if index < len(f.reloadErrs) {
		return f.reloadErrs[index]
	}
	return nil
}

func (f *fakeNginxController) DropSiteDatabase(_ context.Context, domain string) (bool, error) {
	f.dropDomain = domain
	return f.dropResult, f.dropErr
}

func TestCreateStaticProducesCanonicalDiscoverableArtifacts(t *testing.T) {
	manager, nginx, root := newTestManager(t)
	enabled := true
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "example.com",
		Aliases:       []string{"www.example.com"},
		Type:          "static",
		Enabled:       &enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Origin != contract.OriginWeb || !containsString(created.AllowedActions, "update") {
		t.Fatalf("created site is not managed: %#v", created)
	}
	configPath := filepath.Join(root, "conf.d", "example.com.conf")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(config, []byte(managedMarker+"\n")) {
		t.Fatalf("managed marker missing: %s", config)
	}
	index, err := os.ReadFile(filepath.Join(root, "html", "example.com", "index.html"))
	if err != nil || !bytes.Equal(index, renderDefaultIndex("example.com")) {
		t.Fatalf("unexpected default index: err=%v body=%s", err, index)
	}
	discovered, err := NewDiscoverer(root).Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(discovered) != 1 || discovered[0].ID != created.ID ||
		discovered[0].ResourceVersion != created.ResourceVersion ||
		discovered[0].Origin != contract.OriginWeb {
		t.Fatalf("bidirectional discovery mismatch: %#v", discovered)
	}
	if nginx.tests != 2 || nginx.reloads != 1 {
		t.Fatalf("nginx calls tests=%d reloads=%d, want 2/1", nginx.tests, nginx.reloads)
	}
}

func TestCreateAllSupportedSiteServices(t *testing.T) {
	tests := []struct {
		name       string
		input      SiteInput
		wantKind   contract.SiteKind
		wantTarget string
		indexName  string
	}{
		{
			name: "php latest",
			input: SiteInput{
				PrimaryDomain: "php.example.com", Type: "php", PHPVersion: "latest",
			},
			wantKind: contract.SitePHP, wantTarget: "php", indexName: "index.php",
		},
		{
			name: "php 7.4",
			input: SiteInput{
				PrimaryDomain: "php74.example.com", Type: "php", PHPVersion: "7.4",
			},
			wantKind: contract.SitePHP, wantTarget: "php74", indexName: "index.php",
		},
		{
			name: "IP and port proxy",
			input: SiteInput{
				PrimaryDomain: "proxy.example.com", Type: "proxy", Upstream: "http://8.8.8.8:3000",
			},
			wantKind: contract.SiteReverseProxy, wantTarget: "http://8.8.8.8:3000",
		},
		{
			name: "domain proxy",
			input: SiteInput{
				PrimaryDomain: "edge.example.com", Type: "proxy_domain", Upstream: "https://origin.example.net",
			},
			wantKind: contract.SiteDomainProxy, wantTarget: "https://origin.example.net",
		},
		{
			name: "load balance",
			input: SiteInput{
				PrimaryDomain: "balanced.example.com", Type: "load_balance",
				Upstreams: []string{"http://10.0.0.12:8080", "http://10.0.0.11:8080"},
			},
			wantKind:   contract.SiteLoadBalance,
			wantTarget: "http://10.0.0.11:8080, http://10.0.0.12:8080",
		},
		{
			name: "redirect",
			input: SiteInput{
				PrimaryDomain: "old.example.com", Type: "redirect",
				RedirectTarget: "https://new.example.com", RedirectCode: 308,
			},
			wantKind: contract.SiteRedirect, wantTarget: "308 https://new.example.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, nginx, root := newTestManager(t)
			created, err := manager.Create(context.Background(), test.input)
			if err != nil {
				t.Fatal(err)
			}
			if created.Kind != test.wantKind || created.Target != test.wantTarget ||
				created.Consistency != contract.ConsistencyInSync ||
				!containsString(created.AllowedActions, "update") {
				t.Fatalf("created site mismatch: %#v", created)
			}
			if test.indexName != "" {
				path := filepath.Join(root, "html", test.input.PrimaryDomain, test.indexName)
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("default document is missing: %v", err)
				}
			}
			config, err := os.ReadFile(filepath.Join(root, "conf.d", test.input.PrimaryDomain+".conf"))
			if err != nil || !bytes.HasPrefix(config, []byte(managedMarker+"\n")) {
				t.Fatalf("canonical v2 configuration missing: err=%v config=%s", err, config)
			}
			if nginx.tests != 2 || nginx.reloads != 1 {
				t.Fatalf("nginx calls tests=%d reloads=%d, want 2/1", nginx.tests, nginx.reloads)
			}
		})
	}
}

func TestDeleteManagedProxyUsesKWebDel(t *testing.T) {
	manager, _, root := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "app.example.com", Type: "proxy", Upstream: "http://127.0.0.1:8064",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeSiteScriptDeleter{root: root}
	manager.scriptDeleter = runner
	result, err := manager.DeleteWithOptions(context.Background(), created.ID, DeleteInput{
		PrimaryDomain: created.PrimaryDomain,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "deleted" || result.PrimaryDomain != created.PrimaryDomain {
		t.Fatalf("unexpected delete result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "conf.d", "app.example.com.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("proxy config still exists: %v", err)
	}
	if runner.domain != created.PrimaryDomain || result.Mode != "full" {
		t.Fatalf("proxy delete bypassed k web del: result=%#v runner=%#v", result, runner)
	}
}

func TestDeleteManagedStaticRemovesCompleteKWebSite(t *testing.T) {
	manager, _, root := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "static-delete.example.com", Type: "static",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.scriptDeleter = &fakeSiteScriptDeleter{root: root}
	result, err := manager.DeleteWithOptions(context.Background(), created.ID, DeleteInput{PrimaryDomain: created.PrimaryDomain})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "full" || len(result.Removed) != 2 {
		t.Fatalf("unexpected delete result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "conf.d", "static-delete.example.com.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("static config still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "html", "static-delete.example.com")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("k web del left static content: %v", err)
	}
}

func TestFullDeleteWithoutResourceVersionUsesKWebDel(t *testing.T) {
	manager, _, root := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "script-delete.example.com", Type: "static",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeSiteScriptDeleter{
		root: root,
		outcome: scriptDeleteOutcome{
			databaseDropped: true,
			warnings:        []string{"script warning"},
		},
	}
	manager.scriptDeleter = runner

	result, err := manager.DeleteWithOptions(context.Background(), created.ID, DeleteInput{
		PrimaryDomain: created.PrimaryDomain,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner.domain != created.PrimaryDomain ||
		result.Mode != "full" ||
		!result.DatabaseDropped ||
		len(result.Warnings) != 1 ||
		result.Warnings[0] != "script warning" {
		t.Fatalf("script delete mismatch: result=%#v runner=%#v", result, runner)
	}
	if _, statErr := os.Stat(filepath.Join(root, "conf.d", created.PrimaryDomain+".conf")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("script delete left Nginx config: %v", statErr)
	}
}

func TestFullDeleteWithoutResourceVersionRejectsDomainMismatch(t *testing.T) {
	manager, _, _ := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "expected.example.com", Type: "static",
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeSiteScriptDeleter{}
	manager.scriptDeleter = runner

	_, err = manager.DeleteWithOptions(context.Background(), created.ID, DeleteInput{
		PrimaryDomain: "other.example.com",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("domain mismatch error = %v, want ErrConflict", err)
	}
	if runner.domain != "" {
		t.Fatalf("domain mismatch reached k web del: %#v", runner)
	}
}

func TestDeleteManagedPHPUsesScriptDatabaseOutcome(t *testing.T) {
	manager, _, root := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "php-delete.example.com", Type: "php", PHPVersion: "latest",
	})
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(root, "certs", created.PrimaryDomain+"_cert.pem")
	keyPath := filepath.Join(root, "certs", created.PrimaryDomain+"_key.pem")
	if err := os.WriteFile(certPath, []byte("test certificate"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("test key"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &fakeSiteScriptDeleter{root: root, outcome: scriptDeleteOutcome{databaseDropped: true}}
	manager.scriptDeleter = runner
	result, err := manager.DeleteWithOptions(context.Background(), created.ID, DeleteInput{PrimaryDomain: created.PrimaryDomain})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "full" || !result.DatabaseDropped || runner.domain != created.PrimaryDomain || len(result.Removed) != 4 {
		t.Fatalf("unexpected script delete result: result=%#v runner=%#v", result, runner)
	}
	for _, path := range []string{
		filepath.Join(root, "conf.d", created.PrimaryDomain+".conf"),
		filepath.Join(root, "html", created.PrimaryDomain),
		certPath,
		keyPath,
	} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("full delete left artifact %s: %v", path, statErr)
		}
	}
}

func TestFullDeleteDoesNotRequireDatabaseAdapter(t *testing.T) {
	manager, _, root := newTestManager(t)
	manager.siteDataRuntime = nil
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "static-no-db.example.com", Type: "static",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.scriptDeleter = &fakeSiteScriptDeleter{root: root}
	result, err := manager.DeleteWithOptions(context.Background(), created.ID, DeleteInput{PrimaryDomain: created.PrimaryDomain})
	if err != nil {
		t.Fatal(err)
	}
	if result.DatabaseDropped || len(result.Warnings) != 0 {
		t.Fatalf("unexpected database result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "html", created.PrimaryDomain)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("full delete was blocked without a database adapter: %v", err)
	}
}

func TestScriptDeleteReportsDatabaseFailureWithoutRestoringSite(t *testing.T) {
	manager, _, root := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "db-warning.example.com", Type: "php", PHPVersion: "latest",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.scriptDeleter = &fakeSiteScriptDeleter{
		root:    root,
		outcome: scriptDeleteOutcome{warnings: []string{"站点已删除，但同名数据库删除失败"}},
	}
	result, err := manager.DeleteWithOptions(context.Background(), created.ID, DeleteInput{PrimaryDomain: created.PrimaryDomain})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "数据库删除失败") {
		t.Fatalf("database cleanup warning = %#v", result.Warnings)
	}
	for _, path := range []string{
		filepath.Join(root, "conf.d", created.PrimaryDomain+".conf"),
		filepath.Join(root, "html", created.PrimaryDomain),
	} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("database failure restored deleted site artifact %s: %v", path, statErr)
		}
	}
}

func TestParseScriptDeleteOutcomeTreatsDeletedSiteWithDatabaseFailureAsPartialSuccess(t *testing.T) {
	outcome := parseScriptDeleteOutcome(strings.Join([]string{
		"正在删除域名: codex.example.com",
		"正在删除数据库: codex_example_com",
		"KPANEL_DELETE_DATABASE failed codex.example.com",
		"KPANEL_DELETE_SITE deleted codex.example.com",
	}, "\n"))

	if !outcome.siteDeleted || outcome.databaseDropped || len(outcome.warnings) != 1 {
		t.Fatalf("partial delete outcome = %#v", outcome)
	}
	if !strings.Contains(outcome.warnings[0], "数据库删除失败") {
		t.Fatalf("partial delete warning = %#v", outcome.warnings)
	}
}

func TestParseScriptDeleteOutcomeReportsNginxReloadFailure(t *testing.T) {
	outcome := parseScriptDeleteOutcome(strings.Join([]string{
		"KPANEL_DELETE_SITE deleted reload.example.com",
		"Nginx 配置验证或重载失败",
	}, "\n"))

	if !outcome.siteDeleted || len(outcome.warnings) != 1 ||
		!strings.Contains(outcome.warnings[0], "Nginx 配置验证或重载失败") {
		t.Fatalf("Nginx partial delete outcome = %#v", outcome)
	}
}

func TestOrphanWebsiteArtifactCanBeDeleted(t *testing.T) {
	manager, nginx, root := newTestManager(t)
	orphanRoot := filepath.Join(root, "html", "orphan.example.com")
	if err := os.Mkdir(orphanRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphanRoot, "index.html"), []byte("orphan"), 0o640); err != nil {
		t.Fatal(err)
	}
	items, err := manager.discoverer.Discover()
	if err != nil {
		t.Fatal(err)
	}
	var orphan contract.SiteSummary
	for _, site := range items {
		if site.PrimaryDomain == "orphan.example.com" {
			orphan = site
			break
		}
	}
	if orphan.ID == "" || !containsString(orphan.AllowedActions, "delete") {
		t.Fatalf("orphan artifact was not actionable: %#v", orphan)
	}
	manager.scriptDeleter = &fakeSiteScriptDeleter{root: root}
	if _, err := manager.DeleteWithOptions(context.Background(), orphan.ID, DeleteInput{PrimaryDomain: orphan.PrimaryDomain}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphanRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan website artifact still exists: %v", err)
	}
	if nginx.tests != 0 || nginx.reloads != 0 {
		t.Fatalf("detached artifact deletion unnecessarily reloaded Nginx: %#v", nginx)
	}
}

func TestDeleteScriptFailurePreservesExistingSite(t *testing.T) {
	manager, _, root := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "rollback-delete.example.com", Type: "static",
	})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "conf.d", created.PrimaryDomain+".conf")
	indexPath := filepath.Join(root, "html", created.PrimaryDomain, "index.html")

	manager.scriptDeleter = &fakeSiteScriptDeleter{err: errors.New("script rejected deletion")}
	_, err = manager.DeleteWithOptions(context.Background(), created.ID, DeleteInput{PrimaryDomain: created.PrimaryDomain})
	if err == nil || !strings.Contains(err.Error(), "script rejected deletion") {
		t.Fatalf("script failure error = %v", err)
	}
	for _, path := range []string{configPath, indexPath} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("failed delete did not restore %s: %v", path, statErr)
		}
	}
}

func TestUpdateMigratesCanonicalV1SiteWithoutChangingDocumentRoot(t *testing.T) {
	manager, _, root := newTestManager(t)
	spec := managedSpec{Primary: "legacy.example.com", Kind: contract.SiteStatic}
	configPath := filepath.Join(root, "conf.d", spec.Primary+".conf")
	if err := os.WriteFile(configPath, renderManagedConfigV1(spec), 0o640); err != nil {
		t.Fatal(err)
	}
	documentRoot := filepath.Join(root, "html", spec.Primary)
	if err := os.Mkdir(documentRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(documentRoot, "index.html")
	if err := os.WriteFile(indexPath, renderDefaultIndex(spec.Primary), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	items, err := manager.discoverer.Discover()
	if err != nil || len(items) != 1 || !containsString(items[0].AllowedActions, "update") {
		t.Fatalf("canonical v1 site was not manageable: err=%v sites=%#v", err, items)
	}
	updated, err := manager.Update(context.Background(), items[0].ID, SiteInput{
		PrimaryDomain:           spec.Primary,
		Aliases:                 []string{"www." + spec.Primary},
		Type:                    "static",
		ExpectedResourceVersion: items[0].ResourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	config, _ := os.ReadFile(configPath)
	after, _ := os.ReadFile(indexPath)
	if !bytes.HasPrefix(config, []byte(managedMarker+"\n")) ||
		!bytes.Equal(before, after) || updated.Kind != contract.SiteStatic {
		t.Fatalf("v1 migration mismatch: updated=%#v config=%s", updated, config)
	}
}

func TestCreateCollisionLeavesExistingArtifactsUnchanged(t *testing.T) {
	manager, nginx, root := newTestManager(t)
	path := filepath.Join(root, "conf.d", "collision.example.com.conf")
	original := []byte("server {\n    listen 80;\n    server_name collision.example.com;\n}\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	_, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "collision.example.com", Type: "proxy", Upstream: "http://127.0.0.1:8080",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Create() error = %v, want conflict", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(after, original) {
		t.Fatalf("collision changed existing config: err=%v body=%q", readErr, after)
	}
	if nginx.reloads != 0 {
		t.Fatalf("collision triggered %d reloads", nginx.reloads)
	}
}

func TestConcurrentCreateDoesNotOverwriteExternalConfiguration(t *testing.T) {
	manager, nginx, root := newTestManager(t)
	configPath := filepath.Join(root, "conf.d", "concurrent.example.com.conf")
	external := []byte("# external concurrent create\nserver { listen 80; server_name concurrent.example.com; }\n")
	manager.testHook = func(stage, path string) {
		if stage == "before_config_publish" {
			if writeErr := os.WriteFile(path, external, 0o640); writeErr != nil {
				t.Errorf("inject external create: %v", writeErr)
			}
		}
	}
	_, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "concurrent.example.com", Type: "static",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Create() error = %v, want conflict", err)
	}
	after, readErr := os.ReadFile(configPath)
	if readErr != nil || !bytes.Equal(after, external) {
		t.Fatalf("external config was overwritten: err=%v body=%q", readErr, after)
	}
	if pathExists(filepath.Join(root, "html", "concurrent.example.com")) {
		t.Fatal("the transaction's static directory was not rolled back")
	}
	if nginx.reloads != 0 {
		t.Fatalf("concurrent conflict triggered %d reloads", nginx.reloads)
	}
}

func TestInvalidCandidateDoesNotReloadAndRollsBack(t *testing.T) {
	manager, nginx, root := newTestManager(t)
	nginx.testErrs = []error{nil, errors.New("candidate syntax error")}
	_, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "invalid.example.com", Type: "static",
	})
	if !errors.Is(err, ErrUnprocessable) {
		t.Fatalf("Create() error = %v, want unprocessable", err)
	}
	if pathExists(filepath.Join(root, "conf.d", "invalid.example.com.conf")) ||
		pathExists(filepath.Join(root, "html", "invalid.example.com")) {
		t.Fatal("failed candidate artifacts were not rolled back")
	}
	if nginx.reloads != 0 {
		t.Fatalf("invalid candidate triggered %d reloads", nginx.reloads)
	}
}

func TestExternalConcurrentUpdateIsRestoredWithoutOverwrite(t *testing.T) {
	manager, nginx, root := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "race.example.com", Type: "proxy", Upstream: "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatal(err)
	}
	nginx.tests, nginx.reloads = 0, 0
	configPath := filepath.Join(root, "conf.d", "race.example.com.conf")
	external := []byte("# external concurrent edit\nserver { listen 80; server_name race.example.com; }\n")
	manager.testHook = func(stage, path string) {
		if stage == "before_exchange" {
			if writeErr := os.WriteFile(path, external, 0o640); writeErr != nil {
				t.Errorf("inject external edit: %v", writeErr)
			}
		}
	}
	_, err = manager.Update(context.Background(), created.ID, SiteInput{
		PrimaryDomain: "race.example.com", Aliases: []string{"www.race.example.com"},
		Type: "proxy", Upstream: "http://127.0.0.1:8080",
		ExpectedResourceVersion: created.ResourceVersion,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Update() error = %v, want conflict", err)
	}
	after, readErr := os.ReadFile(configPath)
	if readErr != nil || !bytes.Equal(after, external) {
		t.Fatalf("external edit was overwritten: err=%v body=%q", readErr, after)
	}
	if nginx.reloads != 0 {
		t.Fatalf("concurrent conflict triggered %d reloads", nginx.reloads)
	}
}

func TestReloadFailureRestoresPreviousConfiguration(t *testing.T) {
	manager, nginx, root := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "rollback.example.com", Type: "proxy", Upstream: "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "conf.d", "rollback.example.com.conf")
	before, _ := os.ReadFile(path)
	nginx.tests, nginx.reloads = 0, 0
	nginx.testErrs = nil
	nginx.reloadErrs = []error{errors.New("reload failed"), nil}
	_, err = manager.Update(context.Background(), created.ID, SiteInput{
		PrimaryDomain:           "rollback.example.com",
		Aliases:                 []string{"www.rollback.example.com"},
		Type:                    "proxy",
		Upstream:                "http://127.0.0.1:8080",
		ExpectedResourceVersion: created.ResourceVersion,
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Update() error = %v, want unavailable after safe rollback", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(after, before) {
		t.Fatalf("previous config was not restored: err=%v", readErr)
	}
	if nginx.tests != 3 || nginx.reloads != 2 {
		t.Fatalf("rollback calls tests=%d reloads=%d, want 3/2", nginx.tests, nginx.reloads)
	}
}

func TestNonCanonicalManagedConfigCanBeUpdatedWithoutDiscardingManualDirectives(t *testing.T) {
	manager, _, root := newTestManager(t)
	created, err := manager.Create(context.Background(), SiteInput{
		PrimaryDomain: "drift.example.com", Type: "proxy", Upstream: "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "conf.d", "drift.example.com.conf")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("# manual edit\n"); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	drifted, _ := os.ReadFile(path)
	discovered, discoverErr := manager.discoverer.Discover()
	if discoverErr != nil {
		t.Fatal(discoverErr)
	}
	if len(discovered) != 1 || discovered[0].Origin != contract.OriginWeb ||
		discovered[0].Consistency != contract.ConsistencyDrifted ||
		!containsString(discovered[0].AllowedActions, "update") ||
		!containsString(discovered[0].AllowedActions, "delete") {
		t.Fatalf("drifted Panel site ownership/action mismatch: %#v", discovered)
	}
	updated, err := manager.Update(context.Background(), created.ID, SiteInput{
		PrimaryDomain: "drift.example.com", Type: "proxy", Upstream: "http://127.0.0.1:9090",
		ExpectedResourceVersion: discovered[0].ResourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if bytes.Equal(after, drifted) || !bytes.Contains(after, []byte("# manual edit")) ||
		!bytes.Contains(after, []byte("http://127.0.0.1:9090")) ||
		updated.Target != "http://127.0.0.1:9090" {
		t.Fatalf("drifted config update mismatch: updated=%#v config=%s", updated, after)
	}
}

func TestUpdateIPv4HTTPProxyPreservesListener(t *testing.T) {
	manager, _, root := newTestManager(t)
	path := filepath.Join(root, "conf.d", "192.168.1.10.conf")
	config := "server {\n    listen 8443;\n    server_name 192.168.1.10;\n    location / {\n        proxy_pass http://127.0.0.1:3000;\n    }\n}\n"
	if err := os.WriteFile(path, []byte(config), 0o640); err != nil {
		t.Fatal(err)
	}
	items, err := manager.discoverer.Discover()
	if err != nil || len(items) != 1 {
		t.Fatalf("discover: %v %#v", err, items)
	}
	input := SiteInput{PrimaryDomain: "192.168.1.10", Type: "proxy", Upstream: "http://127.0.0.1:3001", ExpectedResourceVersion: items[0].ResourceVersion}
	updated, err := manager.Update(context.Background(), items[0].ID, input)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(path)
	if err != nil || string(actual) != strings.Replace(config, ":3000", ":3001", 1) ||
		!containsString(updated.AccessURLs, "http://192.168.1.10:8443") {
		t.Fatalf("listener or manual config changed: %v %s %#v", err, actual, updated)
	}
	if _, err := manager.Update(context.Background(), items[0].ID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale edit: %v", err)
	}
	input.PrimaryDomain, input.ExpectedResourceVersion = "192.168.1.11", updated.ResourceVersion
	if _, err := manager.Update(context.Background(), items[0].ID, input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("rename accepted: %v", err)
	}
}

func TestStrictSiteInputValidation(t *testing.T) {
	invalid := []SiteInput{
		{PrimaryDomain: "*.example.com", Type: "static"},
		{PrimaryDomain: "127.0.0.1", Type: "static"},
		{PrimaryDomain: "example.com/path", Type: "static"},
		{PrimaryDomain: "example.com ", Type: "static"},
		{PrimaryDomain: "example.com", Type: "proxy", Upstream: "http://user:pass@app:8080"},
		{PrimaryDomain: "example.com", Type: "proxy", Upstream: "http://app:8080/path"},
		{PrimaryDomain: "example.com", Type: "proxy", Upstream: "http://127.0.0.1:0"},
		{PrimaryDomain: "example.com", Type: "proxy_domain", Upstream: "https://127.0.0.1"},
		{PrimaryDomain: "example.com", Type: "load_balance", Upstreams: []string{"http://10.0.0.1:80"}},
		{PrimaryDomain: "example.com", Type: "load_balance", Upstreams: []string{"http://10.0.0.1:80", "https://10.0.0.2:443"}},
		{PrimaryDomain: "example.com", Type: "redirect", RedirectTarget: "https://new.example.com/path"},
		{PrimaryDomain: "example.com", Type: "redirect", RedirectTarget: "https://new.example.com", RedirectCode: 200},
		{PrimaryDomain: "example.com", Type: "php", PHPVersion: "8.0"},
		{PrimaryDomain: "example.com", Type: "static", Upstream: "http://127.0.0.1:80"},
	}
	for _, input := range invalid {
		if _, err := normalizeSiteInput(input); !errors.Is(err, ErrUnprocessable) {
			t.Errorf("normalizeSiteInput(%#v) error = %v, want unprocessable", input, err)
		}
	}
	valid := []string{
		"http://app:8080",
		"http://app.example:8080",
		"http://8.8.8.8:53",
		"https://127.0.0.1",
		"http://10.0.0.2:65535",
		"http://172.16.0.2:80",
		"http://192.168.1.2:443",
		"http://[::1]:8080",
	}
	for _, upstream := range valid {
		if _, err := normalizeSiteInput(SiteInput{
			PrimaryDomain: "example.com", Type: "proxy", Upstream: upstream,
		}); err != nil {
			t.Errorf("valid upstream %q rejected: %v", upstream, err)
		}
	}
}

func newTestManager(t *testing.T) (*Manager, *fakeNginxController, string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"conf.d", "html", "certs"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	nginx := &fakeNginxController{}
	discoverer := NewDiscoverer(root)
	manager := NewManager(root, discoverer, nginx)
	return manager, nginx, root
}
