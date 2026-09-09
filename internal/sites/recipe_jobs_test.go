package sites

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type fakeRecipeJobRunner struct {
	active  map[string]bool
	unknown bool
}

func (runner *fakeRecipeJobRunner) Run(
	_ context.Context,
	name string,
	arguments ...string,
) ([]byte, error) {
	if runner.unknown {
		return nil, errors.New("systemd unavailable")
	}
	if name == "systemctl" && len(arguments) == 2 &&
		arguments[0] == "is-active" && runner.active[arguments[1]] {
		return []byte("active\n"), nil
	}
	return []byte("inactive\n"), errors.New("inactive")
}

func TestNormalizeRecipeInputUsesFixedKejilionSelectors(t *testing.T) {
	for recipe, expected := range map[string]string{
		"discuz": "3", "bitwarden": "25", "halo": "26",
	} {
		domain, selector, err := normalizeRecipeInput(SiteInput{
			PrimaryDomain: recipe + ".example.com",
			Type:          "recipe",
			Recipe:        recipe,
		})
		if err != nil || domain != recipe+".example.com" || selector != expected {
			t.Fatalf("normalizeRecipeInput(%q) = %q, %q, %v", recipe, domain, selector, err)
		}
	}
	for _, input := range []SiteInput{
		{PrimaryDomain: "forum.example.com", Type: "recipe", Recipe: "unknown"},
		{PrimaryDomain: "forum.example.com", Type: "recipe", Recipe: "discuz", Aliases: []string{"www.example.com"}},
		{PrimaryDomain: "forum.example.com", Type: "php", Recipe: "discuz"},
	} {
		if _, _, err := normalizeRecipeInput(input); err == nil {
			t.Fatalf("unsafe recipe input was accepted: %#v", input)
		}
	}
}

func TestNormalizeWordPressInputUsesOnlyKejilionDomain(t *testing.T) {
	spec, err := normalizeWordPressInput(SiteInput{
		PrimaryDomain: "blog.example.com",
		Type:          "wordpress",
	})
	if err != nil || spec.Primary != "blog.example.com" || spec.Kind != contract.SiteWordPress {
		t.Fatalf("normalizeWordPressInput() = %#v, %v", spec, err)
	}
	for _, input := range []SiteInput{
		{PrimaryDomain: "blog.example.com", Type: "php"},
		{PrimaryDomain: "blog.example.com", Type: "wordpress", Aliases: []string{"www.example.com"}},
		{PrimaryDomain: "blog.example.com", Type: "wordpress", Upstream: "http://127.0.0.1:8080"},
	} {
		if _, err := normalizeWordPressInput(input); err == nil {
			t.Fatalf("non-kejilion WordPress input was accepted: %#v", input)
		}
	}
}

func TestNormalizeDirectProxyInputExtractsKejilionHostAndPort(t *testing.T) {
	spec, host, port, err := normalizeDirectProxyInput(SiteInput{
		PrimaryDomain: "proxy.example.com",
		Type:          "proxy",
		Upstream:      "http://127.0.0.1:8080",
	})
	if err != nil || spec.Kind != contract.SiteReverseProxy ||
		host != "127.0.0.1" || port != "8080" {
		t.Fatalf(
			"normalizeDirectProxyInput() = %#v, %q, %q, %v",
			spec,
			host,
			port,
			err,
		)
	}
	for _, input := range []SiteInput{
		{PrimaryDomain: "proxy.example.com", Type: "proxy", Upstream: "http://127.0.0.1"},
		{PrimaryDomain: "proxy.example.com", Type: "proxy", Upstream: "https://127.0.0.1:8443"},
		{PrimaryDomain: "proxy.example.com", Type: "proxy", Upstream: "http://[::1]:8080"},
		{
			PrimaryDomain: "proxy.example.com",
			Type:          "proxy",
			Upstream:      "http://127.0.0.1:8080",
			Aliases:       []string{"www.example.com"},
		},
	} {
		if _, _, _, err := normalizeDirectProxyInput(input); err == nil {
			t.Fatalf("non-kejilion proxy input was accepted: %#v", input)
		}
	}
}

