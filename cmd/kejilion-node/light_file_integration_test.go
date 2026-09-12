package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

type fileRelayTestTelemetry struct{}

func (fileRelayTestTelemetry) Telemetry(context.Context) (contract.HostTelemetry, error) {
	return contract.HostTelemetry{}, nil
}

func TestLightFileEncryptedTransferSurvivesLostAcknowledgement(t *testing.T) {
	var service *cluster.Service
	var dropNext, dropped atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope cluster.FederationEnvelopeV2
		if json.NewDecoder(r.Body).Decode(&envelope) != nil {
			http.Error(w, "invalid envelope", http.StatusBadRequest)
			return
		}
		response, err := service.HandleFederationV2(r.Context(), "198.51.100.10", r.URL.Path, "", envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// The center has accepted the data, but its encrypted acknowledgement
		// is lost. The node must resend the same events, not repeat the file action.
		if dropNext.CompareAndSwap(true, false) {
			dropped.Store(true)
			http.Error(w, "temporary proxy failure", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	var err error
	service, err = cluster.NewService(cluster.ServiceConfig{
		DataDir: t.TempDir(), PublicURL: server.URL, PanelVersion: "1.14.1",
		Telemetry: fileRelayTestTelemetry{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	key, err := cluster.GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	enrollment, err := service.CreateLightEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	identity, err := service.EnrollLightNode("198.51.100.10", cluster.LightEnrollRequest{
		Token: strings.Trim(fields[len(fields)-1], "'"), Name: "relay-test", NodeVersion: "1.14.1",
		TerminalPublicKey: base64.RawURLEncoding.EncodeToString(key.Public),
	})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := base64.RawURLEncoding.DecodeString(identity.TerminalPeerPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	client, err := cluster.NewFileRelayClient(server.Client())
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("verified relay payload\n"), 24<<10)
	var actions atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actions.Add(1)
		_, _ = w.Write(payload)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		runLightFileControl(ctx, nodeConfig{Origin: server.URL, NodeID: identity.NodeID, TargetNodeID: identity.TargetNodeID},
			terminalIdentity{Key: key, Peer: peer}, client, handler)
	}()
	defer func() { cancel(); <-finished }()
	for {
		host, err := service.Host(ctx, identity.NodeID)
		if err == nil && host.FileManagementAvailable {
			break
		}
		if !waitContext(ctx, 5*time.Millisecond) {
			t.Fatal("file relay did not connect")
		}
	}
	response, err := service.OpenLightFile(ctx, identity.NodeID, cluster.LightFileRequest{
		Method: http.MethodGet, Path: "/v1/files/content", Body: http.NoBody,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	prefix := make([]byte, lightFileEventChunkBytes)
	if _, err := io.ReadFull(response.Body, prefix); err != nil {
		t.Fatal(err)
	}
	dropNext.Store(true)
	rest, err := io.ReadAll(response.Body)
	if err != nil || !bytes.Equal(append(prefix, rest...), payload) {
		t.Fatalf("encrypted transfer lost or duplicated data: bytes=%d err=%v", len(prefix)+len(rest), err)
	}
	if !dropped.Load() || actions.Load() != 1 {
		t.Fatalf("failure injection=%v file actions=%d", dropped.Load(), actions.Load())
	}
}
