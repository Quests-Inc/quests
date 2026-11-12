package journal

import (
	"context"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// ActiveQuestIDsPastDeadline lists active quests whose explicit expiry policy is due.
func (s *Store) ActiveQuestIDsPastDeadline(ctx context.Context, now time.Time) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id FROM quest
		WHERE status = 'active' AND deadline_at IS NOT NULL AND deadline_at <= ?
			AND expire_on_deadline=1
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

// ExpiresOnDeadline preserves legacy in-process constructors. Persisted quests always have an explicit policy.
func ExpiresOnDeadline(q Quest) bool {
	if q.ExpireOnDeadline != nil {
		return *q.ExpireOnDeadline
	}
	return q.DurationSeconds != nil && *q.DurationSeconds > 0
}
