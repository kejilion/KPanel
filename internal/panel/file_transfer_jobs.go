package panel

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filetransfer"
)

func (s *Server) handleFileTransferJobs(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, 400, "file_query_invalid", "传输任务参数无效", "")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		s.writeProblem(w, r, 405, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.Method == http.MethodPost && !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || r.Method == http.MethodPost && !s.checkCSRF(w, r, session) {
		return
	}
	if s.fileTransferJobs == nil {
		s.writeTransferJobError(w, r, filetransfer.ErrUnavailable)
		return
	}
	base := "/api/v1/files/transfer-jobs"
	if r.Method == http.MethodGet && r.URL.Path == base {
		items, err := s.fileTransferJobs.List()
		if err != nil {
			s.writeTransferJobError(w, r, err)
			return
		}
		s.writeJSON(w, 200, map[string]any{"items": items})
		return
	}
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path != base {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, base+"/"), "/")
		if len(parts) != 2 || len(parts[0]) != 32 || (parts[1] != "cancel" && parts[1] != "clear") {
			http.NotFound(w, r)
			return
		}
		if err := s.audit(r, session.User.ID, "file.transfer."+parts[1], "file-transfer-job", parts[0], "intent", nil); err != nil {
			s.writeProblem(w, r, 503, "audit_unavailable", "Audit storage unavailable", "")
			return
		}
		if err := s.fileTransferJobs.Change(parts[0], parts[1]); err != nil {
			s.writeTransferJobError(w, r, err)
			return
		}
		_ = s.audit(r, session.User.ID, "file.transfer."+parts[1], "file-transfer-job", parts[0], "success", nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var input filetransfer.Request
	if s.decodeJSON(w, r, &input) != nil {
		return
	}
	if filetransfer.Validate(input) != nil {
		s.writeTransferJobError(w, r, filetransfer.ErrInvalid)
		return
	}
	if _, err := s.transferTarget(r.Context(), input.TargetHostID, input.SourceNodeID); err != nil {
		s.writeProblem(w, r, 409, "file_host_unavailable", "目标主机未就绪，或来源与目标相同", "")
		return
	}
	if err := s.audit(r, session.User.ID, "file.transfer.create", "file-transfer-job", input.ID, "intent", map[string]any{"sourceNodeId": input.SourceNodeID, "targetHostId": input.TargetHostID, "targetDirectory": input.TargetDirectory, "count": len(input.Items)}); err != nil {
		s.writeProblem(w, r, 503, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	actorID, sourceIP := session.User.ID, s.remoteIP(r)
	job, err := s.fileTransferJobs.Start(input, func(ctx context.Context, source filetransfer.Source, emit func(contract.FileTransferEvent)) {
		kind, err := s.transferTarget(ctx, input.TargetHostID, input.SourceNodeID)
		if err != nil {
			emit(contract.FileTransferEvent{State: "error", Code: "target_unavailable", Detail: "目标主机已不可用，请检查节点连接和授权。"})
			return
		}
		// Keep audit attribution, never a browser session, CSRF token or cookie.
		ctx = context.WithValue(ctx, requestIDKey, input.ID)
		request := &http.Request{Method: http.MethodPost, URL: &url.URL{Path: base}, RemoteAddr: net.JoinHostPort(sourceIP, "0"), Header: make(http.Header)}
		request = request.WithContext(ctx)
		change := map[string]any{"jobId": input.ID, "sourceNodeId": input.SourceNodeID, "targetHostId": input.TargetHostID, "targetDirectory": input.TargetDirectory}
		s.executeFileTransfer(request, actorID, contract.FileTransferRequest{SourceNodeID: input.SourceNodeID, Path: source.Path, ResourceVersion: source.ResourceVersion, TargetDirectory: input.TargetDirectory}, input.TargetHostID, kind, change, emit)
	})
	if err != nil {
		s.writeTransferJobError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) transferTarget(ctx context.Context, id, source string) (cluster.HostKind, error) {
	if s.cluster == nil {
		return "", filetransfer.ErrUnavailable
	}
	if id == "" {
		if source == s.cluster.NodeID() {
			return "", filetransfer.ErrInvalid
		}
		return "", nil
	}
	host, err := s.cluster.Host(ctx, id)
	if err != nil || host.IsLocal || !host.FileManagementAvailable || host.RemoteNodeID == source || (host.Kind != cluster.HostKindPanel && host.Kind != cluster.HostKindLightNode) {
		return "", filetransfer.ErrUnavailable
	}
	return host.Kind, nil
}

func (s *Server) writeTransferJobError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, detail := 503, "file_transfer_jobs_unavailable", "传输任务存储暂不可用，请稍后刷新。"
	switch {
	case errors.Is(err, filetransfer.ErrInvalid):
		status, code, detail = 400, "file_transfer_invalid", "传输参数无效；每批最多 64 项。"
	case errors.Is(err, filetransfer.ErrFull):
		status, code, detail = 429, "file_transfer_queue_full", "传输队列已满，请稍后重试。"
		w.Header().Set("Retry-After", "5")
	case errors.Is(err, filetransfer.ErrNotFound):
		status, code, detail = 404, "file_transfer_job_not_found", "传输任务不存在。"
	case errors.Is(err, filetransfer.ErrActive):
		status, code, detail = 409, "file_transfer_job_active", "请先停止传输，再清除记录。"
	}
	s.writeProblem(w, r, status, code, detail, "")
}
