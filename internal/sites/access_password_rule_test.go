package sites

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeAccessRuleEditor struct {
	calls int
	input any
}

func (*fakeAccessRuleEditor) Available(string) error { return nil }
func (f *fakeAccessRuleEditor) Edit(_ context.Context, _, _ string, input any) error {
	f.calls++
	f.input = input
	return nil
}

func TestAccessPasswordRuleAddSelectsAllSiteServers(t *testing.T) {
	root := t.TempDir()
	copyFixtureTree(t, filepath.Join("testdata", "web"), root)
	config := `server { listen 80; server_name static.example.com; root /var/www/html/static.example.com; }
	server { listen 443 ssl; server_name static.example.com; root /var/www/html/static.example.com; }`
	if err := os.WriteFile(filepath.Join(root, "conf.d", "static.example.com.conf"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	d := NewDiscoverer(root)
	all, err := d.Discover()
	if err != nil {
		t.Fatal(err)
	}
	site := findSite(all, "static.example.com")
	m := NewManager(root, d, nil)
	fake := &fakeAccessRuleEditor{}
	m.accessAccountEditor = fake
	input := AccessPasswordRuleInput{Action: "add", ResourceVersion: site.ResourceVersion, Scope: "server", Username: "alice", Password: "secret"}
	if _, err := m.EditAccessPasswordRule(context.Background(), site.ID, input); err != nil {
		t.Fatal(err)
	}
	request, ok := fake.input.(accessRuleAddRequest)
	if !ok || len(request.ServerIndices) != 2 || request.ServerIndices[0] != 0 || request.ServerIndices[1] != 1 {
		t.Fatalf("wrong target: %#v", fake.input)
	}
	input.ResourceVersion = "sha256:" + string(bytesOf('a', 64))
	if _, err := m.EditAccessPasswordRule(context.Background(), site.ID, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale config accepted: %v", err)
	}
	if fake.calls != 1 {
		t.Fatalf("unexpected calls: %d", fake.calls)
	}
}

func TestAccessPasswordRuleDeleteSelectsExactDiscoveredContext(t *testing.T) {
	root := t.TempDir()
	copyFixtureTree(t, filepath.Join("testdata", "web"), root)
	config := `server { server_name static.example.com; root /var/www/html/static.example.com; location /admin { auth_basic "Private"; auth_basic_user_file /etc/nginx/conf.d/.users; } }`
	if err := os.WriteFile(filepath.Join(root, "conf.d", "static.example.com.conf"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conf.d", ".users"), []byte("alice:hash\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d := NewDiscoverer(root)
	all, err := d.Discover()
	if err != nil {
		t.Fatal(err)
	}
	id := findSite(all, "static.example.com").ID
	m := NewManager(root, d, nil)
	fake := &fakeAccessRuleEditor{}
	m.accessAccountEditor = fake
	rules, err := m.AccessPasswordRules(id)
	if err != nil || len(rules) != 1 {
		t.Fatalf("rules: %#v, %v", rules, err)
	}
	input := AccessPasswordRuleInput{Action: "delete", RuleID: rules[0].ID, ResourceVersion: rules[0].ResourceVersion}
	if _, err := m.EditAccessPasswordRule(context.Background(), id, input); err != nil {
		t.Fatal(err)
	}
	request, ok := fake.input.(accessRuleDeleteRequest)
	if !ok || request.Path != "/admin" || request.Scope != "location" {
		t.Fatalf("wrong target: %#v", fake.input)
	}
}
