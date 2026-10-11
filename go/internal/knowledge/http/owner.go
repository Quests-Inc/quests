package knowledgehttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
)

// postAttachmentOwner re-parents an attachment onto another quest, questline or note.
func (h *Handler) postAttachmentOwner(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	var in struct {
		OwnerType string `json:"owner_type"`
		OwnerID   int64  `json:"owner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if in.OwnerType != ownerQuest && in.OwnerType != ownerQuestline && in.OwnerType != ownerNote {
		writeErr(w, 422, "owner_type must be quest, questline or note")
		return
	}
	ok, err := h.OwnerExists(r.Context(), in.OwnerType, in.OwnerID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !ok {
		writeErr(w, 404, "owner not found")
		return
	}
	before, err := h.Store.GetAttachment(r.Context(), id)
	if errors.Is(err, apperr.ErrNotFound) {
		writeErr(w, 404, "attachment not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	a, err := h.Store.SetAttachmentOwner(r.Context(), id, in.OwnerType, in.OwnerID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.LogAttachment(r.Context(), "attachment_moved", a, fmt.Sprintf("перенесено: %s %d → %s %d", before.OwnerType, before.OwnerID, a.OwnerType, a.OwnerID))
	h.publishSilent(r.Context(), "attachment_updated", a.Filename, map[string]any{
		"attachment_id": a.ID, "owner_type": a.OwnerType, "owner_id": a.OwnerID,
	})
	writeJSON(w, 200, knowledge.AttachmentToRead(a))
}