func TestContainsAllRequiresExactWebsiteCommandTokens(t *testing.T) {
	value := `KJ_WEB_RECIPE KJ_WEB_DOMAIN KJ_WEB_PROXY_HOST KJ_WEB_PROXY_PORT ldnmp_wp "${KJ_WEB_DOMAIN:-}" ldnmp_Proxy "${KJ_WEB_DOMAIN:-}" "${KJ_WEB_PROXY_HOST:-}" "${KJ_WEB_PROXY_PORT:-}"`
	if !containsAll(value, []string{"KJ_WEB_RECIPE", `ldnmp_wp "${KJ_WEB_DOMAIN:-}"`}) {
		t.Fatal("expected WordPress command tokens to match")
	}
	if containsAll(value, []string{"KJ_WEB_PROXY_TLS"}) {
		t.Fatal("unexpected website command token matched")
	}
}

func TestDirectWebsiteInvocationsUseKejilionCommands(t *testing.T) {
	wordPress := wordPressInvocation("blog.example.com")
	if !reflect.DeepEqual(wordPress.arguments, []string{"wp", "blog.example.com"}) ||
		!reflect.DeepEqual(
			wordPress.environment,
			[]string{
				"KJ_WEB_NONINTERACTIVE=1",
				"KJ_WEB_INTERACTIVE=1",
				"KJ_WEB_RECIPE=2",
				"KJ_WEB_DOMAIN=blog.example.com",
			},
		) {
		t.Fatalf("WordPress invocation = %#v", wordPress)
	}
	proxy := proxyInvocation("proxy.example.com", "127.0.0.1", "8080")
	if !reflect.DeepEqual(proxy.arguments, []string{"fd", "proxy.example.com", "127.0.0.1", "8080"}) ||
		!reflect.DeepEqual(
			proxy.environment,
			[]string{
				"KJ_WEB_NONINTERACTIVE=1",
				"KJ_WEB_INTERACTIVE=1",
				"KJ_WEB_RECIPE=23",
				"KJ_WEB_DOMAIN=proxy.example.com",
				"KJ_WEB_PROXY_HOST=127.0.0.1",
				"KJ_WEB_PROXY_PORT=8080",
			},
		) {
		t.Fatalf("proxy invocation = %#v", proxy)
	}
}

func TestScriptedTemplatesUseFixedInteractiveKejilionCommands(t *testing.T) {
	tests := []struct {
		siteType string
		command  string
		selector string
		kind     contract.SiteKind
	}{
		{siteType: "static", command: "static-site", selector: "30", kind: contract.SiteStatic},
		{siteType: "php", command: "php-site", selector: "20", kind: contract.SitePHP},
		{siteType: "proxy_domain", command: "domain-proxy", selector: "24", kind: contract.SiteDomainProxy},
		{siteType: "load_balance", command: "loadbalance-site", selector: "28", kind: contract.SiteLoadBalance},
		{siteType: "redirect", command: "redirect-site", selector: "22", kind: contract.SiteRedirect},
	}
	for _, test := range tests {
		t.Run(test.siteType, func(t *testing.T) {
			domain := strings.ReplaceAll(test.siteType, "_", "-") + ".example.com"
			domain, definition, err := normalizeTemplateInput(SiteInput{
				PrimaryDomain: domain,
				Type:          test.siteType,
			})
			if err != nil || definition.kind != test.kind {
				t.Fatalf("normalizeTemplateInput() = %q, %#v, %v", domain, definition, err)
			}
			invocation := templateInvocation(domain, definition)
			if !reflect.DeepEqual(invocation.arguments, []string{test.command, domain}) ||
				!containsAll(strings.Join(invocation.environment, "\n"), []string{
					"KJ_WEB_NONINTERACTIVE=1",
					"KJ_WEB_INTERACTIVE=1",
					"KJ_WEB_RECIPE=" + test.selector,
					"KJ_WEB_DOMAIN=" + domain,
				}) {
				t.Fatalf("template invocation = %#v", invocation)
			}
		})
	}
	for _, input := range []SiteInput{
		{PrimaryDomain: "static.example.com", Type: "static", Aliases: []string{"www.example.com"}},
		{PrimaryDomain: "php.example.com", Type: "php", PHPVersion: "7.4"},
		{PrimaryDomain: "proxy.example.com", Type: "proxy_domain", Upstream: "https://origin.example.com"},
		{PrimaryDomain: "bad.example.com", Type: "unknown"},
	} {
		if _, _, err := normalizeTemplateInput(input); err == nil {
			t.Fatalf("unsafe scripted template input was accepted: %#v", input)
		}
	}
}

