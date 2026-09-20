package agent

import (
	"context"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func (s *Server) systemPackages(w http.ResponseWriter, r *http.Request) {
	if !validSystemResourceURL(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	snapshot, err := s.systemManager.SystemPackagesSnapshot(ctx)
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "system_packages_unavailable", "软件包状态不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) systemPackagesAction(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_system_packages_action", "软件包操作 URL 无效", "")
		return
	}
	var input contract.SystemPackagesActionRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	if field, detail := contract.ValidateSystemPackagesAction(&input); field != "" {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_system_packages_action", "软件包操作无效", field+": "+detail)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	result, err := s.systemManager.ExecuteSystemPackagesAction(ctx, input)
	if err != nil {
		status, code, title, retryable := systemResourceProblem(err)
		writeProblemWithRetryable(w, requestID, status, code, title, safeDetail(err), retryable)
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}
