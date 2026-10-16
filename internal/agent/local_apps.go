package agent

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/appmarket"
)

// A local app body carries a config of up to 256 KiB and a base64 icon.
const maxLocalAppRequestBytes = 512 << 10

// localAppCollection handles POST /v1/apps/local (create).
func (s *Server) localAppCollection(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_app_request", "应用操作 URL 无效", "")
		return
	}
	var input appmarket.LocalAppWriteInput
	if err := decodeJSONWithLimit(w, r, &input, maxLocalAppRequestBytes); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	if input.ResourceVersion != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "新建本地应用不能携带资源版本", "")
		return
	}
	s.saveLocalApp(w, r, input, http.StatusCreated)
}

// localAppItem handles /v1/apps/local/{token} (GET, PUT, DELETE) and
// /v1/apps/local/{token}/icon (GET).
func (s *Server) localAppItem(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_app_request", "应用操作 URL 无效", "")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/apps/local/"), "/")
	if len(parts) == 2 && parts[1] == "icon" {
		s.requireMethod(w, r, requestID, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			s.localAppIcon(w, parts[0])
		})
		return
	}
	if len(parts) != 1 || parts[0] == "" {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "资源不存在", "")
		return
	}
	token := parts[0]
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	switch r.Method {
	case http.MethodGet:
		document, err := s.appMarket.LocalApp(ctx, token)
		if err != nil {
			s.writeLocalAppError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusOK, document)
	case http.MethodPut:
		var input appmarket.LocalAppWriteInput
		if err := decodeJSONWithLimit(w, r, &input, maxLocalAppRequestBytes); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		if input.Token != "" && input.Token != token {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "名称不能在编辑时修改", "")
			return
		}
		if input.ResourceVersion == "" {
			writeProblem(w, requestID, http.StatusBadRequest, "resource_version_required", "必须提供资源版本", "")
			return
		}
		input.Token = token
		s.saveLocalApp(w, r, input, http.StatusOK)
	case http.MethodDelete:
		var input struct {
			ResourceVersion string `json:"resourceVersion"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		if err := s.appMarket.DeleteLocalApp(ctx, token, input.ResourceVersion); err != nil {
			s.writeLocalAppError(w, requestID, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PUT, DELETE")
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
	}
}

func (s *Server) saveLocalApp(w http.ResponseWriter, r *http.Request, input appmarket.LocalAppWriteInput, status int) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	document, err := s.appMarket.SaveLocalApp(ctx, input)
	if err != nil {
		s.writeLocalAppError(w, requestIDFrom(w), err)
		return
	}
	writeJSON(w, status, document)
}

func (s *Server) localAppIcon(w http.ResponseWriter, token string) {
	icon, err := s.appMarket.LocalAppIcon(token)
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusNotFound, "app_icon_not_found", "应用图标不存在", "")
		return
	}
	w.Header().Set("Content-Type", icon.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(icon.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(icon.Data)
}

func (s *Server) writeLocalAppError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, appmarket.ErrLocalAppInvalid):
		detail := strings.TrimPrefix(err.Error(), appmarket.ErrLocalAppInvalid.Error()+": ")
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "local_app_invalid", "本地应用配置未通过校验", detail)
	case errors.Is(err, appmarket.ErrLocalAppExists):
		writeProblem(w, requestID, http.StatusConflict, "local_app_exists", "同名本地应用已存在", "请换一个名称，或编辑已有应用")
	case errors.Is(err, appmarket.ErrConflict):
		writeProblem(w, requestID, http.StatusConflict, "resource_conflict", "配置已在别处被修改", "请重新加载后再保存")
	case errors.Is(err, appmarket.ErrForbidden):
		detail := strings.TrimPrefix(err.Error(), appmarket.ErrForbidden.Error()+": ")
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "local_app_installed", "应用已安装", detail)
	case errors.Is(err, appmarket.ErrNotFound), errors.Is(err, os.ErrNotExist):
		writeProblem(w, requestID, http.StatusNotFound, "app_not_found", "本地应用不存在", "")
	default:
		s.writeAppError(w, requestID, err)
	}
}
