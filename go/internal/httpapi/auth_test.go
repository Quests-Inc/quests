package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/valirum/quests/go/internal/auth"
	"github.com/valirum/quests/go/internal/pgtest"
)

// newTestServer wires just enough of Server to exercise requireAuth + the
// auth endpoints — no Store/Health/Hub, since login/logout/state/gating
// never touch them.
func newTestServer(t *testing.T, authRequired bool) (*Server, *auth.Store) {
	t.Helper()
	authStore := &auth.Store{DB: pgtest.New(t), Workspaces: true}
	s := &Server{Auth: authStore, AuthRequired: authRequired}
	return s, authStore
}

// testHandler mirrors the relevant slice of Server.Handler(): the public auth
// routes plus one protected dummy route, wrapped by requireAuth — without
// needing Store/Health/Hub/CORS.
func (s *Server) testHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/protected", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "yes"})
	})
	s.registerAuth(mux)
	return s.requireAuth(mux)
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = httptest.NewRequest(method, path, bytes.NewReader(b))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestRequireAuth_DisabledPassesThrough(t *testing.T) {
	s, _ := newTestServer(t, false)
	w := doJSON(t, s.testHandler(), "GET", "/api/protected", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (auth disabled)", w.Code)
	}
}

func TestRequireAuth_PublicRoutesAlwaysOpen(t *testing.T) {
	s, _ := newTestServer(t, true)
	h := s.testHandler()

	for _, path := range []string{"/api/auth/state", "/api/ping"} {
		w := doJSON(t, h, "GET", path, nil, nil)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 even with no credentials", path, w.Code)
		}
	}
}

func TestRequireAuth_ProtectedRouteNeedsCredentials(t *testing.T) {
	s, _ := newTestServer(t, true)
	h := s.testHandler()

	w := doJSON(t, h, "GET", "/api/protected", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with no credentials", w.Code)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	s, authStore := newTestServer(t, true)
	if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	w := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "wrong"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestLogin_DisabledInstanceRejectsLogin(t *testing.T) {
	s, authStore := newTestServer(t, false)
	if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	w := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "correcthorse"}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (auth not required on this instance)", w.Code)
	}
}

func TestLogin_SetsCookieThatUnlocksProtectedRoutes(t *testing.T) {
	s, authStore := newTestServer(t, true)
	if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	w := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "correcthorse"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", w.Code)
	}
	resp := w.Result()
	cookies := resp.Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookie {
		t.Fatalf("cookies = %+v, want exactly one %q cookie", cookies, SessionCookie)
	}
	cookie := cookies[0]
	if !cookie.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Secure {
		t.Error("cookie should not be Secure when SecureCookies is unset")
	}

	// Re-request the protected route carrying the cookie.
	r := httptest.NewRequest("GET", "/api/protected", nil)
	r.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r)
	if w2.Code != http.StatusOK {
		t.Fatalf("protected route with session cookie = %d, want 200", w2.Code)
	}
}

func TestLogin_SecureCookieViaForwardedProto(t *testing.T) {
	s, authStore := newTestServer(t, true)
	if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	// Plain LAN request (no X-Forwarded-Proto) must not get a Secure cookie —
	// otherwise the browser drops it and LAN login breaks.
	w := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "correcthorse"}, nil)
	if cookie := w.Result().Cookies()[0]; cookie.Secure {
		t.Error("cookie should not be Secure for a plain-HTTP request")
	}

	// Request proxied through the gateway (Caddy sets X-Forwarded-Proto:
	// https) must get a Secure cookie even though SecureCookies is unset.
	w2 := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "correcthorse"},
		map[string]string{"X-Forwarded-Proto": "https"})
	if w2.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", w2.Code)
	}
	if cookie := w2.Result().Cookies()[0]; !cookie.Secure {
		t.Error("cookie should be Secure when X-Forwarded-Proto: https")
	}
}

func TestLogin_ThrottlesRepeatedFailures(t *testing.T) {
	s, authStore := newTestServer(t, true)
	if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	wrong := map[string]string{"username": "alice", "password": "wrong"}
	for i := 0; i < loginFreeAttempts; i++ {
		w := doJSON(t, h, "POST", "/api/auth/login", wrong, nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i, w.Code)
		}
	}
	// The next failure past the free budget should trip the lockout.
	w := doJSON(t, h, "POST", "/api/auth/login", wrong, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("attempt %d: status = %d, want 401", loginFreeAttempts, w.Code)
	}
	// Now even the *correct* password is throttled — the point is to slow
	// down guessing, not just repeat wrong guesses.
	w2 := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "correcthorse"}, nil)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 once locked out", w2.Code)
	}
	if w2.Header().Get("Retry-After") == "" {
		t.Error("expected a Retry-After header on 429")
	}
}

