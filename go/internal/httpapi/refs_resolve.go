package httpapi

import (
	"net/http"
	"strings"

	"github.com/quests-inc/quests/go/internal/refs"
)

// The helpers below cross contexts (a mention can point at a quest, a note, a template…), so they stay in the root.

func (s *Server) resolveRefs(r *http.Request, list []refs.Ref) []refs.Resolved {
	out := make([]refs.Resolved, 0, len(list))
	// Resolving a title must not fetch each entire note (including its body) separately.
	noteIDs := make([]any, 0)
	for _, ref := range list {
		if ref.Kind == "note" {
			noteIDs = append(noteIDs, ref.ID)
		}
	}
	noteTitles := make(map[int64]string)
	for start := 0; start < len(noteIDs); start += 256 {
		end := min(start+256, len(noteIDs))
		args := noteIDs[start:end]
		rows, err := s.Store.DB.QueryContext(r.Context(), `SELECT id, title FROM note WHERE id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(args)), ",")+`)`, args...)
		if err != nil {
			continue
		}
		for rows.Next() {
			var id int64
			var title string
			if rows.Scan(&id, &title) == nil {
				noteTitles[id] = title
			}
		}
		_ = rows.Close()
	}
	for _, ref := range list {
		row := refs.Resolved{Kind: ref.Kind, ID: ref.ID}
		switch ref.Kind {
		case "repo":
			var title string
			if err := s.Store.DB.QueryRowContext(r.Context(), `SELECT title FROM repository WHERE id=?`, ref.ID).Scan(&title); err == nil {
				row.Title = &title
			}
		case "note":
			if title, ok := noteTitles[ref.ID]; ok {
				row.Title = &title
			}
		case "quest":
			q, err := s.Store.Journal.GetQuest(r.Context(), ref.ID)
			if err == nil {
				row.Title = &q.Title
			}
		case "questline":
			ql, err := s.Store.Journal.GetQuestline(r.Context(), ref.ID)
			if err == nil {
				row.Title = &ql.Title
			}
		case "event":
			ev, err := s.Store.Journal.GetEvent(r.Context(), ref.ID)
			if err == nil {
				row.Title = &ev.Title
			}
		case "step":
			qid, err := s.Store.Journal.QuestIDForStep(r.Context(), ref.ID)
			if err == nil {
				q, err := s.Store.Journal.GetQuest(r.Context(), qid)
				if err == nil {
					for _, st := range q.Steps {
						if st.ID == ref.ID {
							row.Title = &st.Title
							row.QuestID = &q.ID
							break
						}
					}
				}
			}
		case "template":
			t, err := s.Store.Automation.GetTemplate(r.Context(), ref.ID)
			if err == nil {
				row.Title = &t.Title
			}
		case "attachment":
			a, err := s.Store.Knowledge.GetAttachment(r.Context(), ref.ID)
			if err == nil {
				row.Title = &a.Filename
				row.OwnerType = a.OwnerType
				row.OwnerID = &a.OwnerID
			}
		}
		out = append(out, row)
	}
	return out
}

func (s *Server) linkedNotesFromText(r *http.Request, texts ...string) []refs.Resolved {
	var blob strings.Builder
	for _, t := range texts {
		blob.WriteString(t)
		blob.WriteByte('\n')
	}
	noteRefs := refs.FilterKind(refs.Parse(blob.String()), "note")
	return s.resolveRefs(r, noteRefs)
}
