package knowledge

import (
	"context"
	"github.com/quests-inc/quests/go/internal/platform/search"
)

func (s *Store) SearchEntities(ctx context.Context, r search.Request) ([]search.Hit, error) {
	sources := []search.Source{
		{Kind: "note", From: "note n LEFT JOIN note_stat ns ON ns.note_id=n.id", ID: "n.id", Title: "n.title", Body: "n.description", Where: "TRUE", ParentKind: "note", ParentID: "n.parent_id", Tests: map[string]map[string]string{"flag": {"pinned": "n.pinned=1", "readme": "n.is_readme=1", "private": "n.is_private=1", "category": "n.is_category=1", "outdated": "n.outdated=1", "check": "(NULLIF(trim(n.check_command),'') IS NOT NULL OR EXISTS(SELECT 1 FROM script_binding sb WHERE sb.subject_type='note' AND sb.subject_id=n.id))"}}},
		{Kind: "attachment", From: "attachment a", ID: "a.id", Title: "a.filename", Body: "a.comment || ' ' || a.content_type_detected", Where: "TRUE", Fields: map[string]string{"owner": "a.owner_type", "owner_id": "a.owner_id", "rev": "a.current_revision"}, Tests: map[string]map[string]string{"flag": {"hidden": "a.hidden=1"}, "show": {"all": "TRUE", "active": "a.hidden=0"}}},
	}
	// Preserve reading-history/newness/manual weighting for every client, before LIMIT.
	sources[0].RankFactor = `(CASE WHEN n.outdated=1 THEN 0.8 ELSE 1.0 END)*(1.0+GREATEST(-0.1,LEAST(0.3,
      0.2*LEAST(LN(1+GREATEST(0,COALESCE(ns.score,0))*POWER(0.5,GREATEST(0,EXTRACT(EPOCH FROM ((CURRENT_TIMESTAMP AT TIME ZONE 'UTC')-ns.score_at))/86400)/30))/LN(21),1)
      +0.1*POWER(0.5,GREATEST(0,EXTRACT(EPOCH FROM ((CURRENT_TIMESTAMP AT TIME ZONE 'UTC')-n.created_at))/86400)/14)
      +0.05*GREATEST(-2,LEAST(2,COALESCE(ns.manual,0))))))`
	if r.ExcludePrivate {
		sources[1].Where = "NOT EXISTS(SELECT 1 FROM note pn WHERE a.owner_type='note' AND pn.id=a.owner_id AND pn.is_private=1)"
	}
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
