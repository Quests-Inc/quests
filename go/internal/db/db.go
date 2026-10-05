package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// expectedAlembic is the schema revision this binary was built against.
// Keep in sync with alembic heads; append new ids to alembicOrder when adding migrations.
const expectedAlembic = "e6f7a8b9c0d1"

// alembicOrder is oldest→newest. Unknown DB revisions are treated as ahead of
// this binary (deploy race: migrate applied before the new image lands).
var alembicOrder = []string{
	"df501a4accdb",
	"7d093d52e81e",
	"a1b2c3d4e5f6",
	"b2c3d4e5f6a7",
	"c3d4e5f6a7b8",
	"d4e5f6a7b8c9",
	"e5f6a7b8c9d0",
	"f6a7b8c9d0e1",
	"a7b8c9d0e1f2",
	"b8c9d0e1f2a3",
	"c9d0e1f2a3b4",
	"d0e1f2a3b4c5",
	"e1f2a3b4c5d6",
	"f7a8b9c0d1e2",
	"a1c2e3f4b5d6",
	"a2b3c4d5e6f7",
	"b3c4d5e6f7a8",
	"c4d5e6f7a8b9",
	"c5d6e7f8a9b0",
	"d6e7f8a9b0c1",
	"e7f8a9b0c1d2",
	"f8a9b0c1d2e3",
	"a3b4c5d6e7f8",
	"b4c5d6e7f8a9",
	"2aa2bb8ef734",
	"580461765b15",
	"c1d2e3f4a5b6",
	"d2e3f4a5b6c7",
	"e3f4a5b6c7d8",
	"a9b0c1d2e3f4",
	"b0c1d2e3f4a5",
	"c2d3e4f5a6b7",
	"d3e4f5a6b7c8",
	"e4f5a6b7c8d9",
	"f1a2b3c4d5e6",
	"a0b1c2d3e4f5",
	"b1c2d3e4f5a6",
	"d5e6f7a8b9c0",
	"e6f7a8b9c0d1",
}

// SchemaStatus is the alembic check outcome for health probes / logs.
// State is "ok" (match) or "warn" (DB ahead of this binary).
type SchemaStatus struct {
	Version  string
	Expected string
	State    string
	Detail   string
}

func Open(path string) (*sql.DB, SchemaStatus, error) {
	var zero SchemaStatus
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return nil, zero, err
	}
	// WAL + busy timeout — match Python db.py
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, zero, err
	}
	sqlDB.SetMaxOpenConns(1) // SQLite: single writer
	sqlDB.SetMaxIdleConns(1)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, zero, err
	}
	st, err := assertMigrated(sqlDB)
	if err != nil {
		_ = sqlDB.Close()
		return nil, zero, err
	}
	return sqlDB, st, nil
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

func assertMigrated(db *sql.DB) (SchemaStatus, error) {
	var ver string
	err := db.QueryRow(`SELECT version_num FROM alembic_version LIMIT 1`).Scan(&ver)
	if err == sql.ErrNoRows {
		return SchemaStatus{}, fmt.Errorf("quests.db has no alembic_version; run: uv run quests-migrate")
	}
	if err != nil {
		return SchemaStatus{}, fmt.Errorf("read alembic_version: %w (run Python migrate first)", err)
	}
	return checkAlembicVersion(ver)
}

func checkAlembicVersion(ver string) (SchemaStatus, error) {
	return checkAlembicVersionAgainst(ver, expectedAlembic, alembicOrder)
}

func checkAlembicVersionAgainst(ver, expected string, order []string) (SchemaStatus, error) {
	expIdx := indexOf(order, expected)
	if expIdx < 0 {
		return SchemaStatus{}, fmt.Errorf("internal: expectedAlembic %s missing from alembicOrder", expected)
	}
	st := SchemaStatus{Version: ver, Expected: expected, State: "ok"}
	dbIdx := indexOf(order, ver)
	if dbIdx < 0 {
		// Unknown id — treat as ahead (new migrate landed before this binary).
		st.State = "warn"
		st.Detail = fmt.Sprintf("БД alembic=%s новее API (ожидалось %s) — обнови образ", ver, expected)
		return st, nil
	}
	if dbIdx < expIdx {
		return SchemaStatus{}, fmt.Errorf("alembic_version=%s older than required %s; upgrade with Python Alembic first", ver, expected)
	}
	if dbIdx > expIdx {
		st.State = "warn"
		st.Detail = fmt.Sprintf("БД alembic=%s новее API (ожидалось %s) — обнови образ", ver, expected)
		return st, nil
	}
	st.Detail = fmt.Sprintf("alembic=%s", ver)
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
