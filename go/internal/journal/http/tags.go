package journalhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func (h *Handler) listTags(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Store.ListTags(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

func (h *Handler) getTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	t, err := h.Store.GetTag(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Tag not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, t)
}

func (h *Handler) createTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Slug  string `json:"slug"`
		Label string `json:"label"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	t, err := h.Store.CreateTag(r.Context(), journal.TagCreate{
		Slug:  body.Slug,
		Label: body.Label,
		Color: body.Color,
	})
	if errors.Is(err, journal.ErrBadSlug) {
		writeErr(w, 422, "slug or label required")
		return
	}
	if errors.Is(err, apperr.ErrConflict) {
		// Return existing tag with 409 so clients can attach it.
		writeJSON(w, 409, t)
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.publishSilent(r.Context(), "tag_created", t.Label, map[string]any{"tag_id": t.ID, "slug": t.Slug})
	writeJSON(w, 201, t)
}

func (h *Handler) patchTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	var body struct {
		Label *string `json:"label"`
		Color *string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	t, err := h.Store.UpdateTag(r.Context(), id, body.Label, body.Color)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Tag not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Label/color changes show on quest rows — wake journal tabs.
	h.publishSilent(r.Context(), "tag_updated", t.Label, map[string]any{"tag_id": t.ID, "slug": t.Slug})
	writeJSON(w, 200, t)
}

func (h *Handler) deleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	t, err := h.Store.GetTag(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Tag not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := h.Store.DeleteTag(r.Context(), id); err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			writeErr(w, 404, "Tag not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	h.publishSilent(r.Context(), "tag_deleted", t.Label, map[string]any{"tag_id": id, "slug": t.Slug})
	w.WriteHeader(204)
}

func (h *Handler) putQuestTags(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	q, err := h.Store.GetQuest(r.Context(), id)
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Quest not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ids, err := h.decodeTagIDs(r)
	if err != nil {
		writeTagErr(w, err)
		return
	}
	if err := h.Store.SetQuestTags(r.Context(), id, ids); err != nil {
		writeTagErr(w, err)
		return
	}
	q, err = h.Store.GetQuest(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	silent := ""
	if h.Hub != nil {
		qid := q.ID
		h.Hub.Publish("quest_updated", events.PublishOpts{
			Workspace: wsctx.From(r.Context()),
			QuestID:   &qid,
			Title:     q.Title,
			Detail:    "теги",
			Toast:     false,
			Source:    "api",
			Sound:     &silent,
		})
	}
	writeJSON(w, 200, q.Tags)
}

func (h *Handler) putTemplateTags(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	if _, err := h.Template(r.Context(), id); errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ids, err := h.decodeTagIDs(r)
	if err != nil {
		writeTagErr(w, err)
		return
	}
	if err := h.Store.SetTemplateTags(r.Context(), id, ids); err != nil {
		writeTagErr(w, err)
		return
	}
	row, err := h.Template(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.publishSilent(r.Context(), "template_updated", row.Title, map[string]any{"template_id": id})
	writeJSON(w, 200, row.Tags)
}

func (h *Handler) decodeTagIDs(r *http.Request) ([]int64, error) {
	var body struct {
		TagIDs []int64  `json:"tag_ids"`
		Tags   []string `json:"tags"` // slugs or ids as strings
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, err
	}
	if body.TagIDs != nil {
		return body.TagIDs, nil
	}
	if body.Tags != nil {
		return h.Store.ResolveTagIDs(r.Context(), body.Tags)
	}
	return []int64{}, nil
}

func writeTagErr(w http.ResponseWriter, err error) {
	if errors.Is(err, journal.ErrTooManyTags) {
		writeErr(w, 422, err.Error())
		return
	}
	if errors.Is(err, journal.ErrNotFound) {
		writeErr(w, 404, "Tag not found")
		return
	}
	if errors.Is(err, journal.ErrBadSlug) {
		writeErr(w, 422, err.Error())
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "invalid JSON") {
		writeErr(w, 400, msg)
		return
	}
	writeErr(w, 500, msg)
}
