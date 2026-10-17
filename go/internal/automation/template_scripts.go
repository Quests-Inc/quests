package automation

import (
	"context"
	"database/sql"
	"errors"
)

func scriptIDArg(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// templateScript is the script bound to a template's emit, or nil.
func (s *Store) templateScript(ctx context.Context, id int64) (*int64, error) {
	var sid sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT script_id FROM script_binding WHERE subject_type = 'template' AND subject_id = ?`, id).Scan(&sid)
	if errors.Is(err, sql.ErrNoRows) || !sid.Valid {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sid.Int64, nil
}

// ClearEmitCommand empties emit_pool_command when a script takes over.
func (s *Store) ClearEmitCommand(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE questtemplate SET emit_pool_command = NULL WHERE id = ?`, id)
	return err
}
