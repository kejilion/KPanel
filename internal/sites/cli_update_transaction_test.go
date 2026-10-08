//go:build linux

package sites

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type cliTransactionNginx struct {
	*fakeNginxController
	onTest   func()
	onReload func()
}

func (n *cliTransactionNginx) NginxTest(ctx context.Context) error {
	if n.onTest != nil && n.tests > 0 {
		fn := n.onTest
		n.onTest = nil
		fn()
	}
	return n.fakeNginxController.NginxTest(ctx)
}

func (n *cliTransactionNginx) NginxReload(ctx context.Context) error {
	if n.onReload != nil {
		n.onReload()
	}
	return n.fakeNginxController.NginxReload(ctx)
}

func TestCLIUpdateTransaction(t *testing.T) {
	for _, scenario := range []string{"success", "test_failure", "reload_failure", "external_before_exchange", "external_before_test", "external_test", "external_failed_test", "external_reload", "external_failed_reload", "rollback_exchange_failure"} {
		t.Run(scenario, func(t *testing.T) {
			manager, fake, root := newTestManager(t)
			nginx := &cliTransactionNginx{fakeNginxController: fake}
			manager.nginx = nginx
			configPath := filepath.Join(root, "conf.d", "cli.example.com.conf")
			old := []byte("server {\n    listen 80;\n    server_name cli.example.com;\n    location /secret/ { deny all; }\n    location / {\n        proxy_pass http://127.0.0.1:3000;\n    }\n}\n")
			external := bytes.ReplaceAll(old, []byte("3000"), []byte("9000"))
			if err := os.WriteFile(configPath, old, 0o640); err != nil {
				t.Fatal(err)
			}
			items, err := manager.discoverer.Discover()
			if err != nil || len(items) != 1 || items[0].Origin != contract.OriginCLI {
				t.Fatalf("CLI fixture discovery: %+v, %v", items, err)
			}
			writeExternal := func() {
				if err := os.WriteFile(configPath, external, 0o640); err != nil {
					t.Fatal(err)
				}
			}
			wantErr := error(nil)
			want := old
			retain := false
			switch scenario {
			case "success":
				want = bytes.ReplaceAll(old, []byte("3000"), []byte("8080"))
			case "test_failure":
				fake.testErrs = []error{nil, errors.New("invalid candidate")}
				wantErr = ErrUnprocessable
			case "reload_failure":
				fake.reloadErrs = []error{errors.New("reload failed")}
				wantErr = ErrUnavailable
			case "external_before_exchange":
				manager.testHook = func(stage, _ string) {
					if stage == "before_exchange" {
						writeExternal()
					}
				}
				want, wantErr = external, ErrConflict
			case "external_before_test":
				manager.testHook = func(stage, _ string) {
					if stage == "candidate_published" {
						writeExternal()
					}
				}
				want, wantErr, retain = external, ErrNeedsAttention, true
			case "external_test", "external_failed_test":
				nginx.onTest = writeExternal
				if scenario == "external_failed_test" {
					fake.testErrs = []error{nil, errors.New("invalid external config")}
				}
				want, wantErr, retain = external, ErrNeedsAttention, true
			case "external_reload", "external_failed_reload":
				nginx.onReload = writeExternal
				if scenario == "external_failed_reload" {
					fake.reloadErrs = []error{errors.New("reload failed")}
				}
				want, wantErr, retain = external, ErrNeedsAttention, true
			case "rollback_exchange_failure":
				nginx.onTest = func() {
					// Simulate an unavailable exchange operand while retaining
					// the displaced file for recovery.
					paths, _ := filepath.Glob(filepath.Join(root, "conf.d", ".cli.example.com.cli-*.tmp"))
					if len(paths) != 1 {
						t.Fatalf("backup paths: %v", paths)
					}
					if err := os.Rename(paths[0], paths[0]+".retained"); err != nil {
						t.Fatal(err)
					}
				}
				fake.testErrs = []error{nil, errors.New("invalid candidate")}
				want = bytes.ReplaceAll(old, []byte("3000"), []byte("8080"))
				wantErr, retain = ErrNeedsAttention, true
			}
			_, err = manager.Update(context.Background(), items[0].ID, SiteInput{
				PrimaryDomain: "cli.example.com", Type: "proxy",
				Upstream: "http://127.0.0.1:8080", ExpectedResourceVersion: items[0].ResourceVersion,
			})
			if (wantErr == nil && err != nil) || (wantErr != nil && !errors.Is(err, wantErr)) {
				t.Fatalf("update error=%v want=%v", err, wantErr)
			}
			got, readErr := os.ReadFile(configPath)
			if readErr != nil || !bytes.Equal(got, want) {
				t.Fatalf("live config=%q err=%v want=%q", got, readErr, want)
			}
			backups, _ := filepath.Glob(filepath.Join(root, "conf.d", ".cli.example.com.cli-*.tmp*"))
			if !retain && len(backups) != 0 {
				t.Fatalf("unexpected backup leak: %v", backups)
			}
			if retain {
				found := false
				for _, backup := range backups {
					body, _ := os.ReadFile(backup)
					found = found || bytes.Equal(body, old)
				}
				if !found {
					t.Fatalf("previous configuration lost: %v", backups)
				}
				if !strings.Contains(err.Error(), "retained") && !strings.Contains(err.Error(), "restore previous") {
					t.Fatalf("missing recovery path: %v", err)
				}
			}
			if scenario == "external_test" && fake.reloads != 0 {
				t.Fatal("reloaded an externally changed configuration")
			}
			if scenario == "external_before_test" && fake.tests != 1 {
				t.Fatal("validated an externally changed configuration")
			}
		})
	}
}
