package execution

import (
	"context"
	"github.com/quests-inc/quests/go/internal/platform/search"
)

func (s *Store) SearchEntities(ctx context.Context, r search.Request) ([]search.Hit, error) {
	sources := []search.Source{
		{Kind: "script", From: "script s", ID: "s.id", Title: "s.title", Body: "s.description || ' ' || s.subjects", Where: "s.deleted_at IS NULL", Fields: map[string]string{"purpose": "s.purpose"}},
		{Kind: "hook", From: "hook h LEFT JOIN script hs ON hs.id=h.script_id", ID: "h.id", Title: "h.name", Body: "h.signals || ' ' || h.run_on || ' ' || COALESCE(hs.title,'') || ' ' || h.script_id::text || ' ' || h.scope_kind", Where: "h.deleted_at IS NULL", Fields: map[string]string{"scope": "h.scope_kind"}, Tests: map[string]map[string]string{"enabled": {"on": "h.enabled=1", "off": "h.enabled=0"}}},
		{Kind: "runner", From: "runner r", ID: "r.id", Title: "r.label", Body: "r.hostname", Where: "TRUE"},
		{Kind: "job", From: "runner_job j", ID: "j.id", Title: "j.title", Body: "j.runner_label || ' ' || j.state", Where: "TRUE", Fields: map[string]string{"status": "j.state", "device": "j.runner_label"}},
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
