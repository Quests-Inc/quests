package store

import (
	"context"

	"github.com/quests-inc/quests/go/internal/automation"
	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// Identity gives access to accounts, workspaces and roles (the identity context) over the same database.
// Transitional: until every context has its own service injected by the composition root (ADR-0001),
// the pieces that still live in store reach identity through this accessor.
func (s *Store) Identity() *identity.Store { return &identity.Store{DB: s.DB} }

// CopyCategories gives workspace `to` the category set of workspace `from` (a fresh workspace is usable at once).
func (s *Store) CopyCategories(ctx context.Context, from, to int64) error {
	type cat struct {
		slug, label, color string
		sort               int
	}
	rows, err := s.DB.QueryContext(wsctx.With(ctx, from), `SELECT slug, label, sort_order, color FROM questcategory ORDER BY sort_order, id`)
	if err != nil {
		return err
	}
	var cats []cat
	for rows.Next() {
		var c cat
		if err := rows.Scan(&c.slug, &c.label, &c.sort, &c.color); err != nil {
			rows.Close()
			return err
		}
		cats = append(cats, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	toCtx := wsctx.With(ctx, to)
	for _, c := range cats {
		if _, err := s.DB.ExecContext(toCtx,
			`INSERT INTO questcategory (slug, label, sort_order, created_at, color) VALUES (?, ?, ?, ?, ?)`,
			c.slug, c.label, c.sort, timeutil.ToDBUTC(timeutil.NowUTC()), c.color); err != nil {
			return err
		}
	}
	return nil
}

// Knowledge gives access to notes, attachments and search (the knowledge context) over the same database.
// Transitional, like Identity: until the composition root injects each context's service (ADR-0001).
func (s *Store) Knowledge() *knowledge.Store { return &knowledge.Store{DB: s.DB} }

// Journal gives access to quests, steps, questlines, categories, tags and the change log (the journal context)
// over the same database. Transitional, like Identity and Knowledge: until the composition root injects each
// context's service (ADR-0001).
func (s *Store) Journal() *journal.Store { return &journal.Store{DB: s.DB} }

// Automation gives access to templates, runners, jobs and secrets (the automation context) over the same database.
// Transitional, like Identity, Knowledge and Journal: until the composition root injects each context's service.
func (s *Store) Automation() *automation.Store { return &automation.Store{DB: s.DB, Box: s.Box} }
