package panel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/bittorrent"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/httpstream"
	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

const maxPanelRemoteDownloads = 2

func (s *Server) handleFileRemoteDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "file_query_invalid", "远程下载参数无效", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	var input contract.FileRemoteDownloadRequest
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	source, err := validateRemoteDownloadInput(&input)
	if err != nil || !validFileRemoteDownloadTarget(input.TargetDirectory) ||
		(input.Name != "" && !validFileRemoteDownloadName(input.Name)) {
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "remote_download_invalid", "请检查下载地址、目标目录和保存名称。", "")
		return
	}
	// Pass the validated canonical URL to both execution modes. The URL stays in
	// Panel memory only; background persistence receives SourceDisplay instead.
	if input.Background {
		s.startFileRemoteDownloadJob(w, r, input, source, session.User.ID)
		return
	}
	select {
	case s.remoteDownloadGate <- struct{}{}:
		defer func() { <-s.remoteDownloadGate }()
	default:
		w.Header().Set("Retry-After", "1")
		s.writeProblem(w, r, http.StatusTooManyRequests, "remote_download_busy", "当前远程下载较多，请稍后重试。", "")
		return
	}

	change := map[string]any{
		"source":          source,
		"targetDirectory": input.TargetDirectory,
	}
	if input.Name != "" {
		change["requestedName"] = input.Name
	}
	if err := s.audit(r, session.User.ID, "file.remote_download", "directory", input.TargetDirectory, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}

	transferContext, cancel := context.WithTimeout(r.Context(), panelFileTransferMaxDuration)
	defer cancel()
	// The work context has a hard deadline, but the response writer must remain
	// usable long enough to deliver that deadline as the terminal stream event.
	output := httpstream.NewIdleResponseWriter(r.Context(), w, panelFileTransferIdleTimeout)
	responseController := http.NewResponseController(output)
	var encoder *json.Encoder
	streamStarted := false
	writeEvent := func(event contract.FileTransferEvent) bool {
		if !streamStarted {
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			output.WriteHeader(http.StatusOK)
			encoder = json.NewEncoder(output)
			streamStarted = true
		}
		if err := encoder.Encode(event); err != nil {
			cancel()
			return false
		}
		if err := responseController.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
			cancel()
			return false
		}
		return true
	}

	result := s.executeFileRemoteDownload(transferContext, input, requestID(r), writeEvent)
	completed := cloneRemoteDownloadChange(change)
	completed["bytes"] = result.LoadedBytes
	if result.Name != "" {
		completed["targetName"] = result.Name
	}
	if result.TotalBytes > 0 {
		completed["expectedBytes"] = result.TotalBytes
	}
	if result.State == "complete" && result.Entry != nil {
		_ = s.audit(r, session.User.ID, "file.remote_download", "file", result.Entry.Path, "success", completed)
		return
	}
	completed["errorCode"] = result.Code
	_ = s.audit(r, session.User.ID, "file.remote_download", "directory", input.TargetDirectory, "failure", completed)
}

func validateRemoteDownloadInput(input *contract.FileRemoteDownloadRequest) (string, error) {
	if input.Acceleration != "" && input.Acceleration != "auto" && input.Acceleration != "off" {
		return "", bittorrent.ErrMetadata
	}
	input.URL = strings.TrimSpace(input.URL)
	if input.SourceKind != "" && input.SourceKind != "torrent" {
		return "", bittorrent.ErrMetadata
	}
	if len(input.Torrent) > 0 || strings.HasPrefix(input.URL, "magnet:") {
		if !input.Background {
			return "", bittorrent.ErrMetadata
		}
		if _, err := bittorrent.Parse(input.URL, input.Torrent); err != nil {
			return "", err
		}
		input.SourceKind = "torrent"
		return "bittorrent", nil
	}
	u, err := remotedownload.ValidateURL(input.URL)
	if err != nil {
		return "", err
	}
	input.URL = u.String()
	if input.SourceKind == "torrent" {
		if !input.Background {
			return "", bittorrent.ErrMetadata
		}
		return "bittorrent", nil
	}
	return remotedownload.SourceDisplay(u), nil
}

