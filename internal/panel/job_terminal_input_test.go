package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	jobTestID      = "0123456789abcdef0123456789abcdef"
	jobStreamOne   = "00000000000000000000000000000001"
	jobStreamTwo   = "00000000000000000000000000000002"
	jobStreamThree = "00000000000000000000000000000003"
)

// jobInputAgent behaves like the Agent's task-input endpoint: one sequencer per
// task, a claim before data, and a sink that stands in for the job's FIFO.
type jobInputAgent struct {
	terminalAgentStub
	mu         sync.Mutex
	supported  bool
	sequencers map[string]*terminal.InputSequencer
	sink       bytes.Buffer
	writeErr   error
	down       bool
	closed     bool
	applied    int
	writes     []jobSinkWrite
}

// jobSinkWrite records when bytes reached the task, for the benchmark.
type jobSinkWrite struct {
	at time.Time
	n  int
}

// applyLocked is the task's FIFO; the caller holds a.mu.
func (a *jobInputAgent) applyLocked(data []byte) {
	a.sink.Write(data)
	a.applied++
	a.writes = append(a.writes, jobSinkWrite{at: time.Now(), n: len(data)})
}

func newJobInputAgent() *jobInputAgent {
	return &jobInputAgent{supported: true, sequencers: make(map[string]*terminal.InputSequencer)}
}

func (a *jobInputAgent) setFailure(writeErr error, down bool) {
	a.mu.Lock()
	a.writeErr, a.down = writeErr, down
	a.mu.Unlock()
}

func (a *jobInputAgent) setClosed(closed bool) {
	a.mu.Lock()
	a.closed = closed
	a.mu.Unlock()
}

func (a *jobInputAgent) claimedTasks() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.sequencers)
}

func (a *jobInputAgent) received() (string, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sink.String(), a.applied
}

func problem(status int, code string) AgentResponse {
	body, _ := json.Marshal(map[string]string{"code": code})
	return AgentResponse{StatusCode: status, Body: body}
}

func (a *jobInputAgent) Do(ctx context.Context, method, path, query, requestID string, body []byte) (AgentResponse, error) {
	if strings.HasSuffix(path, "/capabilities/input-protocol") {
		a.mu.Lock()
		defer a.mu.Unlock()
		if !a.supported {
			return problem(http.StatusNotFound, "not_found"), nil
		}
		return AgentResponse{StatusCode: 200, Body: []byte(`{"protocol":"terminal-input-v1"}`)}, nil
	}
	if strings.HasPrefix(path, "/v1/app-jobs/") && strings.HasSuffix(path, "/input") {
		// The per-request route that task terminals used before.
		var input struct {
			Data string `json:"data"`
		}
		if err := json.Unmarshal(body, &input); err != nil {
			return problem(http.StatusBadRequest, "invalid_request"), nil
		}
		a.mu.Lock()
		a.applyLocked([]byte(input.Data))
		a.mu.Unlock()
		return AgentResponse{StatusCode: 200, Body: []byte(`{"ok":true}`)}, nil
	}
	if !strings.HasPrefix(path, "/v1/job-terminals/") || !strings.HasSuffix(path, "/input-sequenced") {
		return a.terminalAgentStub.Do(ctx, method, path, query, requestID, body)
	}
	a.mu.Lock()
	down, closed := a.down, a.closed
	a.mu.Unlock()
	if down {
		return AgentResponse{}, errors.New("agent unavailable")
	}
	if closed {
		return problem(http.StatusConflict, "terminal_closed"), nil
	}
	var input struct {
		Frame terminal.InputFrame `json:"frame"`
	}
	if err := json.Unmarshal(body, &input); err != nil {
		return problem(http.StatusBadRequest, "invalid_request"), nil
	}
	key := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/job-terminals/"), "/input-sequenced")
	a.mu.Lock()
	sequencer := a.sequencers[key]
	if sequencer == nil && input.Frame.Seq == 0 {
		sequencer = terminal.NewInputSequencer()
		a.sequencers[key] = sequencer
	}
	a.mu.Unlock()
	if sequencer == nil {
		return problem(http.StatusConflict, "terminal_input_sequence"), nil
	}
	err := sequencer.Admit(ctx, input.Frame, func(data []byte) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.writeErr != nil {
			return a.writeErr
		}
		a.applyLocked(data)
		return nil
	})
	switch {
	case err == nil:
		reply, _ := json.Marshal(map[string]any{"accepted": true, "epoch": sequencer.Epoch()})
		return AgentResponse{StatusCode: 200, Body: reply}, nil
	case errors.Is(err, terminal.ErrInputNotWritten):
		return problem(http.StatusServiceUnavailable, "terminal_input_unavailable"), nil
	case errors.Is(err, terminal.ErrInputUncertain):
		return problem(http.StatusConflict, "terminal_input_uncertain"), nil
	case errors.Is(err, terminal.ErrClosed):
		return problem(http.StatusConflict, "terminal_closed"), nil
	default:
		return problem(http.StatusConflict, "terminal_input_sequence"), nil
	}
}

