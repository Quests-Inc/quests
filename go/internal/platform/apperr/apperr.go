// Package apperr holds the errors that several contexts report the same way, so that a handler
// can recognise them whichever context they came from.
package apperr

import "errors"

// ErrNotFound means the thing asked for does not exist (or is not visible in this workspace).
var ErrNotFound = errors.New("not found")
