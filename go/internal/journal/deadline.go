package journal

import (
	"context"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// ActiveQuestIDsPastDeadline lists the active quests whose window has closed (a deadline without a window is only informational).
func (s *Store) ActiveQuestIDsPastDeadline(ctx context.Context, now time.Time) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id FROM quest
		WHERE status = 'active' AND deadline_at IS NOT NULL AND deadline_at <= ?
			-- duration=0/NULL means "no window": deadline is informational only,
			-- don't auto-flip the quest to expired just because it passed.
			AND duration_seconds IS NOT NULL AND duration_seconds > 0
		ORDER BY id`, timeutil.ToDBUTC(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
