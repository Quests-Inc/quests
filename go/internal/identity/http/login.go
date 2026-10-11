package identityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/platform/httpx"
)

// --- endpoints ---

func (h *Handler) RegisterAuth(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login", h.postLogin)
	mux.HandleFunc("POST /api/auth/logout", h.postLogout)
	mux.HandleFunc("GET /api/auth/state", h.getAuthState)
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

func (h *Handler) postLogin(w http.ResponseWriter, r *http.Request) {
	if !h.AuthRequired {
		httpx.WriteErr(w, http.StatusBadRequest, "authentication is disabled on this instance")
		return
	}
	if !h.OriginAllowed(r) {
		httpx.WriteErr(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	key := httpx.ClientKey(r)
	if ok, retryIn := h.Limiter.Allow(key); !ok {
		w.Header().Set("Retry-After", httpx.FormatRetryAfter(retryIn))
		httpx.WriteErr(w, http.StatusTooManyRequests, "слишком много попыток входа, повторите позже")
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	user := strings.ToLower(strings.TrimSpace(body.Username))
	if len(user) > 64 || len(body.Password) > 1024 {
		httpx.WriteErr(w, http.StatusUnauthorized, "Неверный логин или пароль")
		return
	}
	if d := h.Limiter.UserPenalty(user); d > 0 {
		time.Sleep(d)
	}
	if !h.Limiter.Acquire() {
		w.Header().Set("Retry-After", "2")
		httpx.WriteErr(w, http.StatusTooManyRequests, "сервер занят проверкой паролей, повторите")
		return
	}
	u, err := h.Auth.Authenticate(body.Username, body.Password)
	h.Limiter.Release()
	if err != nil {
		h.Limiter.RecordFailure(key)
		h.Limiter.RecordUserFailure(user)
		if errors.Is(err, identity.ErrBadPassword) {
			httpx.WriteErr(w, http.StatusUnauthorized, "Неверный логин или пароль")
			return
		}
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if on, merr := h.Auth.HasMFA(u.ID); merr != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, merr.Error())
		return
	} else if on {
		// Password right, second factor still owed: a ticket, not a session. The failure counters
		// stay until the code is accepted, so a guesser gains nothing by stopping here.
		ticket, terr := h.mfa.issue(u.ID, u.Username)
		if terr != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, terr.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": ticket})
		return
	}
	h.Limiter.RecordSuccess(key)
	if _, err := h.startSession(w, r, u.ID); err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"username":      u.Username,
	})
}

// startSession logs userID in: creates the session row and sets the cookie. Returns the secret.
func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, userID int64) (string, error) {
	secret, expires, err := h.Auth.CreateSession(userID, r.Header.Get("User-Agent"))
	if err != nil {
		return "", err
	}
	go func() { _ = h.Auth.PurgeExpiredSessions() }()
	http.SetCookie(w, &http.Cookie{
		Name:     identity.SessionCookie,
		Value:    secret,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.SecureCookies || httpx.IsHTTPS(r),
	})
	return secret, nil
}

func (h *Handler) postLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(identity.SessionCookie); err == nil && c.Value != "" {
		_ = h.Auth.DeleteSession(c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     identity.SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.SecureCookies || httpx.IsHTTPS(r),
	})
	w.WriteHeader(http.StatusNoContent)
}

// getAuthState tells the SPA whether to render the login screen. It is public
// by design and reveals only whether auth is on and who (if anyone) is logged in.
func (h *Handler) getAuthState(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"auth_required": h.AuthRequired,
		"authenticated": false,
		"username":      nil,
	}
	if !h.AuthRequired {
		resp["authenticated"] = true
		httpx.WriteJSON(w, http.StatusOK, resp)
		return
	}
	if p, ok := h.Principal(r); ok {
		resp["authenticated"] = true
		resp["username"] = p.User.Username
		// 0 = signed in but in no workspace (the SPA offers to create one).
		resp["workspace_id"] = p.User.WorkspaceID
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}
