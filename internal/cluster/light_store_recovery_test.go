package cluster

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLightStoreRecoversInterruptedReplacement(t *testing.T) {
	for _, afterInstall := range []bool{false, true} {
		t.Run(fmt.Sprintf("after-install=%t", afterInstall), func(t *testing.T) {
			store, secrets, keys := lightRecoveryFixture(t)
			before := store.Hosts()[0]
			crash := errors.New("simulated process interruption")
			rename := store.ops.rename
			store.ops.rename = func(from, to string) error {
				if err := rename(from, to); err != nil {
					return err
				}
				if (!afterInstall && to == store.path+".previous") || (afterInstall && to == store.path) {
					panic(crash)
				}
				return nil
			}
			func() {
				defer func() {
					if got := recover(); got != crash {
						t.Fatalf("interruption = %v, want injected crash", got)
					}
				}()
				_, _ = store.RenameHost(before.ID, "renamed", before.ResourceVersion, time.Now())
			}()
			wantName := before.Name
			if afterInstall {
				wantName = "renamed"
			}
			// Repeated opens must preserve both kinds of credentials and keep the
			// committed target authoritative once it has been installed.
			for attempt := 0; attempt < 2; attempt++ {
				recovered, err := openLightStore(store.path)
				if err != nil {
					t.Fatal(err)
				}
				assertLightRecoveryCredentials(t, recovered, secrets, keys)
				host, err := recovered.Host(before.ID)
				if err != nil || host.Name != wantName {
					t.Fatalf("recovered host name = %q, error = %v; want %q", host.Name, err, wantName)
				}
				if _, err := os.Lstat(store.path + ".previous"); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("recovered backup residue: %v", err)
				}
				if _, err := recovered.RenameHost(host.ID, wantName, host.ResourceVersion, time.Now()); err != nil {
					t.Fatalf("write after recovery: %v", err)
				}
			}
		})
	}
}

