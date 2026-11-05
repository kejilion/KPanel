package sites

import (
	"reflect"
	"testing"
)

func TestAccessPasswordDirectivesReadActualConfig(t *testing.T) {
	config := `server {
		server_name example.com;
		# auth_basic "ignored";
		auth_basic "Private site";
		auth_basic_user_file /etc/nginx/conf.d/.users;
		location /admin/ { auth_basic 'Admin area'; auth_basic_user_file /etc/nginx/conf.d/admin.users; proxy_pass http://app; }
		location /public/ { auth_basic off; }
	}`
	want := []accessPasswordDirective{
		{Scope: "location", Context: "/admin/", Realm: "Admin area", UserFile: "/etc/nginx/conf.d/admin.users"},
		{Scope: "server", Context: "/", Realm: "Private site", UserFile: "/etc/nginx/conf.d/.users"},
	}
	if got, err := parseAccessPasswordDirectives(config); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v, want %#v", got, err, want)
	}
}

func TestAccessPasswordDirectivesDoNotInventRules(t *testing.T) {
	config := `server {
		auth_basic "one"; auth_basic "two"; auth_basic_user_file /etc/nginx/conf.d/users;
		location /incomplete { auth_basic "Only a realm"; }
		location /off { auth_basic off; auth_basic_user_file /etc/nginx/conf.d/users; }
	}`
	if got, err := parseAccessPasswordDirectives(config); err != nil || len(got) != 0 {
		t.Fatalf("unexpected rules: %#v, %v", got, err)
	}
}

func TestAccessPasswordDirectivesRejectUnbalancedBlocks(t *testing.T) {
	for _, config := range []string{
		`server { location /admin { auth_basic "A"; auth_basic_user_file /a; }`,
		`server { auth_basic "A"; auth_basic_user_file /a; } }`,
	} {
		if got, err := parseAccessPasswordDirectives(config); err == nil || len(got) != 0 {
			t.Fatalf("unbalanced configuration produced rules: %#v, %v", got, err)
		}
	}
}

func TestAccessPasswordDirectivesPreserveServerAndLocationIdentity(t *testing.T) {
	config := `http {
		server { location = /admin { auth_basic "A"; auth_basic_user_file /a; } }
		server { location ^~ /admin/ { auth_basic "B"; auth_basic_user_file /b; }
			location ~* \.php$ { auth_basic "C"; auth_basic_user_file /c; } }
	}
	location /invalid { auth_basic "X"; auth_basic_user_file /x; }`
	want := []accessPasswordDirective{
		{ServerIndex: 0, Scope: "location", Context: "/admin", Modifier: "=", Realm: "A", UserFile: "/a"},
		{ServerIndex: 1, Scope: "location", Context: "/admin/", Modifier: "^~", Realm: "B", UserFile: "/b"},
		{ServerIndex: 1, Scope: "location", Context: `\.php$`, Modifier: "~*", Realm: "C", UserFile: "/c"},
	}
	if got, err := parseAccessPasswordDirectives(config); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v, want %#v", got, err, want)
	}
}

func TestAccessPasswordDirectivesResolveInheritedFields(t *testing.T) {
	config := `http {
		server {
			location /admin/ { auth_basic "Admin"; }
			location /public/ { auth_basic off; }
			location /other/ { auth_basic_user_file /etc/nginx/conf.d/other; }
			auth_basic "Private";
		}
		auth_basic_user_file /etc/nginx/conf.d/users;
	}`
	want := []accessPasswordDirective{
		{Scope: "location", Context: "/admin/", Realm: "Admin", UserFile: "/etc/nginx/conf.d/users"},
		{Scope: "location", Context: "/other/", Realm: "Private", UserFile: "/etc/nginx/conf.d/other"},
		{Scope: "server", Context: "/", Realm: "Private", UserFile: "/etc/nginx/conf.d/users"},
	}
	if got, err := parseAccessPasswordDirectives(config); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v, want %#v", got, err, want)
	}
}
