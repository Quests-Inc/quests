package knowledgehttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// A note's check (quest=370): set and trigger. The verdict itself comes from the scheduler.

func (h *Handler) RegisterNoteCheck(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/notes/{id}/check", h.putNoteCheck)
	mux.HandleFunc("POST /api/notes/{id}/check/run", h.runNoteCheck)
	mux.HandleFunc("PUT /api/notes/{id}/outdated", h.putNoteOutdated)
}

func checkView(c knowledge.NoteCheck) NoteCheckView {
	return NoteCheckView{
		Command: c.Command, IntervalSeconds: c.IntervalSeconds, RunOn: c.RunOn,
		LastRunAt: timeutil.ToUTCISO(c.LastRunAt), Outdated: c.Outdated,
		OutdatedAt: timeutil.ToUTCISO(c.OutdatedAt), Reason: c.Reason, Error: c.Error,
		ErrorAt: timeutil.ToUTCISO(c.ErrorAt),
	}
}

func (h *Handler) putNoteCheck(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	var body struct {
		Command         *string `json:"command"`
		IntervalSeconds *int    `json:"interval_seconds"`
		RunOn           *string `json:"run_on"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	c, err := h.Store.SetNoteCheck(r.Context(), id, body.Command, body.IntervalSeconds, body.RunOn)
	switch {
	case err == nil:
	case apperr.IsBad(err):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	case errors.Is(err, kernel.ErrCommandOwner):
		writeErr(w, http.StatusForbidden, err.Error())
		return
	case errors.Is(err, kernel.ErrNoRunner):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	detail := "проверка актуальности настроена"
	if !c.Active() {
		detail = "проверка актуальности снята"
	}
	h.logNote(r.Context(), "note_updated", n, detail, 0)
	writeJSON(w, http.StatusOK, checkView(c))
}

// runNoteCheck asks for a run at the next scheduler tick (within seconds).
func (h *Handler) runNoteCheck(w http.ResponseWriter, r *http.Request) {
	id, _, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	if err := h.Store.RequestNoteCheckNow(r.Context(), id); errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, http.StatusConflict, "у заметки нет проверки")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true})
}

// putNoteOutdated sets or clears the OUTDATED flag by hand: {"outdated": true|false, "reason": "…"}.
func (h *Handler) putNoteOutdated(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	var body struct {
		Outdated *bool  `json:"outdated"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil || body.Outdated == nil {
		writeErr(w, http.StatusBadRequest, `body: {"outdated": true|false, "reason": "…"}`)
		return
	}
	reason := body.Reason
	if *body.Outdated && reason == "" {
		reason = "Отмечена вручную"
	}
	if err := h.Store.SetNoteOutdated(r.Context(), id, *body.Outdated, reason, time.Now().UTC()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	detail := "заметка помечена устаревшей"
	if !*body.Outdated {
		detail = "пометка «устарела» снята"
	}
	h.logNote(r.Context(), "note_updated", n, detail, 0)
	writeJSON(w, http.StatusOK, map[string]any{"outdated": *body.Outdated, "reason": reason})
}
