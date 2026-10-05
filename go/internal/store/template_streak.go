package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/valirum/quests/go/internal/timeutil"
)

// TemplateStreak is the close-rate / streak summary for a recurring template,
// computed from its period instances.
func (s *Store) TemplateStreak(ctx context.Context, templateID int64, now time.Time) (map[string]any, error) {
	var title string
	err := s.DB.QueryRowContext(ctx, `SELECT title FROM questtemplate WHERE id=?`, templateID).Scan(&title)
	if err != nil {
		return nil, ErrNotFound
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, status, period_key, deadline_at FROM quest
		WHERE template_id=? AND period_key IS NOT NULL ORDER BY period_key`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	outcomes := []string{}
	bars := []map[string]any{}
	closed, total := 0, 0
	for rows.Next() {
		var id int64
		var status, pk string
		var deadline sql.NullString
		_ = rows.Scan(&id, &status, &pk, &deadline)
		outcome := "open"
		switch status {
		case "completed":
			outcome = "completed"
		case "failed", "expired", "archived":
			outcome = "miss"
		default:
			if deadline.Valid {
				t, e := timeutil.ParseFlexible(deadline.String)
				if e == nil && !t.After(now) {
					outcome = "miss"
				}
			}
		}
		outcomes = append(outcomes, outcome)
		bars = append(bars, map[string]any{
			"period_key": pk, "status": status, "outcome": outcome, "quest_id": id,
		})
		total++
		if outcome != "open" {
			closed++
		}
	}
	cur, longest := computeStreaks(outcomes)
	rate := 0.0
	nClosed, nComp := 0, 0
	for _, o := range outcomes {
		if o == "open" {
			continue
		}
		nClosed++
		if o == "completed" {
			nComp++
		}
	}
	if nClosed > 0 {
		rate = float64(nComp) / float64(nClosed)
	}
	return map[string]any{
		"id": templateID, "title": title,
		"current_streak": cur, "longest_streak": longest,
		"closed": closed, "total": total, "close_rate": rate,
		"bars": bars,
	}, nil
}

func computeStreaks(outcomes []string) (current, longest int) {
	run := 0
	for _, o := range outcomes {
		if o == "completed" {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	i := len(outcomes) - 1
	for i >= 0 && outcomes[i] == "open" {
		i--
	}
	for i >= 0 && outcomes[i] == "completed" {
		current++
		i--
	}
	return current, longest
}
