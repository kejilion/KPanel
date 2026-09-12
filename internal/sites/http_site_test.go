package sites

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestHTTPPortPersistsAndWrapsEveryRecipe(t *testing.T) {
	jobs := []RecipeJob{
		{Recipe: "wordpress"}, {Recipe: "reverse-proxy", ProxyHost: "127.0.0.1", ProxyPort: "3000"},
		{Recipe: "redirect-site", RedirectTarget: "target.example.com"},
	}
	for recipe := range recipeCommands {
		jobs = append(jobs, RecipeJob{Recipe: recipe})
	}
	for _, def := range scriptTemplateDefinitions {
		jobs = append(jobs, RecipeJob{Recipe: def.recipe})
	}
	for _, job := range jobs {
		job.Domain = "example.com"
		legacy, err := invocationForRecipeJob(job)
		if err != nil {
			t.Fatal(err)
		}
		job.HTTPPort = 8443
		data, _ := json.Marshal(job)
		var recovered RecipeJob
		if err := json.Unmarshal(data, &recovered); err != nil {
			t.Fatal(err)
		}
		got, err := invocationForRecipeJob(recovered)
		want := append([]string{"http-site", "8443"}, legacy.arguments...)
		if err != nil || !reflect.DeepEqual(got.arguments, want) || !containsString(got.required, httpSiteProtocol) {
			t.Fatalf("%s: %#v %v", job.Recipe, got, err)
		}
		if !reflect.DeepEqual(legacy.environment, got.environment) {
			t.Fatalf("changed legacy environment: %s", job.Recipe)
		}
		job.Domain = "192.168.1.10"
		if _, err := invocationForRecipeJob(job); err != nil {
			t.Fatalf("IPv4 %s: %v", job.Recipe, err)
		}
	}
	for _, job := range []RecipeJob{
		{Domain: "example.com", Recipe: "static-site", HTTPPort: 65536},
		{Domain: "example.com", Recipe: "static-site", HTTPPort: -1},
		{Domain: "example.com", Recipe: "static-site", HTTPPort: 80, CustomCertificate: true},
		{Domain: "example.com:80", Recipe: "static-site", HTTPPort: 80},
		{Domain: "192.168.1.10", Recipe: "static-site"},
	} {
		if _, err := invocationForRecipeJob(job); err == nil {
			t.Fatalf("accepted %#v", job)
		}
	}
}

func TestHTTPRejectsCertificateAndOldScript(t *testing.T) {
	if _, _, err := normalizeScriptSiteAddress(ScriptSiteInput{SiteInput: SiteInput{PrimaryDomain: "example.com:8443"}, Certificate: "certificate", PrivateKey: "key"}); err == nil {
		t.Fatal("accepted TLS material")
	}
	// The current test host has no trusted HTTP adapter. Failure must happen before
	// enqueuing anything or falling back to the certificate-issuing command.
	if containsAll("#!/bin/bash\n", []string{httpSiteProtocol}) {
		t.Fatal("old script accepted")
	}
}

func TestImportedAccessURLsFollowServerListeners(t *testing.T) {
	for _, tc := range []struct {
		name, config, primary string
		urls                  []string
	}{
		{"http8443", `server { listen 8443; listen [::]:8443; server_name example.com; root /var/www/html/example.com; }`, "example.com", []string{"http://example.com:8443"}},
		{"tls", `server { listen 9443 ssl; server_name example.com; }`, "example.com", []string{"https://example.com:9443"}},
		{"mixed", `server { listen 8080; server_name example.com; } server { listen 9443 ssl; server_name other.com; }`, "example.com", []string{"http://example.com:8080"}},
		{"ipv4", `server { listen 192.168.1.10:8081; server_name 192.168.1.10; }`, "192.168.1.10", []string{"http://192.168.1.10:8081"}},
		{"ipv6", `server { listen [::]:8081; server_name 2001:db8::1; }`, "2001:db8::1", []string{"http://[2001:db8::1]:8081"}},
		{"comments", "# listen 443 ssl;\nserver { listen 8080; server_name \"example.com\"; return 200 'listen 443 ssl; }'; }", "example.com", []string{"http://example.com:8080"}},
		{"no-listen", `server { server_name example.com; }`, "example.com", []string{"http://example.com:80"}},
		{"literal-delimiters", `server { location / { return 200 "}"; } server_name example.com; listen 8443; }`, "example.com", []string{"http://example.com:8443"}},
		{"variable-braces", `server { set $name ${host}; server_name example.com; listen 8443; }`, "example.com", []string{"http://example.com:8443"}},
		{"unix", `server { listen unix:/run/test.sock; server_name example.com; }`, "example.com", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := discoverAccessURLs(tc.config, tc.primary); !reflect.DeepEqual(got, tc.urls) {
				t.Fatalf("got %#v want %#v", got, tc.urls)
			}
		})
	}
	root := t.TempDir()
	for _, name := range []string{"conf.d", "html", "certs"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	config := []byte("server { listen 8443; server_name example.com; root /var/www/html/example.com; }")
	file := filepath.Join(root, "conf.d", "external.conf")
	if err := os.WriteFile(file, config, 0600); err != nil {
		t.Fatal(err)
	}
	sites, err := NewDiscoverer(root).Discover()
	if err != nil || len(sites) != 1 || sites[0].Kind != contract.SiteStatic || !containsString(sites[0].AccessURLs, "http://example.com:8443") {
		t.Fatalf("%#v %v", sites, err)
	}
	after, _ := os.ReadFile(file)
	if string(after) != string(config) {
		t.Fatal("discovery modified external config")
	}
}
