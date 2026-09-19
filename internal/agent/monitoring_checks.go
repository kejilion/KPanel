package agent

import (
	"errors"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

func (s *Server) monitoringChecks(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPut)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.ContentLength > monitoring.MaxCheckUpdateBytes {
		writeProblem(w, requestID, http.StatusBadRequest, "monitoring_checks_request_invalid", "监控检测请求无效", "")
		return
	}
	provider, ok := s.monitoring.(monitoringChecksProvider)
	if !ok {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "monitoring_unavailable", "历史监控不可用", "")
		return
	}
	if r.Method == http.MethodGet {
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			writeProblem(w, requestID, http.StatusBadRequest, "monitoring_checks_request_invalid", "监控检测请求无效", "")
			return
		}
		writeJSON(w, http.StatusOK, provider.Checks())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, monitoring.MaxCheckUpdateBytes)
	var input monitoring.ReplaceChecksInput
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	snapshot, err := provider.ReplaceChecks(input)
	if err != nil {
		var validation *monitoring.CheckValidationError
		switch {
		case errors.As(err, &validation):
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "validation_failed", "Validation failed", validation.Detail)
		case errors.Is(err, monitoring.ErrChecksConflict):
			writeProblem(w, requestID, http.StatusConflict, "monitoring_checks_changed", "监控检测项已被修改", "")
		case errors.Is(err, monitoring.ErrChecksUnavailable):
			writeProblem(w, requestID, http.StatusServiceUnavailable, "monitoring_checks_unavailable", "监控检测配置不可用", "")
		default:
			writeProblem(w, requestID, http.StatusInternalServerError, "monitoring_checks_update_failed", "监控检测项更新失败", "")
		}
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func hasMonitoringChecks(provider monitoringHistoryProvider) bool {
	_, ok := provider.(monitoringChecksProvider)
	return ok
}
