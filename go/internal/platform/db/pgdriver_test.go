package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/quests-inc/quests/go/internal/platform/wsctx"
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
	url := freshDatabase(t)
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	// A database of its own: the migrations pin SECURITY DEFINER functions to schema public.
	const schema = "public"
	cfg.RuntimeParams["search_path"] = schema
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	baseline := pgMigrations[0].SQL
	conn, err := admin.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(context.Background(), `SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), baseline); err != nil {
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
	pool := sql.OpenDB(pgConnector{inner: stdlib.GetConnector(*cfg)})
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
func openTestPGWorkspaces(t *testing.T) (*sql.DB, string, string) {
	t.Helper()
	url := freshDatabase(t)
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	// A database of its own: the migrations pin SECURITY DEFINER functions to schema public.
	const schema = "public"
	cfg.RuntimeParams["search_path"] = schema
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	conn, err := admin.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}
	for _, m := range pgMigrations[:2] { // baseline + workspaces
		if _, err := conn.ExecContext(ctx, m.SQL); err != nil {
			t.Fatalf("apply %s: %v", m.Name, err)
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
	pool := sql.OpenDB(pgConnector{inner: stdlib.GetConnector(*cfg)})
	t.Cleanup(func() { _ = pool.Close() })
	return pool, schema, url
}

func TestPGWorkspaceIsolation(t *testing.T) {
	pool, _, _ := openTestPGWorkspaces(t)
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

// The API role may not change what is the host's: workspace trust, password hashes, the
// schema revision (migration c8e0a2b4d6f3) — while still doing what the API needs.
func TestPGAPIRoleGrants(t *testing.T) {
	pool, schema, url := openTestPGWorkspaces(t)
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	conn, err := admin.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}
	// The helper stops at the workspaces migration; the later columns the grants name come from here.
	for _, q := range []string{
		`ALTER TABLE workspace ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP`,
		`CREATE TABLE IF NOT EXISTS alembic_version (version_num VARCHAR(32) NOT NULL)`,
		`INSERT INTO alembic_version VALUES ('v')`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON alembic_version TO quests_app`,
	} {
		if _, err := conn.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	grants := pgMigrations[7] // api_role_grants
	if grants.Name != "api_role_grants" {
		t.Fatalf("migration 8 is %s, test expects api_role_grants", grants.Name)
	}
	if _, err := conn.ExecContext(ctx, grants.SQL); err != nil {
		t.Fatalf("apply grants: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO appuser (id, username, password_hash, is_active, created_at) VALUES (1, 'u', 'hash', 1, now())`); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ name, stmt string }{
		{"rewrite a workspace's creation time", `UPDATE workspace SET created_at = now() WHERE id = 1`},
		{"rewrite a password hash", `UPDATE appuser SET password_hash = 'x' WHERE id = 1`},
		{"re-activate an account", `UPDATE appuser SET is_active = 1 WHERE id = 1`},
		{"forge the schema revision", `UPDATE alembic_version SET version_num = 'x'`},
	} {
		if _, err := pool.ExecContext(ctx, c.stmt); err == nil {
			t.Errorf("the API role could %s", c.name)
		} else if !strings.Contains(err.Error(), "permission denied") {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
	}
	for _, c := range []struct{ name, stmt string }{
		{"rename a workspace", `UPDATE workspace SET name = 'renamed' WHERE id = 1`},
		{"soft-delete a workspace", `UPDATE workspace SET deleted_at = now() WHERE id = 1`},
		{"record a login", `UPDATE appuser SET last_login_at = now() WHERE id = 1`},
	} {
		if _, err := pool.ExecContext(ctx, c.stmt); err != nil {
			t.Errorf("the API role must still %s: %v", c.name, err)
		}
	}
}
