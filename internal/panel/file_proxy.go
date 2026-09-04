package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/httpstream"
)

// federatedFileHandler is installed behind the authenticated v2 Panel relay.
// It deliberately calls only the same Agent file methods as the local file
// page; browser session and CSRF checks have already happened on the source
// Panel, while path, method and body contracts are checked again here.
func (s *Server) federatedFileHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestIDValue := r.Header.Get("X-Request-ID")
		if requestIDValue == "" {
			requestIDValue = newRequestID()
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, requestIDValue))
		if r.URL.RawPath != "" || !cluster.FileRelayRequestPath(r.URL.Path) {
			s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
			return
		}
		switch r.URL.Path {
		case "/v1/files":
			s.federatedFileList(w, r)
		case "/v1/files/entry":
			s.federatedFileEntry(w, r)
		case "/v1/files/entries":
			s.federatedFileEntries(w, r)
		case "/v1/files/trash":
			s.federatedFileTrash(w, r)
		case "/v1/files/content":
			s.federatedFileContent(w, r)
		case "/v1/files/archive":
			s.federatedFileArchive(w, r)
		case "/v1/files/text":
			s.federatedFileText(w, r)
		case "/v1/files/tail":
			s.federatedFileTail(w, r)
		case "/v1/files/upload":
			s.federatedFileUpload(w, r)
		case "/v1/files/transfer/export":
			s.federatedFileTransferExport(w, r)
		case "/v1/files/transfer/import":
			s.federatedFileTransferImport(w, r)
		case "/v1/files/actions":
			s.federatedFileAction(w, r)
		default:
			s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
		}
	})
}

func federatedFileQuery(r *http.Request, allowed ...string) (url.Values, bool) {
	if r == nil || r.URL == nil || r.URL.RawPath != "" {
		return nil, false
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || !strictPanelQuery(values, allowed...) {
		return nil, false
	}
	return values, true
}

func (s *Server) federatedFileList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, ok := federatedFileQuery(r, "path", "limit", "offset", "search"); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	response, err := s.agent.Get(r.Context(), "/v1/files", r.URL.RawQuery, requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) federatedFileEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	values, ok := federatedFileQuery(r, "path")
	if !ok || values.Get("path") == "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	response, err := s.agent.Get(r.Context(), "/v1/files/entry", r.URL.RawQuery, requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) federatedFileEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, ok := federatedFileQuery(r); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	var input contract.FileEntryBatchRequest
	if !s.decodeFederatedJSON(w, r, 1<<20, &input, "文件请求无效") {
		return
	}
	if len(input.Paths) == 0 || len(input.Paths) > contract.MaxFileEntryBatch {
		s.writeProblem(w, r, http.StatusBadRequest, "file_request_invalid", "文件请求无效", "")
		return
	}
	seen := make(map[string]struct{}, len(input.Paths))
	for _, filePath := range input.Paths {
		if !validFileDownloadPath(filePath) {
			s.writeProblem(w, r, http.StatusUnprocessableEntity, "validation_failed", "文件路径无效", "")
			return
		}
		if _, exists := seen[filePath]; exists {
			s.writeProblem(w, r, http.StatusUnprocessableEntity, "validation_failed", "文件路径重复", "")
			return
		}
		seen[filePath] = struct{}{}
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	response, err := s.agent.Do(r.Context(), http.MethodPost, "/v1/files/entries", "", requestID(r), body)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) federatedFileTrash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, ok := federatedFileQuery(r); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "回收站查询参数无效", "")
		return
	}
	response, err := s.agent.Get(r.Context(), "/v1/files/trash", "", requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) federatedFileContent(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		if _, ok := federatedFileQuery(r, "path", "disposition", "mode", "version"); !ok {
			s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
			return
		}
		s.streamFileDownload(w, r, r.URL.RawQuery)
	case http.MethodPut:
		s.federatedFileWrite(w, r)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead+", "+http.MethodPut)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) federatedFileWrite(w http.ResponseWriter, r *http.Request) {
	if _, ok := federatedFileQuery(r, "path"); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "json_required", "JSON request body required", "")
		return
	}
	var input contract.FileWriteRequest
	if !s.decodeFederatedJSON(w, r, filemanager.MaxTextBytes+(64<<10), &input, "文件内容无效") {
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	response, err := s.agent.Do(r.Context(), http.MethodPut, "/v1/files/content", r.URL.RawQuery, requestID(r), body)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) federatedFileArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	values, ok := federatedFileQuery(r, "selection", "name")
	if !ok || values.Get("selection") == "" || len(values.Get("selection")) > panelFileArchiveQueryMaxBytes ||
		values.Get("name") == "" || len(values.Get("name")) > 1024 {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "压缩下载参数无效", "")
		return
	}
	s.streamFileArchiveDownload(w, r)
}

