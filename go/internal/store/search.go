package store

import (
	"context"
	"github.com/quests-inc/quests/go/internal/platform/search"
)

// SearchEntities federates narrow context-owned projections, then applies one global page.
func (s *Store) SearchEntities(ctx context.Context, r search.Request) (search.Page, error) {
	if err := r.Normalize(); err != nil {
		return search.Page{}, err
	}
	hits := []search.Hit{}
	providers := []func(context.Context, search.Request) ([]search.Hit, error){s.Journal.SearchEntities, s.Knowledge.SearchEntities, s.Automation.SearchEntities, s.Execution.SearchEntities}
	for _, provider := range providers {
		h, err := provider(ctx, r)
		if err != nil {
			return search.Page{}, err
		}
		hits = append(hits, h...)
	}
	return search.Paginate(hits, r), nil
}
