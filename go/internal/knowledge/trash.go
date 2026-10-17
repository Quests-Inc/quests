package knowledge

import (
	"context"
	"database/sql"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// The notes trash works like the quests' one (journal/trash.go): deleted_at hides the row through the restrictive RLS
// policy of migration 0015, and these statements open it with app.trash = '1'. A note keeps its place: its own parent
// stays, and the children that were lifted to the root are remembered (deleted_children) to be put back on restore.

// TrashedNote is a row of the trash list.
type TrashedNote struct {
	ID        int64
	Title     string
	Size      int
	Children  int
	DeletedAt time.Time
	DeletedBy *int64
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

func nullUser(ctx context.Context) any {
	if u := wsctx.UserFrom(ctx); u > 0 {
		return u
	}
	return nil
}

// TrashNote moves a note to the trash; its children are lifted to the root (as a hard delete does) and remembered.
func (s *Store) TrashNote(ctx context.Context, id int64) error {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
		UPDATE note SET deleted_at = ?, deleted_by = ?,
		       deleted_children = ARRAY(SELECT c.id FROM note c WHERE c.parent_id = note.id AND c.deleted_at IS NULL ORDER BY c.id)
		WHERE id = ? AND deleted_at IS NULL`, timeutil.ToDBUTC(timeutil.NowUTC()), nullUser(ctx), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE note SET parent_id = NULL WHERE parent_id = ? AND deleted_at IS NULL`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ListTrashedNotes is the notes in the trash of the context's workspace, newest first.
func (s *Store) ListTrashedNotes(ctx context.Context) ([]TrashedNote, error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		SELECT id, title, LENGTH(description), COALESCE(cardinality(deleted_children), 0), deleted_at, deleted_by
		FROM note WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrashedNote{}
	for rows.Next() {
		var t TrashedNote
		var by sql.NullInt64
		var at string
		if err := rows.Scan(&t.ID, &t.Title, &t.Size, &t.Children, &at, &by); err != nil {
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

// RestoreNote takes a note out of the trash, back under its parent (or at the root if the parent is gone or trashed),
// and gives it its children back (those still at the root).
func (s *Store) RestoreNote(ctx context.Context, id int64) (title string, err error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, `SELECT title FROM note WHERE id = ? AND deleted_at IS NOT NULL`, id).Scan(&title); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE note SET
		  parent_id = CASE WHEN parent_id IS NOT NULL
		      AND NOT EXISTS (SELECT 1 FROM note p WHERE p.id = note.parent_id AND p.deleted_at IS NULL) THEN NULL ELSE parent_id END,
		  deleted_at = NULL, deleted_by = NULL, updated_at = ?
		WHERE id = ?`, timeutil.ToDBUTC(timeutil.NowUTC()), id); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE note SET parent_id = ?
		WHERE parent_id IS NULL AND deleted_at IS NULL AND id <> ?
		  AND id = ANY (SELECT unnest(deleted_children) FROM note WHERE id = ?)`, id, id, id); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE note SET deleted_children = NULL WHERE id = ?`, id); err != nil {
		return "", err
	}
	return title, tx.Commit()
}

// PurgeNote erases a trashed note for good. Attachments and the icon file are the caller's business.
func (s *Store) PurgeNote(ctx context.Context, id int64) error {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var one int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM note WHERE id = ? AND deleted_at IS NOT NULL`, id).Scan(&one); err != nil {
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		return err
	}
	// Anything still pointing at it (a child restored later under it cannot be: the trash hides the parent) is lifted.
	if _, err := tx.ExecContext(ctx, `UPDATE note SET parent_id = NULL WHERE parent_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM note WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// TrashedNoteIDsBefore is the notes that have been in the trash since before cutoff (for the automatic purge).
func (s *Store) TrashedNoteIDsBefore(ctx context.Context, cutoff time.Time) ([]int64, error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM note WHERE deleted_at IS NOT NULL AND deleted_at < ? ORDER BY id`,
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

// TrashedNoteIcon is the custom icon file of a trashed note ("" if none), so purging can remove the file.
func (s *Store) TrashedNoteIcon(ctx context.Context, id int64) (string, error) {
	tx, err := s.trashTx(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var icon sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT custom_icon FROM note WHERE id = ? AND deleted_at IS NOT NULL`, id).Scan(&icon); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	return icon.String, nil
}
