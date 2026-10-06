package cluster

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestOpenLightStorePrunesRetiredWindowsRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), lightStateFileName)
	store, err := openLightStore(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	windowsID := fmt.Sprintf("%032x", 1)
	legacyWindowsID := fmt.Sprintf("%032x", 2)
	linuxID := fmt.Sprintf("%032x", 3)
	ambiguousID := fmt.Sprintf("%032x", 4)
	secrets := make(map[string][]byte)
	terminalKeys := make(map[string][]byte)
	for index, fixture := range []struct {
		id       string
		platform string
		snapshot *HostSnapshot
	}{
		{id: windowsID, platform: "windows"},
		{id: legacyWindowsID, snapshot: &HostSnapshot{Telemetry: contract.HostTelemetry{OS: "Windows Server"}}},
		{id: linuxID, platform: "linux", snapshot: &HostSnapshot{Telemetry: contract.HostTelemetry{OSID: "debian"}}},
		{id: ambiguousID},
	} {
		secret := bytes.Repeat([]byte{byte(index + 1)}, 32)
		terminalKey := bytes.Repeat([]byte{byte(index + 11)}, 32)
		secrets[fixture.id] = secret
		terminalKeys[fixture.id] = terminalKey
		err := store.AddHostWithTerminal(lightHostRecord{
			Platform: fixture.platform, ID: fixture.id, Name: "node-" + fixture.id[:4],
			CreatedAt: now, UpdatedAt: now, LastSnapshot: fixture.snapshot,
		}, secret, terminalKey)
		if err != nil {
			t.Fatalf("seed node %s: %v", fixture.id, err)
		}
		if !store.HasHost(fixture.id) {
			t.Fatalf("host index did not add %s", fixture.id)
		}
	}
	for _, id := range []string{windowsID, legacyWindowsID} {
		for _, name := range []string{"host-" + id + lightCredentialExtension, terminalKeyName(id)} {
			path := filepath.Join(store.secretDir, name)
			if name == terminalKeyName(id) {
				path = filepath.Join(store.terminalDir, name)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("seeded credential %s missing: %v", name, err)
			}
		}
	}

	reopened, err := openLightStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{windowsID, legacyWindowsID} {
		if reopened.HasHost(id) {
			t.Fatalf("retired Windows host %s remains in membership index", id)
		}
	}
	got := reopened.Hosts()
	if len(got) != 2 {
		t.Fatalf("retained hosts = %d, want Linux and ambiguous records only: %#v", len(got), got)
	}
	for _, id := range []string{windowsID, legacyWindowsID} {
		if _, err := reopened.Host(id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("retired Windows host %s lookup error = %v, want not found", id, err)
		}
		for _, entry := range []struct{ dir, name string }{
			{reopened.secretDir, "host-" + id + lightCredentialExtension},
			{reopened.terminalDir, terminalKeyName(id)},
		} {
			if _, err := os.Stat(filepath.Join(entry.dir, entry.name)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("retired Windows key %s remains: %v", entry.name, err)
			}
		}
	}
	for _, id := range []string{linuxID, ambiguousID} {
		if !reopened.HasHost(id) {
			t.Fatalf("preserved host %s is missing from membership index", id)
		}
		host, err := reopened.Host(id)
		if err != nil {
			t.Fatalf("preserved host %s lookup error = %v", id, err)
		}
		secret, err := reopened.ReadSecret(host)
		if err != nil || !bytes.Equal(secret, secrets[id]) {
			t.Fatalf("preserved host %s reporting credential mismatch: %v", id, err)
		}
		terminalKey, err := reopened.ReadTerminalPublicKey(host)
		if err != nil || !bytes.Equal(terminalKey, terminalKeys[id]) {
			t.Fatalf("preserved host %s terminal key mismatch: %v", id, err)
		}
	}
	content, err := os.ReadFile(reopened.path)
	if err != nil {
		t.Fatal(err)
	}
	var persisted lightPersistedState
	if err := json.Unmarshal(content, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted.Hosts) != 2 {
		t.Fatalf("persisted hosts = %d, want 2", len(persisted.Hosts))
	}
	if _, err := openLightStore(path); err != nil {
		t.Fatalf("repeated open after migration: %v", err)
	}
}

