package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/httpstream"
	"github.com/kejilion/kejilion-panel/internal/redact"
)

const (
	fileTransferMetadataHeader = "X-KPanel-File-Metadata"
	fileTransferResultTrailer  = "X-KPanel-Transfer-Result"
)

const (
	fileTransferIdleTimeout  = 45 * time.Second
	fileTransferMaxDuration  = 2 * time.Hour
	fileThumbnailTimeout     = 20 * time.Second
	fileThumbnailMaxBytes    = 12 << 20
	fileGridSourceMaxBytes   = 64 << 20
	fileThumbnailMaxPixels   = 64_000_000
	fileThumbnailMaxWidth    = 320
	fileThumbnailMaxHeight   = 210
	fileToolTextMaxBytes     = 64 << 10
	fileArchiveQueryMaxBytes = 256 << 10
)

type fileTextResult struct {
	Path            string `json:"path"`
	Content         string `json:"content"`
	SizeBytes       int64  `json:"sizeBytes"`
	ResourceVersion string `json:"resourceVersion"`
}

type fileTailResult struct {
	Path            string `json:"path"`
	Content         string `json:"content"`
	SizeBytes       int64  `json:"sizeBytes"`
	ResourceVersion string `json:"resourceVersion"`
	Truncated       bool   `json:"truncated"`
}

var errThumbnailUnavailable = errors.New("该图片不能生成缩略图")

func (s *Server) fileList(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_path", "文件路径无效", "")
		return
	}
	values := r.URL.Query()
	if !strictQuery(values, "path", "limit", "offset", "search") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "文件查询参数无效", "")
		return
	}
	limit := filemanager.MaxDirectoryEntries
	if raw := values.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > filemanager.MaxDirectoryEntries {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_limit", "目录项目上限无效", "")
			return
		}
		limit = parsed
	}
	offset := 0
	if raw := values.Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 || parsed >= filemanager.MaxDirectoryScan {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_offset", "目录偏移量无效", "")
			return
		}
		offset = parsed
	}
	search := values.Get("search")
	if len(search) > filemanager.MaxSearchBytes {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_search", "目录搜索内容过长", "")
		return
	}
	result, err := s.files.ListPage(r.Context(), values.Get("path"), filemanager.ListOptions{
		Limit: limit, Offset: offset, Search: search,
	})
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	result.FileReceiveVersion = 1
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) fileEntry(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path") || r.URL.Query().Get("path") == "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "文件查询参数无效", "")
		return
	}
	entry, err := s.files.Stat(r.URL.Query().Get("path"))
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) fileShareEntry(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || r.URL.RawPath != "" || !strictQuery(query, "path") || query.Get("path") == "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "文件查询参数无效", "")
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(contract.FileShareFingerprintTimeout + fileTransferIdleTimeout))
	entry, err := s.files.ShareEntry(r.Context(), query.Get("path"))
	if err != nil {
		writeFileShareProblem(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (s *Server) fileEntries(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "文件查询参数无效", "")
		return
	}
	var input contract.FileEntryBatchRequest
	if err := decodeJSON(w, r, &input); err != nil {
		return
	}
	if len(input.Paths) == 0 || len(input.Paths) > contract.MaxFileEntryBatch {
		writeProblem(w, requestID, http.StatusBadRequest, "file_request_invalid", "文件请求无效", "")
		return
	}
	seen := make(map[string]struct{}, len(input.Paths))
	result := contract.FileEntryBatchResult{
		Entries:     make([]contract.FileEntry, 0, len(input.Paths)),
		Unavailable: make([]string, 0),
	}
	for _, filePath := range input.Paths {
		if filePath == "" {
			writeProblem(w, requestID, http.StatusBadRequest, "file_request_invalid", "文件请求无效", "")
			return
		}
		if _, exists := seen[filePath]; exists {
			writeProblem(w, requestID, http.StatusBadRequest, "file_request_invalid", "文件请求无效", "")
			return
		}
		seen[filePath] = struct{}{}
		entry, err := s.files.Stat(filePath)
		if err != nil {
			result.Unavailable = append(result.Unavailable, filePath)
			continue
		}
		result.Entries = append(result.Entries, entry)
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) fileTrashList(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "回收站查询参数无效", "")
		return
	}
	result, err := s.files.ListTrash(r.Context())
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) fileContent(w http.ResponseWriter, r *http.Request, requestID string) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.fileRead(w, r, requestID, false)
	case http.MethodPut:
		s.fileWrite(w, r, requestID)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead+", "+http.MethodPut)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
	}
}

func (s *Server) fileShareContent(w http.ResponseWriter, r *http.Request, requestID string) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		s.fileRead(w, r, requestID, true)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
	}
}

