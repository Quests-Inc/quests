package store

import (
	"github.com/valirum/quests/go/internal/kernel"
	"github.com/valirum/quests/go/internal/platform/apperr"
	"github.com/valirum/quests/go/internal/platform/dbx"
)

// The helpers and errors below moved to platform/dbx, platform/apperr and kernel (ADR-0001) so that the contexts
// being split out of this package can share them; the aliases keep the code and the callers unchanged meanwhile.

type rowScanner = dbx.Scanner

var (
	boolInt       = dbx.BoolInt
	nullStr       = dbx.NullStr
	parseNullTime = dbx.ParseNullTime
	nullInt       = dbx.NullInt
	nullI64       = dbx.NullI64
	trimTo        = dbx.TrimTo
	errBad        = apperr.Bad

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