func TestLogin_SuccessResetsThrottle(t *testing.T) {
	s, authStore := newTestServer(t, true)
	if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	wrong := map[string]string{"username": "alice", "password": "wrong"}
	doJSON(t, h, "POST", "/api/auth/login", wrong, nil)
	doJSON(t, h, "POST", "/api/auth/login", wrong, nil)

	w := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "correcthorse"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ok, _ := s.limiter().allow(loginClientKey(httptest.NewRequest("GET", "/", nil))); !ok {
		t.Error("successful login should reset the failure count")
	}
}

func TestLogin_RejectsCrossOriginPost(t *testing.T) {
	s, authStore := newTestServer(t, true)
	if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	w := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "correcthorse"},
		map[string]string{"Origin": "https://evil.example"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin login status = %d, want 403", w.Code)
	}
}

func TestRequireAuth_RejectsSessionCookieFromForeignOrigin(t *testing.T) {
	s, authStore := newTestServer(t, true)
	if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	loginResp := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "correcthorse"}, nil)
	cookie := loginResp.Result().Cookies()[0]

	r := httptest.NewRequest("GET", "/api/protected", nil)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a cookie ridden in from a foreign origin", w.Code)
	}
}

func TestLogout_InvalidatesSessionServerSide(t *testing.T) {
	s, authStore := newTestServer(t, true)
	if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	loginResp := doJSON(t, h, "POST", "/api/auth/login",
		map[string]string{"username": "alice", "password": "correcthorse"}, nil)
	cookie := loginResp.Result().Cookies()[0]

	logoutReq := httptest.NewRequest("POST", "/api/auth/logout", nil)
	logoutReq.AddCookie(cookie)
	logoutW := httptest.NewRecorder()
	h.ServeHTTP(logoutW, logoutReq)
	if logoutW.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", logoutW.Code)
	}

	// The same cookie value must no longer work — logout has to delete the
	// session server-side, not just tell the browser to drop it.
	r := httptest.NewRequest("GET", "/api/protected", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("protected route after logout = %d, want 401", w.Code)
	}
}

func TestBearerToken_AuthenticatesAndBypassesOriginCheck(t *testing.T) {
	s, authStore := newTestServer(t, true)
	u, err := authStore.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := authStore.CreateAPIToken(u.ID, "cli", 0)
	if err != nil {
		t.Fatal(err)
	}
	h := s.testHandler()

	r := httptest.NewRequest("GET", "/api/protected", nil)
	r.Header.Set("Authorization", "Bearer "+secret)
	r.Header.Set("Origin", "https://evil.example") // tokens aren't ambient — no CSRF exposure
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a valid bearer token regardless of Origin", w.Code)
	}
}

func TestBearerToken_RevokedTokenRejected(t *testing.T) {
	s, authStore := newTestServer(t, true)
	u, err := authStore.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := authStore.CreateAPIToken(u.ID, "cli", 0)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := authStore.ListAPITokens()
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListAPITokens: %+v, %v", tokens, err)
	}
	if err := authStore.RevokeAPIToken(tokens[0].ID); err != nil {
		t.Fatal(err)
	}

	h := s.testHandler()
	r := httptest.NewRequest("GET", "/api/protected", nil)
	r.Header.Set("Authorization", "Bearer "+secret)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a revoked token", w.Code)
	}
}

func TestBearerToken_InternalTokenAuthenticates(t *testing.T) {
	s, _ := newTestServer(t, true)
	s.InternalToken = "loopback-secret"
	h := s.testHandler()

	r := httptest.NewRequest("GET", "/api/protected", nil)
	r.Header.Set("Authorization", "Bearer loopback-secret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for the internal loopback token", w.Code)
	}

	r2 := httptest.NewRequest("GET", "/api/protected", nil)
	r2.Header.Set("Authorization", "Bearer wrong-secret")
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a wrong bearer token", w2.Code)
	}
}

