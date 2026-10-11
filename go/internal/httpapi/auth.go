package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/platform/tokens"
)

// SessionCookie holds the opaque session secret for the SPA.
const SessionCookie = "quests_session"

type ctxKey string

const principalKey ctxKey = "quests.principal"

// PrincipalFrom returns the authenticated caller, if any.
func PrincipalFrom(ctx context.Context) (identity.Principal, bool) {
	p, ok := ctx.Value(principalKey).(identity.Principal)
	return p, ok
}

// publicPaths never require authentication: the login endpoint itself, the
// auth-state probe the SPA uses to decide whether to show the login screen,
// and a bare liveness ping for uptime monitors.
func isPublicAPI(path string) bool {
	// Invitation links are opened before any account exists; the handlers check what they need.
	if strings.HasPrefix(path, "/api/invites/") {
		return true
	}
	switch path {
	case "/api/auth/login", "/api/auth/login/mfa", "/api/auth/state", "/api/ping":
		return true
	}
	return false
}

// isSPAAsset reports whether the path serves the shell that renders the login
// screen. Serving it unauthenticated leaks no data — the SPA fetches everything
// through /api/, which stays gated.
func isSPAAsset(path string) bool {
	return !strings.HasPrefix(path, "/api/") && path != "/ws"
}

// requireAuth gates every request. Credentials come either from the session
// cookie (browser) or an Authorization: Bearer token (CLI, overlay, bot, MCP).
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.AuthRequired {
			next.ServeHTTP(w, r)
			return
		}
		// Runner endpoints check their own runner token (they are not a user session).
		if isPublicAPI(r.URL.Path) || isSPAAsset(r.URL.Path) || strings.HasPrefix(r.URL.Path, "/api/runner/") {
			next.ServeHTTP(w, r)
			return
		}
		p, ok := s.principal(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		// Cookie auth is ambient, so a cross-site form/fetch could ride it.
		// SameSite=Lax already blocks that for top-level posts; this rejects
		// the rest by requiring a same-origin (or allow-listed) Origin.
		if p.Via == "session" && !s.originAllowed(r) {
			writeErr(w, http.StatusForbidden, "cross-origin request rejected")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

func (s *Server) principal(r *http.Request) (identity.Principal, bool) {
	if secret := bearerToken(r); secret != "" {
		// The in-process token the LLM assistant uses to call our own API over
		// loopback. Never persisted, never handed out.
		if s.InternalToken != "" && subtleEqual(secret, s.InternalToken) {
			return identity.Principal{
				User: identity.User{Username: "internal", IsActive: true},
				Via:  "token", TokenName: "internal",
			}, true
		}
		if u, name, err := s.Auth.LookupAPIToken(secret); err == nil {
			return identity.Principal{User: u, Via: "token", TokenName: name}, true
		}
		return identity.Principal{}, false
	}
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		if u, err := s.Auth.LookupSession(c.Value); err == nil {
			return identity.Principal{User: u, Via: "session"}, true
		}
	}
	return identity.Principal{}, false
}

func bearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if h == "" {
		return ""
	}
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func subtleEqual(a, b string) bool {
	return len(a) == len(b) && tokens.Hash(a) == tokens.Hash(b)
}

func formatRetryAfter(d time.Duration) string {
	secs := int(d.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return strconv.Itoa(secs)
}

// requestIsHTTPS reports whether this specific request arrived over TLS,
// either directly or via a reverse proxy (Caddy on the edge gateway) that
// sets X-Forwarded-Proto. s.SecureCookies alone can't express "HTTPS via the
// gateway, plain HTTP on the LAN" — it's all-or-nothing for the whole
// process, which would either break LAN logins or ship cookies without
// Secure over the public gateway. Trusting X-Forwarded-Proto here is safe
// under this deployment: the only route in is direct LAN or Tailscale-only
// gateway traffic, nothing untrusted can reach this port to spoof the header.
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

// originAllowed accepts requests with no Origin (same-origin navigations and
// non-browser clients), a same-host Origin, or an explicitly configured one.
func (s *Server) originAllowed(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	for _, o := range s.CORS {
		if o == origin {
			return true
		}
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}

// --- endpoints ---

func (s *Server) registerAuth(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", s.postLogin)
	mux.HandleFunc("POST /api/auth/logout", s.postLogout)
	mux.HandleFunc("GET /api/auth/state", s.getAuthState)
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

func (s *Server) limiter() *loginLimiter {
	s.loginLimiterOnce.Do(func() { s.loginLimiter = newLoginLimiter() })
	return s.loginLimiter
}

func (s *Server) postLogin(w http.ResponseWriter, r *http.Request) {
	if !s.AuthRequired {
		writeErr(w, http.StatusBadRequest, "authentication is disabled on this instance")
		return
	}
	if !s.originAllowed(r) {
		writeErr(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	key := loginClientKey(r)
	if ok, retryIn := s.limiter().allow(key); !ok {
		w.Header().Set("Retry-After", formatRetryAfter(retryIn))
		writeErr(w, http.StatusTooManyRequests, "слишком много попыток входа, повторите позже")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	user := strings.ToLower(strings.TrimSpace(body.Username))
	if len(user) > 64 || len(body.Password) > 1024 {
		writeErr(w, http.StatusUnauthorized, "Неверный логин или пароль")
		return
	}
	if d := s.limiter().userPenalty(user); d > 0 {
		time.Sleep(d)
	}
	if !s.limiter().acquire() {
		w.Header().Set("Retry-After", "2")
		writeErr(w, http.StatusTooManyRequests, "сервер занят проверкой паролей, повторите")
		return
	}
	u, err := s.Auth.Authenticate(body.Username, body.Password)
	s.limiter().release()
	if err != nil {
		s.limiter().recordFailure(key)
		s.limiter().recordUserFailure(user)
		if errors.Is(err, identity.ErrBadPassword) {
			writeErr(w, http.StatusUnauthorized, "Неверный логин или пароль")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if on, merr := s.Auth.HasMFA(u.ID); merr != nil {
		writeErr(w, http.StatusInternalServerError, merr.Error())
		return
	} else if on {
		// Password right, second factor still owed: a ticket, not a session. The failure counters
		// stay until the code is accepted, so a guesser gains nothing by stopping here.
		ticket, terr := s.mfa.issue(u.ID, u.Username)
		if terr != nil {
			writeErr(w, http.StatusInternalServerError, terr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": ticket})
		return
	}
	s.limiter().recordSuccess(key)
	if _, err := s.startSession(w, r, u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"username":      u.Username,
	})
}

// startSession logs userID in: creates the session row and sets the cookie. Returns the secret.
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) (string, error) {
	secret, expires, err := s.Auth.CreateSession(userID, r.Header.Get("User-Agent"))
	if err != nil {
		return "", err
	}
	go func() { _ = s.Auth.PurgeExpiredSessions() }()
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    secret,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.SecureCookies || requestIsHTTPS(r),
	})
	return secret, nil
}

func (s *Server) postLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		_ = s.Auth.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.SecureCookies || requestIsHTTPS(r),
	})
	w.WriteHeader(http.StatusNoContent)
}

// getAuthState tells the SPA whether to render the login screen. It is public
// by design and reveals only whether auth is on and who (if anyone) is logged in.
func (s *Server) getAuthState(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"auth_required": s.AuthRequired,
		"authenticated": false,
		"username":      nil,
	}
	if !s.AuthRequired {
		resp["authenticated"] = true
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if p, ok := s.principal(r); ok {
		resp["authenticated"] = true
		resp["username"] = p.User.Username
		// 0 = signed in but in no workspace (the SPA offers to create one).
		resp["workspace_id"] = p.User.WorkspaceID
	}
	writeJSON(w, http.StatusOK, resp)
}
