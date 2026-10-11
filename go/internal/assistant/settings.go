package assistant

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

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/llmassist"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
	"github.com/quests-inc/quests/go/internal/store"
)

// Handler serves the assistant endpoints: the account's own LLM settings and the free-text action batches.
type Handler struct {
	Store *store.Store
	// Secrets seals the account's key at rest; nil = nothing can be stored.
	Secrets *secretbox.Box
	// TryLLM spends one request of the caller's assistant budget (false = over the limit).
	TryLLM func(user string) bool
	// SelfBase and InternalToken let the executor call this same server over loopback.
	SelfBase      string
	InternalToken string
}

var writeJSON = httpx.WriteJSON

func writeErr(w http.ResponseWriter, status int, detail string) {
	httpx.WriteErr(w, kernel.StatusFor(status, detail), detail)
}

// maxAssistantText bounds one request to the assistant: it is sent to a third party and paid per token.
const maxAssistantText = 8000

// sharedKeyAllowed: may accounts without their own key use the server's key? Only if the host
// opted in (QUESTS_LLM_SHARED=1).
func (h *Handler) sharedKeyAllowed(*http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("QUESTS_LLM_SHARED"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

var errNoLLM = errors.New("ассистент не настроен: задайте провайдера и свой ключ в «Настройки → Ассистент»")

// llmSettingsFor picks the settings for this request: the account's own provider and key
// when it has them, else the server's key if this caller may use it. source is "own" or "server".
func (h *Handler) llmSettingsFor(ctx context.Context, r *http.Request) (llmassist.Settings, string, error) {
	base := llmassist.LoadSettings()
	if p, ok := identity.PrincipalFrom(ctx); ok && p.User.ID != 0 && h.Secrets != nil {
		row, err := h.Store.GetUserLLM(ctx, p.User.ID)
		switch {
		case err == nil && row.KeySealed != "":
			key, oerr := h.Secrets.Open(row.KeySealed)
			if oerr != nil {
				return base, "", oerr
			}
			return base.WithUser(row.Provider, row.BaseURL, row.Model, key), "own", nil
		case err != nil && !errors.Is(err, apperr.ErrNotFound):
			return base, "", err
		}
	}
	if h.sharedKeyAllowed(r) && base.APIKey != "" {
		return base, "server", nil
	}
	return base, "", errNoLLM
}

func (h *Handler) LLMView(r *http.Request) (LLMView, error) {
	v := LLMView{Providers: llmassist.Providers, Provider: "groq", TextLimit: maxAssistantText}
	if p, ok := identity.PrincipalFrom(r.Context()); ok && p.User.ID != 0 && h.Secrets != nil {
		v.Storage = true
		row, err := h.Store.GetUserLLM(r.Context(), p.User.ID)
		if err != nil && !errors.Is(err, apperr.ErrNotFound) {
			return v, err
		}
		if err == nil {
			v.Provider, v.BaseURL, v.Model = row.Provider, row.BaseURL, row.Model
			v.HasKey, v.KeyHint = row.KeySealed != "", row.KeyHint
		}
	}
	v.ServerFallback = h.sharedKeyAllowed(r) && llmassist.LoadSettings().APIKey != ""
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

// GetMy reports the caller's assistant settings.
func (h *Handler) GetMy(w http.ResponseWriter, r *http.Request) {
	v, err := h.LLMView(r)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// sessionUser returns the account behind a browser session; keys are not set from tokens.
func (h *Handler) llmSessionUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	p, ok := identity.PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 || p.Via != "session" {
		writeErr(w, http.StatusForbidden, "the assistant key is managed from the web interface, not with a token")
		return 0, false
	}
	if h.Secrets == nil {
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

// PutMy stores the caller's own provider and key.
func (h *Handler) PutMy(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.llmSessionUser(w, r)
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
		sealed, err := h.Secrets.Seal(in.APIKey)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		row.KeySealed = sealed
		row.KeyHint = hintOf(in.APIKey)
	} else if cur, err := h.Store.GetUserLLM(r.Context(), uid); err != nil || cur.KeySealed == "" {
		writeErr(w, http.StatusUnprocessableEntity, "введите ключ")
		return
	}
	if err := h.Store.SetUserLLM(r.Context(), uid, row); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.GetMy(w, r)
}

func hintOf(key string) string {
	if len(key) <= 8 {
		return "…"
	}
	return "…" + key[len(key)-4:]
}

// DeleteMy removes the caller's own key.
func (h *Handler) DeleteMy(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.llmSessionUser(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteUserLLM(r.Context(), uid); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.GetMy(w, r)
}

// testMyLLM checks a provider/key pair, the typed one (before saving) or the stored one.
// TestMy checks a provider and key without storing them.
func (h *Handler) TestMy(w http.ResponseWriter, r *http.Request) {
	uid, ok := h.llmSessionUser(w, r)
	if !ok {
		return
	}
	if !h.TryLLM(strconv.FormatInt(uid, 10)) {
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
		cur, gerr := h.Store.GetUserLLM(r.Context(), uid)
		if gerr != nil || cur.KeySealed == "" {
			writeErr(w, http.StatusUnprocessableEntity, "введите ключ")
			return
		}
		if key, err = h.Secrets.Open(cur.KeySealed); err != nil {
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