func (s *Server) fileArchive(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	var input contract.FileArchiveDownloadRequest
	var members contract.FileArchiveMemberDownload
	memberDownload := r.URL.Query().Has("members")
	var name string
	switch r.Method {
	case http.MethodGet:
		if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "selection", "name", "members") {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "压缩下载参数无效", "")
			return
		}
		selection := r.URL.Query().Get("selection")
		name = r.URL.Query().Get("name")
		if len(selection) == 0 || len(selection) > fileArchiveQueryMaxBytes ||
			json.Unmarshal([]byte(selection), &input) != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_archive_download", "压缩下载参数无效", "")
			return
		}
	case http.MethodPost:
		if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "name") {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "压缩下载参数无效", "")
			return
		}
		name = r.URL.Query().Get("name")
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeProblem(w, requestID, http.StatusUnsupportedMediaType, "json_required", "必须提交 JSON", "")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, fileArchiveQueryMaxBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_archive_download", "压缩下载参数无效", "")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_archive_download", "压缩下载参数无效", "")
			return
		}
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	if memberDownload {
		value := r.URL.Query().Get("members")
		if r.Method != http.MethodGet || len(value) == 0 || len(value) > fileArchiveQueryMaxBytes ||
			json.Unmarshal([]byte(value), &members) != nil || len(members.Entries) == 0 || len(members.Entries) > filemanager.MaxBatchItems ||
			len(input.Sources) != 1 || (members.Raw && len(members.Entries) != 1) {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_archive_download", "压缩下载参数无效", "")
			return
		}
	}
	validName := validArchiveDownloadName(name)
	if memberDownload && members.Raw {
		validName = name != "" && name != "." && name != ".." && len(name) <= 1024 && path.Base(name) == name && !strings.ContainsAny(name, "\\\x00\r\n")
	}
	if !validName {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_archive_download", "压缩下载参数无效", "")
		return
	}
	if !validArchiveDownloadSelection(input) {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_archive_download", "压缩下载参数无效", "")
		return
	}

	transferContext, cancel := context.WithTimeout(r.Context(), fileTransferMaxDuration)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	go func() {
		var err error
		if memberDownload {
			err = s.files.ExportArchiveMembers(transferContext, input.Sources[0], input.ExpectedResourceVersions[input.Sources[0]], members.Entries, members.Raw, writer)
		} else {
			err = s.files.ExportZIP(transferContext, input.Sources, input.ExpectedResourceVersions, writer)
		}
		_ = writer.CloseWithError(err)
	}()

	buffer := make([]byte, 64<<10)
	read, err := reader.Read(buffer)
	if err != nil && read == 0 && !(memberDownload && errors.Is(err, io.EOF)) {
		writeFileProblem(w, requestID, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	if memberDownload && members.Raw {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	if formatted := mime.FormatMediaType("attachment", map[string]string{"filename": name}); formatted != "" {
		w.Header().Set("Content-Disposition", formatted)
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	output := httpstream.NewIdleResponseWriter(transferContext, w, fileTransferIdleTimeout)
	output.WriteHeader(http.StatusOK)
	if read > 0 {
		if _, writeErr := output.Write(buffer[:read]); writeErr != nil {
			_ = reader.CloseWithError(writeErr)
			return
		}
	}
	if err == nil {
		_, err = io.CopyBuffer(output, reader, buffer)
	}
	if memberDownload && err != nil && !errors.Is(err, io.EOF) {
		panic(http.ErrAbortHandler)
	}
}

func validArchiveDownloadName(name string) bool {
	return name != "" && len(name) <= 1024 && path.Base(name) == name &&
		!strings.ContainsAny(name, "\\\x00\r\n") && strings.HasSuffix(strings.ToLower(name), ".zip")
}

func validArchiveDownloadSelection(input contract.FileArchiveDownloadRequest) bool {
	if len(input.Sources) == 0 || len(input.Sources) > filemanager.MaxBatchItems ||
		len(input.ExpectedResourceVersions) != len(input.Sources) {
		return false
	}
	seen := make(map[string]struct{}, len(input.Sources))
	for _, source := range input.Sources {
		version, ok := input.ExpectedResourceVersions[source]
		if !ok || version == "" || len(version) > 256 || source == "" || len(source) > 4096 ||
			!strings.HasPrefix(source, "/") || strings.ContainsAny(source, "\\\x00") || path.Clean(source) != source {
			return false
		}
		if _, exists := seen[source]; exists {
			return false
		}
		seen[source] = struct{}{}
	}
	return true
}

// fileText returns a bounded JSON representation for structured consumers.
// It deliberately reuses File Manager path, symlink and protected-directory checks.
func (s *Server) fileText(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "File query is invalid", "")
		return
	}
	file, entry, err := s.files.Open(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	defer file.Close()
	if !entry.Editable || entry.SizeBytes > fileToolTextMaxBytes {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "text_preview_unavailable", "File is not an editable UTF-8 text file up to 64 KiB", "")
		return
	}
	content, err := io.ReadAll(io.LimitReader(file, fileToolTextMaxBytes+1))
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	if len(content) > fileToolTextMaxBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "text_preview_unavailable", "File is not valid UTF-8 text up to 64 KiB", "")
		return
	}
	writeJSON(w, http.StatusOK, fileTextResult{
		Path: entry.Path, Content: string(content), SizeBytes: entry.SizeBytes,
		ResourceVersion: entry.ResourceVersion,
	})
}

func (s *Server) fileTail(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path", "maxBytes") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "File tail query is invalid", "")
		return
	}
	maxBytes := int64(32 << 10)
	if raw := r.URL.Query().Get("maxBytes"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1024 || parsed > fileToolTextMaxBytes {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_limit", "File tail limit must be between 1024 and 65536 bytes", "")
			return
		}
		maxBytes = parsed
	}
	file, entry, err := s.files.Open(r.Context(), r.URL.Query().Get("path"))
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	defer file.Close()
	start := entry.SizeBytes - maxBytes
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	if len(content) > int(maxBytes) {
		content = content[:maxBytes]
	}
	if start > 0 {
		if newline := bytes.IndexByte(content, '\n'); newline >= 0 {
			content = content[newline+1:]
		}
	}
	for removed := 0; start > 0 && len(content) > 0 && !utf8.Valid(content) && removed < utf8.UTFMax-1; removed++ {
		content = content[1:]
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "text_preview_unavailable", "File tail is not valid UTF-8 text", "")
		return
	}
	writeJSON(w, http.StatusOK, fileTailResult{
		Path: entry.Path, Content: string(content), SizeBytes: entry.SizeBytes,
		ResourceVersion: entry.ResourceVersion, Truncated: start > 0,
	})
}

