package cluster

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupPrunesTemporarySecretsAndRejectsMissingReferences(t *testing.T) {
	id := strings.Repeat("a", 32)
	credential := id + ".ed25519"
	legacy, _ := json.Marshal(persistedState{Hosts: []hostRecord{{ID: id, CredentialFile: credential}}})
	files := map[string][]byte{"cluster-state.json": legacy, clusterStateV2FileName: []byte(`{"hosts":[]}`), lightStateFileName: []byte(`{"hosts":[]}`), "cluster-secrets/" + credential: []byte("credential"), clusterSecretsV2DirectoryName + "/" + nodeIdentityV2FileName: []byte("identity"), "cluster-secrets-v2/pair-aaaaaaaaaaaaaaaa.v2key": []byte("temporary"), "cluster-light-secrets/orphan": []byte("orphan")}
	if err := PruneBackupSecrets(files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 5 {
		t.Fatal("temporary secrets retained", len(files))
	}
	delete(files, "cluster-secrets/"+credential)
	if err := PruneBackupSecrets(files); err == nil {
		t.Fatal("missing pairing credential accepted")
	}
}

func TestBackupReferenceValidationReadsActualCredential(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("credential permission contract requires POSIX file modes")
	}
	s, err := NewService(ServiceConfig{DataDir: t.TempDir(), Telemetry: serviceTestTelemetry{now: time.Now}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := strings.Repeat("a", 32)
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	name, err := s.secrets.Write(id, private)
	if err != nil {
		t.Fatal(err)
	}
	s.store.state.Hosts = []hostRecord{{ID: id, CredentialFile: name}}
	if err := s.ValidateBackupReferences(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.secrets.directory, name), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateBackupReferences(); err == nil {
		t.Fatal("malformed credential accepted")
	}
	if err := os.Remove(filepath.Join(s.secrets.directory, name)); err != nil {
		t.Fatal(err)
	}
	if err := s.ValidateBackupReferences(); err == nil {
		t.Fatal("missing credential accepted")
	}
}
