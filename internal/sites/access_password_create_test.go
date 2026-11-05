package sites

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAccessPasswordCreationStagesPrivateInputsWithoutJobSecret(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX private file mode requires Linux")
	}
	root := t.TempDir()
	id := strings.Repeat("a", 32)
	input := &AccessPasswordCreateInput{Scope: "server", Username: "alice", Password: "synthetic-secret"}
	if err := validateAccessPasswordCreate(input); err != nil {
		t.Fatal(err)
	}
	if err := stageAccessPassword(root, id, "example.com", input); err != nil {
		t.Fatal(err)
	}
	path, _ := accessPasswordStagePath(root, id)
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("stage permissions: %v, %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "synthetic-secret") {
		t.Fatalf("missing private stage: %v", err)
	}
	invocation, err := withAccessPasswordInvocation(scriptSiteInvocation{}, root, id, true)
	if err != nil || len(invocation.environment) != 1 || strings.Contains(strings.Join(invocation.environment, ","), "synthetic-secret") {
		t.Fatalf("invalid invocation: %#v, %v", invocation, err)
	}
	jobJSON, err := json.Marshal(RecipeJob{ID: id, AccessPassword: true})
	if err != nil || strings.Contains(string(jobJSON), "synthetic-secret") {
		t.Fatalf("secret in job state: %s", jobJSON)
	}
	cleanupSiteCreationSecrets(root, id)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("private input not cleaned: %v", err)
	}
}

func TestAccessPasswordCreationMustReconcileBeforeSuccess(t *testing.T) {
	root := t.TempDir()
	copyFixtureTree(t, filepath.Join("testdata", "web"), root)
	state := t.TempDir()
	id := strings.Repeat("b", 32)
	input := &AccessPasswordCreateInput{Scope: "server", Username: "alice", Password: "synthetic-secret"}
	if err := stageAccessPassword(state, id, "static.example.com", input); err != nil {
		t.Fatal(err)
	}
	d := NewDiscoverer(root)
	m := NewManager(root, d, nil)
	m.recipeJobs = newRecipeJobRegistry(state)
	job := RecipeJob{ID: id, Domain: "static.example.com", AccessPassword: true}
	all, err := d.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.verifyCreatedAccess(job, findSite(all, job.Domain)); err == nil {
		t.Fatal("missing access rule was reported as verified")
	}
	config := `server { listen 80; server_name static.example.com; return 301 https://static.example.com$request_uri; }
	server { listen 443 ssl; server_name static.example.com; root /var/www/html/static.example.com; auth_basic "Private"; auth_basic_user_file /etc/nginx/conf.d/.users; }`
	if err := os.WriteFile(filepath.Join(root, "conf.d", job.Domain+".conf"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conf.d", ".users"), []byte("alice:hash\n"), 0600); err != nil {
		t.Fatal(err)
	}
	all, err = d.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.verifyCreatedAccess(job, findSite(all, job.Domain)); err != nil {
		t.Fatal(err)
	}
}

func TestAccessPasswordCreationFailsWithoutRequiredStagedFile(t *testing.T) {
	root, id := t.TempDir(), strings.Repeat("a", 32)
	if _, err := withAccessPasswordInvocation(scriptSiteInvocation{}, root, id, true); err == nil {
		t.Fatal("missing credential was accepted")
	}
	if err := validateAccessPasswordCreate(&AccessPasswordCreateInput{Scope: "location", Path: "../../unsafe", Username: "alice", Password: "secret"}); err == nil {
		t.Fatal("invalid path was accepted")
	}
}

func TestAccessPasswordCreationRejectsRecipesWithPublicDirectPorts(t *testing.T) {
	for _, recipe := range []string{"bitwarden", "halo"} {
		m := NewManager(t.TempDir(), nil, nil)
		_, err := m.StartRecipe(context.Background(), ScriptSiteInput{
			SiteInput:      SiteInput{Recipe: recipe},
			AccessPassword: &AccessPasswordCreateInput{Scope: "server", Username: "alice", Password: "secret"},
		})
		if !errors.Is(err, ErrUnprocessable) {
			t.Fatalf("recipe %s: %v", recipe, err)
		}
	}
}
