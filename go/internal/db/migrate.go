package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Migrations are plain SQL files, one per revision, applied in file order.
//
//	sql/pg/NNNN_<revision>_<name>.sql
//
// The first files carry their Alembic revision id (12 hex digits) so
// databases migrated by Alembic carry on seamlessly; new files are named
// NNNN_<name>.sql and their revision is the number, NNNN.
//
//go:embed sql/pg/*.sql
var pgFS embed.FS

type Migration struct {
	Seq      int
	Revision string // what alembic_version holds once this is applied
	Name     string
	SQL      string
}

var fileRe = regexp.MustCompile(`^(\d{4})_(?:([0-9a-f]{12})_)?([a-z0-9_]+)\.sql$`)

// pgMigrations is parsed from the embedded files; a malformed set is a build defect,
// so it panics at start (TestEmbeddedMigrations covers it).
var pgMigrations = mustLoad(pgFS, "sql/pg")

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

// MigrateOptions tunes MigratePostgresOpts.
type MigrateOptions struct {
	// Logf receives progress lines (nil: silent).
	Logf func(string, ...any)
	// BeforeApply runs under the migration lock when an existing database (one already at some
	// revision) is about to get todo applied: the place for a safety backup. An error stops the
	// migration with nothing applied. It is not called for a fresh database.
	BeforeApply func(ctx context.Context, from string, todo []Migration) error
}

// MigratePostgres is MigratePostgresOpts with only a logger.
func MigratePostgres(ctx context.Context, url string, logf func(string, ...any)) ([]string, error) {
	return MigratePostgresOpts(ctx, url, MigrateOptions{Logf: logf})
}

// MigratePostgresOpts brings the database at url to the head of the embedded chain and
// returns the revisions it applied. Run as the database owner: the migrations create
// roles, policies and grants. Concurrent callers (several replicas starting together)
// queue on an advisory lock; the later ones find nothing left to do.
func MigratePostgresOpts(ctx context.Context, url string, opts MigrateOptions) ([]string, error) {
	logf := opts.Logf
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
	if len(todo) > 0 && version != "" && opts.BeforeApply != nil {
		if err := opts.BeforeApply(ctx, version, todo); err != nil {
			return nil, fmt.Errorf("before applying %04d %s: %w", todo[0].Seq, todo[0].Name, err)
		}
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

// CurrentRevision reports the revision a database is at ("" when it has none yet).
func CurrentRevision(databaseURL string) (string, error) {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return "", err
	}
	defer sqlDB.Close()
	var v string
	err = sqlDB.QueryRow(`SELECT version_num FROM alembic_version LIMIT 1`).Scan(&v)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil && strings.Contains(err.Error(), "alembic_version"):
		return "", nil // no such table yet
	case err != nil:
		return "", err
	}
	return v, nil
}

// ExpectedRevision is the schema revision this binary expects (the head of the chain).
func ExpectedRevision() string { return expectedAlembic }