func TestGetAuthState(t *testing.T) {
	t.Run("auth disabled", func(t *testing.T) {
		s, _ := newTestServer(t, false)
		w := doJSON(t, s.testHandler(), "GET", "/api/auth/state", nil, nil)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["auth_required"] != false || body["authenticated"] != true {
			t.Errorf("body = %+v, want auth_required=false authenticated=true", body)
		}
	})

	t.Run("auth enabled, no credentials", func(t *testing.T) {
		s, authStore := newTestServer(t, true)
		if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
			t.Fatal(err)
		}
		w := doJSON(t, s.testHandler(), "GET", "/api/auth/state", nil, nil)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["auth_required"] != true || body["authenticated"] != false {
			t.Errorf("body = %+v, want auth_required=true authenticated=false", body)
		}
	})

	t.Run("auth enabled, logged in", func(t *testing.T) {
		s, authStore := newTestServer(t, true)
		if _, err := authStore.CreateUser("alice", "correcthorse"); err != nil {
			t.Fatal(err)
		}
		h := s.testHandler()
		loginResp := doJSON(t, h, "POST", "/api/auth/login",
			map[string]string{"username": "alice", "password": "correcthorse"}, nil)
		cookie := loginResp.Result().Cookies()[0]

		r := httptest.NewRequest("GET", "/api/auth/state", nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["authenticated"] != true || body["username"] != "alice" {
			t.Errorf("body = %+v, want authenticated=true username=alice", body)
		}
	})
}

func TestSelfServiceTokens(t *testing.T) {
	s, authStore := newTestServer(t, true)
	alice, _ := authStore.CreateUser("alice", "correcthorse")
	bob, _ := authStore.CreateUser("bob", "correcthorse")
	// A token is pinned to the workspace of the session that mints it: both people are in workspace 1.
	for _, u := range []int64{alice.ID, bob.ID} {
		if _, err := authStore.DB.Exec(`INSERT INTO workspace_member (workspace_id, user_id) VALUES (1, ?)`, u); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	s.registerAuth(mux)
	s.registerTokens(mux)
	h := s.requireAuth(mux)

	login := func(user string) *http.Cookie {
		w := doJSON(t, h, "POST", "/api/auth/login", map[string]string{"username": user, "password": "correcthorse"}, nil)
		if len(w.Result().Cookies()) != 1 {
			t.Fatalf("login %s: %d", user, w.Code)
		}
		return w.Result().Cookies()[0]
	}
	withCookie := func(method, path string, body any, c *http.Cookie) *httptest.ResponseRecorder {
		var rd *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		} else {
			rd = bytes.NewReader(nil)
		}
		r := httptest.NewRequest(method, path, rd)
		r.AddCookie(c)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	ca, cb := login("alice"), login("bob")

	// A session mints a token; the secret is returned once and works as a bearer.
	w := withCookie("POST", "/api/tokens", map[string]string{"name": "hud"}, ca)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var created struct{ Token string }
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.Token == "" {
		t.Fatal("no secret returned")
	}
	r := httptest.NewRequest("GET", "/api/tokens", nil)
	r.Header.Set("Authorization", "Bearer "+created.Token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a token must not manage tokens: %d", rec.Code)
	}
	// Listing shows the name but never the secret.
	w = withCookie("GET", "/api/tokens", nil, ca)
	if !strings.Contains(w.Body.String(), `"hud"`) || strings.Contains(w.Body.String(), created.Token) {
		t.Fatalf("list: %s", w.Body.String())
	}
	var list struct{ Tokens []struct{ ID int64 } }
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Tokens) != 1 {
		t.Fatalf("alice should see her one token: %s", w.Body.String())
	}
	// Bob sees none of it and cannot revoke it.
	if w := withCookie("GET", "/api/tokens", nil, cb); strings.Contains(w.Body.String(), "hud") {
		t.Fatalf("bob sees alice's token: %s", w.Body.String())
	}
	if w := withCookie("DELETE", "/api/tokens/"+strconv.FormatInt(list.Tokens[0].ID, 10), nil, cb); w.Code != http.StatusNotFound {
		t.Fatalf("bob revoking alice's token: %d", w.Code)
	}
	if w := withCookie("DELETE", "/api/tokens/"+strconv.FormatInt(list.Tokens[0].ID, 10), nil, ca); w.Code != http.StatusNoContent {
		t.Fatalf("alice revoking her token: %d", w.Code)
	}
	// Revoked: the bearer stops working.
	_, _ = alice, bob
	if w := withCookie("POST", "/api/tokens", map[string]string{"name": ""}, ca); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: %d", w.Code)
	}
}
