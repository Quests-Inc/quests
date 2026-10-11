package knowledgehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/kernel"
	"github.com/valirum/quests/go/internal/knowledge"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/httpx"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/refs"
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
	checks, _ := h.Store.NoteChecks(r.Context())
	out := make([]NoteListItem, 0, len(rows))
	for _, n := range rows {
		read := knowledge.NoteToRead(n)
		redactPrivateNote(r, n, &read)
		out = append(out, NoteListItem{NoteRead: read, NoteVerdict: verdictOf(checks[n.ID])})
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

// countRead feeds the reading signal (quest=353): only the web UI (a person) and MCP (an agent)
// report reads — the CLI, bots, exports and other background clients do not — and an agent
// reading a PRIVATE note, which it only sees as a placeholder, is not interest either.
func (h *Handler) countRead(r *http.Request, n knowledge.Note) {
	src := requestSource(r)
	if src == nil || (*src != "web" && *src != "mcp") {
		return
	}
	agent := *src == "mcp"
	if agent && n.IsPrivate {
		return
	}
	if _, err := h.Store.RecordNoteOpen(r.Context(), n.ID, agent); err != nil {
		log.Printf("notes: counting a read of note %d: %v", n.ID, err)
	}
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

func noteCustomIcon(n knowledge.Note) string {
	if n.CustomIcon != nil {
		return *n.CustomIcon
	}
	return ""
}

func (h *Handler) getNoteIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	n, err := h.Store.GetNote(r.Context(), id)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	custom := noteCustomIcon(n)
	if custom == "" || !kernel.SafeIconName(custom) {
		writeErr(w, 404, "Icon not found")
		return
	}
	path := filepath.Join(h.DataDir, "note-icons", custom)
	httpx.ServeIcon(w, r, path)
}

func (h *Handler) postNoteIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := h.Store.GetNote(r.Context(), id); errors.Is(err, knowledge.ErrNotFound) {
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
	dir := filepath.Join(h.DataDir, "note-icons")
	_ = os.MkdirAll(dir, 0o755)
	name := strconv.FormatInt(id, 10) + ext
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	updated, err := h.Store.SetNoteIcon(r.Context(), id, name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.logNote(r.Context(), "note_updated", updated, "изменена", 10*time.Minute)
	writeJSON(w, 200, h.NotePayloadFiltered(r, updated))
}

func (h *Handler) deleteNoteIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	updated, err := h.Store.ClearNoteIcon(r.Context(), id, h.DataDir)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h.logNote(r.Context(), "note_updated", updated, "изменена", 10*time.Minute)
	writeJSON(w, 200, h.NotePayloadFiltered(r, updated))
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

// searchNotes ranks notes against a query: stemmed words in Russian and English, prefixes, the
// other keyboard layout, typos in titles. Each hit says where it matched and carries a fragment
// with the matches marked (⟪ ⟫). An agent (MCP) is never shown PRIVATE notes.
func (h *Handler) searchNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	opts := knowledge.SearchOpts{Limit: limit}
	if src := requestSource(r); src != nil && *src == "mcp" {
		opts.ExcludePrivate = true
		opts.Agent = true
	}
	want := parseNoteFlagFilter(q.Get("flags"))
	if want.bad != "" {
		writeErr(w, http.StatusUnprocessableEntity, "unknown flag: "+want.bad+" (known: "+noteFlagNames+")")
		return
	}
	if want.active() {
		// Filtering happens after ranking: look at the widest candidate set, then cut to the limit.
		opts.Limit = 50
	}
	hits, err := h.Store.SearchNotes(r.Context(), q.Get("q"), opts)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if want.active() {
		hits, err = h.filterHitsByFlags(r, hits, want, limit)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"q": strings.TrimSpace(q.Get("q")), "hits": hits, "marks": []string{knowledge.MarkOpen, knowledge.MarkClose}})
}