// fileAITail projects a complete bounded snapshot before selecting its tail.
// Selecting first loses structural context (for example, Kubernetes Secret kind).
func (s *Server) fileAITail(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path", "maxBytes") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "File tail query is invalid", "")
		return
	}
	maxBytes := int64(32 << 10)
	if raw := r.URL.Query().Get("maxBytes"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1024 || parsed > fileToolTextMaxBytes {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_limit", "File tail limit must be between 1024 and 65536 bytes", "")
			return
		}
		maxBytes = parsed
	}
	select {
	case s.aiTailGate <- struct{}{}:
		defer func() { <-s.aiTailGate }()
	default:
		writeProblem(w, requestID, http.StatusServiceUnavailable, "projection_busy", "File projection is busy", "")
		return
	}
	const maxSnapshot = 8 << 20
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	file, entry, err := s.files.Open(ctx, r.URL.Query().Get("path"))
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	defer file.Close()
	statFile, ok := file.(interface {
		Stat() (os.FileInfo, error)
		Fd() uintptr
	})
	if !ok || entry.SizeBytes < 0 || uint64(entry.SizeBytes) >= uint64(^uint(0)>>1) {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "projection_unavailable", "A stable file snapshot is unavailable", "")
		return
	}
	lease, err := tryStableReadLease(statFile, cancel)
	if err != nil {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "projection_unavailable", "Stable reading is unavailable: the file may be in use, inaccessible, or on an unsupported filesystem; close writers or use a stable copy", "")
		return
	}
	defer lease.close()
	before, err := statFile.Stat()
	if err != nil || before.Size() != entry.SizeBytes {
		writeProblem(w, requestID, http.StatusConflict, "projection_changed", "File changed during projection", "")
		return
	}
	var projected []byte
	var truncated bool
	if entry.SizeBytes <= maxSnapshot {
		content, readErr := io.ReadAll(io.LimitReader(file, maxSnapshot+1))
		if readErr != nil || len(content) > maxSnapshot || before.Size() != int64(len(content)) {
			writeProblem(w, requestID, http.StatusConflict, "projection_changed", "File changed during projection", "")
			return
		}
		if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "text_preview_unavailable", "File is not valid UTF-8 text", "")
			return
		}
		if bytes.Count(content, []byte("\n")) > 50000 {
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "projection_unavailable", "File exceeds the AI-safe projection budget", "")
			return
		}
		for _, line := range bytes.Split(content, []byte("\n")) {
			if len(line) > 64<<10 {
				writeProblem(w, requestID, http.StatusUnprocessableEntity, "projection_unavailable", "File exceeds the AI-safe projection budget", "")
				return
			}
		}
		// Project after copying; the opened file remains leased until the final
		// identity check, so no writer can alter this snapshot while it is read.
		projected = []byte(redact.Text(string(content)))
		truncated = len(projected) > int(maxBytes)
		if truncated {
			projected = projected[len(projected)-int(maxBytes):]
			if newline := bytes.IndexByte(projected, '\n'); newline >= 0 {
				projected = projected[newline+1:]
			}
			for len(projected) > 0 && !utf8.Valid(projected) {
				projected = projected[1:]
			}
		}
	} else {
		// This conservative stream has no fixed source-size cap. It refuses
		// ambiguous contexts rather than returning a partially checked result.
		tail, wasTruncated, streamErr := redact.ConservativeLogTail(ctx, file, int(entry.SizeBytes), int(maxBytes))
		if streamErr != nil {
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "projection_unavailable", "The file could not be safely projected; try a smaller stable plain-text copy with shorter lines", "")
			return
		}
		projected, truncated = []byte(tail), wasTruncated
	}
	after, statErr := statFile.Stat()
	if statErr != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, after) || ctx.Err() != nil || !lease.valid() {
		writeProblem(w, requestID, http.StatusConflict, "projection_changed", "File changed during projection", "")
		return
	}
	// Verify through the protected root while the original inode is leased,
	// without acquiring a second download slot.
	if err := s.files.VerifyOpenIdentity(ctx, r.URL.Query().Get("path"), before, entry.ResourceVersion); err != nil || !lease.valid() {
		writeProblem(w, requestID, http.StatusConflict, "projection_changed", "File changed during projection", "")
		return
	}
	lease.close() // The projected result no longer depends on the source inode.
	if ctx.Err() != nil {
		writeProblem(w, requestID, http.StatusConflict, "projection_changed", "File projection was interrupted", "")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		fileTailResult
		Projection string `json:"projection"`
	}{fileTailResult: fileTailResult{Path: entry.Path, Content: string(projected), SizeBytes: entry.SizeBytes, ResourceVersion: entry.ResourceVersion, Truncated: truncated}, Projection: "full-context-v1"})
}