func TestSiteWorkerSystemdArgumentsDetachFromAgent(t *testing.T) {
	job := RecipeJob{ID: "0123456789abcdef0123456789abcdef"}
	arguments := siteWorkerSystemdArguments(
		job,
		"/usr/local/bin/kejilion-agent",
		"/var/lib/kejilion-panel/site-recipe-jobs",
		"/home/web",
	)
	joined := strings.Join(arguments, "\n")
	for _, expected := range []string{
		"--unit=kpanel-site-" + job.ID,
		"--no-block",
		"--property=ProtectSystem=no",
		"--",
		"/usr/local/bin/kejilion-agent",
		"site-pty-run",
		"--state-dir",
		"/var/lib/kejilion-panel/site-recipe-jobs",
		"--web-root",
		"/home/web",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("systemd site arguments missing %q: %#v", expected, arguments)
		}
	}
	for _, forbidden := range []string{"--wait", "--pipe"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("detached website worker contains %q: %#v", forbidden, arguments)
		}
	}
	for _, argument := range arguments {
		if strings.ContainsAny(argument, ";&`") {
			t.Fatalf("shell fragment reached systemd invocation: %q", argument)
		}
	}
}

func TestInvocationForRecipeJobRestoresValidatedCommands(t *testing.T) {
	for _, test := range []struct {
		job  RecipeJob
		want []string
	}{
		{
			job:  RecipeJob{Domain: "blog.example.com", Recipe: "wordpress"},
			want: []string{"wp", "blog.example.com"},
		},
		{
			job: RecipeJob{
				Domain: "proxy.example.com", Recipe: "reverse-proxy",
				ProxyHost: "127.0.0.1", ProxyPort: "8080",
			},
			want: []string{"fd", "proxy.example.com", "127.0.0.1", "8080"},
		},
		{
			job:  RecipeJob{Domain: "forum.example.com", Recipe: "discuz"},
			want: []string{"discuz", "forum.example.com"},
		},
		{
			job:  RecipeJob{Domain: "static.example.com", Recipe: "static-site"},
			want: []string{"static-site", "static.example.com"},
		},
		{
			job:  RecipeJob{Domain: "redirect.example.com", Recipe: "redirect-site"},
			want: []string{"redirect-site", "redirect.example.com"},
		},
	} {
		invocation, err := invocationForRecipeJob(test.job)
		if err != nil || !reflect.DeepEqual(invocation.arguments, test.want) {
			t.Fatalf("invocationForRecipeJob(%#v) = %#v, %v", test.job, invocation, err)
		}
	}
	if _, err := invocationForRecipeJob(RecipeJob{
		Domain: "proxy.example.com", Recipe: "reverse-proxy",
		ProxyHost: "127.0.0.1;id", ProxyPort: "8080",
	}); err == nil {
		t.Fatal("unsafe persisted proxy host was accepted")
	}
}

