package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/refs"
	"github.com/valirum/quests/go/internal/schedule"
	"github.com/valirum/quests/go/internal/store"
)

func (s *Server) registerDomainRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/questlines", s.listQuestlines)
	mux.HandleFunc("POST /api/questlines", s.createQuestline)
	mux.HandleFunc("GET /api/questlines/{id}", s.getQuestline)
	mux.HandleFunc("PATCH /api/questlines/{id}", s.patchQuestline)
	mux.HandleFunc("DELETE /api/questlines/{id}", s.deleteQuestline)
	mux.HandleFunc("GET /api/questlines/{id}/icon", s.getQuestlineIcon)
	mux.HandleFunc("POST /api/questlines/{id}/icon", s.postQuestlineIcon)
	mux.HandleFunc("DELETE /api/questlines/{id}/icon", s.deleteQuestlineIcon)

	mux.HandleFunc("GET /api/templates", s.listTemplates)
	mux.HandleFunc("POST /api/templates", s.createTemplate)
	mux.HandleFunc("GET /api/templates/{id}", s.getTemplate)
	mux.HandleFunc("PATCH /api/templates/{id}", s.patchTemplate)
	mux.HandleFunc("DELETE /api/templates/{id}", s.deleteTemplate)
	mux.HandleFunc("POST /api/templates/{id}/copy", s.copyTemplate)
	mux.HandleFunc("POST /api/templates/{id}/emit", s.emitTemplate)
	mux.HandleFunc("GET /api/templates/{id}/emit-attempts", s.listEmitAttempts)
	s.registerSecretRoutes(mux)

	mux.HandleFunc("GET /api/templates/{id}/streak", s.getTemplateStreak)
	mux.HandleFunc("GET /api/quest-log", s.getQuestLog)
	mux.HandleFunc("GET /api/graph", s.getGraph)
	mux.HandleFunc("GET /api/context", s.getContext)
}

func (s *Server) listQuestlines(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListQuestlines(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) getQuestline(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := s.Store.GetQuestline(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	row.Refs = s.resolveRefs(r, refs.Parse(row.Title+"\n"+row.Description))
	writeJSON(w, 200, row)
}

func (s *Server) createQuestline(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	title, _ := body["title"].(string)
	desc, _ := body["description"].(string)
	color, _ := body["color"].(string)
	icon, _ := body["icon"].(string)
	var cat *int64
	if v, ok := body["category_id"].(float64); ok {
		i := int64(v)
		cat = &i
	}
	row, err := s.Store.CreateQuestline(r.Context(), strings.TrimSpace(title), desc, cat, color, icon)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.publishSilent(r.Context(), "questline_created", questlineTitle(row), map[string]any{"questline_id": row.ID})
	writeJSON(w, 201, row)
}

func (s *Server) patchQuestline(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	row, err := s.Store.UpdateQuestline(r.Context(), id, body)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.publishSilent(r.Context(), "questline_updated", questlineTitle(row), map[string]any{"questline_id": id})
	writeJSON(w, 200, row)
}

func (s *Server) deleteQuestline(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := s.Store.GetQuestline(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	title := questlineTitle(row)
	s.purgeOwnerAttachments(r.Context(), ownerQuestline, id)
	if err := s.Store.DeleteQuestline(r.Context(), id, s.DataDir); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "Questline not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	s.publishSilent(r.Context(), "questline_deleted", title, map[string]any{"questline_id": id})
	w.WriteHeader(204)
}

func questlineTitle(row store.QuestlineRead) string {
	return row.Title
}

func questlineCustomIcon(row store.QuestlineRead) string {
	if row.CustomIcon != nil {
		return *row.CustomIcon
	}
	return ""
}

func (s *Server) getQuestlineIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := s.Store.GetQuestline(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	custom := questlineCustomIcon(row)
	if custom == "" || !store.SafeIconName(custom) {
		writeErr(w, 404, "Icon not found")
		return
	}
	path := filepath.Join(s.DataDir, "questline-icons", custom)
	serveIcon(w, r, path)
}

// serveIcon serves a user-uploaded icon file. Icons may be SVG, which runs scripts when
// opened as a page, so the response is sandboxed: it can only ever be drawn as an image.
func serveIcon(w http.ResponseWriter, r *http.Request, path string) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	h.Set("Content-Disposition", "inline")
	http.ServeFile(w, r, path)
}

func (s *Server) postQuestlineIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, err := s.Store.GetQuestline(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
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
	dir := filepath.Join(s.DataDir, "questline-icons")
	_ = os.MkdirAll(dir, 0o755)
	name := strconv.FormatInt(id, 10) + ext
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	row, err := s.Store.SetQuestlineIcon(r.Context(), id, name)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.publishSilent(r.Context(), "questline_updated", questlineTitle(row), map[string]any{"questline_id": id})
	writeJSON(w, 200, row)
}

func (s *Server) deleteQuestlineIcon(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := s.Store.ClearQuestlineIcon(r.Context(), id, s.DataDir)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Questline not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.publishSilent(r.Context(), "questline_updated", questlineTitle(row), map[string]any{"questline_id": id})
	writeJSON(w, 200, row)
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	var en *bool
	if v := r.URL.Query().Get("enabled"); v != "" {
		b := v == "1" || strings.EqualFold(v, "true")
		en = &b
	}
	rows, err := s.Store.ListTemplates(r.Context(), en)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := s.Store.GetTemplate(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, row)
}

func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	row, err := s.Store.CreateTemplate(r.Context(), body)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if row.Enabled {
		_, _ = schedule.MaterializeDue(r.Context(), s.Store, s.Hub, time.Time{}, nil)
	}
	writeJSON(w, 201, row)
}

func (s *Server) patchTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	row, err := s.Store.UpdateTemplate(r.Context(), id, body)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if row.Enabled {
		_, _ = schedule.MaterializeDue(r.Context(), s.Store, s.Hub, time.Time{}, nil)
	}
	writeJSON(w, 200, row)
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.Store.DeleteTemplate(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "Template not found")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) copyTemplate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	row, err := s.Store.CopyTemplate(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, row)
}

