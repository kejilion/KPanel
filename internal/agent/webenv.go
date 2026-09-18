package agent

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

func (s *Server) webEnvironmentSummary(w http.ResponseWriter, r *http.Request) {
	if s.webEnvironment == nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "web_environment_unavailable", "LDNMP 环境服务不可用", "")
		return
	}
	summary, err := s.webEnvironment.Summary(r.Context())
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "web_environment_unavailable", "无法读取 LDNMP 环境", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) webEnvironmentCatalog(w http.ResponseWriter, r *http.Request) {
	if s.webEnvironment == nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "web_environment_unavailable", "LDNMP 环境服务不可用", "")
		return
	}
	catalog, err := s.webEnvironment.Catalog(r.Context())
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "web_environment_unavailable", "无法读取 LDNMP 功能目录", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) webEnvironmentBackups(w http.ResponseWriter, _ *http.Request) {
	if s.webEnvironment == nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "web_environment_unavailable", "LDNMP 环境服务不可用", "")
		return
	}
	backups, err := s.webEnvironment.Backups()
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "web_environment_backup_unavailable", "无法读取 LDNMP 备份", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, contract.PageResult[webenv.Backup]{Items: backups})
}

func (s *Server) webEnvironmentJobs(w http.ResponseWriter, r *http.Request, requestID string) {
	if s.webEnvironment == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "web_environment_unavailable", "LDNMP 环境服务不可用", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, contract.PageResult[webenv.Job]{Items: s.webEnvironment.Jobs()})
	case http.MethodPost:
		var input webenv.ActionRequest
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		job, err := s.webEnvironment.Start(r.Context(), input)
		if err != nil {
			status, code, title := http.StatusServiceUnavailable, "web_environment_unavailable", "LDNMP 环境任务不可用"
			if errors.Is(err, webenv.ErrInvalid) {
				status, code, title = http.StatusUnprocessableEntity, "web_environment_validation_failed", "LDNMP 环境任务参数无效"
			} else if errors.Is(err, webenv.ErrConflict) {
				status, code, title = http.StatusConflict, "resource_conflict", "已有 LDNMP 环境任务正在执行"
			}
			writeProblem(w, requestID, status, code, title, safeDetail(err))
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
	}
}

func (s *Server) webEnvironmentJob(w http.ResponseWriter, r *http.Request, requestID string) {
	suffix := strings.TrimPrefix(r.URL.Path, "/v1/web-environment/jobs/")
	parts := strings.Split(suffix, "/")
	if len(parts) == 0 || len(parts) > 2 {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "环境任务不存在", "")
		return
	}
	if len(parts) == 2 && parts[1] == "terminal" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		query, err := parseTerminalReadQuery(r.URL.Query(), false)
		if err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_terminal_offset", "终端偏移量无效", "")
			return
		}
		chunk, err := waitForTerminalChunk(
			r.Context(),
			query.Wait,
			func() (webenv.TerminalChunk, error) {
				return s.webEnvironment.Terminal(parts[0], query.Offset)
			},
			func(chunk webenv.TerminalChunk) bool {
				return chunk.DataBase64 != "" || chunk.Finished ||
					(query.HasInputState && chunk.InputOpen != query.InputOpen)
			},
		)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			writeProblem(w, requestID, http.StatusNotFound, "environment_terminal_not_found", "环境终端不存在", "")
			return
		}
		writeJSON(w, http.StatusOK, chunk)
		return
	}
	if len(parts) == 2 && parts[1] == "input" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		var input struct {
			Data string `json:"data"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		if err := s.webEnvironment.WriteInput(parts[0], input.Data); err != nil {
			status, code, title := http.StatusConflict, "environment_terminal_closed", "环境终端当前不可输入"
			if errors.Is(err, webenv.ErrInvalid) {
				status, code, title = http.StatusUnprocessableEntity, "invalid_terminal_input", "终端输入无效"
			}
			writeProblem(w, requestID, status, code, title, "")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) != 1 || r.Method != http.MethodGet {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "环境任务不存在", "")
		return
	}
	job, err := s.webEnvironment.Job(parts[0])
	if err != nil {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "环境任务不存在", "")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) webEnvironmentBackupDownload(w http.ResponseWriter, r *http.Request) {
	if s.webEnvironment == nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "web_environment_unavailable", "LDNMP 环境服务不可用", "")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/web-environment/backups/")
	file, info, err := s.webEnvironment.OpenBackup(id)
	if errors.Is(err, webenv.ErrNotFound) {
		writeProblem(w, requestIDFrom(w), http.StatusNotFound, "backup_not_found", "备份不存在", "")
		return
	}
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "backup_unavailable", "备份无法读取", "")
		return
	}
	defer file.Close()
	name := filepath.Base(file.Name())
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Type", "application/gzip")
	http.ServeContent(w, r, name, info.ModTime(), file)
}
