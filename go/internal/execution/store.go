// Package execution is the context of running user code (ADR-0002): the runners that execute it, the job queue and its
// leases, the secrets a command runs with. Where a command may come from and who may write it is one policy,
// kernel.Commands; the contexts that own commands (journal, knowledge, automation) ask it and queue their work here.
package execution

import (
	"database/sql"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
	"github.com/quests-inc/quests/go/internal/platform/dbx"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
)

// Store is the context's data access: the PostgreSQL database behind the row-level-security driver.
type Store struct {
	DB *sql.DB
	// Box seals secret values at rest (nil = stored as given, e.g. in tests).
	Box *secretbox.Box

	Identity *identity.Store
}

// New builds the store over db.
func New(db *sql.DB, box *secretbox.Box) *Store {
	return &Store{DB: db, Box: box, Identity: &identity.Store{DB: db}}
}

// Errors and helpers shared with the other contexts (platform/apperr, platform/dbx, kernel).
var (
	ErrNotFound       = apperr.ErrNotFound
	ErrConflict       = apperr.ErrConflict
	ErrBadRunnerLabel = kernel.ErrBadRunnerLabel
	ErrBadRunOn       = kernel.ErrBadRunOn
	NormalizeRunOn    = kernel.NormalizeRunOn

	errBad        = apperr.Bad
	boolInt       = dbx.BoolInt
	parseNullTime = dbx.ParseNullTime
	trimTo        = dbx.TrimTo
)

type rowScanner = dbx.Scanner
