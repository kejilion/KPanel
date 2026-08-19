package panel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/kejilion/kejilion-panel/internal/version"
)

// The desktop browser feature is served on its own origin, never on the
// panel's. This is the one control that actually contains it.
//
// Scramjet rewrites third-party pages and its Service Worker serves them back
// under the origin the shell runs on, so that page's JavaScript executes with
// that origin's privileges. A rewriting engine is best-effort, not a security
// boundary: any escape hands the page real access to whatever the origin can
// reach. Same-origin with the panel that meant the browsed site could read the
// JS-readable CSRF cookie, let the browser attach the HttpOnly session cookie
// automatically, pass the Origin check (its Origin *is* the panel's), and call
// /api/v1/files or /api/v1/docker as the admin — root on the host. HttpOnly and
// SameSite do not help there; they stop a cookie being read or sent from
// another site, and this is neither.
//
// Splitting the origins moves the boundary into the browser, where it cannot be
// argued with: the panel's cookies are host-only, so they are never sent to the
// browse origin, and a cross-origin call back to the panel is preflighted and
// refused. An escaped page ends up holding a browse-scoped credential that can
// do exactly what it could already do — relay fetches through the egress it was
// already using.
//
// Two halves are required and neither is optional:
//   - browser half: a different origin (see store.AllowedHosts.BrowseOrigin).
//   - server half, here: that origin serves *only* the shell and the browse
//     endpoints. serveBrowseOrigin is an allowlist, and the panel origin stops
//     serving the shell entirely.
const (
	// browseEnterPath receives the handoff ticket minted on the panel origin
	// and exchanges it for this origin's own cookies. It is the only way to
	// obtain a browse credential, and the ticket is single-use.
	browseEnterPath = "/browser-app/enter"

	// browseServiceWorkerPath must stay at the root of the browse origin: the
	// shell registers it with scope "/" so Scramjet can route every proxied
	// path, and a Service Worker's scope cannot exceed its own directory.
	browseServiceWorkerPath = "/scramjet-sw.js"

	// browseHandoffTicketTTL is deliberately far shorter than a session. The
	// ticket only has to survive the redirect from the panel page into the
	// iframe, and it travels in a URL, where it can end up in a Referer or a
	// history entry.
	browseHandoffTicketTTL  = 30 * time.Second
	maxBrowseHandoffTickets = 32

	// maxBrowseOriginSessions bounds the in-memory session table the same way
	// maxFileDownloadTickets bounds its own: paneld runs under a 256MB
	// container limit, so every unbounded map is a liability.
	maxBrowseOriginSessions = 32

	browseSessionCookieName = "kejilion_browse"
	browseCSRFCookieName    = "kejilion_browse_csrf"
)

// browseHandoffTicket is the single-use bearer minted on the panel origin
// (where the admin's session lives) and redeemed on the browse origin (where
// it does not). It carries the user id only: the browse origin must never be
// able to reconstruct a panel session from it.
type browseHandoffTicket struct {
	UserID    string
	ExpiresAt time.Time
}

// browseOriginSession is the browse origin's own credential. It is not an
// auth.Session and deliberately cannot become one — nothing here is accepted
// by requireSession, so a browse cookie replayed against a panel endpoint is
// simply an unauthenticated request.
type browseOriginSession struct {
	UserID    string
	CSRFToken string
	ExpiresAt time.Time
}

// browseOriginHost returns the configured browse origin's host, or "" when the
// operator has not set one. Empty means the feature is off: the shell is never
// served from the panel origin as a fallback, because that fallback is exactly
// the vulnerability this file exists to close.
func (s *Server) browseOriginHost() string {
	if s.store == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(s.store.BrowseOrigin()))
}

func (s *Server) isBrowseOriginRequest(r *http.Request) bool {
	configured := s.browseOriginHost()
	if configured == "" {
		return false
	}
	return secureStringEqual(strings.ToLower(strings.TrimSpace(r.Host)), configured)
}

