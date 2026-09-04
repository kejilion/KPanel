package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFileRelayUsesV2NoiseAndEnablesCapabilityOnlyAfterAuthentication(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	service := newLightServiceForTest(t, clock)
	enrollment, err := service.CreateLightEnrollment()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(enrollment.Command)
	key, err := GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	response, err := service.EnrollLightNode("198.51.100.10", LightEnrollRequest{
		Token: strings.Trim(fields[len(fields)-1], "'"), Name: "edge-files", NodeVersion: "0.40.0",
		TerminalPublicKey: encodeTestKey(key.Public),
	})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := decodeTerminalRelayPublicKey(response.TerminalPeerPublicKey)
	if err != nil || response.TargetNodeID == "" {
		t.Fatalf("invalid file relay enrollment response: %#v, %v", response, err)
	}
	payload, err := json.Marshal(FileRelayPollRequest{})
	if err != nil {
		t.Fatal(err)
	}
	envelope, _, err := sealV2Request(
		"POST", v2FileRelayPath,
		v2Envelope{
			Protocol: FederationProtocolV2, ControllerID: response.NodeID,
			TargetID: response.TargetNodeID, Timestamp: now.Unix(), RequestID: strings.Repeat("a", 32),
		}, key, peer, nil, payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.HandleFederationV2(ctx, "198.51.100.10", v2FileRelayPath, "", envelope); !errors.Is(err, context.Canceled) {
		t.Fatalf("authenticated file relay error = %v, want context.Canceled", err)
	}
	host, err := service.Host(context.Background(), response.NodeID)
	if err != nil || !host.FileManagementAvailable || host.Scope != SummaryTerminalFilesScope {
		t.Fatalf("authenticated v2 file relay did not expose file capability: %#v, %v", host, err)
	}

	wrongKey, err := GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	wrongEnvelope, _, err := sealV2Request(
		"POST", v2FileRelayPath,
		v2Envelope{
			Protocol: FederationProtocolV2, ControllerID: response.NodeID,
			TargetID: response.TargetNodeID, Timestamp: now.Unix(), RequestID: strings.Repeat("b", 32),
		}, wrongKey, peer, nil, payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.HandleFederationV2(context.Background(), "198.51.100.10", v2FileRelayPath, "", wrongEnvelope); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("wrong file relay identity error = %v, want ErrAuthentication", err)
	}
	clock.Advance(lightFileLiveness + time.Second)
	host, err = service.Host(context.Background(), response.NodeID)
	if err != nil || host.FileManagementAvailable || host.Scope != SummaryScope {
		t.Fatalf("stale file relay kept capability available: %#v, %v", host, err)
	}
}

func TestFileRelayRedeliversCommandsAndStreamsResponse(t *testing.T) {
	relay := newLightFileRelay(time.Now)
	nodeID := strings.Repeat("f", 32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startPoll := func(requestIDs []string, events []FileRelayEvent) <-chan filePollResult {
		result := make(chan filePollResult, 1)
		go func() {
			response, err := relay.poll(ctx, nodeID, requestIDs, events)
			result <- filePollResult{response: response, err: err}
		}()
		return result
	}
	awaitPoll := func(result <-chan filePollResult) filePollResult {
		t.Helper()
		select {
		case value := <-result:
			if value.err != nil {
				t.Fatalf("file relay poll error = %v", value.err)
			}
			return value
		case <-time.After(2 * time.Second):
			t.Fatal("file relay poll did not return")
			return filePollResult{}
		}
	}
	waitAvailable := func() {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if relay.available(nodeID) {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("file relay did not become available")
	}

	firstPoll := startPoll(nil, nil)
	waitAvailable()
	openResult := make(chan struct {
		response *http.Response
		err      error
	}, 1)
	go func() {
		response, err := relay.Open(ctx, nodeID, LightFileRequest{
			Method: http.MethodGet, Path: "/v1/files", RawQuery: "path=%2F&limit=100",
			Body: http.NoBody, BodyLength: 0,
		})
		openResult <- struct {
			response *http.Response
			err      error
		}{response: response, err: err}
	}()

	first := awaitPoll(firstPoll)
	if first.response.Command == nil || first.response.Command.Kind != "request" || first.response.Command.Path != "/v1/files" {
		t.Fatalf("first file command = %#v, want request /v1/files", first.response.Command)
	}
	if err := validateFileRelayCommand(*first.response.Command, time.Now().UTC()); err != nil {
		t.Fatalf("first file command validation error = %v", err)
	}

	retry := awaitPoll(startPoll([]string{first.response.Command.RequestID}, nil))
	if retry.response.Command == nil || retry.response.Command.ID != first.response.Command.ID {
		t.Fatalf("redelivered file command = %#v, want command %s", retry.response.Command, first.response.Command.ID)
	}

	requestID := first.response.Command.RequestID
	responsePoll := startPoll([]string{requestID}, []FileRelayEvent{
		{CommandID: first.response.Command.ID, RequestID: requestID, Kind: "accepted"},
		{RequestID: requestID, Kind: "response", Status: http.StatusOK, Headers: map[string]string{"Content-Type": "application/json"}},
		{RequestID: requestID, Kind: "data", Offset: 0, Data: []byte(`{"path":"/"}`)},
		{RequestID: requestID, Kind: "end"},
	})
	opened := <-openResult
	if opened.err != nil || opened.response == nil || opened.response.StatusCode != http.StatusOK {
		t.Fatalf("relay.Open() = %#v, %v", opened.response, opened.err)
	}
	defer opened.response.Body.Close()
	content, err := io.ReadAll(opened.response.Body)
	if err != nil || !bytes.Equal(content, []byte(`{"path":"/"}`)) {
		t.Fatalf("file relay response = %q, error = %v", content, err)
	}
	if opened.response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("file relay response headers = %#v", opened.response.Header)
	}
	_ = awaitPoll(responsePoll)
}

type filePollResult struct {
	response FileRelayPollResponse
	err      error
}

func TestFileRelayRejectsNonFileManagerRoutes(t *testing.T) {
	for _, path := range []string{
		"/v1/files",
		"/v1/files/content",
		"/v1/files/transfer/import",
	} {
		if !FileRelayRequestPath(path) {
			t.Fatalf("FileRelayRequestPath(%q) = false", path)
		}
	}
	for _, path := range []string{
		"/v1/files/transfers",
		"/v1/files/download-tickets",
		"/v1/terminal",
	} {
		if FileRelayRequestPath(path) {
			t.Fatalf("FileRelayRequestPath(%q) = true", path)
		}
	}
	if _, err := (&lightFileRelay{}).Open(context.Background(), strings.Repeat("f", 32), LightFileRequest{Method: http.MethodDelete, Path: "/v1/files"}); !errors.Is(err, ErrFileRelayUnavailable) {
		t.Fatalf("invalid file relay request error = %v", err)
	}
}
