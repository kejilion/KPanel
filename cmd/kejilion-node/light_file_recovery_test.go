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
				if control.retryDelay(err) != 5*time.Minute {
					t.Fatal("legacy unsupported peers should retain slow discovery")
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