func (s *Server) emitTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	qid, err := schedule.MaterializeTemplateManual(r.Context(), s.Store, s.Hub, id, timeutil.NowUTC(), nil)
	if errors.Is(err, schedule.ErrTemplateNotFound) {
		writeErr(w, http.StatusNotFound, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	q, err := s.Store.GetQuest(r.Context(), qid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := domain.ToQuestRead(q, timeutil.NowUTC())
	out.Refs = s.resolveRefs(r, refs.Parse(q.Title+"\n"+q.Description))
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) getTemplateStreak(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad template id")
		return
	}
	row, err := s.Store.TemplateStreak(r.Context(), id, timeutil.NowUTC())
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Template not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, row)
}

// getGraph maps the neighbourhood of one entity: ?quest=N | note=N | questline=N,
// optional &depth=1..4 (default 2).
func (s *Server) getGraph(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var rootType string
	var rootID int64
	for _, t := range []string{"quest", "note", "questline"} {
		if v := q.Get(t); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n <= 0 {
				writeErr(w, 400, "bad "+t+" id")
				return
			}
			rootType, rootID = t, n
			break
		}
	}
	if rootType == "" {
		writeErr(w, 400, "pass quest=, note= or questline=")
		return
	}
	depth := 2
	if v := q.Get("depth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			depth = n
		}
	}
	roots := []store.GraphRoot{{Type: rootType, ID: rootID}}
	// ?extra=quest:5,note:7 adds more centres to the same map.
	for _, part := range strings.Split(q.Get("extra"), ",") {
		t, idStr, ok := strings.Cut(strings.TrimSpace(part), ":")
		n, err := strconv.ParseInt(idStr, 10, 64)
		if ok && err == nil && n > 0 && (t == "quest" || t == "note" || t == "questline") {
			roots = append(roots, store.GraphRoot{Type: t, ID: n})
		}
	}
	// ?expand=step:3,quest:9>note adds the direct neighbours of those cards, however deep they are.
	var expand []store.GraphRoot
	for _, part := range strings.Split(q.Get("expand"), ",") {
		// "quest:5" opens every neighbour, "quest:5>note" only the notes among them.
		part, only, _ := strings.Cut(strings.TrimSpace(part), ">")
		t, idStr, ok := strings.Cut(part, ":")
		n, err := strconv.ParseInt(idStr, 10, 64)
		if ok && err == nil && n > 0 {
			expand = append(expand, store.GraphRoot{Type: t, ID: n, Only: only})
		}
	}
	g, err := s.Store.BuildGraphExpanded(r.Context(), roots, depth, expand)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, g)
}

