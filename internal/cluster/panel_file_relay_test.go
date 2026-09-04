package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPanelFileRelayExecutesConstrainedRequestAndStreamsBody(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	relay := newPanelFileRelay()
	relay.setHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/files/actions" {
			t.Errorf("relayed request = %s %s", r.Method, r.URL.Path)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read relayed request body: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write(append([]byte(`{"body":`), append(mustJSONQuote(body), '}')...))
	}))

	sessionID := strings.Repeat("a", 32)
	requestCommand := FileRelayCommand{
		ID: "00112233445566778899aabbccddeeff", Kind: "request", RequestID: sessionID,
		Method: http.MethodPost, Path: "/v1/files/actions", Query: "",
		Headers: map[string]string{"Content-Type": "application/json"}, BodyLength: 5,
		ExpiresAt: now.Add(time.Minute).Unix(),
	}
	if err := validateFileRelayCommand(requestCommand, now); err != nil {
		t.Fatalf("request command validation: %v", err)
	}
	bodyCommand := FileRelayCommand{
		ID: "11223344556677889900aabbccddeeff", Kind: "body", RequestID: sessionID,
		Offset: 0, Data: []byte("hello"), Final: true,
		ExpiresAt: now.Add(time.Minute).Unix(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	poll := func(input FileRelayPollRequest) FileRelayPollResponse {
		response, err := relay.poll(ctx, input, now)
		if err != nil {
			t.Fatalf("panel file relay poll: %v", err)
		}
		return response
	}

	accepted := poll(FileRelayPollRequest{SessionID: sessionID, Command: &requestCommand})
	if len(accepted.Events) != 1 || accepted.Events[0].Kind != "accepted" ||
		accepted.Events[0].CommandID != requestCommand.ID {
		t.Fatalf("request acceptance = %#v", accepted.Events)
	}

	bodyAccepted := poll(FileRelayPollRequest{SessionID: sessionID, Command: &bodyCommand})
	if len(bodyAccepted.Events) != 1 || bodyAccepted.Events[0].Kind != "accepted" ||
		bodyAccepted.Events[0].CommandID != bodyCommand.ID {
		t.Fatalf("body acceptance = %#v", bodyAccepted.Events)
	}

	responseEvent := poll(FileRelayPollRequest{SessionID: sessionID})
	if len(responseEvent.Events) != 1 || responseEvent.Events[0].Kind != "response" ||
		responseEvent.Events[0].Status != http.StatusAccepted {
		t.Fatalf("response event = %#v", responseEvent.Events)
	}
	dataEvent := poll(FileRelayPollRequest{SessionID: sessionID})
	if len(dataEvent.Events) != 1 || dataEvent.Events[0].Kind != "data" ||
		!bytes.Contains(dataEvent.Events[0].Data, []byte("hello")) {
		t.Fatalf("data event = %#v", dataEvent.Events)
	}
	endEvent := poll(FileRelayPollRequest{SessionID: sessionID})
	if len(endEvent.Events) != 1 || endEvent.Events[0].Kind != "end" {
		t.Fatalf("end event = %#v", endEvent.Events)
	}
}

func TestFileRelayPollShapesStaySeparated(t *testing.T) {
	panelPoll := FileRelayPollRequest{SessionID: strings.Repeat("a", 32)}
	if validateFileRelayPoll(panelPoll) == nil {
		t.Fatal("lightweight-node poll validator accepted a Panel session")
	}
	if err := validatePanelFileRelayPoll(panelPoll, time.Now().UTC()); err != nil {
		t.Fatalf("Panel poll validator rejected an empty poll: %v", err)
	}
}

func TestServiceV2OpensPairedPanelFileInCurrentRelay(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	clock := &serviceTestClock{now: now}
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.40.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"},
		Remote:    targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	centerRemote.streamClient = centerRemote.client
	route.target = target
	center, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.40.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2()
	if err != nil {
		t.Fatalf("CreatePairingCodeV2() error = %v", err)
	}
	host, err := center.AddHost(context.Background(), AddHostInput{
		Name: "paired-panel", Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	if host.Kind != HostKindPanel || host.FederationProtocol != FederationProtocolV2 ||
		!host.FileManagementAvailable {
		t.Fatalf("paired Panel file capability = %#v", host)
	}

	target.SetFileRelayHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/files" || r.URL.Query().Get("path") != "/" {
			t.Errorf("target file request = %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))

	response, err := center.OpenRemotePanelFile(context.Background(), host.ID, LightFileRequest{
		Method: http.MethodGet, Path: "/v1/files", RawQuery: "path=%2F&limit=100",
		Body: http.NoBody, BodyLength: 0,
	})
	if err != nil {
		t.Fatalf("OpenRemotePanelFile() error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("remote file response = status %d headers %#v", response.StatusCode, response.Header)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read remote file response: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || len(payload) != 1 {
		t.Fatalf("remote file response body = %q, error = %v", body, err)
	}
}

func mustJSONQuote(value []byte) []byte {
	encoded, err := json.Marshal(string(value))
	if err != nil {
		panic(err)
	}
	return encoded
}