// panelOriginForFraming returns the panel's own origin for the browse origin's
// frame-ancestors directive. The browse shell is embedded by BrowserView.vue
// from the panel origin, which is now a *different* origin, so the default
// "frame-ancestors 'self'" would block it.
func (s *Server) panelOriginForFraming() string {
	parsed, err := url.Parse(strings.TrimSpace(s.config.PublicURL))
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		strings.ContainsAny(parsed.Host, " ;,'\"") {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

// browseOriginSecurityHeaders replaces the site-wide policy for the browse
// origin. Two directives differ from the panel's, both forced by what the
// shell is:
//   - script-src gains 'wasm-unsafe-eval', because Scramjet's rewriter is a
//     WebAssembly module and strict script-src blocks instantiating it.
//   - frame-ancestors names the panel origin instead of 'self', because the
//     panel embeds this origin in an iframe by design. X-Frame-Options is
//     removed rather than loosened: its ALLOW-FROM form is dead in every
//     current browser, and CSP frame-ancestors takes precedence where both
//     are understood, so leaving a SAMEORIGIN header behind would only risk
//     a browser blocking the legitimate embed.
//
// Nothing else is widened, and no origin outside 'self' is ever granted.
func (s *Server) browseOriginSecurityHeaders(w http.ResponseWriter) {
	ancestors := "'self'"
	if panelOrigin := s.panelOriginForFraming(); panelOrigin != "" {
		ancestors += " " + panelOrigin
	}
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; base-uri 'none'; frame-ancestors "+ancestors+
			"; frame-src 'self' blob:; form-action 'self'; object-src 'none'"+
			"; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'"+
			"; img-src 'self' data:; connect-src 'self'")
	w.Header().Del("X-Frame-Options")
}

// serveBrowseOrigin is the complete surface of the browse origin. It is an
// allowlist and must stay one: anything not named here 404s, so a future panel
// endpoint cannot become reachable from browsed content by accident. In
// particular no /api/v1/files, /api/v1/docker, /api/v1/system or any other
// panel route is routed here, and requireSession is never consulted — the
// panel's cookies do not reach this host at all.
func (s *Server) serveBrowseOrigin(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == browseEnterPath:
		s.handleBrowseEnter(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/browse/fetch":
		s.handleBrowseFetch(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/browse/sessions":
		s.handleBrowseSessionStart(w, r)
	case r.URL.Path == "/api/v1/browse/ws-sessions" ||
		strings.HasPrefix(r.URL.Path, "/api/v1/browse/ws-sessions/"):
		s.handleBrowseWSSession(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/browse/hosts":
		s.handleBrowseHosts(w, r)
	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && isBrowseOriginStaticPath(r.URL.Path):
		s.serveBrowseOriginStatic(w, r)
	default:
		s.writeProblem(w, r, http.StatusNotFound, "not_found", "Not found", "")
	}
}

// isBrowseOriginStaticPath names the shell's own files. It reuses
// browserAppPathPrefixes so the set of "browse shell assets" is defined once
// and the panel origin's 404 rule and this origin's allow rule can never drift
// apart.
func isBrowseOriginStaticPath(path string) bool {
	return isBrowserAppPath(path) || path == browseServiceWorkerPath
}

// serveBrowseOriginStatic resolves one of the shell's own files. It
// deliberately does not reuse serveSPA: that falls back to the panel's
// index.html whenever it cannot resolve a path, which would put the panel's
// SPA shell on the browse origin. Here a miss is a miss.
func (s *Server) serveBrowseOriginStatic(w http.ResponseWriter, r *http.Request) {
	root, err := filepath.Abs(s.config.WebRoot)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	requestPath := filepath.FromSlash(strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), "/"))
	candidate := filepath.Join(root, requestPath)
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}
	regular, err := staticRegularFile(root, candidate)
	if err != nil || !regular {
		http.NotFound(w, r)
		return
	}
	s.serveStaticFile(w, r, root, candidate, requestPath)
}

