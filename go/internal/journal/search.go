package journal

import (
	"context"
	"github.com/quests-inc/quests/go/internal/platform/search"
)

func (s *Store) SearchEntities(ctx context.Context, r search.Request) ([]search.Hit, error) {
	questTests := map[string]map[string]string{
		"status": {"done": "q.status='completed'", "all": "TRUE"},
		"flag":   {"pinned": "q.pinned=1", "hidden": "q.overlay_hidden=1"},
		"type":   {"automated": "q.automated=1", "manual": "q.automated=0"},
	}
	sources := []search.Source{
		{Kind: "quest", From: "quest q LEFT JOIN questcategory c ON c.id=q.category_id LEFT JOIN questline l ON l.id=q.questline_id", ID: "q.id", Title: "q.title", Body: "q.description || ' ' || COALESCE(l.title,'') || ' ' || COALESCE(c.label,'') || ' ' || COALESCE(c.slug,'') || ' ' || q.status || ' ' || COALESCE((SELECT string_agg(st.title || ' ' || st.description,' ') FROM queststep st WHERE st.quest_id=q.id),'')", Where: "TRUE", Fields: map[string]string{"status": "q.status", "sig": "q.significance", "line": "q.questline_id", "cat": "c.slug"}, Tests: questTests},
		{Kind: "step", From: "queststep st JOIN quest q ON q.id=st.quest_id", ID: "st.id", Title: "st.title", Body: "st.description", Where: "TRUE", ParentKind: "quest", ParentID: "st.quest_id", Fields: map[string]string{"status": "q.status", "quest": "q.id"}},
		{Kind: "questline", From: "questline l", ID: "l.id", Title: "l.title", Body: "l.description", Where: "l.deleted_at IS NULL", Fields: map[string]string{"cat": "l.category_id"}},
		{Kind: "event", From: "event e", ID: "e.id", Title: "e.title", Body: "e.description", Where: "TRUE"},
		{Kind: "tag", From: "tag t", ID: "t.id", Title: "t.label", Body: "t.slug", Where: "TRUE"},
		{Kind: "category", From: "questcategory c", ID: "c.id", Title: "c.label", Body: "c.slug", Where: "TRUE"},
	}
	sources[0].Lists = map[string]string{"tag": "ARRAY(SELECT t.slug FROM quest_tag qt JOIN tag t ON t.id=qt.tag_id WHERE qt.quest_id=q.id)"}

	out := []search.Hit{}
	for _, src := range sources {
		h, err := search.Query(ctx, s.DB, src, r)
		if err != nil {
			return nil, err
		}
		out = append(out, h...)
	}
	return out, nil
}