func (s *Server) fileRead(w http.ResponseWriter, r *http.Request, requestID string, shareContent bool) {
	values, queryErr := url.ParseQuery(r.URL.RawQuery)
	allowedQuery := []string{"path", "disposition", "mode", "version"}
	if shareContent {
		allowedQuery = []string{"path", "disposition"}
	}
	if r.URL.RawPath != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_path", "文件路径无效", "")
		return
	}
	if queryErr != nil || !strictQuery(values, allowedQuery...) {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "文件查询参数无效", "")
		return
	}
	disposition := values.Get("disposition")
	if disposition == "" {
		disposition = "inline"
	}
	if disposition != "inline" && disposition != "attachment" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_disposition", "文件响应方式无效", "")
		return
	}
	readMode := values.Get("mode")
	if readMode != "" && readMode != "text" && readMode != "thumbnail" && readMode != "preview" && readMode != "office" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_file_mode", "文件读取模式无效", "")
		return
	}
	shareVersionHeaders := r.Header.Values(contract.FileShareVersionHeader)
	if (!shareContent && len(shareVersionHeaders) != 0) ||
		(shareContent && (len(shareVersionHeaders) != 1 ||
			!validFileShareVersion(shareVersionHeaders[0]) || readMode != "")) {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_file_share_version", "文件分享版本无效", "")
		return
	}
	shareResourceVersion := ""
	if shareContent {
		ifMatchHeaders := r.Header.Values("If-Match")
		var ok bool
		if len(ifMatchHeaders) == 1 {
			shareResourceVersion, ok = parseFileShareResourceVersion(ifMatchHeaders[0])
		}
		if !ok {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_file_share_resource_version", "文件分享资源版本无效", "")
			return
		}
	}
	if readMode != "thumbnail" && readMode != "preview" && values.Get("version") != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_file_version", "文件版本参数无效", "")
		return
	}
	transferTimeout := fileTransferMaxDuration
	if readMode == "thumbnail" || readMode == "preview" {
		if disposition != "inline" || values.Get("version") == "" || len(values.Get("version")) > 256 {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_thumbnail_request", "缩略图请求无效", "")
			return
		}
		transferTimeout = fileThumbnailTimeout
	}
	transferContext, cancel := context.WithTimeout(r.Context(), transferTimeout)
	defer cancel()
	if readMode == "office" {
		if r.Method != http.MethodGet || disposition != "inline" {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_file_mode", "文件读取模式无效", "")
			return
		}
		doc, err := s.files.ReadOffice(transferContext, values.Get("path"))
		if err != nil {
			writeFileProblem(w, requestID, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, doc)
		return
	}
	var file io.ReadSeekCloser
	var entry contract.FileEntry
	var err error
	if shareContent {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(contract.FileShareFingerprintTimeout + fileTransferIdleTimeout))
		file, entry, err = s.files.OpenShare(transferContext, values.Get("path"), shareResourceVersion)
	} else {
		file, entry, err = s.files.Open(transferContext, values.Get("path"))
	}
	if err != nil {
		if shareContent {
			writeFileShareProblem(w, requestID, err)
		} else {
			writeFileProblem(w, requestID, err)
		}
		return
	}
	defer file.Close()
	if len(shareVersionHeaders) == 1 && shareVersionHeaders[0] != entry.ShareVersion {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	if readMode == "thumbnail" || readMode == "preview" {
		if values.Get("version") != entry.ResourceVersion {
			writeProblem(w, requestID, http.StatusConflict, "file_conflict", "文件状态已变化", "")
			return
		}
		// Revalidation and cache hits never decode the image or wait for the
		// generation gate; only a miss spends CPU on a resize.
		etag := `"` + readMode + `-` + entry.ResourceVersion + `"`
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		cacheKey := thumbnailCacheKey(entry.Path, entry.ResourceVersion)
		cache := s.thumbnails
		if readMode == "preview" {
			cacheKey = "preview\x00" + cacheKey
			cache = s.previews
		}
		content, contentType, cached := cache.get(cacheKey)
		if cached && readMode == "thumbnail" {
			s.thumbnailDisk.touch(cacheKey)
		}
		if readMode == "preview" && !cached && s.previewJobs != nil {
			// Do not hold a source download slot while waiting for background work.
			_ = file.Close()
			content, contentType, err = s.previewJobs.get(transferContext, entry)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					writeProblem(w, requestID, http.StatusRequestTimeout, "preview_timeout", "预览生成超时", "")
				} else if errors.Is(err, errPreviewQueueFull) {
					writeProblem(w, requestID, http.StatusServiceUnavailable, "preview_busy", "预览队列繁忙", "")
				} else if errors.Is(err, errThumbnailUnavailable) {
					writeProblem(w, requestID, http.StatusUnprocessableEntity, "file_thumbnail_unavailable", "无法生成文件缩略图", "")
				} else {
					writeFileProblem(w, requestID, err)
				}
				return
			}
			// Cancellation or queued work must not bypass current file access checks.
			check, current, checkErr := s.files.Open(transferContext, entry.Path)
			if checkErr != nil {
				writeFileProblem(w, requestID, checkErr)
				return
			}
			_ = check.Close()
			if current.ResourceVersion != entry.ResourceVersion {
				writeProblem(w, requestID, http.StatusConflict, "file_conflict", "文件状态已变化", "")
				return
			}
			cached = true
		}
		if !cached && readMode == "thumbnail" {
			content, contentType, err = s.loadThumbnail(transferContext, file, entry)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					writeProblem(w, requestID, http.StatusRequestTimeout, "thumbnail_timeout", "缩略图生成超时", "")
				} else if errors.Is(err, errThumbnailUnavailable) {
					writeProblem(w, requestID, http.StatusUnprocessableEntity, "file_thumbnail_unavailable", "无法生成文件缩略图", "")
				} else {
					writeFileProblem(w, requestID, err)
				}
				return
			}
			cached = true
		}
		if !cached {
			// Preview decoding is serialized and also shares the thumbnail gate.
			// This bounds combined work without blocking cached responses.
			if readMode == "preview" {
				select {
				case s.previewGate <- struct{}{}:
					defer func() { <-s.previewGate }()
				case <-transferContext.Done():
					writeProblem(w, requestID, http.StatusRequestTimeout, "preview_timeout", "缩略图生成超时", "")
					return
				}
			}
			select {
			case s.thumbnailGate <- struct{}{}:
			case <-transferContext.Done():
				writeProblem(w, requestID, http.StatusRequestTimeout, "thumbnail_timeout", "缩略图生成超时", "")
				return
			}
			var thumbnailErr error
			if readMode == "preview" {
				content, contentType, thumbnailErr = makeFilePreview(file, entry.SizeBytes)
			} else {
				content, contentType, thumbnailErr = makeFileThumbnail(file, entry.SizeBytes)
			}
			<-s.thumbnailGate
			if thumbnailErr != nil {
				status, code, title := http.StatusUnprocessableEntity, "file_thumbnail_unavailable", "无法生成文件缩略图"
				if errors.Is(thumbnailErr, filemanager.ErrTooLarge) {
					status, code, title = http.StatusRequestEntityTooLarge, "file_too_large", "图片超过缩略图处理上限"
				}
				writeProblem(w, requestID, status, code, title, "")
				return
			}
			cache.put(cacheKey, content, contentType)
		}
		if readMode == "thumbnail" && s.previewJobs != nil {
			s.previewJobs.warm(entry)
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", "inline")
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.Header().Set("ETag", etag)
		_, _ = w.Write(content)
		return
	}
	contentType := entry.MIME
	if readMode == "text" {
		if disposition != "inline" || !entry.Editable || entry.SizeBytes > filemanager.MaxTextBytes {
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "text_preview_unavailable", "该文件不能作为文本编辑", "")
			return
		}
		content, readErr := io.ReadAll(io.LimitReader(file, filemanager.MaxTextBytes+1))
		if readErr != nil {
			writeFileProblem(w, requestID, readErr)
			return
		}
		if len(content) > filemanager.MaxTextBytes || !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "text_preview_unavailable", "该文件不是有效的 UTF-8 文本", "")
			return
		}
		w.Header().Set("Content-Disposition", "inline")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("ETag", `"`+entry.ResourceVersion+`"`)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		http.ServeContent(
			httpstream.NewIdleResponseWriter(transferContext, w, fileTransferIdleTimeout),
			r, entry.Name, entry.ModifiedAt, bytes.NewReader(content),
		)
		return
	}
	if disposition == "inline" && activeContent(entry.Name, contentType) {
		contentType = "text/plain; charset=utf-8"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if formatted := mime.FormatMediaType(disposition, map[string]string{"filename": entry.Name}); formatted != "" {
		w.Header().Set("Content-Disposition", formatted)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", `"`+entry.ResourceVersion+`"`)
	w.Header().Set("Content-Security-Policy", fileContentSecurityPolicy(contentType))
	http.ServeContent(
		httpstream.NewIdleResponseWriter(transferContext, w, fileTransferIdleTimeout),
		r, entry.Name, entry.ModifiedAt, file,
	)
}

func makeFileThumbnail(file io.ReadSeeker, size int64) ([]byte, string, error) {
	return makeFileImage(file, size, fileThumbnailMaxWidth, fileThumbnailMaxHeight, fileThumbnailMaxPixels, false)
}

func makeFilePreview(file io.ReadSeeker, size int64) ([]byte, string, error) {
	return makeFileImage(file, size, 1440, 1440, 24_000_000, true)
}

func makeFileImage(file io.ReadSeeker, size int64, maxWidth, maxHeight, maxPixels int, preview bool) ([]byte, string, error) {
	maxBytes := int64(fileGridSourceMaxBytes)
	if preview {
		maxBytes = fileThumbnailMaxBytes
	}
	if size <= 0 || size > maxBytes {
		return nil, "", filemanager.ErrTooLarge
	}
	// The opened inode may still be modified by a site process. Freeze the
	// bounded bytes before inspecting dimensions, metadata or allocating pixels.
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > maxBytes {
		return nil, "", filemanager.ErrTooLarge
	}
	file = bytes.NewReader(data)
	config, format, err := image.DecodeConfig(io.LimitReader(file, maxBytes+1))
	if err != nil || (preview && format != "jpeg" && format != "png" && format != "gif") ||
		(!preview && format != "jpeg" && format != "png" && format != "gif" && format != "webp" && format != "bmp" && format != "tiff") {
		return nil, "", errThumbnailUnavailable
	}
	if config.Width <= 0 || config.Height <= 0 ||
		config.Width > maxPixels/config.Height {
		return nil, "", filemanager.ErrTooLarge
	}
	// 16-bit TIFF/PNG can require twice the memory of ordinary phone photos.
	// Keep a decoded-pixel budget in addition to the compressed-source limit.
	bytesPerPixel := 4
	if config.ColorModel == color.RGBA64Model || config.ColorModel == color.NRGBA64Model {
		bytesPerPixel = 8
	}
	if int64(config.Width)*int64(config.Height) > (256<<20)/int64(bytesPerPixel) {
		return nil, "", filemanager.ErrTooLarge
	}
	if preview && format == "gif" { // Preserve animation through the native original path.
		return nil, "", errThumbnailUnavailable
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	orientation := 1
	if preview && format == "png" {
		if !filePNGStatic(file) {
			return nil, "", errThumbnailUnavailable
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, "", err
		}
	}
	if format == "jpeg" {
		orientation = fileJPEGOrientation(file)
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, "", err
		}
	}
	source, decodedFormat, err := image.Decode(io.LimitReader(file, maxBytes+1))
	if err != nil || decodedFormat != format {
		return nil, "", errThumbnailUnavailable
	}
	width, height := fitImageDimensions(config.Width, config.Height, maxWidth, maxHeight)
	thumbnail := resizeBilinear(source, width, height)
	var output bytes.Buffer
	if format == "jpeg" {
		if err := jpeg.Encode(&output, thumbnail, &jpeg.Options{Quality: 84}); err != nil {
			return nil, "", err
		}
		content := output.Bytes()
		if orientation > 1 {
			// Keep only orientation, not EXIF GPS or embedded original thumbnails.
			segment := []byte{0xff, 0xe1, 0, 34, 'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, byte(orientation), 0, 0, 0, 0, 0, 0, 0}
			content = append(append(append([]byte{}, content[:2]...), segment...), content[2:]...)
		}
		return content, "image/jpeg", nil
	}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&output, thumbnail); err != nil {
		return nil, "", err
	}
	return output.Bytes(), "image/png", nil
}

func thumbnailDimensions(width, height int) (int, int) {
	return fitImageDimensions(width, height, fileThumbnailMaxWidth, fileThumbnailMaxHeight)
}

func fitImageDimensions(width, height, maxWidth, maxHeight int) (int, int) {
	if width <= maxWidth && height <= maxHeight {
		return width, height
	}
	scaleWidth := float64(maxWidth) / float64(width)
	scaleHeight := float64(maxHeight) / float64(height)
	scale := scaleWidth
	if scaleHeight < scale {
		scale = scaleHeight
	}
	resultWidth := max(1, int(float64(width)*scale))
	resultHeight := max(1, int(float64(height)*scale))
	return resultWidth, resultHeight
}

func resizeBilinear(source image.Image, width, height int) *image.NRGBA {
	target := image.NewNRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	sourceWidth, sourceHeight := bounds.Dx(), bounds.Dy()
	if sourceWidth == width && sourceHeight == height {
		for y := range height {
			for x := range width {
				target.Set(x, y, source.At(bounds.Min.X+x, bounds.Min.Y+y))
			}
		}
		return target
	}
	for y := range height {
		sy := (float64(y)+0.5)*float64(sourceHeight)/float64(height) - 0.5
		y0 := max(0, min(sourceHeight-1, int(sy)))
		y1 := min(sourceHeight-1, y0+1)
		fy := sy - float64(y0)
		if fy < 0 {
			fy = 0
		}
		for x := range width {
			sx := (float64(x)+0.5)*float64(sourceWidth)/float64(width) - 0.5
			x0 := max(0, min(sourceWidth-1, int(sx)))
			x1 := min(sourceWidth-1, x0+1)
			fx := sx - float64(x0)
			if fx < 0 {
				fx = 0
			}
			c00 := color.NRGBAModel.Convert(source.At(bounds.Min.X+x0, bounds.Min.Y+y0)).(color.NRGBA)
			c10 := color.NRGBAModel.Convert(source.At(bounds.Min.X+x1, bounds.Min.Y+y0)).(color.NRGBA)
			c01 := color.NRGBAModel.Convert(source.At(bounds.Min.X+x0, bounds.Min.Y+y1)).(color.NRGBA)
			c11 := color.NRGBAModel.Convert(source.At(bounds.Min.X+x1, bounds.Min.Y+y1)).(color.NRGBA)
			target.SetNRGBA(x, y, color.NRGBA{
				R: bilinearChannel(c00.R, c10.R, c01.R, c11.R, fx, fy),
				G: bilinearChannel(c00.G, c10.G, c01.G, c11.G, fx, fy),
				B: bilinearChannel(c00.B, c10.B, c01.B, c11.B, fx, fy),
				A: bilinearChannel(c00.A, c10.A, c01.A, c11.A, fx, fy),
			})
		}
	}
	return target
}

func bilinearChannel(c00, c10, c01, c11 uint8, fx, fy float64) uint8 {
	top := float64(c00)*(1-fx) + float64(c10)*fx
	bottom := float64(c01)*(1-fx) + float64(c11)*fx
	value := top*(1-fy) + bottom*fy
	return uint8(max(0, min(255, int(value+0.5))))
}

func (s *Server) fileWrite(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "文件查询参数无效", "")
		return
	}
	if contentType := strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]); contentType != "application/json" {
		writeProblem(w, requestID, http.StatusUnsupportedMediaType, "json_required", "必须提交 JSON", "")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, filemanager.MaxTextBytes+(64<<10))
	var input contract.FileWriteRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "文件内容无效", "")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "文件内容无效", "")
		return
	}
	var entry contract.FileEntry
	var err error
	if input.OfficeEdits != nil || input.ExpectedContentVersion != "" {
		entry, err = s.files.WriteOffice(r.Context(), r.URL.Query().Get("path"), input)
	} else {
		entry, err = s.files.WriteText(r.Context(), r.URL.Query().Get("path"), input)
	}
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, contract.FileWriteResult{Entry: entry})
}

