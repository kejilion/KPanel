package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func openLifecycleFile(t *testing.T, relay *lightFileRelay, ctx context.Context, input LightFileRequest) (*lightFileNode, *lightFileSession, *http.Response) {
	t.Helper()
	nodeID := strings.Repeat("e", 32)
	item := relay.node(nodeID, true)
	t.Cleanup(relay.closeAll)
	item.mu.Lock()
	item.available, item.lastPoll = true, relay.now()
	var existing []string
	for id := range item.sessions {
		existing = append(existing, id)
	}
	item.mu.Unlock()
	opened := make(chan *http.Response, 1)
	go func() {
		response, _ := relay.Open(ctx, nodeID, input)
		opened <- response
	}()
	pollCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	poll, err := relay.poll(pollCtx, nodeID, existing, nil)
	if err != nil || poll.Command == nil {
		t.Fatalf("request delivery: %#v, %v", poll, err)
	}
	item.mu.Lock()
	session := item.sessions[poll.Command.RequestID]
	if session == nil || poll.Command.Kind != "request" {
		item.mu.Unlock()
		t.Fatalf("unexpected command: %#v", poll.Command)
	}
	item.applyEvents(context.Background(), []FileRelayEvent{
		{RequestID: session.id, CommandID: poll.Command.ID, Kind: "accepted"},
		{RequestID: session.id, Kind: "response", Status: http.StatusOK},
	}, relay.now())
	item.mu.Unlock()
	select {
	case response := <-opened:
		if response == nil {
			t.Fatal("open failed")
		}
		return item, session, response
	case <-time.After(time.Second):
		t.Fatal("headers did not arrive")
	}
	return nil, nil, nil
}

func lifecycleDownload() LightFileRequest {
	return LightFileRequest{Method: http.MethodGet, Path: "/v1/files/content", Body: http.NoBody}
}

func TestLightFileSlowReaderCancelDoesNotLockNode(t *testing.T) {
	relay := newLightFileRelay(time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	item, session, response := openLifecycleFile(t, relay, ctx, lifecycleDownload())
	for i := 0; i < 8; i++ {
		if err := session.push(context.Background(), int64(i), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	pushDone := make(chan struct{})
	go func() {
		item.mu.Lock()
		item.applyEvents(context.Background(), []FileRelayEvent{{RequestID: session.id, Kind: "data", Offset: 8, Data: []byte("x")}}, time.Now())
		item.mu.Unlock()
		close(pushDone)
	}()
	time.Sleep(20 * time.Millisecond)
	second := make(chan error, 1)
	go func() { _, err := relay.commandNode(item.id); second <- err }()
	select {
	case err := <-second:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(150 * time.Millisecond):
		t.Error("slow reader holds the node lock and blocks a second request")
	}
	secondContext, stopSecond := context.WithCancel(context.Background())
	defer stopSecond()
	_, secondSession, secondResponse := openLifecycleFile(t, relay, secondContext, lifecycleDownload())
	cancel()
	closed := make(chan struct{})
	go func() { _ = response.Body.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(150 * time.Millisecond):
		t.Error("cancel/Close cannot stop a push beyond the eight-block buffer")
		// Release the baseline implementation without leaving a wedged test process.
		<-session.data
	}
	select {
	case <-pushDone:
	case <-time.After(time.Second):
		t.Fatal("push leaked")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close leaked")
	}
	item.mu.Lock()
	item.applyEvents(context.Background(), []FileRelayEvent{
		{RequestID: secondSession.id, Kind: "data", Data: []byte("second")},
		{RequestID: secondSession.id, Kind: "end"},
	}, time.Now())
	item.mu.Unlock()
	content, err := io.ReadAll(secondResponse.Body)
	if err != nil || string(content) != "second" {
		t.Fatalf("second response = %q, %v", content, err)
	}
	_ = secondResponse.Body.Close()
}

func TestLightFileLifecycleTimeoutsAndCleanup(t *testing.T) {
	for _, stage := range []string{"before-headers", "after-headers", "idle-with-live-node"} {
		t.Run(stage, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				relay := newLightFileRelay(time.Now)
				defer relay.closeAll()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				if stage == "before-headers" {
					item := relay.node(strings.Repeat("e", 32), true)
					item.available, item.lastPoll = true, time.Now()
					_, err := relay.Open(ctx, item.id, lifecycleDownload())
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("Open = %v", err)
					}
					synctest.Wait()
					if len(item.sessions) != 0 {
						t.Fatal("timed out session retained")
					}
					return
				}
				if stage == "idle-with-live-node" {
					cancel()
					ctx = context.Background()
				}
				item, _, response := openLifecycleFile(t, relay, ctx, lifecycleDownload())
				heartbeatStop, heartbeatDone := make(chan struct{}), make(chan struct{})
				if stage == "idle-with-live-node" {
					go func() {
						defer close(heartbeatDone)
						ticker := time.NewTicker(time.Minute)
						defer ticker.Stop()
						for {
							select {
							case <-heartbeatStop:
								return
							case <-ticker.C:
								item.mu.Lock()
								item.lastPoll = time.Now()
								item.mu.Unlock()
							}
						}
					}()
					defer func() { close(heartbeatStop); <-heartbeatDone }()
				}
				_, err := io.ReadAll(response.Body)
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("Read = %v", err)
				}
				_ = response.Body.Close()
				synctest.Wait()
				item.mu.Lock()
				if len(item.sessions) != 0 {
					t.Error("finished session retained")
				}
				item.mu.Unlock()
			})
		})
	}
}

