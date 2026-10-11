package journal

import (
	"context"
	"database/sql"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
	"github.com/quests-inc/quests/go/internal/platform/dbx"
)

// Store is the context's data access: the PostgreSQL database behind the row-level-security driver.
type Store struct {
	DB *sql.DB
}

// Errors and helpers shared with the other contexts (platform/apperr, platform/dbx, kernel).
var (
	ErrNotFound    = apperr.ErrNotFound
	ErrConflict    = apperr.ErrConflict
	NormalizeRunOn = kernel.NormalizeRunOn
	SafeIconName   = kernel.SafeIconName

	boolInt  = dbx.BoolInt
	nullStr  = dbx.NullStr
	nullTime = dbx.NullTime
	nullInt  = dbx.NullInt
	nullI64  = dbx.NullI64
)

type rowScanner = dbx.Scanner

// guardAuthor refuses a command written by someone who is not an owner of the workspace (kernel.CommandAuthor).
func (s *Store) guardAuthor(ctx context.Context, command, old *string) error {
	return kernel.CommandAuthor(ctx, &identity.Store{DB: s.DB}, command, old)
}

// guardExec refuses a command that has no runner (kernel.CommandExec).
func (s *Store) guardExec(_ context.Context, command, runOn *string) error {
	return kernel.CommandExec(command, runOn)
}
