package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/valirum/quests/go/internal/llmassist"
	"github.com/valirum/quests/go/internal/store"
)

// maxAssistantText bounds one request to the assistant: it is sent to a third party and paid per token.
const maxAssistantText = 8000

func (s *Server) registerLLMSettings(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/me/llm", s.getMyLLM)
	mux.HandleFunc("PUT /api/me/llm", s.putMyLLM)
	mux.HandleFunc("DELETE /api/me/llm", s.deleteMyLLM)
	mux.HandleFunc("POST /api/me/llm/test", s.testMyLLM)
}

// sharedKeyAllowed: may accounts without their own key use the server's key? The operator
// always may; everyone else only if the operator opted in (QUESTS_LLM_SHARED=1).
func (s *Server) sharedKeyAllowed(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("QUESTS_LLM_SHARED"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return s.isOperator(r)
}

var errNoLLM = errors.New("ассистент не настроен: задайте провайдера и свой ключ в «Настройки → Ассистент»")

// llmSettingsFor picks the settings for this request: the account's own provider and key
// when it has them, else the server's key if this caller may use it. source is "own" or "server".
func (s *Server) llmSettingsFor(ctx context.Context, r *http.Request) (llmassist.Settings, string, error) {
	base := llmassist.LoadSettings()
	if p, ok := PrincipalFrom(ctx); ok && p.User.ID != 0 && s.Store.WorkspacesEnabled() && s.Secrets != nil {
		row, err := s.Store.GetUserLLM(ctx, p.User.ID)
		switch {
		case err == nil && row.KeySealed != "":
			key, oerr := s.Secrets.Open(row.KeySealed)
			if oerr != nil {
				return base, "", oerr
			}
			return base.WithUser(row.Provider, row.BaseURL, row.Model, key), "own", nil
		case err != nil && !errors.Is(err, store.ErrNotFound):
			return base, "", err
		}
	}
	if s.sharedKeyAllowed(r) && base.APIKey != "" {
		return base, "server", nil
	}
	return base, "", errNoLLM
}

type llmView struct {
	Providers      []llmassist.Provider `json:"providers"`
	Provider       string               `json:"provider"`
	BaseURL        string               `json:"base_url"`
	Model          string               `json:"model"`
	HasKey         bool                 `json:"has_key"`
	KeyHint        string               `json:"key_hint"`
	Source         string               `json:"source"` // own | server | none
	ServerFallback bool                 `json:"server_fallback"`
	Storage        bool                 `json:"storage"` // can this instance keep a personal key at all
	TextLimit      int                  `json:"text_limit"`
}

func (s *Server) llmView(r *http.Request) (llmView, error) {
	v := llmView{Providers: llmassist.Providers, Provider: "groq", TextLimit: maxAssistantText}
	if p, ok := PrincipalFrom(r.Context()); ok && p.User.ID != 0 && s.Store.WorkspacesEnabled() && s.Secrets != nil {
		v.Storage = true
		row, err := s.Store.GetUserLLM(r.Context(), p.User.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return v, err
		}
		if err == nil {
			v.Provider, v.BaseURL, v.Model = row.Provider, row.BaseURL, row.Model
			v.HasKey, v.KeyHint = row.KeySealed != "", row.KeyHint
		}
	}
	v.ServerFallback = s.sharedKeyAllowed(r) && llmassist.LoadSettings().APIKey != ""
	switch {
	case v.HasKey:
		v.Source = "own"
	case v.ServerFallback:
		v.Source = "server"
	default:
		v.Source = "none"
	}
	return v, nil
}

func (s *Server) getMyLLM(w http.ResponseWriter, r *http.Request) {
	v, err := s.llmView(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// sessionUser returns the account behind a browser session; keys are not set from tokens.
func (s *Server) llmSessionUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	p, ok := PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 || p.Via != "session" {
		writeErr(w, http.StatusForbidden, "the assistant key is managed from the web interface, not with a token")
		return 0, false
	}
	if !s.Store.WorkspacesEnabled() || s.Secrets == nil {
		writeErr(w, http.StatusConflict, "личный ключ хранится только при PostgreSQL; на этом сервере ассистент берёт ключ из окружения")
		return 0, false
	}
	return p.User.ID, true
}

type llmInput struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
}

// clean validates the form; the returned base is what to store (custom only).
func (in *llmInput) clean() (string, error) {
	p, ok := llmassist.ProviderByID(in.Provider)
	if !ok {
		return "", errors.New("неизвестный провайдер")
	}
	in.Model = strings.TrimSpace(in.Model)
	if len(in.Model) > 100 {
		return "", errors.New("модель: не длиннее 100 символов")
	}
	in.APIKey = strings.TrimSpace(in.APIKey)
	if len(in.APIKey) > 512 {
		return "", errors.New("ключ слишком длинный")
	}
	for _, c := range in.APIKey {
		if unicode.IsSpace(c) || unicode.IsControl(c) {
			return "", errors.New("ключ не должен содержать пробелов и управляющих символов")
		}
	}
	if !p.EditableBase {
		return "", nil
	}
	if in.Model == "" {
		return "", errors.New("для своего адреса укажите модель")
	}
	return llmassist.CheckBaseURL(in.BaseURL)
}

func (s *Server) putMyLLM(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.llmSessionUser(w, r)
	if !ok {
		return
	}
	var in llmInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	base, err := in.clean()
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	row := store.UserLLM{Provider: in.Provider, BaseURL: base, Model: in.Model}
	if in.APIKey != "" {
		sealed, err := s.Secrets.Seal(in.APIKey)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		row.KeySealed = sealed
		row.KeyHint = hintOf(in.APIKey)
	} else if cur, err := s.Store.GetUserLLM(r.Context(), uid); err != nil || cur.KeySealed == "" {
		writeErr(w, http.StatusUnprocessableEntity, "введите ключ")
		return
	}
	if err := s.Store.SetUserLLM(r.Context(), uid, row); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.getMyLLM(w, r)
}

func hintOf(key string) string {
	if len(key) <= 8 {
		return "…"
	}
	return "…" + key[len(key)-4:]
}

func (s *Server) deleteMyLLM(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.llmSessionUser(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteUserLLM(r.Context(), uid); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.getMyLLM(w, r)
}

// testMyLLM checks a provider/key pair, the typed one (before saving) or the stored one.
func (s *Server) testMyLLM(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.llmSessionUser(w, r)
	if !ok {
		return
	}
	if !s.limiter().tryLLM(strconv.FormatInt(uid, 10)) {
		writeErr(w, http.StatusTooManyRequests, "слишком много запросов к ассистенту, повторите позже")
		return
	}
	var in llmInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	base, err := in.clean()
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	key := in.APIKey
	if key == "" {
		cur, gerr := s.Store.GetUserLLM(r.Context(), uid)
		if gerr != nil || cur.KeySealed == "" {
			writeErr(w, http.StatusUnprocessableEntity, "введите ключ")
			return
		}
		if key, err = s.Secrets.Open(cur.KeySealed); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	settings := llmassist.LoadSettings().WithUser(in.Provider, base, in.Model, key)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := llmassist.Ping(ctx, settings); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "detail": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
