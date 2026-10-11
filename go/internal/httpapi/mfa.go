package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valirum/quests/go/internal/identity"
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

func (s *Server) registerMFA(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/login/mfa", s.postLoginMFA)
	mux.HandleFunc("GET /api/me/2fa", s.getMyMFA)
	mux.HandleFunc("POST /api/me/2fa/setup", s.setupMyMFA)
	mux.HandleFunc("POST /api/me/2fa/enable", s.enableMyMFA)
	mux.HandleFunc("POST /api/me/2fa/disable", s.disableMyMFA)
	mux.HandleFunc("POST /api/me/2fa/recovery", s.recoveryMyMFA)
}

// postLoginMFA completes a sign-in that the password step started.
func (s *Server) postLoginMFA(w http.ResponseWriter, r *http.Request) {
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
		Token string `json:"mfa_token"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	tk, ok := s.mfa.attempt(body.Token)
	if !ok {
		s.limiter().recordFailure(key)
		writeErr(w, http.StatusUnauthorized, "сессия входа истекла или попытки кончились — введите логин и пароль заново")
		return
	}
	user := strings.ToLower(tk.username)
	if d := s.limiter().userPenalty(user); d > 0 {
		time.Sleep(d)
	}
	if err := s.Auth.VerifySecondFactor(tk.userID, body.Code); err != nil {
		s.limiter().recordFailure(key)
		s.limiter().recordUserFailure(user)
		if errors.Is(err, identity.ErrBadCode) {
			writeErr(w, http.StatusUnauthorized, "Неверный код")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.mfa.spend(body.Token)
	s.limiter().recordSuccess(key)
	if _, err := s.startSession(w, r, tk.userID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "username": tk.username})
}

// mfaSessionUser returns the account behind a browser session: the second factor is managed
// from the web interface, never with a token.
func (s *Server) mfaSessionUser(w http.ResponseWriter, r *http.Request) (identity.User, bool) {
	p, ok := PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 || p.Via != "session" {
		writeErr(w, http.StatusForbidden, "two-factor sign-in is managed from the web interface, not with a token")
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
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return "", false
	}
	return b.Code, true
}

// mfaFail answers a wrong code and slows repeated guessing on the account.
func (s *Server) mfaFail(w http.ResponseWriter, u identity.User, err error) {
	switch {
	case errors.Is(err, identity.ErrBadCode):
		s.limiter().recordUserFailure("mfa:" + strconv.FormatInt(u.ID, 10))
		writeErr(w, http.StatusUnprocessableEntity, "Неверный код")
	case errors.Is(err, identity.ErrMFAUnavailable):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, identity.ErrMFAEnabled), errors.Is(err, identity.ErrMFANotPending):
		writeErr(w, http.StatusConflict, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func (s *Server) stallMFA(u identity.User) {
	if d := s.limiter().userPenalty("mfa:" + strconv.FormatInt(u.ID, 10)); d > 0 {
		time.Sleep(d)
	}
}

func (s *Server) getMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := s.mfaSessionUser(w, r)
	if !ok {
		return
	}
	st, err := s.Auth.MFAStatus(u.ID)
	if errors.Is(err, identity.ErrMFAUnavailable) {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": true, "enabled": st.Enabled, "pending": st.Pending, "recovery_left": st.RecoveryLeft})
}

// setupMyMFA hands out a new secret (and its otpauth link) to be confirmed with a code.
func (s *Server) setupMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := s.mfaSessionUser(w, r)
	if !ok {
		return
	}
	secret, err := s.Auth.BeginMFA(u.ID)
	if err != nil {
		s.mfaFail(w, u, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": secret, "uri": identity.TOTPURI(mfaIssuer, u.Username, secret)})
}

// enableMyMFA confirms the secret with a code, switches the second factor on and returns the
// recovery codes — the only time they are shown.
func (s *Server) enableMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := s.mfaSessionUser(w, r)
	if !ok {
		return
	}
	code, ok := readCode(w, r)
	if !ok {
		return
	}
	s.stallMFA(u)
	codes, err := s.Auth.ConfirmMFA(u.ID, code)
	if err != nil {
		s.mfaFail(w, u, err)
		return
	}
	if c, cerr := r.Cookie(SessionCookie); cerr == nil {
		_ = s.Auth.DeleteOtherSessions(u.ID, c.Value)
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "recovery_codes": codes})
}

// disableMyMFA turns the second factor off; it takes a valid code, so a hijacked session alone cannot.
func (s *Server) disableMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := s.mfaSessionUser(w, r)
	if !ok {
		return
	}
	code, ok := readCode(w, r)
	if !ok {
		return
	}
	s.stallMFA(u)
	if err := s.Auth.VerifySecondFactor(u.ID, code); err != nil {
		s.mfaFail(w, u, err)
		return
	}
	if err := s.Auth.DisableMFA(u.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
}

// recoveryMyMFA replaces the recovery codes (the old ones stop working).
func (s *Server) recoveryMyMFA(w http.ResponseWriter, r *http.Request) {
	u, ok := s.mfaSessionUser(w, r)
	if !ok {
		return
	}
	code, ok := readCode(w, r)
	if !ok {
		return
	}
	s.stallMFA(u)
	if err := s.Auth.VerifySecondFactor(u.ID, code); err != nil {
		s.mfaFail(w, u, err)
		return
	}
	codes, err := s.Auth.RegenerateRecoveryCodes(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}
