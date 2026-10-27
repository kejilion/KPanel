package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

func TestLightFileTransientUnsupportedResponseAfterConnectionRecoversPromptly(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusUpgradeRequired} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				control := &lightFileControl{}
				err := &cluster.RemoteError{StatusCode: status}
				for attempt, expected := range []time.Duration{
					time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
					16 * time.Second, 32 * time.Second, 64 * time.Second, 128 * time.Second,
					256 * time.Second, 5 * time.Minute, 5 * time.Minute,
				} {
					if delay := control.retryDelay(err); delay != expected {
						t.Fatalf("unsupported retry %d = %v, want %v", attempt, delay, expected)
					}
				}
				control.acceptRelayResponse(cluster.FileRelayPollResponse{Epoch: strings.Repeat("a", 32)})
				started := time.Now()
				if !waitContext(context.Background(), control.retryDelay(err)) {
					t.Fatal("reconnect wait unexpectedly canceled")
				}
				if elapsed := time.Since(started); elapsed != time.Second {
					t.Fatalf("temporary HTTP %d silenced a working broker for %v", status, elapsed)
				}
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				if waitContext(ctx, control.retryDelay(err)) {
					t.Fatal("broker shutdown must cancel reconnect waiting")
				}
			})
		})
	}
}

func TestLightFileTransientRetryBacksOffAndResets(t *testing.T) {
	control := &lightFileControl{}
	err := &cluster.RemoteError{StatusCode: http.StatusBadGateway}
	for attempt, expected := range []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, time.Minute, time.Minute} {
		if delay := control.retryDelay(err); delay != expected {
			t.Fatalf("retry %d = %v, want %v", attempt, delay, expected)
		}
	}
	control.acceptRelayResponse(cluster.FileRelayPollResponse{Epoch: strings.Repeat("a", 32)})
	if delay := control.retryDelay(err); delay != time.Second {
		t.Fatalf("first failure after recovery = %v", delay)
	}
}

func TestLightFileRequestSessionLimitRejectsOverflow(t *testing.T) {
	control := &lightFileControl{
		ctx:       context.Background(),
		sessions:  make(map[string]*lightNodeFileSession, cluster.MaxFileRelayRequests),
		processed: make(map[string]cluster.FileRelayEvent),
	}
	for index := 0; index < cluster.MaxFileRelayRequests; index++ {
		id := fmt.Sprintf("%032x", index)
		control.sessions[id] = &lightNodeFileSession{
			requestID: id,
			ctx:       context.Background(),
			events:    make(chan cluster.FileRelayEvent, 1),
		}
	}
	command := cluster.FileRelayCommand{
		ID: strings.Repeat("a", 32), RequestID: strings.Repeat("b", 32),
		Kind: "request", Method: http.MethodGet, Path: "/v1/files/entry",
	}
	err := control.startRequest(command)
	if err == nil {
		t.Fatal("request above the active session limit was accepted")
	}
	control.queueCommandError(command, err)
	if len(control.sessions) != cluster.MaxFileRelayRequests {
		t.Fatalf("active requests = %d, want bounded limit %d", len(control.sessions), cluster.MaxFileRelayRequests)
	}
	if _, exists := control.sessions[command.RequestID]; exists {
		t.Fatal("overflow request retained a session")
	}
	if len(control.pendingEvents) != 1 || control.pendingEvents[0].Kind != "error" ||
		!control.pollPayloadFits(control.pendingEvents, control.requestIDs()) {
		t.Fatalf("overflow rejection cannot be returned in a valid poll: %#v", control.pendingEvents)
	}
}
