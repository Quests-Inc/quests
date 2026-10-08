package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestRebind(t *testing.T) {
	cases := map[string]string{
		`SELECT 1`:                                                  `SELECT 1`,
		`SELECT * FROM t WHERE a = ? AND b = ?`:                     `SELECT * FROM t WHERE a = $1 AND b = $2`,
		`SELECT '?' , x FROM t WHERE y = ?`:                         `SELECT '?' , x FROM t WHERE y = $1`,
		`SELECT 'it''s ?', ?`:                                       `SELECT 'it''s ?', $1`,
		`SELECT "we?rd" FROM t WHERE a = ?`:                         `SELECT "we?rd" FROM t WHERE a = $1`,
		`INSERT INTO t (a, b) VALUES (?, ?) ON CONFLICT DO NOTHING`: `INSERT INTO t (a, b) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
	}
	for in, want := range cases {
		if got := rebind(in); got != want {
			t.Errorf("rebind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInsertReturningID(t *testing.T) {
	yes := []string{
		`INSERT INTO quest (title) VALUES (?)`,
		"\n\t insert into \"note\" (title) values (?)",
		`INSERT INTO secret (quest_id) SELECT 1 ON CONFLICT DO NOTHING`,
	}
	no := []string{
		`INSERT INTO quest_tag (quest_id, tag_id) VALUES (?, ?)`,
		`INSERT INTO template_tag (template_id, tag_id) VALUES (?, ?)`,
		`INSERT INTO quest (title) VALUES (?) RETURNING id`,
		`UPDATE quest SET title = ?`,
		`SELECT 1`,
	}
	for _, q := range yes {
		if !insertReturningID(q) {
			t.Errorf("insertReturningID(%q) = false", q)
		}
	}
	for _, q := range no {
		if insertReturningID(q) {
			t.Errorf("insertReturningID(%q) = true", q)
		}
	}
}

// openTestPG applies the PG baseline into a throwaway schema of the database
// named by QUESTS_TEST_PG_URL (skips when unset) and returns a pool pinned to it.
func openTestPG(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("QUESTS_TEST_PG_URL")
	if url == "" {
		t.Skip("QUESTS_TEST_PG_URL not set")
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("t_%d", time.Now().UnixNano())
	cfg.RuntimeParams["search_path"] = schema
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
		_ = admin.Close()
	})
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	baseline, err := os.ReadFile(filepath.Join("..", "..", "..", "alembic", "versions_pg", "baseline.sql"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := admin.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), `SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), string(baseline)); err != nil {
		t.Fatalf("apply baseline: %v", err)
	}
	pool := sql.OpenDB(pgConnector{stdlib.GetConnector(*cfg)})
	t.Cleanup(func() { _ = pool.Close() })
	return pool
}

func TestPGLastInsertIDAndPlaceholders(t *testing.T) {
	pool := openTestPG(t)
	ctx := context.Background()
	res, err := pool.ExecContext(ctx, `INSERT INTO tag (slug, label, color, created_at) VALUES (?, ?, ?, ?)`,
		"a", "A", "#000", "2026-01-02 03:04:05.000000")
	if err != nil {
		t.Fatal(err)
	}
	id1, _ := res.LastInsertId()
	res, err = pool.ExecContext(ctx, `INSERT INTO tag (slug, label, color, created_at) VALUES (?, ?, ?, ?)`,
		"b", "B", "#000", "2026-01-02 03:04:05.000000")
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := res.LastInsertId()
	if id1 == 0 || id2 != id1+1 {
		t.Fatalf("LastInsertId: %d, %d", id1, id2)
	}
	// Link tables have no id column: plain exec path.
	if _, err := pool.ExecContext(ctx, `INSERT INTO quest_tag (quest_id, tag_id) VALUES (?, ?)`, 1, id1); err == nil {
		t.Fatal("expected FK violation on quest_tag (no quest 1)")
	}
	// Conflict that inserts nothing reports zero rows, not an error.
	res, err = pool.ExecContext(ctx, `INSERT INTO tag (slug, label, color, created_at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		"a", "A2", "#000", "2026-01-02 03:04:05.000000")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("RowsAffected = %d, want 0", n)
	}
	var label string
	if err := pool.QueryRowContext(ctx, `SELECT label FROM tag WHERE lower(slug) LIKE ?`, "b").Scan(&label); err != nil || label != "B" {
		t.Fatalf("select: %q %v", label, err)
	}
	// Timestamps round-trip as strings the store layer parses.
	var created string
	if err := pool.QueryRowContext(ctx, `SELECT created_at FROM tag WHERE id = ?`, id1).Scan(&created); err != nil || created == "" {
		t.Fatalf("created_at: %q %v", created, err)
	}
	// Transactions go through the same wrapper.
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after the explicit rollback below
	res, err = tx.ExecContext(ctx, `INSERT INTO tag (slug, label, color, created_at) VALUES (?, ?, ?, ?)`,
		"c", "C", "#000", "2026-01-02 03:04:05.000000")
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := res.LastInsertId(); id <= id2 {
		t.Fatalf("tx LastInsertId = %d", id)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}