func (s *Server) federatedFileText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, ok := federatedFileQuery(r, "path"); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	response, err := s.agent.Get(r.Context(), "/v1/files/text", r.URL.RawQuery, requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) federatedFileTail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, ok := federatedFileQuery(r, "path", "maxBytes"); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	response, err := s.agent.Get(r.Context(), "/v1/files/tail", r.URL.RawQuery, requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) federatedFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, ok := federatedFileQuery(r, "path", "name", "overwrite"); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "上传参数无效", "")
		return
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/octet-stream" {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "binary_required", "上传必须使用二进制内容", "")
		return
	}
	if r.ContentLength > filemanager.MaxUploadBytes {
		s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "文件超过 512 MiB", "")
		return
	}
	s.streamFederatedAgent(w, r, http.MethodPost, "/v1/files/upload", r.URL.RawQuery, r.Body, r.ContentLength)
}

func (s *Server) federatedFileTransferExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, ok := federatedFileQuery(r, "path", "resourceVersion"); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "传输参数无效", "")
		return
	}
	s.streamFederatedAgent(w, r, http.MethodGet, "/v1/files/transfer/export", r.URL.RawQuery, http.NoBody, 0)
}

func (s *Server) federatedFileTransferImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, ok := federatedFileQuery(r, "path", "name", "kind", "size"); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "传输参数无效", "")
		return
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/octet-stream" {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "binary_required", "传输必须使用二进制内容", "")
		return
	}
	s.streamFederatedAgent(w, r, http.MethodPost, "/v1/files/transfer/import", r.URL.RawQuery, r.Body, r.ContentLength)
}

func (s *Server) federatedFileAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if _, ok := federatedFileQuery(r); !ok {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件操作参数无效", "")
		return
	}
	var input contract.FileActionRequest
	if !s.decodeFederatedJSON(w, r, 1<<20, &input, "文件操作参数无效") {
		return
	}
	if !allowedFileAction(input.Action) {
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "validation_failed", "不支持的文件操作", "")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	response, err := s.doFileAction(r, input.Action, body)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) decodeFederatedJSON(w http.ResponseWriter, r *http.Request, limit int64, target any, title string) bool {
	if limit < 1 {
		limit = 1
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		s.writeProblem(w, r, http.StatusBadRequest, "request_invalid", title, "")
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, "request_invalid", title, "")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		s.writeProblem(w, r, http.StatusBadRequest, "request_invalid", title, "")
		return false
	}
	return true
}

func (s *Server) streamFederatedAgent(
	w http.ResponseWriter,
	r *http.Request,
	method, agentPath, rawQuery string,
	body io.Reader,
	contentLength int64,
) {
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_stream_unavailable", "Agent 文件流不可用", "")
		return
	}
	if body == nil {
		body = http.NoBody
	}
	transferContext, cancel := context.WithTimeout(r.Context(), panelFileTransferMaxDuration)
	defer cancel()
	if body != http.NoBody {
		body = httpstream.NewIdleReader(transferContext, w, body, panelFileTransferIdleTimeout)
	}
	headers := make(http.Header)
	if value := r.Header.Get("Content-Type"); value != "" {
		headers.Set("Content-Type", value)
	}
	response, err := streamer.OpenStream(transferContext, method, agentPath, rawQuery, requestID(r), body, headers, contentLength)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	defer response.Body.Close()
	copyFileHeaders(w.Header(), response.Header)
	if value := response.Header.Get(cluster.FileTransferMetadataHeader); value != "" {
		w.Header().Set(cluster.FileTransferMetadataHeader, value)
	}
	if response.Header.Get("Content-Length") == "" && response.ContentLength >= 0 {
		w.Header().Set("Content-Length", formatContentLength(response.ContentLength))
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	writer := httpstream.NewIdleResponseWriter(transferContext, w, panelFileTransferIdleTimeout)
	writer.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	// Transfer exports can legitimately contain a large file or directory;
	// the Agent already applies the transfer contract and the relay applies
	// the request/session duration limits. Do not truncate the stream here.
	_, _ = io.CopyBuffer(writer, response.Body, make([]byte, 64<<10))
}

func formatContentLength(value int64) string {
	if value < 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}
