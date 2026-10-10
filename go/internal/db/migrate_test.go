package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"
)

// freshDatabase creates an empty database on the server named by QUESTS_TEST_PG_URL
// and returns its URL (skips without the variable).
func freshDatabase(t *testing.T) string {
	t.Helper()
	base := os.Getenv("QUESTS_TEST_PG_URL")
	if base == "" {
		t.Skip("QUESTS_TEST_PG_URL not set")
	}
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("qt_mig_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP DATABASE ` + name + ` WITH (FORCE)`)
		_ = admin.Close()
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

func versionOf(t *testing.T, dbURL string) string {
	t.Helper()
	v, err := CurrentRevision(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMigratePostgresFreshThenIdempotent(t *testing.T) {
	dbURL := freshDatabase(t)
	applied, err := MigratePostgres(context.Background(), dbURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(pgMigrations) {
		t.Fatalf("applied %d, want %d", len(applied), len(pgMigrations))
	}
	if got := versionOf(t, dbURL); got != expectedAlembic {
		t.Fatalf("version %q, want %q", got, expectedAlembic)
	}
	again, err := MigratePostgres(context.Background(), dbURL, nil)
	if err != nil || len(again) != 0 {
		t.Fatalf("second run: applied=%v err=%v", again, err)
	}
	// And the API opens it as current.
	sqlDB, st, err := OpenPostgres(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if st.State != "ok" {
		t.Fatalf("schema state %q: %s", st.State, st.Detail)
	}
}

// A database Alembic left at an intermediate revision is continued, not rebuilt.
func TestMigratePostgresResumesFromAlembicRevision(t *testing.T) {
	dbURL := freshDatabase(t)
	raw, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	const upTo = 3
	for _, m := range pgMigrations[:upTo] {
		if _, err := raw.Exec(m.SQL); err != nil {
			t.Fatalf("seed %s: %v", m.Name, err)
		}
	}
	for _, q := range []string{
		`CREATE TABLE alembic_version (version_num VARCHAR(32) NOT NULL, CONSTRAINT alembic_version_pkc PRIMARY KEY (version_num))`,
		`INSERT INTO alembic_version VALUES ('` + pgMigrations[upTo-1].Revision + `')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	applied, err := MigratePostgres(context.Background(), dbURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) != len(pgMigrations)-upTo || applied[0] != pgMigrations[upTo].Revision {
		t.Fatalf("applied %v", applied)
	}
	if got := versionOf(t, dbURL); got != expectedAlembic {
		t.Fatalf("version %q", got)
	}
}

func TestMigratePostgresLeavesNewerDatabaseAlone(t *testing.T) {
	dbURL := freshDatabase(t)
	if _, err := MigratePostgres(context.Background(), dbURL, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := sql.Open("pgx", dbURL)
	defer raw.Close()
	if _, err := raw.Exec(`UPDATE alembic_version SET version_num = '9999'`); err != nil {
		t.Fatal(err)
	}
	applied, err := MigratePostgres(context.Background(), dbURL, nil)
	if err != nil || len(applied) != 0 {
		t.Fatalf("applied=%v err=%v", applied, err)
	}
	if got := versionOf(t, dbURL); got != "9999" {
		t.Fatalf("version rewritten to %q", got)
	}
}

func TestMigratePostgresRefusesUnversionedData(t *testing.T) {
	dbURL := freshDatabase(t)
	raw, _ := sql.Open("pgx", dbURL)
	defer raw.Close()
	if _, err := raw.Exec(`CREATE TABLE quest (id SERIAL PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := MigratePostgres(context.Background(), dbURL, nil); err == nil {
		t.Fatal("must refuse a database with tables but no alembic_version")
	}
}

func TestMigratePostgresRollsBackBrokenMigration(t *testing.T) {
	dbURL := freshDatabase(t)
	if _, err := MigratePostgres(context.Background(), dbURL, nil); err != nil {
		t.Fatal(err)
	}
	raw, _ := sql.Open("pgx", dbURL)
	defer raw.Close()
	bad := Migration{Seq: 99, Revision: "0099", Name: "broken", SQL: `CREATE TABLE half_done (id INT); SELECT nope FROM nowhere;`}
	conn, _ := raw.Conn(context.Background())
	defer conn.Close()
	if err := applyPG(context.Background(), conn, bad, true); err == nil {
		t.Fatal("broken migration must fail")
	}
	var n int
	_ = raw.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'half_done'`).Scan(&n)
	if n != 0 || versionOf(t, dbURL) != expectedAlembic {
		t.Fatalf("a failed migration left traces: tables=%d version=%s", n, versionOf(t, dbURL))
	}
}

func TestMigrateBeforeApplyRunsOnlyForExistingDatabases(t *testing.T) {
	// fresh database: nothing to protect, the hook is not called
	fresh := freshDatabase(t)
	called := false
	if _, err := MigratePostgresOpts(context.Background(), fresh, MigrateOptions{BeforeApply: func(context.Context, string, []Migration) error {
		called = true
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("BeforeApply must not run on a fresh database")
	}

	// a database stopped at revision 3: the hook sees where it is and what is coming
	dbURL := freshDatabase(t)
	raw, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, m := range pgMigrations[:3] {
		if _, err := raw.Exec(m.SQL); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{versionTable, `INSERT INTO alembic_version VALUES ('` + pgMigrations[2].Revision + `')`} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var from string
	var got int
	hook := func(_ context.Context, f string, todo []Migration) error {
		from, got = f, len(todo)
		return nil
	}
	if _, err := MigratePostgresOpts(context.Background(), dbURL, MigrateOptions{BeforeApply: hook}); err != nil {
		t.Fatal(err)
	}
	if from != pgMigrations[2].Revision || got != len(pgMigrations)-3 {
		t.Fatalf("hook saw from=%q todo=%d", from, got)
	}

	// a failing hook stops the migration with nothing applied
	stuck := freshDatabase(t)
	raw2, _ := sql.Open("pgx", stuck)
	defer raw2.Close()
	if _, err := raw2.Exec(pgMigrations[0].SQL); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{versionTable, `INSERT INTO alembic_version VALUES ('` + pgMigrations[0].Revision + `')`} {
		if _, err := raw2.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	applied, err := MigratePostgresOpts(context.Background(), stuck, MigrateOptions{BeforeApply: func(context.Context, string, []Migration) error {
		return errors.New("no backup, no migration")
	}})
	if err == nil || len(applied) != 0 {
		t.Fatalf("a failing hook must stop the migration: applied=%v err=%v", applied, err)
	}
	if v := versionOf(t, stuck); v != pgMigrations[0].Revision {
		t.Fatalf("revision moved to %q", v)
	}
}