type jobTerminalFixture struct {
	t       *testing.T
	server  *Server
	agent   *jobInputAgent
	http    *httptest.Server
	session *http.Cookie
	csrf    *http.Cookie
	headers map[string]string
}

func newJobTerminalFixture(t *testing.T) *jobTerminalFixture {
	t.Helper()
	server, tokenPath := newTestServer(t)
	agent := newJobInputAgent()
	server.agent = agent
	session, csrf := bootstrapCookies(t, server, tokenPath)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "panel.test"; server.ServeHTTP(w, r) }))
	t.Cleanup(httpServer.Close)
	return &jobTerminalFixture{t: t, server: server, agent: agent, http: httpServer, session: session, csrf: csrf,
		headers: map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test", "X-CSRF-Token": csrf.Value}}
}

func (f *jobTerminalFixture) path(kind, action string) string {
	return jobTerminalInputPrefix + kind + "/" + jobTestID + "/" + action
}

func (f *jobTerminalFixture) post(path string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	return authenticatedRequest(f.server, http.MethodPost, path, data, f.session, f.csrf, f.headers)
}

func (f *jobTerminalFixture) dial(ctx context.Context, kind, origin string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{"Origin": []string{origin}, "Cookie": []string{f.session.String() + "; " + f.csrf.String()}}
	return websocket.Dial(ctx, f.http.URL+f.path(kind, "input-stream"), &websocket.DialOptions{HTTPHeader: header, Subprotocols: []string{terminalInputSocketProtocol}})
}

func jobSocketWrite(t *testing.T, ctx context.Context, ws *websocket.Conn, value any) {
	t.Helper()
	data, _ := json.Marshal(value)
	if err := ws.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

func jobSocketRead(t *testing.T, ctx context.Context, ws *websocket.Conn) terminalInputReply {
	t.Helper()
	_, data, err := ws.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var reply terminalInputReply
	if err := json.Unmarshal(data, &reply); err != nil {
		t.Fatal(err)
	}
	return reply
}

// connect opens a stream and returns it with the ready reply.
func (f *jobTerminalFixture) connect(ctx context.Context, kind, stream string) (*websocket.Conn, terminalInputReply) {
	f.t.Helper()
	ws, _, err := f.dial(ctx, kind, "http://panel.test")
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { ws.CloseNow() })
	jobSocketWrite(f.t, ctx, ws, map[string]string{"type": "auth", "csrf": f.csrf.Value, "stream": stream})
	return ws, jobSocketRead(f.t, ctx, ws)
}

func sendJobFrame(t *testing.T, ctx context.Context, ws *websocket.Conn, stream string, seq uint64, data string) {
	t.Helper()
	frame := terminal.InputFrame{Stream: stream, Seq: seq, Data: []byte(data)}
	jobSocketWrite(t, ctx, ws, terminalInputMessage{Type: "input", Frame: &frame})
}