func (s *Server) fileUpload(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path", "name", "overwrite") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "上传参数无效", "")
		return
	}
	if strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]) != "application/octet-stream" {
		writeProblem(w, requestID, http.StatusUnsupportedMediaType, "binary_required", "上传必须使用二进制内容", "")
		return
	}
	overwrite := false
	if raw := r.URL.Query().Get("overwrite"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_overwrite", "覆盖选项无效", "")
			return
		}
		overwrite = parsed
	}
	if r.ContentLength > filemanager.MaxUploadBytes {
		writeFileProblem(w, requestID, filemanager.ErrTooLarge)
		return
	}
	transferContext, cancel := context.WithTimeout(r.Context(), fileTransferMaxDuration)
	defer cancel()
	r.Body = http.MaxBytesReader(w, r.Body, filemanager.MaxUploadBytes+1)
	content := httpstream.NewIdleReader(
		transferContext, w, r.Body, fileTransferIdleTimeout,
	)
	entry, err := s.files.Upload(
		transferContext,
		r.URL.Query().Get("path"),
		r.URL.Query().Get("name"),
		content,
		r.ContentLength,
		overwrite,
	)
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

func (s *Server) fileTransferExport(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path", "resourceVersion", "offset", "transferVersion") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "传输参数无效", "")
		return
	}
	entry, err := s.files.Stat(r.URL.Query().Get("path"))
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	if entry.Kind != "file" && entry.Kind != "directory" {
		writeFileProblem(w, requestID, filemanager.ErrNotRegular)
		return
	}
	if expected := r.URL.Query().Get("resourceVersion"); expected == "" || expected != entry.ResourceVersion {
		writeFileProblem(w, requestID, filemanager.ErrConflict)
		return
	}
	offset := int64(0)
	version := r.URL.Query().Get("transferVersion")
	if raw := r.URL.Query().Get("offset"); raw != "" {
		var parseErr error
		offset, parseErr = strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || offset < 0 || offset > entry.SizeBytes || entry.Kind != "file" || version != "1" {
			writeFileProblem(w, requestID, filemanager.ErrInvalidPath)
			return
		}
	}
	if version != "" && version != "1" {
		writeFileProblem(w, requestID, filemanager.ErrInvalidPath)
		return
	}
	transferMetadata := contract.FileTransferMetadata{
		Name: entry.Name, Kind: entry.Kind, SizeBytes: entry.SizeBytes,
		ResourceVersion: entry.ResourceVersion,
	}
	if version == "1" {
		transferMetadata.Offset, transferMetadata.TransferVersion = offset, 1
		transferMetadata.Mode, transferMetadata.ModifiedAt = fileTransferMode(entry.Mode), &entry.ModifiedAt
	}
	metadata, err := json.Marshal(transferMetadata)
	if err != nil {
		writeProblem(w, requestID, http.StatusInternalServerError, "transfer_metadata_failed", "无法准备文件传输", "")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set(fileTransferMetadataHeader, base64.RawURLEncoding.EncodeToString(metadata))
	w.Header().Set("Trailer", fileTransferResultTrailer)
	transferContext, cancel := context.WithTimeout(r.Context(), fileTransferMaxDuration)
	defer cancel()
	output := httpstream.NewIdleResponseWriter(transferContext, w, fileTransferIdleTimeout)
	w.WriteHeader(http.StatusOK)
	var transferErr error
	if entry.Kind == "file" {
		var file io.ReadSeekCloser
		file, entry, transferErr = s.files.Open(transferContext, entry.Path)
		if transferErr == nil && entry.ResourceVersion != r.URL.Query().Get("resourceVersion") {
			transferErr = filemanager.ErrConflict
		}
		if file != nil {
			defer file.Close()
		}
		if transferErr == nil {
			_, transferErr = file.Seek(offset, io.SeekStart)
		}
		if transferErr == nil {
			_, transferErr = io.CopyBuffer(output, file, make([]byte, 64<<10))
		}
		if transferErr == nil {
			current, statErr := s.files.Stat(entry.Path)
			if statErr != nil || current.ResourceVersion != entry.ResourceVersion {
				transferErr = filemanager.ErrConflict
			}
		}
	} else {
		_, transferErr = s.files.ExportDirectory(
			transferContext, entry.Path, entry.ResourceVersion, output,
		)
	}
	if transferErr == nil {
		w.Header().Set(fileTransferResultTrailer, "ok")
	} else {
		w.Header().Set(fileTransferResultTrailer, "error")
	}
}

