package remotedownload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AdaptivePolicy makes one bounded trial per transfer. Measurements include
// durable receiver writes, not merely bytes arriving from the origin.
type AdaptivePolicy struct {
	Mode        string
	bytes       int64
	read, write time.Duration
	baseline    float64
	tried       bool
}

func (p *AdaptivePolicy) Observe(bytes, remaining int64, read, write time.Duration) bool {
	if p.Mode == "" {
		p.Mode = "single"
	}
	p.bytes += bytes
	p.read += read
	p.write += write
	if p.bytes < 16<<20 || p.read+p.write <= 0 {
		return false
	}
	rate := float64(p.bytes) / (p.read + p.write).Seconds()
	mode := p.Mode
	switch p.Mode {
	case "single":
		if !p.tried && remaining >= 32<<20 && p.read >= time.Second && p.read > 2*p.write && float64(remaining)/rate >= 3 {
			p.baseline, p.tried, p.Mode = rate, true, "probing"
		}
	case "probing":
		if rate >= p.baseline*1.15 && p.read > p.write {
			p.Mode = "parallel"
		} else {
			p.Mode = "single"
		}
	case "parallel":
		if remaining >= 16<<20 && (rate < p.baseline*1.05 || p.write > 2*p.read) {
			p.Mode = "single"
		}
	}
	p.bytes, p.read, p.write = 0, 0, 0
	return mode != p.Mode
}

// AdaptiveBody swaps sources only after the consumer confirms a durable offset.
// Two global slots bound additional read-ahead to 8 MiB across all tasks.
type AdaptiveBody struct {
	ctx       context.Context
	cancel    context.CancelFunc
	client    *Client
	raw       string
	identity  ResumeRequest
	mu        sync.Mutex
	body      io.ReadCloser
	closed    bool
	switching bool
	policy    AdaptivePolicy
	budget    chan struct{}
	held      bool
	offset    int64
	overhead  time.Duration
}

func (c *Client) Adaptive(ctx context.Context, raw string, response *http.Response, budget chan struct{}) io.ReadCloser {
	if response.ContentLength < 64<<20 || strings.ToLower(strings.TrimSpace(response.Header.Get("Accept-Ranges"))) != "bytes" || !StrongETag(response.Header.Get("ETag")) || response.Request == nil || response.Request.URL == nil {
		return response.Body
	}
	ctx, cancel := context.WithCancel(ctx)
	return &AdaptiveBody{ctx: ctx, cancel: cancel, client: c, raw: raw, body: response.Body, budget: budget,
		identity: ResumeRequest{SizeBytes: response.ContentLength, ETag: response.Header.Get("ETag"), FinalURL: response.Request.URL.String()},
		policy:   AdaptivePolicy{Mode: "single"}}
}

func (b *AdaptiveBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	body, closed := b.body, b.closed
	b.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	return body.Read(p)
}

func (b *AdaptiveBody) ReadAheadBytes() int { return 2 * segmentBytes }

func (b *AdaptiveBody) TransferMode() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.policy.Mode
}

func (b *AdaptiveBody) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	body := b.body
	held := b.held && !b.switching
	if held {
		b.held = false
	}
	b.mu.Unlock()
	b.cancel()
	err := body.Close()
	if held {
		<-b.budget
	}
	return err
}

func (b *AdaptiveBody) releaseLocked() {
	if b.held {
		<-b.budget
		b.held = false
	}
}

func (b *AdaptiveBody) Checkpoint(offset int64, prefix string, read, write time.Duration) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return io.ErrClosedPipe
	}
	if offset <= b.offset || offset >= b.identity.SizeBytes {
		b.mu.Unlock()
		return ErrSourceChanged
	}
	old := b.policy.Mode
	changed := b.policy.Observe(offset-b.offset, b.identity.SizeBytes-offset, read+b.overhead, write)
	b.offset, b.overhead = offset, 0
	if !changed || old == "probing" && b.policy.Mode == "parallel" {
		b.mu.Unlock()
		return nil
	}
	parallel := b.policy.Mode == "probing"
	if parallel {
		select {
		case b.budget <- struct{}{}:
			b.held = true
		default:
			b.policy.Mode = "single"
			b.mu.Unlock()
			return nil
		}
	}
	previous := b.body
	b.switching = true
	b.mu.Unlock()
	started := time.Now()
	identity := b.identity
	identity.Offset = offset
	var response *http.Response
	var err error
	if parallel {
		response, err = b.client.OpenSegmentedRange(b.ctx, b.raw, identity, 2)
	} else {
		response, err = b.client.OpenRange(b.ctx, b.raw, identity)
	}
	if parallel && err != nil && b.ctx.Err() == nil && !errors.Is(err, ErrSourceChanged) && !errors.Is(err, ErrPartialContent) && !errors.Is(err, ErrEncoding) {
		// Keep the original stream at the durable offset until a probe opens.
		// Temporary range failures must not destroy a still usable download.
		b.mu.Lock()
		defer b.mu.Unlock()
		b.switching = false
		b.releaseLocked()
		b.policy.Mode = "single"
		b.overhead = time.Since(started)
		if b.closed {
			return io.ErrClosedPipe
		}
		return nil
	}
	_ = previous.Close()
	if err == nil && response.StatusCode == http.StatusOK {
		if response.ContentLength != identity.SizeBytes {
			err = ErrSourceChanged
		} else {
			hash := sha256.New()
			var written int64
			written, err = io.CopyBuffer(hash, io.LimitReader(response.Body, offset), make([]byte, 64<<10))
			if err == nil && (written != offset || hex.EncodeToString(hash.Sum(nil)) != prefix) {
				err = ErrSourceChanged
			}
		}
		parallel = false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.switching = false
	if err != nil || b.closed {
		if response != nil {
			_ = response.Body.Close()
		}
		b.releaseLocked()
		if err != nil {
			return err
		}
		return io.ErrClosedPipe
	}
	b.body = response.Body
	if !parallel {
		b.policy.Mode = "single"
		b.releaseLocked()
	}
	b.overhead = time.Since(started)
	return nil
}
