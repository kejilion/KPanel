package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

func awaitProcessReaders(t *testing.T, reads *processReads, count int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		reads.mu.Lock()
		n := 0
		if reads.active != nil {
			n = reads.active.waiters
		}
		reads.mu.Unlock()
		if n == count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("process readers did not reach %d", count)
}

func TestProcessReadsShareOnlyIdenticalInflightAndDoNotCache(t *testing.T) {
	var reads processReads
	t.Cleanup(reads.close)
	key := processReadKey{query: systeminfo.ProcessQuery{Sort: "cpu", Order: "desc", Limit: 200}}
	release := make(chan struct{})
	var calls atomic.Int32
	collect := func(ctx context.Context) (systeminfo.ProcessSnapshot, error) {
		n := calls.Add(1)
		select {
		case <-release:
			return systeminfo.ProcessSnapshot{Scanned: int(n)}, nil
		case <-ctx.Done():
			return systeminfo.ProcessSnapshot{}, ctx.Err()
		}
	}
	results := make(chan systeminfo.ProcessSnapshot, maxProcessReadWaiters)
	errs := make(chan error, maxProcessReadWaiters)
	for i := 0; i < maxProcessReadWaiters; i++ {
		go func() {
			result, err := reads.read(context.Background(), key, collect)
			results <- result
			errs <- err
		}()
	}
	awaitProcessReaders(t, &reads, maxProcessReadWaiters)
	for _, other := range []processReadKey{key, {legacy: true}, {query: systeminfo.ProcessQuery{Search: "other"}}} {
		if _, err := reads.read(context.Background(), other, collect); !errors.Is(err, errProcessReadBusy) {
			t.Fatalf("overflow/different query error = %v", err)
		}
	}
	close(release)
	for i := 0; i < maxProcessReadWaiters; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		if result := <-results; result.Scanned != 1 {
			t.Fatalf("in-flight result = %#v", result)
		}
	}
	result, err := reads.read(context.Background(), key, collect)
	if err != nil || result.Scanned != 2 || calls.Load() != 2 {
		t.Fatalf("completed result reused: %#v, %v, calls=%d", result, err, calls.Load())
	}
}

func TestProcessReadsCancellationIsPerCallerAndNeverOverlapsCollectors(t *testing.T) {
	var reads processReads
	t.Cleanup(reads.close)
	key := processReadKey{legacy: true}
	firstCtx, firstCancel := context.WithCancel(context.Background())
	secondCtx, secondCancel := context.WithCancel(context.Background())
	t.Cleanup(firstCancel)
	t.Cleanup(secondCancel)
	sharedCanceled := make(chan struct{})
	allowExit := make(chan struct{})
	t.Cleanup(func() { close(allowExit) })
	collect := func(ctx context.Context) (systeminfo.ProcessSnapshot, error) {
		<-ctx.Done()
		close(sharedCanceled)
		<-allowExit // A slow cancellation must still occupy the sampling slot.
		return systeminfo.ProcessSnapshot{}, ctx.Err()
	}
	firstErr := make(chan error, 1)
	secondErr := make(chan error, 1)
	go func() { _, err := reads.read(firstCtx, key, collect); firstErr <- err }()
	awaitProcessReaders(t, &reads, 1)
	go func() { _, err := reads.read(secondCtx, key, collect); secondErr <- err }()
	awaitProcessReaders(t, &reads, 2)
	firstCancel()
	if !errors.Is(<-firstErr, context.Canceled) {
		t.Fatal("first client did not cancel")
	}
	select {
	case <-sharedCanceled:
		t.Fatal("first client canceled another client's sample")
	default:
	}
	secondCancel()
	if !errors.Is(<-secondErr, context.Canceled) {
		t.Fatal("second client did not cancel")
	}
	select {
	case <-sharedCanceled:
	case <-time.After(time.Second):
		t.Fatal("last client did not cancel collection")
	}
	if _, err := reads.read(context.Background(), key, collect); !errors.Is(err, errProcessReadBusy) {
		t.Fatalf("new collector overlapped canceled collector: %v", err)
	}
}

func TestProcessReadsFailureIsNotCachedAndServerCloseCancels(t *testing.T) {
	var reads processReads
	key := processReadKey{legacy: true}
	failure := errors.New("probe unavailable")
	_, err := reads.read(context.Background(), key, func(context.Context) (systeminfo.ProcessSnapshot, error) {
		return systeminfo.ProcessSnapshot{}, failure
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		_, err := reads.read(context.Background(), key, func(ctx context.Context) (systeminfo.ProcessSnapshot, error) {
			<-ctx.Done()
			return systeminfo.ProcessSnapshot{}, ctx.Err()
		})
		errCh <- err
	}()
	awaitProcessReaders(t, &reads, 1)
	reads.close()
	if !errors.Is(<-errCh, context.Canceled) {
		t.Fatal("close did not cancel collection")
	}
	if _, err := reads.read(context.Background(), key, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("closed sampler accepted work: %v", err)
	}
}

func TestProcessReadsWriteBoundaryPreventsJoiningOlderSample(t *testing.T) {
	var reads processReads
	t.Cleanup(reads.close)
	key := processReadKey{legacy: true}
	release := make(chan struct{})
	resultCh := make(chan error, 1)
	go func() {
		_, err := reads.read(context.Background(), key, func(ctx context.Context) (systeminfo.ProcessSnapshot, error) {
			select {
			case <-release:
				return systeminfo.ProcessSnapshot{}, nil
			case <-ctx.Done():
				return systeminfo.ProcessSnapshot{}, ctx.Err()
			}
		})
		resultCh <- err
	}()
	awaitProcessReaders(t, &reads, 1)
	reads.seal()
	if _, err := reads.read(context.Background(), key, nil); !errors.Is(err, errProcessReadBusy) {
		t.Fatalf("post-write read joined older sample: %v", err)
	}
	close(release)
	if err := <-resultCh; err != nil {
		t.Fatalf("existing reader was disrupted: %v", err)
	}
	_, err := reads.read(context.Background(), key, func(context.Context) (systeminfo.ProcessSnapshot, error) {
		return systeminfo.ProcessSnapshot{}, nil
	})
	if err != nil {
		t.Fatalf("new observation blocked after old sample finished: %v", err)
	}
}

func TestProcessReadsHaveIndependentThreeSecondDeadline(t *testing.T) {
	var reads processReads
	t.Cleanup(reads.close)
	_, err := reads.read(context.Background(), processReadKey{legacy: true}, func(ctx context.Context) (systeminfo.ProcessSnapshot, error) {
		deadline, ok := ctx.Deadline()
		remaining := time.Until(deadline)
		if !ok || remaining <= 0 || remaining > 3*time.Second {
			t.Errorf("shared deadline missing or unbounded: %s", remaining)
		}
		return systeminfo.ProcessSnapshot{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestProcessReadsTimeoutReleasesSlot(t *testing.T) {
	var reads processReads
	t.Cleanup(reads.close)
	_, err := reads.read(context.Background(), processReadKey{legacy: true}, func(ctx context.Context) (systeminfo.ProcessSnapshot, error) {
		<-ctx.Done()
		return systeminfo.ProcessSnapshot{}, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed out sample = %v", err)
	}
	_, err = reads.read(context.Background(), processReadKey{legacy: true}, func(context.Context) (systeminfo.ProcessSnapshot, error) {
		return systeminfo.ProcessSnapshot{}, nil
	})
	if err != nil {
		t.Fatalf("timeout left slot occupied: %v", err)
	}
}
