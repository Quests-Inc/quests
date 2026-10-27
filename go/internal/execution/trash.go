package execution

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// The trash of scripts and hooks (migration 0030, the same scheme as quests and notes in 0015): deleting one sets
// deleted_at and a restrictive row-level-security policy hides the row from every ordinary query. The statements below open
// the policy for their own transaction with app.trash = '1'. A trashed script keeps its versions; a trashed hook does not fire.

// TrashedItem is a script or a hook in the trash.
type TrashedItem struct {
	ID        int64
	Title     string
	DeletedAt time.Time
	DeletedBy *int64
	// ScriptID is a hook's script (to tell that it is in the trash too).
	ScriptID int64
}

// ErrScriptTrashed: a hook cannot come back while its script is in the trash.
var ErrScriptTrashed = errors.New("скрипт этого хука в корзине: сначала восстановите его")

func trashTable(kind string) string {
	if kind == "hook" {
		return "hook"
	}
	return "script"
}

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

func trashUser(ctx context.Context) any {
	if u := wsctx.UserFrom(ctx); u > 0 {
		return u
	}
	return nil
}

// trashRow moves a live script or hook to the trash.
func (s *Store) trashRow(ctx context.Context, kind string, id int64) error {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE `+trashTable(kind)+` SET deleted_at = ?, deleted_by = ? WHERE id = ? AND deleted_at IS NULL`,
		timeutil.ToDBUTC(timeutil.NowUTC()), trashUser(ctx), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// ListTrashed is the scripts or the hooks in the trash of the context's workspace, newest first.
func (s *Store) ListTrashed(ctx context.Context, kind string) ([]TrashedItem, error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	q := `SELECT id, title, deleted_at, deleted_by, 0 FROM script WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC, id DESC`
	if kind == "hook" {
		q = `SELECT id, COALESCE(NULLIF(name, ''), 'хук'), deleted_at, deleted_by, script_id FROM hook
			WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC, id DESC`
	}
	rows, err := tx.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrashedItem{}
	for rows.Next() {
		var t TrashedItem
		var at string
		var by sql.NullInt64
		if err := rows.Scan(&t.ID, &t.Title, &at, &by, &t.ScriptID); err != nil {
			return nil, err
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

// RestoreTrashed takes a script or a hook out of the trash. A hook whose script is still in the trash stays there.
func (s *Store) RestoreTrashed(ctx context.Context, kind string, id int64) (title string, err error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	if kind == "hook" {
		var scriptGone bool
		err = tx.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(h.name, ''), 'хук'), s.deleted_at IS NOT NULL
			FROM hook h JOIN script s ON s.id = h.script_id WHERE h.id = ? AND h.deleted_at IS NOT NULL`, id).Scan(&title, &scriptGone)
		if err == nil && scriptGone {
			return "", ErrScriptTrashed
		}
	} else {
		err = tx.QueryRowContext(ctx, `SELECT title FROM script WHERE id = ? AND deleted_at IS NOT NULL`, id).Scan(&title)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE `+trashTable(kind)+` SET deleted_at = NULL, deleted_by = NULL, updated_at = ? WHERE id = ?`,
		timeutil.ToDBUTC(timeutil.NowUTC()), id); err != nil {
		return "", err
	}
	return title, tx.Commit()
}

// PurgeTrashed erases a trashed script (its versions, and the trashed hooks that still name it) or a trashed hook for good.
func (s *Store) PurgeTrashed(ctx context.Context, kind string, id int64) error {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	table := trashTable(kind)
	var one int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM `+table+` WHERE id = ? AND deleted_at IS NOT NULL`, id).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if kind != "hook" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM hook WHERE script_id = ? AND deleted_at IS NOT NULL`, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// TrashedIDsBefore is the scripts or hooks that have been in the trash since before cutoff (for the automatic purge).
func (s *Store) TrashedIDsBefore(ctx context.Context, kind string, cutoff time.Time) ([]int64, error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM `+trashTable(kind)+` WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
		timeutil.ToDBUTC(cutoff))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
