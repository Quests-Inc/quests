package journal

import (
	"context"
	"database/sql"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/db"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// The trash. Deleting a quest only sets deleted_at; a restrictive row-level-security policy (migration 0015) hides such a
// row from every ordinary query. The statements below run in a transaction that opens the policy with app.trash = '1',
// which is the only way to see, restore or erase a trashed quest. Steps, tags, secrets, attachments and history are not
// touched until the quest is purged.

// TrashedQuest is a row of the trash list.
type TrashedQuest struct {
	ID          int64
	Title       string
	Status      string
	QuestlineID *int64
	StepsTotal  int
	StepsDone   int
	DeletedAt   time.Time
	DeletedBy   *int64
}

// trashTx begins a transaction in which trashed rows are visible.
func (s *Store) trashTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.trash', '1', true)`); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

func nullUser(ctx context.Context) any {
	if u := wsctx.UserFrom(ctx); u > 0 {
		return u
	}
	return nil
}

// TrashQuest moves a quest to the trash (what DELETE /api/quests/{id} does).
func (s *Store) TrashQuest(ctx context.Context, id int64, title string) error {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE quest SET deleted_at = ?, deleted_by = ? WHERE id = ? AND deleted_at IS NULL`,
		timeutil.ToDBUTC(timeutil.NowUTC()), nullUser(ctx), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := stageChange(tx, ctx, "quest_deleted", &id, title, "в корзину", ""); err != nil {
		return err
	}
	return tx.Commit()
}

// ListTrashedQuests is the quests in the trash of the context's workspace, newest first.
func (s *Store) ListTrashedQuests(ctx context.Context) ([]TrashedQuest, error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		SELECT q.id, q.title, q.status, q.questline_id, q.deleted_at, q.deleted_by,
		       (SELECT COUNT(*) FROM queststep st WHERE st.quest_id = q.id),
		       (SELECT COUNT(*) FROM queststep st WHERE st.quest_id = q.id AND st.progress_current >= st.progress_total)
		FROM quest q WHERE q.deleted_at IS NOT NULL ORDER BY q.deleted_at DESC, q.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrashedQuest{}
	for rows.Next() {
		var t TrashedQuest
		var line, by sql.NullInt64
		var at string
		if err := rows.Scan(&t.ID, &t.Title, &t.Status, &line, &at, &by, &t.StepsTotal, &t.StepsDone); err != nil {
			return nil, err
		}
		if line.Valid {
			v := line.Int64
			t.QuestlineID = &v
		}
		if by.Valid {
			v := by.Int64
			t.DeletedBy = &v
		}
		t.DeletedAt, _ = timeutil.ParseFlexible(at)
		out = append(out, t)
	}
	return out, rows.Err()
}

// RestoreQuest takes a quest out of the trash. A questline that was deleted in the meantime is dropped from it; a
// template period that has been emitted again meanwhile is detached (the restored quest stays, as a one-off).
func (s *Store) RestoreQuest(ctx context.Context, id int64) (title string, err error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	restore := func(detach bool) error {
		extra := ""
		if detach {
			extra = ", template_id = NULL, period_key = NULL"
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE quest SET deleted_at = NULL, deleted_by = NULL, updated_at = ?,
			  questline_id = CASE WHEN questline_id IS NOT NULL
			      AND NOT EXISTS (SELECT 1 FROM questline l WHERE l.id = quest.questline_id) THEN NULL ELSE questline_id END`+extra+`
			WHERE id = ? AND deleted_at IS NOT NULL`, timeutil.ToDBUTC(timeutil.NowUTC()), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	}
	if err := tx.QueryRowContext(ctx, `SELECT title FROM quest WHERE id = ? AND deleted_at IS NOT NULL`, id).Scan(&title); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	// A unique violation aborts the transaction, so the retry goes through a savepoint.
	if _, err := tx.ExecContext(ctx, `SAVEPOINT restore`); err != nil {
		return "", err
	}
	if err := restore(false); err != nil {
		if !db.IsUniqueViolation(err) {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT restore`); err != nil {
			return "", err
		}
		if err := restore(true); err != nil {
			return "", err
		}
	}
	if err := stageChange(tx, ctx, "quest_restored", &id, title, "восстановлено", ""); err != nil {
		return "", err
	}
	return title, tx.Commit()
}

// PurgeQuest erases a trashed quest for good (steps first: they have no ON DELETE CASCADE). Attachments are the
// caller's business: their files live on WebDAV.
func (s *Store) PurgeQuest(ctx context.Context, id int64) error {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var one int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM quest WHERE id = ? AND deleted_at IS NOT NULL`, id).Scan(&one); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM queststep WHERE quest_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM quest WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// TrashedQuestIDsBefore is the quests that have been in the trash since before cutoff (for the automatic purge).
func (s *Store) TrashedQuestIDsBefore(ctx context.Context, cutoff time.Time) ([]int64, error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM quest WHERE deleted_at IS NOT NULL AND deleted_at < ? ORDER BY id`,
		timeutil.ToDBUTC(cutoff))
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
