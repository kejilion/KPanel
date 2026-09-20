package agent

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/systemmanage"
)

func (s *Server) virusScan(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_virus_scan_query", "病毒扫描查询参数无效", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	snapshot, err := s.systemManager.VirusScanSnapshot(ctx)
	if err != nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "virus_scan_unavailable", "病毒扫描状态不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) virusScanAction(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_virus_scan_action", "病毒扫描 URL 无效", "")
		return
	}
	var input contract.VirusScanActionRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	if field, detail := contract.ValidateVirusScanAction(&input); field != "" {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_virus_scan_action", "病毒扫描选项无效", field+": "+detail)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()
	result, err := s.systemManager.ExecuteVirusScanAction(ctx, input)
	if err != nil {
		status, code, title := http.StatusServiceUnavailable, "virus_scan_failed", "病毒扫描任务提交失败"
		switch {
		case errors.Is(err, systemmanage.ErrInvalidInput):
			status, code, title = http.StatusUnprocessableEntity, "invalid_virus_scan_action", "病毒扫描选项无效"
		case errors.Is(err, systemmanage.ErrDisabled), errors.Is(err, systemmanage.ErrUnsupported):
			status, code, title = http.StatusForbidden, "virus_scan_unavailable", "病毒扫描不可用"
		case errors.Is(err, systemmanage.ErrConflict):
			status, code, title = http.StatusConflict, "virus_scan_conflict", "另一项系统维护任务正在运行"
		}
		writeProblem(w, requestID, status, code, title, safeDetail(err))
		return
	}
	writeJSON(w, http.StatusAccepted, result)
}