// handleBrowseHosts is the node picker's data source. The shell cannot call
// /api/v1/cluster/hosts any more — that endpoint lives on the panel origin and
// the browse origin has no panel session — so this returns the same envelope
// shape with only the five fields the picker renders. Keeping it to a
// projection rather than proxying the cluster view means a field added to
// cluster.Host is not automatically exposed to browsed content.
func (s *Server) handleBrowseHosts(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	if _, ok := s.requireBrowseSession(w, r); !ok {
		return
	}
	type browseHostView struct {
		ID                string `json:"id"`
		Name              string `json:"name"`
		IsLocal           bool   `json:"isLocal"`
		BrowseAvailable   bool   `json:"browseAvailable"`
		BrowseWSAvailable bool   `json:"browseWSAvailable"`
	}
	hosts := s.cluster.Hosts(r.Context())
	items := make([]browseHostView, 0, len(hosts.Items))
	for _, host := range hosts.Items {
		if !host.BrowseAvailable {
			continue
		}
		items = append(items, browseHostView{
			ID: host.ID, Name: host.Name, IsLocal: host.IsLocal,
			BrowseAvailable: host.BrowseAvailable, BrowseWSAvailable: host.BrowseWSAvailable,
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

type browseHandoffResponse struct {
	URL string `json:"url"`
}

// handleBrowseHandoff runs on the *panel* origin: it is the bridge across the
// boundary. An authenticated admin trades their panel session for a one-shot
// ticket, and the returned URL is what BrowserView.vue points its iframe at.
//
// When no browse origin is configured this fails closed with a distinct code
// the UI turns into "go set it in Settings", rather than silently falling back
// to the same-origin shell.
func (s *Server) handleBrowseHandoff(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_browse_request", "Invalid browse request", "")
		return
	}
	if !s.checkOrigin(w, r) {
		return
	}
	_, session, ok := s.requireSession(w, r)
	if !ok || !s.checkCSRF(w, r, session) {
		return
	}
	origin := s.browseOriginHost()
	if origin == "" {
		s.writeProblem(w, r, http.StatusPreconditionFailed, "browse_origin_not_configured",
			"浏览器功能尚未配置独立域名", "请在“设置 → 面板访问域名”里填写浏览器专用域名后再打开。")
		return
	}
	ticket, err := s.issueBrowseHandoffTicket(session.User.ID)
	if err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "browse_handoff_unavailable",
			"浏览会话暂时无法建立，请稍后重试", "")
		return
	}
	target := url.URL{
		Scheme:   s.browseOriginScheme(),
		Host:     origin,
		Path:     browseEnterPath,
		RawQuery: url.Values{"t": {ticket}}.Encode(),
	}
	s.writeJSON(w, http.StatusCreated, browseHandoffResponse{URL: target.String()})
}

// browseOriginScheme mirrors the panel's own configured scheme rather than
// hardcoding https. In production publicUrl is https and this returns https,
// which is the only thing that works there anyway — the shell needs a secure
// context to register its Service Worker. But loopback is also a secure
// context, and the documented way to verify this feature without a
// certificate is to reach the panel on http://localhost (see
// docs/deployment.md); hardcoding https would send that setup to a port that
// is not listening.
func (s *Server) browseOriginScheme() string {
	parsed, err := url.Parse(strings.TrimSpace(s.config.PublicURL))
	if err == nil && parsed.Scheme == "http" {
		return "http"
	}
	return "https"
}

// handleBrowseEnter runs on the browse origin and is the only endpoint that
// mints a browse credential. It redeems the single-use ticket, sets this
// origin's own cookies, and redirects to the shell so the ticket does not
// linger in the address bar.
func (s *Server) handleBrowseEnter(w http.ResponseWriter, r *http.Request) {
	ticket := strings.TrimSpace(r.URL.Query().Get("t"))
	userID, ok := s.redeemBrowseHandoffTicket(ticket)
	if !ok {
		s.writeBrowseEnterError(w)
		return
	}
	token, csrfToken, err := s.issueBrowseOriginSession(userID)
	if err != nil {
		s.writeBrowseEnterError(w)
		return
	}
	s.setBrowseCookies(w, token, csrfToken)
	// The version stamp gives each release its own cache key for the shell.
	// app.html is served no-cache, but it is loaded in an iframe, which a
	// parent-page hard reload does not revalidate — a stale copy surviving with
	// no user-reachable way to clear it was a real bug during development. The
	// version comes from the server rather than the frontend build so the
	// redirect target and the running binary can never disagree.
	http.Redirect(w, r, "/browser-app/app.html?v="+url.QueryEscape(version.Version), http.StatusSeeOther)
}

// writeBrowseEnterError answers in HTML, not the usual JSON problem document:
// this endpoint is reached by a top-level iframe navigation, so whatever it
// returns is what the operator actually reads inside the browser window.
func (s *Server) writeBrowseEnterError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8">` +
		`<title>` + html.EscapeString("浏览会话已失效") + `</title>` +
		`<body style="font:14px/1.7 system-ui,sans-serif;padding:24px;color:#3c4043">` +
		`<h1 style="font-size:16px;margin:0 0 8px">` + html.EscapeString("浏览会话已失效") + `</h1>` +
		`<p style="margin:0">` + html.EscapeString("这个入口链接是一次性的，且有效期很短。请关闭此窗口后从桌面重新打开浏览器。") + `</p>` +
		`</body>`))
}

