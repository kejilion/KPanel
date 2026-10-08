package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func (s *Server) handleFileReceives(w http.ResponseWriter, r *http.Request) {
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && (!s.checkOrigin(w, r) || !s.checkCSRF(w, r, session)) {
		return
	}
	s.proxyFileReceives(w, r, false, session.User.ID)
}

func (s *Server) proxyFileReceives(w http.ResponseWriter, r *http.Request, federated bool, userID string) {
	if r.URL.RawPath != "" || !strictPanelQuery(r.URL.Query(), "id", "sourceKey", "offset", "sha256") ||
		(r.Method == http.MethodGet && !strictPanelQuery(r.URL.Query(), "id", "sourceKey")) ||
		(r.Method == http.MethodPost && r.URL.RawQuery != "") {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "传输会话参数无效", "")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, POST, PUT")
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_stream_unavailable", "Agent 文件流不可用", "")
		return
	}
	var body io.Reader = http.NoBody
	length := int64(0)
	headers := make(http.Header)
	var command contract.FileReceiveRequest
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if federated {
			if !s.decodeFederatedJSON(w, r, 64<<10, &command, "传输会话请求无效") {
				return
			}
		} else if s.decodeJSON(w, r, &command) != nil {
			return
		}
		payload, err := json.Marshal(command)
		if err != nil {
			s.writeProblem(w, r, http.StatusBadRequest, "file_request_invalid", "传输会话请求无效", "")
			return
		}
		body, length = bytes.NewReader(payload), int64(len(payload))
		headers.Set("Content-Type", "application/json")
	} else if r.Method == http.MethodPut {
		if r.ContentLength > contract.FileTransferChunkBytes {
			s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "传输分块超过上限", "")
			return
		}
		if _, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64); err != nil {
			s.writeProblem(w, r, http.StatusBadRequest, "file_request_invalid", "传输位置无效", "")
			return
		}
		body, length = http.MaxBytesReader(w, r.Body, contract.FileTransferChunkBytes), r.ContentLength
		headers.Set("Content-Type", r.Header.Get("Content-Type"))
	}
	audited := !federated && r.Method == http.MethodPost
	if audited && s.audit(r, userID, "file.transfer."+command.Operation, "file-transfer-session", command.ID, "intent", nil) != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), panelFileTransferMaxDuration)
	defer cancel()
	response, err := streamer.OpenStream(ctx, r.Method, "/v1/files/transfer/sessions", r.URL.RawQuery, requestID(r), body, headers, length)
	if err != nil {
		if audited {
			_ = s.audit(r, userID, "file.transfer."+command.Operation, "file-transfer-session", command.ID, "failure", nil)
		}
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	defer response.Body.Close()
	if audited {
		outcome := "failure"
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			outcome = "success"
		}
		_ = s.audit(r, userID, "file.transfer."+command.Operation, "file-transfer-session", command.ID, outcome, nil)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(response.Body, 64<<10))
}

func (s *Server) abortFileReceive(id, key string, target fileReceiveTarget) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = target.command(ctx, contract.FileReceiveRequest{Operation: "abort", ID: id, SourceKey: key})
}
