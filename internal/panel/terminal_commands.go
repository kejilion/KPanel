package panel

import (
	"errors"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/terminalcommands"
)

const terminalCommandsPath = "/api/v1/terminal-commands"

func (s *Server) handleTerminalCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "terminal_commands_request_invalid", "Terminal commands request is invalid", "")
		return
	}
	if r.Method == http.MethodPut && !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if s.terminalCommands == nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "terminal_commands_unavailable", "Terminal commands unavailable", "")
		return
	}
	if r.Method == http.MethodGet {
		s.writeJSON(w, http.StatusOK, s.terminalCommands.Snapshot())
		return
	}
	if !s.checkCSRF(w, r, session) {
		return
	}

	var input terminalcommands.ReplaceInput
	if err := decodeLimitedJSON(w, r, terminalcommands.MaxUpdateBytes, &input); err != nil {
		return
	}
	if !terminalcommands.ValidResourceVersion(input.ExpectedResourceVersion) {
		s.writeValidationProblem(w, r, "expectedResourceVersion", "a valid resourceVersion is required")
		return
	}
	change := map[string]any{
		"changedCommands": terminalCommandsChangedCount(s.terminalCommands.Snapshot().Items, input.Items),
	}
	if err := s.audit(r, session.User.ID, "terminal.commands.update", "terminal-commands", "collection", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	snapshot, err := s.terminalCommands.Replace(input)
	if err != nil {
		_ = s.audit(r, session.User.ID, "terminal.commands.update", "terminal-commands", "collection", "failure", change)
		s.writeTerminalCommandsError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "terminal.commands.update", "terminal-commands", "collection", "success", change)
	s.writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) writeTerminalCommandsError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *terminalcommands.ValidationError
	switch {
	case errors.As(err, &validation):
		s.writeValidationProblem(w, r, validation.Field, validation.Detail)
	case errors.Is(err, terminalcommands.ErrConflict):
		s.writeProblem(w, r, http.StatusConflict, "terminal_commands_changed", "Terminal commands changed", "")
	case errors.Is(err, terminalcommands.ErrUnavailable):
		s.writeProblem(w, r, http.StatusServiceUnavailable, "terminal_commands_unavailable", "Terminal commands unavailable", "")
	default:
		s.writeProblem(w, r, http.StatusInternalServerError, "terminal_commands_operation_failed", "Terminal commands operation failed", "")
	}
}

func terminalCommandsChangedCount(current, next []terminalcommands.Command) int {
	currentByID := make(map[string]terminalcommands.Command, len(current))
	nextByID := make(map[string]terminalcommands.Command, len(next))
	for _, item := range current {
		currentByID[item.ID] = item
	}
	for _, item := range next {
		nextByID[item.ID] = item
	}
	changed := 0
	for id, currentItem := range currentByID {
		nextItem, ok := nextByID[id]
		if !ok || currentItem.Name != nextItem.Name || currentItem.Command != nextItem.Command {
			changed++
		}
	}
	for id := range nextByID {
		if _, ok := currentByID[id]; !ok {
			changed++
		}
	}
	return changed
}
