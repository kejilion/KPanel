package cluster

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type browseWSStubInput struct {
	owner  string
	id     string
	binary bool
	data   []byte
}

// serviceV2BrowseWSStub implements BrowseWSBackend with an offset-indexed
// frame list (mirrors internal/agent's browseWSManager: offset is a message
// count, not a byte offset) so tests can script exactly how many frames are
// "buffered" and whether the session reports closed once they're exhausted.
type serviceV2BrowseWSStub struct {
	mu     sync.Mutex
	opens  []struct{ owner, url string }
	inputs []browseWSStubInput
	closes []struct{ owner, id string }

	openErr   error
	sessionID string

	allFrames              []BrowseWSFrame
	closeAfterAllDelivered bool
	closeReason            string
	outputErr              error
}

func (s *serviceV2BrowseWSStub) Open(_ context.Context, owner, targetURL string, _ map[string][]string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opens = append(s.opens, struct{ owner, url string }{owner, targetURL})
	if s.openErr != nil {
		return "", s.openErr
	}
	id := s.sessionID
	if id == "" {
		id = "ws-session-1"
	}
	return id, nil
}

func (s *serviceV2BrowseWSStub) Output(_ context.Context, _, _ string, offset int, _ time.Duration) ([]BrowseWSFrame, int, bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outputErr != nil {
		return nil, 0, false, "", s.outputErr
	}
	if offset < 0 || offset > len(s.allFrames) {
		return nil, 0, false, "", ErrAuthentication
	}
	frames := append([]BrowseWSFrame(nil), s.allFrames[offset:]...)
	return frames, len(s.allFrames), s.closeAfterAllDelivered, s.closeReason, nil
}

func (s *serviceV2BrowseWSStub) Input(_ context.Context, owner, id string, binary bool, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputs = append(s.inputs, browseWSStubInput{owner: owner, id: id, binary: binary, data: append([]byte(nil), data...)})
	return nil
}

func (s *serviceV2BrowseWSStub) Close(_ context.Context, owner, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closes = append(s.closes, struct{ owner, id string }{owner, id})
	return nil
}

// newBrowseWSFederationPair wires a target (browse-ws backed by stub) and a
// center paired against it with the given scope, mirroring the target/center
// setup in browse_v2_test.go.
func newBrowseWSFederationPair(t *testing.T, clock *serviceTestClock, scope string, stub BrowseWSBackend) (target, center *Service, host Host, route *serviceV2RoundTripper) {
	t.Helper()
	targetRemote, _ := newServiceV2Remote(t)
	target, err := NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "target"), PanelVersion: "v0.72.0", Hostname: "target-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "target-v2"}, BrowseWS: stub,
		Remote: targetRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("target NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })

	centerRemote, route := newServiceV2Remote(t)
	route.target = target
	center, err = NewService(ServiceConfig{
		DataDir: filepath.Join(t.TempDir(), "center"), PanelVersion: "v0.72.0", Hostname: "center-v2",
		Telemetry: serviceTestTelemetry{now: clock.Now, hostname: "center-v2"},
		Remote:    centerRemote, Now: clock.Now, Jitter: func(value time.Duration) time.Duration { return value },
	})
	if err != nil {
		t.Fatalf("center NewService() error = %v", err)
	}
	t.Cleanup(func() { _ = center.Close() })

	code, err := target.CreatePairingCodeV2(scope)
	if err != nil {
		t.Fatalf("CreatePairingCodeV2(%q) error = %v", scope, err)
	}
	host, err = center.AddHost(context.Background(), AddHostInput{
		Name: "browse-ws-target", Origin: "http://8.8.8.8:1801", PairingCode: code.Code,
	})
	if err != nil {
		t.Fatalf("AddHost() error = %v", err)
	}
	return target, center, host, route
}

