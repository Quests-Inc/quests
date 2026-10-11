package identityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/platform/httpx"
	"github.com/valirum/quests/go/internal/platform/wsctx"
)

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