func TestLightFileUploadCancellationAndEarlyEOF(t *testing.T) {
	for _, stop := range []string{"cancel", "response-end", "early-eof", "waiting-ack"} {
		t.Run(stop, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				relay := newLightFileRelay(time.Now)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				reader, writer := io.Pipe()
				defer writer.Close()
				input := LightFileRequest{Method: http.MethodPut, Path: "/v1/files/content", Body: reader, BodyLength: 10}
				item, session, response := openLifecycleFile(t, relay, ctx, input)
				synctest.Wait()
				switch stop {
				case "cancel":
					cancel()
				case "response-end":
					session.finish(nil)
				case "early-eof":
					_ = writer.Close()
				case "waiting-ack":
					_, _ = writer.Write([]byte("part"))
					synctest.Wait()
					cancel()
				}
				synctest.Wait()
				_, writeErr := writer.Write([]byte("x"))
				if writeErr == nil {
					t.Fatal("upload source was not closed")
				}
				_, readErr := io.ReadAll(response.Body)
				if stop == "early-eof" && (readErr == nil || !strings.Contains(readErr.Error(), "incomplete")) {
					t.Fatalf("early EOF = %v", readErr)
				}
				if stop == "cancel" || stop == "waiting-ack" {
					if !errors.Is(readErr, context.Canceled) {
						t.Fatalf("cancel = %v", readErr)
					}
				}
				item.mu.Lock()
				if len(item.sessions) != 0 {
					t.Error("upload session retained")
				}
				for _, command := range item.pending {
					if command.command.Kind != "cancel" {
						t.Errorf("retained command %s", command.command.Kind)
					}
				}
				// Cancel acknowledgements must work after the session is removed.
				for _, command := range item.pending {
					item.applyEvents(context.Background(), []FileRelayEvent{{RequestID: session.id, CommandID: command.command.ID, Kind: "accepted"}}, time.Now())
				}
				item.pruneCommands(time.Now())
				if len(item.pending) != 0 || len(item.queued) != 0 {
					t.Error("commands did not converge")
				}
				item.mu.Unlock()
				_ = response.Body.Close()
			})
		})
	}
}

func TestLightFilePollCancelReleasesBackpressure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		relay := newLightFileRelay(time.Now)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		item, session, response := openLifecycleFile(t, relay, ctx, lifecycleDownload())
		for i := 0; i < 8; i++ {
			_ = session.push(ctx, int64(i), []byte("x"))
		}
		pollCtx, cancelPoll := context.WithCancel(context.Background())
		pollDone := make(chan struct{})
		go func() {
			_, _ = relay.poll(pollCtx, item.id, []string{session.id}, []FileRelayEvent{{RequestID: session.id, Kind: "data", Offset: 8, Data: []byte("x")}})
			close(pollDone)
		}()
		synctest.Wait()
		cancelPoll()
		<-pollDone
		if _, err := response.Body.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
			t.Fatalf("poll cancellation Read = %v", err)
		}
		_ = response.Body.Close()
	})
}

