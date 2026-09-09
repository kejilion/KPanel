package agent

import (
	"errors"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"net/http"
	"os"
)

func (s *Server) fileArchiveContents(w http.ResponseWriter, r *http.Request) {
	id := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeFileProblem(w, id, filemanager.ErrInvalidPath)
		return
	}
	var input contract.FileArchiveQuery
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, id, http.StatusBadRequest, "invalid_json", "文件请求无效", "")
		return
	}
	result, err := s.files.ArchiveContents(r.Context(), input)
	if err != nil {
		writeFileProblem(w, id, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) fileArchiveJobs(w http.ResponseWriter, r *http.Request) {
	id := requestIDFrom(w)
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "id") {
		writeFileProblem(w, id, filemanager.ErrInvalidPath)
		return
	}
	fail := func(err error) {
		if errors.Is(err, filemanager.ErrArchiveJobsUnavailable) {
			writeProblem(w, id, http.StatusServiceUnavailable, "archive_jobs_unavailable", err.Error(), "")
		} else {
			writeFileProblem(w, id, err)
		}
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.files.ArchiveJobs()
		if err != nil {
			fail(err)
			return
		}
		if jobID := r.URL.Query().Get("id"); jobID != "" {
			for _, item := range items {
				if item.ID == jobID {
					writeJSON(w, http.StatusOK, item)
					return
				}
			}
			fail(os.ErrNotExist)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		if r.URL.RawQuery != "" {
			fail(filemanager.ErrInvalidPath)
			return
		}
		var input contract.FileArchiveJobRequest
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, id, http.StatusBadRequest, "invalid_json", "文件请求无效", "")
			return
		}
		if input.Operation == "create" && input.Input != nil && input.ID == "" {
			job, err := s.files.StartArchiveJob(*input.Input)
			if err != nil {
				fail(err)
				return
			}
			writeJSON(w, http.StatusAccepted, job)
			return
		}
		if (input.Operation == "cancel" || input.Operation == "clear") && input.Input == nil && input.ID != "" {
			if err := s.files.ChangeArchiveJob(input.ID, input.Operation); err != nil {
				fail(err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		fail(filemanager.ErrAction)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeProblem(w, id, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
	}
}