// searchInNote finds a query inside one note with the same rules as the notes search; the answer is
// the matched words (to mark them in the text) and the number of paragraphs that carry them.
func (h *Handler) searchInNote(w http.ResponseWriter, r *http.Request) {
	_, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	if src := requestSource(r); src != nil && *src == "mcp" && n.IsPrivate {
		writeErr(w, 404, "Note not found")
		return
	}
	res, err := h.Store.SearchInNote(r.Context(), n.Description, r.URL.Query().Get("q"))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// noteFeedback is the cheap explicit signal: this note was useful (or misleading) for the task at hand.
func (h *Handler) noteFeedback(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	var body struct {
		Useful *bool `json:"useful"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Useful == nil {
		writeErr(w, http.StatusBadRequest, `body: {"useful": true|false}`)
		return
	}
	if err := h.Store.RateNote(r.Context(), id, *body.Useful); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writePopularity(w, r, n)
}

// noteWeight sets the manual lever: −2 (misleading) … +2 (always worth a look).
func (h *Handler) noteWeight(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	var body struct {
		Weight *int `json:"weight"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Weight == nil || *body.Weight < -2 || *body.Weight > 2 {
		writeErr(w, http.StatusBadRequest, `body: {"weight": -2..2}`)
		return
	}
	if err := h.Store.SetNoteWeight(r.Context(), id, *body.Weight); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writePopularity(w, r, n)
}

// resetNoteStats forgets what was counted for the note (a mistaken streak of opens, a stale favourite).
func (h *Handler) resetNoteStats(w http.ResponseWriter, r *http.Request) {
	id, n, ok := h.noteForStats(w, r)
	if !ok {
		return
	}
	if err := h.Store.ResetNoteStats(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writePopularity(w, r, n)
}

func (h *Handler) noteForStats(w http.ResponseWriter, r *http.Request) (int64, knowledge.Note, bool) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	n, err := h.Store.GetNote(r.Context(), id)
	if errors.Is(err, knowledge.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return 0, n, false
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return 0, n, false
	}
	return id, n, true
}

func (h *Handler) writePopularity(w http.ResponseWriter, r *http.Request, n knowledge.Note) {
	pop, err := h.Store.Popularity(r.Context(), map[int64]time.Time{n.ID: n.CreatedAt})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, pop[n.ID])
}

// noteFlagNames are the flags a note search can filter on (flags=outdated,-private,…).
const noteFlagNames = "outdated|stale, readme, private, category, pinned|pin, check"

type noteFlagFilter struct {
	include, exclude []string
	bad              string
}

func (f noteFlagFilter) active() bool { return len(f.include)+len(f.exclude) > 0 }

func canonNoteFlag(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "outdated", "stale":
		return "outdated"
	case "readme", "private", "category", "check":
		return strings.ToLower(strings.TrimSpace(name))
	case "pinned", "pin":
		return "pinned"
	}
	return ""
}

// parseNoteFlagFilter reads "outdated,-private" (a minus means "without the flag").
func parseNoteFlagFilter(raw string) noteFlagFilter {
	var f noteFlagFilter
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		neg := strings.HasPrefix(part, "-")
		name := canonNoteFlag(strings.TrimPrefix(part, "-"))
		if name == "" {
			f.bad = part
			return f
		}
		if neg {
			f.exclude = append(f.exclude, name)
		} else {
			f.include = append(f.include, name)
		}
	}
	return f
}

func (h *Handler) filterHitsByFlags(r *http.Request, hits []knowledge.SearchHit, f noteFlagFilter, limit int) ([]knowledge.SearchHit, error) {
	rows, err := h.Store.ListNotes(r.Context(), knowledge.NoteFilter{})
	if err != nil {
		return nil, err
	}
	checks, _ := h.Store.NoteChecks(r.Context())
	byID := make(map[int64]knowledge.Note, len(rows))
	for _, n := range rows {
		byID[n.ID] = n
	}
	has := func(id int64, flag string) bool {
		n := byID[id]
		switch flag {
		case "outdated":
			return checks[id].Outdated
		case "readme":
			return n.IsReadme
		case "private":
			return n.IsPrivate
		case "category":
			return n.IsCategory
		case "pinned":
			return n.Pinned
		case "check":
			return checks[id].Active()
		}
		return false
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	out := make([]knowledge.SearchHit, 0, len(hits))
next:
	for _, h := range hits {
		for _, fl := range f.include {
			if !has(h.ID, fl) {
				continue next
			}
		}
		for _, fl := range f.exclude {
			if has(h.ID, fl) {
				continue next
			}
		}
		out = append(out, h)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
