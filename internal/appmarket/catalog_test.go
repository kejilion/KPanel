package appmarket

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
)

type fakeDocker struct {
	containers []contract.ContainerSummary
}

func (f *fakeDocker) Containers(context.Context) ([]contract.ContainerSummary, error) {
	return append([]contract.ContainerSummary(nil), f.containers...), nil
}

func (f *fakeDocker) Lifecycle(context.Context, string, string, string) (dockerx.ActionResult, error) {
	return dockerx.ActionResult{}, nil
}

func (f *fakeDocker) LifecycleDeclarativeApp(
	context.Context,
	dockerx.DeclarativeAppSpec,
	string,
	string,
) (dockerx.ActionResult, error) {
	return dockerx.ActionResult{}, nil
}

func (f *fakeDocker) InstallDeclarativeApp(
	context.Context,
	dockerx.DeclarativeAppSpec,
	uint16,
	string,
) (dockerx.AppMutationResult, error) {
	return dockerx.AppMutationResult{}, nil
}

func (f *fakeDocker) UpdateDeclarativeApp(
	context.Context,
	dockerx.DeclarativeAppSpec,
	string,
) (dockerx.AppMutationResult, error) {
	return dockerx.AppMutationResult{}, nil
}

func (f *fakeDocker) SetDeclarativeAppAccess(
	context.Context,
	dockerx.DeclarativeAppSpec,
	string,
	string,
) (dockerx.AppMutationResult, error) {
	return dockerx.AppMutationResult{}, nil
}

func (f *fakeDocker) UninstallDeclarativeApp(
	context.Context,
	dockerx.DeclarativeAppSpec,
	string,
) (dockerx.AppMutationResult, error) {
	return dockerx.AppMutationResult{}, nil
}

func (f *fakeDocker) CheckContainerImageUpdate(
	context.Context,
	string,
	string,
) (dockerx.ImageUpdateResult, error) {
	return dockerx.ImageUpdateResult{}, nil
}

func TestEmbeddedCatalogMatchesAuditedApplicationMarket(t *testing.T) {
	catalog, legacy, scriptSHA256, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Apps) != 153 || len(legacy) != 115 {
		t.Fatalf("catalog counts = %d/%d, want 153/115", len(catalog.Apps), len(legacy))
	}
	if !strings.HasPrefix(catalog.Source, "https://app.kejilion.sh") ||
		len(scriptSHA256) != 64 {
		t.Fatalf("catalog provenance is incomplete: source=%q hash=%q", catalog.Source, scriptSHA256)
	}
	icons := make(map[string]bool, len(catalog.Apps))
	foundKPanel := false
	for _, app := range catalog.Apps {
		if !strings.HasPrefix(app.Icon, "/app-icons/") || len(app.IconSHA256) != 64 {
			t.Fatalf("invalid local icon metadata for %s", app.ID)
		}
		if icons[app.Icon] {
			t.Fatalf("duplicate icon path %s", app.Icon)
		}
		icons[app.Icon] = true
		if app.Token == "kpanel" {
			foundKPanel = app.Icon == "/app-icons/kpanel.webp" &&
				app.IconSHA256 == "19ca9151548dcb4b82bbc48d4dc4bec62e8ef9d4bdaa90c27040281803253088"
		}
	}
	if !foundKPanel {
		t.Fatal("KPanel local application icon is missing or has an unexpected digest")
	}
	for _, category := range catalog.Categories {
		if category.ZHTW == "" {
			t.Fatalf("Traditional Chinese category label is missing for %s", category.Key)
		}
	}
	for _, app := range catalog.Apps {
		if app.NameZHTW == "" || app.DescriptionZHTW == "" {
			t.Fatalf("Traditional Chinese app metadata is missing for %s", app.ID)
		}
	}
}

func TestCuratedGameIconsMatchCatalogDigests(t *testing.T) {
	catalog, _, _, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	gameTokens := []string{"arena-brawl", "bomb-party", "ice-climber-arena", "neon-arena-fps"}
	digests := make(map[string]string, len(gameTokens))
	for _, token := range gameTokens {
		app := embeddedAppByToken(t, catalog, token)
		iconPath := filepath.Join("..", "..", "web", "public", "app-icons", path.Base(app.Icon))
		content, err := os.ReadFile(iconPath)
		if err != nil {
			t.Fatalf("read curated icon for %s: %v", token, err)
		}
		hash := sha256.Sum256(content)
		actual := hex.EncodeToString(hash[:])
		if actual != app.IconSHA256 {
			t.Fatalf("curated icon digest for %s = %s, want %s", token, actual, app.IconSHA256)
		}
		if previous, duplicated := digests[actual]; duplicated {
			t.Fatalf("curated game icons %s and %s share digest %s", previous, token, actual)
		}
		digests[actual] = token
	}
}

