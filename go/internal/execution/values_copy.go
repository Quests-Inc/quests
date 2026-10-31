package execution

import (
	"context"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// CopyNoteValues copies stored values verbatim, including formulas and roll provenance.
// The destination must have no values; inherited values remain inherited from its parent.
func (s *Store) CopyNoteValues(ctx context.Context, sourceID, targetID int64, hidePrivate bool) error {
	if sourceID == targetID {
		return &ValueError{Msg: "нельзя копировать переменные в ту же заметку"}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sourcePrivate bool
	if err := tx.QueryRowContext(ctx, `SELECT is_private FROM note WHERE id = ?`, sourceID).Scan(&sourcePrivate); err != nil {
		return err
	}
	if hidePrivate && sourcePrivate {
		return ErrPrivateValue
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM note WHERE id = ? FOR UPDATE`, targetID).Scan(&id); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM named_value WHERE note_id = ?`, targetID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return &ValueError{Msg: "у копии уже есть собственные переменные"}
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	_, err = tx.ExecContext(ctx, `INSERT INTO named_value (note_id,name,type,value,formula,roll_script,roll_arg,created_at,updated_at)
 SELECT ?,name,type,value,formula,roll_script,roll_arg,?,? FROM named_value WHERE note_id = ?`, targetID, now, now, sourceID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