func TestLightStoreFailedRecoveryPreservesEvidence(t *testing.T) {
	for _, scenario := range []string{"missing-state", "invalid-backup", "invalid-schema", "oversized-backup", "backup-directory", "corrupt-target", "backup-symlink"} {
		t.Run(scenario, func(t *testing.T) {
			store, secrets, keys := lightRecoveryFixture(t)
			original, err := os.ReadFile(store.path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(store.path); err != nil {
				t.Fatal(err)
			}
			backup := store.path + ".previous"
			var backupContent []byte
			switch scenario {
			case "invalid-backup":
				backupContent = []byte("{broken")
			case "invalid-schema":
				backupContent = []byte(`{"schemaVersion":99,"hosts":[],"enrollments":[]}`)
			case "oversized-backup":
				backupContent = bytes.Repeat([]byte(" "), int(maxLightStateBytes)+1)
			case "backup-directory":
				if err := os.Mkdir(backup, 0o700); err != nil {
					t.Fatal(err)
				}
			case "corrupt-target":
				backupContent = original
				if err := os.WriteFile(store.path, []byte("{broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "backup-symlink":
				external := filepath.Join(t.TempDir(), "state.json")
				if err := os.WriteFile(external, original, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, backup); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			if backupContent != nil {
				if err := os.WriteFile(backup, backupContent, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for attempt := 0; attempt < 2; attempt++ {
				if _, err := openLightStore(store.path); err == nil {
					t.Fatal("unsafe recovery succeeded")
				}
				assertLightRecoveryCredentials(t, store, secrets, keys)
				if scenario == "corrupt-target" {
					if got, err := os.ReadFile(store.path); err != nil || string(got) != "{broken" {
						t.Fatalf("corrupt target was changed: %v", err)
					}
				} else if _, err := os.Lstat(store.path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("missing target was initialized during failed recovery: %v", err)
				}
				if backupContent != nil {
					if got, err := os.ReadFile(backup); err != nil || !bytes.Equal(got, backupContent) {
						t.Fatalf("backup evidence changed: %v", err)
					}
				}
			}
		})
	}
}

func TestLightStoreFirstInstallRequiresEmptyCredentialDirectories(t *testing.T) {
	for _, folder := range []string{"", lightSecretsDirectory, lightTerminalKeysDirectory} {
		t.Run("residue="+folder, func(t *testing.T) {
			dir := t.TempDir()
			if folder != "" {
				if err := os.Mkdir(filepath.Join(dir, folder), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, folder, "residue"), []byte("preserve"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(dir, lightStateFileName)
			_, err := openLightStore(path)
			if folder == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("initialized empty state over existing credentials")
			} else if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("created empty state: %v", statErr)
			}
		})
	}
}

func lightRecoveryFixture(t *testing.T) (*lightStore, map[string][]byte, map[string][]byte) {
	t.Helper()
	store, err := openLightStore(filepath.Join(t.TempDir(), lightStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	secrets, keys := map[string][]byte{}, map[string][]byte{}
	for i := 1; i <= 9; i++ {
		id := fmt.Sprintf("%032x", i)
		secrets[id] = bytes.Repeat([]byte{byte(i)}, 32)
		keys[id] = bytes.Repeat([]byte{byte(i + 10)}, 32)
		if err := store.AddHostWithTerminal(lightHostRecord{
			ID: id, Name: fmt.Sprintf("edge-%d", i), CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}, secrets[id], keys[id]); err != nil {
			t.Fatal(err)
		}
	}
	return store, secrets, keys
}

func assertLightRecoveryCredentials(t *testing.T, store *lightStore, secrets, keys map[string][]byte) {
	t.Helper()
	if got := len(store.Hosts()); got != len(secrets) {
		t.Fatalf("nodes after restart = %d, want %d", got, len(secrets))
	}
	for _, host := range store.Hosts() {
		if got, err := store.ReadSecret(host); err != nil || !bytes.Equal(got, secrets[host.ID]) {
			t.Fatalf("reporting credential changed for %s: %v", host.ID, err)
		}
		if got, err := store.ReadTerminalPublicKey(host); err != nil || !bytes.Equal(got, keys[host.ID]) {
			t.Fatalf("terminal identity changed for %s: %v", host.ID, err)
		}
	}
}

func TestLightServiceAcceptsOriginalCredentialAfterRecovery(t *testing.T) {
	clock := &serviceTestClock{now: time.Date(2026, 9, 25, 10, 31, 0, 0, time.UTC)}
	service := newLightServiceForTest(t, clock)
	enrolled := enrollLightHostForTest(t, service, "edge")
	path := service.light.path
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".previous"); err != nil {
		t.Fatal(err)
	}
	recovered, err := NewService(ServiceConfig{
		DataDir: filepath.Dir(path), PanelVersion: "1.22.0", PublicURL: "https://panel.example",
		Hostname: "center", Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"}, Now: clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	input := LightReportRequest{Telemetry: serviceTelemetry(clock.Now(), "edge")}
	body, auth := signedLightReportForTest(t, clock.Now(), enrolled, input, strings.Repeat("a", 32))
	if _, err := recovered.AcceptLightReport(auth, body, input); err != nil {
		t.Fatalf("original node cannot report after recovery: %v", err)
	}
}

func TestLightStoreRecoveryDoesNotResurrectDeletedNode(t *testing.T) {
	store, _, _ := lightRecoveryFixture(t)
	host := store.Hosts()[0]
	old, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.DeleteHost(host.ID, host.ResourceVersion); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.path+".previous", old, 0o600); err != nil {
		t.Fatal(err)
	}
	recovered, err := openLightStore(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recovered.Host(host.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted node restored from stale backup: %v", err)
	}
	if got := len(recovered.Hosts()); got != 8 {
		t.Fatalf("remaining nodes = %d, want 8", got)
	}
}

// A crash between installing the light state target and removing its .previous
// backup leaves a residue that used to deadlock every later persist with
// "cluster v2 atomic backup already exists". Opening the store after a restart
// must discard the stale backup once the target decodes, and state writes must
// flow again.
func TestLightStoreRecoversFromStaleBackupResidue(t *testing.T) {
	clock := &serviceTestClock{now: time.Date(2026, 9, 17, 15, 49, 0, 0, time.UTC)}
	service := newLightServiceForTest(t, clock)
	first := enrollLightHostForTest(t, service, "edge-1")
	statePath := service.light.path
	target, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Dir(statePath)
	_ = service.Close()

	if err := os.WriteFile(statePath+".previous", target, 0o600); err != nil {
		t.Fatal(err)
	}

	recovered, err := NewService(ServiceConfig{
		DataDir: dataDir, PanelVersion: "1.19.0", PublicURL: "https://panel.example",
		Hostname:  "center",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center"}, Now: clock.Now,
	})
	if err != nil {
		t.Fatalf("reopen with stale backup residue: %v", err)
	}
	defer func() { _ = recovered.Close() }()

	if _, err := os.Lstat(statePath + ".previous"); !os.IsNotExist(err) {
		t.Fatalf("stale backup survived a validated reload: %v", err)
	}

	// The same operation that returned HTTP 500 in production must succeed now.
	second := enrollLightHostForTest(t, recovered, "edge-2")
	if second.NodeID == first.NodeID {
		t.Fatal("second enrollment reused the first node ID")
	}
	if _, err := recovered.Host(context.Background(), first.NodeID); err != nil {
		t.Fatalf("pre-existing host lost after recovery: %v", err)
	}
}