func (s *Server) fileTransferImport(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path", "name", "kind", "size") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "传输参数无效", "")
		return
	}
	if strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]) != "application/octet-stream" {
		writeProblem(w, requestID, http.StatusUnsupportedMediaType, "binary_required", "传输必须使用二进制内容", "")
		return
	}
	size, err := strconv.ParseInt(r.URL.Query().Get("size"), 10, 64)
	if err != nil || size < 0 {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_size", "传输大小无效", "")
		return
	}
	transferContext, cancel := context.WithTimeout(r.Context(), fileTransferMaxDuration)
	defer cancel()
	content := httpstream.NewIdleReader(transferContext, w, r.Body, fileTransferIdleTimeout)
	var entry contract.FileEntry
	switch r.URL.Query().Get("kind") {
	case "file":
		if size > filemanager.MaxUploadBytes {
			writeFileProblem(w, requestID, filemanager.ErrTooLarge)
			return
		}
		entry, err = s.files.Upload(
			transferContext, r.URL.Query().Get("path"), r.URL.Query().Get("name"),
			&exactTransferReader{source: content, remaining: size}, size, false,
		)
	case "directory":
		entry, err = s.files.ImportDirectory(
			transferContext, r.URL.Query().Get("path"), r.URL.Query().Get("name"), content,
		)
	default:
		err = filemanager.ErrNotRegular
	}
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusCreated, entry)
}

