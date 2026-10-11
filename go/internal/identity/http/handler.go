// Package identityhttp is the HTTP face of the identity context (ADR-0001): sign-in and sessions, two-factor,
// API tokens, workspaces, members and invitations. The root (httpapi) owns the authentication middleware and
// hands this package what it needs from outside the context as plain functions.
package identityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"crypto/rand"
	"encoding/base64"
	"sync"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/platform/httpx"
	"github.com/valirum/quests/go/internal/platform/ratelimit"
	"github.com/valirum/quests/go/internal/platform/wsctx"
)

// Handler serves the identity endpoints.
type Handler struct {
	Auth          *identity.Store
	AuthRequired  bool // reported by GET /api/auth/state; login is refused when false
	SecureCookies bool // force the Secure flag on the session cookie
	// Limiter is shared with the rest of the server (login, MFA, invitations and the assistant draw on one budget).
	Limiter *ratelimit.Limiter
	// Principal resolves who is calling, for the few endpoints that are public but behave differently when signed in.
	Principal func(*http.Request) (identity.Principal, bool)
	// OriginAllowed is the server's cross-origin policy for cookie-authenticated requests.
	OriginAllowed func(*http.Request) bool
	// Provision creates a workspace and, when copyFrom > 0, seeds it from that one: identity makes the workspace,
	// the data that fills it belongs to other contexts, so the root composes the steps.
	Provision func(ctx context.Context, userID int64, name string, copyFrom int64) (identity.Workspace, error)
	// Usage reports the attachment bytes the current workspace keeps and its quota (ok=false: unknown).
	Usage func(ctx context.Context) (used, quota int64, ok bool)

	mfa mfaTickets
}

// Register mounts every identity route.
func (h *Handler) Register(mux *http.ServeMux) {
	h.RegisterAuth(mux)
	h.RegisterMFA(mux)
	h.RegisterTokens(mux)
	h.RegisterWorkspaces(mux)
}

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

// Two-factor sign-in (quest=359). Password first; an account with a second factor gets a
// short-lived, single-use, attempt-limited ticket instead of a session, and trades it plus a
// code for the session.

const (
	mfaTicketTTL      = 5 * time.Minute
	mfaTicketAttempts = 5
	mfaIssuer         = "Quests"
)

type mfaTicket struct {
	userID   int64
	username string
	expires  time.Time
	attempts int
}

type mfaTickets struct {
	mu sync.Mutex
	m  map[string]*mfaTicket
}

func (t *mfaTickets) issue(userID int64, username string) (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]*mfaTicket{}
	}
	now := time.Now()
	for k, v := range t.m { // drop the expired ones; the map stays small
		if now.After(v.expires) {
			delete(t.m, k)
		}
	}
	if len(t.m) > 1000 {
		t.m = map[string]*mfaTicket{}
	}
	t.m[id] = &mfaTicket{userID: userID, username: username, expires: now.Add(mfaTicketTTL)}
	return id, nil
}

// attempt counts one try on the ticket; ok=false when it is unknown, expired or used up.
func (t *mfaTickets) attempt(id string) (mfaTicket, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v, ok := t.m[id]
	if !ok || time.Now().After(v.expires) {
		delete(t.m, id)
		return mfaTicket{}, false
	}
	v.attempts++
	if v.attempts > mfaTicketAttempts {
		delete(t.m, id)
		return mfaTicket{}, false
	}
	return *v, true
}

func (t *mfaTickets) spend(id string) {
	t.mu.Lock()
	delete(t.m, id)
	t.mu.Unlock()
}

func (h *Handler) RegisterMFA(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login/mfa", h.postLoginMFA)
	mux.HandleFunc("GET /api/me/2fa", h.getMyMFA)
	mux.HandleFunc("POST /api/me/2fa/setup", h.setupMyMFA)
	mux.HandleFunc("POST /api/me/2fa/enable", h.enableMyMFA)
	mux.HandleFunc("POST /api/me/2fa/disable", h.disableMyMFA)
	mux.HandleFunc("POST /api/me/2fa/recovery", h.recoveryMyMFA)
}

