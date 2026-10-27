package panel

import (
	"log/slog"
	"sync"
	"time"
)

// Diagnostics are sampled per server/listener, not per attacker-controlled key.
// Never include request paths, queries, headers, bodies or upstream error text.
type failureDiagnostics struct {
	mu         sync.Mutex
	next       time.Time
	suppressed uint64
}

func (d *failureDiagnostics) record(layer, code, id string, status int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	if now.Before(d.next) {
		d.suppressed++
		return
	}
	slog.Warn("panel request failure", "layer", layer, "code", code,
		"status", status, "request_id", id, "suppressed_since_last", d.suppressed)
	d.next = now.Add(time.Second)
	d.suppressed = 0
}