type exactTransferReader struct {
	source    io.Reader
	remaining int64
	checked   bool
}

func (r *exactTransferReader) Read(output []byte) (int, error) {
	if r.remaining > 0 {
		if int64(len(output)) > r.remaining {
			output = output[:r.remaining]
		}
		count, err := r.source.Read(output)
		r.remaining -= int64(count)
		if err == io.EOF && r.remaining > 0 {
			return count, io.ErrUnexpectedEOF
		}
		return count, err
	}
	if r.checked {
		return 0, io.EOF
	}
	r.checked = true
	var probe [1]byte
	count, err := r.source.Read(probe[:])
	if count > 0 {
		return 0, filemanager.ErrTooLarge
	}
	if err == nil {
		return 0, io.ErrNoProgress
	}
	return 0, err
}

func (s *Server) fileAction(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "文件操作参数无效", "")
		return
	}
	var input contract.FileActionRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "文件操作无效", "")
		return
	}
	result, err := s.files.Action(r.Context(), input)
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	status := http.StatusOK
	if len(result.Failed) > 0 {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, result)
}

func strictQuery(values map[string][]string, allowed ...string) bool {
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

func validFileShareVersion(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") || value != strings.ToLower(value) {
		return false
	}
	digest, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && len(digest) == sha256.Size
}

func parseFileShareResourceVersion(value string) (string, bool) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", false
	}
	version := value[1 : len(value)-1]
	return version, validFileShareVersion(version)
}

func activeContent(name, contentType string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".html") ||
		strings.HasSuffix(lower, ".htm") ||
		strings.HasSuffix(lower, ".svg") ||
		contentType == "text/html" ||
		contentType == "image/svg+xml" ||
		contentType == "application/xhtml+xml"
}

func fileContentSecurityPolicy(contentType string) string {
	mediaType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if strings.HasPrefix(mediaType, "audio/") || strings.HasPrefix(mediaType, "video/") {
		// Media has no document subresources and does not need a CSP sandbox.
		// Keeping the sandbox flag here can make native media loading behave
		// differently across browsers while providing no additional protection.
		return "default-src 'none'"
	}
	return "default-src 'none'; sandbox"
}

func writeFileShareProblem(w http.ResponseWriter, requestID string, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		w.Header().Set("Retry-After", "60")
		writeProblem(w, requestID, http.StatusGatewayTimeout, "file_share_fingerprint_timeout", "文件指纹计算超时，请稍后重试", "")
		return
	}
	writeFileProblem(w, requestID, err)
}

