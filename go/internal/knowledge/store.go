// Package knowledge is the notes-and-files context (ADR-0001): notes with their tree, reading statistics and
// freshness checks, attachments with their versions, and search over notes.
package knowledge

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
	ErrNotFound       = apperr.ErrNotFound
	ErrConflict       = apperr.ErrConflict
	ErrBadRunnerLabel = kernel.ErrBadRunnerLabel
	ErrBadRunOn       = kernel.ErrBadRunOn
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

// guardCommand applies the shared policy for user code (kernel.Commands): owner-only authoring, and a runner is required.
func (s *Store) guardCommand(ctx context.Context, command, old, runOn *string) error {
	return kernel.Commands{Roles: &identity.Store{DB: s.DB}}.Check(ctx, command, old, runOn)
}
