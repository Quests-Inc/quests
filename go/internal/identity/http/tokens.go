package identityhttp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/platform/httpx"
)

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
