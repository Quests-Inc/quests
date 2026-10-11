// Package dbx has the small helpers every data-access layer needs: nullable values, row scanning.
package dbx

import (
	"database/sql"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/platform/timeutil"
)

// Scanner is what *sql.Row and *sql.Rows share.
type Scanner interface {
	Scan(dest ...any) error
}

// BoolInt is the 0/1 the schema uses for flags.
func BoolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// NullStr turns a nil string pointer into SQL NULL.
func NullStr(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

// ParseNullTime reads a nullable timestamp column (nil when NULL, empty or unparseable).
func ParseNullTime(n sql.NullString) *time.Time {
	if !n.Valid || n.String == "" {
		return nil
	}
	t, err := timeutil.ParseFlexible(n.String)
	if err != nil {
		return nil
	}
	return &t
}

// TrimTo trims spaces and cuts s to n bytes.
func TrimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// NullTime turns a nil time pointer into SQL NULL (and formats the rest the way the schema stores them).
func NullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return timeutil.ToDBUTC(*t)
}

// NullInt turns a nil int pointer into SQL NULL.
func NullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

// NullI64 turns a nil int64 pointer into SQL NULL.
func NullI64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
