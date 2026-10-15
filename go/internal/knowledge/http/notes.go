package knowledgehttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/refs"
)

func (h *Handler) RegisterNotes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/notes", h.listNotes)
	mux.HandleFunc("POST /api/notes", h.createNote)
	mux.HandleFunc("GET /api/notes/search", h.searchNotes)
	mux.HandleFunc("GET /api/notes/{id}/search", h.searchInNote)
	mux.HandleFunc("POST /api/notes/{id}/feedback", h.noteFeedback)
	mux.HandleFunc("PUT /api/notes/{id}/weight", h.noteWeight)
	mux.HandleFunc("DELETE /api/notes/{id}/stats", h.resetNoteStats)
	mux.HandleFunc("GET /api/notes/{id}", h.getNote)
	mux.HandleFunc("PATCH /api/notes/{id}", h.patchNote)
	mux.HandleFunc("DELETE /api/notes/{id}", h.deleteNote)
	mux.HandleFunc("GET /api/notes/{id}/icon", h.getNoteIcon)
	mux.HandleFunc("POST /api/notes/{id}/icon", h.postNoteIcon)
	mux.HandleFunc("DELETE /api/notes/{id}/icon", h.deleteNoteIcon)
}

func (h *Handler) listNotes(w http.ResponseWriter, r *http.Request) {
	var f knowledge.NoteFilter
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
	rows, err := h.Store.ListNotes(r.Context(), f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if v := r.URL.Query().Get("category_id"); v != "" {
		rows, err = h.notesUnderCategory(r, rows, v)
		if err != nil {
			var he httpError
			if errors.As(err, &he) {
				writeErr(w, he.code, he.msg)
			} else {
				writeErr(w, 500, err.Error())
			}
			return
		}
	}
	checks, _ := h.Store.NoteChecks(r.Context())
	out := make([]NoteListItem, 0, len(rows))
	for _, n := range rows {
		read := knowledge.NoteToRead(n)
		redactPrivateNote(r, n, &read)
		out = append(out, NoteListItem{NoteRead: read, NoteVerdict: verdictOf(checks[n.ID])})
	}
	if fl := r.URL.Query().Get("flags"); fl != "" {
		kept, ferr := filterNoteFlags(out, fl)
		if ferr != nil {
			writeErr(w, 422, ferr.Error())
			return
		}
		out = kept
	}
	if r.URL.Query().Get("sort") == "popular" {
		// Reading history: decayed score first, then the boost (newness, manual weight); explained per note.
		born := make(map[int64]time.Time, len(rows))
		for _, n := range rows {
			born[n.ID] = n.CreatedAt
		}
		pop, perr := h.Store.Popularity(r.Context(), born)
		if perr != nil {
			writeErr(w, 500, perr.Error())
			return
		}
		for i := range out {
			p := pop[out[i].ID]
			out[i].Popularity = &p
		}
		sort.SliceStable(out, func(i, j int) bool {
			pi, pj := out[i].Popularity, out[j].Popularity
			if pi.Score != pj.Score {
				return pi.Score > pj.Score
			}
			return pi.Boost > pj.Boost
		})
	}
	if q := r.URL.Query(); q.Get("brief") == "1" || strings.EqualFold(q.Get("brief"), "true") {
		limit, _ := strconv.Atoi(q.Get("limit"))
		page, perr := briefNotePage(out, limit, q.Get("cursor"))
		if perr != nil {
			writeErr(w, 400, perr.Error())
			return
		}
		writeJSON(w, 200, page)
		return
	}
	writeJSON(w, 200, out)
}

func verdictOf(c knowledge.NoteCheck) NoteVerdict {
	return NoteVerdict{Outdated: c.Outdated, OutdatedReason: c.Reason, HasCheck: c.Active()}
}

// redactPrivateNote replaces description in-place with a placeholder when the
// note is marked private and this request came in via MCP (X-Quests-Source:
// mcp) — see note=82. Every other source (web, cli, telegram, api, ...) keeps
// seeing the real content; only the agent's default tool path is blocked.
func redactPrivateNote(r *http.Request, n knowledge.Note, out *knowledge.NoteRead) {
	if !n.IsPrivate {
		return
	}
	if src := requestSource(r); src == nil || *src != "mcp" {
		return
	}
	out.Description = "[приватная заметка — содержимое скрыто от MCP; попросите пользователя прочитать её самостоятельно, если это правда нужно]"
}

func (h *Handler) getNote(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	n, err := h.Store.GetNote(r.Context(), id)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.countRead(r, n)
	writeJSON(w, 200, h.NotePayloadFiltered(r, n))
}

func (h *Handler) createNote(w http.ResponseWriter, r *http.Request) {
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
	n := knowledge.Note{
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
	if err := h.validateNoteParent(r, 0, n.ParentID); err != nil {
		writeErr(w, 422, err.Error())
		return
	}
	var parent knowledge.Note
	var hasParent bool
	if n.ParentID != nil {
		p, err := h.Store.GetNote(r.Context(), *n.ParentID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		parent, hasParent = p, true
	}
	created, err := h.Store.CreateNote(r.Context(), n)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if hasParent && parent.CustomIcon != nil && *parent.CustomIcon != "" {
		if copied, err := h.Store.CopyNoteCustomIcon(r.Context(), parent.ID, created.ID, h.DataDir); err == nil {
			created = copied
		}
	}
	h.logNote(r.Context(), "note_created", created, "создана", 0)
	writeJSON(w, 201, h.NotePayloadFiltered(r, created))
}

func (h *Handler) patchNote(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	cur, err := h.Store.GetNote(r.Context(), id)
	if errors.Is(err, knowledge.ErrNotFound) {
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
		if err := h.validateNoteParent(r, id, pid); err != nil {
			writeErr(w, 422, err.Error())
			return
		}
		if pid == nil {
			body["parent_id"] = nil
		} else {
			body["parent_id"] = *pid
		}
	}
	updated, err := h.Store.UpdateNote(r.Context(), id, body)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if errors.Is(err, knowledge.ErrNoteCycle) {
		writeErr(w, 422, err.Error())
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.logNote(r.Context(), "note_updated", updated, "изменена", 10*time.Minute)
	writeJSON(w, 200, h.NotePayloadFiltered(r, updated))
}

func (h *Handler) deleteNote(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	cur, err := h.Store.GetNote(r.Context(), id)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.PurgeAttachments(r.Context(), ownerNote, id)
	if cur.CustomIcon != nil && *cur.CustomIcon != "" {
		_ = os.Remove(filepath.Join(h.DataDir, "note-icons", *cur.CustomIcon))
	}
	if err := h.Store.DeleteNote(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.logNote(r.Context(), "note_deleted", cur, "удалена", 0)
	w.WriteHeader(204)
}

func (h *Handler) validateNoteParent(r *http.Request, noteID int64, parentID *int64) error {
	if parentID == nil {
		return nil
	}
	if noteID != 0 && *parentID == noteID {
		return knowledge.ErrNoteCycle
	}
	ok, err := h.Store.NoteExists(r.Context(), *parentID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("parent note not found")
	}
	return nil
}

// NotePayloadFiltered is notePayload with private-note redaction applied —
// use this everywhere a note goes back in a response, not the bare notePayload.
func (h *Handler) NotePayloadFiltered(r *http.Request, n knowledge.Note) NotePayload {
	out := h.notePayload(r, n)
	redactPrivateNote(r, n, &out.NoteRead)
	return out
}

func (h *Handler) notePayload(r *http.Request, n knowledge.Note) NotePayload {
	children, _ := h.Store.ListNotes(r.Context(), knowledge.NoteFilter{ParentID: &n.ID})
	childReads := make([]knowledge.NoteBriefRead, 0, len(children))
	for _, c := range children {
		childReads = append(childReads, knowledge.NoteBrief(c))
	}
	c, _ := h.Store.GetNoteCheck(r.Context(), n.ID)
	return NotePayload{
		NoteListItem: NoteListItem{NoteRead: knowledge.NoteToRead(n), NoteVerdict: verdictOf(c)},
		Children:     childReads,
		Refs:         h.ResolveRefs(r, refs.Parse(n.Title+"\n"+n.Description)),
		Backlinks:    h.backlinksTo(r, "note", n.ID),
		Check:        checkView(c),
	}
}

func (h *Handler) backlinksTo(r *http.Request, kind string, id int64) []Backlink {
	corpus, err := h.Store.ListLinkCorpus(r.Context())
	if err != nil {
		return []Backlink{}
	}
	out := make([]Backlink, 0)
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
		out = append(out, Backlink{Kind: doc.Kind, ID: doc.ID, Title: doc.Title})
	}
	return out
}

// logNote records the note event in the change log (kind names follow the
// hub's note_* events) and publishes it live.
func (h *Handler) logNote(ctx context.Context, kind string, n knowledge.Note, detail string, coalesce time.Duration) {
	h.LogEvent(context.WithoutCancel(ctx), ChangeEvent{
		Kind: kind, EntityType: "note", EntityID: n.ID, Title: n.Title, Detail: detail, Coalesce: coalesce,
	})
	h.publishNote(ctx, kind, n)
}

func (h *Handler) publishNote(ctx context.Context, kind string, n knowledge.Note) {
	if h.Hub == nil {
		return
	}
	silent := ""
	h.Hub.Publish(kind, events.PublishOpts{
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
