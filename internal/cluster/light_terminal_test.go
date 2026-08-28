package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

func TestTerminalRelayUsesV2NoiseAndEnablesCapabilityOnlyAfterAuthentication(t *testing.T) {
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
		Token: strings.Trim(fields[len(fields)-1], "'"), Name: "edge-1", NodeVersion: "0.40.0",
		TerminalPublicKey: encodeTestKey(key.Public),
	})
	if err != nil {
		t.Fatal(err)
	}
	peer, err := decodeTerminalRelayPublicKey(response.TerminalPeerPublicKey)
	if err != nil || response.TargetNodeID == "" {
		t.Fatalf("invalid terminal enrollment response: %#v, %v", response, err)
	}
	payload, err := json.Marshal(TerminalRelayPollRequest{})
	if err != nil {
		t.Fatal(err)
	}
	envelope, _, err := sealV2Request(
		"POST", v2TerminalRelayPath,
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
	if _, err := service.HandleFederationV2(ctx, "198.51.100.10", v2TerminalRelayPath, "", envelope); !errors.Is(err, context.Canceled) {
		t.Fatalf("authenticated terminal relay error = %v, want context.Canceled", err)
	}
	host, err := service.Host(context.Background(), response.NodeID)
	if err != nil || !host.TerminalAvailable || host.Scope != SummaryTerminalScope {
		t.Fatalf("authenticated v2 relay did not expose terminal capability: %#v, %v", host, err)
	}
	wrongKey, err := GenerateFederationV2Keypair()
	if err != nil {
		t.Fatal(err)
	}
	wrongEnvelope, _, err := sealV2Request(
		"POST", v2TerminalRelayPath,
		v2Envelope{
			Protocol: FederationProtocolV2, ControllerID: response.NodeID,
			TargetID: response.TargetNodeID, Timestamp: now.Unix(), RequestID: strings.Repeat("b", 32),
		}, wrongKey, peer, nil, payload,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.HandleFederationV2(context.Background(), "198.51.100.10", v2TerminalRelayPath, "", wrongEnvelope); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("wrong terminal identity error = %v, want ErrAuthentication", err)
	}
	clock.Advance(lightTerminalLiveness + time.Second)
	host, err = service.Host(context.Background(), response.NodeID)
	if err != nil || host.TerminalAvailable || host.Scope != SummaryScope {
		t.Fatalf("stale v2 relay kept capability available: %#v, %v", host, err)
	}
}

