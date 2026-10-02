package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

// delayProxy relays TCP between the benchmark's browser models and the Panel,
// delaying each direction without limiting bandwidth. It stands in for a
// long-haul link on loopback and counts what crosses it.
type delayProxy struct {
	addr   string
	target string
	oneWay time.Duration
	up     atomic.Int64
	down   atomic.Int64
}

func newDelayProxy(t *testing.T, target string, oneWay time.Duration) *delayProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &delayProxy{addr: listener.Addr().String(), target: target, oneWay: oneWay}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			go p.relay(client)
		}
	}()
	return p
}

func (p *delayProxy) relay(client net.Conn) {
	upstream, err := net.Dial("tcp", p.target)
	if err != nil {
		client.Close()
		return
	}
	pump := func(src, dst net.Conn, counter *atomic.Int64) {
		type chunk struct {
			data []byte
			at   time.Time
		}
		queue := make(chan chunk, 8192)
		go func() {
			for c := range queue {
				time.Sleep(time.Until(c.at))
				if _, err := dst.Write(c.data); err != nil {
					break
				}
			}
			dst.Close()
		}()
		buffer := make([]byte, 64<<10)
		for {
			n, err := src.Read(buffer)
			if n > 0 {
				counter.Add(int64(n))
				queue <- chunk{data: append([]byte(nil), buffer[:n]...), at: time.Now().Add(p.oneWay)}
			}
			if err != nil {
				close(queue)
				return
			}
		}
	}
	go pump(client, upstream, &p.up)
	pump(upstream, client, &p.down)
}

func (p *delayProxy) counts() (up, down int64) { return p.up.Load(), p.down.Load() }

type jobBenchRig struct {
	t      *testing.T
	f      *jobTerminalFixture
	proxy  *delayProxy
	client *http.Client
	kind   string
	nextID atomic.Int64
}

func newJobBenchRig(t *testing.T, f *jobTerminalFixture, oneWay time.Duration) *jobBenchRig {
	proxy := newDelayProxy(t, f.http.Listener.Addr().String(), oneWay)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, proxy.addr)
		},
		MaxIdleConnsPerHost: 4,
		DisableCompression:  true,
	}
	t.Cleanup(transport.CloseIdleConnections)
	return &jobBenchRig{t: t, f: f, proxy: proxy, client: &http.Client{Transport: transport}, kind: "app"}
}

// browserHeaders are what a browser sends with every request; they are part of
// the per-request cost the old route paid and the socket pays once.
func (r *jobBenchRig) browserHeaders(h http.Header) {
	h.Set("Cookie", r.f.session.String()+"; "+r.f.csrf.String())
	h.Set("Origin", "http://panel.test")
	h.Set("X-CSRF-Token", r.f.csrf.Value)
	h.Set("Content-Type", "application/json")
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	h.Set("Accept", "application/json, text/plain, */*")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	h.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	h.Set("Referer", "https://panel.example.com/apps")
	h.Set("sec-ch-ua", `"Chromium";v="130", "Google Chrome";v="130", "Not?A_Brand";v="99"`)
	h.Set("sec-ch-ua-mobile", "?0")
	h.Set("sec-ch-ua-platform", `"Windows"`)
	h.Set("sec-fetch-site", "same-origin")
	h.Set("sec-fetch-mode", "cors")
	h.Set("sec-fetch-dest", "empty")
}

func (r *jobBenchRig) post(ctx context.Context, path string, body any) (int, error) {
	data, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+r.proxy.addr+path, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	r.browserHeaders(request.Header)
	response, err := r.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode, nil
}

func (r *jobBenchRig) dial(ctx context.Context) (*websocket.Conn, error) {
	header := http.Header{}
	header.Set("Cookie", r.f.session.String()+"; "+r.f.csrf.String())
	header.Set("Origin", "http://panel.test")
	header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36")
	ws, _, err := websocket.Dial(ctx, "ws://"+r.proxy.addr+r.f.path(r.kind, "input-stream"), &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{terminalInputSocketProtocol}})
	return ws, err
}

