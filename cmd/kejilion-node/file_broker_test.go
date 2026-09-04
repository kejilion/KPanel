package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func TestEnsureFileCapabilityUpgradesAnExistingNodeWithoutEnrollment(t *testing.T) {
	secret := bytes.Repeat([]byte{7}, 32)
	targetID := strings.Repeat("b", 32)
	peer, err := cluster.GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != cluster.LightFileCapabilityPath || r.URL.RawQuery != "" {
			t.Fatalf("capability request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Fatalf("read capability body: %v", readErr)
		}
		var input cluster.LightFileCapabilityRequest
		if err := json.Unmarshal(body, &input); err != nil || input.TerminalPublicKey == "" {
			t.Fatalf("capability body = %q, error = %v", body, err)
		}
		expected := cluster.LightRequestSignature(
			secret, r.Method, r.URL.Path, r.Header.Get("X-KPanel-Light-Node-ID"),
			r.Header.Get("X-KPanel-Timestamp"), r.Header.Get("X-KPanel-Request-ID"), body,
		)
		if r.Header.Get("X-KPanel-Signature") != expected {
			t.Fatalf("capability signature = %q, want %q", r.Header.Get("X-KPanel-Signature"), expected)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cluster.LightFileCapabilityResponse{
			TerminalPeerPublicKey: base64.RawURLEncoding.EncodeToString(peer.Public),
			TargetNodeID:          targetID,
		})
	}))
	defer server.Close()
	previousClient := nodeHTTPClient
	nodeHTTPClient = server.Client()
	defer func() { nodeHTTPClient = previousClient }()

	dataDir := t.TempDir()
	configPath := filepath.Join(dataDir, "node.json")
	terminalPath := filepath.Join(dataDir, "terminal.json")
	config := nodeConfig{
		SchemaVersion: 1, Origin: server.URL, NodeID: strings.Repeat("a", 32),
		ReportingKey: base64.RawURLEncoding.EncodeToString(secret), ReportInterval: 30,
	}
	if err := writeConfigAtomic(configPath, config); err != nil {
		t.Fatal(err)
	}

	updated, identity, err := ensureFileCapability(context.Background(), configPath, config, secret, terminalPath)
	if err != nil {
		t.Fatalf("ensureFileCapability() error = %v", err)
	}
	if updated.TargetNodeID != targetID || !bytes.Equal(identity.Peer, peer.Public) {
		t.Fatalf("updated capability = %#v, identity = %#v", updated, identity)
	}
	loaded, loadedSecret, err := readConfig(configPath)
	if err != nil || loaded.TargetNodeID != targetID || !bytes.Equal(loadedSecret, secret) {
		t.Fatalf("persisted node config = %#v, secret = %x, error = %v", loaded, loadedSecret, err)
	}
	terminal, loadedIdentity, err := readTerminalConfig(terminalPath)
	if err != nil || terminal.PeerPublicKey == "" || !bytes.Equal(loadedIdentity.Key.Public, identity.Key.Public) {
		t.Fatalf("persisted terminal config = %#v, identity = %#v, error = %v", terminal, loadedIdentity, err)
	}
	if info, err := os.Stat(terminalPath); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("terminal config file = %#v, error = %v", info, err)
	}
}
