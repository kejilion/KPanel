package sites

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccessPasswordRulesReadCurrentSiteAndDetectFileChanges(t *testing.T) {
	root := t.TempDir()
	copyFixtureTree(t, filepath.Join("testdata", "web"), root)
	config := `server { server_name static.example.com; root /var/www/html/static.example.com;
		auth_basic "Private"; auth_basic_user_file /etc/nginx/conf.d/.users;
		location /admin/ { auth_basic "Admin"; auth_basic_user_file /etc/nginx/conf.d/missing; }
	}`
	if err := os.WriteFile(filepath.Join(root, "conf.d", "static.example.com.conf"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	users := filepath.Join(root, "conf.d", ".users")
	if err := os.WriteFile(users, []byte("alice:$apr1$secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d := NewDiscoverer(root)
	all, err := d.Discover()
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(root, d, nil)
	first, err := manager.AccessPasswordRules(findSite(all, "static.example.com").ID)
	if err != nil || len(first) != 2 {
		t.Fatalf("got %#v, %v", first, err)
	}
	if first[0].AccountsReadable || !first[1].AccountsReadable || len(first[1].Usernames) != 1 || first[1].Usernames[0] != "alice" {
		t.Fatalf("unexpected rules %#v", first)
	}
	encoded, err := json.Marshal(first)
	if err != nil || strings.Contains(string(encoded), "$apr1$secret") {
		t.Fatalf("password hash leaked: %s", encoded)
	}
	if err := os.WriteFile(users, []byte("alice:$apr1$changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := manager.AccessPasswordRules(findSite(all, "static.example.com").ID)
	if err != nil || first[1].ID != second[1].ID || first[1].ResourceVersion == second[1].ResourceVersion {
		t.Fatalf("file change not detected: %#v, %v", second, err)
	}
}

func TestAccessPasswordFileRejectsUnmappedOrEscapingPaths(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "conf.d"), 0700); err != nil {
		t.Fatal(err)
	}
	m := NewManager(root, nil, nil)
	for _, path := range []string{"/etc/shadow", "/etc/nginx/conf.d/../certs/key", "/etc/nginx/conf.d/$user", "../conf.d/users"} {
		if _, err := m.readAccessPasswordFile(path); err == nil {
			t.Fatalf("unexpectedly read %q", path)
		}
	}
}

func TestAccessPasswordFileReadsRelativeNginxPath(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "conf.d"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conf.d", "users"), []byte("alice:hash\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := NewManager(root, nil, nil).readAccessPasswordFile("conf.d/users")
	if err != nil || string(data) != "alice:hash\n" {
		t.Fatalf("read relative path: %q, %v", data, err)
	}
}
