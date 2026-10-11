// Package knowledge is the notes-and-files context (ADR-0001): notes with their tree, reading statistics and
// freshness checks, attachments with their versions, and search over notes.
package knowledge

import (
	"context"
	"database/sql"

	"github.com/valirum/quests/go/internal/identity"
	"github.com/valirum/quests/go/internal/kernel"
	"github.com/valirum/quests/go/internal/platform/apperr"
	"github.com/valirum/quests/go/internal/platform/dbx"
)

// Store is the context's data access: the PostgreSQL database behind the row-level-security driver.
type Store struct {
	DB *sql.DB
}

// Errors and helpers shared with the other contexts (platform/apperr, platform/dbx, kernel).
var (
	ErrNotFound       = apperr.ErrNotFound
	ErrConflict       = apperr.ErrConflict
	ErrBadRunnerLabel = kernel.ErrBadRunnerLabel
	NormalizeRunOn    = kernel.NormalizeRunOn
	ValidRunOn        = kernel.ValidRunOn
	SafeIconName      = kernel.SafeIconName

	errBad        = apperr.Bad
	boolInt       = dbx.BoolInt
	nullStr       = dbx.NullStr
	parseNullTime = dbx.ParseNullTime
	trimTo        = dbx.TrimTo
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
