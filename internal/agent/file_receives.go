package agent

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/httpstream"
)

func (s *Server) fileReceives(w http.ResponseWriter, r *http.Request) {
	id := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "id", "sourceKey", "offset", "sha256") {
		writeFileProblem(w, id, filemanager.ErrInvalidPath)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), fileTransferMaxDuration)
	defer cancel()
	var session contract.FileReceiveSession
	var err error
	switch r.Method {
	case http.MethodGet:
		if !strictQuery(r.URL.Query(), "id", "sourceKey") {
			err = filemanager.ErrInvalidPath
			break
		}
		session, err = s.files.ReceiveStatus(ctx, r.URL.Query().Get("id"), r.URL.Query().Get("sourceKey"))
	case http.MethodPut:
		if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/octet-stream" {
			writeProblem(w, id, http.StatusUnsupportedMediaType, "binary_required", "传输分块必须使用二进制内容", "")
			return
		}
		offset, parseErr := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		if parseErr != nil {
			err = filemanager.ErrInvalidPath
			break
		}
		body := http.MaxBytesReader(w, r.Body, contract.FileTransferChunkBytes)
		content := httpstream.NewIdleReader(ctx, w, body, fileTransferIdleTimeout)
		session, err = s.files.WriteReceiveChunk(ctx, r.URL.Query().Get("id"), r.URL.Query().Get("sourceKey"), offset, content, r.URL.Query().Get("sha256"))
	case http.MethodPost:
		if r.URL.RawQuery != "" {
			err = filemanager.ErrInvalidPath
			break
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var input contract.FileReceiveRequest
		if decodeJSON(w, r, &input) != nil {
			err = filemanager.ErrInvalidPath
			break
		}
		switch {
		case input.Operation == "create" && input.Input != nil && input.ID == "" && input.SourceKey == "" && input.SizeBytes == 0 && input.SHA256 == "":
			session, err = s.files.BeginReceive(ctx, *input.Input)
		case input.Operation == "commit" && input.Input == nil:
			session, err = s.files.CommitReceive(ctx, input.ID, input.SourceKey, input.SizeBytes, input.SHA256)
		case input.Operation == "abort" && input.Input == nil && input.SizeBytes == 0 && input.SHA256 == "":
			err = s.files.AbortReceive(ctx, input.ID, input.SourceKey)
			if err == nil {
				writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
				return
			}
		default:
			err = filemanager.ErrInvalidPath
		}
	default:
		w.Header().Set("Allow", "GET, POST, PUT")
		writeProblem(w, id, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	}
	if err != nil {
		writeFileProblem(w, id, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func fileTransferMode(mode string) string {
	if len(mode) != 10 {
		return ""
	}
	var bits uint32
	for i, character := range mode[1:] {
		if character == rune("rwx"[i%3]) || i%3 == 2 && (character == 's' || character == 't') {
			bits |= 1 << (8 - i)
		}
	}
	return strconv.FormatUint(uint64(bits), 8)
}
