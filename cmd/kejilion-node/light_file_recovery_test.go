package main

import (
	"context"
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
				if elapsed := time.Since(started); elapsed != 5*time.Second {
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
