package ai

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStoreCreateRunConcurrentSessions(t *testing.T) {
	for _, shared := range []bool{false, true} {
		name := "independent"
		if shared {
			name = "shared"
		}
		t.Run(name, func(t *testing.T) {
			s, _, p, m := runtimeFixture(t)
			defer s.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			const workers = 16
			sessions := make([]Session, workers)
			for i := range sessions {
				if shared && i > 0 {
					sessions[i] = sessions[0]
					continue
				}
				var err error
				sessions[i], err = s.CreateSession(ctx, Session{UserID: "admin", ProviderID: p.ID, ProviderName: p.Name, ModelID: m.ID, ModelName: m.DisplayName})
				if err != nil {
					t.Fatal(err)
				}
			}
			type result struct {
				run Run
				err error
			}
			start := make(chan struct{})
			results := make(chan result, workers)
			for _, session := range sessions {
				go func() {
					<-start
					run, err := s.CreateRun(ctx, Run{SessionID: session.ID, UserID: "admin", ProviderID: p.ID, ModelID: m.ID})
					results <- result{run, err}
				}()
			}
			close(start)
			successes, busy := 0, 0
			for range workers {
				got := <-results
				switch {
				case got.err == nil:
					successes++
					if got.run.Status != RunQueued {
						t.Errorf("new run status = %s", got.run.Status)
					}
				case shared && errors.Is(got.err, ErrBusy):
					busy++
				default:
					t.Errorf("concurrent CreateRun: %v", got.err)
				}
			}
			wantSuccesses, wantBusy := workers, 0
			if shared {
				wantSuccesses, wantBusy = 1, workers-1
			}
			if successes != wantSuccesses || busy != wantBusy {
				t.Fatalf("admission results: successes=%d busy=%d, want %d/%d", successes, busy, wantSuccesses, wantBusy)
			}
		})
	}
}
