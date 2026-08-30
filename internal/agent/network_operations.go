package agent

import (
	"context"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func (s *Server) systemPortUsage(w http.ResponseWriter, r *http.Request) {
	if !validSystemResourceURL(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	snapshot, err := s.systemManager.PortUsage(ctx)
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "system_port_usage_unavailable", "端口占用状态不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) systemTrafficShutdown(w http.ResponseWriter, r *http.Request) {
	if !validSystemResourceURL(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	snapshot, err := s.systemManager.TrafficShutdown(ctx)
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "system_traffic_shutdown_unavailable", "限流关机状态不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) systemTrafficShutdownAction(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_traffic_shutdown_action", "限流关机操作 URL 无效", "")
		return
	}
	var input contract.TrafficShutdownActionRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	result, err := s.systemManager.ExecuteTrafficShutdownAction(ctx, input)
	if err != nil {
		status, code, title, retryable := systemResourceProblem(err)
		writeProblemWithRetryable(w, requestID, status, code, title, safeDetail(err), retryable)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