func TestRedirectTemplatePassesTargetToScriptAndRestoresJob(t *testing.T) {
	input := SiteInput{PrimaryDomain: "old.example.com", Type: "redirect", RedirectTarget: "https://NEW.example.com", RedirectCode: 301}
	if _, _, err := normalizeTemplateInput(input); err != nil {
		t.Fatal(err)
	}
	target, err := normalizeScriptRedirectTarget(input.RedirectTarget, input.PrimaryDomain)
	if err != nil || target != "new.example.com" {
		t.Fatalf("target = %q, %v", target, err)
	}
	invocation := redirectInvocation(input.PrimaryDomain, target)
	job := RecipeJob{Domain: input.PrimaryDomain, Recipe: "redirect-site", RedirectTarget: target}
	encoded, _ := json.Marshal(job)
	var restored RecipeJob
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	got, err := invocationForRecipeJob(restored)
	if err != nil || !reflect.DeepEqual(got, invocation) || !reflect.DeepEqual(got.arguments, []string{"redirect-site", "old.example.com", "new.example.com"}) {
		t.Fatalf("restored invocation = %#v, %v", got, err)
	}
	if !containsAll(strings.Join(got.required, "\n"), []string{`KPANEL_WEB_REDIRECT_PROTOCOL_VERSION="1"`}) {
		t.Fatal("new target protocol must be required before execution")
	}
	for _, raw := range []string{"https://old.example.com", "http://new.example.com", "https://new.example.com:8443", "https://new.example.com/path", "https://new.example.com?x=1", "https://new.example.com;id", "https://127.0.0.1", "https://user@new.example.com"} {
		input.RedirectTarget = raw
		if _, _, err := normalizeTemplateInput(input); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	input.RedirectTarget = "https://new.example.com"
	input.RedirectCode = 308
	if _, _, err := normalizeTemplateInput(input); err == nil {
		t.Fatal("accepted non-script redirect code")
	}
	for _, raw := range []string{"new.example.com;id", "new.example.com/path", "old.example.com", "NEW.example.com"} {
		job.RedirectTarget = raw
		if _, err := invocationForRecipeJob(job); err == nil {
			t.Fatalf("accepted persisted target %q", raw)
		}
	}
}

func TestRecipeFailureMessagePreservesSafeProtocolReason(t *testing.T) {
	job := RecipeJob{
		Message:  "Nginx 配置校验失败",
		Progress: 85,
	}
	message := recipeFailureMessage(job, "failed", errors.New("exit status 1"))
	if message != "建站失败：Nginx 配置校验失败" {
		t.Fatalf("recipeFailureMessage() = %q", message)
	}
	if got := recipeFailureMessage(job, "reconcile_failed", errors.New("missing")); !strings.Contains(got, "未发现完整站点产物") {
		t.Fatalf("reconcile failure message = %q", got)
	}
}

func TestRecipeJobEventsAreBoundedAndDeduplicated(t *testing.T) {
	job := RecipeJob{}
	appendRecipeEvent(&job, "queued", 0, "等待执行")
	appendRecipeEvent(&job, "queued", 0, "等待执行")
	if len(job.Events) != 1 {
		t.Fatalf("duplicate events = %d, want 1", len(job.Events))
	}
	for index := 0; index < 60; index++ {
		appendRecipeEvent(&job, "installing", index, "执行阶段")
	}
	if len(job.Events) != 48 {
		t.Fatalf("events = %d, want 48", len(job.Events))
	}
	if job.Events[0].Progress != 12 || job.Events[len(job.Events)-1].Progress != 59 {
		t.Fatalf("bounded events = %#v", job.Events)
	}
}

func TestRecipeJobRegistryRefreshesWorkerStateFromDisk(t *testing.T) {
	stateDir := t.TempDir()
	agentRegistry := newRecipeJobRegistry(stateDir)
	workerRegistry := newRecipeJobRegistry(stateDir)
	id := "0123456789abcdef0123456789abcdef"
	job := RecipeJob{
		ID: id, Domain: "blog.example.com", Recipe: "wordpress",
		Status: "queued", Stage: "queued", CreatedAt: time.Now().UTC(),
	}
	if err := agentRegistry.put(job); err != nil {
		t.Fatal(err)
	}
	job.Status = "running"
	job.Stage = "installing"
	job.Progress = 38
	if err := workerRegistry.put(job); err != nil {
		t.Fatal(err)
	}
	jobs := agentRegistry.list()
	if len(jobs) != 1 || jobs[0].Status != "running" ||
		jobs[0].Stage != "installing" || jobs[0].Progress != 38 {
		t.Fatalf("agent registry did not refresh worker state: %#v", jobs)
	}
}

func TestConfigureRecipeJobsPreservesActiveDetachedWorker(t *testing.T) {
	stateDir := t.TempDir()
	id := "0123456789abcdef0123456789abcdef"
	registry := newRecipeJobRegistry(stateDir)
	job := RecipeJob{
		ID: id, Domain: "blog.example.com", Recipe: "wordpress",
		Status: "running", Stage: "installing", Progress: 38, CreatedAt: time.Now().UTC(),
	}
	if err := registry.put(job); err != nil {
		t.Fatal(err)
	}
	manager := NewManager("/home/web", nil, nil)
	runner := &fakeRecipeJobRunner{active: map[string]bool{
		recipeJobUnitPrefix + id + ".service": true,
	}}
	if err := manager.configureRecipeJobState(
		stateDir,
		filepath.Join(t.TempDir(), "kejilion-agent"),
		runner,
	); err != nil {
		t.Fatal(err)
	}
	recovered, err := manager.recipeJobs.read(id)
	if err != nil || recovered.Status != "running" ||
		recovered.Stage != "installing" || recovered.Progress != 38 {
		t.Fatalf("active detached website worker was not preserved: %#v, %v", recovered, err)
	}
}

func TestConfigureRecipeJobsDoesNotFailOnUnknownSystemdState(t *testing.T) {
	stateDir := t.TempDir()
	id := "0123456789abcdef0123456789abcdef"
	registry := newRecipeJobRegistry(stateDir)
	job := RecipeJob{
		ID: id, Domain: "blog.example.com", Recipe: "wordpress",
		Status: "running", Stage: "installing", Progress: 38, CreatedAt: time.Now().UTC(),
	}
	if err := registry.put(job); err != nil {
		t.Fatal(err)
	}
	manager := NewManager("/home/web", nil, nil)
	if err := manager.configureRecipeJobState(
		stateDir,
		filepath.Join(t.TempDir(), "kejilion-agent"),
		&fakeRecipeJobRunner{unknown: true},
	); err != nil {
		t.Fatal(err)
	}
	recovered, err := manager.recipeJobs.read(id)
	if err != nil || recovered.Status != "running" {
		t.Fatalf("unknown systemd state was treated as task failure: %#v, %v", recovered, err)
	}
}

func TestSiteInstallationTerminalReturnsRawChunks(t *testing.T) {
	stateDir := t.TempDir()
	registry := newRecipeJobRegistry(stateDir)
	manager := &Manager{recipeJobs: registry}
	id := "0123456789abcdef0123456789abcdef"
	job := RecipeJob{
		ID:          id,
		Domain:      "proxy.example.com",
		Recipe:      "reverse-proxy",
		Status:      "running",
		Stage:       "installing",
		Interactive: true,
		InputOpen:   true,
	}
	if err := registry.put(job); err != nil {
		t.Fatal(err)
	}
	raw := []byte("\x1b[32mkejilion.sh terminal\x1b[0m\r\n")
	if err := os.WriteFile(registry.logPath(id), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	chunk, err := manager.InstallationTerminal(id, 0)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(chunk.DataBase64)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(raw) || chunk.NextOffset != int64(len(raw)) ||
		!chunk.InputOpen || chunk.Finished {
		t.Fatalf("terminal chunk = %#v raw=%q", chunk, decoded)
	}
}

func TestStripSiteTerminalControlsKeepsProgressPayload(t *testing.T) {
	got := stripSiteTerminalControls("\x1b[2K\rKPANEL_PROGRESS 35 正在安装\x1b[0m\r\n")
	if got != "\rKPANEL_PROGRESS 35 正在安装\r\n" {
		t.Fatalf("stripped terminal output = %q", got)
	}
}
