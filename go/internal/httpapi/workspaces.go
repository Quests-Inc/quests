package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
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
			id, err := s.Store.Automation.RunnerWorkspaceByToken(r.Context(), bearerToken(r))
			if err != nil {
				writeErr(w, http.StatusUnauthorized, "invalid runner token")
				return
			}
			next.ServeHTTP(w, r.WithContext(wsctx.With(r.Context(), id)))
			return
		}

		ws := int64(identity.DefaultWorkspaceID)
		p, hasUser := identity.PrincipalFrom(r.Context())
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
