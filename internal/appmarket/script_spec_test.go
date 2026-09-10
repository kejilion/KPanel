package appmarket

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestThirdPartyScriptAppUsesVerifiedMainContainerAndLifecycleProtocol(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "apps")
	if err := os.Mkdir(configRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "appno.txt"),
		[]byte("AIClient-2-API\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(configRoot, "AIClient-2-API.conf"),
		[]byte("docker_name=\"aiclient-2-api\"\ndocker_app_service=aiclient-web\ndocker_app_plus\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "aiclient-2-api_access.conf"),
		[]byte("domain_only\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	containerID := strings.Repeat("a", 64)
	service, err := New(&fakeDocker{containers: []contract.ContainerSummary{{
		ID: containerID, Name: "aiclient-web", Image: "example/aiclient:latest",
		State: "running", Status: "Up",
		Ports: []contract.PortBinding{{
			PrivatePort: 3000, PublicPort: 18081, IP: "0.0.0.0", Type: "tcp",
		}},
		ResourceVersion: "sha256:" + strings.Repeat("b", 64),
		AllowedActions:  []string{"stop", "restart"},
	}}}, root)
	if err != nil {
		t.Fatal(err)
	}
	service.scriptAppRoot = configRoot
	service.fileOwnerTrusted = func(os.FileInfo) bool { return true }
	runner := &fakeJobRunner{}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }
	service.scriptInteractiveManageFinder = func() (string, error) { return "/usr/local/bin/k", nil }
	service.scriptManageFinder = func() (string, error) { return "/usr/local/bin/k", nil }

	item, err := service.Find(context.Background(), "thirdparty-AIClient-2-API")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Runtime.Installed || item.Runtime.ContainerName != "aiclient-web" ||
		item.Runtime.AccessMode != "domain_only" {
		t.Fatalf("verified runtime was not reconciled: %#v", item.Runtime)
	}
	for _, detector := range []string{"docker", "appno", "app_config", "access_state"} {
		if !containsString(item.Runtime.DetectedBy, detector) {
			t.Fatalf("missing detector %q: %#v", detector, item.Runtime.DetectedBy)
		}
	}
	for _, action := range []string{"update", "uninstall", "direct_access"} {
		if !item.Capabilities[action].Enabled {
			t.Fatalf("%s was not enabled: %#v", action, item.Capabilities[action])
		}
	}
	if item.Capabilities["manage"].Enabled {
		t.Fatalf("verified docker_app_plus application exposed script management: %#v", item.Capabilities["manage"])
	}
	if _, scriptBacked, err := service.StartScriptMutation(
		context.Background(),
		item.ID,
		"manage",
		MutationInput{ResourceVersion: "stale"},
	); !scriptBacked || !errors.Is(err, ErrForbidden) {
		t.Fatalf("docker_app_plus script management request: script=%v err=%v", scriptBacked, err)
	}
	if _, scriptBacked, err := service.StartScriptMutation(
		context.Background(),
		item.ID,
		"uninstall",
		MutationInput{ResourceVersion: "stale"},
	); !scriptBacked || err == nil {
		t.Fatalf("stale mutation was accepted: script=%v err=%v", scriptBacked, err)
	}
	job, scriptBacked, err := service.StartScriptMutation(
		context.Background(),
		item.ID,
		"direct_access",
		MutationInput{
			ResourceVersion: item.Runtime.ResourceVersion,
			AccessMode:      "direct",
		},
	)
	if err != nil || !scriptBacked {
		t.Fatalf("direct access mutation was rejected: script=%v err=%v", scriptBacked, err)
	}
	record, err := service.jobs.read(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !record.Interactive || record.AccessMode != "direct" ||
		record.ExpectedContainerID != containerID {
		t.Fatalf("direct access job lost its verified target: %#v", record)
	}
	if len(runner.calls) != 1 ||
		!strings.Contains(
			strings.Join(runner.calls[0], " "),
			"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK",
		) {
		t.Fatalf("direct access job cannot use iptables-nft netlink: %#v", runner.calls)
	}
}

func TestStoppedThirdPartyContainerRemainsLifecycleManageableWithoutLegacyMarker(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "apps")
	if err := os.Mkdir(configRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(configRoot, "CLIProxyAPI.conf"),
		[]byte("docker_name=\"CLIProxyAPI\"\ndocker_app_plus\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	containerID := strings.Repeat("e", 64)
	service, err := New(&fakeDocker{containers: []contract.ContainerSummary{{
		ID: containerID, Name: "CLIProxyAPI", Image: "example/cliproxyapi:latest",
		State: "exited", Status: "exited",
		Ports: []contract.PortBinding{{
			PrivatePort: 8317, PublicPort: 11451, IP: "0.0.0.0", Type: "tcp",
		}},
		ResourceVersion: "sha256:" + strings.Repeat("f", 64),
		AllowedActions:  []string{"start"},
	}}}, root)
	if err != nil {
		t.Fatal(err)
	}
	service.scriptAppRoot = configRoot
	service.fileOwnerTrusted = func(os.FileInfo) bool { return true }
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		&fakeJobRunner{},
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }
	service.scriptManageFinder = func() (string, error) { return "/usr/local/bin/k", nil }

	item, err := service.Find(context.Background(), "thirdparty-CLIProxyAPI")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Runtime.Installed || item.Runtime.State != "exited" ||
		item.Runtime.ContainerID != containerID {
		t.Fatalf("stopped runtime was not retained: %#v", item.Runtime)
	}
	if containsString(item.Runtime.DetectedBy, "appno") ||
		!containsString(item.Runtime.DetectedBy, "app_config") {
		t.Fatalf("unexpected runtime evidence: %#v", item.Runtime.DetectedBy)
	}
	for _, action := range []string{"start", "update", "uninstall", "direct_access"} {
		if !item.Capabilities[action].Enabled {
			t.Fatalf("%s was disabled for a verified stopped application: %#v", action, item.Capabilities[action])
		}
	}
	if item.Capabilities["manage"].Enabled {
		t.Fatalf("stopped docker_app_plus application exposed script management: %#v", item.Capabilities["manage"])
	}
}

func TestMarkerOnlyApplicationCanOpenFixedSelectorRecoveryTerminal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "appno.txt"),
		[]byte("114\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	service, err := New(&fakeDocker{}, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		&fakeJobRunner{},
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }
	service.scriptInteractiveManageFinder = func() (string, error) { return "/usr/local/bin/k", nil }

	item, err := service.Find(context.Background(), "builtin-114")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Runtime.Installed || item.Runtime.State != "unknown" ||
		item.Runtime.ContainerID != "" ||
		!strings.HasPrefix(item.Runtime.ResourceVersion, "marker:sha256:") {
		t.Fatalf("marker-only application state is incomplete: %#v", item.Runtime)
	}
	if !item.Capabilities["manage"].Enabled {
		t.Fatalf("marker-only application did not expose script recovery: %#v", item.Capabilities["manage"])
	}
	for _, action := range []string{"start", "stop", "restart", "update", "uninstall", "direct_access"} {
		if item.Capabilities[action].Enabled {
			t.Fatalf("marker-only application exposed %s: %#v", action, item.Capabilities[action])
		}
	}

	job, scriptBacked, err := service.StartScriptMutation(
		context.Background(),
		item.ID,
		"manage",
		MutationInput{ResourceVersion: item.Runtime.ResourceVersion},
	)
	if err != nil || !scriptBacked {
		t.Fatalf("script recovery job = %#v script=%v err=%v", job, scriptBacked, err)
	}
	record, err := service.jobs.read(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Action != "manage" || record.Selector != "114" ||
		record.ExpectedContainerID != "" || !record.Interactive {
		t.Fatalf("unsafe marker recovery job record: %#v", record)
	}
}

func TestDynamicThirdPartyConfigDoesNotDisableLifecycleManagement(t *testing.T) {
	root := t.TempDir()
	configRoot := filepath.Join(root, "apps")
	if err := os.Mkdir(configRoot, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "appno.txt"),
		[]byte("AIClient-2-API\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(configRoot, "AIClient-2-API.conf"),
		[]byte("docker_name=\"${APP_NAME}\"\ndocker_app_plus\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	containerID := strings.Repeat("c", 64)
	service, err := New(&fakeDocker{containers: []contract.ContainerSummary{{
		ID: containerID, Name: "aiclient-runtime", Image: "example/aiclient:latest",
		State: "running", Status: "Up",
		Labels: map[string]string{
			"com.docker.compose.project": "AIClient-2-API",
		},
		ResourceVersion: "sha256:" + strings.Repeat("d", 64),
		AllowedActions:  []string{"stop", "restart", "logs", "exec"},
	}}}, root)
	if err != nil {
		t.Fatal(err)
	}
	service.scriptAppRoot = configRoot
	service.fileOwnerTrusted = func(os.FileInfo) bool { return true }
	if err := service.configureJobs(
		filepath.Join(root, "jobs"),
		filepath.Join(root, "kejilion-agent"),
		&fakeJobRunner{},
	); err != nil {
		t.Fatal(err)
	}
	service.scriptInteractiveFinder = func() (string, error) { return "/usr/local/bin/k", nil }
	service.scriptInteractiveManageFinder = func() (string, error) { return "/usr/local/bin/k", nil }
	service.scriptManageFinder = func() (string, error) { return "/usr/local/bin/k", nil }

	item, err := service.Find(context.Background(), "thirdparty-AIClient-2-API")
	if err != nil {
		t.Fatal(err)
	}
	if !item.Runtime.Installed || item.Runtime.ContainerID != containerID ||
		!containsString(item.Runtime.DetectedBy, "compose_label") {
		t.Fatalf("dynamic script product was not reconciled from Docker: %#v", item.Runtime)
	}
	for _, action := range []string{"update", "uninstall", "direct_access"} {
		if !item.Capabilities[action].Enabled {
			t.Fatalf("%s stayed disabled by config parsing: %#v", action, item.Capabilities[action])
		}
	}
	if item.Capabilities["manage"].Enabled {
		t.Fatalf("dynamic docker_app_plus application exposed script management")
	}
}

func TestThirdPartyScriptConfigRejectsUnsafeOrDynamicContainerMetadata(t *testing.T) {
	root := t.TempDir()
	service, err := New(&fakeDocker{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service.scriptAppRoot = root
	service.fileOwnerTrusted = func(os.FileInfo) bool { return true }

	tests := map[string]string{
		"dynamic":    "docker_name=\"${APP_NAME}\"\ndocker_app_plus\n",
		"duplicate":  "docker_name=one\ndocker_name=two\ndocker_app_plus\n",
		"adapter":    "docker_name=one\ndocker_app\ndocker_app_plus\n",
		"incomplete": "docker_app_plus\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, name+".conf")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := service.readThirdPartyScriptSpec(name); err == nil {
				t.Fatal("unsafe application configuration was accepted")
			}
		})
	}

	target := filepath.Join(root, "target.conf")
	if err := os.WriteFile(target, []byte("docker_name=one\ndocker_app_plus\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "linked.conf")); err == nil {
		if _, err := service.readThirdPartyScriptSpec("linked"); err == nil {
			t.Fatal("symlinked application configuration was accepted")
		}
	}
}

func TestManageCompatibilityRequiresExactNoninteractiveProtocol(t *testing.T) {
	installOnly := []byte(
		"KJ_APP_NONINTERACTIVE\nkpanel_run_docker_app_install\npermission_granted=\"true\"\n",
	)
	if !isKPanelCompatibleScript(installOnly) {
		t.Fatal("install-compatible script was rejected")
	}
	if isKPanelManageCompatibleScript(installOnly) {
		t.Fatal("install-only script enabled destructive management")
	}
	managed := append(
		append([]byte{}, installOnly...),
		[]byte("kpanel_run_docker_app_action\nKJ_APP_EXPECTED_CONTAINER_ID\nKJ_APP_RECONCILE_MARKER\n")...,
	)
	if !isKPanelManageCompatibleScript(managed) {
		t.Fatal("management-compatible script was rejected")
	}
}

func TestThirdPartyScriptSpecReadsFixedInstallPortWithoutExecutingConfig(t *testing.T) {
	for name, assignment := range map[string]string{
		"literal":  `local docker_port="8317"`,
		"fallback": `local docker_port="${docker_port:-3000}"`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			service, err := New(&fakeDocker{}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			service.scriptAppRoot = root
			service.fileOwnerTrusted = func(os.FileInfo) bool { return true }
			content := strings.Join([]string{
				`local docker_name="trusted-app"`,
				assignment,
				"docker_app",
				"",
			}, "\n")
			if err := os.WriteFile(filepath.Join(root, "trusted.conf"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}

			spec, err := service.readThirdPartyScriptSpec("trusted")
			if err != nil {
				t.Fatal(err)
			}
			want := uint16(8317)
			if name == "fallback" {
				want = 3000
			}
			if spec.Port != want {
				t.Fatalf("port = %d, want %d", spec.Port, want)
			}
		})
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
