package agent

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/terminal"
)

type terminalOpenInput struct {
	Owner           string `json:"owner"`
	Rows            uint16 `json:"rows"`
	Columns         uint16 `json:"columns"`
	ContainerID     string `json:"containerId,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

type terminalInput struct {
	Owner string `json:"owner"`
	Data  string `json:"data"`
}

type terminalResizeInput struct {
	Owner   string `json:"owner"`
	Rows    uint16 `json:"rows"`
	Columns uint16 `json:"columns"`
}

func (s *Server) terminalOpen(w http.ResponseWriter, r *http.Request) {
	requestID := r.Header.Get("X-Request-ID")
	if s.terminals == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "terminal_unavailable", "Terminal service unavailable", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_terminal_request", "Invalid terminal request", "")
		return
	}
	var input terminalOpenInput
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	if (input.ContainerID == "") != (input.ResourceVersion == "") {
		writeProblem(w, requestID, http.StatusBadRequest, "terminal_invalid", "Container identity and resource version are required together", "")
		return
	}
	var snapshot terminal.Snapshot
	var err error
	if input.ContainerID != "" {
		if s.docker == nil {
			writeProblem(w, requestID, http.StatusServiceUnavailable, "container_terminal_unavailable", "Container terminal unavailable", "")
			return
		}
		snapshot, err = s.terminals.OpenWithStarter(input.Owner, input.Rows, input.Columns, func(rows, columns uint16) (terminal.Process, error) {
			return s.docker.OpenContainerTerminal(r.Context(), input.ContainerID, input.ResourceVersion, rows, columns)
		})
	} else {
		snapshot, err = s.terminals.Open(input.Owner, input.Rows, input.Columns)
	}
	if err != nil {
		switch {
		case errors.Is(err, dockerx.ErrResourceConflict):
			writeProblem(w, requestID, http.StatusConflict, "resource_conflict", "Container state changed", "")
			return
		case errors.Is(err, dockerx.ErrActionUnsupported), errors.Is(err, dockerx.ErrContainerTerminalUnsupported):
			writeProblem(w, requestID, http.StatusConflict, "container_terminal_unavailable", "Container terminal unavailable", "")
			return
		}
		s.writeTerminalError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusCreated, snapshot)
}

func (s *Server) terminalOperation(w http.ResponseWriter, r *http.Request, requestID string) {
	if s.terminals == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "terminal_unavailable", "Terminal service unavailable", "")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/terminals/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || r.URL.RawPath != "" {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "Terminal route not found", "")
		return
	}
	id, action := parts[0], parts[1]
	if action != "output" && r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_terminal_request", "Invalid terminal request", "")
		return
	}
	switch action {
	case "input-protocol":
		if r.Method != http.MethodPost || id != "capabilities" {
			writeProblem(w, requestID, http.StatusNotFound, "not_found", "Terminal route not found", "")
			return
		}
		var input struct{}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"protocol": terminal.InputProtocol})
	case "input-sequenced":
		if r.Method != http.MethodPost {
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		var input struct {
			Owner string              `json:"owner"`
			Frame terminal.InputFrame `json:"frame"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if err := s.terminals.InputSequencedContext(r.Context(), input.Owner, id, input.Frame); err != nil {
			s.writeTerminalError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	case "output":
		if r.Method != http.MethodGet {
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		s.terminalOutput(w, r, requestID, id)
	case "input":
		if r.Method != http.MethodPost {
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		var input terminalInput
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		data, err := decodeTerminalInput(input.Data)
		if err != nil {
			s.writeTerminalError(w, requestID, errors.New("invalid terminal input"))
			return
		}
		if err := s.terminals.InputContext(r.Context(), input.Owner, id, data); err != nil {
			s.writeTerminalError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	case "resize":
		if r.Method != http.MethodPost {
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		var input terminalResizeInput
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if err := s.terminals.Resize(input.Owner, id, input.Rows, input.Columns); err != nil {
			s.writeTerminalError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"accepted": true})
	case "close":
		if r.Method != http.MethodPost {
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "Request method not allowed", "")
			return
		}
		var input struct {
			Owner string `json:"owner"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			return
		}
		if err := s.terminals.Close(input.Owner, id); err != nil {
			s.writeTerminalError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"closed": true})
	default:
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "Terminal route not found", "")
	}
}

func decodeTerminalInput(value string) ([]byte, error) {
	data, err := base64.RawStdEncoding.DecodeString(value)
	if err == nil {
		return data, nil
	}
	return base64.StdEncoding.DecodeString(value)
}

func (s *Server) terminalOutput(w http.ResponseWriter, r *http.Request, requestID, id string) {
	query := r.URL.Query()
	if len(query) > 3 || len(query["owner"]) != 1 || len(query["offset"]) != 1 || len(query["wait"]) != 1 {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_terminal_query", "Invalid terminal query", "")
		return
	}
	offset, err := strconv.ParseInt(query.Get("offset"), 10, 64)
	if err != nil || offset < 0 {
		s.writeTerminalError(w, requestID, terminal.ErrOffset)
		return
	}
	waitMilliseconds, err := strconv.Atoi(query.Get("wait"))
	if err != nil || waitMilliseconds < 0 || waitMilliseconds > 1500 {
		s.writeTerminalError(w, requestID, errors.New("invalid terminal wait"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(waitMilliseconds+250)*time.Millisecond)
	defer cancel()
	output, err := s.terminals.Output(ctx, query.Get("owner"), id, offset, time.Duration(waitMilliseconds)*time.Millisecond)
	if err != nil {
		s.writeTerminalError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, output)
}

func (s *Server) writeTerminalError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, dockerx.ErrContainerShellUnavailable):
		writeProblem(w, requestID, http.StatusConflict, "container_shell_unavailable", "Container shell unavailable", "The container does not contain the required /bin/sh")
	case errors.Is(err, terminal.ErrInputSequence):
		writeProblem(w, requestID, http.StatusConflict, "terminal_input_sequence", "Terminal input sequence is invalid", "")
	case errors.Is(err, terminal.ErrInputUncertain):
		writeProblem(w, requestID, http.StatusConflict, "terminal_input_uncertain", "Terminal input is uncertain; open a new terminal", "")
	case errors.Is(err, terminal.ErrNotFound):
		writeProblem(w, requestID, http.StatusNotFound, "terminal_not_found", "Terminal session not found", "")
	case errors.Is(err, terminal.ErrLimit):
		writeProblem(w, requestID, http.StatusTooManyRequests, "terminal_limit", "Terminal session limit reached", "")
	case errors.Is(err, terminal.ErrCleanupPending):
		writeProblem(w, requestID, http.StatusConflict, "terminal_cleanup_pending", "Terminal process cleanup is pending", "Retry closing this terminal")
	case errors.Is(err, terminal.ErrClosed):
		writeProblem(w, requestID, http.StatusConflict, "terminal_closed", "Terminal session is closed", "")
	default:
		writeProblem(w, requestID, http.StatusBadRequest, "terminal_invalid", "Terminal request failed", err.Error())
	}
}
