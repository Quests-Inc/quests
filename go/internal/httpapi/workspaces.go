package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/valirum/quests/go/internal/auth"
	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/wsctx"
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
		if s.Store == nil || !s.Store.WorkspacesEnabled() {
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

		ws := int64(store.DefaultWorkspaceID)
		p, hasUser := PrincipalFrom(r.Context())
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
			writeErr(w, http.StatusForbidden, "no workspace")
			return
		}
		next.ServeHTTP(w, r.WithContext(wsctx.With(r.Context(), ws)))
	})
}

func (s *Server) registerWorkspaces(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspaces", s.listWorkspaces)
	mux.HandleFunc("POST /api/workspaces", s.createWorkspace)
	mux.HandleFunc("POST /api/workspaces/{id}/switch", s.switchWorkspace)
}

func (s *Server) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	cur := wsctx.From(r.Context())
	if cur == 0 {
		cur = store.DefaultWorkspaceID
	}
	var list []store.Workspace
	var err error
	if p, ok := PrincipalFrom(r.Context()); ok && p.User.ID != 0 {
		list, err = s.Store.UserWorkspaces(r.Context(), p.User.ID)
	} else {
		list, err = s.Store.AllWorkspaces(r.Context())
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.Workspace{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": cur, "workspaces": list, "enabled": s.Store.WorkspacesEnabled()})
}

func (s *Server) createWorkspace(w http.ResponseWriter, r *http.Request) {
	if !s.Store.WorkspacesEnabled() {
		writeErr(w, http.StatusBadRequest, "workspaces need PostgreSQL")
		return
	}
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
	ws, err := s.Store.CreateWorkspace(r.Context(), p.User.ID, body.Name, wsctx.From(r.Context()))
	if errors.Is(err, store.ErrWorkspaceName) {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
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
		if errors.Is(err, auth.ErrNotMember) {
			writeErr(w, http.StatusForbidden, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"current": id})
}
