package automation

import (
	"context"
	"github.com/quests-inc/quests/go/internal/platform/search"
)

func (s *Store) SearchEntities(ctx context.Context, r search.Request) ([]search.Hit, error) {
	src := search.Source{Kind: "template", From: "questtemplate t", ID: "t.id", Title: "t.title", Body: "t.description", Where: "t.deleted_at IS NULL", Fields: map[string]string{"freq": "t.freq", "line": "t.questline_id"}, Tests: map[string]map[string]string{"enabled": {"on": "t.enabled=1", "off": "t.enabled=0"}}}
	return search.Query(ctx, s.DB, src, r)
}