// postLoginMFA completes a sign-in that the password step started.
func (h *Handler) postLoginMFA(w http.ResponseWriter, r *http.Request) {
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
		Token string `json:"mfa_token"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	tk, ok := h.mfa.attempt(body.Token)
	if !ok {
		h.Limiter.RecordFailure(key)
		httpx.WriteErr(w, http.StatusUnauthorized, "сессия входа истекла или попытки кончились — введите логин и пароль заново")
		return
	}
	user := strings.ToLower(tk.username)
	if d := h.Limiter.UserPenalty(user); d > 0 {
		time.Sleep(d)
	}
	if err := h.Auth.VerifySecondFactor(tk.userID, body.Code); err != nil {
		h.Limiter.RecordFailure(key)
		h.Limiter.RecordUserFailure(user)
		if errors.Is(err, identity.ErrBadCode) {
			httpx.WriteErr(w, http.StatusUnauthorized, "Неверный код")
			return
		}
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.mfa.spend(body.Token)
	h.Limiter.RecordSuccess(key)
	if _, err := h.startSession(w, r, tk.userID); err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"authenticated": true, "username": tk.username})
}

// mfaSessionUser returns the account behind a browser session: the second factor is managed
// from the web interface, never with a token.
func (h *Handler) mfaSessionUser(w http.ResponseWriter, r *http.Request) (identity.User, bool) {
	p, ok := identity.PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 || p.Via != "session" {
		httpx.WriteErr(w, http.StatusForbidden, "two-factor sign-in is managed from the web interface, not with a token")
		return identity.User{}, false
	}
	return p.User, true
}

type codeBody struct {
	Code string `json:"code"`
}

func readCode(w http.ResponseWriter, r *http.Request) (string, bool) {
	var b codeBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&b); err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return "", false
	}
	return b.Code, true
}

// mfaFail answers a wrong code and slows repeated guessing on the account.
func (h *Handler) mfaFail(w http.ResponseWriter, u identity.User, err error) {
	switch {
	case errors.Is(err, identity.ErrBadCode):
		h.Limiter.RecordUserFailure("mfa:" + strconv.FormatInt(u.ID, 10))
		httpx.WriteErr(w, http.StatusUnprocessableEntity, "Неверный код")
	case errors.Is(err, identity.ErrMFAUnavailable):
		httpx.WriteErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, identity.ErrMFAEnabled), errors.Is(err, identity.ErrMFANotPending):
		httpx.WriteErr(w, http.StatusConflict, err.Error())
	default:
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *Handler) stallMFA(u identity.User) {
	if d := h.Limiter.UserPenalty("mfa:" + strconv.FormatInt(u.ID, 10)); d > 0 {
		time.Sleep(d)
	}
}

func (h *Handler) getMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := h.mfaSessionUser(w, r)
	if !ok {
		return
	}
	st, err := h.Auth.MFAStatus(u.ID)
	if errors.Is(err, identity.ErrMFAUnavailable) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"available": true, "enabled": st.Enabled, "pending": st.Pending, "recovery_left": st.RecoveryLeft})
}

// setupMyMFA hands out a new secret (and its otpauth link) to be confirmed with a code.
func (h *Handler) setupMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := h.mfaSessionUser(w, r)
	if !ok {
		return
	}
	secret, err := h.Auth.BeginMFA(u.ID)
	if err != nil {
		h.mfaFail(w, u, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"secret": secret, "uri": identity.TOTPURI(mfaIssuer, u.Username, secret)})
}

// enableMyMFA confirms the secret with a code, switches the second factor on and returns the
// recovery codes — the only time they are shown.
func (h *Handler) enableMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := h.mfaSessionUser(w, r)
	if !ok {
		return
	}
	code, ok := readCode(w, r)
	if !ok {
		return
	}
	h.stallMFA(u)
	codes, err := h.Auth.ConfirmMFA(u.ID, code)
	if err != nil {
		h.mfaFail(w, u, err)
		return
	}
	if c, cerr := r.Cookie(identity.SessionCookie); cerr == nil {
		_ = h.Auth.DeleteOtherSessions(u.ID, c.Value)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": true, "recovery_codes": codes})
}

// disableMyMFA turns the second factor off; it takes a valid code, so a hijacked session alone cannot.
func (h *Handler) disableMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := h.mfaSessionUser(w, r)
	if !ok {
		return
	}
	code, ok := readCode(w, r)
	if !ok {
		return
	}
	h.stallMFA(u)
	if err := h.Auth.VerifySecondFactor(u.ID, code); err != nil {
		h.mfaFail(w, u, err)
		return
	}
	if err := h.Auth.DisableMFA(u.ID); err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enabled": false})
}

// recoveryMyMFA replaces the recovery codes (the old ones stop working).
func (h *Handler) recoveryMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := h.mfaSessionUser(w, r)
	if !ok {
		return
	}
	code, ok := readCode(w, r)
	if !ok {
		return
	}
	h.stallMFA(u)
	if err := h.Auth.VerifySecondFactor(u.ID, code); err != nil {
		h.mfaFail(w, u, err)
		return
	}
	codes, err := h.Auth.RegenerateRecoveryCodes(u.ID)
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// Self-service API tokens: the signed-in user mints tokens for their own integrations
// (HUD, MCP, Android, CLI, bot) pinned to the workspace they are working in. Only a
// browser session may do it — a leaked token must not be able to mint more of itself.

func (h *Handler) RegisterTokens(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tokens", h.listMyTokens)
	mux.HandleFunc("POST /api/tokens", h.createMyToken)
	mux.HandleFunc("DELETE /api/tokens/{id}", h.revokeMyToken)
}

func (h *Handler) SessionUser(w http.ResponseWriter, r *http.Request) (userID, workspaceID int64, ok bool) {
	p, has := identity.PrincipalFrom(r.Context())
	if !has || p.User.ID == 0 {
		httpx.WriteErr(w, http.StatusBadRequest, "tokens need an account")
		return 0, 0, false
	}
	if p.Via != "session" {
		httpx.WriteErr(w, http.StatusForbidden, "tokens are managed from the web interface, not with a token")
		return 0, 0, false
	}
	return p.User.ID, p.User.WorkspaceID, true
}

func (h *Handler) listMyTokens(w http.ResponseWriter, r *http.Request) {
	uid, ws, ok := h.SessionUser(w, r)
	if !ok {
		return
	}
	tokens, err := h.Auth.ListUserTokens(uid, ws)
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tokens": tokens})
}

func (h *Handler) createMyToken(w http.ResponseWriter, r *http.Request) {
	uid, ws, ok := h.SessionUser(w, r)
	if !ok {
		return
	}
	if ws <= 0 {
		httpx.WriteErr(w, http.StatusConflict, "no workspace to pin the token to")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len([]rune(name)) > 64 {
		httpx.WriteErr(w, http.StatusUnprocessableEntity, "название токена: 1–64 символа")
		return
	}
	if n, err := h.Auth.CountUserTokens(uid); err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	} else if n >= 25 {
		httpx.WriteErr(w, http.StatusConflict, "слишком много токенов — отзовите ненужные")
		return
	}
	secret, err := h.Auth.CreateAPIToken(uid, name, ws)
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The secret is shown exactly once: only its hash is stored.
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"token": secret, "name": name})
}

func (h *Handler) revokeMyToken(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := h.SessionUser(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid token id")
		return
	}
	if err := h.Auth.RevokeUserToken(uid, id); err != nil {
		httpx.WriteErr(w, http.StatusNotFound, "token not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RequireOwner lets only an owner of the caller's current workspace through. Callers
// without an account (auth off, the in-process token) pass: there is nobody to tell apart.
func (h *Handler) RequireOwner(w http.ResponseWriter, r *http.Request, what string) bool {
	p, ok := identity.PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 {
		return true
	}
	role, err := h.Auth.MemberRole(r.Context(), wsctx.From(r.Context()), p.User.ID)
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if role != identity.RoleOwner {
		httpx.WriteErr(w, http.StatusForbidden, "only an owner can "+what)
		return false
	}
	return true
}

func (h *Handler) RegisterWorkspaces(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspaces", h.listWorkspaces)
	mux.HandleFunc("POST /api/workspaces", h.createWorkspace)
	mux.HandleFunc("POST /api/workspaces/{id}/switch", h.switchWorkspace)
	mux.HandleFunc("PATCH /api/workspaces/{id}", h.renameWorkspace)
	mux.HandleFunc("DELETE /api/workspaces/{id}", h.deleteWorkspace)
	mux.HandleFunc("GET /api/workspaces/{id}/members", h.listMembers)
	mux.HandleFunc("DELETE /api/workspaces/{id}/members/{uid}", h.removeMember)
	mux.HandleFunc("POST /api/workspaces/{id}/invites", h.createInvite)
	mux.HandleFunc("GET /api/invites/{token}", h.getInvite)
	mux.HandleFunc("POST /api/invites/{token}/accept", h.acceptInvite)
}

// MemberOf checks that the caller belongs to workspace {id} and returns the id
// and their role. Callers without an account (auth off) are refused: members
// and invitations only make sense with accounts.
func (h *Handler) MemberOf(w http.ResponseWriter, r *http.Request) (int64, string, identity.User, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, ok := identity.PrincipalFrom(r.Context())
	if err != nil || id <= 0 || !ok || p.User.ID == 0 {
		httpx.WriteErr(w, http.StatusBadRequest, "needs an account and a workspace id")
		return 0, "", identity.User{}, false
	}
	role, err := h.Auth.MemberRole(r.Context(), id, p.User.ID)
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return 0, "", identity.User{}, false
	}
	if role == "" {
		// Do not reveal whether the workspace exists.
		httpx.WriteErr(w, http.StatusNotFound, "workspace not found")
		return 0, "", identity.User{}, false
	}
	return id, role, p.User, true
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) {
	id, role, _, ok := h.MemberOf(w, r)
	if !ok {
		return
	}
	members, err := h.Auth.WorkspaceMembers(r.Context(), id)
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"members": members, "role": role})
}

// removeMember lets an owner remove anyone, and anyone leave.
func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	id, role, me, ok := h.MemberOf(w, r)
	if !ok {
		return
	}
	uid, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil || uid <= 0 {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if uid != me.ID && role != identity.RoleOwner {
		httpx.WriteErr(w, http.StatusForbidden, "only an owner can remove other members")
		return
	}
	switch err := h.Auth.RemoveWorkspaceMember(r.Context(), id, uid); {
	case errors.Is(err, identity.ErrNotFound):
		httpx.WriteErr(w, http.StatusNotFound, "not a member")
	case errors.Is(err, identity.ErrLastOwner):
		httpx.WriteErr(w, http.StatusConflict, err.Error())
	case err != nil:
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// createInvite mints a one-time link (owners only). The secret is shown once.
func (h *Handler) createInvite(w http.ResponseWriter, r *http.Request) {
	id, role, me, ok := h.MemberOf(w, r)
	if !ok {
		return
	}
	if role != identity.RoleOwner {
		httpx.WriteErr(w, http.StatusForbidden, "only an owner can invite")
		return
	}
	secret, exp, err := h.Auth.CreateInvite(r.Context(), id, me.ID)
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"token":      secret,
		"path":       "/?invite=" + secret,
		"expires_at": exp.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

func (h *Handler) getInvite(w http.ResponseWriter, r *http.Request) {
	if !h.Limiter.TryInvite(httpx.ClientKey(r)) {
		httpx.WriteErr(w, http.StatusTooManyRequests, "слишком много запросов")
		return
	}
	name, exp, err := h.Auth.InviteInfo(r.Context(), r.PathValue("token"))
	if errors.Is(err, identity.ErrInviteInvalid) {
		httpx.WriteErr(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, loggedIn := h.Principal(r)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"workspace":     name,
		"expires_at":    exp.UTC().Format("2006-01-02T15:04:05Z"),
		"authenticated": loggedIn,
	})
}

// acceptInvite spends an invitation. A logged-in caller joins with their account;
// otherwise username+password create a new account that joins (the only way to
// sign up on this instance besides the admin CLI).
func (h *Handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	if !h.AuthRequired {
		httpx.WriteErr(w, http.StatusBadRequest, "invitations need accounts and workspaces")
		return
	}
	if !h.OriginAllowed(r) {
		httpx.WriteErr(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	if !h.Limiter.TryInvite(httpx.ClientKey(r)) {
		httpx.WriteErr(w, http.StatusTooManyRequests, "слишком много попыток, повторите позже")
		return
	}
	secret := r.PathValue("token")
	if _, _, err := h.Auth.InviteInfo(r.Context(), secret); err != nil {
		httpx.WriteErr(w, http.StatusNotFound, identity.ErrInviteInvalid.Error())
		return
	}

	var user identity.User
	created := false
	if p, ok := h.Principal(r); ok && p.User.ID != 0 {
		user = p.User
	} else {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
			httpx.WriteErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		u, err := h.Auth.CreateUser(body.Username, body.Password)
		switch {
		case errors.Is(err, identity.ErrUserExists):
			httpx.WriteErr(w, http.StatusConflict, "такой логин уже занят — войдите и откройте ссылку снова")
			return
		case errors.Is(err, identity.ErrWeakPassword):
			httpx.WriteErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		case err != nil:
			httpx.WriteErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		user, created = u, true
	}

	wsID, err := h.Auth.ClaimInvite(r.Context(), secret, user.ID)
	if err != nil {
		if created {
			_ = h.Auth.DeleteUser(user.ID) // lost a race for the invitation: undo the signup
		}
		if errors.Is(err, identity.ErrInviteInvalid) {
			httpx.WriteErr(w, http.StatusNotFound, err.Error())
			return
		}
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if created {
		secretSess, err := h.startSession(w, r, user.ID)
		if err != nil {
			httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = h.Auth.SetSessionWorkspace(secretSess, user.ID, wsID)
	} else if c, err := r.Cookie(identity.SessionCookie); err == nil {
		_ = h.Auth.SetSessionWorkspace(c.Value, user.ID, wsID)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"workspace_id": wsID, "username": user.Username, "created": created})
}

func (h *Handler) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	cur := wsctx.From(r.Context())
	if p, ok := identity.PrincipalFrom(r.Context()); cur == 0 && (!ok || p.User.ID == 0) {
		cur = identity.DefaultWorkspaceID
	}
	var list []identity.Workspace
	var err error
	if p, ok := identity.PrincipalFrom(r.Context()); ok && p.User.ID != 0 {
		list, err = h.Auth.UserWorkspaces(r.Context(), p.User.ID)
	} else {
		list, err = h.Auth.AllWorkspaces(r.Context())
	}
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []identity.Workspace{}
	}
	view := WorkspacesView{Current: cur, Workspaces: list}
	if used, quota, ok := h.Usage(r.Context()); ok {
		view.AttachmentsUsed, view.AttachmentsQuota = &used, &quota
	}
	httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *Handler) createWorkspace(w http.ResponseWriter, r *http.Request) {
	p, ok := identity.PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 {
		httpx.WriteErr(w, http.StatusBadRequest, "creating a workspace needs an account")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	ws, err := h.Provision(r.Context(), p.User.ID, body.Name, wsctx.From(r.Context()))
	if errors.Is(err, identity.ErrWorkspaceName) {
		httpx.WriteErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if errors.Is(err, identity.ErrWorkspaceLimit) {
		httpx.WriteErr(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, ws)
}

// switchWorkspace pins the calling login session to another workspace. API
// tokens are pinned when minted, so only browser sessions switch.
func (h *Handler) switchWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid workspace id")
		return
	}
	p, ok := identity.PrincipalFrom(r.Context())
	if !ok || p.Via != "session" {
		httpx.WriteErr(w, http.StatusBadRequest, "only a logged-in browser session can switch workspace")
		return
	}
	c, err := r.Cookie(identity.SessionCookie)
	if err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "no session")
		return
	}
	if err := h.Auth.SetSessionWorkspace(c.Value, p.User.ID, id); err != nil {
		if errors.Is(err, identity.ErrNotMember) {
			httpx.WriteErr(w, http.StatusForbidden, err.Error())
			return
		}
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"current": id})
}

func (h *Handler) renameWorkspace(w http.ResponseWriter, r *http.Request) {
	id, role, _, ok := h.MemberOf(w, r)
	if !ok {
		return
	}
	if role != identity.RoleOwner {
		httpx.WriteErr(w, http.StatusForbidden, "only an owner can rename")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	switch err := h.Auth.RenameWorkspace(r.Context(), id, body.Name); {
	case errors.Is(err, identity.ErrWorkspaceName):
		httpx.WriteErr(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, identity.ErrNotFound):
		httpx.WriteErr(w, http.StatusNotFound, "workspace not found")
	case err != nil:
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "name": strings.TrimSpace(body.Name)})
	}
}

// deleteWorkspace hides the workspace and schedules its purge (owners only, name must be typed).
func (h *Handler) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id, role, _, ok := h.MemberOf(w, r)
	if !ok {
		return
	}
	if role != identity.RoleOwner {
		httpx.WriteErr(w, http.StatusForbidden, "only an owner can delete a workspace")
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		httpx.WriteErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name, err := h.Auth.WorkspaceName(r.Context(), id)
	if err != nil {
		httpx.WriteErr(w, http.StatusNotFound, "workspace not found")
		return
	}
	if strings.TrimSpace(body.Confirm) != name {
		httpx.WriteErr(w, http.StatusUnprocessableEntity, "введите название пространства для подтверждения")
		return
	}
	purgeAt, err := h.Auth.DeleteWorkspace(r.Context(), id)
	switch {
	case errors.Is(err, identity.ErrMainWorkspace):
		httpx.WriteErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, identity.ErrNotFound):
		httpx.WriteErr(w, http.StatusNotFound, "workspace not found")
	case err != nil:
		httpx.WriteErr(w, http.StatusInternalServerError, err.Error())
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"purge_at": purgeAt.UTC().Format("2006-01-02T15:04:05Z")})
	}
}

// attachmentUsage reports how much of the attachment quota the current workspace uses.