// TestServiceV2BrowseWSRelaysMessagesInputAndCloseOverEncryptedChannel pairs
// with browse-ws-only scope, opens a WS session, drains scripted frames via
// Output polling, sends Input, and closes — confirming the round trip works
// end to end and that no frame content or target URL crosses the simulated
// HTTP wire in plaintext.
func TestServiceV2BrowseWSRelaysMessagesInputAndCloseOverEncryptedChannel(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	stub := &serviceV2BrowseWSStub{
		allFrames: []BrowseWSFrame{
			{Data: []byte("frame-secret-one")},
			{Binary: true, Data: []byte{0x01, 0x02, 0x03}},
			{Data: []byte("frame-secret-three")},
		},
		closeAfterAllDelivered: true,
		closeReason:            "target closed",
	}
	_, center, host, route := newBrowseWSFederationPair(t, clock, BuildV2Scope(false, false, true), stub)
	if !host.BrowseWSAvailable || host.BrowseAvailable || host.TerminalAvailable {
		t.Fatalf("unexpected paired capabilities: %#v", host)
	}

	opened, err := center.BrowseWSOpen(context.Background(), host.ID, BrowseWSOpenRequest{URL: "wss://example.com/secret-socket"})
	if err != nil {
		t.Fatalf("BrowseWSOpen() error = %v", err)
	}
	if opened.SessionID == "" {
		t.Fatal("BrowseWSOpen() returned empty session id")
	}
	stub.mu.Lock()
	if len(stub.opens) != 1 || stub.opens[0].url != "wss://example.com/secret-socket" {
		t.Fatalf("unexpected Open call recorded: %#v", stub.opens)
	}
	if stub.opens[0].owner == "" {
		t.Fatal("Open() was called with an empty owner")
	}
	stub.mu.Unlock()

	var messages []BrowseWSMessage
	offset := 0
	closed := false
	for polls := 0; !closed; polls++ {
		if polls > 10 {
			t.Fatal("too many Output polls, relay likely stuck")
		}
		output, err := center.BrowseWSOutput(context.Background(), host.ID, BrowseWSOutputRequest{SessionID: opened.SessionID, Offset: offset})
		if err != nil {
			t.Fatalf("BrowseWSOutput() error = %v", err)
		}
		messages = append(messages, output.Messages...)
		offset = output.NextOffset
		closed = output.Closed
		if closed && output.CloseReason != "target closed" {
			t.Fatalf("CloseReason = %q, want %q", output.CloseReason, "target closed")
		}
	}
	if len(messages) != len(stub.allFrames) {
		t.Fatalf("assembled %d messages, want %d", len(messages), len(stub.allFrames))
	}
	for i, want := range stub.allFrames {
		got := messages[i]
		wantType := "text"
		if want.Binary {
			wantType = "binary"
		}
		if got.Type != wantType {
			t.Fatalf("message %d type = %q, want %q", i, got.Type, wantType)
		}
		decoded, err := base64.StdEncoding.DecodeString(got.Data)
		if err != nil || !bytes.Equal(decoded, want.Data) {
			t.Fatalf("message %d payload mismatch: got %v, want %v (err=%v)", i, decoded, want.Data, err)
		}
	}

	inputPayload := []byte("hello-target-secret")
	if err := center.BrowseWSInput(context.Background(), host.ID, BrowseWSInputRequest{
		SessionID: opened.SessionID, Type: "text", Data: base64.StdEncoding.EncodeToString(inputPayload),
	}); err != nil {
		t.Fatalf("BrowseWSInput() error = %v", err)
	}
	stub.mu.Lock()
	if len(stub.inputs) != 1 || stub.inputs[0].binary || !bytes.Equal(stub.inputs[0].data, inputPayload) {
		t.Fatalf("unexpected Input recorded: %#v", stub.inputs)
	}
	stub.mu.Unlock()

	if err := center.BrowseWSClose(context.Background(), host.ID, BrowseWSCloseRequest{SessionID: opened.SessionID}); err != nil {
		t.Fatalf("BrowseWSClose() error = %v", err)
	}
	stub.mu.Lock()
	if len(stub.closes) != 1 || stub.closes[0].id != opened.SessionID {
		t.Fatalf("unexpected Close recorded: %#v", stub.closes)
	}
	stub.mu.Unlock()

	for _, body := range route.requestBodies() {
		if bytes.Contains(body, []byte("secret-socket")) ||
			bytes.Contains(body, []byte("frame-secret-one")) ||
			bytes.Contains(body, []byte("frame-secret-three")) ||
			bytes.Contains(body, []byte("hello-target-secret")) {
			t.Fatalf("browse WS plaintext leaked on HTTP wire: %s", body)
		}
	}
}