func TestCatalogDateValidation(t *testing.T) {
	tests := map[string]bool{
		"":           true,
		"2026-08-16": true,
		"2026-02-29": false,
		"2024-02-29": true,
		"2026-8-16":  false,
		"2026-08-32": false,
		"not-a-date": false,
	}
	for value, expected := range tests {
		if actual := validCatalogDate(value); actual != expected {
			t.Errorf("validCatalogDate(%q) = %v, want %v", value, actual, expected)
		}
	}
}

func TestInventoryCombinesDockerTruthAndScriptMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "appno.txt"), []byte("28\n64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docker := &fakeDocker{containers: []contract.ContainerSummary{{
		ID: strings.Repeat("a", 64), Name: "speedtest",
		Image: "ghcr.io/librespeed/speedtest", State: "running", Status: "Up",
		Ports:  []contract.PortBinding{{PrivatePort: 8080, PublicPort: 8028, IP: "0.0.0.0", Type: "tcp"}},
		Mounts: []contract.Mount{}, ResourceVersion: "sha256:" + strings.Repeat("b", 64),
		AllowedActions: []string{"restart", "stop"},
	}}}
	service, err := New(docker, root)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := service.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Items) != 153 || inventory.Installed != 2 || inventory.Running != 1 {
		t.Fatalf("inventory counts are wrong: %#v", inventory)
	}
	var speedtest, itTools Summary
	for _, item := range inventory.Items {
		switch item.Token {
		case "speedtest":
			speedtest = item
		case "it-tools":
			itTools = item
		}
	}
	if speedtest.Runtime.State != "running" ||
		!speedtest.Capabilities["update"].Enabled ||
		!speedtest.Capabilities["direct_access"].Enabled {
		t.Fatalf("speedtest was not safely manageable: %#v", speedtest)
	}
	if itTools.Runtime.State != "unknown" ||
		itTools.Capabilities["start"].Enabled ||
		itTools.Runtime.Warning == "" {
		t.Fatalf("marker-only application was not degraded safely: %#v", itTools)
	}
}

func TestInventoryUsesDockerAppServiceAsMainContainer(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "appno.txt"), []byte("81\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docker := &fakeDocker{containers: []contract.ContainerSummary{{
		ID: strings.Repeat("c", 64), Name: "jitsi-web-1",
		Image: "jitsi/web:latest", State: "running", Status: "Up",
		Ports: []contract.PortBinding{{
			PrivatePort: 80, PublicPort: 8081, IP: "0.0.0.0", Type: "tcp",
		}},
		ResourceVersion: "sha256:" + strings.Repeat("d", 64),
		AllowedActions:  []string{"stop", "restart"},
	}}}
	service, err := New(docker, root)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeJobRunner{}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }
	service.scriptManageFinder = func() (string, error) { return "/usr/local/bin/k", nil }
	item, err := service.Find(context.Background(), "builtin-81")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Runtime.Installed || item.Runtime.ContainerName != "jitsi-web-1" ||
		!item.Capabilities["update"].Enabled {
		t.Fatalf("docker_app_service alias was not manageable: %#v", item)
	}
}

