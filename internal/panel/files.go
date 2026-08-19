package panel

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/httpstream"
)

const (
	panelFileTransferIdleTimeout  = 45 * time.Second
	panelFileTransferMaxDuration  = 2 * time.Hour
	fileDownloadTicketTTL         = 5 * time.Minute
	maxFileDownloadTickets        = 128
	panelFileArchiveQueryMaxBytes = 256 << 10
	desktopFileTransferDirectory  = "/home/KPanel Desktop"
)

type fileDownloadTicket struct {
	Path      string
	ExpiresAt time.Time
}

type fileDownloadTicketRequest struct {
	Path string `json:"path"`
}

type fileDownloadTicketResponse struct {
	DownloadURL string    `json:"downloadUrl"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

var errFileDownloadTicketLimit = errors.New("file download ticket limit reached")

type agentStreamAPI interface {
	OpenStream(
		ctx context.Context,
		method, path, rawQuery, requestID string,
		body io.Reader,
		headers http.Header,
		contentLength int64,
	) (*http.Response, error)
}

func (s *Server) handleFileList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || !strictPanelQuery(r.URL.Query(), "path", "limit", "offset", "search") {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	_, _, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	response, err := s.agent.Get(r.Context(), "/v1/files", r.URL.RawQuery, requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleFileEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || !strictPanelQuery(r.URL.Query(), "path") || r.URL.Query().Get("path") == "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	response, err := s.agent.Get(r.Context(), "/v1/files/entry", r.URL.RawQuery, requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleFileEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input contract.FileEntryBatchRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if len(input.Paths) == 0 || len(input.Paths) > contract.MaxFileEntryBatch {
		s.writeProblem(w, r, http.StatusBadRequest, "file_request_invalid", "文件请求无效", "")
		return
	}
	seen := make(map[string]struct{}, len(input.Paths))
	for _, filePath := range input.Paths {
		if !validFileDownloadPath(filePath) {
			s.writeValidationProblem(w, r, "paths", "all file paths must be canonical absolute paths")
			return
		}
		if _, exists := seen[filePath]; exists {
			s.writeValidationProblem(w, r, "paths", "duplicate file paths are not allowed")
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

func (s *Server) handleFileTrashList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "回收站查询参数无效", "")
		return
	}
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	response, err := s.agent.Get(r.Context(), "/v1/files/trash", "", requestID(r))
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleFileContent(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.handleFileDownload(w, r)
	case http.MethodPut:
		s.handleFileWrite(w, r)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead+", "+http.MethodPut)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
	}
}

func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || !strictPanelQuery(r.URL.Query(), "path", "disposition", "mode", "version") {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	_, _, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	s.streamFileDownload(w, r, r.URL.RawQuery)
}

func (s *Server) handleFileArchiveDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || !strictPanelQuery(r.URL.Query(), "selection", "name") {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "压缩下载参数无效", "")
		return
	}
	if selection, name := r.URL.Query().Get("selection"), r.URL.Query().Get("name"); selection == "" || len(selection) > panelFileArchiveQueryMaxBytes || name == "" || len(name) > 1024 {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "压缩下载参数无效", "")
		return
	}
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	s.streamFileArchiveDownload(w, r)
}

func (s *Server) handleFileDownloadTicketCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input fileDownloadTicketRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if !validFileDownloadPath(input.Path) {
		s.writeValidationProblem(w, r, "path", "file path must be a canonical absolute path")
		return
	}
	token, expiresAt, err := s.issueFileDownloadTicket(input.Path)
	if errors.Is(err, errFileDownloadTicketLimit) {
		s.writeProblem(w, r, http.StatusTooManyRequests, "file_download_ticket_limit", "下载请求过多，请稍后重试", "")
		return
	}
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "file_download_ticket_unavailable", "无法创建下载链接", "")
		return
	}
	s.writeJSON(w, http.StatusCreated, fileDownloadTicketResponse{
		DownloadURL: "/api/v1/files/download/" + token,
		ExpiresAt:   expiresAt,
	})
}

func (s *Server) handleFileDownloadTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "file_download_not_found", "下载链接无效或已过期", "")
		return
	}
	key, ok := fileDownloadTicketKey(strings.TrimPrefix(r.URL.Path, "/api/v1/files/download/"))
	if !ok {
		s.writeProblem(w, r, http.StatusNotFound, "file_download_not_found", "下载链接无效或已过期", "")
		return
	}
	ticket, ok := s.lookupFileDownloadTicket(key)
	if !ok {
		s.writeProblem(w, r, http.StatusNotFound, "file_download_not_found", "下载链接无效或已过期", "")
		return
	}
	query := url.Values{"path": []string{ticket.Path}, "disposition": []string{"attachment"}}
	s.streamFileDownload(w, r, query.Encode())
}

func (s *Server) streamFileDownload(w http.ResponseWriter, r *http.Request, rawQuery string) {
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_stream_unavailable", "Agent 文件流不可用", "")
		return
	}
	headers := make(http.Header)
	for _, key := range []string{"Range", "If-None-Match", "If-Modified-Since", "If-Range"} {
		if value := r.Header.Get(key); value != "" {
			headers.Set(key, value)
		}
	}
	transferContext, cancel := context.WithTimeout(r.Context(), panelFileTransferMaxDuration)
	defer cancel()
	response, err := streamer.OpenStream(
		transferContext, r.Method, "/v1/files/content", rawQuery,
		requestID(r), http.NoBody, headers, 0,
	)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	defer response.Body.Close()
	copyFileHeaders(w.Header(), response.Header)
	// net/http stores a received Content-Length on Response.ContentLength and
	// removes it from Response.Header. Preserve it before writing the response;
	// ranged media requests need the exact segment length to start playback.
	if response.Header.Get("Content-Length") == "" && response.ContentLength >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	writer := httpstream.NewIdleResponseWriter(
		transferContext, w, panelFileTransferIdleTimeout,
	)
	writer.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.CopyBuffer(writer, response.Body, make([]byte, 64<<10))
}

func (s *Server) streamFileArchiveDownload(w http.ResponseWriter, r *http.Request) {
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_stream_unavailable", "Agent 文件流不可用", "")
		return
	}
	transferContext, cancel := context.WithTimeout(r.Context(), panelFileTransferMaxDuration)
	defer cancel()
	response, err := streamer.OpenStream(
		transferContext, http.MethodGet, "/v1/files/archive", r.URL.RawQuery,
		requestID(r), http.NoBody, nil, 0,
	)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	defer response.Body.Close()
	copyFileHeaders(w.Header(), response.Header)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	writer := httpstream.NewIdleResponseWriter(transferContext, w, panelFileTransferIdleTimeout)
	writer.WriteHeader(response.StatusCode)
	_, _ = io.CopyBuffer(writer, response.Body, make([]byte, 64<<10))
}

func validFileDownloadPath(value string) bool {
	return value != "" && len(value) <= 4096 && strings.HasPrefix(value, "/") &&
		!strings.ContainsAny(value, "\\\x00") && path.Clean(value) == value
}

func isFileDownloadTicketPath(requestPath string) bool {
	if !strings.HasPrefix(requestPath, "/api/v1/files/download/") {
		return false
	}
	_, ok := fileDownloadTicketKey(strings.TrimPrefix(requestPath, "/api/v1/files/download/"))
	return ok
}

func fileDownloadTicketKey(token string) ([32]byte, bool) {
	if len(token) != 43 {
		return [32]byte{}, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return [32]byte{}, false
	}
	return sha256.Sum256(decoded), true
}

func (s *Server) issueFileDownloadTicket(filePath string) (string, time.Time, error) {
	now := time.Now().UTC()
	s.downloadTicketMu.Lock()
	defer s.downloadTicketMu.Unlock()
	if s.downloadTickets == nil {
		s.downloadTickets = make(map[[32]byte]fileDownloadTicket)
	}
	for key, ticket := range s.downloadTickets {
		if !ticket.ExpiresAt.After(now) {
			delete(s.downloadTickets, key)
		}
	}
	if len(s.downloadTickets) >= maxFileDownloadTickets {
		return "", time.Time{}, errFileDownloadTicketLimit
	}
	for range 3 {
		value := make([]byte, 32)
		if _, err := rand.Read(value); err != nil {
			return "", time.Time{}, err
		}
		token := base64.RawURLEncoding.EncodeToString(value)
		key := sha256.Sum256(value)
		if _, exists := s.downloadTickets[key]; exists {
			continue
		}
		expiresAt := now.Add(fileDownloadTicketTTL)
		s.downloadTickets[key] = fileDownloadTicket{Path: filePath, ExpiresAt: expiresAt}
		return token, expiresAt, nil
	}
	return "", time.Time{}, errors.New("file download ticket collision")
}

func (s *Server) lookupFileDownloadTicket(key [32]byte) (fileDownloadTicket, bool) {
	now := time.Now().UTC()
	s.downloadTicketMu.Lock()
	defer s.downloadTicketMu.Unlock()
	ticket, ok := s.downloadTickets[key]
	if !ok || !ticket.ExpiresAt.After(now) {
		delete(s.downloadTickets, key)
		return fileDownloadTicket{}, false
	}
	return ticket, true
}

func (s *Server) handleFileWrite(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || !strictPanelQuery(r.URL.Query(), "path") {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件查询参数无效", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "json_required", "JSON request body required", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, filemanager.MaxTextBytes+(64<<10))
	var input contract.FileWriteRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		s.writeProblem(w, r, http.StatusBadRequest, "file_content_invalid", "文件内容无效", "")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		s.writeProblem(w, r, http.StatusBadRequest, "file_content_invalid", "文件内容无效", "")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	target := r.URL.Query().Get("path")
	change := map[string]any{"bytes": len(input.Content), "resourceVersion": input.ExpectedResourceVersion}
	if err := s.audit(r, session.User.ID, "file.write", "file", target, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.agent.Do(
		r.Context(), http.MethodPut, "/v1/files/content", r.URL.RawQuery, requestID(r), body,
	)
	if err != nil {
		_ = s.audit(r, session.User.ID, "file.write", "file", target, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, "file.write", "file", target, result, change)
	s.writeAgentResponse(w, r, response)
}

func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || !strictPanelQuery(r.URL.Query(), "path", "name", "overwrite") {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "上传参数无效", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/octet-stream" {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "binary_required", "上传必须使用二进制内容", "")
		return
	}
	if r.ContentLength > filemanager.MaxUploadBytes {
		s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "file_too_large", "文件超过 512 MiB", "")
		return
	}
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_stream_unavailable", "Agent 文件流不可用", "")
		return
	}
	target := path.Join(r.URL.Query().Get("path"), r.URL.Query().Get("name"))
	change := map[string]any{"bytes": r.ContentLength, "overwrite": r.URL.Query().Get("overwrite") == "true"}
	if err := s.audit(r, session.User.ID, "file.upload", "file", target, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/octet-stream")
	transferContext, cancel := context.WithTimeout(r.Context(), panelFileTransferMaxDuration)
	defer cancel()
	content := httpstream.NewIdleReader(
		transferContext, w, r.Body, panelFileTransferIdleTimeout,
	)
	response, err := streamer.OpenStream(
		transferContext, http.MethodPost, "/v1/files/upload", r.URL.RawQuery,
		requestID(r), content, headers, r.ContentLength,
	)
	if err != nil {
		_ = s.audit(r, session.User.ID, "file.upload", "file", target, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	defer response.Body.Close()
	result := "failure"
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		result = "success"
	}
	_ = s.audit(r, session.User.ID, "file.upload", "file", target, result, change)
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json; charset=utf-8"
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(response.StatusCode)
	_, _ = io.CopyBuffer(w, io.LimitReader(response.Body, 1<<20), make([]byte, 32<<10))
}

func (s *Server) handleFileTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "传输参数无效", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input contract.FileTransferRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if input.SourceNodeID == "" || !validFileDownloadPath(input.Path) || input.Path == "/" ||
		input.ResourceVersion == "" || !validFileDownloadPath(input.TargetDirectory) {
		s.writeProblem(w, r, http.StatusBadRequest, "file_transfer_invalid", "跨面板传输参数无效", "")
		return
	}
	change := map[string]any{
		"sourceNodeId":    input.SourceNodeID,
		"targetDirectory": input.TargetDirectory,
	}
	if err := s.audit(r, session.User.ID, "file.transfer.copy", "file-transfer", input.SourceNodeID, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	flusher, _ := w.(http.Flusher)
	writeEvent := func(event contract.FileTransferEvent) {
		_ = encoder.Encode(event)
		if flusher != nil {
			flusher.Flush()
		}
	}
	writeEvent(contract.FileTransferEvent{State: "connecting"})

	transferContext, cancel := context.WithTimeout(r.Context(), panelFileTransferMaxDuration)
	defer cancel()
	if err := s.ensureFileTransferDirectory(transferContext, input.TargetDirectory, requestID(r)); err != nil {
		_ = s.audit(r, session.User.ID, "file.transfer.copy", "file-transfer", input.SourceNodeID, "failure", change)
		writeEvent(contract.FileTransferEvent{State: "error", Detail: "目标目录不存在或不可写。"})
		return
	}
	content, metadata, err := s.cluster.OpenRemoteFileV2(
		transferContext, input.SourceNodeID,
		cluster.FederationFileOpenRequest{Path: input.Path, ResourceVersion: input.ResourceVersion},
	)
	if err != nil {
		_ = s.audit(r, session.User.ID, "file.transfer.copy", "file-transfer", input.SourceNodeID, "failure", change)
		writeEvent(contract.FileTransferEvent{State: "error", Detail: "无法连接来源 KPanel，或配对未授权文件复制。"})
		return
	}
	defer content.Close()
	if metadata.Name != path.Base(input.Path) || metadata.ResourceVersion != input.ResourceVersion {
		_ = s.audit(r, session.User.ID, "file.transfer.copy", "file-transfer", input.SourceNodeID, "failure", change)
		writeEvent(contract.FileTransferEvent{State: "error", Detail: "来源文件在拖拽后已发生变化。"})
		return
	}
	name, err := s.uniqueFileTransferName(transferContext, input.TargetDirectory, metadata.Name, requestID(r))
	if err != nil {
		writeEvent(contract.FileTransferEvent{State: "error", Detail: "无法确定目标文件名。"})
		return
	}
	change["kind"] = metadata.Kind
	change["bytes"] = metadata.SizeBytes
	change["targetName"] = name
	writeEvent(contract.FileTransferEvent{State: "transferring", TotalBytes: metadata.SizeBytes})

	loaded := int64(0)
	lastReported := time.Now()
	tracked := &fileTransferProgressReader{source: content, report: func(count int64) {
		loaded += count
		if time.Since(lastReported) >= 180*time.Millisecond {
			writeEvent(contract.FileTransferEvent{
				State: "transferring", LoadedBytes: loaded, TotalBytes: metadata.SizeBytes,
			})
			lastReported = time.Now()
		}
	}}
	query := url.Values{
		"path": []string{input.TargetDirectory}, "name": []string{name},
		"kind": []string{metadata.Kind}, "size": []string{strconv.FormatInt(metadata.SizeBytes, 10)},
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/octet-stream")
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		writeEvent(contract.FileTransferEvent{State: "error", Detail: "Agent 文件流不可用。"})
		return
	}
	response, err := streamer.OpenStream(
		transferContext, http.MethodPost, "/v1/files/transfer/import", query.Encode(),
		// Keep the local request chunked even for regular files. The Agent's
		// exact-length reader must consume the Noise end record before Upload
		// can atomically publish the destination.
		requestID(r), tracked, headers, -1,
	)
	if err != nil {
		_ = s.audit(r, session.User.ID, "file.transfer.copy", "file-transfer", input.SourceNodeID, "failure", change)
		writeEvent(contract.FileTransferEvent{State: "error", LoadedBytes: loaded, TotalBytes: metadata.SizeBytes, Detail: "目标 Agent 写入中断。"})
		return
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		_ = s.audit(r, session.User.ID, "file.transfer.copy", "file-transfer", input.SourceNodeID, "failure", change)
		writeEvent(contract.FileTransferEvent{State: "error", LoadedBytes: loaded, TotalBytes: metadata.SizeBytes, Detail: "目标文件写入失败，未保留半成品。"})
		return
	}
	writeEvent(contract.FileTransferEvent{State: "committing", LoadedBytes: loaded, TotalBytes: metadata.SizeBytes})
	var entry contract.FileEntry
	responseDecoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	responseDecoder.DisallowUnknownFields()
	if err := responseDecoder.Decode(&entry); err != nil {
		_ = s.audit(r, session.User.ID, "file.transfer.copy", "file-transfer", input.SourceNodeID, "failure", change)
		writeEvent(contract.FileTransferEvent{State: "error", LoadedBytes: loaded, TotalBytes: metadata.SizeBytes, Detail: "目标 Agent 返回无效结果。"})
		return
	}
	_ = s.audit(r, session.User.ID, "file.transfer.copy", "file-transfer", input.SourceNodeID, "success", change)
	writeEvent(contract.FileTransferEvent{
		State: "complete", LoadedBytes: loaded, TotalBytes: metadata.SizeBytes, Entry: &entry,
	})
}

type fileTransferProgressReader struct {
	source io.Reader
	report func(int64)
}

func (r *fileTransferProgressReader) Read(content []byte) (int, error) {
	count, err := r.source.Read(content)
	if count > 0 && r.report != nil {
		r.report(int64(count))
	}
	return count, err
}

func (s *Server) ensureFileTransferDirectory(ctx context.Context, directory, requestID string) error {
	query := url.Values{"path": []string{directory}}
	response, err := s.agent.Get(ctx, "/v1/files/entry", query.Encode(), requestID)
	if err == nil && response.StatusCode == http.StatusOK {
		var entry contract.FileEntry
		if json.Unmarshal(response.Body, &entry) == nil && entry.Kind == "directory" {
			return nil
		}
		return errors.New("target is not a directory")
	}
	if directory != desktopFileTransferDirectory || err != nil || response.StatusCode != http.StatusNotFound {
		return errors.New("target directory unavailable")
	}
	body, _ := json.Marshal(contract.FileActionRequest{Action: "mkdir", Target: "/home", Name: "KPanel Desktop"})
	created, err := s.agent.Do(ctx, http.MethodPost, "/v1/files/actions", "", requestID, body)
	if err != nil || (created.StatusCode < 200 || created.StatusCode >= 300) {
		response, checkErr := s.agent.Get(ctx, "/v1/files/entry", query.Encode(), requestID)
		if checkErr != nil || response.StatusCode != http.StatusOK {
			return errors.New("create target directory failed")
		}
	}
	return nil
}

func (s *Server) uniqueFileTransferName(ctx context.Context, directory, original, requestID string) (string, error) {
	for attempt := 0; attempt <= 999; attempt++ {
		candidate := suffixedFileTransferName(original, attempt)
		query := url.Values{"path": []string{path.Join(directory, candidate)}}
		response, err := s.agent.Get(ctx, "/v1/files/entry", query.Encode(), requestID)
		if err != nil {
			return "", err
		}
		if response.StatusCode == http.StatusNotFound {
			return candidate, nil
		}
		if response.StatusCode != http.StatusOK {
			return "", errors.New("target lookup failed")
		}
	}
	return "", errors.New("too many duplicate names")
}

func suffixedFileTransferName(name string, attempt int) string {
	if attempt == 0 {
		return name
	}
	extension := ""
	stem := name
	if dot := strings.LastIndex(name, "."); dot > 0 && dot < len(name)-1 {
		extension = name[dot:]
		stem = name[:dot]
	}
	suffix := " (" + strconv.Itoa(attempt) + ")"
	limit := 255 - len(extension) - len(suffix)
	for len(stem) > limit {
		_, size := utf8.DecodeLastRuneInString(stem)
		stem = stem[:len(stem)-size]
	}
	return stem + suffix + extension
}

func (s *Server) handleFileAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "文件操作参数无效", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input contract.FileActionRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if !allowedFileAction(input.Action) {
		s.writeValidationProblem(w, r, "action", "unsupported file action")
		return
	}
	body, err := json.Marshal(input)
	if err != nil {
		s.writeProblem(w, r, http.StatusInternalServerError, "request_encoding_failed", "Request encoding failed", "")
		return
	}
	target := input.Target
	if target == "" && len(input.Sources) > 0 {
		target = input.Sources[0]
	} else if target == "" && len(input.TrashIDs) > 0 {
		target = input.TrashIDs[0]
	}
	if input.Action == "trash_empty" {
		target = "file-trash"
	}
	itemCount := len(input.Sources)
	if len(input.TrashIDs) > 0 {
		itemCount = len(input.TrashIDs)
	}
	change := map[string]any{"action": input.Action, "items": itemCount}
	if input.Action == "compress" || input.Action == "extract" {
		change["format"] = input.Format
		change["name"] = input.Name
	}
	if err := s.audit(r, session.User.ID, "file."+input.Action, "file", target, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	response, err := s.doFileAction(r, input.Action, body)
	if err != nil {
		_ = s.audit(r, session.User.ID, "file."+input.Action, "file", target, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	result := "failure"
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		result = "success"
		var actionResult contract.FileActionResult
		if json.Unmarshal(response.Body, &actionResult) == nil && len(actionResult.Failed) > 0 {
			result = "partial_failure"
		}
	}
	_ = s.audit(r, session.User.ID, "file."+input.Action, "file", target, result, change)
	s.writeAgentResponse(w, r, response)
}

func (s *Server) doFileAction(r *http.Request, action string, body []byte) (AgentResponse, error) {
	if action != "compress" && action != "extract" {
		return s.agent.Do(
			r.Context(), http.MethodPost, "/v1/files/actions", "", requestID(r), body,
		)
	}
	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		return s.agent.Do(
			r.Context(), http.MethodPost, "/v1/files/actions", "", requestID(r), body,
		)
	}
	headers := make(http.Header)
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	response, err := streamer.OpenStream(
		r.Context(), http.MethodPost, "/v1/files/actions", "", requestID(r),
		bytes.NewReader(body), headers, int64(len(body)),
	)
	if err != nil {
		return AgentResponse{}, err
	}
	defer response.Body.Close()
	const maxArchiveActionResponse = 1 << 20
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxArchiveActionResponse+1))
	if err != nil {
		return AgentResponse{}, err
	}
	if len(responseBody) > maxArchiveActionResponse {
		return AgentResponse{}, errors.New("agent archive response exceeds configured limit")
	}
	return AgentResponse{
		StatusCode:  response.StatusCode,
		ContentType: response.Header.Get("Content-Type"),
		Body:        responseBody,
	}, nil
}

func strictPanelQuery(values map[string][]string, allowed ...string) bool {
	keys := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		keys[value] = struct{}{}
	}
	for key, values := range values {
		if _, ok := keys[key]; !ok || len(values) != 1 {
			return false
		}
	}
	return true
}

func allowedFileAction(action string) bool {
	switch action {
	case "mkdir", "rename", "copy", "move", "trash", "chmod",
		"compress", "extract", "trash_restore", "trash_delete", "trash_empty":
		return true
	default:
		return false
	}
}

func copyFileHeaders(target, source http.Header) {
	for _, key := range []string{
		"Accept-Ranges", "Content-Disposition", "Content-Length", "Content-Range",
		"Content-Security-Policy", "Content-Type", "ETag", "Last-Modified",
	} {
		if value := source.Get(key); value != "" {
			target.Set(key, value)
		}
	}
	target.Set("X-Content-Type-Options", "nosniff")
	if target.Get("Content-Security-Policy") == "" {
		target.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	}
}
