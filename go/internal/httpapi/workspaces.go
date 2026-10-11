package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/valirum/quests/go/internal/platform/httpx"
	"net/http"
	"strconv"
	"strings"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/store"
)

// WorkspaceHeader lets a client pick one of its workspaces per request. Tokens
// are pinned to a workspace at creation; sessions switch via POST
// /api/workspaces/{id}/switch. The header is checked against membership.
const WorkspaceHeader = "X-Quests-Workspace"

// withWorkspace puts the caller's workspace on the request context, which the
// PostgreSQL driver turns into the row-level-security setting. Without
// PostgreSQL it is a no-op (a single implicit workspace).
func (s *Server) withWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Store == nil {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		if isPublicAPI(path) || isSPAAsset(path) || path == "/api/runner/register" {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(path, "/api/runner/") {
			// Runners authenticate with their own token; it names the workspace.
			id, err := s.Store.RunnerWorkspaceByToken(r.Context(), bearerToken(r))
			if err != nil {
				writeErr(w, http.StatusUnauthorized, "invalid runner token")
				return
			}
			next.ServeHTTP(w, r.WithContext(wsctx.With(r.Context(), id)))
			return
		}

		ws := int64(identity.DefaultWorkspaceID)
		p, hasUser := PrincipalFrom(r.Context())
		if p.TokenName == "internal" && strings.TrimSpace(r.Header.Get(WorkspaceHeader)) == "" {
			// The loopback token is not a person: falling back to the default workspace would
			// let whoever triggered the call act on someone else's data.
			writeErr(w, http.StatusBadRequest, "internal calls must name a workspace ("+WorkspaceHeader+")")
			return
		}
		hasUser = hasUser && p.User.ID != 0
		if hasUser {
			ws = p.User.WorkspaceID
		}
		if raw := strings.TrimSpace(r.Header.Get(WorkspaceHeader)); raw != "" {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 {
				writeErr(w, http.StatusBadRequest, "invalid "+WorkspaceHeader)
				return
			}
			if hasUser {
				ok, err := s.Auth.IsMember(p.User.ID, id)
				if err != nil {
					writeErr(w, http.StatusInternalServerError, err.Error())
					return
				}
				if !ok {
					writeErr(w, http.StatusForbidden, "not a member of that workspace")
					return
				}
			}
			// No user (auth off, or the in-process token) = trusted, any workspace.
			ws = id
		}
		if ws <= 0 {
			// An account without any workspace may only manage workspaces / sessions.
			if strings.HasPrefix(path, "/api/workspaces") || strings.HasPrefix(path, "/api/auth/") {
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, http.StatusForbidden, "no workspace")
			return
		}
		ctx := wsctx.With(r.Context(), ws)
		switch {
		case hasUser:
			ctx = wsctx.WithUser(ctx, p.User.ID)
		case p.TokenName == "internal":
			ctx = wsctx.WithUser(ctx, wsctx.ActorAutomation)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireOwner lets only an owner of the caller's current workspace through. Callers
// without an account (auth off, the in-process token) pass: there is nobody to tell apart.
func (s *Server) requireOwner(w http.ResponseWriter, r *http.Request, what string) bool {
	p, ok := PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 {
		return true
	}
	role, err := s.Store.Identity().MemberRole(r.Context(), wsctx.From(r.Context()), p.User.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if role != identity.RoleOwner {
		writeErr(w, http.StatusForbidden, "only an owner can "+what)
		return false
	}
	return true
}

func (s *Server) registerWorkspaces(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspaces", s.listWorkspaces)
	mux.HandleFunc("POST /api/workspaces", s.createWorkspace)
	mux.HandleFunc("POST /api/workspaces/{id}/switch", s.switchWorkspace)
	mux.HandleFunc("PATCH /api/workspaces/{id}", s.renameWorkspace)
	mux.HandleFunc("DELETE /api/workspaces/{id}", s.deleteWorkspace)
	mux.HandleFunc("GET /api/workspaces/{id}/members", s.listMembers)
	mux.HandleFunc("DELETE /api/workspaces/{id}/members/{uid}", s.removeMember)
	mux.HandleFunc("POST /api/workspaces/{id}/invites", s.createInvite)
	mux.HandleFunc("GET /api/invites/{token}", s.getInvite)
	mux.HandleFunc("POST /api/invites/{token}/accept", s.acceptInvite)
}

// memberOf checks that the caller belongs to workspace {id} and returns the id
// and their role. Callers without an account (auth off) are refused: members
// and invitations only make sense with accounts.
func (s *Server) memberOf(w http.ResponseWriter, r *http.Request) (int64, string, identity.User, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, ok := PrincipalFrom(r.Context())
	if err != nil || id <= 0 || !ok || p.User.ID == 0 {
		writeErr(w, http.StatusBadRequest, "needs an account and a workspace id")
		return 0, "", identity.User{}, false
	}
	role, err := s.Store.Identity().MemberRole(r.Context(), id, p.User.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return 0, "", identity.User{}, false
	}
	if role == "" {
		// Do not reveal whether the workspace exists.
		writeErr(w, http.StatusNotFound, "workspace not found")
		return 0, "", identity.User{}, false
	}
	return id, role, p.User, true
}

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	id, role, _, ok := s.memberOf(w, r)
	if !ok {
		return
	}
	members, err := s.Store.Identity().WorkspaceMembers(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members, "role": role})
}

// removeMember lets an owner remove anyone, and anyone leave.
func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	id, role, me, ok := s.memberOf(w, r)
	if !ok {
		return
	}
	uid, err := strconv.ParseInt(r.PathValue("uid"), 10, 64)
	if err != nil || uid <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if uid != me.ID && role != identity.RoleOwner {
		writeErr(w, http.StatusForbidden, "only an owner can remove other members")
		return
	}
	switch err := s.Store.Identity().RemoveWorkspaceMember(r.Context(), id, uid); {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not a member")
	case errors.Is(err, identity.ErrLastOwner):
		writeErr(w, http.StatusConflict, err.Error())
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// createInvite mints a one-time link (owners only). The secret is shown once.
func (s *Server) createInvite(w http.ResponseWriter, r *http.Request) {
	id, role, me, ok := s.memberOf(w, r)
	if !ok {
		return
	}
	if role != identity.RoleOwner {
		writeErr(w, http.StatusForbidden, "only an owner can invite")
		return
	}
	secret, exp, err := s.Store.Identity().CreateInvite(r.Context(), id, me.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":      secret,
		"path":       "/?invite=" + secret,
		"expires_at": exp.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

func (s *Server) getInvite(w http.ResponseWriter, r *http.Request) {
	if !s.limiter().TryInvite(httpx.ClientKey(r)) {
		writeErr(w, http.StatusTooManyRequests, "слишком много запросов")
		return
	}
	name, exp, err := s.Store.Identity().InviteInfo(r.Context(), r.PathValue("token"))
	if errors.Is(err, identity.ErrInviteInvalid) {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, loggedIn := s.principal(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace":     name,
		"expires_at":    exp.UTC().Format("2006-01-02T15:04:05Z"),
		"authenticated": loggedIn,
	})
}

// acceptInvite spends an invitation. A logged-in caller joins with their account;
// otherwise username+password create a new account that joins (the only way to
// sign up on this instance besides the admin CLI).
func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request) {
	if !s.AuthRequired {
		writeErr(w, http.StatusBadRequest, "invitations need accounts and workspaces")
		return
	}
	if !s.originAllowed(r) {
		writeErr(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	if !s.limiter().TryInvite(httpx.ClientKey(r)) {
		writeErr(w, http.StatusTooManyRequests, "слишком много попыток, повторите позже")
		return
	}
	secret := r.PathValue("token")
	if _, _, err := s.Store.Identity().InviteInfo(r.Context(), secret); err != nil {
		writeErr(w, http.StatusNotFound, identity.ErrInviteInvalid.Error())
		return
	}

	var user identity.User
	created := false
	if p, ok := s.principal(r); ok && p.User.ID != 0 {
		user = p.User
	} else {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		u, err := s.Auth.CreateUser(body.Username, body.Password)
		switch {
		case errors.Is(err, identity.ErrUserExists):
			writeErr(w, http.StatusConflict, "такой логин уже занят — войдите и откройте ссылку снова")
			return
		case errors.Is(err, identity.ErrWeakPassword):
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		case err != nil:
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		user, created = u, true
	}

	wsID, err := s.Store.Identity().ClaimInvite(r.Context(), secret, user.ID)
	if err != nil {
		if created {
			_ = s.Auth.DeleteUser(user.ID) // lost a race for the invitation: undo the signup
		}
		if errors.Is(err, identity.ErrInviteInvalid) {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if created {
		secretSess, err := s.startSession(w, r, user.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = s.Auth.SetSessionWorkspace(secretSess, user.ID, wsID)
	} else if c, err := r.Cookie(SessionCookie); err == nil {
		_ = s.Auth.SetSessionWorkspace(c.Value, user.ID, wsID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace_id": wsID, "username": user.Username, "created": created})
}

// WorkspacesView is the account's workspaces and what the current one has stored (absent when the usage is unknown).
type WorkspacesView struct {
	Current          int64                `json:"current"`
	Workspaces       []identity.Workspace `json:"workspaces"`
	AttachmentsUsed  *int64               `json:"attachments_used,omitempty"`
	AttachmentsQuota *int64               `json:"attachments_quota,omitempty"`
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	cur := wsctx.From(r.Context())
	if p, ok := PrincipalFrom(r.Context()); cur == 0 && (!ok || p.User.ID == 0) {
		cur = identity.DefaultWorkspaceID
	}
	var list []identity.Workspace
	var err error
	if p, ok := PrincipalFrom(r.Context()); ok && p.User.ID != 0 {
		list, err = s.Store.Identity().UserWorkspaces(r.Context(), p.User.ID)
	} else {
		list, err = s.Store.Identity().AllWorkspaces(r.Context())
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []identity.Workspace{}
	}
	view := WorkspacesView{Current: cur, Workspaces: list}
	if used, err := s.Store.AttachmentUsage(r.Context()); err == nil {
		view.AttachmentsUsed, view.AttachmentsQuota = &used, &s.WorkspaceQuota
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	p, ok := PrincipalFrom(r.Context())
	if !ok || p.User.ID == 0 {
		writeErr(w, http.StatusBadRequest, "creating a workspace needs an account")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	ws, err := ProvisionWorkspace(r.Context(), s.Store, p.User.ID, body.Name, wsctx.From(r.Context()))
	if errors.Is(err, identity.ErrWorkspaceName) {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if errors.Is(err, identity.ErrWorkspaceLimit) {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ws)
}

// switchWorkspace pins the calling login session to another workspace. API
// tokens are pinned when minted, so only browser sessions switch.
func (s *Server) switchWorkspace(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid workspace id")
		return
	}
	p, ok := PrincipalFrom(r.Context())
	if !ok || p.Via != "session" {
		writeErr(w, http.StatusBadRequest, "only a logged-in browser session can switch workspace")
		return
	}
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no session")
		return
	}
	if err := s.Auth.SetSessionWorkspace(c.Value, p.User.ID, id); err != nil {
		if errors.Is(err, identity.ErrNotMember) {
			writeErr(w, http.StatusForbidden, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": id})
}

func (s *Server) renameWorkspace(w http.ResponseWriter, r *http.Request) {
	id, role, _, ok := s.memberOf(w, r)
	if !ok {
		return
	}
	if role != identity.RoleOwner {
		writeErr(w, http.StatusForbidden, "only an owner can rename")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	switch err := s.Store.Identity().RenameWorkspace(r.Context(), id, body.Name); {
	case errors.Is(err, identity.ErrWorkspaceName):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "workspace not found")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "name": strings.TrimSpace(body.Name)})
	}
}

// deleteWorkspace hides the workspace and schedules its purge (owners only, name must be typed).
func (s *Server) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	id, role, _, ok := s.memberOf(w, r)
	if !ok {
		return
	}
	if role != identity.RoleOwner {
		writeErr(w, http.StatusForbidden, "only an owner can delete a workspace")
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	name, err := s.Store.Identity().WorkspaceName(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "workspace not found")
		return
	}
	if strings.TrimSpace(body.Confirm) != name {
		writeErr(w, http.StatusUnprocessableEntity, "введите название пространства для подтверждения")
		return
	}
	purgeAt, err := s.Store.Identity().DeleteWorkspace(r.Context(), id)
	switch {
	case errors.Is(err, identity.ErrMainWorkspace):
		writeErr(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "workspace not found")
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]any{"purge_at": purgeAt.UTC().Format("2006-01-02T15:04:05Z")})
	}
}

// attachmentUsage reports how much of the attachment quota the current workspace uses.
