package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	agentpkg "github.com/kejilion/kejilion-panel/internal/agent"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/hostpty"
	"github.com/kejilion/kejilion-panel/internal/terminal"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

// The tests above run the Panel against an in-memory Agent. These run it against
// the real Agent handlers, the real environment-task service and a real FIFO,
// the way a deployed Panel reaches a task: nothing between the browser socket
// and the bytes the job process reads is faked except the job process itself,
// which is a goroutine reading the FIFO.
type liveJobStack struct {
	t        *testing.T
	root     string
	stateDir string
	socket   string
	token    string
	fixture  *jobTerminalFixture
	stop     func()

	mu       sync.Mutex
	received bytes.Buffer
}

func newLiveJobStack(t *testing.T) *liveJobStack {
	t.Helper()
	// Unix socket paths are short; keep the tree shallow.
	root, err := os.MkdirTemp("", "kpjob")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	s := &liveJobStack{t: t, root: root, stateDir: filepath.Join(root, "env"), socket: filepath.Join(root, "agent.sock"), token: strings.Repeat("t", 40)}
	tokenFile := filepath.Join(root, "agent.token")
	if err := os.WriteFile(tokenFile, []byte(s.token), 0o600); err != nil {
		t.Fatal(err)
	}
	s.startAgent()
	t.Cleanup(func() { s.stop() })
	s.fixture = newJobTerminalFixtureFor(t, NewAgentClient(s.socket, tokenFile, 8<<20), nil)
	return s
}

// startAgent starts a new Agent process worth of state on the shared state
// directory: new in-memory registries, same tasks on disk.
func (s *liveJobStack) startAgent() {
	s.t.Helper()
	service, err := webenv.New(s.stateDir)
	if err != nil {
		s.t.Fatal(err)
	}
	webRoot := filepath.Join(s.root, "html")
	if err := os.MkdirAll(webRoot, 0o750); err != nil {
		s.t.Fatal(err)
	}
	server, err := agentpkg.NewServer(agentpkg.Config{
		Token: []byte(s.token), Version: "test", ProtocolVersion: "test", WebRoot: webRoot,
		Docker:         dockerx.New(filepath.Join(s.root, "docker.sock"), webRoot, filepath.Join(s.root, "docker")),
		WebEnvironment: service,
	})
	if err != nil {
		s.t.Fatal(err)
	}
	_ = os.Remove(s.socket)
	listener, err := net.Listen("unix", s.socket)
	if err != nil {
		s.t.Fatal(err)
	}
	httpServer := &http.Server{Handler: server}
	go httpServer.Serve(listener)
	s.stop = func() { httpServer.Close(); listener.Close() }
}

func (s *liveJobStack) restartAgent() {
	s.t.Helper()
	s.stop()
	s.startAgent()
}

func (s *liveJobStack) writeJob(status string) {
	s.t.Helper()
	// Started in the future so the service never asks systemd about this task.
	started := time.Now().UTC().Add(time.Hour)
	job := webenv.Job{ID: jobTestID, Action: "update", Status: status, Stage: "running", CreatedAt: time.Now().UTC(), StartedAt: &started}
	data, _ := json.Marshal(job)
	if err := os.MkdirAll(s.stateDir, 0o750); err != nil {
		s.t.Fatal(err)
	}
	tmp := filepath.Join(s.stateDir, jobTestID+".json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		s.t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(s.stateDir, jobTestID+".json")); err != nil {
		s.t.Fatal(err)
	}
}

func (s *liveJobStack) inputPath() string {
	return filepath.Join(s.stateDir, jobTestID+".terminal.input")
}

// createFIFO is the job process creating its input pipe; startReader is it
// opening and reading it.
func (s *liveJobStack) createFIFO() {
	s.t.Helper()
	if err := hostpty.CreateInput(s.inputPath()); err != nil {
		s.t.Fatal(err)
	}
}

