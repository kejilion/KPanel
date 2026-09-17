package cluster

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
