package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
	"github.com/quests-inc/quests/go/internal/store"
)

type fixture struct {
	h    *Handler
	user identity.User
}

func newFixture(t *testing.T, withSecrets bool) *fixture {
	t.Helper()
	t.Setenv("QUESTS_LLM_SHARED", "")
	t.Setenv("QUESTS_LLM_API_KEY", "")
	pool, url := pgtest.NewWithURL(t)
	st := store.New(pool, url, nil)
	user, err := st.Identity.CreateUser("alice", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Store: st, TryLLM: func(string) bool { return true }}
	if withSecrets {
		box, err := secretbox.New(bytes.Repeat([]byte{7}, 32))
		if err != nil {
			t.Fatal(err)
		}
		h.Secrets = box
	}
	return &fixture{h: h, user: user}
}

func (f *fixture) call(fn http.HandlerFunc, via string, body any) *httptest.ResponseRecorder {
	var r *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		r = httptest.NewRequest("POST", "/api/me/llm", bytes.NewReader(b))
	} else {
		r = httptest.NewRequest("GET", "/api/me/llm", nil)
	}
	if via != "" {
		r = r.WithContext(identity.WithPrincipal(context.Background(), identity.Principal{User: f.user, Via: via}))
	}
	w := httptest.NewRecorder()
	fn(w, r)
	return w
}

func view(t *testing.T, w *httptest.ResponseRecorder) LLMView {
	t.Helper()
	var v LLMView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	return v
}

func TestSettingsLifecycle(t *testing.T) {
	f := newFixture(t, true)
	v := view(t, f.call(f.h.GetMy, "session", nil))
	if v.Source != "none" || v.HasKey || !v.Storage || v.TextLimit != maxAssistantText {
		t.Fatalf("fresh view = %+v", v)
	}

	// A key is required the first time; then it is stored sealed, shown only as a hint.
	if w := f.call(f.h.PutMy, "session", map[string]any{"provider": "groq"}); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "введите ключ") {
		t.Fatalf("no key = %d %s", w.Code, w.Body.String())
	}
	w := f.call(f.h.PutMy, "session", map[string]any{"provider": "groq", "model": " llama ", "api_key": "sk-secret-abcdef"})
	if w.Code != http.StatusOK {
		t.Fatalf("put = %d %s", w.Code, w.Body.String())
	}
	v = view(t, w)
	if !v.HasKey || v.Source != "own" || v.KeyHint != "…cdef" || v.Model != "llama" {
		t.Fatalf("after put = %+v", v)
	}
	if strings.Contains(w.Body.String(), "sk-secret") {
		t.Fatal("the key must never come back")
	}
	// Changing only the model keeps the stored key.
	if v = view(t, f.call(f.h.PutMy, "session", map[string]any{"provider": "groq", "model": "other"})); !v.HasKey || v.Model != "other" {
		t.Fatalf("model only = %+v", v)
	}
	if v = view(t, f.call(f.h.DeleteMy, "session", nil)); v.HasKey || v.Source != "none" {
		t.Fatalf("after delete = %+v", v)
	}
}

func TestSettingsRefusals(t *testing.T) {
	f := newFixture(t, true)
	for _, c := range []struct {
		name string
		via  string
		body map[string]any
		code int
		want string
	}{
		{"token", "token", map[string]any{"provider": "groq", "api_key": "k"}, http.StatusForbidden, "web interface"},
		{"anonymous", "", map[string]any{"provider": "groq", "api_key": "k"}, http.StatusForbidden, "web interface"},
		{"provider", "session", map[string]any{"provider": "nope", "api_key": "k"}, http.StatusUnprocessableEntity, "провайдер"},
		{"spaces in key", "session", map[string]any{"provider": "groq", "api_key": "a b"}, http.StatusUnprocessableEntity, "пробел"},
		{"long key", "session", map[string]any{"provider": "groq", "api_key": strings.Repeat("k", 600)}, http.StatusUnprocessableEntity, "длинный"},
		{"long model", "session", map[string]any{"provider": "groq", "model": strings.Repeat("m", 200), "api_key": "k"}, http.StatusUnprocessableEntity, "100"},
	} {
		w := f.call(f.h.PutMy, c.via, c.body)
		if w.Code != c.code || !strings.Contains(w.Body.String(), c.want) {
			t.Errorf("%s: %d %s", c.name, w.Code, w.Body.String())
		}
	}
	// Without PostgreSQL-side secrets the personal key cannot be kept at all.
	nf := newFixture(t, false)
	if w := nf.call(nf.h.PutMy, "session", map[string]any{"provider": "groq", "api_key": "k"}); w.Code != http.StatusConflict {
		t.Errorf("no secretbox = %d", w.Code)
	}
	if v := view(t, nf.call(nf.h.GetMy, "session", nil)); v.Storage {
		t.Errorf("storage must be off without secretbox: %+v", v)
	}
}

func TestSettingsPickedPerRequest(t *testing.T) {
	f := newFixture(t, true)
	r := httptest.NewRequest("POST", "/api/llm/actions/preview", nil)
	if _, _, err := f.h.llmSettingsFor(r.Context(), r); err != errNoLLM {
		t.Fatalf("nothing configured: %v", err)
	}
	ctx := identity.WithPrincipal(context.Background(), identity.Principal{User: f.user, Via: "session"})
	r = r.WithContext(ctx)
	f.call(f.h.PutMy, "session", map[string]any{"provider": "groq", "api_key": "sk-own-key-1234"})
	got, source, err := f.h.llmSettingsFor(ctx, r)
	if err != nil || source != "own" || got.APIKey != "sk-own-key-1234" {
		t.Fatalf("own key: %v %q %q", err, source, got.APIKey)
	}
}

func TestPreviewRefusals(t *testing.T) {
	f := newFixture(t, true)
	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		f.h.Preview(w, httptest.NewRequest("POST", "/api/llm/actions/preview", strings.NewReader(body)))
		return w
	}
	if w := post("не json"); w.Code != http.StatusBadRequest {
		t.Errorf("bad json = %d", w.Code)
	}
	if w := post(`{"text":"   "}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "text is required") {
		t.Errorf("empty = %d %s", w.Code, w.Body.String())
	}
	long, _ := json.Marshal(map[string]string{"text": strings.Repeat("я", maxAssistantText+1)})
	if w := post(string(long)); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("long = %d", w.Code)
	}
	if w := post(`{"text":"сделай"}`); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "не настроен") {
		t.Errorf("no llm = %d %s", w.Code, w.Body.String())
	}
}
