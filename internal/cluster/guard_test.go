package cluster

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestReplayGuardIsolatesPrincipalCapacityAndBoundsGlobalEntries(t *testing.T) {
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)
	guard := newReplayGuard(4, 2, 5*time.Minute)

	for _, nonce := range []string{"a1", "a2"} {
		if err := guard.Accept("light:node-a", nonce, now); err != nil {
			t.Fatalf("Accept(node-a, %q) error: %v", nonce, err)
		}
	}
	if err := guard.Accept("light:node-a", "a1", now); !errors.Is(err, ErrReplay) {
		t.Fatalf("duplicate at principal capacity error = %v, want ErrReplay", err)
	}
	if err := guard.Accept("light:node-a", "a3", now); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("over-quota principal error = %v, want ErrRateLimited", err)
	}

	for _, nonce := range []string{"b1", "b2"} {
		if err := guard.Accept("light:node-b", nonce, now); err != nil {
			t.Fatalf("Accept(node-b, %q) should remain available: %v", nonce, err)
		}
	}
	if err := guard.Accept("light:node-c", "c1", now); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("over-capacity global cache error = %v, want ErrRateLimited", err)
	}

	if err := guard.Accept("light:node-c", "c1", now.Add(5*time.Minute)); err != nil {
		t.Fatalf("expired entries should free global and principal capacity: %v", err)
	}
}

func TestReplayGuardCheckDoesNotConsumeNonce(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	guard := newReplayGuard(2, 1, 5*time.Minute)
	for range 2 {
		if err := guard.Check("controller:a", "nonce-1", now); err != nil {
			t.Fatalf("Check() before Accept error = %v", err)
		}
	}
	if err := guard.Accept("controller:a", "nonce-1", now); err != nil {
		t.Fatalf("Accept() after read-only checks error = %v", err)
	}
	if err := guard.Check("controller:a", "nonce-1", now); !errors.Is(err, ErrReplay) {
		t.Fatalf("Check() accepted an existing nonce: %v", err)
	}
	if err := guard.Check("controller:b", "nonce-2", now); err != nil {
		t.Fatalf("Check() rejected available global capacity: %v", err)
	}
	if err := guard.Accept("controller:b", "nonce-2", now); err != nil {
		t.Fatalf("Accept() after Check() error = %v", err)
	}
	if err := guard.Check("controller:c", "nonce-3", now); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Check() did not report full global capacity: %v", err)
	}
}

func TestReplayGuardProductionPrincipalLimitLeavesOtherNodesAvailable(t *testing.T) {
	now := time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC)
	guard := newReplayGuard(replayGuardTotalEntries, replayGuardEntriesPerPrincipal, 5*time.Minute)
	principal := lightNodePrincipal("node-a")

	for i := 0; i < replayGuardEntriesPerPrincipal; i++ {
		if err := guard.Accept(principal, strconv.Itoa(i), now); err != nil {
			t.Fatalf("Accept(node-a, entry %d) error before configured principal limit: %v", i, err)
		}
	}
	if err := guard.Accept(principal, "over-limit", now); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("over-limit light principal error = %v, want ErrRateLimited", err)
	}
	if err := guard.Accept(lightNodePrincipal("node-b"), "first", now); err != nil {
		t.Fatalf("a full light principal must leave another node available: %v", err)
	}
}

func TestPrincipalNamespacesIsolateReplayAndRateLimits(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	id := "same-text-controller-id"
	principals := []string{
		controllerV1Principal(id),
		controllerV2Principal(id),
		lightNodePrincipal(id),
		linkedPeerPrincipal(id),
	}
	guard := newReplayGuard(len(principals), 1, 5*time.Minute)
	limiter := newFixedWindowLimiter(1, time.Minute, len(principals))
	seen := make(map[string]struct{}, len(principals))
	for _, principal := range principals {
		if _, ok := seen[principal]; ok {
			t.Fatalf("principal namespace collision for %q", principal)
		}
		seen[principal] = struct{}{}
		if err := guard.Accept(principal, "same-nonce", now); err != nil {
			t.Fatalf("Accept(%q, shared nonce) error = %v, want isolated principal capacity", principal, err)
		}
		if !limiter.Allow(principal, now) {
			t.Fatalf("Allow(%q) = false, want isolated rate-limit capacity", principal)
		}
	}
	for _, principal := range principals {
		if err := guard.Accept(principal, "next-nonce", now); !errors.Is(err, ErrRateLimited) {
			t.Fatalf("Accept(%q, over-quota) error = %v, want ErrRateLimited", principal, err)
		}
		if limiter.Allow(principal, now) {
			t.Fatalf("Allow(%q) exceeded its independent rate limit", principal)
		}
	}
}
