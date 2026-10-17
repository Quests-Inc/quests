package journalhttp

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// The quests in the trash: DELETE /api/quests/{id} puts a quest there (see journal/trash.go); these routes list it,
// bring a quest back and erase it for good.

type trashedQuestJSON struct {
	Kind        string  `json:"kind"`
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Status      string  `json:"status"`
	QuestlineID *int64  `json:"questline_id"`
	StepsTotal  int     `json:"steps_total"`
	StepsDone   int     `json:"steps_done"`
	DeletedAt   string  `json:"deleted_at"`
	DeletedBy   *int64  `json:"deleted_by"`
	PurgeAt     *string `json:"purge_at,omitempty"`
}

// TrashRetention is how long an item stays in the trash before it is erased by the scheduler.
const TrashRetention = 30 * 24 * time.Hour

func (h *Handler) listTrashQuests(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListTrashedQuests(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]trashedQuestJSON, 0, len(list))
	for _, t := range list {
		purgeAt := t.DeletedAt.Add(TrashRetention)
		purge := *timeutil.ToUTCISO(&purgeAt)
		out = append(out, trashedQuestJSON{
			Kind: "quest", ID: t.ID, Title: t.Title, Status: t.Status, QuestlineID: t.QuestlineID,
			StepsTotal: t.StepsTotal, StepsDone: t.StepsDone, DeletedAt: *timeutil.ToUTCISO(&t.DeletedAt), DeletedBy: t.DeletedBy,
			PurgeAt: &purge,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) restoreQuest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	title, err := h.Store.RestoreQuest(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest is not in the trash")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	quiet, source := publishOpts(r)
	h.Hub.Publish("quest_restored", events.PublishOpts{
		Workspace: wsctx.From(r.Context()),
		QuestID:   &id, Title: title, Detail: "восстановлено", Toast: !quiet, Source: source,
	})
	q, err := h.Store.GetQuest(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, journal.ToQuestRead(q, timeutil.NowUTC()))
}

// purgeTrashedQuest erases one trashed quest and its attachments.
func (h *Handler) purgeTrashedQuest(r *http.Request, id int64) error {
	if err := h.Store.PurgeQuest(r.Context(), id); err != nil {
		return err
	}
	h.PurgeAttachments(r.Context(), ownerQuest, id)
	return nil
}

func (h *Handler) purgeQuest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	err = h.purgeTrashedQuest(r, id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "Quest is not in the trash")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// emptyTrashQuests erases every trashed quest of the workspace.
func (h *Handler) emptyTrashQuests(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListTrashedQuests(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, t := range list {
		if err := h.purgeTrashedQuest(r, t.ID); err != nil && !errors.Is(err, journal.ErrNotFound) {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
