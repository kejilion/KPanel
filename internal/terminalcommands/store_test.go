package terminalcommands

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const testCommandID = "0123456789abcdef0123456789abcdef"

func TestOpenCreatesPrivateStoreAndPersistsReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "terminal-commands")
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	initial := store.Snapshot()
	if !initial.Available || initial.SchemaVersion != SchemaVersion || !ValidResourceVersion(initial.ResourceVersion) || initial.Items == nil {
		t.Fatalf("unexpected initial snapshot: %#v", initial)
	}
	if runtime.GOOS != "windows" {
		assertMode(t, root, 0o700)
		assertMode(t, filepath.Join(root, "commands.json"), 0o600)
	}

	saved, err := store.Replace(validInput(initial.ResourceVersion))
	if err != nil {
		t.Fatal(err)
	}
	if saved.ResourceVersion == initial.ResourceVersion || len(saved.Items) != 1 || saved.Items[0].Command != "docker ps --format '{{.Names}}'" {
		t.Fatalf("replacement was not committed: %#v", saved)
	}

	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	restored := reopened.Snapshot()
	if restored.ResourceVersion != saved.ResourceVersion || len(restored.Items) != 1 || restored.Items[0].Name != "容器列表" {
		t.Fatalf("terminal commands did not survive restart: %#v", restored)
	}
	if _, err := reopened.Replace(validInput(initial.ResourceVersion)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale resourceVersion error = %v, want conflict", err)
	}

	cleared, err := reopened.Replace(ReplaceInput{
		ExpectedResourceVersion: restored.ResourceVersion,
		Items:                   []Command{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.ResourceVersion == restored.ResourceVersion || len(cleared.Items) != 0 {
		t.Fatalf("deletion was not committed: %#v", cleared)
	}
	reopenedAfterDelete, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	deleted := reopenedAfterDelete.Snapshot()
	if deleted.ResourceVersion != cleared.ResourceVersion || deleted.Items == nil || len(deleted.Items) != 0 {
		t.Fatalf("terminal command deletion did not survive restart: %#v", deleted)
	}
}

func TestReplaceValidatesCommandContract(t *testing.T) {
	store := openTestStore(t)
	version := store.Snapshot().ResourceVersion
	tests := []struct {
		name  string
		item  Command
		field string
	}{
		{name: "invalid id", item: Command{ID: "invalid", Name: "Status", Command: "uptime"}, field: "items"},
		{name: "blank name", item: Command{ID: testCommandID, Name: " ", Command: "uptime"}, field: "items.name"},
		{name: "long name", item: Command{ID: testCommandID, Name: strings.Repeat("界", MaxNameRunes+1), Command: "uptime"}, field: "items.name"},
		{name: "blank command", item: Command{ID: testCommandID, Name: "Status", Command: "\n\t"}, field: "items.command"},
		{name: "nul command", item: Command{ID: testCommandID, Name: "Status", Command: "echo\x00value"}, field: "items.command"},
		{name: "long command", item: Command{ID: testCommandID, Name: "Status", Command: strings.Repeat("a", MaxCommandBytes+1)}, field: "items.command"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.Replace(ReplaceInput{ExpectedResourceVersion: version, Items: []Command{test.item}})
			if validationField(err) != test.field {
				t.Fatalf("validation error = %v, want field %s", err, test.field)
			}
		})
	}

	duplicate := Command{ID: testCommandID, Name: "Status", Command: "uptime"}
	if _, err := store.Replace(ReplaceInput{ExpectedResourceVersion: version, Items: []Command{duplicate, duplicate}}); validationField(err) != "items" {
		t.Fatalf("duplicate command error = %v", err)
	}
	tooMany := make([]Command, MaxCommands+1)
	for index := range tooMany {
		tooMany[index] = Command{ID: fmt.Sprintf("%032x", index), Name: "Status", Command: "uptime"}
	}
	if _, err := store.Replace(ReplaceInput{ExpectedResourceVersion: version, Items: tooMany}); validationField(err) != "items" {
		t.Fatalf("command limit error = %v", err)
	}
	if _, err := store.Replace(ReplaceInput{ExpectedResourceVersion: "invalid"}); validationField(err) != "expectedResourceVersion" {
		t.Fatalf("invalid resourceVersion error = %v", err)
	}
}

func TestCommandAllowsMultilineShellTextWithoutTerminalControlSequences(t *testing.T) {
	store := openTestStore(t)
	input := ReplaceInput{
		ExpectedResourceVersion: store.Snapshot().ResourceVersion,
		Items:                   []Command{{ID: testCommandID, Name: "Logs", Command: "journalctl -u nginx\n--no-pager\t-n 50"}},
	}
	if _, err := store.Replace(input); err != nil {
		t.Fatalf("multiline command rejected: %v", err)
	}
	input.ExpectedResourceVersion = store.Snapshot().ResourceVersion
	input.Items[0].Command = "echo ready\x1b[2J"
	if _, err := store.Replace(input); validationField(err) != "items.command" {
		t.Fatalf("terminal control sequence error = %v", err)
	}
}

func TestCorruptStoreDegradesWithoutOverwritingSource(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "malformed", data: []byte("{not-json")},
		{name: "unknown schema", data: []byte(`{"schemaVersion":99,"items":[]}`)},
		{name: "unknown field", data: []byte(`{"schemaVersion":1,"items":[],"extra":true}`)},
		{name: "oversized", data: bytes.Repeat([]byte("x"), maxPersistedBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "terminal-commands")
			if _, err := Open(root); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "commands.json")
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			store, err := Open(root)
			if err != nil {
				t.Fatalf("corrupt command store must not fail panel initialization: %v", err)
			}
			snapshot := store.Snapshot()
			if snapshot.Available || snapshot.Warning != unavailableWarning || len(snapshot.Items) != 0 {
				t.Fatalf("unexpected degraded snapshot: %#v", snapshot)
			}
			before, _ := os.ReadFile(path)
			if _, err := store.Replace(ReplaceInput{ExpectedResourceVersion: snapshot.ResourceVersion}); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("corrupt store write error = %v, want unavailable", err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("unavailable store overwrote corrupt commands")
			}
		})
	}
}