// TestServiceV2BrowseWSRequiresBrowseWSScope confirms a controller paired
// with browse-fetch scope (but not browse-ws) is rejected locally, without
// the WS capability silently riding along on the unrelated fetch grant.
func TestServiceV2BrowseWSRequiresBrowseWSScope(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	stub := &serviceV2BrowseWSStub{}
	_, center, host, route := newBrowseWSFederationPair(t, clock, BuildV2Scope(false, true, false), stub)
	if host.BrowseWSAvailable {
		t.Fatalf("browse-fetch-only pairing must not grant BrowseWSAvailable: %#v", host)
	}

	requestsBefore := len(route.requestBodies())
	if _, err := center.BrowseWSOpen(context.Background(), host.ID, BrowseWSOpenRequest{URL: "wss://example.com/"}); !errors.Is(err, ErrBrowseWSUnavailable) {
		t.Fatalf("BrowseWSOpen() on browse-fetch-only host error = %v, want ErrBrowseWSUnavailable", err)
	}
	if len(route.requestBodies()) != requestsBefore {
		t.Fatal("scope rejection should be local; it must not reach the network")
	}
	stub.mu.Lock()
	if len(stub.opens) != 0 {
		t.Fatalf("backend Open() must not be called when scope rejects locally, got %d calls", len(stub.opens))
	}
	stub.mu.Unlock()
}

// TestServiceV2BrowseWSOutputSuppressesClosedUntilFullyDrained forces the
// scripted frame set past a single federation envelope's budget so Output
// must poll twice; the first poll must not report Closed even though the
// backend already says the session ended, because messages are still
// waiting to be delivered — reporting Closed early would make a controller
// stop polling before draining the tail.
func TestServiceV2BrowseWSOutputSuppressesClosedUntilFullyDrained(t *testing.T) {
	now := time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC)
	clock := &serviceTestClock{now: now}
	big := bytes.Repeat([]byte("x"), 11000) // only 2 fit per maxFederationBrowseWSOutputBytes poll (2*14700=29400 < 32768 < 3*14700)
	stub := &serviceV2BrowseWSStub{
		allFrames: []BrowseWSFrame{
			{Data: append([]byte(nil), big...)},
			{Data: append([]byte(nil), big...)},
			{Data: append([]byte(nil), big...)},
			{Data: []byte("final-tail-frame")},
		},
		closeAfterAllDelivered: true,
		closeReason:            "done",
	}
	_, center, host, _ := newBrowseWSFederationPair(t, clock, BuildV2Scope(false, false, true), stub)

	opened, err := center.BrowseWSOpen(context.Background(), host.ID, BrowseWSOpenRequest{URL: "wss://example.com/"})
	if err != nil {
		t.Fatalf("BrowseWSOpen() error = %v", err)
	}

	first, err := center.BrowseWSOutput(context.Background(), host.ID, BrowseWSOutputRequest{SessionID: opened.SessionID, Offset: 0})
	if err != nil {
		t.Fatalf("first BrowseWSOutput() error = %v", err)
	}
	if first.Closed {
		t.Fatal("first poll reported Closed before all buffered frames were delivered")
	}
	if len(first.Messages) == 0 || len(first.Messages) >= len(stub.allFrames) {
		t.Fatalf("expected a partial batch, got %d of %d messages", len(first.Messages), len(stub.allFrames))
	}
	if first.NextOffset != len(first.Messages) {
		t.Fatalf("NextOffset = %d, want %d (no drops expected at this frame size)", first.NextOffset, len(first.Messages))
	}

	second, err := center.BrowseWSOutput(context.Background(), host.ID, BrowseWSOutputRequest{SessionID: opened.SessionID, Offset: first.NextOffset})
	if err != nil {
		t.Fatalf("second BrowseWSOutput() error = %v", err)
	}
	if !second.Closed {
		t.Fatal("second poll should report Closed once every buffered frame is drained")
	}
	if second.CloseReason != "done" {
		t.Fatalf("CloseReason = %q, want %q", second.CloseReason, "done")
	}
	if len(first.Messages)+len(second.Messages) != len(stub.allFrames) {
		t.Fatalf("delivered %d+%d messages, want %d total", len(first.Messages), len(second.Messages), len(stub.allFrames))
	}
}

