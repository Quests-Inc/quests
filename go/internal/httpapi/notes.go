package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/valirum/quests/go/internal/wsctx"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/refs"
	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/timeutil"
)

func (s *Server) registerNotes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/notes", s.listNotes)
	mux.HandleFunc("POST /api/notes", s.createNote)
	mux.HandleFunc("GET /api/notes/{id}", s.getNote)
	mux.HandleFunc("PATCH /api/notes/{id}", s.patchNote)
	mux.HandleFunc("DELETE /api/notes/{id}", s.deleteNote)
	mux.HandleFunc("GET /api/notes/{id}/icon", s.getNoteIcon)
	mux.HandleFunc("POST /api/notes/{id}/icon", s.postNoteIcon)
	mux.HandleFunc("DELETE /api/notes/{id}/icon", s.deleteNoteIcon)
}

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) {
	var f store.NoteFilter
	if v := r.URL.Query().Get("parent_id"); v != "" {
		if strings.EqualFold(v, "null") || v == "-" {
			f.ParentUnset = true
		} else if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.ParentID = &n
		}
	}
	if p := r.URL.Query().Get("pinned"); p != "" {
		v := p == "1" || strings.EqualFold(p, "true")
		f.Pinned = &v
	}
	rows, err := s.Store.ListNotes(r.Context(), f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]store.NoteRead, 0, len(rows))
	for _, n := range rows {
		read := store.NoteToRead(n)
		redactPrivateNote(r, n, read)
		out = append(out, read)
	}
	writeJSON(w, 200, out)
}

// redactPrivateNote replaces description in-place with a placeholder when the
// note is marked private and this request came in via MCP (X-Quests-Source:
// mcp) — see note=82. Every other source (web, cli, telegram, api, ...) keeps
// seeing the real content; only the agent's default tool path is blocked.
func redactPrivateNote(r *http.Request, n store.Note, out store.NoteRead) {
	if !n.IsPrivate {
		return
	}
	if src := requestSource(r); src == nil || *src != "mcp" {
		return
	}
	out["description"] = "[приватная заметка — содержимое скрыто от MCP; попросите пользователя прочитать её самостоятельно, если это правда нужно]"
}

func (s *Server) getNote(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	n, err := s.Store.GetNote(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.notePayloadFiltered(r, n))
}

