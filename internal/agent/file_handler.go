package agent

import (
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

// NewFileHandler exposes only the Agent file-manager routes. It is used by
// the lightweight node's root broker, which must not start the full Agent
// socket or expose unrelated host APIs.
func NewFileHandler(files *filemanager.Manager) http.Handler {
	server := &Server{files: files, now: time.Now}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestID()
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		if files == nil {
			writeProblem(w, requestID, http.StatusServiceUnavailable, "files_unavailable", "文件管理服务不可用", "")
			return
		}
		switch r.URL.Path {
		case "/v1/files":
			server.requireMethod(w, r, requestID, http.MethodGet, server.fileList)
		case "/v1/files/entry":
			server.requireMethod(w, r, requestID, http.MethodGet, server.fileEntry)
		case "/v1/files/entries":
			server.requireMethod(w, r, requestID, http.MethodPost, server.fileEntries)
		case "/v1/files/trash":
			server.requireMethod(w, r, requestID, http.MethodGet, server.fileTrashList)
		case "/v1/files/content":
			server.fileContent(w, r, requestID)
		case "/v1/files/archive":
			server.fileArchive(w, r)
		case "/v1/files/text":
			server.requireMethod(w, r, requestID, http.MethodGet, server.fileText)
		case "/v1/files/tail":
			server.requireMethod(w, r, requestID, http.MethodGet, server.fileTail)
		case "/v1/files/upload":
			server.requireMethod(w, r, requestID, http.MethodPost, server.fileUpload)
		case "/v1/files/transfer/export":
			server.requireMethod(w, r, requestID, http.MethodGet, server.fileTransferExport)
		case "/v1/files/transfer/import":
			server.requireMethod(w, r, requestID, http.MethodPost, server.fileTransferImport)
		case "/v1/files/actions":
			server.requireMethod(w, r, requestID, http.MethodPost, server.fileAction)
		default:
			writeProblem(w, requestID, http.StatusNotFound, "not_found", "资源不存在", "")
		}
	})
}
