package agent

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/appmarket"
	"github.com/kejilion/kejilion-panel/internal/diagnostics"
	"github.com/kejilion/kejilion-panel/internal/hostpty"
	"github.com/kejilion/kejilion-panel/internal/sites"
	"github.com/kejilion/kejilion-panel/internal/terminal"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

// Task terminals (application, website, diagnostic and environment jobs) feed
// a FIFO read by a separate job process, so the Agent keeps the sequencing
// state that a PTY owner keeps for host terminals. State is in memory only:
// every claim reports the sequencer's epoch, which lets the browser notice a
// restart instead of replaying frames the previous process may have written.
const (
	jobInputMaxEntries = 1024
	jobInputIdleExpiry = 24 * time.Hour
)

type jobInputRegistry struct {
	mu    sync.Mutex
	items map[string]*jobInputEntry
}

type jobInputEntry struct {
	sequencer *terminal.InputSequencer
	used      time.Time
}

func jobInputKey(kind, id string) string { return kind + "/" + id }

// claim returns the job's sequencer, creating it for a task that was just
// proven to accept input. Entries are bounded and expire when idle.
func (r *jobInputRegistry) claim(key string, now time.Time) (*terminal.InputSequencer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item, ok := r.items[key]; ok {
		item.used = now
		return item.sequencer, nil
	}
	if r.items == nil {
		r.items = make(map[string]*jobInputEntry)
	}
	if len(r.items) >= jobInputMaxEntries {
		for existing, item := range r.items {
			if now.Sub(item.used) > jobInputIdleExpiry {
				delete(r.items, existing)
			}
		}
		if len(r.items) >= jobInputMaxEntries {
			return nil, terminal.ErrLimit
		}
	}
	item := &jobInputEntry{sequencer: terminal.NewInputSequencer(), used: now}
	r.items[key] = item
	return item.sequencer, nil
}

// lookup finds an existing sequencer without creating one. A data frame for a
// task nobody claimed (for example after an Agent restart) must not write.
func (r *jobInputRegistry) lookup(key string, now time.Time) (*terminal.InputSequencer, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[key]
	if !ok {
		return nil, false
	}
	item.used = now
	return item.sequencer, true
}

func (r *jobInputRegistry) forget(key string) {
	r.mu.Lock()
	delete(r.items, key)
	r.mu.Unlock()
}

// claimed reports whether an acknowledged stream owns the task's input.
func (r *jobInputRegistry) claimed(kind, id string) bool {
	r.mu.Lock()
	item, ok := r.items[jobInputKey(kind, id)]
	r.mu.Unlock()
	return ok && item.sequencer.Claimed()
}

// Server.jobInputBackends, when set, replaces these adapters; only tests do.
//
// jobInput adapts one task service. probe is side-effect free and only says
// whether the task still accepts input; write is the service's own validated
// WriteInput. The error values name the service's classes for those failures.
type jobInput struct {
	probe    func(id string) error
	write    func(id string, data string) error
	invalid  error
	missing  error
	conflict error
}

func (s *Server) jobInputFor(kind string) (jobInput, bool) {
	if backend, ok := s.jobInputBackends[kind]; ok {
		return backend, true
	}
	accepting := func(status string, inputOpen bool) error {
		if !inputOpen || (status != "queued" && status != "running") {
			return terminal.ErrClosed
		}
		return nil
	}
	switch kind {
	case "app":
		if s.appMarket == nil {
			return jobInput{}, false
		}
		return jobInput{
			probe: func(id string) error {
				job, err := s.appMarket.AppJob(id)
				if err != nil {
					return terminal.ErrNotFound
				}
				return accepting(job.Status, job.Interactive && job.InputOpen)
			},
			write:    s.appMarket.WriteAppJobInput,
			invalid:  appmarket.ErrForbidden,
			missing:  appmarket.ErrNotFound,
			conflict: appmarket.ErrConflict,
		}, true
	case "site":
		if s.sitesManager == nil {
			return jobInput{}, false
		}
		return jobInput{
			probe: func(id string) error {
				job, err := s.sitesManager.RecipeJob(id)
				if err != nil {
					return terminal.ErrNotFound
				}
				return accepting(job.Status, job.Interactive && job.InputOpen)
			},
			write:    s.sitesManager.WriteInstallationInput,
			invalid:  sites.ErrInvalidInput,
			conflict: sites.ErrConflict,
		}, true
	case "diagnostic":
		if s.diagnostics == nil {
			return jobInput{}, false
		}
		return jobInput{
			probe: func(id string) error {
				job, err := s.diagnostics.Job(id)
				if err != nil {
					return terminal.ErrNotFound
				}
				return accepting(job.Status, job.Interactive && job.InputOpen)
			},
			write:    s.diagnostics.WriteInput,
			invalid:  diagnostics.ErrInvalidInput,
			missing:  diagnostics.ErrNotFound,
			conflict: diagnostics.ErrConflict,
		}, true
	case "environment":
		if s.webEnvironment == nil {
			return jobInput{}, false
		}
		return jobInput{
			probe: func(id string) error {
				job, err := s.webEnvironment.Job(id)
				if err != nil {
					return terminal.ErrNotFound
				}
				return accepting(job.Status, true)
			},
			write:    s.webEnvironment.WriteInput,
			invalid:  webenv.ErrInvalid,
			missing:  webenv.ErrNotFound,
			conflict: webenv.ErrConflict,
		}, true
	}
	return jobInput{}, false
}

