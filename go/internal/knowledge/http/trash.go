package knowledgehttp

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// The notes in the trash: DELETE /api/notes/{id} puts a note there (see knowledge/trash.go); these routes list it,
// bring a note back and erase it for good.

type trashedNoteJSON struct {
	Kind      string  `json:"kind"`
	ID        int64   `json:"id"`
	Title     string  `json:"title"`
	Size      int     `json:"size"`
	Children  int     `json:"children"`
	DeletedAt string  `json:"deleted_at"`
	DeletedBy *int64  `json:"deleted_by"`
	PurgeAt   *string `json:"purge_at,omitempty"`
}

// TrashRetention is how long an item stays in the trash before it is erased by the scheduler.
const TrashRetention = 30 * 24 * time.Hour

func (h *Handler) listTrashNotes(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListTrashedNotes(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]trashedNoteJSON, 0, len(list))
	for _, t := range list {
		purgeAt := t.DeletedAt.Add(TrashRetention)
		purge := *timeutil.ToUTCISO(&purgeAt)
		out = append(out, trashedNoteJSON{
			Kind: "note", ID: t.ID, Title: t.Title, Size: t.Size, Children: t.Children,
			DeletedAt: *timeutil.ToUTCISO(&t.DeletedAt), DeletedBy: t.DeletedBy, PurgeAt: &purge,
		})
	}
	writeJSON(w, 200, out)
}

func (h *Handler) restoreNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	title, err := h.Store.RestoreNote(r.Context(), id)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note is not in the trash")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.logNote(r.Context(), "note_restored", knowledge.Note{ID: id, Title: title}, "восстановлена", 0)
	n, err := h.Store.GetNote(r.Context(), id)
	if err != nil {
		w.WriteHeader(204)
		return
	}
	writeJSON(w, 200, map[string]any{"id": n.ID, "title": n.Title, "parent_id": n.ParentID})
}

// purgeTrashedNote erases one trashed note, its attachments and its icon file.
func (h *Handler) purgeTrashedNote(r *http.Request, id int64) error {
	icon, err := h.trashedNoteIcon(r, id)
	if err != nil {
		return err
	}
	if err := h.Store.PurgeNote(r.Context(), id); err != nil {
		return err
	}
	h.PurgeAttachments(r.Context(), ownerNote, id)
	if icon != "" {
		_ = os.Remove(filepath.Join(h.DataDir, "note-icons", icon))
	}
	return nil
}

func (h *Handler) trashedNoteIcon(r *http.Request, id int64) (string, error) {
	icon, err := h.Store.TrashedNoteIcon(r.Context(), id)
	if err != nil {
		return "", err
	}
	return icon, nil
}

func (h *Handler) purgeNote(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	err = h.purgeTrashedNote(r, id)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note is not in the trash")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

// emptyTrashNotes erases every trashed note of the workspace.
func (h *Handler) emptyTrashNotes(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListTrashedNotes(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, t := range list {
		if err := h.purgeTrashedNote(r, t.ID); err != nil && !errors.Is(err, knowledge.ErrNotFound) {
			writeErr(w, 500, err.Error())
			return
		}
	}
	w.WriteHeader(204)
}
