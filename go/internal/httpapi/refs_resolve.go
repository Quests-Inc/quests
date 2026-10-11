package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/refs"
)

// The helpers below cross contexts (a mention can point at a quest, a note, a template…), so they stay in the root.

func (s *Server) resolveRefs(r *http.Request, list []refs.Ref) []refs.Resolved {
	out := make([]refs.Resolved, 0, len(list))
	for _, ref := range list {
		row := refs.Resolved{Kind: ref.Kind, ID: ref.ID}
		switch ref.Kind {
		case "note":
			n, err := s.Store.Knowledge().GetNote(r.Context(), ref.ID)
			if err == nil {
				row.Title = &n.Title
			}
		case "quest":
			q, err := s.Store.GetQuest(r.Context(), ref.ID)
			if err == nil {
				row.Title = &q.Title
			}
		case "questline":
			ql, err := s.Store.GetQuestline(r.Context(), ref.ID)
			if err == nil {
				row.Title = &ql.Title
			}
		case "step":
			qid, err := s.Store.QuestIDForStep(r.Context(), ref.ID)
			if err == nil {
				q, err := s.Store.GetQuest(r.Context(), qid)
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
			t, err := s.Store.GetTemplate(r.Context(), ref.ID)
			if err == nil {
				row.Title = &t.Title
			}
		case "attachment":
			a, err := s.Store.Knowledge().GetAttachment(r.Context(), ref.ID)
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
