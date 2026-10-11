package store

import (
	"database/sql"

	"github.com/quests-inc/quests/go/internal/platform/apperr"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
)

// ErrNotFound is the shared "no such thing" error (platform/apperr).
var ErrNotFound = apperr.ErrNotFound

// Store is what is left of the data layer while the contexts are being split out of it (ADR-0001): automation
// (templates, runners, secrets), the cross-cutting reads (graph, export/import, stats) and the backup log.
type Store struct {
	DB *sql.DB
	// DatabaseURL is the connection URL of DB: backups hand it to pg_dump.
	DatabaseURL string
	// Box seals secret values at rest (nil = stored as given, e.g. in tests).
	Box *secretbox.Box
}
