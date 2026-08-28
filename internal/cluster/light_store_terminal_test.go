package cluster

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLightStoreKeepsTerminalIdentityOutsideLegacyState(t *testing.T) {
	dataDir := t.TempDir()
	statePath := filepath.Join(dataDir, lightStateFileName)
	store, err := openLightStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	enrollmentID := strings.Repeat("a", 32)
	secret := bytes.Repeat([]byte{1}, 32)
	hash := sha256.Sum256(secret)
	if err := store.AddEnrollment(lightEnrollmentRecord{
		ID: enrollmentID, SecretHash: hex.EncodeToString(hash[:]), ExpiresAt: now.Add(time.Minute),
	}, now); err != nil {
		t.Fatal(err)
	}
	key, err := GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	hostID := strings.Repeat("b", 32)
	if err := store.EnrollHost(
		enrollmentID,
		hex.EncodeToString(hash[:]),
		lightHostRecord{ID: hostID, Name: "edge-1", NodeVersion: "0.40.0", CreatedAt: now, UpdatedAt: now},
		bytes.Repeat([]byte{2}, 32), key.Public, now,
	); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(content, []byte("terminalPublicKey")) {
		t.Fatal("legacy light state persisted the terminal public key")
	}
	record, err := store.Host(hostID)
	if err != nil {
		t.Fatal(err)
	}
	loadedKey, err := store.ReadTerminalPublicKey(record)
	if err != nil || !bytes.Equal(loadedKey, key.Public) {
		t.Fatalf("ReadTerminalPublicKey() = %x, %v", loadedKey, err)
	}

	reopened, err := openLightStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	reopenedRecord, err := reopened.Host(hostID)
	if err != nil {
		t.Fatal(err)
	}
	loadedKey, err = reopened.ReadTerminalPublicKey(reopenedRecord)
	if err != nil || !bytes.Equal(loadedKey, key.Public) {
		t.Fatalf("reopened ReadTerminalPublicKey() = %x, %v", loadedKey, err)
	}
	if _, removed, err := reopened.DeleteHost(hostID, reopenedRecord.ResourceVersion); err != nil || !removed {
		t.Fatalf("DeleteHost() = removed:%v, error:%v", removed, err)
	}
	if _, err := os.Stat(terminalKeyPath(filepath.Join(dataDir, lightTerminalKeysDirectory), hostID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("terminal key file remains after deletion: %v", err)
	}
}

func TestDecodeLightStatePreservesLegacySchemaBoundary(t *testing.T) {
	var state lightPersistedState
	if err := decodeLightState([]byte(`{"schemaVersion":1,"enrollments":[],"hosts":[]}`), &state); err != nil {
		t.Fatalf("legacy light state was rejected: %v", err)
	}
	if err := decodeLightState([]byte(`{"schemaVersion":1,"enrollments":[],"hosts":[],"terminalPublicKey":"unexpected"}`), &state); err == nil {
		t.Fatal("light state accepted a new terminal identity field")
	}
}