func TestPersistenceFailureLeavesSnapshotUnchanged(t *testing.T) {
	store := openTestStore(t)
	initial := store.Snapshot()
	store.writeAtomic = func(string, string, []byte) error { return errors.New("injected write failure") }
	if _, err := store.Replace(validInput(initial.ResourceVersion)); err == nil {
		t.Fatal("expected injected persistence failure")
	}
	current := store.Snapshot()
	if current.ResourceVersion != initial.ResourceVersion || len(current.Items) != 0 {
		t.Fatalf("failed write changed memory state: %#v", current)
	}
}

func TestConcurrentReplacementWithSameVersionHasOneWinner(t *testing.T) {
	store := openTestStore(t)
	version := store.Snapshot().ResourceVersion
	inputs := []ReplaceInput{
		{ExpectedResourceVersion: version, Items: []Command{{ID: testCommandID, Name: "Status", Command: "uptime"}}},
		{ExpectedResourceVersion: version, Items: []Command{{ID: "ffffffffffffffffffffffffffffffff", Name: "Disk", Command: "df -h"}}},
	}
	var wait sync.WaitGroup
	errorsByInput := make([]error, len(inputs))
	for index := range inputs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, errorsByInput[index] = store.Replace(inputs[index])
		}(index)
	}
	wait.Wait()
	successes, conflicts := 0, 0
	for _, err := range errorsByInput {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "terminal-commands"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func validInput(version string) ReplaceInput {
	return ReplaceInput{
		ExpectedResourceVersion: version,
		Items:                   []Command{{ID: testCommandID, Name: "容器列表", Command: "docker ps --format '{{.Names}}'"}},
	}
}

func validationField(err error) string {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return validation.Field
	}
	return ""
}

func assertMode(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("mode %o for %s, want %o", info.Mode().Perm(), path, mode)
	}
}
