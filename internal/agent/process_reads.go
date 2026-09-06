package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/systeminfo"
)

const maxProcessReadWaiters = 8

var errProcessReadBusy = errors.New("process sampling is busy")

type processReadKey struct {
	query  systeminfo.ProcessQuery
	legacy bool
}

type processReadCall struct {
	key     processReadKey
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int
	sealed  bool
	result  systeminfo.ProcessSnapshot
	err     error
}

// processReads shares only an unfinished identical observation. There is no
// completed-result cache, no query queue and never more than one collector.
type processReads struct {
	mu     sync.Mutex
	active *processReadCall
	closed bool
}

func (p *processReads) read(ctx context.Context, key processReadKey, collect func(context.Context) (systeminfo.ProcessSnapshot, error)) (systeminfo.ProcessSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return systeminfo.ProcessSnapshot{}, err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return systeminfo.ProcessSnapshot{}, context.Canceled
	}
	call := p.active
	if call != nil {
		if call.key != key || call.sealed || call.waiters >= maxProcessReadWaiters || call.ctx.Err() != nil {
			p.mu.Unlock()
			return systeminfo.ProcessSnapshot{}, errProcessReadBusy
		}
		call.waiters++
	} else {
		// One client leaving must not cancel another client's observation.
		shared, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		call = &processReadCall{key: key, ctx: shared, cancel: cancel, done: make(chan struct{}), waiters: 1}
		p.active = call
		go func() {
			result, err := collect(shared)
			p.mu.Lock()
			call.result, call.err = result, err
			p.active = nil
			close(call.done)
			call.cancel()
			p.mu.Unlock()
		}()
	}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		call.waiters--
		if call.waiters == 0 {
			// Keep active until the collector actually exits, even after cancel.
			call.cancel()
		}
		p.mu.Unlock()
	}()
	select {
	case <-ctx.Done():
		return systeminfo.ProcessSnapshot{}, ctx.Err()
	case <-call.done:
		return call.result, call.err
	}
}

// A write boundary prevents a later read joining a pre-write observation.
// Existing readers finish normally; new readers retain the original busy path.
func (p *processReads) seal() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.active != nil {
		p.active.sealed = true
	}
}

func (p *processReads) close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	if p.active != nil {
		p.active.cancel()
	}
}