func TestRuntimeFromStoppedContainerSerializesPortsAsArray(t *testing.T) {
	runtime := runtimeFromContainer(contract.ContainerSummary{
		ID: strings.Repeat("e", 64), Name: "stopped-app",
		Image: "example/stopped-app:latest", State: "exited", Status: "Exited (0)",
	})
	if runtime.Ports == nil {
		t.Fatal("stopped container ports must be an initialized empty slice")
	}
	encoded, err := json.Marshal(runtime)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Ports json.RawMessage `json:"ports"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if string(payload.Ports) != "[]" {
		t.Fatalf("runtime JSON ports = %s, want []", payload.Ports)
	}
}

func TestRemoteCatalogDynamicallyReplacesBuiltinAndThirdPartyEntries(t *testing.T) {
	embedded, _, _, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for index := range embedded.Apps {
		if embedded.Apps[index].Source == "builtin" {
			embedded.Apps[index].AddedAt = "2026-06-01"
			break
		}
	}
	payload := remotePayloadFromCatalog(embedded)
	removedThirdParty := ""
	updatedBuiltin := ""
	historicalBuiltin := ""
	apps := make([]App, 0, len(payload.Apps))
	for _, app := range payload.Apps {
		if app.Source == "thirdparty" && removedThirdParty == "" {
			removedThirdParty = app.Token
			continue
		}
		if app.Source == "builtin" && updatedBuiltin == "" {
			updatedBuiltin = app.Token
			app.NameZH = "动态更新的内置应用"
			app.AddedAt = "2026-08-16"
		} else if app.Source == "builtin" && historicalBuiltin == "" {
			historicalBuiltin = app.Token
			app.AddedAt = "2026-08-16"
		}
		apps = append(apps, app)
	}
	apps = append(apps, App{
		ID: "builtin-117", Num: 117, Source: "builtin", Token: "new-builtin-app",
		NameZH: "新内置应用", NameEN: "New Builtin App", Description: "动态内置目录测试",
		DescriptionEN: "Dynamic builtin catalog test", Category: "ai",
		Icon: "icons/new-builtin-app.webp", Slug: "new-builtin-app", AddedAt: "2026-08-15",
	})
	apps = append(apps, App{
		ID: "thirdparty-new-safe-app", Source: "thirdparty", Token: "new-safe-app",
		NameZH: "新入驻应用", NameEN: "New Safe App", Description: "动态目录测试",
		DescriptionEN: "Dynamic catalog test", Category: "commtools",
		Website: "https://example.com", Icon: "icons/new-safe-app.webp", Slug: "new-safe-app",
		AddedAt: "2026-08-16",
	})
	payload.Apps = apps
	payload.Meta.Builtin++
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := decodeRemoteCatalog(
		[]byte("<script>window.__APPS__ = " + string(encoded) + ";\n  </script>"),
	)
	if err != nil {
		t.Fatal(err)
	}
	merged := mergeRemoteCatalogWithDynamicIcons(embedded, remote, true)
	dynamicSources := dynamicRemoteIconSources(embedded, remote)
	fallback := mergeRemoteCatalog(embedded, remote)
	for _, slug := range []string{"new-builtin-app", "new-safe-app"} {
		app := appBySlug(t, fallback, slug)
		if app.Icon != genericThirdPartyIcon || app.IconSHA256 != "" {
			t.Fatalf("dynamic app %q did not retain the local fallback: %#v", slug, app)
		}
	}
	foundNewThirdParty := false
	foundNewBuiltin := false
	foundUpdatedBuiltin := false
	foundHistoricalBuiltin := false
	foundRemovedThirdParty := false
	for _, app := range merged.Apps {
		if app.Token == "new-safe-app" {
			foundNewThirdParty = app.Icon == dynamicAppIconPrefix+"new-safe-app.webp" &&
				app.IconSHA256 == "" && app.AddedAt == "2026-08-16" &&
				dynamicSources[app.Slug] == "icons/new-safe-app.webp"
		}
		if app.Token == "new-builtin-app" {
			foundNewBuiltin = app.Num == 117 &&
				app.Icon == dynamicAppIconPrefix+"new-builtin-app.webp" &&
				app.IconSHA256 == "" && app.AddedAt == "2026-08-15" &&
				dynamicSources[app.Slug] == "icons/new-builtin-app.webp"
		}
		if app.Token == updatedBuiltin {
			local := embeddedAppByToken(t, embedded, updatedBuiltin)
			foundUpdatedBuiltin = app.NameZH == "动态更新的内置应用" &&
				app.Icon == local.Icon && app.IconSHA256 == local.IconSHA256 &&
				app.AddedAt == "2026-06-01"
		}
		if app.Token == historicalBuiltin {
			foundHistoricalBuiltin = app.AddedAt == ""
		}
		if app.Token == removedThirdParty {
			foundRemovedThirdParty = true
		}
	}
	if !foundNewThirdParty || !foundNewBuiltin || !foundUpdatedBuiltin || !foundHistoricalBuiltin ||
		foundRemovedThirdParty || len(merged.Apps) != len(embedded.Apps)+1 {
		t.Fatalf(
			"dynamic merge failed: thirdParty=%v builtin=%v updated=%v historical=%v removedStillPresent=%v count=%d",
			foundNewThirdParty, foundNewBuiltin, foundUpdatedBuiltin, foundHistoricalBuiltin,
			foundRemovedThirdParty, len(merged.Apps),
		)
	}

	refreshed := merged
	refreshed.Apps = append([]App(nil), merged.Apps...)
	for index := range refreshed.Apps {
		if refreshed.Apps[index].Token == "new-safe-app" {
			refreshed.Apps[index].AddedAt = "2026-08-17"
		}
	}
	preserveExistingAddedDates(merged, &refreshed)
	if app := embeddedAppByToken(t, refreshed, "new-safe-app"); app.AddedAt != "2026-08-16" {
		t.Fatalf("existing application addedAt was refreshed: %q", app.AddedAt)
	}
}

func TestRemoteCatalogRejectsUntrustedDynamicBuiltinMetadata(t *testing.T) {
	embedded, _, _, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*remoteCatalogPayload)
	}{
		{
			name: "source",
			mutate: func(payload *remoteCatalogPayload) {
				payload.Meta.Source = "https://example.com/untrusted"
			},
		},
		{
			name: "number",
			mutate: func(payload *remoteCatalogPayload) {
				for index := range payload.Apps {
					if payload.Apps[index].Source == "builtin" {
						payload.Apps[index].Num = 499
						return
					}
				}
			},
		},
		{
			name: "count",
			mutate: func(payload *remoteCatalogPayload) {
				payload.Meta.Builtin++
			},
		},
		{
			name: "icon identity",
			mutate: func(payload *remoteCatalogPayload) {
				payload.Apps[0].Icon = "icons/another-app.webp"
			},
		},
		{
			name: "invalid added date",
			mutate: func(payload *remoteCatalogPayload) {
				payload.Apps[0].AddedAt = "2026-02-29"
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := remotePayloadFromCatalog(embedded)
			test.mutate(&payload)
			encoded, marshalErr := json.Marshal(payload)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if _, decodeErr := decodeRemoteCatalog(
				[]byte("<script>window.__APPS__ = " + string(encoded) + ";</script>"),
			); decodeErr == nil {
				t.Fatal("unsafe remote catalog was accepted")
			}
		})
	}
}

func TestDynamicBuiltinUsesTheExistingKejilionSelectorPipeline(t *testing.T) {
	app := App{ID: "builtin-116", Num: 116, Source: "builtin", Token: "new-builtin-app"}
	service := &Service{}
	selector, scriptBacked := service.scriptSelectorFor(Summary{App: app})
	capabilities := defaultCapabilities(app, LegacyApp{}, true)
	if !scriptBacked || selector != "116" || !capabilities["install"].Enabled ||
		installerKind(app, LegacyApp{}, true) != "kejilion" {
		t.Fatalf(
			"dynamic builtin did not reuse the kejilion selector pipeline: selector=%q backed=%v capabilities=%#v",
			selector, scriptBacked, capabilities,
		)
	}
}

func TestManagedScriptRemainsDefaultAndHostScriptIsFallback(t *testing.T) {
	candidates := preferredKejilionScriptCandidates()
	if len(candidates) < 4 {
		t.Fatalf("script candidate list is incomplete: %#v", candidates)
	}
	if candidates[0] != "/home/docker/kpanel/bin/kejilion.sh" ||
		candidates[1] != "/usr/local/bin/k" {
		t.Fatalf("managed script must remain the default with the host script as fallback: %#v", candidates)
	}
}

func TestBuiltinSelectorRequiresExplicitScriptSupport(t *testing.T) {
	base := []byte("permission_granted=\"true\"\nKJ_APP_NONINTERACTIVE\nkpanel_run_docker_app_install\n")
	compatible := appScriptCompatible(isKPanelCompatibleScript, "builtin-116", "116")
	if compatible(append(base, []byte("\t115|old-app)\n")...)) {
		t.Fatal("a compatible but stale managed script accepted an unknown builtin selector")
	}
	if compatible(append(base, []byte("\t116|documented-but-not-a-case-branch\n")...)) {
		t.Fatal("a non-case script line was accepted as builtin selector support")
	}
	if !compatible(append(base, []byte("\t116|new-app|new-app-alias)\n")...)) {
		t.Fatal("an updated compatible host script rejected its builtin selector")
	}
	if appScriptCompatible(isKPanelCompatibleScript, "builtin-116", "115")(
		append(base, []byte("\t115|old-app)\n")...),
	) {
		t.Fatal("a builtin app identity accepted a different selector")
	}
	if !appScriptCompatible(isKPanelCompatibleScript, "thirdparty-example", "example")(base) {
		t.Fatal("third-party apps must keep the existing dynamic configuration mechanism")
	}
}

func TestRemoteCatalogFallsBackToLastKnownGood(t *testing.T) {
	embedded, _, _, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	fetcher := func(context.Context) (Catalog, error) {
		if calls.Add(1) == 1 {
			return embedded, nil
		}
		return Catalog{}, errors.New("upstream unavailable")
	}
	service, err := newService(&fakeDocker{}, t.TempDir(), fetcher)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	first := service.currentCatalog(context.Background())
	if first.Mode != "embedded" || first.Warning != "" {
		t.Fatalf("cold catalog should return embedded immediately: %#v", first)
	}
	first = waitForCatalogState(t, service, "live", "")
	if calls.Load() != 1 {
		t.Fatalf("unexpected first refresh calls=%d", calls.Load())
	}
	now = now.Add(remoteCatalogTTL + time.Second)
	second := service.currentCatalog(context.Background())
	if second.Mode != "cached" {
		t.Fatalf("stale catalog should remain immediately usable: %#v", second)
	}
	second = waitForCatalogState(t, service, "cached", "warning")
	if second.Warning == "" || calls.Load() != 2 {
		t.Fatalf("unexpected cached catalog state: %#v calls=%d", second, calls.Load())
	}
}

func TestRemoteCatalogColdReadDoesNotWaitForNetwork(t *testing.T) {
	embedded, _, _, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	fetcher := func(context.Context) (Catalog, error) {
		close(started)
		<-release
		return embedded, nil
	}
	service, err := newService(&fakeDocker{}, t.TempDir(), fetcher)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	snapshot := service.currentCatalog(context.Background())
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("cold catalog read blocked for %s", elapsed)
	}
	if snapshot.Mode != "embedded" {
		t.Fatalf("cold catalog mode = %s", snapshot.Mode)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("catalog refresh did not start")
	}
	close(release)
	_ = waitForCatalogState(t, service, "live", "")
}

func TestCompatibilityFilesReconcileScriptAndManualDrift(t *testing.T) {
	root := t.TempDir()
	service := &Service{appRoot: root}
	spec := declarativeSpecs["speedtest"]
	item := Summary{App: App{Num: 28, Token: spec.Token}}
	portPath := filepath.Join(root, spec.ContainerName+"_port.conf")
	markerPath := filepath.Join(root, "appno.txt")
	if err := os.WriteFile(portPath, []byte("changed-by-script\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, []byte("custom marker from script\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := service.addCompatibilityFiles(item, spec, 18028); err != nil {
		t.Fatal(err)
	}
	portData, err := os.ReadFile(portPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(portData) != "18028\n" {
		t.Fatalf("port compatibility data = %q", portData)
	}
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(markerData) != "custom marker from script\n28\n" {
		t.Fatalf("marker compatibility data = %q", markerData)
	}

	if err := os.WriteFile(portPath, []byte("changed-again\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := service.removeCompatibilityFiles(item, spec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(portPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("port compatibility artifact still exists: %v", err)
	}
	markerData, err = os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(markerData) != "custom marker from script\n" {
		t.Fatalf("unrelated script marker was changed: %q", markerData)
	}
}

func waitForCatalogState(t *testing.T, service *Service, mode, warning string) catalogSnapshot {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := service.currentCatalog(context.Background())
		if snapshot.Mode == mode && (warning == "" || snapshot.Warning != "") {
			return snapshot
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("catalog did not reach mode %s", mode)
	return catalogSnapshot{}
}

func remotePayloadFromCatalog(catalog Catalog) remoteCatalogPayload {
	payload := remoteCatalogPayload{
		Meta:       remoteCatalogMeta{Source: officialCatalogSource},
		Categories: append([]Category(nil), catalog.Categories...),
		Apps:       append([]App(nil), catalog.Apps...),
	}
	for index := range payload.Apps {
		app := &payload.Apps[index]
		app.Icon = "icons/" + app.Slug + ".webp"
		app.IconSHA256 = ""
		if app.Source == "builtin" {
			payload.Meta.Builtin++
		} else {
			payload.Meta.ThirdParty++
		}
	}
	return payload
}

func embeddedAppByToken(t *testing.T, catalog Catalog, token string) App {
	t.Helper()
	for _, app := range catalog.Apps {
		if app.Token == token {
			return app
		}
	}
	t.Fatalf("embedded application %q was not found", token)
	return App{}
}
