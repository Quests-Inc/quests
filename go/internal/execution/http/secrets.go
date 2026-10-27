package executionhttp

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"

	"github.com/quests-inc/quests/go/internal/execution"
)

// secretKeyPattern matches a valid env var name — secrets are injected into a
// command's process verbatim as NAME=value (emit_pool_command and step
// check_command), so the name has to be one.
var secretKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// secretOwnerKinds maps the URL segment to the owner kind:
// /api/{questlines|templates|quests|steps}/{id}/secrets[/{key}].
var secretOwnerKinds = map[string]string{
	"questlines": execution.SecretOwnerQuestline,
	"templates":  execution.SecretOwnerTemplate,
	"quests":     execution.SecretOwnerQuest,
	"steps":      execution.SecretOwnerStep,
	"notes":      execution.SecretOwnerNote,
}

// workspaceSegment is the route of the workspace's own secrets (no id: the workspace is the one the request works in).
const workspaceSegment = "workspace"

var secretOwnerLabels = map[string]string{
	execution.SecretOwnerQuestline: "Questline",
	execution.SecretOwnerTemplate:  "Template",
	execution.SecretOwnerQuest:     "Quest",
	execution.SecretOwnerStep:      "Step",
	execution.SecretOwnerNote:      "Note",
	execution.SecretOwnerWorkspace: "Workspace",
}

// secretOwner resolves {id} for the route's owner kind and 404s when the
// owning row does not exist.
func (h *Handler) secretOwner(w http.ResponseWriter, r *http.Request, segment string) (execution.SecretOwner, bool) {
	if segment == workspaceSegment {
		return execution.WorkspaceSecretOwner(r.Context()), true
	}
	kind := secretOwnerKinds[segment]
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return execution.SecretOwner{}, false
	}
	o := execution.SecretOwner{Kind: kind, ID: id}
	ok, err := h.Store.SecretOwnerExists(r.Context(), o)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return o, false
	}
	if !ok {
		writeErr(w, http.StatusNotFound, secretOwnerLabels[kind]+" not found")
		return o, false
	}
	return o, true
}

// listSecrets returns only names: the owner's own (`keys`, the original
// response shape) and what it inherits (`inherited`, each with its source).
// Values never come back from a read endpoint or MCP tool.
func (h *Handler) listSecrets(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o, ok := h.secretOwner(w, r, segment)
		if !ok {
			return
		}
		if fromMCP(r) && o.Kind == execution.SecretOwnerNote {
			priv, err := h.Store.NotePrivate(r.Context(), o.ID)
			if err != nil {
				valueWriteErr(w, err)
				return
			}
			if priv {
				writeErr(w, http.StatusForbidden, "приватная заметка")
				return
			}
		}
		own, err := h.Store.ListSecretKeys(r.Context(), o)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		eff, err := h.Store.ListVisibleSecretKeys(r.Context(), o, fromMCP(r))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ownRows := []map[string]any{}
		inherited := []map[string]any{}
		effective := []map[string]any{}
		for _, ref := range eff {
			row := map[string]any{"key": ref.Key, "from": ref.Owner.Kind, "id": ref.Owner.ID, "title": h.Store.OwnerTitle(r.Context(), ref.Owner)}
			effective = append(effective, row)
			if ref.Owner == o {
				ownRows = append(ownRows, row)
			} else {
				inherited = append(inherited, row)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"keys": own, "own": ownRows, "inherited": inherited, "effective": effective})
	}
}

// putSecret writes one secret value — write-only: the request body is never
// echoed back and no GET returns it.
func (h *Handler) putSecret(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Secrets feed the environment of the owner's commands: only owners write them.
		if !h.RequireOwner(w, r, "set secrets") {
			return
		}
		key := r.PathValue("key")
		if !secretKeyPattern.MatchString(key) {
			writeErr(w, 422, "key must look like an env var name: [A-Za-z_][A-Za-z0-9_]*")
			return
		}
		if execution.ReservedSecretName(key) {
			writeErr(w, 422, "this name is reserved: it would change how commands start, not carry a credential")
			return
		}
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			writeErr(w, 400, "invalid JSON")
			return
		}
		if body.Value == "" {
			writeErr(w, 422, "value required (use DELETE to remove a secret)")
			return
		}
		o, ok := h.secretOwner(w, r, segment)
		if !ok {
			return
		}
		if err := h.Store.SetSecret(r.Context(), o, key, body.Value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.WriteHeader(204)
	}
}

func (h *Handler) deleteSecret(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.RequireOwner(w, r, "remove secrets") {
			return
		}
		o, ok := h.secretOwner(w, r, segment)
		if !ok {
			return
		}
		if err := h.Store.DeleteSecret(r.Context(), o, r.PathValue("key")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.WriteHeader(204)
	}
}

func (h *Handler) registerSecretRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspace/secrets", h.listSecrets(workspaceSegment))
	mux.HandleFunc("PUT /api/workspace/secrets/{key}", h.putSecret(workspaceSegment))
	mux.HandleFunc("DELETE /api/workspace/secrets/{key}", h.deleteSecret(workspaceSegment))
	for segment := range secretOwnerKinds {
		mux.HandleFunc("GET /api/"+segment+"/{id}/secrets", h.listSecrets(segment))
		mux.HandleFunc("PUT /api/"+segment+"/{id}/secrets/{key}", h.putSecret(segment))
		mux.HandleFunc("DELETE /api/"+segment+"/{id}/secrets/{key}", h.deleteSecret(segment))
	}
}
