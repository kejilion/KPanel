package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/hostbackup"
)

func TestBackupCLIFileInteroperability(t *testing.T) {
	t.Setenv("KEJILION_AGENT_STATE_DIR", t.TempDir())
	dir := t.TempDir()
	password := "fixture-password-123"
	id := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payloads := map[string][]byte{"apps": []byte("application payload"), "web": []byte("website payload"), "docker": []byte("docker payload")}
	var calls []string
	var selected []string
	call := func(args []string, input io.Reader, output io.Writer) error {
		calls = append(calls, args[0])
		switch args[0] {
		case "inventory":
			return json.NewEncoder(output).Encode(hostbackup.Preview{Revision: "fixture"})
		case "export", "import":
			var request hostbackup.Request
			if err := json.NewDecoder(input).Decode(&request); err != nil {
				return err
			}
			selected = request.Modules
			return json.NewEncoder(output).Encode(backup.Record{ID: id, Status: "queued", Modules: selected})
		case "status", "inspect":
			return json.NewEncoder(output).Encode(backup.Record{ID: id, Status: "ready", Modules: selected})
		case "download":
			_, err := output.Write(payloads[args[2]])
			return err
		case "upload":
			body, err := io.ReadAll(input)
			if !bytes.Equal(body, payloads[args[2]]) {
				t.Fatal("payload changed during script import")
			}
			return err
		case "delete", "abort":
			return nil
		default:
			t.Fatalf("unexpected command %v", args)
			return nil
		}
	}
	path := filepath.Join(dir, "script-export.kpb")
	if err := exportBackupFile(call, backupFileRequest{Path: path, Password: password, Modules: []string{"apps", "web", "docker"}}); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	unpacked := t.TempDir()
	manifest, err := backup.Read(in, password, unpacked)
	in.Close()
	if err != nil || len(manifest.Parts) != 3 {
		t.Fatal(manifest, err)
	}
	for module, expected := range payloads {
		actual, err := os.ReadFile(filepath.Join(unpacked, module+".payload"))
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatal(module, err)
		}
	}
	count := len(calls)
	if err := exportBackupFile(call, backupFileRequest{Path: path, Password: password, Modules: []string{"web"}}); err == nil || len(calls) != count {
		t.Fatal("existing file must not be overwritten or start a host job")
	}

	// Panel uses the same writer and can include panel identity alongside host data.
	panelPath := filepath.Join(unpacked, "panel.payload")
	if err := os.WriteFile(panelPath, []byte("panel identity stays in KPanel"), 0600); err != nil {
		t.Fatal(err)
	}
	mixed := filepath.Join(dir, "panel-export.kpb")
	out, err := os.Create(mixed)
	if err != nil {
		t.Fatal(err)
	}
	_, err = backup.Write(out, password, "fixture", []backup.Source{{Module: "panel", Path: panelPath}, {Module: "web", Path: filepath.Join(unpacked, "web.payload")}, {Module: "docker", Path: filepath.Join(unpacked, "docker.payload")}})
	if err := errors.Join(err, out.Close()); err != nil {
		t.Fatal(err)
	}
	calls = nil
	if _, err := inspectBackupFile(call, backupFileRequest{Path: mixed, Password: "incorrect-password"}); err == nil || len(calls) != 0 {
		t.Fatal("wrong password reached Agent")
	}
	record, err := inspectBackupFile(call, backupFileRequest{Path: mixed, Password: password, Modules: []string{"web"}})
	if err != nil || record.Status != "ready" || !slices.Equal(record.Modules, []string{"web"}) {
		t.Fatal(record, err)
	}
	if slices.Contains(calls, "restore") || slices.Contains(calls, "export") {
		t.Fatal("import must inspect only")
	}
	entries, err := os.ReadDir(filepath.Join(os.Getenv("KEJILION_AGENT_STATE_DIR"), "backup-cli"))
	if err != nil || len(entries) != 0 {
		t.Fatal("temporary decrypted data retained", err)
	}
}
