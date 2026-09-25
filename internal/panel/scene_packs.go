package panel

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/scenepacks"
)

const scenePacksPath = "/api/v1/desktop/scene-packs"

// A browser can request every file in one pack, plus every catalog image, in parallel.
// Keep that burst bounded while limiting buffered file bodies to four concurrent streams.
const maxScenePackStreamQueue = scenepacks.MaxFiles + 2*scenepacks.MaxPacks

var sceneHostPattern = regexp.MustCompile(`^[a-zA-Z0-9.:[\]-]+$`)

func scenePackFilePath(requestPath string) (id, token, name string, ok bool) {
	if !strings.HasPrefix(requestPath, scenePacksPath+"/") {
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(requestPath, scenePacksPath+"/"), "/", 4)
	if len(parts) != 4 || parts[1] != "files" || !scenepacks.ValidID(parts[0]) || !scenepacks.ValidToken(parts[2]) || !scenepacks.ValidPath(parts[3]) {
		return
	}
	return parts[0], parts[2], parts[3], true
}

func (s *Server) handleScenePacks(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || len(r.URL.RawQuery) > 100 || (r.URL.RawQuery != "" && (!strings.HasSuffix(r.URL.Path, "/thumb") || len(r.URL.Query()) != 1 || len(r.URL.Query()["v"]) != 1)) {
		s.writeProblem(w, r, http.StatusBadRequest, "scene_pack_request_invalid", "Invalid scene pack request", "")
		return
	}
	if s.scenePacks == nil {
		s.scenePackError(w, r, scenepacks.ErrUnavailable)
		return
	}
	if id, token, name, ok := scenePackFilePath(r.URL.Path); ok {
		if r.Method != http.MethodGet {
			s.scenePackMethodError(w, r)
			return
		}
		if !s.scenePackStream(w, r) {
			return
		}
		defer func() { <-s.scenePackStreams }()
		body, contentType, err := s.scenePacks.File(id, token, name)
		if err != nil {
			s.scenePackError(w, r, err)
			return
		}
		if !sceneHostPattern.MatchString(r.Host) {
			s.scenePackError(w, r, scenepacks.ErrInvalid)
			return
		}
		scheme := "http"
		if s.requestUsesHTTPS(r) {
			scheme = "https"
		}
		base := scheme + "://" + r.Host + scenepacks.FilePrefix + id + "/files/" + token + "/"
		// A path-scoped source (not 'self') prevents the opaque frame from sending
		// requests to other Panel endpoints. CORS applies only to capability files.
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; default-src 'none'; script-src "+base+" 'wasm-unsafe-eval'; style-src "+base+" 'unsafe-inline'; img-src "+base+" data: blob:; media-src "+base+" blob:; font-src "+base+" data:; connect-src "+base+"; worker-src "+base+" blob:; frame-src 'none'; frame-ancestors 'self'; form-action 'none'; base-uri 'none'; object-src 'none'")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), fullscreen=(), autoplay=()")
		s.serveScenePackBytes(w, r, body, contentType, name)
		return
	}
	if r.Method != http.MethodGet && !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet && !s.checkCSRF(w, r, session) {
		return
	}
	if r.URL.Path == scenePacksPath && r.Method == http.MethodGet {
		list, err := s.scenePacks.List(r.Context())
		if err != nil {
			s.scenePackError(w, r, err)
			return
		}
		s.writeJSON(w, http.StatusOK, list)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, scenePacksPath+"/"), "/")
	if len(parts) == 2 && scenepacks.ValidID(parts[0]) && (parts[1] == "thumb" || parts[1] == "poster") && r.Method == http.MethodGet {
		if !s.scenePackStream(w, r) {
			return
		}
		defer func() { <-s.scenePackStreams }()
		body, err := s.scenePacks.Image(r.Context(), parts[0], parts[1]+".webp")
		if err != nil {
			s.scenePackError(w, r, err)
			return
		}
		s.serveSceneBytes(w, body, "image/webp")
		return
	}
	var input struct {
		Source                  string `json:"source"`
		ExpectedResourceVersion string `json:"expectedResourceVersion"`
	}
	sourceChange := len(parts) == 1 && parts[0] == "source" && r.Method == http.MethodPut
	install := len(parts) == 2 && scenepacks.ValidID(parts[0]) && parts[1] == "install" && r.Method == http.MethodPost
	remove := len(parts) == 1 && scenepacks.ValidID(parts[0]) && r.Method == http.MethodDelete
	if !sourceChange && !install && !remove {
		s.scenePackMethodError(w, r)
		return
	}
	if err := decodeLimitedJSON(w, r, 1024, &input); err != nil {
		return
	}
	if sourceChange && !scenepacks.ValidSource(input.Source) || !sourceChange && !resourceVersionPattern.MatchString(input.ExpectedResourceVersion) {
		s.scenePackError(w, r, scenepacks.ErrInvalid)
		return
	}
	action := "desktop.scene-pack.install"
	if sourceChange {
		action = "desktop.scene-pack.source"
	} else if remove {
		action = "desktop.scene-pack.delete"
	}
	if err := s.audit(r, session.User.ID, action, "desktop-scene-pack", parts[0], "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	var result any
	var err error
	if sourceChange {
		err = s.scenePacks.SetSource(input.Source)
		result = map[string]string{"source": input.Source}
	} else if install {
		// paneld defaults to a 60-second write deadline. Catalog refresh plus a
		// bounded 3-minute installation must still be able to return its result.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(4 * time.Minute))
		result, err = s.scenePacks.Install(r.Context(), parts[0], input.ExpectedResourceVersion)
	} else {
		err = s.scenePacks.Delete(parts[0], input.ExpectedResourceVersion)
	}
	if err != nil {
		_ = s.audit(r, session.User.ID, action, "desktop-scene-pack", parts[0], "failure", nil)
		s.scenePackError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, action, "desktop-scene-pack", parts[0], "success", nil)
	if remove {
		w.WriteHeader(http.StatusNoContent)
	} else {
		s.writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) scenePackStream(w http.ResponseWriter, r *http.Request) bool {
	if r.Context().Err() != nil {
		return false
	}
	select {
	case s.scenePackStreamQueue <- struct{}{}:
		defer func() { <-s.scenePackStreamQueue }()
	default:
		s.scenePackError(w, r, scenepacks.ErrBusy)
		return false
	}
	select {
	case s.scenePackStreams <- struct{}{}:
		if r.Context().Err() != nil {
			<-s.scenePackStreams
			return false
		}
		return true
	case <-r.Context().Done():
		return false
	}
}
func (s *Server) serveSceneBytes(w http.ResponseWriter, body []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// Scene file URLs contain an unguessable, per-install capability. Allow the
// browser to retain bytes, but require revalidation before every reuse so an
// install replacement or uninstall immediately revokes the old capability.
func (s *Server) serveScenePackBytes(w http.ResponseWriter, r *http.Request, body []byte, contentType, name string) {
	digest := sha256.Sum256(body)
	etag := `W/"` + hex.EncodeToString(digest[:]) + `"`
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, no-cache, must-revalidate")
	w.Header().Set("ETag", etag)
	addVary(w.Header(), "Accept-Encoding")
	if matchesScenePackETag(r.Header.Values("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	payload := body
	compressed := len(body) >= 1024 && r.Header.Get("Range") == "" && acceptsGzip(r.Header.Get("Accept-Encoding")) && compressibleScenePackFile(name)
	if compressed {
		var buffer bytes.Buffer
		writer, err := gzip.NewWriterLevel(&buffer, gzip.BestSpeed)
		if err == nil {
			_, writeErr := writer.Write(body)
			closeErr := writer.Close()
			if writeErr == nil && closeErr == nil && buffer.Len() < len(body) {
				payload = buffer.Bytes()
				w.Header().Set("Content-Encoding", "gzip")
			}
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func matchesScenePackETag(values []string, etag string) bool {
	current := strings.TrimPrefix(etag, "W/")
	for _, value := range values {
		for _, candidate := range strings.Split(value, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" || strings.TrimPrefix(candidate, "W/") == current {
				return true
			}
		}
	}
	return false
}

func compressibleScenePackFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html", ".js", ".mjs", ".css", ".json", ".gltf", ".bin", ".glb", ".wasm", ".txt", ".md":
		return true
	default:
		return false
	}
}

func (s *Server) scenePackMethodError(w http.ResponseWriter, r *http.Request) {
	s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
}
func (s *Server) scenePackError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, title := http.StatusServiceUnavailable, "scene_pack_unavailable", "Scene pack storage unavailable"
	switch {
	case errors.Is(err, scenepacks.ErrNotFound):
		status, code, title = http.StatusNotFound, "scene_pack_not_found", "Scene pack not found"
	case errors.Is(err, scenepacks.ErrInvalid):
		status, code, title = http.StatusBadRequest, "scene_pack_invalid", "Invalid scene pack or request"
	case errors.Is(err, scenepacks.ErrConflict):
		status, code, title = http.StatusConflict, "scene_pack_changed", "Scene pack changed; refresh and retry"
	case errors.Is(err, scenepacks.ErrBusy):
		status, code, title = http.StatusConflict, "scene_pack_busy", "Scene pack operation in progress"
	case errors.Is(err, scenepacks.ErrQuota):
		status, code, title = http.StatusInsufficientStorage, "scene_pack_quota", "Scene pack storage limit reached"
	case errors.Is(err, scenepacks.ErrSource):
		status, code, title = http.StatusBadGateway, "scene_pack_source_unavailable", "Scene download or verification failed"
	}
	s.writeProblem(w, r, status, code, title, "")
}
