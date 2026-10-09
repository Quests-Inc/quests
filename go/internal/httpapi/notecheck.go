package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/timeutil"
)

// A note's check (quest=370): set and trigger. The verdict itself comes from the scheduler.

func (s *Server) registerNoteCheck(mux *http.ServeMux) {
	mux.HandleFunc("PUT /api/notes/{id}/check", s.putNoteCheck)
	mux.HandleFunc("POST /api/notes/{id}/check/run", s.runNoteCheck)
}

// checkView is a note's check as the API shows it.
func checkView(c store.NoteCheck) map[string]any {
	return map[string]any{
		"command":          c.Command,
		"interval_seconds": c.IntervalSeconds,
		"run_on":           c.RunOn,
		"last_run_at":      timeutil.ToUTCISO(c.LastRunAt),
		"outdated":         c.Outdated,
		"outdated_at":      timeutil.ToUTCISO(c.OutdatedAt),
		"reason":           c.Reason,
		"error":            c.Error,
		"error_at":         timeutil.ToUTCISO(c.ErrorAt),
	}
}

// withCheck adds the check state to a note read: the verdict (and why) on every note that has one,
// the full check — command, interval, where it runs — only on the single-note read.
func withCheck(out map[string]any, c store.NoteCheck, full bool) {
	out["outdated"] = c.Outdated
	out["outdated_reason"] = c.Reason
	out["has_check"] = c.Active()
	if full {
		out["check"] = checkView(c)
	}
}

func (s *Server) putNoteCheck(w http.ResponseWriter, r *http.Request) {
	id, n, ok := s.noteForStats(w, r)
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
	c, err := s.Store.SetNoteCheck(r.Context(), id, body.Command, body.IntervalSeconds, body.RunOn)
	switch {
	case err == nil:
	case store.IsBadInput(err):
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	case errors.Is(err, store.ErrCommandOwner):
		writeErr(w, http.StatusForbidden, err.Error())
		return
	case errors.Is(err, store.ErrServerExec):
		writeErr(w, http.StatusForbidden, err.Error())
		return
	default:
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	detail := "проверка актуальности настроена"
	if !c.Active() {
		detail = "проверка актуальности снята"
	}
	s.logNote(r.Context(), "note_updated", n, detail, 0)
	writeJSON(w, http.StatusOK, checkView(c))
}

// runNoteCheck asks for a run at the next scheduler tick (within seconds).
func (s *Server) runNoteCheck(w http.ResponseWriter, r *http.Request) {
	id, _, ok := s.noteForStats(w, r)
	if !ok {
		return
	}
	if err := s.Store.RequestNoteCheckNow(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusConflict, "у заметки нет проверки")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"queued": true})
}
