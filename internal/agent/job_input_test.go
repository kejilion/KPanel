package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kejilion/kejilion-panel/internal/appmarket"
	"github.com/kejilion/kejilion-panel/internal/diagnostics"
	"github.com/kejilion/kejilion-panel/internal/hostpty"
	"github.com/kejilion/kejilion-panel/internal/sites"
	"github.com/kejilion/kejilion-panel/internal/terminal"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

const (
	jobStreamA = "00000000000000000000000000000001"
	jobStreamB = "00000000000000000000000000000002"
)

type fakeJob struct {
	mu     sync.Mutex
	probe  error
	writeE error
	data   strings.Builder
	writes int
}

func (f *fakeJob) backend() jobInput {
	return jobInput{
		probe: func(string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.probe
		},
		write: func(_ string, data string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.writes++
			if f.writeE != nil {
				return f.writeE
			}
			f.data.WriteString(data)
			return nil
		},
		invalid:  appmarket.ErrForbidden,
		missing:  appmarket.ErrNotFound,
		conflict: appmarket.ErrConflict,
	}
}

func (f *fakeJob) set(probe, write error) {
	f.mu.Lock()
	f.probe, f.writeE = probe, write
	f.mu.Unlock()
}

func (f *fakeJob) received() (string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.data.String(), f.writes
}

func jobInputServer(t *testing.T, kind string, job *fakeJob) *Server {
	t.Helper()
	s := testServer(t)
	s.jobInputBackends = map[string]jobInput{kind: job.backend()}
	return s
}

func postJobInput(s *Server, path string, body any) *httptest.ResponseRecorder {
	data, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func sequenced(s *Server, kind, id, stream string, seq uint64, data string) *httptest.ResponseRecorder {
	frame := map[string]any{"stream": stream, "seq": seq}
	if data != "" {
		frame["data"] = []byte(data)
	}
	return postJobInput(s, "/v1/job-terminals/"+kind+"/"+id+"/input-sequenced", map[string]any{"frame": frame})
}

func epochOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var reply struct {
		Accepted bool   `json:"accepted"`
		Epoch    string `json:"epoch"`
	}
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &reply) != nil || !reply.Accepted || len(reply.Epoch) != 32 {
		t.Fatalf("expected an accepted frame with an epoch: %d %s", w.Code, w.Body.String())
	}
	return reply.Epoch
}

func expectProblem(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status || !strings.Contains(w.Body.String(), code) {
		t.Fatalf("expected %d %s, got %d %s", status, code, w.Code, w.Body.String())
	}
}

func TestJobTerminalAdvertisesTheInputProtocol(t *testing.T) {
	s := testServer(t)
	w := postJobInput(s, "/v1/job-terminals/capabilities/input-protocol", struct{}{})
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), terminal.InputProtocol) {
		t.Fatalf("capability: %d %s", w.Code, w.Body.String())
	}
	r := httptest.NewRequest(http.MethodGet, "/v1/job-terminals/capabilities/input-protocol", nil)
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, r)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET capability: %d", recorder.Code)
	}
}

func TestJobTerminalSequencedInputClaimsOrdersAndDeduplicates(t *testing.T) {
	job := &fakeJob{}
	s := jobInputServer(t, "app", job)
	claim := epochOf(t, sequenced(s, "app", "job1", jobStreamA, 0, ""))
	if again := epochOf(t, sequenced(s, "app", "job1", jobStreamA, 0, "")); again != claim {
		t.Fatalf("epoch changed between claims: %s %s", claim, again)
	}
	if got := epochOf(t, sequenced(s, "app", "job1", jobStreamA, 1, "ls\r")); got != claim {
		t.Fatalf("epoch changed on data: %s", got)
	}
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 1, "ls\r"))
	if data, writes := job.received(); data != "ls\r" || writes != 1 {
		t.Fatalf("replay reached the task: %q after %d writes", data, writes)
	}
	expectProblem(t, sequenced(s, "app", "job1", jobStreamA, 3, "gap"), 409, "terminal_input_sequence")
	expectProblem(t, sequenced(s, "app", "job1", jobStreamA, 1, "changed"), 409, "terminal_input_sequence")
	if !s.jobInputs.claimed("app", "job1") || s.jobInputs.claimed("app", "job2") || s.jobInputs.claimed("site", "job1") {
		t.Fatal("claim must be scoped to kind and id")
	}
}

