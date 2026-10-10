package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Migrations are plain SQL files, one per revision, applied in file order.
//
//	sql/pg/NNNN_<revision>_<name>.sql   PostgreSQL chain (the one that grows)
//	sql/sqlite/...                      baseline of the frozen SQLite chain
//
// The first PostgreSQL files carry their Alembic revision id (12 hex digits) so
// databases migrated by Alembic carry on seamlessly; new files are named
// NNNN_<name>.sql and their revision is the number, NNNN.
//
//go:embed sql/pg/*.sql
var pgFS embed.FS

//go:embed sql/sqlite/*.sql
var sqliteFS embed.FS

type Migration struct {
	Seq      int
	Revision string // what alembic_version holds once this is applied
	Name     string
	SQL      string
}

var fileRe = regexp.MustCompile(`^(\d{4})_(?:([0-9a-f]{12})_)?([a-z0-9_]+)\.sql$`)

// pgMigrations / sqliteMigrations are parsed from the embedded files; a malformed
// set is a build defect, so it panics at start (TestEmbeddedMigrations covers it).
var (
	pgMigrations     = mustLoad(pgFS, "sql/pg")
	sqliteMigrations = mustLoad(sqliteFS, "sql/sqlite")
)

func mustLoad(fsys fs.FS, dir string) []Migration {
	ms, err := loadMigrations(fsys, dir)
	if err != nil {
		panic("db: embedded migrations: " + err.Error())
	}
	return ms
}

func loadMigrations(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var ms []Migration
	seen := map[string]bool{}
	for _, e := range entries {
		m := fileRe.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("%s: name must be NNNN_[revision_]name.sql", e.Name())
		}
		seq, _ := strconv.Atoi(m[1])
		rev := m[2]
		if rev == "" {
			rev = m[1]
		}
		if seen[rev] {
			return nil, fmt.Errorf("%s: duplicate revision %s", e.Name(), rev)
		}
		seen[rev] = true
		body, err := fs.ReadFile(fsys, dir+"/"+e.Name())
		if err != nil {
			return nil, err
		}
		ms = append(ms, Migration{Seq: seq, Revision: rev, Name: m[3], SQL: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Seq < ms[j].Seq })
	for i, m := range ms {
		if m.Seq != i+1 {
			return nil, fmt.Errorf("sequence gap: expected %04d, found %04d", i+1, m.Seq)
		}
	}
	if len(ms) == 0 {
		return nil, fmt.Errorf("no migrations in %s", dir)
	}
	return ms, nil
}

// pending returns the migrations after the one the database is at. A version this
// binary does not know is ahead of it (nothing to apply).
func pending(ms []Migration, version string) (todo []Migration, known bool) {
	if version == "" {
		return ms, true
	}
	for i, m := range ms {
		if m.Revision == version {
			return ms[i+1:], true
		}
	}
	return nil, false
}

// migrateLockKey is the pg_advisory_lock key serialising concurrent migrators.
const migrateLockKey = 0x71657374 // "qest"

const versionTable = `CREATE TABLE IF NOT EXISTS alembic_version (
	version_num VARCHAR(32) NOT NULL,
	CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num)
)`

// MigratePostgres brings the database at url to the head of the embedded chain and
// returns the revisions it applied. It logs through logf (may be nil). Run as the
// database owner: the migrations create roles, policies and grants.
func MigratePostgres(ctx context.Context, url string, logf func(string, ...any)) ([]string, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrateLockKey); err != nil {
		return nil, fmt.Errorf("migration lock: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrateLockKey) }()

	var hasVersion, hasData bool
	if err := conn.QueryRowContext(ctx, `SELECT to_regclass('alembic_version') IS NOT NULL, to_regclass('quest') IS NOT NULL`).
		Scan(&hasVersion, &hasData); err != nil {
		return nil, err
	}
	if !hasVersion && hasData {
		return nil, fmt.Errorf("the database has tables but no alembic_version: refusing to guess its revision")
	}
	if _, err := conn.ExecContext(ctx, versionTable); err != nil {
		return nil, err
	}
	var version string
	switch err := conn.QueryRowContext(ctx, `SELECT version_num FROM alembic_version LIMIT 1`).Scan(&version); err {
	case nil, sql.ErrNoRows:
	default:
		return nil, err
	}

	todo, known := pending(pgMigrations, version)
	if !known {
		logf("database revision %s is newer than this binary (head %s): nothing to apply", version, expectedAlembic)
		return nil, nil
	}
	var applied []string
	for _, m := range todo {
		if err := applyPG(ctx, conn, m, version != ""); err != nil {
			return applied, fmt.Errorf("migration %04d %s: %w", m.Seq, m.Name, err)
		}
		version = m.Revision
		applied = append(applied, m.Revision)
		logf("applied %04d %s (%s)", m.Seq, m.Name, m.Revision)
	}
	return applied, nil
}

func applyPG(ctx context.Context, conn *sql.Conn, m Migration, haveRow bool) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return err
	}
	if haveRow {
		_, err = tx.ExecContext(ctx, `UPDATE alembic_version SET version_num = $1`, m.Revision)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO alembic_version (version_num) VALUES ($1)`, m.Revision)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// MigrateSQLite creates a fresh SQLite database at path from the baseline. An
// existing file is only checked: its chain is frozen, an older one has to be brought
// to the baseline with the archived Alembic chain.
func MigrateSQLite(path string, logf func(string, ...any)) ([]string, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	sqlDB, st, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	defer sqlDB.Close()
	if st != nil {
		logf("%s", st.Detail)
		return nil, nil
	}
	tx, err := sqlDB.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	base := sqliteMigrations[0]
	if _, err := tx.Exec(base.SQL); err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	logf("created %s at %s", path, base.Revision)
	return []string{base.Revision}, nil
}

// openSQLite opens path and, when it already holds a schema, validates its revision
// (st != nil). A nil status means an empty database that needs the baseline.
func openSQLite(path string) (*sql.DB, *SchemaStatus, error) {
	if err := ensureDir(path); err != nil {
		return nil, nil, err
	}
	sqlDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(30000)&_pragma=foreign_keys(ON)", path))
	if err != nil {
		return nil, nil, err
	}
	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&n); err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	if n == 0 {
		return sqlDB, nil, nil
	}
	st, err := assertMigrated(sqlDB, expectedAlembicSQLite)
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	return sqlDB, &st, nil
}

// CurrentRevision reports the revision a database is at ("" when empty).
func CurrentRevision(path, databaseURL string) (string, error) {
	var sqlDB *sql.DB
	var err error
	if databaseURL != "" {
		sqlDB, err = sql.Open("pgx", databaseURL)
	} else {
		if _, statErr := os.Stat(path); statErr != nil {
			return "", nil
		}
		sqlDB, err = sql.Open("sqlite", "file:"+path+"?mode=ro")
	}
	if err != nil {
		return "", err
	}
	defer sqlDB.Close()
	var v string
	switch err := sqlDB.QueryRow(`SELECT version_num FROM alembic_version LIMIT 1`).Scan(&v); {
	case err == sql.ErrNoRows:
		return "", nil
	case err != nil && strings.Contains(err.Error(), "alembic_version"):
		return "", nil
	case err != nil:
		return "", err
	}
	return v, nil
}
