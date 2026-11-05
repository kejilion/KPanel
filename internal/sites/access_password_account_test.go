package sites

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeAccessAccountEditor struct {
	calls   int
	request accessAccountRequest
}

func (*fakeAccessAccountEditor) Available(string) error { return nil }

func (f *fakeAccessAccountEditor) Edit(_ context.Context, root, _ string, payload any) error {
	f.calls++
	f.request = payload.(accessAccountRequest)
	return os.WriteFile(filepath.Join(root, "conf.d", ".users"), []byte("bob:$apr1$fake\n"), 0600)
}

func TestAccessPasswordAccountUsesResolvedRuleAndRejectsStaleVersion(t *testing.T) {
	root := t.TempDir()
	copyFixtureTree(t, filepath.Join("testdata", "web"), root)
	config := `server { server_name static.example.com; root /var/www/html/static.example.com;
		location /admin/ { proxy_pass http://app; auth_basic "Private"; auth_basic_user_file /etc/nginx/conf.d/.users; } }`
	if err := os.WriteFile(filepath.Join(root, "conf.d", "static.example.com.conf"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conf.d", ".users"), []byte("alice:$apr1$old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d := NewDiscoverer(root)
	all, err := d.Discover()
	if err != nil {
		t.Fatal(err)
	}
	id := findSite(all, "static.example.com").ID
	m := NewManager(root, d, nil)
	fake := &fakeAccessAccountEditor{}
	m.accessAccountEditor = fake
	rules, err := m.AccessPasswordRules(id)
	if err != nil || len(rules) != 1 {
		t.Fatalf("rules: %#v, %v", rules, err)
	}
	input := AccessPasswordAccountInput{RuleID: rules[0].ID, ResourceVersion: rules[0].ResourceVersion, Action: "update", Username: "bob", PreviousUsername: "alice", Password: "synthetic-secret"}
	updated, err := m.EditAccessPasswordAccount(context.Background(), id, input)
	if err != nil || len(updated) != 1 || len(updated[0].Usernames) != 1 || updated[0].Usernames[0] != "bob" {
		t.Fatalf("updated: %#v, %v", updated, err)
	}
	if fake.calls != 1 || fake.request.ConfigName != "static.example.com.conf" || fake.request.Path != "/admin/" || fake.request.Password != "synthetic-secret" {
		t.Fatalf("unexpected request: %#v", fake.request)
	}
	_, err = m.EditAccessPasswordAccount(context.Background(), id, input)
	if !errors.Is(err, ErrConflict) || fake.calls != 1 {
		t.Fatalf("stale request: %v, calls=%d", err, fake.calls)
	}
}

func TestAccessPasswordAccountInputRejectsUnsafeFields(t *testing.T) {
	valid := AccessPasswordAccountInput{RuleID: "0123456789abcdef0123456789abcdef", ResourceVersion: "sha256:" + string(bytesOf('a', 64)), Action: "add", Username: "test", Password: "secret"}
	if err := validateAccessAccountInput(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*AccessPasswordAccountInput){
		func(i *AccessPasswordAccountInput) { i.Username = "user:other" },
		func(i *AccessPasswordAccountInput) { i.Password = "secret\nother" },
		func(i *AccessPasswordAccountInput) { i.Password = strings.Repeat("密", 86) },
		func(i *AccessPasswordAccountInput) { i.Action = "delete" },
	} {
		input := valid
		mutate(&input)
		if err := validateAccessAccountInput(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("unexpected validation result %v", err)
		}
	}
}

func bytesOf(value byte, length int) []byte {
	result := make([]byte, length)
	for i := range result {
		result[i] = value
	}
	return result
}
