package httpapi

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/platform/httpx"
	"github.com/valirum/quests/go/internal/platform/ratelimit"
	"github.com/valirum/quests/go/internal/platform/tokens"
)

// SessionCookie holds the opaque session secret for the SPA.
const SessionCookie = identity.SessionCookie

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
		next.ServeHTTP(w, r.WithContext(identity.WithPrincipal(r.Context(), p)))
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

var bearerToken = httpx.BearerToken

func subtleEqual(a, b string) bool {
	return len(a) == len(b) && tokens.Hash(a) == tokens.Hash(b)
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

func (s *Server) limiter() *ratelimit.Limiter {
	s.loginLimiterOnce.Do(func() { s.loginLimiter = ratelimit.New() })
	return s.loginLimiter
}
