package dockerx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestComposeSourceDiscoveryBound(t *testing.T) {
	client := &Client{appRoot: t.TempDir(), webRoot: t.TempDir()}
	for i := 0; i < 513; i++ {
		if err := os.WriteFile(filepath.Join(client.appRoot, fmt.Sprintf("file-%04d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.managedComposeDirectoriesWithLimit(context.Background(), 512); !errors.Is(err, ErrActionUnsupported) {
		t.Fatalf("unbounded discovery=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.managedComposeDirectoriesWithLimit(ctx, 512); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled discovery=%v", err)
	}
}

func TestComposeSourceNativeIdentity(t *testing.T) {
	for _, mode := range []string{"env", "nested_env", "nested_ancestor_env", "nested_sibling_env", "nested_bad_env", "nested_ordinary_yaml", "label_relative", "label_absolute", "ordinary_yaml", "docker_unavailable", "bad_env"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "demo")
			if mode == "nested_env" || mode == "nested_ancestor_env" || mode == "nested_sibling_env" || mode == "nested_bad_env" || mode == "nested_ordinary_yaml" {
				directory = filepath.Join(directory, "group", "project")
			}
			if err := os.MkdirAll(directory, 0o700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(directory, "prod-stack.conf")
			if mode == "nested_ancestor_env" {
				if err := os.Mkdir(filepath.Join(directory, "config"), 0o700); err != nil {
					t.Fatal(err)
				}
				file = filepath.Join(directory, "config", "prod-stack.conf")
			}
			if mode == "nested_sibling_env" {
				shared := filepath.Join(root, "demo", "shared")
				if err := os.MkdirAll(shared, 0o700); err != nil {
					t.Fatal(err)
				}
				file = filepath.Join(shared, "prod-stack.conf")
			}
			if err := os.WriteFile(file, []byte("services: {}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			var labels map[string]string
			switch mode {
			case "env", "nested_env", "nested_ancestor_env", "nested_sibling_env":
				source, err := filepath.Rel(directory, file)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("COMPOSE_FILE="+filepath.ToSlash(source)+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "label_relative", "label_absolute":
				source := file
				if mode == "label_relative" {
					source = "sub/../prod-stack.conf"
				}
				labels = map[string]string{"com.docker.compose.project": "demo", "com.docker.compose.project.working_dir": directory, "com.docker.compose.project.config_files": source}
			case "bad_env", "nested_bad_env":
				if err := os.WriteFile(filepath.Join(directory, ".env"), []byte("COMPOSE_FILE=${UNKNOWN}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "ordinary_yaml", "nested_ordinary_yaml":
				file = filepath.Join(directory, "settings.yml")
				if err := os.WriteFile(file, []byte("color: blue\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "docker_unavailable" {
					w.WriteHeader(503)
					return
				}
				if r.URL.Path != "/containers/json" {
					t.Errorf("unexpected Docker request: %s", r.URL.Path)
				}
				items := []any{}
				if labels != nil {
					items = append(items, map[string]any{"Id": "fixture", "Names": []string{"/demo"}, "Labels": labels})
				}
				_ = json.NewEncoder(w).Encode(items)
			}))
			defer server.Close()
			client := testHTTPClient(server)
			client.appRoot, client.webRoot = root, t.TempDir()
			got, err := client.IsComposeSource(context.Background(), file)
			if mode == "docker_unavailable" || mode == "bad_env" || mode == "nested_bad_env" {
				if err == nil {
					t.Fatal("uncertain source identity did not fail closed")
				}
				return
			}
			want := mode != "ordinary_yaml" && mode != "nested_ordinary_yaml"
			if err != nil || got != want {
				t.Fatalf("compose source=%v err=%v want=%v", got, err, want)
			}
		})
	}
}

func TestComposeSourceNestedDiscoveryFailsClosed(t *testing.T) {
	t.Run("entry_budget", func(t *testing.T) {
		client := &Client{appRoot: t.TempDir(), webRoot: t.TempDir()}
		nested := filepath.Join(client.appRoot, "group", "project")
		if err := os.MkdirAll(nested, 0o700); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 512; i++ {
			if err := os.WriteFile(filepath.Join(nested, fmt.Sprintf("file-%04d", i)), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := client.managedComposeDirectoriesWithLimit(context.Background(), 512); !errors.Is(err, ErrActionUnsupported) {
			t.Fatalf("nested entries escaped discovery budget: %v", err)
		}
	})
	t.Run("directory_link", func(t *testing.T) {
		client := &Client{appRoot: t.TempDir(), webRoot: t.TempDir()}
		if err := os.Symlink(t.TempDir(), filepath.Join(client.appRoot, "project")); err != nil {
			t.Skipf("symlink fixture unavailable: %v", err)
		}
		if _, err := client.managedComposeDirectoriesWithLimit(context.Background(), 512); !errors.Is(err, ErrActionUnsupported) {
			t.Fatalf("linked discovery was treated as complete: %v", err)
		}
	})
}
