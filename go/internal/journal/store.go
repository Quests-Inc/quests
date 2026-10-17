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

// guardCommand applies the shared policy for user code (kernel.Commands): owner-only authoring, and a runner is required.
func (s *Store) guardCommand(ctx context.Context, command, old, runOn *string) error {
	return kernel.Commands{Roles: &identity.Store{DB: s.DB}}.Check(ctx, command, old, runOn)
}
