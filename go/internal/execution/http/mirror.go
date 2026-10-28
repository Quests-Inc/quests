package executionhttp

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// A step that follows a quest (quest=298): read it, set it, stop it. Whoever may edit steps may do this: the hook and the
// script it makes have a fixed shape, no command and no runner.
func (h *Handler) registerMirror(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/steps/{id}/mirror", h.getMirror)
	mux.HandleFunc("PUT /api/steps/{id}/mirror", h.putMirror)
	mux.HandleFunc("DELETE /api/steps/{id}/mirror", h.deleteMirror)
}

func mirrorStepID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func (h *Handler) getMirror(w http.ResponseWriter, r *http.Request) {
	id, ok := mirrorStepID(w, r)
	if !ok {
		return
	}
	m, err := h.Store.StepMirror(r.Context(), id)
	if err != nil {
		hookErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) putMirror(w http.ResponseWriter, r *http.Request) {
	id, ok := mirrorStepID(w, r)
	if !ok {
		return
	}
	var in struct {
		QuestID int64 `json:"quest_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&in); err != nil || in.QuestID < 1 {
		writeErr(w, http.StatusBadRequest, "quest_id: номер квеста")
		return
	}
	m, err := h.Store.SetStepMirror(r.Context(), id, in.QuestID)
	if err != nil {
		hookErr(w, err)
		return
	}
	// A step that follows has no check of its own, and takes the quest's progress now rather than at its next change.
	if h.ClearCommand != nil {
		_ = h.ClearCommand(r.Context(), "step", id)
	}
	if h.MirrorChanged != nil {
		h.MirrorChanged(r.Context(), in.QuestID)
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) deleteMirror(w http.ResponseWriter, r *http.Request) {
	id, ok := mirrorStepID(w, r)
	if !ok {
		return
	}
	if err := h.Store.ClearStepMirror(r.Context(), id); err != nil {
		hookErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
