// Package trashrow implements the shared write for recoverable deletion, independent of a domain.
package trashrow

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func Move(ctx context.Context, db *sql.DB, table string, id int64) error {
	if table != "questline" && table != "questtemplate" {
		return fmt.Errorf("invalid trash table")
	}
	var user any
	if n := wsctx.UserFrom(ctx); n > 0 {
		user = n
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT set_config('app.trash','1',true)`); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE "+table+" SET deleted_at=?,deleted_by=? WHERE id=? AND deleted_at IS NULL", timeutil.ToDBUTC(timeutil.NowUTC()), user, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperr.ErrNotFound
	}
	return tx.Commit()
}
