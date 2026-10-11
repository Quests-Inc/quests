package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/valirum/quests/go/internal/journal"
	knowledgehttp "github.com/valirum/quests/go/internal/knowledge/http"
	"github.com/valirum/quests/go/internal/platform/httpx"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/store"
)

func (s *Server) registerDomainRoutes(mux *http.ServeMux) {

	s.automationHTTP().Register(mux)
	mux.HandleFunc("GET /api/graph", s.getGraph)
	mux.HandleFunc("GET /api/context", s.getContext)
}

var serveIcon = httpx.ServeIcon

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
	var quests []journal.Quest
	switch focusType {
	case "questline":
		lineID = &focusID
	case "quest":
		qq, err := s.Store.Journal().GetQuest(r.Context(), focusID)
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
			quests = []journal.Quest{qq}
		}
	case "step":
		qid, err := s.Store.Journal().QuestIDForStep(r.Context(), focusID)
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "Step not found")
			return
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		qq, err := s.Store.Journal().GetQuest(r.Context(), qid)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		lineID = qq.QuestlineID
		if lineID == nil {
			quests = []journal.Quest{qq}
		}
	}
	var line any
	if lineID != nil {
		lr, err := s.Store.Journal().GetQuestline(r.Context(), *lineID)
		if err != nil {
			writeErr(w, 404, "Questline not found")
			return
		}
		line = lr
		qs, err := s.Store.Journal().ListQuests(r.Context(), journal.ListFilter{
			QuestlineID: lineID,
			NewestFirst: true,
		})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		quests = qs
	}
	reads := make([]journal.QuestRead, 0, len(quests))
	for _, qq := range quests {
		reads = append(reads, journal.ToQuestRead(qq, timeutil.NowUTC()))
	}
	attachments := map[string]any{
		"questline": []knowledgehttp.AttachmentView{},
		"by_quest":  map[string][]knowledgehttp.AttachmentView{},
		"note":      []knowledgehttp.AttachmentView{},
	}
	if lineID != nil {
		attachments["questline"] = s.knowledgeHTTP().AttachmentsForOwner(r.Context(), ownerQuestline, *lineID, true)
	}
	byQuest := map[string][]knowledgehttp.AttachmentView{}
	for _, qq := range quests {
		byQuest[strconv.FormatInt(qq.ID, 10)] = s.knowledgeHTTP().AttachmentsForOwner(r.Context(), ownerQuest, qq.ID, true)
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
	n, err := s.Store.Knowledge().GetNote(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, 404, "Note not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	payload := s.knowledgeHTTP().NotePayloadFiltered(r, n)
	attachments := s.knowledgeHTTP().AttachmentsForOwner(r.Context(), ownerNote, n.ID, true)
	payload.Attachments = attachments
	writeJSON(w, 200, map[string]any{
		"focus":       map[string]any{"type": "note", "id": n.ID},
		"note":        payload,
		"questline":   nil,
		"quests":      []any{},
		"attachments": map[string]any{"note": attachments},
	})
}
