package cluster

import (
	"container/heap"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"
)

const (
	// One light-node principal can pass at most 4,680 replay-protected requests
	// per minute across the current independent route limiters. This budget
	// covers five minutes of that configured maximum, with a small margin.
	replayGuardEntriesPerPrincipal = 24_576
	// Keep the cache bounded while allowing more than one principal to reach its
	// own budget without denying every other authenticated principal.
	replayGuardTotalEntries = 65_536
)

type replayKey [sha256.Size]byte

type replayEntry struct {
	principal string
	expiresAt time.Time
}

type replayPrincipalCount struct {
	identity string
	count    int
}

type replayExpiry struct {
	key       replayKey
	expiresAt time.Time
}

type replayExpiryHeap []replayExpiry

func (h replayExpiryHeap) Len() int           { return len(h) }
func (h replayExpiryHeap) Less(i, j int) bool { return h[i].expiresAt.Before(h[j].expiresAt) }
func (h replayExpiryHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *replayExpiryHeap) Push(value any) {
	*h = append(*h, value.(replayExpiry))
}

func (h *replayExpiryHeap) Pop() any {
	old := *h
	last := len(old) - 1
	value := old[last]
	old[last] = replayExpiry{}
	*h = old[:last]
	return value
}

type replayGuard struct {
	mu                   sync.Mutex
	entries              map[replayKey]replayEntry
	principalEntries     map[string]replayPrincipalCount
	expiries             replayExpiryHeap
	capacity             int
	perPrincipalCapacity int
	ttl                  time.Duration
}

func newReplayGuard(capacity, perPrincipalCapacity int, ttl time.Duration) *replayGuard {
	if capacity < 1 {
		capacity = 1
	}
	if perPrincipalCapacity < 1 || perPrincipalCapacity > capacity {
		perPrincipalCapacity = capacity
	}
	return &replayGuard{
		entries:              make(map[replayKey]replayEntry),
		principalEntries:     make(map[string]replayPrincipalCount),
		capacity:             capacity,
		perPrincipalCapacity: perPrincipalCapacity,
		ttl:                  ttl,
	}
}

func controllerV1Principal(controllerID string) string {
	return "controller:v1:" + controllerID
}

func controllerV2Principal(controllerID string) string {
	return "controller:v2:" + controllerID
}

func lightNodePrincipal(nodeID string) string {
	return "light:" + nodeID
}

func linkedPeerPrincipal(nodeID string) string {
	return "linked-peer:" + nodeID
}

func replayCacheKey(principal, nonce string) replayKey {
	input := make([]byte, 16+len(principal)+len(nonce))
	binary.BigEndian.PutUint64(input[:8], uint64(len(principal)))
	copy(input[8:], principal)
	offset := 8 + len(principal)
	binary.BigEndian.PutUint64(input[offset:offset+8], uint64(len(nonce)))
	copy(input[offset+8:], nonce)
	return sha256.Sum256(input)
}

// Check rejects a nonce that has already been accepted, or a request that
// cannot be admitted to the replay cache, without consuming the nonce. It is
// useful before expensive request-body work; Accept remains the authoritative
// atomic insertion after the complete request has been verified.
func (g *replayGuard) Check(principal, nonce string, now time.Time) error {
	key := replayCacheKey(principal, nonce)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pruneLocked(now)
	if _, exists := g.entries[key]; exists {
		return ErrReplay
	}
	principalCount := g.principalEntries[principal]
	if len(g.entries) >= g.capacity || principalCount.count >= g.perPrincipalCapacity {
		return ErrRateLimited
	}
	return nil
}

func (g *replayGuard) Accept(principal, nonce string, now time.Time) error {
	key := replayCacheKey(principal, nonce)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pruneLocked(now)
	if _, exists := g.entries[key]; exists {
		return ErrReplay
	}
	principalCount, hasPrincipal := g.principalEntries[principal]
	if hasPrincipal {
		// Route helpers create prefixed strings per request. Reuse one immutable
		// identity string for all live entries to keep cache memory predictable.
		principal = principalCount.identity
	}
	if len(g.entries) >= g.capacity ||
		principalCount.count >= g.perPrincipalCapacity {
		return ErrRateLimited
	}
	expiresAt := now.Add(g.ttl)
	g.entries[key] = replayEntry{principal: principal, expiresAt: expiresAt}
	principalCount.identity = principal
	principalCount.count++
	g.principalEntries[principal] = principalCount
	heap.Push(&g.expiries, replayExpiry{key: key, expiresAt: expiresAt})
	return nil
}

func (g *replayGuard) pruneLocked(now time.Time) {
	for len(g.expiries) > 0 && !g.expiries[0].expiresAt.After(now) {
		expired := heap.Pop(&g.expiries).(replayExpiry)
		entry, exists := g.entries[expired.key]
		if !exists || !entry.expiresAt.Equal(expired.expiresAt) {
			continue
		}
		delete(g.entries, expired.key)
		principalCount := g.principalEntries[entry.principal]
		if principalCount.count <= 1 {
			delete(g.principalEntries, entry.principal)
		} else {
			principalCount.count--
			g.principalEntries[entry.principal] = principalCount
		}
	}
}

type fixedWindowLimiter struct {
	mu          sync.Mutex
	entries     map[string]windowCount
	limit       int
	window      time.Duration
	maxSubjects int
}

type windowCount struct {
	start time.Time
	count int
}

func newFixedWindowLimiter(limit int, window time.Duration, maxSubjects int) *fixedWindowLimiter {
	return &fixedWindowLimiter{
		entries:     make(map[string]windowCount),
		limit:       limit,
		window:      window,
		maxSubjects: maxSubjects,
	}
}

func (l *fixedWindowLimiter) Allow(subject string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	current, ok := l.entries[subject]
	if !ok || now.Sub(current.start) >= l.window {
		if !ok && len(l.entries) >= l.maxSubjects {
			l.pruneLocked(now)
			if len(l.entries) >= l.maxSubjects {
				return false
			}
		}
		l.entries[subject] = windowCount{start: now, count: 1}
		return true
	}
	if current.count >= l.limit {
		return false
	}
	current.count++
	l.entries[subject] = current
	return true
}

func (l *fixedWindowLimiter) pruneLocked(now time.Time) {
	for key, current := range l.entries {
		if now.Sub(current.start) >= l.window {
			delete(l.entries, key)
		}
	}
}
