package executionhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
)

// Workspace hooks (quest=438): event + script from the library. Members read; only an owner writes.
func (h *Handler) registerHooks(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/hooks", h.listHooks)
	mux.HandleFunc("POST /api/hooks", h.createHook)
	mux.HandleFunc("GET /api/hooks/{id}", h.getHook)
	mux.HandleFunc("PATCH /api/hooks/{id}", h.patchHook)
	mux.HandleFunc("DELETE /api/hooks/{id}", h.deleteHook)
	mux.HandleFunc("GET /api/hook-events", h.listHookEvents)
}

func hookErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, execution.ErrNotFound):
		writeErr(w, http.StatusNotFound, "хук не найден")
	case errors.Is(err, kernel.ErrNoRunner):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
	case apperr.IsBad(err):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
	}
}

func hookID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func (h *Handler) listHooks(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListHooks(r.Context())
	if err != nil {
		hookErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) getHook(w http.ResponseWriter, r *http.Request) {
	id, ok := hookID(w, r)
	if !ok {
		return
	}
	hk, err := h.Store.GetHook(r.Context(), id)
	if err != nil {
		hookErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hk)
}

func (h *Handler) listHookEvents(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, execution.HookEventsCatalog())
}

func (h *Handler) createHook(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "create hooks") {
		return
	}
	var in struct {
		Name       string   `json:"name"`
		Enabled    *bool    `json:"enabled"`
		Events     []string `json:"events"`
		ScriptID   int64    `json:"script_id"`
		QuestID    *int64   `json:"quest_id"`
		RunOn      string   `json:"run_on"`
		TimeoutSec int      `json:"timeout_sec"`
		ScopeKind  *string  `json:"scope_kind"`
		ScopeID    *int64   `json:"scope_id"`
		Cooldown   *int     `json:"cooldown_sec"`
		MaxPerHour *int     `json:"max_per_hour"`
		Private    *bool    `json:"include_private"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if in.ScriptID < 1 {
		writeErr(w, http.StatusUnprocessableEntity, "script_id required")
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	hk, err := h.Store.CreateHookIn(r.Context(), in.Name, in.Events, in.ScriptID, in.QuestID, in.RunOn, in.TimeoutSec, enabled, execution.HookIn{
		ScopeKind: in.ScopeKind, ScopeID: in.ScopeID, CooldownSec: in.Cooldown, MaxPerHour: in.MaxPerHour, IncludePrivate: in.Private,
	})
	if err != nil {
		hookErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, hk)
}

func (h *Handler) patchHook(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "change hooks") {
		return
	}
	id, ok := hookID(w, r)
	if !ok {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&raw); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(raw) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, "provide at least one field")
		return
	}
	var in execution.HookIn
	if v, ok := raw["name"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			writeErr(w, http.StatusBadRequest, "name: string")
			return
		}
		in.Name = &s
	}
	if v, ok := raw["enabled"]; ok {
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			writeErr(w, http.StatusBadRequest, "enabled: bool")
			return
		}
		in.Enabled = &b
	}
	if v, ok := raw["events"]; ok {
		var ev []string
		if err := json.Unmarshal(v, &ev); err != nil {
			writeErr(w, http.StatusBadRequest, "events: string array")
			return
		}
		in.Events = ev
	}
	if v, ok := raw["script_id"]; ok {
		var id int64
		if err := json.Unmarshal(v, &id); err != nil || id < 1 {
			writeErr(w, http.StatusBadRequest, "script_id: positive integer")
			return
		}
		in.ScriptID = &id
	}
	if v, ok := raw["quest_id"]; ok {
		if string(v) == "null" {
			in.ClearQuest = true
		} else {
			var qid int64
			if err := json.Unmarshal(v, &qid); err != nil {
				writeErr(w, http.StatusBadRequest, "quest_id: integer or null")
				return
			}
			in.QuestID = &qid
		}
	}
	if v, ok := raw["run_on"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			writeErr(w, http.StatusBadRequest, "run_on: string")
			return
		}
		in.RunOn = &s
	}
	if v, ok := raw["timeout_sec"]; ok {
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			writeErr(w, http.StatusBadRequest, "timeout_sec: integer")
			return
		}
		in.TimeoutSec = &n
	}
	if v, ok := raw["scope_kind"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			writeErr(w, http.StatusBadRequest, "scope_kind: string")
			return
		}
		in.ScopeKind = &s
		if v, ok := raw["scope_id"]; ok && string(v) != "null" {
			var n int64
			if err := json.Unmarshal(v, &n); err != nil {
				writeErr(w, http.StatusBadRequest, "scope_id: integer or null")
				return
			}
			in.ScopeID = &n
		}
	}
	for field, dst := range map[string]**int{"cooldown_sec": &in.CooldownSec, "max_per_hour": &in.MaxPerHour} {
		if v, ok := raw[field]; ok {
			var n int
			if err := json.Unmarshal(v, &n); err != nil {
				writeErr(w, http.StatusBadRequest, field+": integer")
				return
			}
			*dst = &n
		}
	}
	if v, ok := raw["include_private"]; ok {
		var b bool
		if err := json.Unmarshal(v, &b); err != nil {
			writeErr(w, http.StatusBadRequest, "include_private: bool")
			return
		}
		in.IncludePrivate = &b
	}
	hk, err := h.Store.UpdateHook(r.Context(), id, in)
	if err != nil {
		hookErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hk)
}

func (h *Handler) deleteHook(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "delete hooks") {
		return
	}
	id, ok := hookID(w, r)
	if !ok {
		return
	}
	if err := h.Store.DeleteHook(r.Context(), id); err != nil {
		hookErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
