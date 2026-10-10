package panel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type cancelStreamAgentStub struct {
	terminalAgentStub
	started       chan context.Context
	release       chan struct{}
	waitForCancel bool
}

func (s *cancelStreamAgentStub) Get(ctx context.Context, path, _, _ string) (AgentResponse, error) {
	s.started <- ctx
	if s.waitForCancel {
		<-ctx.Done()
		return AgentResponse{}, ctx.Err()
	}
	// A successful read may race with cancellation. Return that response even
	// after cancellation so the pump must check its own subscription lifetime.
	<-s.release
	body := []byte(`{"dataBase64":"b2xk","nextOffset":3,"finished":true}`)
	if strings.HasSuffix(path, "/output") {
		body = []byte(`{"data":"b2xk","offset":0,"nextOffset":3,"closed":true}`)
	}
	return AgentResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: body}, nil
}

func TestTerminalStreamPumpStopsOnSubscriptionCancel(t *testing.T) {
	for _, kind := range []string{"terminal", "job"} {
		for _, scenario := range []string{"full-queue", "ready-queue", "long-poll"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				streamCtx, stopStream := context.WithCancel(context.Background())
				defer stopStream()
				stream := &terminalStream{ctx: streamCtx, userID: "user", events: make(chan terminalStreamEvent, terminalStreamEventBuffer)}
				queued := 0
				if scenario == "full-queue" {
					queued = cap(stream.events)
					for range queued {
						stream.events <- terminalStreamEvent{Key: "queued"}
					}
				}
				stub := &cancelStreamAgentStub{started: make(chan context.Context, 1), release: make(chan struct{}), waitForCancel: scenario == "long-poll"}
				release := sync.OnceFunc(func() { close(stub.release) })
				defer release()
				server := &Server{agent: stub, terminalSessions: map[string]panelTerminalSession{
					"id": {ID: "id", BackendSessionID: "backend", HostID: "local", UserID: "user"},
				}}
				server.hostOps = newHostOperationService(server)
				ctx, cancel := context.WithCancel(streamCtx)
				defer cancel()
				done := make(chan struct{})
				go func() {
					defer close(done)
					item := terminalStreamSubscription{Kind: kind, Job: "app", ID: "id"}
					if kind == "terminal" {
						server.pumpHostTerminal(ctx, stream, "terminal:id", item)
					} else {
						server.pumpJobTerminal(ctx, stream, "job:app:id", item)
					}
				}()
				t.Cleanup(func() {
					cancel()
					stopStream()
					release()
					select {
					case <-done:
					case <-time.After(time.Second):
						t.Error("pump survived stream shutdown")
					}
				})
				select {
				case <-stub.started:
				case <-time.After(time.Second):
					t.Fatal("pump did not start its backend read")
				}
				if scenario == "full-queue" {
					release()
					select {
					case <-done:
						t.Fatal("pump returned while its output queue was full")
					case <-time.After(25 * time.Millisecond):
					}
				}
				cancel()
				release()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("subscription cancellation did not release the pump")
				}
				if streamCtx.Err() != nil {
					t.Fatal("subscription cancellation closed the shared stream")
				}
				if got := len(stream.events); got != queued {
					t.Fatalf("canceled pump queued output: events=%d, want %d", got, queued)
				}
			})
		}
	}
}

func TestTerminalStreamReplacementCancelsPreviousRead(t *testing.T) {
	server, tokenPath := newTestServer(t)
	stub := &cancelStreamAgentStub{started: make(chan context.Context, 2), release: make(chan struct{}), waitForCancel: true}
	server.agent = stub
	sessionCookie, csrfCookie := bootstrapCookies(t, server, tokenPath)
	session, err := server.auth.Authenticate(sessionCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	stream, ok := server.terminalStreams.register(session.User.ID, sha256.Sum256([]byte(sessionCookie.Value)))
	if !ok {
		t.Fatal("stream registration failed")
	}
	defer server.terminalStreams.remove(stream)
	item := terminalStreamSubscription{Kind: "job", Job: "app", ID: "0123456789abcdef0123456789abcdef"}
	headers := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrfCookie.Value}
	subscribe := func(input terminalStreamSubscriptionRequest) {
		t.Helper()
		body, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		response := authenticatedRequest(server, http.MethodPost, terminalStreamSubscriptionsPath, body, sessionCookie, csrfCookie, headers)
		if response.Code != http.StatusOK {
			t.Fatalf("subscribe = %d %s", response.Code, response.Body.String())
		}
	}
	readContext := func() context.Context {
		t.Helper()
		select {
		case ctx := <-stub.started:
			return ctx
		case <-time.After(time.Second):
			t.Fatal("subscription did not start a backend read")
			return nil
		}
	}
	subscribe(terminalStreamSubscriptionRequest{StreamID: stream.id, Add: []terminalStreamSubscription{item}})
	previous := readContext()
	item.Offset = 42
	subscribe(terminalStreamSubscriptionRequest{StreamID: stream.id, Add: []terminalStreamSubscription{item}})
	current := readContext()
	if previous.Err() != context.Canceled || current.Err() != nil {
		t.Fatalf("replacement contexts: previous=%v current=%v", previous.Err(), current.Err())
	}
	subscribe(terminalStreamSubscriptionRequest{StreamID: stream.id, Remove: []string{terminalStreamKey(item)}})
	if current.Err() != context.Canceled || stream.ctx.Err() != nil {
		t.Fatalf("removal contexts: subscription=%v stream=%v", current.Err(), stream.ctx.Err())
	}
}

func TestTerminalStreamSkipsCanceledSubscriptionEvents(t *testing.T) {
	server, tokenPath := newTestServer(t)
	sessionCookie, _ := bootstrapCookies(t, server, tokenPath)
	response, reader, streamID := openTerminalStream(t, server, sessionCookie)
	timeout := time.AfterFunc(2*time.Second, func() { response.Body.Close() })
	defer timeout.Stop()
	session, err := server.auth.Authenticate(sessionCookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	stream := server.terminalStreams.lookup(streamID, session.User.ID, sha256.Sum256([]byte(sessionCookie.Value)))
	if stream == nil {
		t.Fatal("stream not found")
	}
	previous, cancel := context.WithCancel(stream.ctx)
	defer cancel()
	// Stage an event before cancellation without letting the HTTP writer race
	// ahead, then deliver it to the real stream after the subscription ends.
	pending := &terminalStream{ctx: stream.ctx, events: make(chan terminalStreamEvent, 1)}
	key := "job:app:0123456789abcdef0123456789abcdef"
	if !pending.emit(previous, terminalStreamEvent{Key: key, Job: &jobTerminalChunk{DataBase64: "b2xk", NextOffset: 3}}) {
		t.Fatal("could not stage previous subscription output")
	}
	cancel()
	stream.events <- <-pending.events
	if !stream.emit(stream.ctx, terminalStreamEvent{Key: key, Job: &jobTerminalChunk{DataBase64: "bmV3", NextOffset: 6}}) {
		t.Fatal("live subscription could not enqueue output")
	}
	event, data := reader.next(t)
	var payload terminalStreamEvent
	if event != "output" || json.Unmarshal([]byte(data), &payload) != nil || payload.Key != key ||
		payload.Job == nil || payload.Job.DataBase64 != "bmV3" || payload.Job.NextOffset != 6 {
		t.Fatalf("first output after subscription replacement = %s %s", event, data)
	}
}