func TestJobTerminalDataWithoutClaimNeverWrites(t *testing.T) {
	// After an Agent restart the registry is empty: frames of the old stream
	// must be refused instead of being replayed into a task that may have
	// received them already.
	job := &fakeJob{}
	s := jobInputServer(t, "diagnostic", job)
	expectProblem(t, sequenced(s, "diagnostic", "job1", jobStreamA, 1, "x"), 409, "terminal_input_sequence")
	if _, writes := job.received(); writes != 0 {
		t.Fatalf("unclaimed frame wrote %d times", writes)
	}
}

func TestJobTerminalRefusesToClaimAFinishedOrUnknownTask(t *testing.T) {
	job := &fakeJob{probe: terminal.ErrClosed}
	s := jobInputServer(t, "site", job)
	expectProblem(t, sequenced(s, "site", "job1", jobStreamA, 0, ""), 409, "terminal_closed")
	if len(s.jobInputs.items) != 0 {
		t.Fatalf("a finished task left state behind: %d", len(s.jobInputs.items))
	}
	job.set(terminal.ErrNotFound, nil)
	expectProblem(t, sequenced(s, "site", "job1", jobStreamA, 0, ""), 404, "terminal_not_found")
	expectProblem(t, sequenced(s, "unknown", "job1", jobStreamA, 0, ""), 404, "terminal_not_found")
	// A malformed frame must not allocate state either.
	job.set(nil, nil)
	expectProblem(t, postJobInput(s, "/v1/job-terminals/site/job1/input-sequenced", map[string]any{"frame": map[string]any{"stream": "bad", "seq": 0}}), 409, "terminal_input_sequence")
	if len(s.jobInputs.items) != 0 {
		t.Fatalf("a malformed claim left state behind: %d", len(s.jobInputs.items))
	}
}

func TestJobTerminalForgetsATaskThatFinishesMidStream(t *testing.T) {
	job := &fakeJob{}
	s := jobInputServer(t, "environment", job)
	epochOf(t, sequenced(s, "environment", "job1", jobStreamA, 0, ""))
	job.set(nil, fmt.Errorf("%w: input is closed", appmarket.ErrConflict))
	expectProblem(t, sequenced(s, "environment", "job1", jobStreamA, 1, "x"), 409, "terminal_closed")
	if s.jobInputs.claimed("environment", "job1") {
		t.Fatal("finished task kept its claim")
	}
}

func TestJobTerminalRetriesOnlyWritesThatNeverReachedTheFIFO(t *testing.T) {
	job := &fakeJob{}
	s := jobInputServer(t, "app", job)
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 0, ""))

	job.set(nil, fmt.Errorf("%w: %w", appmarket.ErrConflict, fmt.Errorf("%w: %w", hostpty.ErrNotWritten, errors.New("fifo busy"))))
	expectProblem(t, sequenced(s, "app", "job1", jobStreamA, 1, "cmd\r"), 503, "terminal_input_unavailable")
	job.set(nil, nil)
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 1, "cmd\r"))
	if data, _ := job.received(); data != "cmd\r" {
		t.Fatalf("retry content %q", data)
	}

	job.set(nil, fmt.Errorf("%w: %w", appmarket.ErrConflict, fmt.Errorf("%w: %w", hostpty.ErrPartialWrite, errors.New("fifo busy"))))
	expectProblem(t, sequenced(s, "app", "job1", jobStreamA, 2, "next\r"), 409, "terminal_input_uncertain")
	job.set(nil, nil)
	_, writes := job.received()
	expectProblem(t, sequenced(s, "app", "job1", jobStreamA, 2, "next\r"), 409, "terminal_input_uncertain")
	if _, after := job.received(); after != writes {
		t.Fatal("a possibly delivered frame was written again")
	}
}

func TestJobTerminalReloadedPageFencesTheOldStream(t *testing.T) {
	job := &fakeJob{}
	s := jobInputServer(t, "app", job)
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 0, ""))
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 1, "a"))
	epochOf(t, sequenced(s, "app", "job1", jobStreamB, 0, ""))
	expectProblem(t, sequenced(s, "app", "job1", jobStreamA, 2, "b"), 409, "terminal_input_sequence")
	epochOf(t, sequenced(s, "app", "job1", jobStreamB, 1, "c"))
	if data, _ := job.received(); data != "ac" {
		t.Fatalf("content %q", data)
	}
}

