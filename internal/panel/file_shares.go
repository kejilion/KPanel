package panel

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/httpstream"
	"github.com/kejilion/kejilion-panel/internal/store"
)

const (
	fileSharesAdminPath       = "/api/v1/files/shares"
	fileShareAPIPrefix        = "/api/v1/public/file-shares/"
	fileSharePagePrefix       = "/share/file/"
	fileShareRawPrefix        = "/f/"
	fileShareRateWindow       = time.Minute
	fileShareRateLimit        = 300
	fileShareRateKeys         = 2048
	fileShareValidationWindow = time.Minute
	fileShareValidationUnit   = int64(2 << 20)
	fileShareValidationShare  = int64(512)
	fileShareValidationGlobal = int64(512)
	fileShareValidationKeys   = store.MaxFileShares
	maxFileShareMetadataReads = 16
	maxPublicFileShareStreams = 2
	fileShareIDBytes          = 16
	fileShareTokenBytes       = 32
)

var (
	fileShareMetadataTimeout    = 8 * time.Second
	fileShareFingerprintTimeout = 30 * time.Second
)

const fileShareContentSecurityPolicy = "default-src 'none'; sandbox; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

type fileShareCreateInput struct {
	Path                    string `json:"path"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
	ExpectedShareID         string `json:"expectedShareID"`
	ExpiresIn               string `json:"expiresIn"`
}

type fileShareAdminView struct {
	ID             string     `json:"id"`
	Path           string     `json:"path,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	LinksAvailable bool       `json:"linksAvailable"`
	SharePath      string     `json:"sharePath,omitempty"`
	DirectPath     string     `json:"directPath,omitempty"`
}

type fileShareLookupResponse struct {
	Share *fileShareAdminView `json:"share"`
}

type fileShareListResponse struct {
	Shares []fileShareAdminView `json:"shares"`
}

