package panel

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/desktopwallpapers"
)

const desktopWallpapersPath = "/api/v1/desktop/wallpapers"

// The multipart envelope around the metadata and the two images.
const maxDesktopWallpaperUploadBytes = desktopwallpapers.MaxImageBytes + desktopwallpapers.MaxThumbBytes + desktopwallpapers.MaxMetadataBytes + 16<<10

// Enough for the largest upload at roughly 250 kbit/s.
const desktopWallpaperUploadTimeout = 5 * time.Minute

type desktopWallpaperResponse struct {
	desktopwallpapers.Wallpaper
	ImageURL string `json:"imageURL"`
	ThumbURL string `json:"thumbURL"`
}

func desktopWallpaperView(wallpaper desktopwallpapers.Wallpaper) desktopWallpaperResponse {
	base := desktopWallpapersPath + "/" + wallpaper.ID
	return desktopWallpaperResponse{Wallpaper: wallpaper, ImageURL: base + "/image", ThumbURL: base + "/thumb"}
}

func (s *Server) handleDesktopWallpapers(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "desktop_wallpaper_request_invalid", "Desktop wallpaper request is invalid", "")
		return
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, desktopWallpapersPath), "/")
	parts := strings.Split(rest, "/")
	list := rest == ""
	file := len(parts) == 2 && desktopwallpapers.ValidID(parts[0]) && (parts[1] == "image" || parts[1] == "thumb")
	item := len(parts) == 1 && desktopwallpapers.ValidID(parts[0])
	switch {
	case list && (r.Method == http.MethodGet || r.Method == http.MethodPost),
		file && r.Method == http.MethodGet,
		item && r.Method == http.MethodDelete:
	case list || file || item:
		allowed := http.MethodGet
		if list {
			allowed += ", " + http.MethodPost
		} else if item {
			allowed = http.MethodDelete
		}
		w.Header().Set("Allow", allowed)
		s.writeProblem(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed", "")
		return
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
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
	if s.desktopWallpapers == nil {
		s.writeDesktopWallpaperError(w, r, desktopwallpapers.ErrUnavailable)
		return
	}
	switch {
	case list && r.Method == http.MethodGet:
		wallpapers, usage := s.desktopWallpapers.List()
		views := make([]desktopWallpaperResponse, 0, len(wallpapers))
		for _, wallpaper := range wallpapers {
			views = append(views, desktopWallpaperView(wallpaper))
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"wallpapers": views, "usage": usage})
	case list:
		s.uploadDesktopWallpaper(w, r, session.User.ID)
	case file:
		s.serveDesktopWallpaperFile(w, r, parts[0], parts[1])
	default:
		s.deleteDesktopWallpaper(w, r, session.User.ID, parts[0])
	}
}

func (s *Server) serveDesktopWallpaperFile(w http.ResponseWriter, r *http.Request, id, kind string) {
	file, size, contentType, err := s.desktopWallpapers.OpenFile(id, kind)
	if err != nil {
		s.writeDesktopWallpaperError(w, r, err)
		return
	}
	defer file.Close()
	// A wallpaper's images never change after upload: a new picture is a new ID.
	etag := `"` + id + "-" + kind + `"`
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", etag)
	if requestMatchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

func (s *Server) uploadDesktopWallpaper(w http.ResponseWriter, r *http.Request, actorID string) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		s.writeProblem(w, r, http.StatusUnsupportedMediaType, "desktop_wallpaper_type_invalid", "Multipart wallpaper upload required", "")
		return
	}
	if r.ContentLength > maxDesktopWallpaperUploadBytes {
		s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "desktop_wallpaper_too_large", "Desktop wallpaper is too large", "")
		return
	}
	// Decoding a large picture is the costly step; one upload prepares at a time.
	select {
	case s.desktopWallpaperUploads <- struct{}{}:
		defer func() { <-s.desktopWallpaperUploads }()
	default:
		s.writeProblem(w, r, http.StatusTooManyRequests, "desktop_wallpaper_busy", "Another wallpaper is being uploaded", "")
		return
	}
	// A picture of a few megabytes can take longer than the server-wide read timeout
	// on a slow uplink; this authenticated, size-bounded body gets its own deadline.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(desktopWallpaperUploadTimeout))
	_ = controller.SetWriteDeadline(time.Now().Add(desktopWallpaperUploadTimeout + time.Minute))
	r.Body = http.MaxBytesReader(w, r.Body, maxDesktopWallpaperUploadBytes)
	meta, imageData, thumbData, err := readDesktopWallpaperParts(r)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) || errors.Is(err, errDesktopWallpaperPartTooLarge) {
			s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "desktop_wallpaper_too_large", "Desktop wallpaper is too large", "")
			return
		}
		s.writeProblem(w, r, http.StatusBadRequest, "desktop_wallpaper_upload_invalid", "Desktop wallpaper upload is invalid", "")
		return
	}
	change := map[string]any{"imageBytes": len(imageData), "thumbBytes": len(thumbData)}
	prepared, err := desktopwallpapers.Prepare(meta, imageData, thumbData)
	if err != nil {
		s.writeDesktopWallpaperError(w, r, err)
		return
	}
	if err := s.audit(r, actorID, "desktop.wallpaper.upload", "desktop-wallpaper", "new", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	wallpaper, err := s.desktopWallpapers.Add(prepared)
	if err != nil {
		_ = s.audit(r, actorID, "desktop.wallpaper.upload", "desktop-wallpaper", "new", "failure", change)
		s.writeDesktopWallpaperError(w, r, err)
		return
	}
	change["width"], change["height"], change["format"] = wallpaper.Width, wallpaper.Height, wallpaper.Format
	_ = s.audit(r, actorID, "desktop.wallpaper.upload", "desktop-wallpaper", wallpaper.ID, "success", change)
	s.writeJSON(w, http.StatusCreated, desktopWallpaperView(wallpaper))
}