func TestJobInputClassifiesEveryServiceFailure(t *testing.T) {
	notWritten := fmt.Errorf("%w: %w", hostpty.ErrNotWritten, errors.New("no reader"))
	partial := fmt.Errorf("%w: %w", hostpty.ErrPartialWrite, errors.New("busy"))
	cases := []struct {
		kind    string
		backend jobInput
		invalid error
		missing error
		closed  error
	}{
		{"app", jobInput{invalid: appmarket.ErrForbidden, missing: appmarket.ErrNotFound, conflict: appmarket.ErrConflict}, appmarket.ErrForbidden, appmarket.ErrNotFound, appmarket.ErrConflict},
		{"site", jobInput{invalid: sites.ErrInvalidInput, conflict: sites.ErrConflict}, sites.ErrInvalidInput, nil, sites.ErrConflict},
		{"diagnostic", jobInput{invalid: diagnostics.ErrInvalidInput, missing: diagnostics.ErrNotFound, conflict: diagnostics.ErrConflict}, diagnostics.ErrInvalidInput, diagnostics.ErrNotFound, diagnostics.ErrConflict},
		{"environment", jobInput{invalid: webenv.ErrInvalid, missing: webenv.ErrNotFound, conflict: webenv.ErrConflict}, webenv.ErrInvalid, webenv.ErrNotFound, webenv.ErrConflict},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			// The services wrap the FIFO error under their conflict class, so
			// the FIFO outcome must be looked at before the class.
			if err := c.backend.classify(fmt.Errorf("%w: unavailable: %w", c.closed, notWritten)); !errors.Is(err, terminal.ErrInputNotWritten) || errors.Is(err, terminal.ErrClosed) {
				t.Fatalf("zero-byte failure: %v", err)
			}
			err := c.backend.classify(fmt.Errorf("%w: unavailable: %w", c.closed, partial))
			if errors.Is(err, terminal.ErrClosed) || errors.Is(err, terminal.ErrInputNotWritten) || !errors.Is(err, hostpty.ErrPartialWrite) {
				t.Fatalf("possibly delivered bytes must stay uncertain: %v", err)
			}
			if err := c.backend.classify(fmt.Errorf("%w: input is not open", c.closed)); !errors.Is(err, terminal.ErrClosed) {
				t.Fatalf("closed task: %v", err)
			}
			if err := c.backend.classify(fmt.Errorf("%w: bad bytes", c.invalid)); !errors.Is(err, terminal.ErrInputSequence) {
				t.Fatalf("invalid bytes: %v", err)
			}
			if c.missing != nil {
				if err := c.backend.classify(c.missing); !errors.Is(err, terminal.ErrNotFound) {
					t.Fatalf("missing task: %v", err)
				}
			}
			unknown := errors.New("disk on fire")
			if err := c.backend.classify(unknown); err != unknown {
				t.Fatalf("unknown failures must reach the freeze path unchanged: %v", err)
			}
		})
	}
}

func TestJobInputRegistryBoundsAndExpiresEntries(t *testing.T) {
	var registry jobInputRegistry
	now := time.Now()
	for i := 0; i < jobInputMaxEntries; i++ {
		if _, err := registry.claim(fmt.Sprintf("app/%d", i), now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := registry.claim("app/overflow", now); !errors.Is(err, terminal.ErrLimit) {
		t.Fatalf("full registry accepted another task: %v", err)
	}
	if _, err := registry.claim("app/overflow", now.Add(jobInputIdleExpiry+time.Second)); err != nil {
		t.Fatalf("idle entries must be reclaimed: %v", err)
	}
	if len(registry.items) != 1 {
		t.Fatalf("expired entries kept: %d", len(registry.items))
	}
}

func TestClaimedJobRefusesBareInputThroughTheLegacyRoutes(t *testing.T) {
	job := &fakeJob{}
	s := jobInputServer(t, "app", job)
	recorder := httptest.NewRecorder()
	if s.rejectClaimedJobInput(recorder, "id", "app", "job1") {
		t.Fatal("unclaimed task rejected bare input")
	}
	epochOf(t, sequenced(s, "app", "job1", jobStreamA, 0, ""))
	if !s.rejectClaimedJobInput(recorder, "id", "app", "job1") || recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "terminal_input_sequence") {
		t.Fatalf("claimed task accepted bare input: %d %s", recorder.Code, recorder.Body.String())
	}
}
