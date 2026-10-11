// Package pgtest gives tests a real PostgreSQL database with the full schema.
//
// QUESTS_TEST_PG_URL names a server on which the role may CREATE DATABASE (and
// CREATE ROLE: the workspaces migration makes the API role). Tests skip without it.
//
// The schema is migrated once into a template database (qt_tpl_<revision>, kept
// between runs and shared by concurrent test binaries); every test then gets its
// own copy, which makes it independent and costs a few dozen milliseconds.
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/platform/db"
)

const (
	envURL = "QUESTS_TEST_PG_URL"
	// lockKey serialises building the template across test binaries run in parallel.
	lockKey = 0x71747074 // "qtpt"
)

var counter atomic.Int64

// URL returns the server URL from QUESTS_TEST_PG_URL, skipping the test without it.
func URL(t testing.TB) string {
	t.Helper()
	u := os.Getenv(envURL)
	if u == "" {
		t.Skip(envURL + " not set (just pg-test starts a throwaway PostgreSQL)")
	}
	return u
}

// New returns a pool on a fresh database. It connects as the API does (role
// quests_app, row-level security on) and treats a context without a workspace as
// workspace 1, so tests need not thread one through every call.
func New(t testing.TB) *sql.DB {
	t.Helper()
	pool, _ := NewWithURL(t)
	return pool
}

// NewWithURL is New that also returns the database URL (for owner-level access).
func NewWithURL(t testing.TB) (*sql.DB, string) {
	t.Helper()
	base := URL(t)
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx := context.Background()

	tpl, err := ensureTemplate(ctx, admin, base)
	if err != nil {
		t.Fatalf("pgtest: template: %v", err)
	}
	name := fmt.Sprintf("qt_%d_%d_%d", os.Getpid(), time.Now().UnixNano()%1_000_000_000, counter.Add(1))
	if _, err := admin.ExecContext(ctx, fmt.Sprintf(`CREATE DATABASE %s TEMPLATE %s`, name, tpl)); err != nil {
		t.Fatalf("pgtest: create database: %v", err)
	}
	dbURL := withDatabase(t, base, name)
	pool, _, err := db.OpenPostgresOpts(dbURL, db.PGOptions{DefaultWorkspace: 1})
	if err != nil {
		_, _ = admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`)
		t.Fatalf("pgtest: open: %v", err)
	}
	t.Cleanup(func() {
		_ = pool.Close()
		c, err := sql.Open("pgx", base)
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`)
	})
	return pool, dbURL
}

func ensureTemplate(ctx context.Context, admin *sql.DB, base string) (string, error) {
	tpl := "qt_tpl_" + db.ExpectedRevision()
	conn, err := admin.Conn(ctx)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return "", err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey) }()

	var exists bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, tpl).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return tpl, nil
	}
	// Build under a temporary name and rename when complete, so a crashed build never
	// looks like a finished template.
	building := tpl + "_building"
	_, _ = conn.ExecContext(ctx, `DROP DATABASE IF EXISTS `+building+` WITH (FORCE)`)
	if _, err := conn.ExecContext(ctx, `CREATE DATABASE `+building); err != nil {
		return "", err
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	u.Path = "/" + building
	if _, err := db.MigratePostgres(ctx, u.String(), nil); err != nil {
		return "", err
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(`ALTER DATABASE %s RENAME TO %s`, building, tpl)); err != nil {
		return "", err
	}
	return tpl, nil
}

func withDatabase(t testing.TB, base, name string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

// SeedQuest inserts a minimal active quest and returns its id.
func SeedQuest(t testing.TB, pool *sql.DB, title string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(`INSERT INTO quest (title, description, status, pinned, sort_order, created_at, updated_at)
		VALUES (?, '', 'active', 0, 0, '2026-01-01 00:00:00', '2026-01-01 00:00:00') RETURNING id`, title).Scan(&id)
	if err != nil {
		t.Fatalf("pgtest: seed quest: %v", err)
	}
	return id
}

// SeedTemplate inserts a minimal daily template and returns its id.
func SeedTemplate(t testing.TB, pool *sql.DB, title string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(`INSERT INTO questtemplate (title, created_at, updated_at)
		VALUES (?, '2026-01-01 00:00:00', '2026-01-01 00:00:00') RETURNING id`, title).Scan(&id)
	if err != nil {
		t.Fatalf("pgtest: seed template: %v", err)
	}
	return id
}

// FixSequences moves every serial sequence past the largest id in its table. Call it
// after seeding rows with explicit ids, which leave the sequences behind. url is the
// database URL NewWithURL returned (the owner role is needed to set sequences).
func FixSequences(t testing.TB, url string) {
	t.Helper()
	owner, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	_, err = owner.Exec(`DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT c.table_name AS t, pg_get_serial_sequence(quote_ident(c.table_name), 'id') AS s
           FROM information_schema.columns c
           WHERE c.table_schema = 'public' AND c.column_name = 'id'
  LOOP
    IF r.s IS NOT NULL THEN
      EXECUTE format('SELECT setval(%L, GREATEST(COALESCE((SELECT MAX(id) FROM %I), 0), 1), (SELECT COUNT(*) FROM %I) > 0)', r.s, r.t, r.t);
    END IF;
  END LOOP;
END $$`)
	if err != nil {
		t.Fatalf("pgtest: fix sequences: %v", err)
	}
}

// Owner is a pool on the same database as the owner role, as the host-side admin
// commands run: no row-level security, and it may change what the API role may not
// (password hashes, account state). It treats a context without a workspace as workspace 1.
func Owner(t testing.TB, url string) *sql.DB {
	t.Helper()
	pool, _, err := db.OpenPostgresOpts(url, db.PGOptions{DefaultWorkspace: 1, Owner: true})
	if err != nil {
		t.Fatalf("pgtest: open as owner: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

// ExecOwner runs statements as the database owner: for what the API role may not
// touch (workspace trust, extra workspaces, password hashes). Sequences are fixed after.
func ExecOwner(t testing.TB, url, stmts string) {
	t.Helper()
	owner, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err := owner.Exec(stmts); err != nil {
		t.Fatalf("pgtest: exec as owner: %v", err)
	}
	FixSequences(t, url)
}

// Seed runs several statements one by one (the pool's driver wrapper handles a single
// statement per Exec) and fails the test on the first error.
func Seed(t testing.TB, pool *sql.DB, script string) {
	t.Helper()
	for _, stmt := range splitStatements(script) {
		if _, err := pool.Exec(stmt); err != nil {
			t.Fatalf("pgtest: seed %q: %v", firstLine(stmt), err)
		}
	}
}

func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			return s[:i]
		}
	}
	return s
}

// splitStatements splits on semicolons outside '…' literals.
func splitStatements(script string) []string {
	var out []string
	var cur []rune
	inQuote := false
	flush := func() {
		if s := trimSpace(string(cur)); s != "" {
			out = append(out, s)
		}
		cur = cur[:0]
	}
	for _, c := range script {
		switch {
		case c == '\'':
			inQuote = !inQuote
			cur = append(cur, c)
		case c == ';' && !inQuote:
			flush()
		default:
			cur = append(cur, c)
		}
	}
	flush()
	return out
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\n' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\n' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
