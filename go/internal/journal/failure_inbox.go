package journal

import (
	"context"
	"database/sql"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// ensureFailureInboxTx puts questID into the failure inbox (or re-opens a
// dismissed row). Call from the same transaction that set status=failed.
func ensureFailureInboxTx(tx *sql.Tx, ctx context.Context, questID int64, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO quest_failure_inbox (quest_id, created_at, dismissed_at)
		VALUES (?, ?, NULL)
		ON CONFLICT (quest_id) DO UPDATE SET
			dismissed_at = NULL,
			created_at = EXCLUDED.created_at`,
		questID, timeutil.ToDBUTC(now),
	)
	return err
}

// DismissFailureInbox hides a failed quest from the journal inbox. Idempotent.
func (s *Store) DismissFailureInbox(ctx context.Context, questID int64) error {
	now := timeutil.NowUTC()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE quest_failure_inbox
		SET dismissed_at = ?
		WHERE quest_id = ? AND dismissed_at IS NULL`,
		timeutil.ToDBUTC(now), questID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Already dismissed, or never in the inbox — still OK if the quest exists.
		var one int
		err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM quest WHERE id = ?`, questID).Scan(&one)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	return nil
}
