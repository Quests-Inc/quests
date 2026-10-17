// Package automation is the context of everything that runs by itself or elsewhere (ADR-0001): templates and their
// emission, runners and the jobs they take, secrets for commands, emit attempts.
package automation

import (
	"context"
	"database/sql"

	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
	"github.com/quests-inc/quests/go/internal/platform/dbx"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
)

// Store is the context's data access: the PostgreSQL database behind the row-level-security driver.
type Store struct {
	DB *sql.DB
	// Box seals secret values at rest (nil = stored as given, e.g. in tests).
	Box *secretbox.Box

	// Journal, Knowledge and Identity are the other contexts this one builds on, over the same database.
	Journal   *journal.Store
	Knowledge *knowledge.Store
	Identity  *identity.Store
}

// New builds the store over db together with the contexts it builds on.
func New(db *sql.DB, box *secretbox.Box) *Store {
	return &Store{
		DB: db, Box: box,
		Journal: &journal.Store{DB: db}, Knowledge: &knowledge.Store{DB: db}, Identity: &identity.Store{DB: db},
	}
}

// Errors and helpers shared with the other contexts (platform/apperr, platform/dbx, kernel).
var (
	ErrNotFound       = apperr.ErrNotFound
	ErrConflict       = apperr.ErrConflict
	ErrBadRunnerLabel = kernel.ErrBadRunnerLabel
	ErrBadRunOn       = kernel.ErrBadRunOn
	ErrCommandOwner   = kernel.ErrCommandOwner
	ErrNoRunner       = kernel.ErrNoRunner
	ValidRunOn        = kernel.ValidRunOn
	NormalizeRunOn    = kernel.NormalizeRunOn

	errBad        = apperr.Bad
	boolInt       = dbx.BoolInt
	nullStr       = dbx.NullStr
	nullInt       = dbx.NullInt
	nullI64       = dbx.NullI64
	parseNullTime = dbx.ParseNullTime
	trimTo        = dbx.TrimTo
)

type rowScanner = dbx.Scanner

// guardCommand applies the shared policy for user code (kernel.Commands): owner-only authoring, and a runner is required.
func (s *Store) guardCommand(ctx context.Context, command, old, runOn *string) error {
	return kernel.Commands{Roles: s.Identity}.Check(ctx, command, old, runOn)
}