type publicFileShareView struct {
	Name         string     `json:"name"`
	MIME         string     `json:"mime,omitempty"`
	SizeBytes    int64      `json:"sizeBytes"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	DirectPath   string     `json:"directPath"`
	DownloadPath string     `json:"downloadPath"`
}

type fileShareRateEntry struct {
	startedAt time.Time
	count     int
}

type fileShareValidationEntry struct {
	startedAt time.Time
	units     int64
}

func (s *Server) handleFileShares(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == fileSharesAdminPath {
		switch r.Method {
		case http.MethodGet:
			s.handleFileShareLookup(w, r)
		case http.MethodPost:
			s.handleFileShareCreate(w, r)
		default:
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
			s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		}
		return
	}
	if r.Method == http.MethodDelete && isFileShareDeletePath(r.URL.Path) {
		s.handleFileShareDelete(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleFileShareLookup(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_share_query_invalid", "文件分享查询参数无效", "")
		return
	}
	if _, _, ok := s.requireSession(w, r); !ok {
		return
	}
	if r.URL.RawQuery == "" {
		values := s.store.ListFileShares(time.Now().UTC())
		shares := make([]fileShareAdminView, 0, len(values))
		for _, value := range values {
			view := fileShareAdminResponse(value, "")
			view.Path = value.Path
			shares = append(shares, view)
		}
		s.writeJSON(w, http.StatusOK, fileShareListResponse{Shares: shares})
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || !strictPanelQuery(query, "path", "resourceVersion") {
		s.writeProblem(w, r, http.StatusBadRequest, "file_share_query_invalid", "文件分享查询参数无效", "")
		return
	}
	filePath := query.Get("path")
	resourceVersion := query.Get("resourceVersion")
	if !validFileShareTarget(filePath, resourceVersion) {
		s.writeProblem(w, r, http.StatusBadRequest, "file_share_query_invalid", "文件分享查询参数无效", "")
		return
	}
	value, err := s.store.FileShareByPath(filePath, resourceVersion, time.Now().UTC())
	if errors.Is(err, store.ErrNotFound) {
		s.writeJSON(w, http.StatusOK, fileShareLookupResponse{})
		return
	}
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "file_share_storage_unavailable", "文件分享存储不可用", "")
		return
	}
	fingerprintContext, cancelFingerprint := context.WithTimeout(r.Context(), fileShareFingerprintTimeout)
	defer cancelFingerprint()
	entry, status, entryErr := s.loadFileShareEntry(fingerprintContext, value.Path)
	if entryErr != nil {
		if errors.Is(entryErr, context.DeadlineExceeded) {
			s.writeProblem(w, r, http.StatusGatewayTimeout, "file_share_fingerprint_timeout", "文件指纹计算超时，请稍后重试", "")
			return
		}
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	if fileShareAgentUnavailableStatus(status) {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	if status != http.StatusOK || !fileShareMatchesEntry(value, entry) {
		s.writeJSON(w, http.StatusOK, fileShareLookupResponse{})
		return
	}
	view := fileShareAdminResponse(value, "")
	s.writeJSON(w, http.StatusOK, fileShareLookupResponse{Share: &view})
}

func (s *Server) handleFileShareCreate(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_share_request_invalid", "文件分享请求无效", "")
		return
	}
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input fileShareCreateInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	expiresAt, ok := fileShareExpiry(input.ExpiresIn, time.Now().UTC())
	if !validFileShareTarget(input.Path, input.ExpectedResourceVersion) ||
		(input.ExpectedShareID != "" && !validFileShareID(input.ExpectedShareID)) || !ok {
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "file_share_request_invalid", "文件分享参数无效", "")
		return
	}
	fingerprintContext, cancelFingerprint := context.WithTimeout(r.Context(), fileShareFingerprintTimeout)
	defer cancelFingerprint()
	entry, status, err := s.loadFileShareEntry(fingerprintContext, input.Path)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			s.writeProblem(w, r, http.StatusGatewayTimeout, "file_share_fingerprint_timeout", "文件指纹计算超时，请稍后重试", "")
			return
		}
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	if status != http.StatusOK {
		if fileShareAgentUnavailableStatus(status) {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
			return
		}
		if status == http.StatusRequestEntityTooLarge {
			s.writeProblem(w, r, status, "file_share_too_large", "文件超过 512 MiB，无法创建轻量分享", "")
			return
		}
		if status == http.StatusConflict {
			s.writeProblem(w, r, status, "file_share_changed", "文件已发生变化，请刷新后重试", "")
			return
		}
		s.writeProblem(w, r, status, "file_share_target_unavailable", "无法分享此文件", "")
		return
	}
	if entry.Path != input.Path || entry.Kind != "file" {
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "file_share_regular_file_required", "只能分享普通文件", "")
		return
	}
	if entry.SizeBytes < 0 || entry.SizeBytes > contract.MaxFileShareBytes {
		s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "file_share_too_large", "文件超过 512 MiB，无法创建轻量分享", "")
		return
	}
	if entry.ResourceVersion != input.ExpectedResourceVersion {
		s.writeProblem(w, r, http.StatusConflict, "file_share_changed", "文件已发生变化，请刷新后重试", "")
		return
	}
	id, err := newFileShareSecret(fileShareIDBytes)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "file_share_token_unavailable", "无法创建分享链接", "")
		return
	}
	token, err := newFileShareSecret(fileShareTokenBytes)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "file_share_token_unavailable", "无法创建分享链接", "")
		return
	}
	now := time.Now().UTC()
	change := map[string]any{
		"path": input.Path, "expiresIn": input.ExpiresIn,
	}
	if err := s.audit(r, session.User.ID, "file.share.create", "file", input.Path, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	digest := sha256.Sum256([]byte(token))
	value := store.FileShare{
		ID: id, TokenHash: hex.EncodeToString(digest[:]), Path: input.Path,
		ResourceVersion: input.ExpectedResourceVersion, ShareVersion: entry.ShareVersion,
		SizeBytes: entry.SizeBytes,
		CreatedAt: now, ExpiresAt: expiresAt,
	}
	created, replaced, err := s.store.CreateFileShare(value, input.ExpectedShareID, now)
	if err != nil {
		_ = s.audit(r, session.User.ID, "file.share.create", "file", input.Path, "failure", change)
		if errors.Is(err, store.ErrConflict) {
			s.writeProblem(w, r, http.StatusConflict, "file_share_changed", "文件分享状态已变化，请刷新后重试", "")
			return
		}
		if errors.Is(err, store.ErrLimitReached) {
			s.writeProblem(w, r, http.StatusConflict, "file_share_limit_reached", "文件分享数量已达上限", "")
			return
		}
		s.writeProblem(w, r, http.StatusServiceUnavailable, "file_share_storage_unavailable", "文件分享存储不可用", "")
		return
	}
	if replaced.ID != "" {
		s.cancelFileShareStreams(replaced.ID)
	}
	successChange := map[string]any{
		"path": input.Path, "expiresIn": input.ExpiresIn, "replaced": replaced.ID != "",
	}
	_ = s.audit(r, session.User.ID, "file.share.create", "file", input.Path, "success", successChange)
	s.writeJSON(w, http.StatusCreated, fileShareAdminResponse(created, token))
}

func (s *Server) handleFileShareDelete(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, fileSharesAdminPath+"/")
	change := map[string]any{"shareID": id}
	if err := s.audit(r, session.User.ID, "file.share.delete", "file-share", id, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	deleted, err := s.store.DeleteFileShare(id)
	if err != nil {
		_ = s.audit(r, session.User.ID, "file.share.delete", "file-share", id, "failure", change)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		s.writeProblem(w, r, http.StatusServiceUnavailable, "file_share_storage_unavailable", "文件分享存储不可用", "")
		return
	}
	s.cancelFileShareStreams(deleted.ID)
	_ = s.audit(r, session.User.ID, "file.share.delete", "file-share", id, "success", change)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePublicFileShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.RawPath != "" || r.URL.RawQuery != "" || !isPublicFileShareAPIPath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if !s.allowFileShareRequest(s.remoteIP(r), time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		s.writeProblem(w, r, http.StatusTooManyRequests, "file_share_rate_limited", "文件分享访问过于频繁", "")
		return
	}
	token := strings.TrimPrefix(r.URL.Path, fileShareAPIPrefix)
	value, entry, status := s.resolvePublicFileShare(r.Context(), token)
	if status == http.StatusNotFound {
		http.NotFound(w, r)
		return
	}
	if status == http.StatusTooManyRequests {
		w.Header().Set("Retry-After", "1")
	}
	if status != http.StatusOK {
		s.writeProblem(w, r, status, "file_share_unavailable", "文件分享暂时不可用", "")
		return
	}
	mimeType := entry.MIME
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	s.writeJSON(w, http.StatusOK, publicFileShareView{
		Name: entry.Name, MIME: mimeType, SizeBytes: entry.SizeBytes,
		ExpiresAt:  cloneTimePointer(value.ExpiresAt),
		DirectPath: fileShareRawPrefix + token, DownloadPath: fileShareRawPrefix + token + "?download=1",
	})
}

func (s *Server) handlePublicFileShareContent(w http.ResponseWriter, r *http.Request) {
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || r.URL.RawPath != "" ||
		!isPublicFileShareContentPath(r.URL.Path) || !validFileShareContentQuery(r.URL.RawQuery) {
		http.NotFound(w, r)
		return
	}
	if !s.allowFileShareRequest(s.remoteIP(r), time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		s.writeProblem(w, r, http.StatusTooManyRequests, "file_share_rate_limited", "文件分享访问过于频繁", "")
		return
	}
	token := strings.TrimPrefix(r.URL.Path, fileShareRawPrefix)
	value, err := s.store.FileShareByToken(token, time.Now().UTC())
	if err != nil || !validFileShareTarget(value.Path, value.ResourceVersion) {
		http.NotFound(w, r)
		return
	}
	if !s.acquireFileShareStream() {
		w.Header().Set("Retry-After", "1")
		s.writeProblem(w, r, http.StatusTooManyRequests, "file_share_stream_limit", "文件分享下载繁忙", "")
		return
	}
	defer s.releaseFileShareStream()

	deadline := time.Now().Add(panelFileTransferMaxDuration)
	if value.ExpiresAt != nil && value.ExpiresAt.Before(deadline) {
		deadline = *value.ExpiresAt
	}
	transferContext, cancel := context.WithDeadline(r.Context(), deadline)
	finish := s.registerFileShareStream(value.ID, cancel)
	defer finish()
	// Close the lookup-to-registration race with a second state check. A delete
	// either cancels this registered stream or makes this recheck fail.
	current, err := s.store.FileShareByToken(token, time.Now().UTC())
	if err != nil || current.ID != value.ID {
		http.NotFound(w, r)
		return
	}
	if transferContext.Err() != nil {
		http.NotFound(w, r)
		return
	}

	streamer, ok := s.agent.(agentStreamAPI)
	if !ok {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "file_share_stream_unavailable", "文件分享下载暂时不可用", "")
		return
	}
	if !s.allowFileShareValidation(value.Path, value.SizeBytes, time.Now().UTC()) {
		w.Header().Set("Retry-After", "60")
		s.writeProblem(w, r, http.StatusTooManyRequests, "file_share_validation_limited", "文件分享校验过于频繁", "")
		return
	}
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	query := url.Values{"path": []string{value.Path}, "disposition": []string{disposition}}
	headers := make(http.Header)
	headers.Set("If-Match", `"`+value.ResourceVersion+`"`)
	headers.Set(contract.FileShareVersionHeader, value.ShareVersion)
	for _, key := range []string{"Range", "If-None-Match", "If-Modified-Since", "If-Range"} {
		if headerValue := r.Header.Get(key); headerValue != "" {
			headers.Set(key, headerValue)
		}
	}
	response, err := streamer.OpenStream(
		transferContext, r.Method, "/v1/files/share-content", query.Encode(), requestID(r),
		http.NoBody, headers, 0,
	)
	if err != nil {
		if transferContext.Err() != nil {
			if r.Context().Err() != nil {
				return
			}
			if _, lookupErr := s.store.FileShareByToken(token, time.Now().UTC()); lookupErr != nil {
				http.NotFound(w, r)
			} else if !time.Now().Before(deadline) {
				s.writeProblem(w, r, http.StatusGatewayTimeout, "file_share_stream_timeout", "文件分享下载超时", "")
			} else {
				s.writeProblem(w, r, http.StatusServiceUnavailable, "file_share_stream_unavailable", "文件分享下载暂时不可用", "")
			}
			return
		}
		s.writeProblem(w, r, http.StatusServiceUnavailable, "agent_unavailable", "Agent unavailable", "")
		return
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusNotModified, http.StatusRequestedRangeNotSatisfiable:
		// Forward only the bounded file response contract below.
	case http.StatusTooManyRequests:
		w.Header().Set("Retry-After", "1")
		s.writeProblem(w, r, http.StatusTooManyRequests, "file_share_stream_limit", "文件分享下载繁忙", "")
		return
	case http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		http.NotFound(w, r)
		return
	default:
		s.writeProblem(w, r, http.StatusServiceUnavailable, "file_share_unavailable", "文件分享暂时不可用", "")
		return
	}
	copyFileHeaders(w.Header(), response.Header)
	w.Header().Set("Content-Security-Policy", fileShareContentSecurityPolicy)
	if disposition == "attachment" || !safeFileShareInlineContentType(w.Header().Get("Content-Type")) {
		forceFileShareAttachment(w.Header())
	}
	if response.Header.Get("Content-Length") == "" && response.ContentLength >= 0 &&
		(response.StatusCode == http.StatusOK || response.StatusCode == http.StatusPartialContent) {
		w.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	writer := httpstream.NewIdleResponseWriter(transferContext, w, panelFileTransferIdleTimeout)
	writer.WriteHeader(response.StatusCode)
	if r.Method == http.MethodHead || response.StatusCode == http.StatusNotModified {
		return
	}
	_, _ = io.CopyBuffer(writer, response.Body, make([]byte, 64<<10))
}

func safeFileShareInlineContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return false
	}
	switch strings.ToLower(mediaType) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/avif":
		return true
	default:
		return false
	}
}

func forceFileShareAttachment(header http.Header) {
	_, parameters, err := mime.ParseMediaType(header.Get("Content-Disposition"))
	filename := ""
	if err == nil {
		filename = parameters["filename"]
	}
	header.Set("Content-Type", "application/octet-stream")
	if filename == "" {
		header.Set("Content-Disposition", "attachment")
		return
	}
	formatted := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	if formatted == "" {
		header.Set("Content-Disposition", "attachment")
		return
	}
	header.Set("Content-Disposition", formatted)
}

func (s *Server) loadFileShareEntry(ctx context.Context, filePath string) (contract.FileShareEntry, int, error) {
	query := url.Values{"path": []string{filePath}}
	response, err := s.agent.Get(ctx, "/v1/files/share-entry", query.Encode(), newRequestID())
	if err != nil {
		return contract.FileShareEntry{}, 0, err
	}
	if response.StatusCode != http.StatusOK {
		return contract.FileShareEntry{}, response.StatusCode, nil
	}
	var entry contract.FileShareEntry
	if err := json.Unmarshal(response.Body, &entry); err != nil {
		return contract.FileShareEntry{}, http.StatusServiceUnavailable, err
	}
	return entry, http.StatusOK, nil
}

func (s *Server) loadFileEntry(ctx context.Context, filePath string) (contract.FileEntry, int, error) {
	query := url.Values{"path": []string{filePath}}
	response, err := s.agent.Get(ctx, "/v1/files/entry", query.Encode(), newRequestID())
	if err != nil {
		return contract.FileEntry{}, 0, err
	}
	if response.StatusCode != http.StatusOK {
		return contract.FileEntry{}, response.StatusCode, nil
	}
	var entry contract.FileEntry
	if err := json.Unmarshal(response.Body, &entry); err != nil {
		return contract.FileEntry{}, http.StatusServiceUnavailable, err
	}
	return entry, http.StatusOK, nil
}

func (s *Server) resolvePublicFileShare(ctx context.Context, token string) (store.FileShare, contract.FileEntry, int) {
	value, err := s.store.FileShareByToken(token, time.Now().UTC())
	if err != nil {
		return store.FileShare{}, contract.FileEntry{}, http.StatusNotFound
	}
	if !s.acquireFileShareMetadata() {
		return store.FileShare{}, contract.FileEntry{}, http.StatusTooManyRequests
	}
	defer s.releaseFileShareMetadata()
	metadataContext, cancel := context.WithTimeout(ctx, fileShareMetadataTimeout)
	defer cancel()
	entry, status, err := s.loadFileEntry(metadataContext, value.Path)
	if err != nil {
		return store.FileShare{}, contract.FileEntry{}, http.StatusServiceUnavailable
	}
	if fileShareAgentUnavailableStatus(status) {
		return store.FileShare{}, contract.FileEntry{}, http.StatusServiceUnavailable
	}
	if status != http.StatusOK || !fileShareMetadataMatchesEntry(value, entry) {
		return store.FileShare{}, contract.FileEntry{}, http.StatusNotFound
	}
	current, lookupErr := s.store.FileShareByToken(token, time.Now().UTC())
	if lookupErr != nil || current.ID != value.ID {
		return store.FileShare{}, contract.FileEntry{}, http.StatusNotFound
	}
	return value, entry, http.StatusOK
}

func fileShareMatchesEntry(value store.FileShare, entry contract.FileShareEntry) bool {
	return entry.Kind == "file" && entry.Path == value.Path && entry.ResourceVersion == value.ResourceVersion &&
		entry.ShareVersion == value.ShareVersion && entry.SizeBytes == value.SizeBytes
}

func fileShareMetadataMatchesEntry(value store.FileShare, entry contract.FileEntry) bool {
	return entry.Kind == "file" && entry.Path == value.Path && entry.ResourceVersion == value.ResourceVersion &&
		entry.SizeBytes == value.SizeBytes
}

func fileShareAgentUnavailableStatus(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func validFileShareTarget(filePath, resourceVersion string) bool {
	return validFileDownloadPath(filePath) && resourceVersionPattern.MatchString(resourceVersion)
}

func fileShareExpiry(value string, now time.Time) (*time.Time, bool) {
	var duration time.Duration
	switch value {
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	case "never":
		return nil, true
	default:
		return nil, false
	}
	expiresAt := now.Add(duration)
	return &expiresAt, true
}

func fileShareAdminResponse(value store.FileShare, token string) fileShareAdminView {
	response := fileShareAdminView{
		ID: value.ID, CreatedAt: value.CreatedAt, ExpiresAt: cloneTimePointer(value.ExpiresAt),
		LinksAvailable: token != "",
	}
	if token != "" {
		response.SharePath = fileSharePagePrefix + token
		response.DirectPath = fileShareRawPrefix + token
	}
	return response
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func newFileShareSecret(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validFileShareToken(token string) bool {
	if len(token) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == fileShareTokenBytes
}

func validFileShareID(id string) bool {
	if len(id) != 22 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	return err == nil && len(decoded) == fileShareIDBytes
}

func isFileShareDeletePath(requestPath string) bool {
	id := strings.TrimPrefix(requestPath, fileSharesAdminPath+"/")
	return id != requestPath && validFileShareID(id)
}

func isPublicFileShareAPIPath(requestPath string) bool {
	token := strings.TrimPrefix(requestPath, fileShareAPIPrefix)
	return token != requestPath && validFileShareToken(token)
}

func isFileSharePagePath(requestPath string) bool {
	token := strings.TrimPrefix(requestPath, fileSharePagePrefix)
	return token != requestPath && validFileShareToken(token)
}

func isPublicFileShareContentPath(requestPath string) bool {
	token := strings.TrimPrefix(requestPath, fileShareRawPrefix)
	return token != requestPath && validFileShareToken(token)
}

func validFileShareContentQuery(rawQuery string) bool {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return false
	}
	if !strictPanelQuery(values, "download") {
		return false
	}
	value, exists := values["download"]
	return !exists || (len(value) == 1 && value[0] == "1")
}

func (s *Server) allowFileShareRequest(key string, now time.Time) bool {
	s.fileShareRateMu.Lock()
	defer s.fileShareRateMu.Unlock()
	if s.fileShareRates == nil {
		s.fileShareRates = make(map[string]fileShareRateEntry)
	}
	entry, exists := s.fileShareRates[key]
	if exists && now.Sub(entry.startedAt) >= fileShareRateWindow {
		entry = fileShareRateEntry{}
		exists = false
	}
	if !exists && len(s.fileShareRates) >= fileShareRateKeys {
		for existingKey, existing := range s.fileShareRates {
			if now.Sub(existing.startedAt) >= fileShareRateWindow {
				delete(s.fileShareRates, existingKey)
			}
		}
		if len(s.fileShareRates) >= fileShareRateKeys {
			return false
		}
	}
	if !exists {
		entry = fileShareRateEntry{startedAt: now}
	}
	if entry.count >= fileShareRateLimit {
		return false
	}
	entry.count++
	s.fileShareRates[key] = entry
	return true
}

func (s *Server) allowFileShareValidation(key string, sizeBytes int64, now time.Time) bool {
	if key == "" || sizeBytes < 0 || sizeBytes > contract.MaxFileShareBytes {
		return false
	}
	units := (sizeBytes + fileShareValidationUnit - 1) / fileShareValidationUnit
	if units < 1 {
		units = 1
	}

	s.fileShareValidationMu.Lock()
	defer s.fileShareValidationMu.Unlock()
	if s.fileShareValidations == nil {
		s.fileShareValidations = make(map[string]fileShareValidationEntry)
	}
	global := s.fileShareGlobalCost
	if global.startedAt.IsZero() || now.Sub(global.startedAt) >= fileShareValidationWindow {
		global = fileShareValidationEntry{startedAt: now}
	}
	entry, exists := s.fileShareValidations[key]
	if exists && now.Sub(entry.startedAt) >= fileShareValidationWindow {
		delete(s.fileShareValidations, key)
		entry = fileShareValidationEntry{}
		exists = false
	}
	if !exists && len(s.fileShareValidations) >= fileShareValidationKeys {
		for existingKey, existing := range s.fileShareValidations {
			if now.Sub(existing.startedAt) >= fileShareValidationWindow {
				delete(s.fileShareValidations, existingKey)
			}
		}
		if len(s.fileShareValidations) >= fileShareValidationKeys {
			return false
		}
	}
	if !exists {
		entry = fileShareValidationEntry{startedAt: now}
	}
	if entry.units+units > fileShareValidationShare || global.units+units > fileShareValidationGlobal {
		return false
	}
	entry.units += units
	global.units += units
	s.fileShareValidations[key] = entry
	s.fileShareGlobalCost = global
	return true
}

func (s *Server) acquireFileShareStream() bool {
	select {
	case s.fileShareStreamGate <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) acquireFileShareMetadata() bool {
	select {
	case s.fileShareMetadataGate <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) releaseFileShareMetadata() {
	<-s.fileShareMetadataGate
}

func (s *Server) releaseFileShareStream() {
	<-s.fileShareStreamGate
}

func (s *Server) registerFileShareStream(shareID string, cancel context.CancelFunc) func() {
	s.fileShareStreamMu.Lock()
	if s.fileShareStreams == nil {
		s.fileShareStreams = make(map[string]map[uint64]context.CancelFunc)
	}
	s.fileShareStreamNext++
	streamID := s.fileShareStreamNext
	if s.fileShareStreams[shareID] == nil {
		s.fileShareStreams[shareID] = make(map[uint64]context.CancelFunc)
	}
	s.fileShareStreams[shareID][streamID] = cancel
	s.fileShareStreamMu.Unlock()
	return func() {
		cancel()
		s.fileShareStreamMu.Lock()
		delete(s.fileShareStreams[shareID], streamID)
		if len(s.fileShareStreams[shareID]) == 0 {
			delete(s.fileShareStreams, shareID)
		}
		s.fileShareStreamMu.Unlock()
	}
}

func (s *Server) cancelFileShareStreams(shareID string) {
	s.fileShareStreamMu.Lock()
	cancels := s.fileShareStreams[shareID]
	delete(s.fileShareStreams, shareID)
	s.fileShareStreamMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (s *Server) closeFileShareStreams() {
	s.fileShareStreamMu.Lock()
	all := s.fileShareStreams
	s.fileShareStreams = nil
	s.fileShareStreamMu.Unlock()
	for _, streams := range all {
		for _, cancel := range streams {
			cancel()
		}
	}
}