func encodeTestKey(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func TestTerminalRelayRedeliversCommandsAndTracksLifecycle(t *testing.T) {
	relay := newLightTerminalRelay(time.Now)
	nodeID := strings.Repeat("c", 32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startPoll := func(sessionIDs []string, events []TerminalRelayEvent) <-chan lightPollResult {
		result := make(chan lightPollResult, 1)
		go func() {
			response, err := relay.poll(ctx, nodeID, sessionIDs, events)
			result <- lightPollResult{response: response, err: err}
		}()
		return result
	}
	awaitPoll := func(result <-chan lightPollResult) lightPollResult {
		t.Helper()
		select {
		case value := <-result:
			if value.err != nil {
				t.Fatalf("terminal relay poll error = %v", value.err)
			}
			return value
		case <-time.After(2 * time.Second):
			t.Fatal("terminal relay poll did not return")
			return lightPollResult{}
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
		t.Fatal("terminal relay did not become available")
	}

	firstPoll := startPoll(nil, nil)
	waitAvailable()
	openResult := make(chan struct {
		response TerminalOpenResponse
		err      error
	}, 1)
	go func() {
		response, err := relay.open(ctx, nodeID, 30, 120)
		openResult <- struct {
			response TerminalOpenResponse
			err      error
		}{response: response, err: err}
	}()
	first := awaitPoll(firstPoll)
	if !validID(first.response.Epoch) {
		t.Fatalf("first terminal relay epoch = %q, want a valid center epoch", first.response.Epoch)
	}
	if first.response.Command == nil || first.response.Command.Path != v2TerminalOpenPath {
		t.Fatalf("first terminal command = %#v, want open", first.response.Command)
	}
	if err := validateTerminalRelayCommand(*first.response.Command, time.Now().UTC()); err != nil {
		t.Fatalf("first terminal command validation error = %v", err)
	}

	retry := awaitPoll(startPoll(nil, nil))
	if retry.response.Command == nil || retry.response.Command.ID != first.response.Command.ID {
		t.Fatalf("redelivered terminal command = %#v, want command %s", retry.response.Command, first.response.Command.ID)
	}
	sessionID := first.response.Command.SessionID
	openedPoll := startPoll([]string{sessionID}, []TerminalRelayEvent{{
		CommandID: first.response.Command.ID, Kind: "opened", SessionID: sessionID,
	}})
	opened := <-openResult
	if opened.err != nil || opened.response.SessionID != sessionID {
		t.Fatalf("relay.open() = %#v, %v", opened.response, opened.err)
	}

	inputResult := make(chan error, 1)
	go func() {
		inputResult <- relay.input(ctx, nodeID, TerminalInputRequest{SessionID: sessionID, Data: "ZWNobyBvaw=="})
	}()
	inputCommand := awaitPoll(openedPoll)
	if inputCommand.response.Command == nil || inputCommand.response.Command.Path != v2TerminalInputPath {
		t.Fatalf("input terminal command = %#v, want input", inputCommand.response.Command)
	}
	acceptedPoll := startPoll([]string{sessionID}, []TerminalRelayEvent{{
		CommandID: inputCommand.response.Command.ID, Kind: "accepted", SessionID: sessionID,
	}})
	if err := <-inputResult; err != nil {
		t.Fatalf("relay.input() error = %v", err)
	}
	_ = awaitPoll(acceptedPoll)

	outputPoll := startPoll([]string{sessionID}, []TerminalRelayEvent{{
		Kind: "output", SessionID: sessionID, Offset: 0, NextOffset: 5, Data: []byte("hello"),
	}})
	outputDeadline := time.Now().Add(2 * time.Second)
	for {
		output, err := relay.output(context.Background(), nodeID, TerminalOutputRequest{SessionID: sessionID, Offset: 0})
		if err == nil && bytes.Equal(output.Data, []byte("hello")) && output.NextOffset == 5 {
			break
		}
		if time.Now().After(outputDeadline) {
			t.Fatalf("relay.output() = %#v, %v", output, err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	closeResult := make(chan error, 1)
	go func() { closeResult <- relay.close(ctx, nodeID, TerminalCloseRequest{SessionID: sessionID}) }()
	closeCommand := awaitPoll(outputPoll)
	if closeCommand.response.Command == nil || closeCommand.response.Command.Path != v2TerminalClosePath {
		t.Fatalf("close terminal command = %#v, want close", closeCommand.response.Command)
	}
	finalPoll := startPoll(nil, []TerminalRelayEvent{{
		CommandID: closeCommand.response.Command.ID, Kind: "closed", SessionID: sessionID,
	}})
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatalf("relay.close() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay.close() did not receive its acknowledgement")
	}
	cancel()
	select {
	case result := <-finalPoll:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("final terminal poll error = %v, want context.Canceled", result.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("final terminal poll did not stop")
	}
	if _, err := relay.output(context.Background(), nodeID, TerminalOutputRequest{SessionID: sessionID, Offset: 0}); !errors.Is(err, terminal.ErrNotFound) {
		t.Fatalf("closed relay output error = %v, want terminal.ErrNotFound", err)
	}
}

type lightPollResult struct {
	response TerminalRelayPollResponse
	err      error
}

func TestTerminalRelayReconcilesOnlyPendingOpensAfterRestart(t *testing.T) {
	relay := newLightTerminalRelay(time.Now)
	nodeID := strings.Repeat("e", 32)
	sessionID := strings.Repeat("f", 32)
	openID := strings.Repeat("1", 32)
	inputID := strings.Repeat("2", 32)
	now := time.Now().UTC()
	item := relay.node(nodeID, true)
	item.mu.Lock()
	item.sessions[sessionID] = &lightTerminalSession{id: sessionID, notify: make(chan struct{})}
	openRequest := &lightTerminalCommand{
		command: TerminalRelayCommand{ID: openID, Path: v2TerminalOpenPath, SessionID: sessionID},
		done:    make(chan error, 1),
	}
	inputRequest := &lightTerminalCommand{
		command: TerminalRelayCommand{ID: inputID, Path: v2TerminalInputPath, SessionID: sessionID},
		done:    make(chan error, 1),
	}
	item.pending[openID] = openRequest
	item.pending[inputID] = inputRequest
	item.reconcileSessions(nil, now)
	_, sessionKept := item.sessions[sessionID]
	_, openKept := item.pending[openID]
	_, inputKept := item.pending[inputID]
	item.mu.Unlock()
	if !sessionKept || !openKept {
		t.Fatalf("pending open was not preserved: session=%v open=%v", sessionKept, openKept)
	}
	if inputKept {
		t.Fatal("stale input command kept a session after restart reconciliation")
	}
	select {
	case err := <-inputRequest.done:
		if !errors.Is(err, terminal.ErrClosed) {
			t.Fatalf("stale input completion error = %v, want terminal.ErrClosed", err)
		}
	default:
		t.Fatal("stale input command was not completed during reconciliation")
	}
}
