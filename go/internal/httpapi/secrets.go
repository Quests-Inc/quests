package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"

	"github.com/valirum/quests/go/internal/store"
)

// secretKeyPattern matches a valid env var name — secrets are injected into a
// command's process verbatim as NAME=value (emit_pool_command and step
// check_command), so the name has to be one.
var secretKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// secretOwnerKinds maps the URL segment to the owner kind:
// /api/{questlines|templates|quests|steps}/{id}/secrets[/{key}].
var secretOwnerKinds = map[string]string{
	"questlines": store.SecretOwnerQuestline,
	"templates":  store.SecretOwnerTemplate,
	"quests":     store.SecretOwnerQuest,
	"steps":      store.SecretOwnerStep,
	"notes":      store.SecretOwnerNote,
}

var secretOwnerLabels = map[string]string{
	store.SecretOwnerQuestline: "Questline",
	store.SecretOwnerTemplate:  "Template",
	store.SecretOwnerQuest:     "Quest",
	store.SecretOwnerStep:      "Step",
	store.SecretOwnerNote:      "Note",
}

// secretOwner resolves {id} for the route's owner kind and 404s when the
// owning row does not exist.
func (s *Server) secretOwner(w http.ResponseWriter, r *http.Request, segment string) (store.SecretOwner, bool) {
	kind := secretOwnerKinds[segment]
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return store.SecretOwner{}, false
	}
	o := store.SecretOwner{Kind: kind, ID: id}
	ok, err := s.Store.SecretOwnerExists(r.Context(), o)
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
func (s *Server) listSecrets(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		o, ok := s.secretOwner(w, r, segment)
		if !ok {
			return
		}
		own, err := s.Store.ListSecretKeys(r.Context(), o)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		eff, err := s.Store.ListEffectiveSecretKeys(r.Context(), o)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		inherited := []map[string]any{}
		for _, ref := range eff {
			if ref.Owner == o {
				continue
			}
			inherited = append(inherited, map[string]any{"key": ref.Key, "from": ref.Owner.Kind, "id": ref.Owner.ID})
		}
		writeJSON(w, http.StatusOK, map[string]any{"keys": own, "inherited": inherited})
	}
}

// putSecret writes one secret value — write-only: the request body is never
// echoed back and no GET returns it.
func (s *Server) putSecret(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Secrets feed the environment of the owner's commands: only owners write them.
		if !s.requireOwner(w, r, "set secrets") {
			return
		}
		key := r.PathValue("key")
		if !secretKeyPattern.MatchString(key) {
			writeErr(w, 422, "key must look like an env var name: [A-Za-z_][A-Za-z0-9_]*")
			return
		}
		if store.ReservedSecretName(key) {
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
		o, ok := s.secretOwner(w, r, segment)
		if !ok {
			return
		}
		if err := s.Store.SetSecret(r.Context(), o, key, body.Value); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.WriteHeader(204)
	}
}

func (s *Server) deleteSecret(segment string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.requireOwner(w, r, "remove secrets") {
			return
		}
		o, ok := s.secretOwner(w, r, segment)
		if !ok {
			return
		}
		if err := s.Store.DeleteSecret(r.Context(), o, r.PathValue("key")); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.WriteHeader(204)
	}
}

func (s *Server) registerSecretRoutes(mux *http.ServeMux) {
	for segment := range secretOwnerKinds {
		mux.HandleFunc("GET /api/"+segment+"/{id}/secrets", s.listSecrets(segment))
		mux.HandleFunc("PUT /api/"+segment+"/{id}/secrets/{key}", s.putSecret(segment))
		mux.HandleFunc("DELETE /api/"+segment+"/{id}/secrets/{key}", s.deleteSecret(segment))
	}
}