// setBrowseCookies scopes the browse credential to the browse origin. Both
// cookies are SameSite=None because the shell is embedded in an iframe from
// the panel origin, which may be a different site entirely — Lax or Strict
// would simply not be sent there. Secure costs nothing: the feature already
// requires a secure context for the Service Worker to register at all.
//
// The CSRF cookie is readable by script, as double-submit requires; the
// session cookie is not. That split is the same one the panel makes, and it is
// safe for the same reason: a *cross-site* attacker can read neither, and a
// same-origin attacker is inside the blast radius this origin exists to bound.
func (s *Server) setBrowseCookies(w http.ResponseWriter, token, csrfToken string) {
	expiresAt := time.Now().UTC().Add(s.config.SessionTTL)
	maxAge := max(int(s.config.SessionTTL.Seconds()), 1)
	http.SetCookie(w, &http.Cookie{
		Name: browseSessionCookieName, Value: token, Path: "/",
		MaxAge: maxAge, Expires: expiresAt, HttpOnly: true,
		Secure: true, SameSite: http.SameSiteNoneMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: browseCSRFCookieName, Value: csrfToken, Path: "/",
		MaxAge: maxAge, Expires: expiresAt, HttpOnly: false,
		Secure: true, SameSite: http.SameSiteNoneMode,
	})
}

// requireBrowseSession authenticates a request on the browse origin. It is the
// browse endpoints' *only* accepted credential; s.requireSession is never
// called here, which is what stops a leaked panel cookie from being useful and
// — more importantly — stops this credential from being useful anywhere else.
func (s *Server) requireBrowseSession(w http.ResponseWriter, r *http.Request) (browseOriginSession, bool) {
	cookie, err := r.Cookie(browseSessionCookieName)
	if err != nil || cookie.Value == "" {
		s.writeProblem(w, r, http.StatusUnauthorized, "browse_authentication_required", "浏览会话未建立", "")
		return browseOriginSession{}, false
	}
	session, ok := s.lookupBrowseOriginSession(cookie.Value)
	if !ok {
		s.writeProblem(w, r, http.StatusUnauthorized, "browse_session_expired", "浏览会话已过期", "")
		return browseOriginSession{}, false
	}
	return session, true
}

// checkBrowseCSRF is double-submit against the browse origin's own token. It
// does not defend against browsed content — that content is same-origin here
// by design and would pass any check — it defends against a genuinely
// different site driving the egress through the admin's browser, which
// SameSite=None otherwise permits.
func (s *Server) checkBrowseCSRF(w http.ResponseWriter, r *http.Request, session browseOriginSession) bool {
	headerToken := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	cookie, err := r.Cookie(browseCSRFCookieName)
	cookieToken := ""
	if err == nil {
		cookieToken = cookie.Value
	}
	if headerToken == "" || !secureStringEqual(headerToken, cookieToken) ||
		!secureStringEqual(headerToken, session.CSRFToken) {
		s.writeProblem(w, r, http.StatusForbidden, "csrf_validation_failed", "CSRF validation failed", "")
		return false
	}
	return true
}

// checkBrowseOrigin validates the Origin header on the browse origin's own
// mutating endpoints. Requests here must come from the shell itself.
func (s *Server) checkBrowseOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
	if origin == "" {
		// Same-origin GETs legitimately omit Origin; anything cross-site that
		// reaches a mutating handler carries either an Origin or a
		// Sec-Fetch-Site that is not same-origin.
		fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
		if fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
			s.writeProblem(w, r, http.StatusForbidden, "origin_validation_failed", "Origin validation failed", "")
			return false
		}
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" ||
		!secureStringEqual(strings.ToLower(parsed.Host), s.browseOriginHost()) {
		s.writeProblem(w, r, http.StatusForbidden, "origin_validation_failed", "Origin validation failed", "")
		return false
	}
	return true
}

