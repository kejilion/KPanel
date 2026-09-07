package appmarket

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
)

type blockedImageUpdateLookup struct {
	*fakeDocker
	started chan struct{}
}

func (d *blockedImageUpdateLookup) ContainersForImageUpdate(ctx context.Context) ([]contract.ContainerSummary, error) {
	d.started <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestImageUpdateBudgetIncludesAppResolutionAndSharesDockerSlots(t *testing.T) {
	d := &blockedImageUpdateLookup{fakeDocker: &fakeDocker{}, started: make(chan struct{}, 3)}
	s, err := New(d, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 2)
	for range 2 {
		go func() { _, err := s.CheckUpdate(ctx, "builtin-28", "v"); done <- err }()
	}
	for range 2 {
		select {
		case <-d.started:
		case <-time.After(3 * time.Second):
			t.Fatal("lookup never started")
		}
	}
	if _, err := s.CheckUpdate(ctx, "builtin-28", "v"); !errors.Is(err, dockerx.ErrImageUpdateBusy) {
		t.Fatalf("third lookup escaped budget: %v", err)
	}
	client := dockerx.New("/nonexistent-test-engine.sock", t.TempDir(), t.TempDir())
	if _, err := client.CheckContainerImageUpdate(ctx, strings.Repeat("a", 64), "v"); !errors.Is(err, dockerx.ErrImageUpdateBusy) {
		t.Fatalf("Docker did not share app lookup slots: %v", err)
	}
	cancel()
	for range 2 {
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("cancel leaked lookup")
		}
	}
	_, err = dockerx.WithImageUpdateCheck(context.Background(), func(ctx context.Context) (dockerx.ImageUpdateResult, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("missing total deadline")
		}
		return dockerx.WithImageUpdateCheck(ctx, func(nested context.Context) (dockerx.ImageUpdateResult, error) {
			nestedDeadline, ok := nested.Deadline()
			if !ok || !deadline.Equal(nestedDeadline) {
				t.Fatal("nested check reset deadline")
			}
			return dockerx.ImageUpdateResult{Status: "current"}, nil
		})
	})
	if err != nil {
		t.Fatalf("cancellation leaked shared slots: %v", err)
	}
}