// sinkBytes is what the task has received so far.
func (r *jobBenchRig) sinkBytes() int {
	r.f.agent.mu.Lock()
	defer r.f.agent.mu.Unlock()
	total := 0
	for _, write := range r.f.agent.writes {
		total += write.n
	}
	return total
}

func (r *jobBenchRig) newStream() string { return fmt.Sprintf("%032x", r.nextID.Add(1)) }

// legacyTyper is the task terminal before this change: input is coalesced for
// 12 ms and then drained one request at a time, each awaiting its response.
type legacyTyper struct {
	rig      *jobBenchRig
	mu       sync.Mutex
	queue    []byte
	timer    *time.Timer
	sending  bool
	requests atomic.Int64
	failed   atomic.Bool
}

func (l *legacyTyper) key(b byte) {
	l.mu.Lock()
	l.queue = append(l.queue, b)
	if len(l.queue) >= 2048 {
		l.mu.Unlock()
		l.flush()
		return
	}
	if l.timer == nil {
		l.timer = time.AfterFunc(12*time.Millisecond, l.flush)
	}
	l.mu.Unlock()
}

func (l *legacyTyper) paste(data []byte) {
	l.mu.Lock()
	l.queue = append(l.queue, data...)
	l.mu.Unlock()
	l.flush()
}

func (l *legacyTyper) flush() {
	l.mu.Lock()
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
	if l.sending {
		l.mu.Unlock()
		return
	}
	l.sending = true
	l.mu.Unlock()
	go l.drain()
}

func (l *legacyTyper) drain() {
	for {
		l.mu.Lock()
		if len(l.queue) == 0 {
			l.sending = false
			l.mu.Unlock()
			return
		}
		n := min(2048, len(l.queue))
		chunk := string(l.queue[:n])
		l.queue = l.queue[n:]
		l.mu.Unlock()
		l.requests.Add(1)
		status, err := l.rig.post(context.Background(), "/api/v1/app-jobs/"+jobTestID+"/input", map[string]string{"data": chunk})
		if err != nil || status != 200 {
			l.failed.Store(true)
		}
	}
}

func (l *legacyTyper) idle() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.queue) == 0 && !l.sending && l.timer == nil
}

func (l *legacyTyper) close() {}

// streamTyper is the task terminal after this change: the same coalescing, then
// frames of up to 2048 bytes sent without waiting, 32 in flight.
type streamTyper struct {
	rig      *jobBenchRig
	ws       *websocket.Conn
	stream   string
	cancel   context.CancelFunc
	sendMu   sync.Mutex
	mu       sync.Mutex
	queue    []byte
	timer    *time.Timer
	seq      uint64
	inflight int
	frames   atomic.Int64
	failed   atomic.Bool
}

func (r *jobBenchRig) newStreamTyper(ctx context.Context) (*streamTyper, error) {
	ws, err := r.dial(ctx)
	if err != nil {
		return nil, err
	}
	s := &streamTyper{rig: r, ws: ws, stream: r.newStream()}
	hello, _ := json.Marshal(map[string]string{"type": "auth", "csrf": r.f.csrf.Value, "stream": s.stream})
	if err := ws.Write(ctx, websocket.MessageText, hello); err != nil {
		return nil, err
	}
	_, data, err := ws.Read(ctx)
	var ready terminalInputReply
	if err != nil || json.Unmarshal(data, &ready) != nil || ready.Type != "ready" {
		return nil, fmt.Errorf("stream not ready: %s %v", data, err)
	}
	readCtx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		for {
			_, data, err := ws.Read(readCtx)
			if err != nil {
				return
			}
			var reply terminalInputReply
			if json.Unmarshal(data, &reply) != nil || reply.Type != "ack" {
				s.failed.Store(true)
				return
			}
			s.mu.Lock()
			s.inflight--
			s.mu.Unlock()
			s.pump()
		}
	}()
	return s, nil
}