func writeFileProblem(w http.ResponseWriter, requestID string, err error) {
	status, code, title := http.StatusUnprocessableEntity, "file_operation_failed", "文件操作失败"
	var maxBytesError *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesError):
		status, code, title = http.StatusRequestEntityTooLarge, "file_too_large", "文件超过允许的大小"
	case errors.Is(err, os.ErrNotExist):
		status, code, title = http.StatusNotFound, "file_not_found", "文件不存在"
	case errors.Is(err, os.ErrPermission):
		status, code, title = http.StatusForbidden, "file_permission_denied", "文件权限不足"
	case errors.Is(err, filemanager.ErrInvalidPath),
		errors.Is(err, filemanager.ErrRootOperation),
		errors.Is(err, filemanager.ErrAction),
		errors.Is(err, filemanager.ErrBatchTooLarge),
		errors.Is(err, filemanager.ErrInvalidEncoding):
		status, code, title = http.StatusBadRequest, "file_request_invalid", "文件请求无效"
	case errors.Is(err, filemanager.ErrProtected):
		status, code, title = http.StatusForbidden, "file_path_protected", "KPanel 保护目录不可访问"
	case errors.Is(err, filemanager.ErrReadOnly):
		status, code, title = http.StatusForbidden, "file_path_read_only", "系统虚拟目录仅支持查看"
	case errors.Is(err, filemanager.ErrSymlink):
		status, code, title = http.StatusUnprocessableEntity, "file_symlink_rejected", "符号链接不能在面板中打开"
	case errors.Is(err, filemanager.ErrConflict),
		errors.Is(err, filemanager.ErrAlreadyExists):
		status, code, title = http.StatusConflict, "file_conflict", "文件状态冲突"
	case errors.Is(err, filemanager.ErrReceiveChecksum):
		status, code, title = http.StatusConflict, "file_transfer_checksum", "传输内容校验失败"
	case errors.Is(err, filemanager.ErrReceiveUnavailable):
		status, code, title = http.StatusServiceUnavailable, "file_transfer_state_unavailable", "传输恢复记录不可用"
	case errors.Is(err, filemanager.ErrTooLarge):
		status, code, title = http.StatusRequestEntityTooLarge, "file_too_large", "文件超过允许的大小"
	case errors.Is(err, filemanager.ErrBusy):
		status, code, title = http.StatusTooManyRequests, "file_transfer_busy", "文件传输任务繁忙"
	case errors.Is(err, filemanager.ErrTrashFull):
		status, code, title = http.StatusInsufficientStorage, "file_trash_full", "回收站已满"
	case errors.Is(err, filemanager.ErrTrashMetadata):
		status, code, title = http.StatusUnprocessableEntity, "file_trash_not_restorable", "回收站项目无法恢复"
	case errors.Is(err, filemanager.ErrInvalidArchive):
		status, code, title = http.StatusUnprocessableEntity, "file_archive_invalid", "压缩包格式或内容无效"
	case errors.Is(err, filemanager.ErrOfficeUnsupported):
		status, code, title = http.StatusUnprocessableEntity, "office_unsupported", "文档结构暂不支持"
	case errors.Is(err, filemanager.ErrOfficeInvalidEdit):
		status, code, title = http.StatusUnprocessableEntity, "office_edit_invalid", "只能修改支持的文本内容"
	case errors.Is(err, filemanager.ErrNotDirectory),
		errors.Is(err, filemanager.ErrNotRegular):
		status, code, title = http.StatusUnprocessableEntity, "file_type_invalid", "文件类型不支持此操作"
	}
	writeProblem(w, requestID, status, code, title, safeDetail(err))
}

// fileJPEGOrientation reads only bounded JPEG headers; malformed EXIF is ignored.
func fileJPEGOrientation(file io.Reader) int {
	r := io.LimitReader(file, 256<<10)
	var marker [2]byte
	if _, err := io.ReadFull(r, marker[:]); err != nil || marker != [2]byte{0xff, 0xd8} {
		return 1
	}
	for {
		if _, err := io.ReadFull(r, marker[:]); err != nil || marker[0] != 0xff {
			return 1
		}
		// JPEG B.1.1.2 allows arbitrary FF fill bytes before a marker.
		for marker[1] == 0xff {
			if _, err := io.ReadFull(r, marker[1:]); err != nil {
				return 1
			}
		}
		if marker[1] == 0xda || marker[1] == 0xd9 || marker[1] == 0 {
			return 1
		}
		if marker[1] == 0x01 || (marker[1] >= 0xd0 && marker[1] <= 0xd7) {
			continue
		}
		var length [2]byte
		if _, err := io.ReadFull(r, length[:]); err != nil {
			return 1
		}
		n := int(binary.BigEndian.Uint16(length[:])) - 2
		if n < 0 {
			return 1
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(r, data); err != nil {
			return 1
		}
		if marker[1] != 0xe1 || len(data) < 14 || string(data[:6]) != "Exif\x00\x00" {
			continue
		}
		tiff := data[6:]
		var order binary.ByteOrder
		switch string(tiff[:2]) {
		case "II":
			order = binary.LittleEndian
		case "MM":
			order = binary.BigEndian
		default:
			return 1
		}
		if order.Uint16(tiff[2:4]) != 42 {
			return 1
		}
		offset := uint64(order.Uint32(tiff[4:8]))
		if offset+2 > uint64(len(tiff)) {
			return 1
		}
		count := int(order.Uint16(tiff[offset : offset+2]))
		for i := 0; i < count; i++ {
			start := offset + 2 + uint64(i)*12
			if start+12 > uint64(len(tiff)) {
				return 1
			}
			field := tiff[start : start+12]
			if order.Uint16(field[:2]) == 0x112 && order.Uint16(field[2:4]) == 3 && order.Uint32(field[4:8]) == 1 {
				value := int(order.Uint16(field[8:10]))
				if value >= 1 && value <= 8 {
					return value
				}
				return 1
			}
		}
		return 1
	}
}

// PNG animation control precedes IDAT. Uncertain/oversized headers use the
// native original rather than silently flattening APNG into its first frame.
func filePNGStatic(file io.Reader) bool {
	r := io.LimitReader(file, 256<<10)
	var signature [8]byte
	if _, err := io.ReadFull(r, signature[:]); err != nil || string(signature[:]) != "\x89PNG\r\n\x1a\n" {
		return false
	}
	for {
		var header [8]byte
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return false
		}
		switch string(header[4:]) {
		case "acTL":
			return false
		case "IDAT":
			return true
		}
		length := int64(binary.BigEndian.Uint32(header[:4]))
		if _, err := io.CopyN(io.Discard, r, length+4); err != nil {
			return false
		}
	}
}
