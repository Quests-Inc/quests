// Package apperr holds the errors that several contexts report the same way, so that a handler
// can recognise them whichever context they came from.
package apperr

import "errors"

// ErrNotFound means the thing asked for does not exist (or is not visible in this workspace).
var ErrNotFound = errors.New("not found")

// ErrConflict means the change clashes with the current state (a duplicate, a stale revision).
var ErrConflict = errors.New("conflict")

// BadInput is a validation message meant for the caller (HTTP 422), as opposed to a failure of ours.
type BadInput string

func (e BadInput) Error() string { return string(e) }

// Bad makes a BadInput error.
func Bad(msg string) error { return BadInput(msg) }

// IsBad reports whether err is a BadInput.
func IsBad(err error) bool {
	var b BadInput
	return errors.As(err, &b)
}