func (s *streamTyper) pump() {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	for {
		s.mu.Lock()
		if s.inflight >= terminal.InputWindow || len(s.queue) == 0 {
			s.mu.Unlock()
			return
		}
		n := min(terminal.InputFrameBytes, len(s.queue))
		chunk := append([]byte(nil), s.queue[:n]...)
		s.queue = s.queue[n:]
		s.seq++
		frame := terminal.InputFrame{Stream: s.stream, Seq: s.seq, Data: chunk}
		s.inflight++
		s.mu.Unlock()
		s.frames.Add(1)
		message, _ := json.Marshal(terminalInputMessage{Type: "input", Frame: &frame})
		if err := s.ws.Write(context.Background(), websocket.MessageText, message); err != nil {
			s.failed.Store(true)
			return
		}
	}
}

func (s *streamTyper) key(b byte) {
	s.mu.Lock()
	s.queue = append(s.queue, b)
	if len(s.queue) >= terminal.InputFrameBytes {
		s.mu.Unlock()
		s.flush()
		return
	}
	if s.timer == nil {
		s.timer = time.AfterFunc(12*time.Millisecond, s.flush)
	}
	s.mu.Unlock()
}

func (s *streamTyper) paste(data []byte) {
	s.mu.Lock()
	s.queue = append(s.queue, data...)
	s.mu.Unlock()
	s.flush()
}

func (s *streamTyper) flush() {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	s.pump()
}

func (s *streamTyper) idle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue) == 0 && s.inflight == 0 && s.timer == nil
}

func (s *streamTyper) close() {
	s.cancel()
	s.ws.CloseNow()
}

type benchTyper interface {
	key(byte)
	paste([]byte)
	idle() bool
	close()
}

func waitIdle(t *testing.T, typer benchTyper) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for !typer.idle() {
		if time.Now().After(deadline) {
			t.Fatal("input never drained")
		}
		time.Sleep(time.Millisecond)
	}
}