func (s *liveJobStack) startReader() {
	s.t.Helper()
	reader, err := hostpty.OpenInput(s.inputPath())
	if err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() { reader.Close() })
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				s.mu.Lock()
				s.received.Write(buffer[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
}

func (s *liveJobStack) waitReceived(want string) {
	s.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		got := s.received.String()
		s.mu.Unlock()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("the job process read %q, want %q", got, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (s *liveJobStack) readNow() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.received.String()
}

func (s *liveJobStack) batch(frames []terminal.InputFrame) *httptest.ResponseRecorder {
	return s.fixture.post(s.fixture.path("environment", "input-batch"), map[string]any{"frames": frames})
}

func TestJobTerminalInputRealAgentDeliversEachByteExactlyOnce(t *testing.T) {
	s := newLiveJobStack(t)
	s.writeJob("running")
	s.createFIFO()
	s.startReader()
	f := s.fixture
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if response := f.post(f.path("environment", "input-transport"), struct{}{}); response.Code != 200 || !strings.Contains(response.Body.String(), terminal.InputProtocol) {
		t.Fatalf("the real Agent did not advertise the protocol: %d %s", response.Code, response.Body.String())
	}

	first, ready := f.connect(ctx, "environment", jobStreamOne)
	if ready.Type != "ready" || len(ready.Epoch) != 32 {
		t.Fatalf("ready: %+v", ready)
	}
	for seq, data := range []string{"ls\r", "pwd\r", "中文\r"} {
		sendJobFrame(t, ctx, first, jobStreamOne, uint64(seq+1), data)
	}
	for seq := uint64(1); seq <= 3; seq++ {
		if reply := jobSocketRead(t, ctx, first); reply.Type != "ack" || reply.Seq != seq {
			t.Fatalf("ack %d: %+v", seq, reply)
		}
	}
	s.waitReceived("ls\rpwd\r中文\r")

	// The page loses the socket before it saw the last ACK and offers it again.
	first.CloseNow()
	again, secondReady := f.connect(ctx, "environment", jobStreamOne)
	if secondReady.Epoch != ready.Epoch {
		t.Fatalf("the same Agent state must report the same epoch: %s %s", ready.Epoch, secondReady.Epoch)
	}
	sendJobFrame(t, ctx, again, jobStreamOne, 3, "中文\r")
	if reply := jobSocketRead(t, ctx, again); reply.Type != "ack" || reply.Seq != 3 {
		t.Fatalf("replay ack: %+v", reply)
	}
	time.Sleep(50 * time.Millisecond)
	if got := s.readNow(); got != "ls\rpwd\r中文\r" {
		t.Fatalf("a replayed frame reached the job process again: %q", got)
	}

	// Once a stream owns the task, the per-request route refuses bare writes.
	bare := f.post("/api/v1/web-environment/jobs/"+jobTestID+"/input", map[string]string{"data": "x"})
	if bare.Code != 409 || !strings.Contains(bare.Body.String(), "terminal_input_sequence") {
		t.Fatalf("bare write beside a claimed stream: %d %s", bare.Code, bare.Body.String())
	}

	// A reloaded page takes over; the old page can no longer interleave.
	reloaded, _ := f.connect(ctx, "environment", jobStreamTwo)
	sendJobFrame(t, ctx, again, jobStreamOne, 4, "stale\r")
	if reply := jobSocketRead(t, ctx, again); reply.Type != "error" || reply.Code != "terminal_input_sequence" || reply.Retryable {
		t.Fatalf("fenced page: %+v", reply)
	}
	sendJobFrame(t, ctx, reloaded, jobStreamTwo, 1, "new\r")
	if reply := jobSocketRead(t, ctx, reloaded); reply.Type != "ack" || reply.Seq != 1 {
		t.Fatalf("new page: %+v", reply)
	}
	s.waitReceived("ls\rpwd\r中文\rnew\r")

	// The task finishes: input is closed for good and nothing more is written.
	s.writeJob("succeeded")
	sendJobFrame(t, ctx, reloaded, jobStreamTwo, 2, "late\r")
	if reply := jobSocketRead(t, ctx, reloaded); reply.Type != "error" || reply.Code != "terminal_not_found" || reply.Retryable {
		t.Fatalf("a finished task must close the stream for good: %+v", reply)
	}
	if _, claim := f.connect(ctx, "environment", jobStreamThree); claim.Code != "terminal_input_claim" || claim.Retryable {
		t.Fatalf("claiming a finished task: %+v", claim)
	}
	time.Sleep(50 * time.Millisecond)
	if got := s.readNow(); got != "ls\rpwd\r中文\rnew\r" {
		t.Fatalf("input reached a finished task: %q", got)
	}
}

func TestJobTerminalInputRealAgentRestartForgetsTheSequenceState(t *testing.T) {
	s := newLiveJobStack(t)
	s.writeJob("running")
	s.createFIFO()
	s.startReader()
	f := s.fixture
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ws, ready := f.connect(ctx, "environment", jobStreamOne)
	sendJobFrame(t, ctx, ws, jobStreamOne, 1, "one\r")
	if reply := jobSocketRead(t, ctx, ws); reply.Type != "ack" {
		t.Fatalf("ack: %+v", reply)
	}
	s.waitReceived("one\r")
	ws.CloseNow()

	// The job outlives the Agent; the Agent's memory of the stream does not.
	s.restartAgent()

	// Frame 1 of the old stream may or may not have been applied by the old
	// process. The new one must neither guess nor replay it.
	if response := s.batch([]terminal.InputFrame{{Stream: jobStreamOne, Seq: 1, Data: []byte("one\r")}}); response.Code != 409 {
		t.Fatalf("data for an unclaimed task after a restart: %d %s", response.Code, response.Body.String())
	}
	if got := s.readNow(); got != "one\r" {
		t.Fatalf("the restarted Agent replayed input: %q", got)
	}

	// Reclaiming works, and the epoch tells the browser its state was lost.
	response := s.batch([]terminal.InputFrame{{Stream: jobStreamOne}})
	var claim terminalInputBatchReply
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &claim) != nil || len(claim.Epoch) != 32 {
		t.Fatalf("reclaim: %d %s", response.Code, response.Body.String())
	}
	if claim.Epoch == ready.Epoch {
		t.Fatal("a restarted Agent must not report the epoch of the state it lost")
	}
}

func TestJobTerminalInputRealAgentWaitsForTheTaskToReadItsFIFO(t *testing.T) {
	s := newLiveJobStack(t)
	s.writeJob("running")
	// The task is running but its process has not opened the pipe yet.
	s.createFIFO()
	f := s.fixture
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ws, ready := f.connect(ctx, "environment", jobStreamOne)
	if ready.Type != "ready" {
		t.Fatalf("a running task can be claimed before its process reads: %+v", ready)
	}
	sendJobFrame(t, ctx, ws, jobStreamOne, 1, "early\r")
	if reply := jobSocketRead(t, ctx, ws); reply.Type != "error" || reply.Code != "terminal_input_unavailable" || !reply.Retryable {
		t.Fatalf("nothing could have been written, so the frame is retryable: %+v", reply)
	}
	ws.CloseNow()
	if got := s.readNow(); got != "" {
		t.Fatalf("a refused frame reached the pipe: %q", got)
	}

	s.startReader()
	again, _ := f.connect(ctx, "environment", jobStreamOne)
	sendJobFrame(t, ctx, again, jobStreamOne, 1, "early\r")
	if reply := jobSocketRead(t, ctx, again); reply.Type != "ack" || reply.Seq != 1 {
		t.Fatalf("retry: %+v", reply)
	}
	s.waitReceived("early\r")
}
