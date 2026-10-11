package store

import (
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/apperr"
	"github.com/quests-inc/quests/go/internal/platform/dbx"
)

// The helpers and errors below moved to platform/dbx, platform/apperr and kernel (ADR-0001) so that the contexts
// being split out of this package can share them; the aliases keep the code and the callers unchanged meanwhile.

var (
	boolInt = dbx.BoolInt

	// IsBadInput reports whether err is a validation message meant for the caller (HTTP 422).
	IsBadInput = apperr.IsBad
	// ErrConflict means the change clashes with the current state.
	ErrConflict = apperr.ErrConflict

	ErrBadRunnerLabel = kernel.ErrBadRunnerLabel
	ErrCommandOwner   = kernel.ErrCommandOwner
	ErrNoRunner       = kernel.ErrNoRunner
	ValidRunOn        = kernel.ValidRunOn
	NormalizeRunOn    = kernel.NormalizeRunOn
	SafeIconName      = kernel.SafeIconName
)
