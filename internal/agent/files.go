package agent

import (
	"bytes"
	"context"
	"encoding/base64"
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
	"os"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/httpstream"
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
	fileThumbnailMaxPixels   = 8_000_000
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
		s.fileRead(w, r, requestID)
	case http.MethodPut:
		s.fileWrite(w, r, requestID)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead+", "+http.MethodPut)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
	}
}

func (s *Server) fileArchive(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "selection", "name") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_query", "压缩下载参数无效", "")
		return
	}
	selection := r.URL.Query().Get("selection")
	name := r.URL.Query().Get("name")
	if len(selection) == 0 || len(selection) > fileArchiveQueryMaxBytes || !validArchiveDownloadName(name) {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_archive_download", "压缩下载参数无效", "")
		return
	}
	var input contract.FileArchiveDownloadRequest
	if err := json.Unmarshal([]byte(selection), &input); err != nil {
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
		err := s.files.ExportZIP(
			transferContext, input.Sources, input.ExpectedResourceVersions, writer,
		)
		_ = writer.CloseWithError(err)
	}()

	buffer := make([]byte, 64<<10)
	read, err := reader.Read(buffer)
	if err != nil && read == 0 {
		writeFileProblem(w, requestID, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
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
		_, _ = io.CopyBuffer(output, reader, buffer)
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
	for _, source := range input.Sources {
		version, ok := input.ExpectedResourceVersions[source]
		if !ok || version == "" || len(version) > 256 || source == "" || len(source) > 4096 ||
			!strings.HasPrefix(source, "/") || strings.ContainsAny(source, "\\\x00") || path.Clean(source) != source {
			return false
		}
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

func (s *Server) fileRead(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.URL.RawPath != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_path", "文件路径无效", "")
		return
	}
	values := r.URL.Query()
	if !strictQuery(values, "path", "disposition", "mode", "version") {
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
	if readMode != "" && readMode != "text" && readMode != "thumbnail" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_file_mode", "文件读取模式无效", "")
		return
	}
	if readMode != "thumbnail" && values.Get("version") != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_file_version", "文件版本参数无效", "")
		return
	}
	transferTimeout := fileTransferMaxDuration
	if readMode == "thumbnail" {
		if disposition != "inline" || values.Get("version") == "" || len(values.Get("version")) > 256 {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_thumbnail_request", "缩略图请求无效", "")
			return
		}
		transferTimeout = fileThumbnailTimeout
	}
	transferContext, cancel := context.WithTimeout(r.Context(), transferTimeout)
	defer cancel()
	if readMode == "thumbnail" {
		select {
		case s.thumbnailGate <- struct{}{}:
			defer func() { <-s.thumbnailGate }()
		case <-transferContext.Done():
			writeProblem(w, requestID, http.StatusRequestTimeout, "thumbnail_timeout", "缩略图生成超时", "")
			return
		}
	}
	file, entry, err := s.files.Open(transferContext, values.Get("path"))
	if err != nil {
		writeFileProblem(w, requestID, err)
		return
	}
	defer file.Close()
	if readMode == "thumbnail" {
		if values.Get("version") != entry.ResourceVersion {
			writeProblem(w, requestID, http.StatusConflict, "file_conflict", "文件状态已变化", "")
			return
		}
		content, contentType, thumbnailErr := makeFileThumbnail(file, entry.SizeBytes)
		if thumbnailErr != nil {
			status, code, title := http.StatusUnprocessableEntity, "file_thumbnail_unavailable", "无法生成文件缩略图"
			if errors.Is(thumbnailErr, filemanager.ErrTooLarge) {
				status, code, title = http.StatusRequestEntityTooLarge, "file_too_large", "图片超过缩略图处理上限"
			}
			writeProblem(w, requestID, status, code, title, "")
			return
		}
		etag := `"thumbnail-` + entry.ResourceVersion + `"`
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
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
	if size <= 0 || size > fileThumbnailMaxBytes {
		return nil, "", filemanager.ErrTooLarge
	}
	config, format, err := image.DecodeConfig(io.LimitReader(file, fileThumbnailMaxBytes+1))
	if err != nil || (format != "jpeg" && format != "png" && format != "gif") {
		return nil, "", errThumbnailUnavailable
	}
	if config.Width <= 0 || config.Height <= 0 ||
		config.Width > fileThumbnailMaxPixels/config.Height {
		return nil, "", filemanager.ErrTooLarge
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", err
	}
	source, decodedFormat, err := image.Decode(io.LimitReader(file, fileThumbnailMaxBytes+1))
	if err != nil || decodedFormat != format {
		return nil, "", errThumbnailUnavailable
	}
	width, height := thumbnailDimensions(config.Width, config.Height)
	thumbnail := resizeBilinear(source, width, height)
	var output bytes.Buffer
	if format == "jpeg" {
		if err := jpeg.Encode(&output, thumbnail, &jpeg.Options{Quality: 84}); err != nil {
			return nil, "", err
		}
		return output.Bytes(), "image/jpeg", nil
	}
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&output, thumbnail); err != nil {
		return nil, "", err
	}
	return output.Bytes(), "image/png", nil
}

func thumbnailDimensions(width, height int) (int, int) {
	if width <= fileThumbnailMaxWidth && height <= fileThumbnailMaxHeight {
		return width, height
	}
	scaleWidth := float64(fileThumbnailMaxWidth) / float64(width)
	scaleHeight := float64(fileThumbnailMaxHeight) / float64(height)
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
	entry, err := s.files.WriteText(r.Context(), r.URL.Query().Get("path"), input)
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
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path", "resourceVersion") {
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
	metadata, err := json.Marshal(contract.FileTransferMetadata{
		Name: entry.Name, Kind: entry.Kind, SizeBytes: entry.SizeBytes,
		ResourceVersion: entry.ResourceVersion,
	})
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
	case errors.Is(err, filemanager.ErrNotDirectory),
		errors.Is(err, filemanager.ErrNotRegular):
		status, code, title = http.StatusUnprocessableEntity, "file_type_invalid", "文件类型不支持此操作"
	}
	writeProblem(w, requestID, status, code, title, safeDetail(err))
}