// classify turns a service failure into what the sequencer needs to know. A
// failure that provably wrote nothing may be retried; one that may have written
// a prefix, and anything unrecognised, falls through to the sequencer's freeze.
func (j jobInput) classify(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, hostpty.ErrPartialWrite):
		return err
	case errors.Is(err, hostpty.ErrNotWritten):
		return fmt.Errorf("%w: %w", terminal.ErrInputNotWritten, err)
	case j.invalid != nil && errors.Is(err, j.invalid):
		// A well-formed frame never carries NUL or oversize data.
		return terminal.ErrInputSequence
	case j.missing != nil && errors.Is(err, j.missing):
		return terminal.ErrNotFound
	case j.conflict != nil && errors.Is(err, j.conflict):
		return terminal.ErrClosed
	}
	return err
}

func (s *Server) jobTerminalInput(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/job-terminals/"), "/")
	if r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "Invalid task terminal request", "")
		return
	}
	switch {
	case len(parts) == 2 && parts[0] == "capabilities" && parts[1] == "input-protocol":
		var input struct{}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"protocol": terminal.InputProtocol})
	case len(parts) == 3 && parts[2] == "input-sequenced":
		s.jobTerminalSequencedInput(w, r, requestID, parts[0], parts[1])
	default:
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "Task terminal route not found", "")
	}
}

func (s *Server) jobTerminalSequencedInput(w http.ResponseWriter, r *http.Request, requestID, kind, id string) {
	backend, ok := s.jobInputFor(kind)
	if !ok || id == "" {
		writeProblem(w, requestID, http.StatusNotFound, "terminal_not_found", "Task terminal not found", "")
		return
	}
	var input struct {
		Frame terminal.InputFrame `json:"frame"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	frame := input.Frame
	if !frame.Valid() {
		// Reject before any state is created for this task id.
		s.writeTerminalError(w, requestID, terminal.ErrInputSequence)
		return
	}
	key := jobInputKey(kind, id)
	now := time.Now()
	var (
		sequencer *terminal.InputSequencer
		err       error
	)
	if frame.Seq == 0 {
		// Prove the task is open before any state exists for this id.
		if err = backend.probe(id); err == nil {
			sequencer, err = s.jobInputs.claim(key, now)
		}
	} else if found, exists := s.jobInputs.lookup(key, now); exists {
		sequencer = found
	} else {
		err = terminal.ErrInputSequence
	}
	if err == nil {
		err = sequencer.Admit(r.Context(), frame, func(data []byte) error {
			return backend.classify(backend.write(id, string(data)))
		})
	}
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "epoch": sequencer.Epoch()})
	case errors.Is(err, terminal.ErrInputNotWritten):
		writeProblem(w, requestID, http.StatusServiceUnavailable, "terminal_input_unavailable", "Task terminal input temporarily unavailable", "")
	default:
		if errors.Is(err, terminal.ErrClosed) || errors.Is(err, terminal.ErrNotFound) {
			s.jobInputs.forget(key)
		}
		s.writeTerminalError(w, requestID, err)
	}
}

// rejectClaimedJobInput keeps bare writes out once an acknowledged stream owns
// the task's input, so no byte bypasses the sequence and replay accounting.
func (s *Server) rejectClaimedJobInput(w http.ResponseWriter, requestID, kind, id string) bool {
	if !s.jobInputs.claimed(kind, id) {
		return false
	}
	writeProblem(w, requestID, http.StatusConflict, "terminal_input_sequence", "Terminal input stream owns this task", "")
	return true
}
