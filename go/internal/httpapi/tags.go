package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/valirum/quests/go/internal/journal"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/store"
)

func (s *Server) registerTags(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tags", s.listTags)
	mux.HandleFunc("POST /api/tags", s.createTag)
	mux.HandleFunc("GET /api/tags/{id}", s.getTag)
	mux.HandleFunc("PATCH /api/tags/{id}", s.patchTag)
	mux.HandleFunc("DELETE /api/tags/{id}", s.deleteTag)
	mux.HandleFunc("PUT /api/quests/{id}/tags", s.putQuestTags)
	mux.HandleFunc("PUT /api/templates/{id}/tags", s.putTemplateTags)
}

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.Journal().ListTags(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) getTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	t, err := s.Store.Journal().GetTag(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Tag not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Slug  string `json:"slug"`
		Label string `json:"label"`
		Color string `json:"color"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	t, err := s.Store.Journal().CreateTag(r.Context(), journal.TagCreate{
		Slug:  body.Slug,
		Label: body.Label,
		Color: body.Color,
	})
	if errors.Is(err, journal.ErrBadSlug) {
		writeErr(w, 422, "slug or label required")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		// Return existing tag with 409 so clients can attach it.
		writeJSON(w, 409, t)
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.publishSilent(r.Context(), "tag_created", t.Label, map[string]any{"tag_id": t.ID, "slug": t.Slug})
	writeJSON(w, 201, t)
}

func (s *Server) patchTag(w http.ResponseWriter, r *http.Request) {
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
	t, err := s.Store.Journal().UpdateTag(r.Context(), id, body.Label, body.Color)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Tag not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Label/color changes show on quest rows — wake journal tabs.
	s.publishSilent(r.Context(), "tag_updated", t.Label, map[string]any{"tag_id": t.ID, "slug": t.Slug})
	writeJSON(w, 200, t)
}

func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	t, err := s.Store.Journal().GetTag(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Tag not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.Store.Journal().DeleteTag(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "Tag not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	s.publishSilent(r.Context(), "tag_deleted", t.Label, map[string]any{"tag_id": id, "slug": t.Slug})
	w.WriteHeader(204)
}

func (s *Server) putQuestTags(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	q, err := s.Store.Journal().GetQuest(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Quest not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ids, err := s.decodeTagIDs(r)
	if err != nil {
		writeTagErr(w, err)
		return
	}
	if err := s.Store.Journal().SetQuestTags(r.Context(), id, ids); err != nil {
		writeTagErr(w, err)
		return
	}
	q, err = s.Store.Journal().GetQuest(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	silent := ""
	if s.Hub != nil {
		qid := q.ID
		s.Hub.Publish("quest_updated", events.PublishOpts{
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

func (s *Server) putTemplateTags(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid id")
		return
	}
	if _, err := s.Store.GetTemplate(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ids, err := s.decodeTagIDs(r)
	if err != nil {
		writeTagErr(w, err)
		return
	}
	if err := s.Store.Journal().SetTemplateTags(r.Context(), id, ids); err != nil {
		writeTagErr(w, err)
		return
	}
	row, err := s.Store.GetTemplate(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.publishSilent(r.Context(), "template_updated", row.Title, map[string]any{"template_id": id})
	writeJSON(w, 200, row.Tags)
}

func (s *Server) decodeTagIDs(r *http.Request) ([]int64, error) {
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
		return s.Store.Journal().ResolveTagIDs(r.Context(), body.Tags)
	}
	return []int64{}, nil
}

func writeTagErr(w http.ResponseWriter, err error) {
	if errors.Is(err, journal.ErrTooManyTags) {
		writeErr(w, 422, err.Error())
		return
	}
	if errors.Is(err, store.ErrNotFound) {
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