func TestLightStoreFailedWindowsPrunePreservesStateAndCredentials(t *testing.T) {
	store, err := openLightStore(filepath.Join(t.TempDir(), lightStateFileName))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	windowsID := fmt.Sprintf("%032x", 1)
	linuxID := fmt.Sprintf("%032x", 2)
	for index, fixture := range []struct {
		id       string
		platform string
	}{
		{id: windowsID, platform: "windows"},
		{id: linuxID, platform: "linux"},
	} {
		if err := store.AddHostWithTerminal(lightHostRecord{
			Platform: fixture.platform, ID: fixture.id, Name: "node-" + fixture.id[:4],
			CreatedAt: now, UpdatedAt: now,
		}, bytes.Repeat([]byte{byte(index + 1)}, 32), bytes.Repeat([]byte{byte(index + 11)}, 32)); err != nil {
			t.Fatal(err)
		}
	}
	writeErr := errors.New("simulated state replacement failure")
	rename := store.ops.rename
	store.ops.rename = func(from, to string) error {
		if to == store.path+".previous" {
			return writeErr
		}
		return rename(from, to)
	}
	if err := store.pruneRetiredWindowsHosts(); !errors.Is(err, writeErr) {
		t.Fatalf("failed migration error = %v, want %v", err, writeErr)
	}
	if len(store.Hosts()) != 2 {
		t.Fatalf("in-memory state after failed migration = %#v, want both records", store.Hosts())
	}
	if _, err := os.Stat(filepath.Join(store.secretDir, "host-"+windowsID+lightCredentialExtension)); err != nil {
		t.Fatalf("Windows credential was removed after failed migration: %v", err)
	}
	if _, err := os.Stat(terminalKeyPath(store.terminalDir, windowsID)); err != nil {
		t.Fatalf("Windows terminal key was removed after failed migration: %v", err)
	}
	if _, err := openLightStore(store.path); err != nil {
		t.Fatalf("retry after failed migration: %v", err)
	}
}

func TestLightStoreWindowsPruneRecoversInterruptedReplacement(t *testing.T) {
	for _, afterInstall := range []bool{false, true} {
		t.Run(fmt.Sprintf("after-install=%t", afterInstall), func(t *testing.T) {
			store, err := openLightStore(filepath.Join(t.TempDir(), lightStateFileName))
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			windowsID := fmt.Sprintf("%032x", 1)
			linuxID := fmt.Sprintf("%032x", 2)
			for index, fixture := range []struct {
				id       string
				platform string
			}{
				{id: windowsID, platform: "windows"},
				{id: linuxID, platform: "linux"},
			} {
				if err := store.AddHostWithTerminal(lightHostRecord{
					Platform: fixture.platform, ID: fixture.id, Name: "node-" + fixture.id[:4],
					CreatedAt: now, UpdatedAt: now,
				}, bytes.Repeat([]byte{byte(index + 1)}, 32), bytes.Repeat([]byte{byte(index + 11)}, 32)); err != nil {
					t.Fatal(err)
				}
			}

			interrupted := errors.New("simulated process interruption")
			rename := store.ops.rename
			store.ops.rename = func(from, to string) error {
				if err := rename(from, to); err != nil {
					return err
				}
				if (!afterInstall && to == store.path+".previous") || (afterInstall && to == store.path) {
					panic(interrupted)
				}
				return nil
			}
			func() {
				defer func() {
					if got := recover(); got != interrupted {
						t.Fatalf("interruption = %v, want injected crash", got)
					}
				}()
				_ = store.pruneRetiredWindowsHosts()
			}()

			for attempt := 0; attempt < 2; attempt++ {
				recovered, err := openLightStore(store.path)
				if err != nil {
					t.Fatal(err)
				}
				if len(recovered.Hosts()) != 1 {
					t.Fatalf("recovered hosts = %#v, want Linux only", recovered.Hosts())
				}
				if _, err := os.Stat(filepath.Join(recovered.secretDir, "host-"+windowsID+lightCredentialExtension)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("retired credential after recovery: %v", err)
				}
				if _, err := os.Stat(terminalKeyPath(recovered.terminalDir, windowsID)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("retired terminal key after recovery: %v", err)
				}
			}
		})
	}
}
