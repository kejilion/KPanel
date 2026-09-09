package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func (s *Server) handleFileArchives(w http.ResponseWriter, r *http.Request) {
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && (!s.checkOrigin(w, r) || !s.checkCSRF(w, r, session)) {
		return
	}
	s.proxyFileArchives(w, r, false, session.User.ID)
}

// Remote callers already passed the shared authenticated file relay.
func (s *Server) proxyFileArchives(w http.ResponseWriter, r *http.Request, federated bool, userID string) {
	agentPath := strings.TrimPrefix(r.URL.Path, "/api")
	contents := agentPath == "/v1/files/archive-contents"
	var auditAction, auditTargetKind, auditTargetID string
	var auditChange map[string]any
	auditEnabled := false
	if r.URL.RawPath != "" || (!contents && !strictPanelQuery(r.URL.Query(), "id")) || (contents && r.URL.RawQuery != "") || (r.Method == http.MethodPost && r.URL.RawQuery != "") {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	if r.Method != http.MethodPost && (r.Method != http.MethodGet || contents) {
		w.Header().Set("Allow", "GET, POST")
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	var payload []byte
	if r.Method == http.MethodPost {
		var input any
		if contents {
			input = &contract.FileArchiveQuery{}
		} else {
			input = &contract.FileArchiveJobRequest{}
		}
		if federated {
			if !s.decodeFederatedJSON(w, r, 64<<10, input, "文件请求无效") {
				return
			}
		} else {
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
			if s.decodeJSON(w, r, input) != nil {
				return
			}
		}
		var err error
		if request, ok := input.(*contract.FileArchiveJobRequest); ok {
			validCreate := request.Operation == "create" && request.Input != nil && request.ID == "" &&
				(request.Input.Action == "compress" || request.Input.Action == "extract")
			valid := validCreate ||
				((request.Operation == "cancel" || request.Operation == "clear") && request.Input == nil && ownerJobIDPattern.MatchString(request.ID))
			if !valid {
				s.writeProblem(w, r, http.StatusBadRequest, "file_request_invalid", "文件请求无效", "")
				return
			}
		}
		payload, err = json.Marshal(input)
		if err != nil {
			s.writeProblem(w, r, http.StatusBadRequest, "file_request_invalid", "文件请求无效", "")
			return
		}
		if !federated && !contents {
			request := input.(*contract.FileArchiveJobRequest)
			auditAction, auditTargetKind, auditTargetID, auditChange = fileArchiveAudit(*request)
			auditEnabled = true
			if err := s.audit(r, userID, auditAction, auditTargetKind, auditTargetID, "intent", cloneFileArchiveAuditChange(auditChange)); err != nil {
				s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
				return
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	var response AgentResponse
	var err error
	if stream, ok := s.agent.(agentStreamAPI); ok {
		headers := http.Header{"Content-Type": []string{"application/json"}, "Accept": []string{"application/json"}}
		remote, streamErr := stream.OpenStream(ctx, r.Method, agentPath, r.URL.RawQuery, requestID(r), bytes.NewReader(payload), headers, int64(len(payload)))
		if streamErr != nil {
			err = streamErr
		} else {
			defer remote.Body.Close()
			body, readErr := io.ReadAll(io.LimitReader(remote.Body, (4<<20)+1))
			err = readErr
			if len(body) > 4<<20 {
				if auditEnabled {
					_ = s.audit(r, userID, auditAction, auditTargetKind, auditTargetID, "failure", cloneFileArchiveAuditChange(auditChange))
				}
				s.writeProblem(w, r, http.StatusBadGateway, "archive_response_invalid", "归档响应超过上限", "")
				return
			}
			response = AgentResponse{StatusCode: remote.StatusCode, ContentType: remote.Header.Get("Content-Type"), Body: body}
		}
	} else {
		response, err = s.agent.Do(ctx, r.Method, agentPath, r.URL.RawQuery, requestID(r), payload)
	}
	if err != nil {
		if auditEnabled {
			_ = s.audit(r, userID, auditAction, auditTargetKind, auditTargetID, "failure", cloneFileArchiveAuditChange(auditChange))
		}
		s.writeProblem(w, r, http.StatusServiceUnavailable, "archive_unavailable", "暂时无法确认归档状态，请刷新后重试", "")
		return
	}
	if auditEnabled {
		outcome, change := acceptedJobAudit(response, "file-archive", cloneFileArchiveAuditChange(auditChange))
		_ = s.audit(r, userID, auditAction, auditTargetKind, auditTargetID, outcome, change)
	}
	s.writeAgentResponse(w, r, response)
}

func fileArchiveAudit(request contract.FileArchiveJobRequest) (action, targetKind, targetID string, change map[string]any) {
	if request.Operation != "create" || request.Input == nil {
		return "file.archive." + request.Operation, "file-archive-job", request.ID, nil
	}
	input := request.Input
	change = map[string]any{
		"sourceCount": len(input.Sources),
		"memberCount": len(input.ArchiveEntries),
	}
	if input.Name != "" && (input.Action == "compress" || len(input.Sources) == 1) {
		change["name"] = input.Name
	}
	if input.Format != "" && input.Action == "compress" {
		change["format"] = input.Format
	}
	return "file." + input.Action, "file", input.Target, change
}

func cloneFileArchiveAuditChange(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}
