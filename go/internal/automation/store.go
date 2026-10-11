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
}

// Journal, Knowledge and Identity are the other contexts this one builds on, over the same database.
func (s *Store) Journal() *journal.Store     { return &journal.Store{DB: s.DB} }
func (s *Store) Knowledge() *knowledge.Store { return &knowledge.Store{DB: s.DB} }
func (s *Store) Identity() *identity.Store   { return &identity.Store{DB: s.DB} }

// Errors and helpers shared with the other contexts (platform/apperr, platform/dbx, kernel).
var (
	ErrNotFound       = apperr.ErrNotFound
	ErrConflict       = apperr.ErrConflict
	ErrBadRunnerLabel = kernel.ErrBadRunnerLabel
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

// guardAuthor refuses a command written by someone who is not an owner of the workspace (kernel.CommandAuthor).
func (s *Store) guardAuthor(ctx context.Context, command, old *string) error {
	return kernel.CommandAuthor(ctx, s.Identity(), command, old)
}

// guardExec refuses a command that has no runner (kernel.CommandExec).
func (s *Store) guardExec(_ context.Context, command, runOn *string) error {
	return kernel.CommandExec(command, runOn)
}
