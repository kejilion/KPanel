package panel

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const (
	appIconBrowserCache         = "private, max-age=86400, stale-if-error=604800"
	appIconFallbackBrowserCache = "private, max-age=300"
	appIconFallbackPath         = "/app-icons/thirdparty-default.svg"
)

var appIconSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func (s *Server) handleAppIcon(w http.ResponseWriter, r *http.Request) {
	_, _, ok := s.requireSession(w, r)
	if !ok {
		return
	}
	agentPath, _, allowed := allowedAppIconPath(r.URL.Path)
	if !allowed || r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusNotFound, "app_icon_not_found", "Application icon not found", "")
		return
	}
	response, err := s.hostOps.Get(r.Context(), agentPath, "", requestID(r))
	if err != nil || response.StatusCode != http.StatusOK {
		writeAppIconFallback(w, r)
		return
	}
	contentType := strings.TrimSpace(strings.SplitN(response.ContentType, ";", 2)[0])
	if contentType != "image/webp" || len(response.Body) == 0 ||
		len(response.Body) > 128<<10 || !siteIconBodyMatchesContentType(response.Body, contentType) {
		writeAppIconFallback(w, r)
		return
	}

	digest := sha256.Sum256(response.Body)
	etag := `"` + hex.EncodeToString(digest[:]) + `"`
	w.Header().Set("Cache-Control", appIconBrowserCache)
	w.Header().Set("ETag", etag)
	if requestMatchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(response.Body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response.Body)
}

func allowedAppIconPath(publicPath string) (agentPath, slug string, allowed bool) {
	const prefix = "/api/v1/apps/icons/"
	const suffix = ".webp"
	if !strings.HasPrefix(publicPath, prefix) || !strings.HasSuffix(publicPath, suffix) {
		return "", "", false
	}
	slug = strings.TrimSuffix(strings.TrimPrefix(publicPath, prefix), suffix)
	if strings.Contains(slug, "/") || !appIconSlugPattern.MatchString(slug) {
		return "", "", false
	}
	return "/v1/apps/icons/" + slug + ".webp", slug, true
}

func writeAppIconFallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", appIconFallbackBrowserCache)
	http.Redirect(w, r, appIconFallbackPath, http.StatusTemporaryRedirect)
}
