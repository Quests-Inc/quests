package store

import (
	"database/sql"

	"github.com/quests-inc/quests/go/internal/automation"
	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/identity"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/knowledge"
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

	// The contexts, built once over DB by New (the composition root hands them out; nothing else constructs them).
	Identity   *identity.Store
	Knowledge  *knowledge.Store
	Journal    *journal.Store
	Automation *automation.Store
	Execution  *execution.Store
}

// New builds the store and the context stores over db.
func New(db *sql.DB, databaseURL string, box *secretbox.Box) *Store {
	return &Store{
		DB: db, DatabaseURL: databaseURL, Box: box,
		Identity: &identity.Store{DB: db}, Knowledge: &knowledge.Store{DB: db}, Journal: &journal.Store{DB: db},
		Automation: automation.New(db, box), Execution: execution.New(db, box),
	}
}