func validFileRemoteDownloadTarget(value string) bool {
	if !validFileDownloadPath(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validFileRemoteDownloadName(name string) bool {
	if name == "" || strings.TrimSpace(name) != name || name == "." || name == ".." ||
		len(name) > 255 || !utf8.ValidString(name) || strings.ContainsAny(name, `/\`) ||
		strings.HasPrefix(name, ".kpanel-") {
		return false
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validFileRemoteDownloadEntry(entry contract.FileEntry, directory, name string, loadedBytes int64) bool {
	return entry.Kind == "file" && entry.Name == name && entry.Path == path.Join(directory, name) &&
		entry.SizeBytes == loadedBytes && entry.ResourceVersion != ""
}

func fileRemoteDownloadName(requested string, response *http.Response) string {
	if validFileRemoteDownloadName(requested) {
		return requested
	}
	if disposition := response.Header.Get("Content-Disposition"); disposition != "" {
		if _, parameters, err := mime.ParseMediaType(disposition); err == nil {
			if name := strings.TrimSpace(parameters["filename"]); validFileRemoteDownloadName(name) {
				return name
			}
		}
	}
	// Never derive the saved name from the URL path. Signed download URLs often
	// carry bearer material in path segments, while names are persisted in audit
	// events and streamed status. A fixed fallback keeps the complete URL secret.
	return "download"
}

func fileRemoteDownloadError(err error) (string, string) {
	var statusError *remotedownload.StatusError
	switch {
	case errors.Is(err, bittorrent.ErrVersion):
		return "bt_version_unsupported", "此候选支持 BitTorrent v1 种子和 btih 磁力链接。"
	case errors.Is(err, bittorrent.ErrMetadata), errors.Is(err, bittorrent.ErrIntegrity):
		return "bt_metadata_invalid", "种子信息无效、超过限制或校验失败。"
	case errors.Is(err, bittorrent.ErrStorage):
		return "bt_storage_unavailable", "BT 暂存区不可用或磁盘空间不足。"
	case errors.Is(err, bittorrent.ErrNoPeers):
		return "bt_no_peers", "暂未找到可用节点或连续 2 分钟无有效下载，请稍后重试。"
	case errors.Is(err, context.Canceled):
		return "remote_download_cancelled", "停止请求已发送；文件可能已经完成保存，请刷新目标目录确认结果。"
	case errors.Is(err, context.DeadlineExceeded):
		return "remote_download_timeout", "远程下载超过 2 小时，已停止。"
	case errors.Is(err, remotedownload.ErrAddressBlocked):
		return "remote_download_address_blocked", "该地址不是允许访问的公开网络地址。"
	case errors.Is(err, remotedownload.ErrRedirectRejected):
		return "remote_download_redirect_rejected", "远程服务器跳转到了不允许的地址。"
	case errors.Is(err, remotedownload.ErrTLS):
		return "remote_download_tls_failed", "远程服务器证书校验失败。"
	case errors.Is(err, remotedownload.ErrEncoding):
		return "remote_download_encoding_unsupported", "远程服务器返回了不支持的内容编码。"
	case errors.Is(err, remotedownload.ErrPartialContent):
		return "remote_download_partial_unsupported", "远程服务器只返回了部分内容，未保存为完整文件。"
	case errors.Is(err, remotedownload.ErrSourceChanged):
		return "remote_download_source_changed", "来源文件或续传范围发生变化，已停止拼接。"
	case errors.Is(err, remotedownload.ErrIdleTimeout):
		return "remote_download_idle_timeout", "远程服务器 45 秒内没有返回数据。"
	case errors.As(err, &statusError):
		return "remote_download_upstream_status", "远程服务器返回 HTTP " + strconv.Itoa(statusError.StatusCode) + "。"
	default:
		return "remote_download_unreachable", "无法连接远程服务器，请检查地址后重试。"
	}
}

func cloneRemoteDownloadChange(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}

type fileRemoteDownloadLimitReader struct {
	source    io.Reader
	remaining int64
	exceeded  atomic.Bool
}

func (r *fileRemoteDownloadLimitReader) Read(buffer []byte) (int, error) {
	if r.exceeded.Load() {
		return 0, filemanager.ErrTooLarge
	}
	limit := int64(len(buffer))
	if limit > r.remaining+1 {
		limit = r.remaining + 1
	}
	count, err := r.source.Read(buffer[:limit])
	if int64(count) > r.remaining {
		allowed := int(r.remaining)
		r.remaining = 0
		r.exceeded.Store(true)
		return allowed, filemanager.ErrTooLarge
	}
	r.remaining -= int64(count)
	return count, err
}

type fileRemoteDownloadUploadBody struct {
	source    io.Reader
	closer    io.Closer
	loaded    *atomic.Int64
	closeOnce sync.Once
	closeErr  error
}

func (body *fileRemoteDownloadUploadBody) Read(buffer []byte) (int, error) {
	count, err := body.source.Read(buffer)
	if count > 0 {
		body.loaded.Add(int64(count))
	}
	return count, err
}

func (body *fileRemoteDownloadUploadBody) Close() error {
	body.closeOnce.Do(func() {
		body.closeErr = body.closer.Close()
	})
	return body.closeErr
}
