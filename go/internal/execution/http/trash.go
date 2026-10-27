package executionhttp

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// TrashRetention is how long a script or a hook stays in the trash before the scheduler erases it (as quests and notes).
const TrashRetention = 30 * 24 * time.Hour

// The scripts and hooks in the trash: DELETE /api/scripts/{id} and DELETE /api/hooks/{id} put them there
// (execution/trash.go); these routes list the trash, bring an item back and erase it for good. Owners only, like writing.
func (h *Handler) registerTrash(mux *http.ServeMux) {
	for _, kind := range []string{"script", "hook"} {
		seg := kind + "s"
		mux.HandleFunc("GET /api/trash/"+seg, h.listTrash(kind))
		mux.HandleFunc("POST /api/trash/"+seg+"/{id}/restore", h.restoreTrash(kind))
		mux.HandleFunc("DELETE /api/trash/"+seg+"/{id}", h.purgeTrash(kind))
		mux.HandleFunc("DELETE /api/trash/"+seg, h.emptyTrash(kind))
	}
}

type trashedJSON struct {
	Kind      string `json:"kind"`
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	DeletedAt string `json:"deleted_at"`
	DeletedBy *int64 `json:"deleted_by"`
	PurgeAt   string `json:"purge_at"`
	// ScriptID is a hook's script; ScriptTrashed says it is in the trash too (restore the script first).
	ScriptID      int64 `json:"script_id,omitempty"`
	ScriptTrashed bool  `json:"script_trashed,omitempty"`
}

func (h *Handler) listTrash(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := h.Store.ListTrashed(r.Context(), kind)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		var trashedScripts map[int64]bool
		if kind == "hook" {
			trashedScripts = map[int64]bool{}
			if ss, err := h.Store.ListTrashed(r.Context(), "script"); err == nil {
				for _, s := range ss {
					trashedScripts[s.ID] = true
				}
			}
		}
		out := make([]trashedJSON, 0, len(list))
		for _, t := range list {
			purge := t.DeletedAt.Add(TrashRetention)
			out = append(out, trashedJSON{
				Kind: kind, ID: t.ID, Title: t.Title, DeletedAt: *timeutil.ToUTCISO(&t.DeletedAt), DeletedBy: t.DeletedBy,
				PurgeAt: *timeutil.ToUTCISO(&purge), ScriptID: t.ScriptID, ScriptTrashed: trashedScripts[t.ScriptID],
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func trashID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func notInTrash(kind string) string {
	if kind == "hook" {
		return "хука нет в корзине"
	}
	return "скрипта нет в корзине"
}

func (h *Handler) restoreTrash(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.RequireOwner(w, r, "restore "+kind+"s") {
			return
		}
		id, ok := trashID(w, r)
		if !ok {
			return
		}
		title, err := h.Store.RestoreTrashed(r.Context(), kind, id)
		switch {
		case errors.Is(err, execution.ErrNotFound):
			writeErr(w, http.StatusNotFound, notInTrash(kind))
		case errors.Is(err, execution.ErrScriptTrashed):
			writeErr(w, http.StatusConflict, err.Error())
		case err != nil:
			writeErr(w, http.StatusInternalServerError, err.Error())
		default:
			writeJSON(w, http.StatusOK, map[string]any{"id": id, "title": title})
		}
	}
}

func (h *Handler) purgeTrash(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.RequireOwner(w, r, "erase "+kind+"s") {
			return
		}
		id, ok := trashID(w, r)
		if !ok {
			return
		}
		err := h.Store.PurgeTrashed(r.Context(), kind, id)
		switch {
		case errors.Is(err, execution.ErrNotFound):
			writeErr(w, http.StatusNotFound, notInTrash(kind))
		case err != nil:
			writeErr(w, http.StatusInternalServerError, err.Error())
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (h *Handler) emptyTrash(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.RequireOwner(w, r, "erase "+kind+"s") {
			return
		}
		list, err := h.Store.ListTrashed(r.Context(), kind)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, t := range list {
			if err := h.Store.PurgeTrashed(r.Context(), kind, t.ID); err != nil && !errors.Is(err, execution.ErrNotFound) {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
