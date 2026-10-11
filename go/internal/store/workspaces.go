package store

import (
	"context"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

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