var errDesktopWallpaperPartTooLarge = errors.New("desktop wallpaper part too large")

// readDesktopWallpaperParts accepts exactly one metadata, image and thumb part.
func readDesktopWallpaperParts(r *http.Request) (desktopwallpapers.Metadata, []byte, []byte, error) {
	var meta desktopwallpapers.Metadata
	reader, err := r.MultipartReader()
	if err != nil {
		return meta, nil, nil, err
	}
	limits := map[string]int64{
		"metadata": desktopwallpapers.MaxMetadataBytes,
		"image":    desktopwallpapers.MaxImageBytes,
		"thumb":    desktopwallpapers.MaxThumbBytes,
	}
	parts := map[string][]byte{}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return meta, nil, nil, err
		}
		name := part.FormName()
		limit, known := limits[name]
		if !known || parts[name] != nil {
			_ = part.Close()
			return meta, nil, nil, errors.New("unexpected wallpaper part")
		}
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		_ = part.Close()
		if err != nil {
			return meta, nil, nil, err
		}
		if int64(len(data)) > limit {
			return meta, nil, nil, errDesktopWallpaperPartTooLarge
		}
		parts[name] = data
	}
	if parts["metadata"] == nil || parts["image"] == nil || parts["thumb"] == nil {
		return meta, nil, nil, errors.New("missing wallpaper part")
	}
	decoder := json.NewDecoder(strings.NewReader(string(parts["metadata"])))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&meta); err != nil || decoder.More() {
		return meta, nil, nil, errors.New("invalid wallpaper metadata")
	}
	return meta, parts["image"], parts["thumb"], nil
}

func (s *Server) deleteDesktopWallpaper(w http.ResponseWriter, r *http.Request, actorID, id string) {
	if err := s.audit(r, actorID, "desktop.wallpaper.delete", "desktop-wallpaper", id, "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if err := s.desktopWallpapers.Delete(id); err != nil {
		_ = s.audit(r, actorID, "desktop.wallpaper.delete", "desktop-wallpaper", id, "failure", nil)
		s.writeDesktopWallpaperError(w, r, err)
		return
	}
	_ = s.audit(r, actorID, "desktop.wallpaper.delete", "desktop-wallpaper", id, "success", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeDesktopWallpaperError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *desktopwallpapers.ValidationError
	switch {
	case errors.As(err, &validation):
		s.writeValidationProblem(w, r, validation.Field, validation.Detail)
	case errors.Is(err, desktopwallpapers.ErrInvalid):
		s.writeProblem(w, r, http.StatusUnprocessableEntity, "desktop_wallpaper_image_invalid", "Use a still JPEG or WebP picture", "")
	case errors.Is(err, desktopwallpapers.ErrQuota):
		s.writeProblem(w, r, http.StatusConflict, "desktop_wallpaper_quota_exceeded", "Wallpaper storage is full", "")
	case errors.Is(err, desktopwallpapers.ErrNotFound):
		s.writeProblem(w, r, http.StatusNotFound, "desktop_wallpaper_not_found", "Desktop wallpaper not found", "")
	case errors.Is(err, desktopwallpapers.ErrUnavailable):
		s.writeProblem(w, r, http.StatusServiceUnavailable, "desktop_wallpapers_unavailable", "Desktop wallpapers unavailable", "")
	default:
		s.writeProblem(w, r, http.StatusInternalServerError, "desktop_wallpaper_failed", "Desktop wallpaper operation failed", "")
	}
}
