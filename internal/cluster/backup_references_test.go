//go:build !windows

package cluster

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupMissingClusterReferences(t *testing.T) {
	for _, kind := range []string{"legacy", "v2", "light"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			cfg := ServiceConfig{DataDir: dir, Telemetry: serviceTestTelemetry{now: time.Now, hostname: "offline-fixture"}}
			s, err := NewService(cfg)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			switch kind {
			case "legacy":
				err = s.store.AddHost(hostRecord{ID: strings.Repeat("a", 32), Name: "fixture", Origin: "https://fixture.invalid", RemoteNodeID: strings.Repeat("b", 32), ControllerID: strings.Repeat("c", 32), CredentialFile: strings.Repeat("a", 32) + ".ed25519", FederationProtocol: FederationProtocol, CreatedAt: now, UpdatedAt: now})
			case "v2":
				err = s.storeV2.AddHost(testHostRecordV2(1, now))
			case "light":
				id := strings.Repeat("d", 32)
				err = s.light.AddHost(lightHostRecord{ID: id, Name: "fixture", CreatedAt: now, UpdatedAt: now}, bytes.Repeat([]byte{7}, 32))
				if err == nil {
					err = os.Remove(filepath.Join(dir, lightSecretsDirectory, "host-"+id+lightCredentialExtension))
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := NewService(cfg)
			if err != nil {
				t.Fatalf("fixture no longer demonstrates permissive constructor: %v", err)
			}
			defer reopened.Close()
			if err := reopened.ValidateBackupReferences(); err == nil {
				t.Fatal("missing referenced credential accepted")
			} else {
				t.Logf("constructor succeeds; explicit backup validator rejects %s: %v", kind, err)
			}
		})
	}
}
func TestBackupBackupTargetAndLightKeyValidation(t *testing.T) {
	for _, kind := range []string{"v2-target-mismatch", "legacy-light-without-terminal", "malformed-light-terminal"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			s, err := NewService(ServiceConfig{DataDir: dir, Telemetry: serviceTestTelemetry{now: time.Now, hostname: "offline-fixture"}})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			now := time.Now().UTC()
			if kind == "v2-target-mismatch" {
				record := testHostRecordV2(4, now)
				if err := s.storeV2.AddHost(record); err != nil {
					t.Fatal(err)
				}
				if _, err := s.secretsV2.WriteHostCredential(record.ID, testHostCredentialV2(t, 2)); err != nil {
					t.Fatal(err)
				}
			} else {
				id := strings.Repeat("e", 32)
				if err := s.light.AddHost(lightHostRecord{ID: id, Name: "fixture", CreatedAt: now, UpdatedAt: now}, bytes.Repeat([]byte{7}, 32)); err != nil {
					t.Fatal(err)
				}
				if kind == "malformed-light-terminal" {
					if err := os.WriteFile(filepath.Join(dir, lightTerminalKeysDirectory, terminalKeyName(id)), []byte("bad key"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			err = s.ValidateBackupReferences()
			if kind == "legacy-light-without-terminal" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid/mismatched key accepted")
			}
		})
	}
}
func TestBackupPrunePendingAndOrphanKeys(t *testing.T) {
	dir := t.TempDir()
	s, err := NewService(ServiceConfig{DataDir: dir, Telemetry: serviceTestTelemetry{now: time.Now, hostname: "offline-fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	files := map[string][]byte{}
	for _, name := range []string{"cluster-state.json", clusterStateV2FileName, lightStateFileName, clusterSecretsV2DirectoryName + "/" + nodeIdentityV2FileName} {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		files[name] = b
	}
	pair := clusterSecretsV2DirectoryName + "/pair-0123456789abcdef.v2key"
	orphan := clusterSecretsV2DirectoryName + "/host-" + strings.Repeat("a", 32) + ".v2key"
	unknown := "cluster-secrets-v2/unrelated-backup-copy"
	files[pair] = []byte("pending-secret")
	files[orphan] = []byte("orphan-secret")
	files[unknown] = []byte("unexpected-secret")
	if err := PruneBackupSecrets(files); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{pair, orphan, unknown} {
		if _, ok := files[name]; ok {
			t.Fatal("unreferenced secret retained:", name)
		}
	}
	if len(files[clusterSecretsV2DirectoryName+"/"+nodeIdentityV2FileName]) == 0 {
		t.Fatal("node identity lost")
	}
	var state persistedStateV2
	if err := json.Unmarshal(files[clusterStateV2FileName], &state); err != nil {
		t.Fatal(err)
	}
	state.Hosts = []hostRecordV2{testHostRecordV2(8, time.Now().UTC())}
	files[clusterStateV2FileName], _ = json.Marshal(state)
	if err := PruneBackupSecrets(files); err == nil {
		t.Fatal("missing reference silently pruned")
	}
}