func (s *Server) createNote(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	title, _ := body["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		writeErr(w, 422, "title required")
		return
	}
	desc, _ := body["description"].(string)
	pinned, _ := body["pinned"].(bool)
	isReadme, _ := body["is_readme"].(bool)
	isPrivate, _ := body["is_private"].(bool)
	isCategory, _ := body["is_category"].(bool)
	sortOrder := 0
	if v, ok := body["sort_order"].(float64); ok {
		sortOrder = int(v)
	}
	n := store.Note{
		Title:       title,
		Description: desc,
		Pinned:      pinned,
		IsReadme:    isReadme,
		IsPrivate:   isPrivate,
		IsCategory:  isCategory,
		SortOrder:   sortOrder,
		CreatedAt:   timeutil.NowUTC(),
		UpdatedAt:   timeutil.NowUTC(),
	}
	if v, ok := body["color"].(string); ok && strings.TrimSpace(v) != "" {
		n.Color = strings.TrimSpace(v)
	}
	if v, ok := body["icon"].(string); ok && strings.TrimSpace(v) != "" {
		n.Icon = strings.TrimSpace(v)
	}
	if _, ok := body["parent_id"]; ok {
		n.ParentID = noteJSONInt64(body["parent_id"])
	}
	if err := s.validateNoteParent(r, 0, n.ParentID); err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	var parent store.Note
	var hasParent bool
	if n.ParentID != nil {
		p, err := s.Store.GetNote(r.Context(), *n.ParentID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		parent, hasParent = p, true
	}
	created, err := s.Store.CreateNote(r.Context(), n)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if hasParent && parent.CustomIcon != nil && *parent.CustomIcon != "" {
		if copied, err := s.Store.CopyNoteCustomIcon(r.Context(), parent.ID, created.ID, s.DataDir); err == nil {
			created = copied
		}
	}
	s.logNote(r.Context(), "note_created", created, "создана", 0)
	writeJSON(w, 201, s.notePayloadFiltered(r, created))
}

func (s *Server) patchNote(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	cur, err := s.Store.GetNote(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	} else if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	_, hasFullDesc := body["description"]
	_, hasInsert := body["description_insert"]
	_, hasReplace := body["description_replace"]
	ops := 0
	if hasFullDesc {
		ops++
	}
	if hasInsert {
		ops++
	}
	if hasReplace {
		ops++
	}
	if ops > 1 {
		writeErr(w, 422, "provide only one of description, description_insert, description_replace")
		return
	}
	if hasInsert {
		op, ok := body["description_insert"].(map[string]any)
		if !ok {
			writeErr(w, 422, "description_insert must be an object")
			return
		}
		text, _ := op["text"].(string)
		var after, before *string
		var at *int
		if v, ok := op["after"].(string); ok {
			after = &v
		}
		if v, ok := op["before"].(string); ok {
			before = &v
		}
		if v, ok := noteJSONInt(op["at"]); ok {
			at = &v
		}
		next, err := applyDescriptionInsert(cur.Description, text, after, before, at)
		if err != nil {
			writeErr(w, 422, err.Error())
			return
		}
		delete(body, "description_insert")
		body["description"] = next
	}
	if hasReplace {
		op, ok := body["description_replace"].(map[string]any)
		if !ok {
			writeErr(w, 422, "description_replace must be an object")
			return
		}
		find, _ := op["find"].(string)
		withText, _ := op["with"].(string)
		all, _ := op["all"].(bool)
		next, err := applyDescriptionReplace(cur.Description, find, withText, all)
		if err != nil {
			writeErr(w, 422, err.Error())
			return
		}
		delete(body, "description_replace")
		body["description"] = next
	}
	if v, ok := body["title"].(string); ok {
		v = strings.TrimSpace(v)
		if v == "" {
			writeErr(w, 422, "title required")
			return
		}
		body["title"] = v
	}
	if _, ok := body["parent_id"]; ok {
		pid := noteJSONInt64(body["parent_id"])
		if err := s.validateNoteParent(r, id, pid); err != nil {
			writeErr(w, 422, err.Error())
			return
		}
		if pid == nil {
			body["parent_id"] = nil
		} else {
			body["parent_id"] = *pid
		}
	}
	updated, err := s.Store.UpdateNote(r.Context(), id, body)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if errors.Is(err, store.ErrNoteCycle) {
		writeErr(w, 422, err.Error())
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.logNote(r.Context(), "note_updated", updated, "изменена", 10*time.Minute)
	writeJSON(w, 200, s.notePayloadFiltered(r, updated))
}

func (s *Server) deleteNote(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	cur, err := s.Store.GetNote(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.purgeOwnerAttachments(r.Context(), ownerNote, id)
	if cur.CustomIcon != nil && *cur.CustomIcon != "" {
		_ = os.Remove(filepath.Join(s.DataDir, "note-icons", *cur.CustomIcon))
	}
	if err := s.Store.DeleteNote(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.logNote(r.Context(), "note_deleted", cur, "удалена", 0)
	w.WriteHeader(204)
}

func (s *Server) validateNoteParent(r *http.Request, noteID int64, parentID *int64) error {
	if parentID == nil {
		return nil
	}
	if noteID != 0 && *parentID == noteID {
		return store.ErrNoteCycle
	}
	ok, err := s.Store.NoteExists(r.Context(), *parentID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("parent note not found")
	}
	return nil
}

func noteCustomIcon(n store.Note) string {
	if n.CustomIcon != nil {
		return *n.CustomIcon
	}
	return ""
}

func (s *Server) getNoteIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	n, err := s.Store.GetNote(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	custom := noteCustomIcon(n)
	if custom == "" || !store.SafeIconName(custom) {
		writeErr(w, 404, "Icon not found")
		return
	}
	path := filepath.Join(s.DataDir, "note-icons", custom)
	serveIcon(w, r, path)
}

func (s *Server) postNoteIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := s.Store.GetNote(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err := r.ParseMultipartForm(600 << 10); err != nil {
		writeErr(w, 400, "invalid multipart")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "file required")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 512*1024+1))
	if err != nil || len(raw) == 0 || len(raw) > 512*1024 {
		writeErr(w, 400, "icon too large or empty")
		return
	}
	ct := hdr.Header.Get("Content-Type")
	ext := ".png"
	switch {
	case strings.Contains(ct, "jpeg"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".jpg"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".jpeg"):
		ext = ".jpg"
	case strings.Contains(ct, "webp"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".webp"):
		ext = ".webp"
	case strings.Contains(ct, "gif"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".gif"):
		ext = ".gif"
	case strings.Contains(ct, "svg"), strings.HasSuffix(strings.ToLower(hdr.Filename), ".svg"):
		ext = ".svg"
	}
	dir := filepath.Join(s.DataDir, "note-icons")
	_ = os.MkdirAll(dir, 0o755)
	name := strconv.FormatInt(id, 10) + ext
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	updated, err := s.Store.SetNoteIcon(r.Context(), id, name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.logNote(r.Context(), "note_updated", updated, "изменена", 10*time.Minute)
	writeJSON(w, 200, s.notePayloadFiltered(r, updated))
}

func (s *Server) deleteNoteIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	updated, err := s.Store.ClearNoteIcon(r.Context(), id, s.DataDir)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.logNote(r.Context(), "note_updated", updated, "изменена", 10*time.Minute)
	writeJSON(w, 200, s.notePayloadFiltered(r, updated))
}

// notePayloadFiltered is notePayload with private-note redaction applied —
// use this everywhere a note goes back in a response, not the bare notePayload.
func (s *Server) notePayloadFiltered(r *http.Request, n store.Note) map[string]any {
	out := s.notePayload(r, n)
	redactPrivateNote(r, n, store.NoteRead(out))
	return out
}

func (s *Server) notePayload(r *http.Request, n store.Note) map[string]any {
	out := store.NoteToRead(n)
	children, _ := s.Store.ListNotes(r.Context(), store.NoteFilter{ParentID: &n.ID})
	childReads := make([]store.NoteRead, 0, len(children))
	for _, c := range children {
		childReads = append(childReads, store.NoteBrief(c))
	}
	out["children"] = childReads
	out["refs"] = s.resolveRefs(r, refs.Parse(n.Title+"\n"+n.Description))
	out["backlinks"] = s.backlinksTo(r, "note", n.ID)
	return out
}

func (s *Server) resolveRefs(r *http.Request, list []refs.Ref) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, ref := range list {
		row := map[string]any{"kind": ref.Kind, "id": ref.ID, "title": nil}
		switch ref.Kind {
		case "note":
			n, err := s.Store.GetNote(r.Context(), ref.ID)
			if err == nil {
				row["title"] = n.Title
			}
		case "quest":
			q, err := s.Store.GetQuest(r.Context(), ref.ID)
			if err == nil {
				row["title"] = q.Title
			}
		case "questline":
			ql, err := s.Store.GetQuestline(r.Context(), ref.ID)
			if err == nil {
				if t, ok := ql["title"].(string); ok {
					row["title"] = t
				}
			}
		case "step":
			qid, err := s.Store.QuestIDForStep(r.Context(), ref.ID)
			if err == nil {
				q, err := s.Store.GetQuest(r.Context(), qid)
				if err == nil {
					for _, st := range q.Steps {
						if st.ID == ref.ID {
							row["title"] = st.Title
							row["quest_id"] = q.ID
							break
						}
					}
				}
			}
		case "template":
			t, err := s.Store.GetTemplate(r.Context(), ref.ID)
			if err == nil {
				if title, ok := t["title"].(string); ok {
					row["title"] = title
				}
			}
		case "attachment":
			a, err := s.Store.GetAttachment(r.Context(), ref.ID)
			if err == nil {
				row["title"] = a.Filename
				row["owner_type"] = a.OwnerType
				row["owner_id"] = a.OwnerID
			}
		}
		out = append(out, row)
	}
	return out
}

func (s *Server) backlinksTo(r *http.Request, kind string, id int64) []map[string]any {
	corpus, err := s.Store.ListLinkCorpus(r.Context())
	if err != nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0)
	for _, doc := range corpus {
		if doc.Kind == kind && doc.ID == id {
			continue
		}
		found := false
		for _, ref := range refs.Parse(doc.Body) {
			if ref.Kind == kind && ref.ID == id {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		out = append(out, map[string]any{
			"kind":  doc.Kind,
			"id":    doc.ID,
			"title": doc.Title,
		})
	}
	return out
}

func (s *Server) linkedNotesFromText(r *http.Request, texts ...string) []map[string]any {
	var blob strings.Builder
	for _, t := range texts {
		blob.WriteString(t)
		blob.WriteByte('\n')
	}
	noteRefs := refs.FilterKind(refs.Parse(blob.String()), "note")
	return s.resolveRefs(r, noteRefs)
}

// logNote records the note event in the change log (kind names follow the
// hub's note_* events) and publishes it live.
func (s *Server) logNote(ctx context.Context, kind string, n store.Note, detail string, coalesce time.Duration) {
	_ = s.Store.LogEvent(context.WithoutCancel(ctx), store.ChangeEvent{
		Kind: kind, EntityType: "note", EntityID: n.ID, Title: n.Title, Detail: detail, Coalesce: coalesce,
	})
	s.publishNote(ctx, kind, n)
}

func (s *Server) publishNote(ctx context.Context, kind string, n store.Note) {
	if s.Hub == nil {
		return
	}
	silent := ""
	s.Hub.Publish(kind, events.PublishOpts{
		Workspace:   wsctx.From(ctx),
		Title:       n.Title,
		Description: n.Description,
		Detail:      "",
		Toast:       false,
		Source:      "api",
		Sound:       &silent,
		Extra:       map[string]any{"note_id": n.ID},
	})
}

// publishSilent wakes journal tabs (they reload on any quests_changed) without
// a HUD toast or sound. Same contract as note events.
func (s *Server) publishSilent(ctx context.Context, kind, title string, extra map[string]any) {
	if s.Hub == nil {
		return
	}
	silent := ""
	s.Hub.Publish(kind, events.PublishOpts{
		Workspace: wsctx.From(ctx),
		Title:     title,
		Toast:     false,
		Source:    "api",
		Sound:     &silent,
		Extra:     extra,
	})
}

func noteJSONInt64(v any) *int64 {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case float64:
		n := int64(t)
		return &n
	case int64:
		return &t
	case int:
		n := int64(t)
		return &n
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return nil
		}
		return &n
	default:
		return nil
	}
}

func noteJSONInt(v any) (int, bool) {
	p := noteJSONInt64(v)
	if p == nil {
		return 0, false
	}
	return int(*p), true
}
