package identityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"crypto/rand"
	"encoding/base64"
	"sync"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
)

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