func TestTruncateBrowseWSFramesDropsOversizedFrameMidStream(t *testing.T) {
	oversized := bytes.Repeat([]byte("x"), maxFederationBrowseWSMessageBytes+1)
	frames := []BrowseWSFrame{
		{Data: []byte("small-a")},
		{Data: oversized},
		{Data: []byte("small-c")},
	}
	messages, next := truncateBrowseWSFrames(frames, 50+len(frames))
	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2 (oversized middle frame dropped)", len(messages))
	}
	if next != 50+len(frames) {
		t.Fatalf("next = %d, want %d (dropped frame still counts toward offset)", next, 50+len(frames))
	}
	decodedA, _ := base64.StdEncoding.DecodeString(messages[0].Data)
	decodedC, _ := base64.StdEncoding.DecodeString(messages[1].Data)
	if !bytes.Equal(decodedA, []byte("small-a")) || !bytes.Equal(decodedC, []byte("small-c")) {
		t.Fatalf("unexpected surviving message content: %q, %q", decodedA, decodedC)
	}
}

func TestTruncateBrowseWSFramesDropsLeadingOversizedFrame(t *testing.T) {
	oversized := bytes.Repeat([]byte("x"), maxFederationBrowseWSMessageBytes+1)
	frames := []BrowseWSFrame{
		{Data: oversized},
		{Data: []byte("small")},
	}
	messages, next := truncateBrowseWSFrames(frames, 10+len(frames))
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1 (leading oversized frame dropped, not force-included)", len(messages))
	}
	decoded, _ := base64.StdEncoding.DecodeString(messages[0].Data)
	if !bytes.Equal(decoded, []byte("small")) {
		t.Fatalf("surviving message = %q, want %q", decoded, "small")
	}
	if next != 10+len(frames) {
		t.Fatalf("next = %d, want %d", next, 10+len(frames))
	}
}

func TestTruncateBrowseWSFramesStopsAtBudgetAndReportsPartialOffset(t *testing.T) {
	const perEntryOverhead = 32
	const rawLen = 8000
	frame := func(seed byte) BrowseWSFrame {
		return BrowseWSFrame{Data: bytes.Repeat([]byte{seed}, rawLen)}
	}
	frames := []BrowseWSFrame{frame('a'), frame('b'), frame('c'), frame('d'), frame('e')}
	entrySize := base64.StdEncoding.EncodedLen(rawLen) + perEntryOverhead
	fitCount := maxFederationBrowseWSOutputBytes / entrySize
	if fitCount < 1 || fitCount >= len(frames) {
		t.Fatalf("test frame sizing produced an unusable fit count %d for %d frames", fitCount, len(frames))
	}

	messages, next := truncateBrowseWSFrames(frames, 100+len(frames))
	if len(messages) != fitCount {
		t.Fatalf("included %d messages, want %d", len(messages), fitCount)
	}
	if want := 100 + fitCount; next != want {
		t.Fatalf("next = %d, want %d", next, want)
	}
	for i, msg := range messages {
		decoded, err := base64.StdEncoding.DecodeString(msg.Data)
		if err != nil || !bytes.Equal(decoded, frames[i].Data) {
			t.Fatalf("message %d mismatch (err=%v)", i, err)
		}
	}
}

func TestTruncateBrowseWSFramesAlwaysIncludesFirstFrameEvenAloneOverBudget(t *testing.T) {
	// A single frame under maxFederationBrowseWSMessageBytes but whose
	// base64 form alone exceeds maxFederationBrowseWSOutputBytes must still
	// be delivered (as the sole entry) rather than starving the poll loop
	// forever — only frames that exceed the per-message cap are dropped.
	huge := bytes.Repeat([]byte("y"), maxFederationBrowseWSOutputBytes+1000)
	if len(huge) >= maxFederationBrowseWSMessageBytes {
		t.Fatalf("test fixture invalid: huge frame (%d) must stay under the per-message cap (%d)", len(huge), maxFederationBrowseWSMessageBytes)
	}
	frames := []BrowseWSFrame{{Data: huge}, {Data: []byte("never-reached")}}
	messages, next := truncateBrowseWSFrames(frames, 5+len(frames))
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1 (first frame force-included alone)", len(messages))
	}
	if next != 5+1 {
		t.Fatalf("next = %d, want %d", next, 5+1)
	}
	decoded, err := base64.StdEncoding.DecodeString(messages[0].Data)
	if err != nil || !bytes.Equal(decoded, huge) {
		t.Fatal("first frame content mismatch")
	}
}