func TestJobTerminalInputTransportFollowsTheAgent(t *testing.T) {
	f := newJobTerminalFixture(t)
	for _, kind := range []string{"app", "site", "diagnostic", "environment"} {
		response := f.post(f.path(kind, "input-transport"), struct{}{})
		if response.Code != 200 || !strings.Contains(response.Body.String(), terminal.InputProtocol) {
			t.Fatalf("%s transport: %d %s", kind, response.Code, response.Body.String())
		}
	}
	// An Agent that predates the protocol: the browser must fall back to POST.
	f.agent.mu.Lock()
	f.agent.supported = false
	f.agent.mu.Unlock()
	response := f.post(f.path("app", "input-transport"), struct{}{})
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"protocol":""`) {
		t.Fatalf("old Agent transport: %d %s", response.Code, response.Body.String())
	}
}

func TestJobTerminalInputRoutesRejectBadRequests(t *testing.T) {
	f := newJobTerminalFixture(t)
	for _, path := range []string{
		jobTerminalInputPrefix + "unknown/" + jobTestID + "/input-transport",
		jobTerminalInputPrefix + "app/not-a-job/input-transport",
		jobTerminalInputPrefix + "app/" + jobTestID + "/other",
		jobTerminalInputPrefix + "app/" + jobTestID,
	} {
		if response := f.post(path, struct{}{}); response.Code != 404 {
			t.Fatalf("%s: %d", path, response.Code)
		}
	}
	if response := authenticatedRequest(f.server, http.MethodGet, f.path("app", "input-transport"), nil, f.session, f.csrf, f.headers); response.Code != 405 {
		t.Fatalf("GET transport: %d", response.Code)
	}
	noCSRF := map[string]string{"Content-Type": "application/json", "Origin": "http://panel.test"}
	if response := authenticatedRequest(f.server, http.MethodPost, f.path("app", "input-transport"), []byte(`{}`), f.session, f.csrf, noCSRF); response.Code == 200 {
		t.Fatal("transport accepted without CSRF")
	}
	anonymous := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, f.path("app", "input-transport"), strings.NewReader(`{}`))
	request.Host = "panel.test"
	f.server.ServeHTTP(anonymous, request)
	if anonymous.Code != 401 {
		t.Fatalf("anonymous transport: %d", anonymous.Code)
	}
}

func TestJobTerminalInputSocketGuardsTheHandshake(t *testing.T) {
	f := newJobTerminalFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if ws, response, err := f.dial(ctx, "app", "https://evil.example"); err == nil {
		ws.CloseNow()
		t.Fatal("foreign origin accepted")
	} else if response == nil || response.StatusCode != 403 {
		t.Fatalf("foreign origin: %v %v", response, err)
	}
	if ws, response, err := f.dial(ctx, "app", ""); err == nil {
		ws.CloseNow()
		t.Fatal("missing origin accepted")
	} else if response == nil || response.StatusCode != 400 {
		t.Fatalf("missing origin: %v %v", response, err)
	}
	ws, _, err := f.dial(ctx, "app", "http://panel.test")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	jobSocketWrite(t, ctx, ws, map[string]string{"type": "auth", "csrf": "wrong", "stream": jobStreamOne})
	if reply := jobSocketRead(t, ctx, ws); reply.Code != "csrf_validation_failed" {
		t.Fatalf("bad csrf: %+v", reply)
	}
	if _, writes := f.agent.received(); writes != 0 || f.agent.claimedTasks() != 0 {
		t.Fatal("an unauthenticated stream reached the Agent")
	}
}

func TestJobTerminalInputStreamsAcknowledgedFramesAndReplaysALostACK(t *testing.T) {
	f := newJobTerminalFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, ready := f.connect(ctx, "diagnostic", jobStreamOne)
	if ready.Type != "ready" || ready.Window != terminal.InputWindow || len(ready.Epoch) != 32 {
		t.Fatalf("ready: %+v", ready)
	}
	// A burst is sent without waiting for any ACK, as the browser does.
	for seq := uint64(1); seq <= 5; seq++ {
		sendJobFrame(t, ctx, ws, jobStreamOne, seq, "<"+string(rune('0'+seq))+">")
	}
	for seq := uint64(1); seq <= 5; seq++ {
		if reply := jobSocketRead(t, ctx, ws); reply.Type != "ack" || reply.Seq != seq {
			t.Fatalf("ack %d: %+v", seq, reply)
		}
	}
	// The page drops before it saw the ACK of the last frame, reconnects with
	// the same stream and offers the frame again.
	ws.CloseNow()
	again, secondReady := f.connect(ctx, "diagnostic", jobStreamOne)
	if secondReady.Epoch != ready.Epoch {
		t.Fatalf("the same task must report the same epoch: %s %s", ready.Epoch, secondReady.Epoch)
	}
	sendJobFrame(t, ctx, again, jobStreamOne, 5, "<5>")
	if reply := jobSocketRead(t, ctx, again); reply.Type != "ack" || reply.Seq != 5 {
		t.Fatalf("replay ack: %+v", reply)
	}
	if data, writes := f.agent.received(); data != "<1><2><3><4><5>" || writes != 5 {
		t.Fatalf("task received %q after %d writes", data, writes)
	}
	sendJobFrame(t, ctx, again, jobStreamOne, 7, "gap")
	if reply := jobSocketRead(t, ctx, again); reply.Type != "error" || reply.Code != "terminal_input_sequence" || reply.Retryable {
		t.Fatalf("gap: %+v", reply)
	}
}

func TestJobTerminalInputReloadedPageTakesOverWhileTheOldSocketDrains(t *testing.T) {
	f := newJobTerminalFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	oldPage, _ := f.connect(ctx, "app", jobStreamOne)
	sendJobFrame(t, ctx, oldPage, jobStreamOne, 1, "old")
	if reply := jobSocketRead(t, ctx, oldPage); reply.Type != "ack" {
		t.Fatalf("old page: %+v", reply)
	}
	// The reloaded page must not be refused because the old socket is lingering.
	newPage, ready := f.connect(ctx, "app", jobStreamTwo)
	if ready.Type != "ready" {
		t.Fatalf("takeover: %+v", ready)
	}
	sendJobFrame(t, ctx, oldPage, jobStreamOne, 2, "stale")
	if reply := jobSocketRead(t, ctx, oldPage); reply.Type != "error" || reply.Code != "terminal_input_sequence" || reply.Retryable {
		t.Fatalf("fenced page: %+v", reply)
	}
	sendJobFrame(t, ctx, newPage, jobStreamTwo, 1, "new")
	if reply := jobSocketRead(t, ctx, newPage); reply.Type != "ack" || reply.Seq != 1 {
		t.Fatalf("new page: %+v", reply)
	}
	if data, _ := f.agent.received(); data != "oldnew" {
		t.Fatalf("task received %q", data)
	}
}

func TestJobTerminalInputStreamCapacityIsPerTaskAndGlobal(t *testing.T) {
	f := newJobTerminalFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	streams := []string{jobStreamOne, jobStreamTwo, jobStreamThree}
	var sockets []*websocket.Conn
	for _, stream := range streams[:maxJobInputStreamsPerTask] {
		ws, ready := f.connect(ctx, "app", stream)
		if ready.Type != "ready" {
			t.Fatalf("stream %s: %+v", stream, ready)
		}
		sockets = append(sockets, ws)
	}
	if ws, response, err := f.dial(ctx, "app", "http://panel.test"); err == nil {
		ws.CloseNow()
		t.Fatal("per-task capacity was not enforced")
	} else if response == nil || response.StatusCode != 429 {
		t.Fatalf("per-task capacity status: %v %v", response, err)
	}
	// A batch shares the same admission.
	if response := f.post(f.path("app", "input-batch"), map[string]any{"frames": []terminal.InputFrame{{Stream: jobStreamOne}}}); response.Code != 429 {
		t.Fatalf("batch beyond capacity: %d", response.Code)
	}
	sockets[0].CloseNow()
	deadline := time.Now().Add(2 * time.Second)
	for {
		f.server.jobInputs.mu.Lock()
		held := f.server.jobInputs.tasks["app/"+jobTestID]
		f.server.jobInputs.mu.Unlock()
		if held < maxJobInputStreamsPerTask {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("a closed socket kept its slot")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, ready := f.connect(ctx, "app", jobStreamOne); ready.Type != "ready" {
		t.Fatalf("released slot unusable: %+v", ready)
	}

	var gate jobInputGate
	var releases []func()
	for i := 0; i < maxJobInputStreams; i++ {
		release, ok := gate.acquire(fmt.Sprintf("task/%d", i))
		if !ok {
			t.Fatalf("slot %d refused", i)
		}
		releases = append(releases, release)
	}
	if _, ok := gate.acquire("task/extra"); ok {
		t.Fatal("global capacity was not enforced")
	}
	releases[0]()
	releases[0]()
	if gate.total != maxJobInputStreams-1 {
		t.Fatalf("release is not idempotent: %d", gate.total)
	}
}

func TestJobTerminalInputRetriesOnlyWhatNeverReachedTheTask(t *testing.T) {
	f := newJobTerminalFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _ := f.connect(ctx, "site", jobStreamOne)

	f.agent.setFailure(terminal.ErrInputNotWritten, false)
	sendJobFrame(t, ctx, ws, jobStreamOne, 1, "cmd\r")
	if reply := jobSocketRead(t, ctx, ws); reply.Type != "error" || reply.Code != "terminal_input_unavailable" || !reply.Retryable {
		t.Fatalf("unwritten frame: %+v", reply)
	}
	f.agent.setFailure(nil, false)
	again, _ := f.connect(ctx, "site", jobStreamOne)
	sendJobFrame(t, ctx, again, jobStreamOne, 1, "cmd\r")
	if reply := jobSocketRead(t, ctx, again); reply.Type != "ack" || reply.Seq != 1 {
		t.Fatalf("retry: %+v", reply)
	}

	f.agent.setFailure(errors.New("short write"), false)
	sendJobFrame(t, ctx, again, jobStreamOne, 2, "next\r")
	if reply := jobSocketRead(t, ctx, again); reply.Code != "terminal_input_uncertain" || reply.Retryable {
		t.Fatalf("possibly delivered frame: %+v", reply)
	}
	f.agent.setFailure(nil, false)
	_, writes := f.agent.received()
	// The frozen stream cannot be resumed, so a reconnect is refused for good.
	if _, reply := f.connect(ctx, "site", jobStreamOne); reply.Code != "terminal_input_claim" || reply.Retryable {
		t.Fatalf("frozen stream reconnect: %+v", reply)
	}
	if _, after := f.agent.received(); after != writes {
		t.Fatal("a possibly delivered frame was offered again")
	}
}

func TestJobTerminalInputClaimFailuresAreClassified(t *testing.T) {
	f := newJobTerminalFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f.agent.setFailure(nil, true)
	ws, _, err := f.dial(ctx, "environment", "http://panel.test")
	if err != nil {
		t.Fatal(err)
	}
	defer ws.CloseNow()
	jobSocketWrite(t, ctx, ws, map[string]string{"type": "auth", "csrf": f.csrf.Value, "stream": jobStreamOne})
	if reply := jobSocketRead(t, ctx, ws); reply.Code != "terminal_input_claim" || !reply.Retryable {
		t.Fatalf("Agent down: %+v", reply)
	}
	f.agent.setFailure(nil, false)
	f.agent.setClosed(true)
	closed, _, err := f.dial(ctx, "environment", "http://panel.test")
	if err != nil {
		t.Fatal(err)
	}
	defer closed.CloseNow()
	jobSocketWrite(t, ctx, closed, map[string]string{"type": "auth", "csrf": f.csrf.Value, "stream": jobStreamOne})
	if reply := jobSocketRead(t, ctx, closed); reply.Code != "terminal_input_claim" || reply.Retryable {
		t.Fatalf("finished task: %+v", reply)
	}
}

func TestJobTerminalInputBatchFallbackClaimsOrdersAndReplays(t *testing.T) {
	f := newJobTerminalFixture(t)
	batch := func(frames []terminal.InputFrame) *httptest.ResponseRecorder {
		return f.post(f.path("app", "input-batch"), map[string]any{"frames": frames})
	}
	claim := batch([]terminal.InputFrame{{Stream: jobStreamOne}})
	var reply terminalInputBatchReply
	if claim.Code != 200 || json.Unmarshal(claim.Body.Bytes(), &reply) != nil || reply.Acked != 0 || len(reply.Epoch) != 32 {
		t.Fatalf("claim: %d %s", claim.Code, claim.Body.String())
	}
	frames := make([]terminal.InputFrame, terminal.InputWindow)
	var want string
	for i := range frames {
		frames[i] = terminal.InputFrame{Stream: jobStreamOne, Seq: uint64(i + 1), Data: []byte{byte('a' + i), '\r'}}
		want += string(frames[i].Data)
	}
	for repeat := 0; repeat < 2; repeat++ {
		response := batch(frames)
		var data terminalInputBatchReply
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &data) != nil || data.Acked != terminal.InputWindow {
			t.Fatalf("batch %d: %d %s", repeat, response.Code, response.Body.String())
		}
	}
	if data, writes := f.agent.received(); data != want || writes != terminal.InputWindow {
		t.Fatalf("a replayed batch changed the task input: %d writes", writes)
	}
	if response := batch(append(frames, terminal.InputFrame{Stream: jobStreamOne, Seq: 33, Data: []byte("x")})); response.Code != 400 {
		t.Fatalf("oversized batch: %d", response.Code)
	}
	if response := batch([]terminal.InputFrame{{Stream: jobStreamOne, Seq: 40, Data: []byte("x")}}); response.Code != 409 {
		t.Fatalf("gap: %d %s", response.Code, response.Body.String())
	}
	// Data for a task nobody claimed (an Agent restart) must not be applied.
	f.agent.mu.Lock()
	f.agent.sequencers = make(map[string]*terminal.InputSequencer)
	f.agent.mu.Unlock()
	if response := batch([]terminal.InputFrame{{Stream: jobStreamOne, Seq: 33, Data: []byte("x")}}); response.Code != 409 {
		t.Fatalf("unclaimed data: %d", response.Code)
	}
	f.agent.setFailure(nil, true)
	if response := batch([]terminal.InputFrame{{Stream: jobStreamOne}}); response.Code != 503 {
		t.Fatalf("Agent down: %d", response.Code)
	}
}

func TestJobTerminalInputSocketReleasesAVanishedPage(t *testing.T) {
	// A page that stops answering pings (dead network, suspended machine) must
	// not hold its slot until TCP eventually notices.
	previous := jobInputKeepalive
	jobInputKeepalive = 100 * time.Millisecond
	defer func() { jobInputKeepalive = previous }()
	f := newJobTerminalFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// The client never reads after the handshake, so it never answers a ping.
	_, ready := f.connect(ctx, "app", jobStreamOne)
	if ready.Type != "ready" {
		t.Fatalf("ready: %+v", ready)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		f.server.jobInputs.mu.Lock()
		total := f.server.jobInputs.total
		f.server.jobInputs.mu.Unlock()
		if total == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the vanished page kept its stream slot")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