func TestLightFileBackpressurePreservesOrderedBoundedStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		relay := newLightFileRelay(time.Now)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, session, response := openLifecycleFile(t, relay, ctx, lifecycleDownload())
		chunk := strings.Repeat("stream", lightFileChunkBytes/6)
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			for i := 0; i < 40; i++ {
				if err := session.push(ctx, int64(i*len(chunk)), []byte(chunk)); err != nil {
					session.finish(err)
					return
				}
			}
			session.finish(nil)
		}()
		synctest.Wait()
		if len(session.data) != 8 || cap(session.data) != 8 {
			t.Fatal("response buffer is not bounded at eight chunks")
		}
		content, err := io.ReadAll(response.Body)
		if err != nil || string(content) != strings.Repeat(chunk, 40) {
			t.Fatalf("stream length=%d, err=%v", len(content), err)
		}
		<-finished
		_ = response.Body.Close()
	})
}

func TestLightFileSessionAndExpiredCancelQueueLimits(t *testing.T) {
	relay := newLightFileRelay(time.Now)
	item := relay.node(strings.Repeat("e", 32), true)
	item.available, item.lastPoll = true, time.Now()
	for i := 0; i < lightFileQueueLimit; i++ {
		item.sessions[fmt.Sprintf("%032x", i)] = nil
	}
	if _, err := relay.Open(context.Background(), item.id, lifecycleDownload()); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("session limit = %v", err)
	}
	item.sessions = make(map[string]*lightFileSession)
	for i := 0; i < lightFileQueueLimit; i++ {
		id := fmt.Sprintf("%032x", i)
		command := &lightFileCommand{command: FileRelayCommand{ID: id, Kind: "cancel", ExpiresAt: time.Now().Add(-time.Second).Unix()}, done: make(chan error, 1)}
		item.pending[id] = command
		item.queued = append(item.queued, command)
	}
	item.pruneCommands(time.Now())
	if len(item.pending) != 0 || len(item.queued) != 0 {
		t.Fatal("expired cancel commands retained queue capacity")
	}
}

func TestLightFileLongPollDoesNotReplayAnOldSessionSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		relay := newLightFileRelay(time.Now)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		nodeID, requestID := strings.Repeat("e", 32), strings.Repeat("a", 32)
		polled := make(chan struct{})
		go func() { _, _ = relay.poll(ctx, nodeID, nil, nil); close(polled) }()
		synctest.Wait()
		item := relay.node(nodeID, false)
		session := &lightFileSession{id: requestID, responseReady: make(chan struct{}), data: make(chan []byte, 8), done: make(chan struct{}), space: make(chan struct{}, 1)}
		item.mu.Lock()
		// Another overlapping poll has delivered a newly created request while
		// this poll still holds the node's earlier empty inventory.
		item.sessions[requestID] = session
		command := &lightFileCommand{command: FileRelayCommand{ID: requestID, RequestID: requestID, Kind: "request", ExpiresAt: time.Now().Add(time.Minute).Unix()}, delivered: true, done: make(chan error, 1)}
		item.pending[requestID] = command
		wakeLightFileNode(item)
		item.mu.Unlock()
		synctest.Wait()
		if session.isFinished() {
			t.Error("old long-poll inventory discarded the newly delivered request")
		}
		cancel()
		<-polled
		relay.closeAll()
	})
}

func TestLightFileBodyReadObservesCancellationAfterHeaders(t *testing.T) {
	relay := newLightFileRelay(time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	_, session, response := openLifecycleFile(t, relay, ctx, lifecycleDownload())
	result := make(chan error, 1)
	go func() { _, err := response.Body.Read(make([]byte, 1)); result <- err }()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Read = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("body read ignores cancellation after headers")
		session.finish(io.ErrClosedPipe)
		<-result
	}
	_ = response.Body.Close()
}

func TestLightFileOfflineAfterHeadersTerminatesRead(t *testing.T) {
	clock := &serviceTestClock{now: time.Now()}
	relay := newLightFileRelay(clock.Now)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, session, response := openLifecycleFile(t, relay, ctx, lifecycleDownload())
	result := make(chan error, 1)
	go func() { _, err := response.Body.Read(make([]byte, 1)); result <- err }()
	clock.Advance(lightFileLiveness + time.Second)
	select {
	case err := <-result:
		if !errors.Is(err, ErrFileRelayUnavailable) {
			t.Errorf("offline Read = %v", err)
		}
	case <-time.After(1500 * time.Millisecond):
		t.Error("offline node leaves body read waiting")
		session.finish(io.ErrClosedPipe)
		<-result
	}
	_ = response.Body.Close()
}