func (s *Server) getQuestLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	i64 := func(key string) *int64 {
		if v := q.Get(key); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return &n
			}
		}
		return nil
	}
	list := func(key string) []string {
		var out []string
		for _, p := range strings.Split(q.Get(key), ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	when := func(key string) *time.Time {
		if v := q.Get(key); v != "" {
			if t, err := timeutil.ParseFlexible(v); err == nil {
				return &t
			}
		}
		return nil
	}
	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	rows, err := s.Store.ListChangeLog(r.Context(), store.ChangeLogFilter{
		Limit:       limit,
		QuestID:     i64("quest_id"),
		EntityTypes: list("entity_type"),
		EntityID:    i64("entity_id"),
		OwnerType:   q.Get("owner_type"),
		OwnerID:     i64("owner_id"),
		Kinds:       list("kind"),
		QuestlineID: i64("questline_id"),
		CategoryID:  i64("category_id"),
		TagID:       i64("tag_id"),
		From:        when("from"),
		To:          when("to"),
		Query:       q.Get("q"),
		BeforeID:    i64("before_id"),
		BeforeAt:    when("before_at"),
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) getContext(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var focusType string
	var focusID int64
	n := 0
	if v := q.Get("quest"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			focusType, focusID, n = "quest", id, n+1
		}
	}
	if v := q.Get("step"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			focusType, focusID, n = "step", id, n+1
		}
	}
	if v := q.Get("questline"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			focusType, focusID, n = "questline", id, n+1
		}
	}
	if v := q.Get("note"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			focusType, focusID, n = "note", id, n+1
		}
	}
	if n != 1 {
		writeErr(w, 400, "pass exactly one of quest, step, questline, note")
		return
	}
	if focusType == "note" {
		s.getNoteContext(w, r, focusID)
		return
	}
	var lineID *int64
	var quests []domain.Quest
	switch focusType {
	case "questline":
		lineID = &focusID
	case "quest":
		qq, err := s.Store.GetQuest(r.Context(), focusID)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "Quest not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		lineID = qq.QuestlineID
		if lineID == nil {
			quests = []domain.Quest{qq}
		}
	case "step":
		qid, err := s.Store.QuestIDForStep(r.Context(), focusID)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "Step not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		qq, err := s.Store.GetQuest(r.Context(), qid)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		lineID = qq.QuestlineID
		if lineID == nil {
			quests = []domain.Quest{qq}
		}
	}
	var line any
	if lineID != nil {
		lr, err := s.Store.GetQuestline(r.Context(), *lineID)
		if err != nil {
			writeErr(w, 404, "Questline not found")
			return
		}
		line = lr
		qs, err := s.Store.ListQuests(r.Context(), store.ListFilter{
			QuestlineID: lineID,
			NewestFirst: true,
		})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		quests = qs
	}
	reads := make([]domain.QuestRead, 0, len(quests))
	for _, qq := range quests {
		reads = append(reads, domain.ToQuestRead(qq, timeutil.NowUTC()))
	}
	attachments := map[string]any{
		"questline": []AttachmentView{},
		"by_quest":  map[string][]AttachmentView{},
		"note":      []AttachmentView{},
	}
	if lineID != nil {
		attachments["questline"] = s.attachmentsForOwner(r.Context(), ownerQuestline, *lineID, true)
	}
	byQuest := map[string][]AttachmentView{}
	for _, qq := range quests {
		byQuest[strconv.FormatInt(qq.ID, 10)] = s.attachmentsForOwner(r.Context(), ownerQuest, qq.ID, true)
	}
	attachments["by_quest"] = byQuest
	var texts []string
	if line != nil {
		if m, ok := line.(map[string]any); ok {
			if d, ok := m["description"].(string); ok {
				texts = append(texts, d)
			}
			if t, ok := m["title"].(string); ok {
				texts = append(texts, t)
			}
		}
	}
	for _, qq := range quests {
		texts = append(texts, qq.Title, qq.Description)
		for _, st := range qq.Steps {
			texts = append(texts, st.Title, st.Description)
		}
	}
	writeJSON(w, 200, map[string]any{
		"focus":        map[string]any{"type": focusType, "id": focusID},
		"questline":    line,
		"quests":       reads,
		"linked_notes": s.linkedNotesFromText(r, texts...),
		"attachments":  attachments,
	})
}

func (s *Server) getNoteContext(w http.ResponseWriter, r *http.Request, id int64) {
	n, err := s.Store.GetNote(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	payload := s.notePayloadFiltered(r, n)
	attachments := s.attachmentsForOwner(r.Context(), ownerNote, n.ID, true)
	payload.Attachments = attachments
	writeJSON(w, 200, map[string]any{
		"focus":       map[string]any{"type": "note", "id": n.ID},
		"note":        payload,
		"questline":   nil,
		"quests":      []any{},
		"attachments": map[string]any{"note": attachments},
	})
}