func (s *Server) issueBrowseHandoffTicket(userID string) (string, error) {
	now := time.Now().UTC()
	s.browseOriginMu.Lock()
	defer s.browseOriginMu.Unlock()
	if s.browseHandoffTickets == nil {
		s.browseHandoffTickets = make(map[[32]byte]browseHandoffTicket)
	}
	for key, ticket := range s.browseHandoffTickets {
		if !ticket.ExpiresAt.After(now) {
			delete(s.browseHandoffTickets, key)
		}
	}
	if len(s.browseHandoffTickets) >= maxBrowseHandoffTickets {
		return "", errors.New("browse handoff ticket limit reached")
	}
	token, key, err := newBrowseToken()
	if err != nil {
		return "", err
	}
	s.browseHandoffTickets[key] = browseHandoffTicket{UserID: userID, ExpiresAt: now.Add(browseHandoffTicketTTL)}
	return token, nil
}

// redeemBrowseHandoffTicket consumes the ticket whether or not it was still
// valid: a ticket is single-use, so even a replay of an expired one must not
// leave anything behind to retry against.
func (s *Server) redeemBrowseHandoffTicket(token string) (string, bool) {
	key, ok := browseTokenKey(token)
	if !ok {
		return "", false
	}
	now := time.Now().UTC()
	s.browseOriginMu.Lock()
	defer s.browseOriginMu.Unlock()
	ticket, exists := s.browseHandoffTickets[key]
	delete(s.browseHandoffTickets, key)
	if !exists || !ticket.ExpiresAt.After(now) {
		return "", false
	}
	return ticket.UserID, true
}

func (s *Server) issueBrowseOriginSession(userID string) (string, string, error) {
	now := time.Now().UTC()
	s.browseOriginMu.Lock()
	defer s.browseOriginMu.Unlock()
	if s.browseOriginSessions == nil {
		s.browseOriginSessions = make(map[[32]byte]browseOriginSession)
	}
	for key, session := range s.browseOriginSessions {
		if !session.ExpiresAt.After(now) {
			delete(s.browseOriginSessions, key)
		}
	}
	if len(s.browseOriginSessions) >= maxBrowseOriginSessions {
		return "", "", errors.New("browse session limit reached")
	}
	token, key, err := newBrowseToken()
	if err != nil {
		return "", "", err
	}
	csrfToken, _, err := newBrowseToken()
	if err != nil {
		return "", "", err
	}
	s.browseOriginSessions[key] = browseOriginSession{
		UserID: userID, CSRFToken: csrfToken, ExpiresAt: now.Add(s.config.SessionTTL),
	}
	return token, csrfToken, nil
}

func (s *Server) lookupBrowseOriginSession(token string) (browseOriginSession, bool) {
	key, ok := browseTokenKey(token)
	if !ok {
		return browseOriginSession{}, false
	}
	now := time.Now().UTC()
	s.browseOriginMu.Lock()
	defer s.browseOriginMu.Unlock()
	session, exists := s.browseOriginSessions[key]
	if !exists || !session.ExpiresAt.After(now) {
		delete(s.browseOriginSessions, key)
		return browseOriginSession{}, false
	}
	return session, true
}

// revokeBrowseOriginSessionsForUser is called when a panel session ends. The
// browse credential is independent by design, which would otherwise let it
// outlive the logout that was supposed to end the admin's access.
func (s *Server) revokeBrowseOriginSessionsForUser(userID string) {
	if userID == "" {
		return
	}
	s.browseOriginMu.Lock()
	defer s.browseOriginMu.Unlock()
	for key, session := range s.browseOriginSessions {
		if session.UserID == userID {
			delete(s.browseOriginSessions, key)
		}
	}
	for key, ticket := range s.browseHandoffTickets {
		if ticket.UserID == userID {
			delete(s.browseHandoffTickets, key)
		}
	}
}

// newBrowseToken returns a 256-bit token and the sha256 of its raw bytes. Only
// the digest is stored, so a memory disclosure of the session table does not
// yield usable cookies — the same shape fileDownloadTicket uses.
func newBrowseToken() (string, [32]byte, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", [32]byte{}, err
	}
	return base64.RawURLEncoding.EncodeToString(value), sha256.Sum256(value), nil
}

func browseTokenKey(token string) ([32]byte, bool) {
	if len(token) != 43 {
		return [32]byte{}, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return [32]byte{}, false
	}
	return sha256.Sum256(decoded), true
}
