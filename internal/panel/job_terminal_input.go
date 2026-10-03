package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/terminal"
)

const (
	jobTerminalInputPrefix = "/api/v1/job-terminals/"
	// A task outlives the page that opened it, so a reloaded page connects
	// while the previous socket may still be draining. The per-task cap
	// covers that overlap and the global one bounds sockets in total.
	maxJobInputStreams        = 64
	maxJobInputStreamsPerTask = 3
)

// jobInputKeepalive is how often a stream pings its page; one that does not
// answer within the same time is dropped. Tests shorten it.
var jobInputKeepalive = 20 * time.Second

// jobInputGate bounds concurrent task-terminal input streams and batches.
type jobInputGate struct {
	mu    sync.Mutex
	tasks map[string]int
	total int
}

func (g *jobInputGate) acquire(task string) (release func(), ok bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.tasks == nil {
		g.tasks = make(map[string]int)
	}
	if g.total >= maxJobInputStreams || g.tasks[task] >= maxJobInputStreamsPerTask {
		return nil, false
	}
	g.tasks[task]++
	g.total++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.tasks[task]--; g.tasks[task] <= 0 {
				delete(g.tasks, task)
			}
			g.total--
		})
	}, true
}

// handleJobTerminalInput serves the input channel of application, website,
// diagnostic and environment task terminals:
//
//	POST /api/v1/job-terminals/{kind}/{id}/input-transport  which protocol the Agent speaks
//	GET  /api/v1/job-terminals/{kind}/{id}/input-stream     WebSocket, pipelined acknowledged frames
//	POST /api/v1/job-terminals/{kind}/{id}/input-batch      the same frames over plain HTTP
//
// Authentication, framing and windowing are those of host terminals. The
// Agent keeps the ordering state because the FIFO's reader is a separate job
// process, not a PTY the Agent owns.
func (s *Server) handleJobTerminalInput(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_terminal_request", "Invalid terminal request", "")
		return
	}
	token, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && (!s.checkOrigin(w, r) || !s.checkCSRF(w, r, session)) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, jobTerminalInputPrefix), "/")
	if len(parts) != 3 {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	kind, id, action := parts[0], parts[1], parts[2]
	if _, known := jobTerminalAgentPrefixes[kind]; !known || !siteIDPattern.MatchString(id) {
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		return
	}
	switch action {
	case "input-transport":
		if r.Method != http.MethodPost {
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		protocol := ""
		if s.jobTerminalSupportsSequencedInput(r.Context()) {
			protocol = terminal.InputProtocol
		}
		s.writeJSON(w, http.StatusOK, map[string]string{"protocol": protocol})
	case "input-stream":
		s.serveTerminalInputSocket(w, r, token, session, s.jobTerminalInputTarget(kind, id))
	case "input-batch":
		s.serveTerminalInputBatch(w, r, token, session, s.jobTerminalInputTarget(kind, id))
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

// A task terminal whose Agent predates this protocol (an upgrade in progress)
// answers 404; the browser then keeps using the per-request input route.
func (s *Server) jobTerminalSupportsSequencedInput(ctx context.Context) bool {
	response, err := s.agent.Do(ctx, http.MethodPost, "/v1/job-terminals/capabilities/input-protocol", "", newRequestID(), []byte("{}"))
	var result struct {
		Protocol string `json:"protocol"`
	}
	return decodeTerminalAgentResponse(response, err, &result) == nil && result.Protocol == terminal.InputProtocol
}

func (s *Server) jobTerminalInputTarget(kind, id string) terminalInputTarget {
	apply := func(ctx context.Context, frame terminal.InputFrame) (string, error) {
		body, _ := json.Marshal(map[string]any{"frame": frame})
		response, err := s.agent.Do(ctx, http.MethodPost, "/v1/job-terminals/"+kind+"/"+id+"/input-sequenced", "", newRequestID(), body)
		var result struct {
			Accepted bool   `json:"accepted"`
			Epoch    string `json:"epoch"`
		}
		err = decodeTerminalAgentResponse(response, err, &result)
		return result.Epoch, err
	}
	return terminalInputTarget{
		admit: func(*terminalInputConnection, bool) (func(), int, string, string) {
			release, ok := s.jobInputs.acquire(kind + "/" + id)
			if !ok {
				return nil, http.StatusTooManyRequests, "terminal_stream_limit", "Too many task terminal input streams"
			}
			return release, 0, "", ""
		},
		// Task liveness is the Agent's to judge, once per applied frame.
		alive: func(*terminalInputConnection) bool { return true },
		claim: apply,
		send: func(ctx context.Context, frame terminal.InputFrame) (func() error, error) {
			_, err := apply(ctx, frame)
			return func() error { return err }, nil
		},
		immediate: func() bool { return true },
		claimed:   func(*terminalInputConnection) {},
		keepalive: jobInputKeepalive,
	}
}
