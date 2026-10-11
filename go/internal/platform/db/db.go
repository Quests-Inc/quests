package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// The schema chain is embedded (sql/pg, applied by MigratePostgres); its head is
// headRevision and needs no manual bump. The version table keeps the name and
// format of the Alembic era (production was created by Alembic), so existing
// databases carry on from where they are.

// revisionOrder is oldest→newest. Unknown DB revisions are treated as ahead of this
// binary (deploy race: migrate applied before the new image lands).
var revisionOrder = buildOrder()

// headRevision is the schema revision this binary was built against.
var headRevision = pgMigrations[len(pgMigrations)-1].Revision

func buildOrder() []string {
	order := make([]string, 0, len(pgMigrations))
	for _, m := range pgMigrations {
		order = append(order, m.Revision)
	}
	return order
}

// SchemaStatus is the schema revision check outcome for health probes / logs.
// State is "ok" (match) or "warn" (DB ahead of this binary).
type SchemaStatus struct {
	Version  string
	Expected string
	State    string
	Detail   string
}

func assertMigrated(db *sql.DB, expected string) (SchemaStatus, error) {
	var ver string
	err := db.QueryRow(`SELECT version_num FROM alembic_version LIMIT 1`).Scan(&ver)
	if err == sql.ErrNoRows {
		return SchemaStatus{}, fmt.Errorf("the database has no alembic_version: start the server, it migrates the schema itself")
	}
	if err != nil {
		return SchemaStatus{}, fmt.Errorf("read alembic_version: %w (start the server: it migrates the schema itself)", err)
	}
	return checkSchemaVersionAgainst(ver, expected, revisionOrder)
}

func checkSchemaVersion(ver string) (SchemaStatus, error) {
	return checkSchemaVersionAgainst(ver, headRevision, revisionOrder)
}

func checkSchemaVersionAgainst(ver, expected string, order []string) (SchemaStatus, error) {
	expIdx := indexOf(order, expected)
	if expIdx < 0 {
		return SchemaStatus{}, fmt.Errorf("internal: headRevision %s missing from revisionOrder", expected)
	}
	st := SchemaStatus{Version: ver, Expected: expected, State: "ok"}
	dbIdx := indexOf(order, ver)
	if dbIdx < 0 {
		// Unknown id — treat as ahead (new migrate landed before this binary).
		st.State = "warn"
		st.Detail = fmt.Sprintf("Схема БД %s новее API (ожидалось %s) — обнови образ", ver, expected)
		return st, nil
	}
	if dbIdx < expIdx {
		return SchemaStatus{}, fmt.Errorf("alembic_version=%s older than required %s: start the server, it migrates the schema itself", ver, expected)
	}
	if dbIdx > expIdx {
		st.State = "warn"
		st.Detail = fmt.Sprintf("Схема БД %s новее API (ожидалось %s) — обнови образ", ver, expected)
		return st, nil
	}
	st.Detail = fmt.Sprintf("схема %s", ver)
	return st, nil
}

func indexOf(xs []string, want string) int {
	for i, x := range xs {
		if x == want {
			return i
		}
	}
	return -1
}

// IsUniqueViolation reports a UNIQUE / primary-key conflict.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
