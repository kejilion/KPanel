package mcpaccess

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPolicyStoreRejectsOversizeWithoutBreakingRestart(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	_, _ = s.SetEnabled(true, s.Snapshot().ResourceVersion)
	p := &Policy{Domains: []string{"files"}, Operations: []string{"host_file_read"}, OperationVersions: map[string]string{"host_file_read": strings.Repeat("a", 64)}}
	for i := 0; i < 16; i++ {
		p.FileRoots = append(p.FileRoots, "/"+strings.Repeat(string(rune('a'+i)), 4094))
	}
	limited := false
	for i := 0; i < MaxClients; i++ {
		_, _, err := s.CreateWithPolicy("Large grant", []HostGrant{{ID: "local", Identity: strings.Repeat("a", 64)}}, p, time.Hour, s.Snapshot().ResourceVersion)
		if errors.Is(err, ErrLimited) {
			limited = true
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !limited || !s.Snapshot().Available || !Open(dir).Snapshot().Available {
		t.Fatal("oversize policy broke live or reopened access")
	}
}

func TestOperationArgumentsAreEncryptedAndTamperFailsClosed(t *testing.T) {
	dir := t.TempDir()
	s := OpenOperations(dir)
	_, err := s.Plan(strings.Repeat("a", 32), "secret-operation", "local", strings.Repeat("b", 64), "host_backup_export", json.RawMessage(`{"password":"must-not-be-plaintext"}`), false)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil || strings.Contains(string(data), "must-not-be-plaintext") {
		t.Fatal("plaintext operation arguments")
	}
	var envelope map[string]any
	_ = json.Unmarshal(data, &envelope)
	envelope["ciphertext"] = "dGFtcGVyZWQ="
	data, _ = json.Marshal(envelope)
	if err := os.WriteFile(s.path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenOperations(dir).List(""); !errors.Is(err, ErrUnavailable) {
		t.Fatal("tampered operations accepted")
	}
}

func TestPlanReplayApprovalClaimAndCrash(t *testing.T) {
	dir := t.TempDir()
	s := OpenOperations(dir)
	client, identity := strings.Repeat("a", 32), strings.Repeat("b", 64)
	o, err := s.Plan(client, "request-1", "local", identity, "host_app_action", json.RawMessage(`{"action":"restart","appId":"nginx"}`), false)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Plan(client, "request-1", "local", identity, "host_app_action", json.RawMessage(`{ "appId":"nginx", "action":"restart" }`), false)
	if err != nil || again.ID != o.ID {
		t.Fatalf("retry did not return original plan: %v", err)
	}
	if _, err := s.Plan(client, "request-1", "local", identity, "host_app_action", json.RawMessage(`{"action":"uninstall","appId":"nginx"}`), false); !errors.Is(err, ErrConflict) {
		t.Fatalf("key reuse: %v", err)
	}
	if _, err := s.Claim(o.ID, client, o.Digest, identity); !errors.Is(err, ErrConflict) {
		t.Fatal("unapproved claim accepted")
	}
	if _, err := s.Decide(o.ID, o.Digest, "admin", true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Claim(o.ID, client, o.Digest, strings.Repeat("c", 64)); !errors.Is(err, ErrConflict) {
		t.Fatal("rebound host accepted")
	}
	var winners atomic.Int32
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			if _, err := s.Claim(o.ID, client, o.Digest, identity); err == nil {
				winners.Add(1)
			}
		})
	}
	group.Wait()
	if winners.Load() != 1 {
		t.Fatalf("dispatches=%d", winners.Load())
	}
	loaded := OpenOperations(dir)
	result, err := loaded.Get(o.ID)
	if err != nil || result.State != "unknown" {
		t.Fatalf("crash must not imply safe retry: %#v %v", result, err)
	}
	if _, err := loaded.Claim(o.ID, client, o.Digest, identity); !errors.Is(err, ErrConflict) {
		t.Fatal("replayed interrupted operation")
	}
}

func TestOperationExpiresAndFailsClosedOnPersistence(t *testing.T) {
	s := OpenOperations(t.TempDir())
	now := time.Now()
	s.now = func() time.Time { return now }
	o, err := s.Plan(strings.Repeat("a", 32), "request-1", "local", strings.Repeat("b", 64), "host_app_action", json.RawMessage(`{}`), false)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(16 * time.Minute)
	if _, err = s.Decide(o.ID, o.Digest, "admin", true); !errors.Is(err, ErrConflict) {
		t.Fatal("expired approval accepted")
	}
	s.write = func(string, any) error { return errors.New("disk full") }
	if _, err = s.Plan(strings.Repeat("a", 32), "request-2", "local", strings.Repeat("b", 64), "host_app_action", json.RawMessage(`{}`), true); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("write failure: %v", err)
	}
	if _, err = s.Get(o.ID); !errors.Is(err, ErrUnavailable) {
		t.Fatal("continued from uncertain disk state")
	}
}

func TestLegacyAndExplicitPolicy(t *testing.T) {
	var old *Policy
	if !old.Valid() || old.Allows("docker", true) || old.AllowsOperation("host_docker_summary", "docker", false) {
		t.Fatal("legacy permission expanded")
	}
	p := &Policy{Domains: []string{"files"}, Operations: []string{"host_file_read"}, OperationVersions: map[string]string{"host_file_read": strings.Repeat("a", 64)}, FileRoots: []string{"/home/web"}}
	if !p.Valid() || !p.AllowsPath("/home/web/site/config") || p.AllowsPath("/home/web-other/password") || p.AllowsPath("/home/web/../../etc/shadow") || p.AllowsOperation("future_file_tool", "files", false) {
		t.Fatal("policy boundary")
	}
	p.FileRoots = []string{"/"}
	if p.Valid() {
		t.Fatal("whole host root grant accepted")
	}
}