func median(values []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

func percentile(values []time.Duration, q float64) time.Duration {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[min(len(sorted)-1, int(float64(len(sorted))*q))]
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

type benchMode string

const (
	modeLegacy benchMode = "legacy"
	modeStream benchMode = "stream"
)

func (r *jobBenchRig) typer(ctx context.Context, mode benchMode) benchTyper {
	r.t.Helper()
	if mode == modeLegacy {
		// A keep-alive connection that is already open, as in a used page.
		if status, err := r.post(ctx, "/api/v1/app-jobs/"+jobTestID+"/input", map[string]string{"data": ""}); err != nil || status == 0 {
			r.t.Fatalf("prime: %d %v", status, err)
		}
		return &legacyTyper{rig: r}
	}
	typer, err := r.newStreamTyper(ctx)
	if err != nil {
		r.t.Fatal(err)
	}
	return typer
}

func (r *jobBenchRig) failed(typer benchTyper) bool {
	switch t := typer.(type) {
	case *legacyTyper:
		return t.failed.Load()
	case *streamTyper:
		return t.failed.Load()
	}
	return false
}

func (r *jobBenchRig) requestsOf(typer benchTyper) int64 {
	switch t := typer.(type) {
	case *legacyTyper:
		return t.requests.Load()
	case *streamTyper:
		return t.frames.Load()
	}
	return 0
}

// singleKey is the latency of one key in an idle terminal, until the browser
// knows the task has it.
func (r *jobBenchRig) singleKey(ctx context.Context, mode benchMode, rounds int) time.Duration {
	typer := r.typer(ctx, mode)
	defer typer.close()
	var samples []time.Duration
	for i := 0; i < rounds; i++ {
		started := time.Now()
		typer.paste([]byte("x"))
		waitIdle(r.t, typer)
		samples = append(samples, time.Since(started))
		time.Sleep(60 * time.Millisecond)
	}
	if r.failed(typer) {
		r.t.Fatalf("%s single key failed", mode)
	}
	return median(samples)
}

type typingResult struct {
	p50, p95, max time.Duration
	units         int64
	wire          int64
}

// typing is a person typing steadily: one key every interval. Every key's
// latency is how long it took to reach the task.
func (r *jobBenchRig) typing(ctx context.Context, mode benchMode, keys int, interval time.Duration) typingResult {
	typer := r.typer(ctx, mode)
	defer typer.close()
	r.f.agent.mu.Lock()
	first := len(r.f.agent.writes)
	r.f.agent.mu.Unlock()
	up0, down0 := r.proxy.counts()
	units0 := r.requestsOf(typer)
	typed := make([]time.Time, keys)
	start := time.Now()
	for i := 0; i < keys; i++ {
		time.Sleep(time.Until(start.Add(time.Duration(i) * interval)))
		typed[i] = time.Now()
		typer.key(byte('a' + i%26))
	}
	waitIdle(r.t, typer)
	if r.failed(typer) {
		r.t.Fatalf("%s typing failed", mode)
	}
	r.f.agent.mu.Lock()
	writes := append([]jobSinkWrite(nil), r.f.agent.writes[first:]...)
	r.f.agent.mu.Unlock()
	var latencies []time.Duration
	index := 0
	for _, write := range writes {
		for n := 0; n < write.n && index < keys; n++ {
			latencies = append(latencies, write.at.Sub(typed[index]))
			index++
		}
	}
	if len(latencies) != keys {
		r.t.Fatalf("%s: %d of %d keys reached the task", mode, len(latencies), keys)
	}
	up1, down1 := r.proxy.counts()
	return typingResult{p50: percentile(latencies, 0.5), p95: percentile(latencies, 0.95), max: percentile(latencies, 1),
		units: r.requestsOf(typer) - units0, wire: (up1 - up0) + (down1 - down0)}
}

type pasteResult struct {
	total    time.Duration
	units    int64
	up, down int64
}

// paste is a block of text arriving at once; total is until the browser has an
// answer for all of it.
func (r *jobBenchRig) paste(ctx context.Context, mode benchMode, size int) pasteResult {
	typer := r.typer(ctx, mode)
	defer typer.close()
	up0, down0 := r.proxy.counts()
	units0 := r.requestsOf(typer)
	delivered0 := r.sinkBytes()
	started := time.Now()
	typer.paste(bytes.Repeat([]byte("p"), size))
	waitIdle(r.t, typer)
	total := time.Since(started)
	if r.failed(typer) {
		r.t.Fatalf("%s paste failed", mode)
	}
	if got := r.sinkBytes() - delivered0; got != size {
		r.t.Fatalf("%s paste: the task received %d of %d bytes", mode, got, size)
	}
	up1, down1 := r.proxy.counts()
	return pasteResult{total: total, units: r.requestsOf(typer) - units0, up: up1 - up0, down: down1 - down0}
}

// batchPaste is the HTTP fallback: one claim, then up to 32 frames per request.
func (r *jobBenchRig) batchPaste(ctx context.Context, size int) pasteResult {
	stream := r.newStream()
	path := r.f.path(r.kind, "input-batch")
	up0, down0 := r.proxy.counts()
	delivered0 := r.sinkBytes()
	started := time.Now()
	requests := int64(0)
	send := func(frames []terminal.InputFrame) {
		requests++
		if status, err := r.post(ctx, path, map[string]any{"frames": frames}); err != nil || status != 200 {
			r.t.Fatalf("batch: %d %v", status, err)
		}
	}
	send([]terminal.InputFrame{{Stream: stream}})
	seq := uint64(0)
	for sent := 0; sent < size; {
		var frames []terminal.InputFrame
		for len(frames) < terminal.InputWindow && sent < size {
			n := min(terminal.InputFrameBytes, size-sent)
			seq++
			frames = append(frames, terminal.InputFrame{Stream: stream, Seq: seq, Data: bytes.Repeat([]byte("p"), n)})
			sent += n
		}
		send(frames)
	}
	up1, down1 := r.proxy.counts()
	total := time.Since(started)
	if got := r.sinkBytes() - delivered0; got != size {
		r.t.Fatalf("batch paste: the task received %d of %d bytes", got, size)
	}
	return pasteResult{total: total, units: requests, up: up1 - up0, down: down1 - down0}
}

// coldKey is the first key of a page that did not prepare a connection:
// negotiation, the socket upgrade, the claim, then the frame and its ACK.
func (r *jobBenchRig) coldKey(ctx context.Context) time.Duration {
	// The page already has an open HTTP connection.
	if status, err := r.post(ctx, r.f.path(r.kind, "input-transport"), struct{}{}); err != nil || status != 200 {
		r.t.Fatalf("prime: %d %v", status, err)
	}
	started := time.Now()
	if status, err := r.post(ctx, r.f.path(r.kind, "input-transport"), struct{}{}); err != nil || status != 200 {
		r.t.Fatalf("negotiate: %d %v", status, err)
	}
	typer, err := r.newStreamTyper(ctx)
	if err != nil {
		r.t.Fatal(err)
	}
	defer typer.close()
	typer.paste([]byte("x"))
	waitIdle(r.t, typer)
	return time.Since(started)
}

type benchRow struct {
	RTTms float64 `json:"rttMs"`

	SingleKeyLegacyMs float64 `json:"singleKeyLegacyMs"`
	SingleKeyStreamMs float64 `json:"singleKeyStreamMs"`

	TypingLegacyP50Ms float64 `json:"typingLegacyP50Ms"`
	TypingLegacyP95Ms float64 `json:"typingLegacyP95Ms"`
	TypingStreamP50Ms float64 `json:"typingStreamP50Ms"`
	TypingStreamP95Ms float64 `json:"typingStreamP95Ms"`

	TypingLegacyRequests int64 `json:"typingLegacyRequests"`
	TypingStreamFrames   int64 `json:"typingStreamFrames"`
	TypingLegacyBytes    int64 `json:"typingLegacyBytes"`
	TypingStreamBytes    int64 `json:"typingStreamBytes"`

	Paste8KLegacyMs  float64 `json:"paste8KLegacyMs"`
	Paste8KStreamMs  float64 `json:"paste8KStreamMs"`
	Paste64KLegacyMs float64 `json:"paste64KLegacyMs"`
	Paste64KStreamMs float64 `json:"paste64KStreamMs"`
	Paste64KBatchMs  float64 `json:"paste64KBatchMs"`

	Paste64KLegacyRequests int64 `json:"paste64KLegacyRequests"`
	Paste64KStreamFrames   int64 `json:"paste64KStreamFrames"`
	Paste64KBatchRequests  int64 `json:"paste64KBatchRequests"`
	Paste64KLegacyBytes    int64 `json:"paste64KLegacyBytes"`
	Paste64KStreamBytes    int64 `json:"paste64KStreamBytes"`
	Paste64KBatchBytes     int64 `json:"paste64KBatchBytes"`

	ColdStreamMs float64 `json:"coldStreamMs"`
}

// TestJobInputLegacyVersusStreamed is an opt-in benchmark of the browser's
// task-terminal input, before (one request at a time) and after (acknowledged
// frames in flight), against the real Panel handlers with injected latency:
//
//	KPANEL_JOB_INPUT_BENCH=1 go test -run JobInputLegacyVersusStreamed -v -timeout 30m ./internal/panel/
//
// KPANEL_JOB_INPUT_BENCH_OUT=<file> also writes the rows as JSON.
func TestJobInputLegacyVersusStreamed(t *testing.T) {
	if os.Getenv("KPANEL_JOB_INPUT_BENCH") != "1" {
		t.Skip("set KPANEL_JOB_INPUT_BENCH=1 to run the latency comparison")
	}
	f := newJobTerminalFixture(t)
	ctx := context.Background()
	var rows []benchRow
	for _, oneWay := range []time.Duration{0, 25 * time.Millisecond, 75 * time.Millisecond} {
		rig := newJobBenchRig(t, f, oneWay)
		row := benchRow{RTTms: ms(2 * oneWay)}
		row.SingleKeyLegacyMs = ms(rig.singleKey(ctx, modeLegacy, 15))
		row.SingleKeyStreamMs = ms(rig.singleKey(ctx, modeStream, 15))
		legacyTyping := rig.typing(ctx, modeLegacy, 75, 40*time.Millisecond)
		streamTyping := rig.typing(ctx, modeStream, 75, 40*time.Millisecond)
		row.TypingLegacyP50Ms, row.TypingLegacyP95Ms = ms(legacyTyping.p50), ms(legacyTyping.p95)
		row.TypingStreamP50Ms, row.TypingStreamP95Ms = ms(streamTyping.p50), ms(streamTyping.p95)
		row.TypingLegacyRequests, row.TypingStreamFrames = legacyTyping.units, streamTyping.units
		row.TypingLegacyBytes, row.TypingStreamBytes = legacyTyping.wire, streamTyping.wire
		row.Paste8KLegacyMs = ms(rig.paste(ctx, modeLegacy, 8<<10).total)
		row.Paste8KStreamMs = ms(rig.paste(ctx, modeStream, 8<<10).total)
		legacyPaste := rig.paste(ctx, modeLegacy, 64<<10)
		streamPaste := rig.paste(ctx, modeStream, 64<<10)
		batchPaste := rig.batchPaste(ctx, 64<<10)
		row.Paste64KLegacyMs, row.Paste64KStreamMs, row.Paste64KBatchMs = ms(legacyPaste.total), ms(streamPaste.total), ms(batchPaste.total)
		row.Paste64KLegacyRequests, row.Paste64KStreamFrames, row.Paste64KBatchRequests = legacyPaste.units, streamPaste.units, batchPaste.units
		row.Paste64KLegacyBytes = legacyPaste.up + legacyPaste.down
		row.Paste64KStreamBytes = streamPaste.up + streamPaste.down
		row.Paste64KBatchBytes = batchPaste.up + batchPaste.down
		var cold []time.Duration
		for i := 0; i < 5; i++ {
			cold = append(cold, rig.coldKey(ctx))
		}
		row.ColdStreamMs = ms(median(cold))
		rows = append(rows, row)
		t.Logf("RTT %3.0f ms | 1 key legacy %.0f stream %.0f | typing p50 legacy %.0f stream %.0f, p95 legacy %.0f stream %.0f | 8 KiB legacy %.0f stream %.0f | 64 KiB legacy %.0f stream %.0f batch %.0f | cold stream %.0f",
			row.RTTms, row.SingleKeyLegacyMs, row.SingleKeyStreamMs, row.TypingLegacyP50Ms, row.TypingStreamP50Ms, row.TypingLegacyP95Ms, row.TypingStreamP95Ms,
			row.Paste8KLegacyMs, row.Paste8KStreamMs, row.Paste64KLegacyMs, row.Paste64KStreamMs, row.Paste64KBatchMs, row.ColdStreamMs)
	}
	var table strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&table, "RTT %.0f ms: 75 typed keys = legacy %d requests / %d B, stream %d frames / %d B\n",
			row.RTTms, row.TypingLegacyRequests, row.TypingLegacyBytes, row.TypingStreamFrames, row.TypingStreamBytes)
		fmt.Fprintf(&table, "RTT %.0f ms: 64 KiB paste = legacy %d requests / %d B on the wire, stream %d frames / %d B, batch %d requests / %d B\n",
			row.RTTms, row.Paste64KLegacyRequests, row.Paste64KLegacyBytes, row.Paste64KStreamFrames, row.Paste64KStreamBytes, row.Paste64KBatchRequests, row.Paste64KBatchBytes)
	}
	t.Log("\n" + table.String())
	if out := os.Getenv("KPANEL_JOB_INPUT_BENCH_OUT"); out != "" {
		data, _ := json.MarshalIndent(rows, "", "  ")
		if err := os.WriteFile(out, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
