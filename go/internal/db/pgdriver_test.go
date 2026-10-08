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

	"github.com/valirum/quests/go/internal/wsctx"
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
	// If the workspaces migration ever ran on this server the pool drops to quests_app.
	var hasRole int
	if err := conn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM pg_roles WHERE rolname = 'quests_app'`).Scan(&hasRole); err == nil && hasRole > 0 {
		for _, g := range []string{
			`GRANT USAGE ON SCHEMA ` + schema + ` TO quests_app`,
			`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ` + schema + ` TO quests_app`,
			`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ` + schema + ` TO quests_app`,
		} {
			if _, err := conn.ExecContext(context.Background(), g); err != nil {
				t.Fatal(err)
			}
		}
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

// openTestPGWorkspaces is openTestPG plus the workspaces migration, with the
// quests_app grants pointed at the throwaway schema. The pool drops to
// quests_app (as the API does), so RLS applies. Needs a role that may CREATE ROLE.
func openTestPGWorkspaces(t *testing.T) (*sql.DB, string) {
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
	conn, err := admin.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"1a2b3c4d5e6f_pg_baseline.py", "c7e1a9b3d5f2_workspaces.sql"} {
		path := filepath.Join("..", "..", "..", "alembic", "versions_pg", f)
		if f == "1a2b3c4d5e6f_pg_baseline.py" {
			path = filepath.Join("..", "..", "..", "alembic", "versions_pg", "baseline.sql")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, string(b)); err != nil {
			t.Fatalf("apply %s: %v", f, err)
		}
	}
	for _, g := range []string{
		`GRANT USAGE ON SCHEMA ` + schema + ` TO quests_app`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA ` + schema + ` TO quests_app`,
		`GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ` + schema + ` TO quests_app`,
	} {
		if _, err := conn.ExecContext(ctx, g); err != nil {
			t.Fatal(err)
		}
	}
	pool := sql.OpenDB(pgConnector{stdlib.GetConnector(*cfg)})
	t.Cleanup(func() { _ = pool.Close() })
	return pool, schema
}

func TestPGWorkspaceIsolation(t *testing.T) {
	pool, _ := openTestPGWorkspaces(t)
	bg := context.Background()
	ws1, ws2 := wsctx.With(bg, 1), wsctx.With(bg, 2)
	if _, err := pool.ExecContext(bg, `INSERT INTO workspace (id, name) VALUES (2, 'two') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	now := "2026-01-02 03:04:05.000000"
	insertQuest := func(ctx context.Context, title string) (int64, error) {
		res, err := pool.ExecContext(ctx, `INSERT INTO quest (title, description, status, pinned, sort_order, created_at, updated_at)
			VALUES (?, '', 'active', 0, 0, ?, ?)`, title, now, now)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	count := func(ctx context.Context) int {
		var n int
		if err := pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM quest`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// A workspace-less context sees and writes nothing (fail closed).
	if _, err := insertQuest(bg, "orphan"); err == nil {
		t.Fatal("insert without a workspace must fail")
	}
	id1, err := insertQuest(ws1, "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := insertQuest(ws2, "two"); err != nil {
		t.Fatal(err)
	}
	if n := count(bg); n != 0 {
		t.Fatalf("no workspace sees %d rows", n)
	}
	if n1, n2 := count(ws1), count(ws2); n1 != 1 || n2 != 1 {
		t.Fatalf("ws1=%d ws2=%d, want 1/1", n1, n2)
	}
	// Another workspace can neither read, change nor delete the row.
	var title string
	if err := pool.QueryRowContext(ws2, `SELECT title FROM quest WHERE id = ?`, id1).Scan(&title); err != sql.ErrNoRows {
		t.Fatalf("cross-workspace read: %q %v", title, err)
	}
	res, err := pool.ExecContext(ws2, `UPDATE quest SET title = 'hacked' WHERE id = ?`, id1)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("cross-workspace update touched %d rows", n)
	}
	res, err = pool.ExecContext(ws2, `DELETE FROM quest WHERE id = ?`, id1)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("cross-workspace delete touched %d rows", n)
	}
	// Writing a row into another workspace is rejected by the policy.
	if _, err := pool.ExecContext(ws2, `INSERT INTO quest (title, description, status, pinned, sort_order, created_at, updated_at, workspace_id)
		VALUES ('smuggled', '', 'active', 0, 0, ?, ?, 1)`, now, now); err == nil {
		t.Fatal("inserting into a foreign workspace must fail")
	}
	// The same connection flips workspaces correctly, also inside a transaction.
	tx, err := pool.BeginTx(ws2, nil)
	if err != nil {
		t.Fatal(err)
	}
	var inTx int
	if err := tx.QueryRowContext(ws2, `SELECT COUNT(*) FROM quest`).Scan(&inTx); err != nil || inTx != 1 {
		t.Fatalf("tx count=%d err=%v", inTx, err)
	}
	if _, err := tx.ExecContext(ws2, `INSERT INTO quest (title, description, status, pinned, sort_order, created_at, updated_at)
		VALUES ('rolled back', '', 'active', 0, 0, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if n1, n2 := count(ws1), count(ws2); n1 != 1 || n2 != 1 {
		t.Fatalf("after rollback ws1=%d ws2=%d, want 1/1", n1, n2)
	}
	// Per-workspace uniqueness: both workspaces can own the same category slug.
	for _, ctx := range []context.Context{ws1, ws2} {
		if _, err := pool.ExecContext(ctx, `INSERT INTO questcategory (slug, label, sort_order, created_at) VALUES ('work', 'Work', 0, ?)`, now); err != nil {
			t.Fatalf("category per workspace: %v", err)
		}
	}
	if _, err := pool.ExecContext(ws1, `INSERT INTO questcategory (slug, label, sort_order, created_at) VALUES ('work', 'Again', 0, ?)`, now); err == nil {
		t.Fatal("duplicate slug within a workspace must fail")
	}
}

func TestPGBoolArgsAreIntegers(t *testing.T) {
	pool := openTestPG(t)
	ctx := context.Background()
	if _, err := pool.ExecContext(ctx, `INSERT INTO questcategory (slug, label, sort_order, created_at) VALUES ('c', 'C', 0, '2026-01-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	// A Go bool into an INTEGER 0/1 column (attachment.hidden, runner_job.timed_out, …).
	if _, err := pool.ExecContext(ctx, `INSERT INTO note (title, description, pinned, sort_order, created_at, updated_at, is_readme) VALUES ('n', '', ?, 0, '2026-01-01', '2026-01-01', ?)`, true, false); err != nil {
		t.Fatalf("bool args: %v", err)
	}
	var pinned, readme int
	if err := pool.QueryRowContext(ctx, `SELECT pinned, is_readme FROM note WHERE title = 'n'`).Scan(&pinned, &readme); err != nil || pinned != 1 || readme != 0 {
		t.Fatalf("pinned=%d readme=%d err=%v", pinned, readme, err)
	}
}
