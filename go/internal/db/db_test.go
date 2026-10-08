package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckAlembicVersionMatch(t *testing.T) {
	st, err := checkAlembicVersion(expectedAlembic)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "ok" {
		t.Fatalf("state=%q want ok", st.State)
	}
}

func TestCheckAlembicVersionOlderFails(t *testing.T) {
	older := alembicOrder[len(alembicOrder)-4]
	_, err := checkAlembicVersion(older)
	if err == nil {
		t.Fatal("expected error for older alembic")
	}
	if !strings.Contains(err.Error(), "older than required") {
		t.Fatalf("err=%v", err)
	}
}

func TestCheckAlembicVersionUnknownIsAhead(t *testing.T) {
	st, err := checkAlembicVersion("ffffdeadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "warn" {
		t.Fatalf("state=%q want warn", st.State)
	}
	if !strings.Contains(st.Detail, "новее") {
		t.Fatalf("detail=%q", st.Detail)
	}
}

func TestCheckAlembicVersionKnownAhead(t *testing.T) {
	order := []string{"aaa", "bbb", "ccc"}
	st, err := checkAlembicVersionAgainst("ccc", "bbb", order)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "warn" {
		t.Fatalf("state=%q want warn", st.State)
	}
}

func TestExpectedIsLastInOrder(t *testing.T) {
	if alembicOrder[len(alembicOrder)-1] != expectedAlembic {
		t.Fatalf("expectedAlembic %s must be last in alembicOrder (got %s)",
			expectedAlembic, alembicOrder[len(alembicOrder)-1])
	}
}

func TestOpenAcceptsCurrentAndRejectsOlder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quests.db")
	seed := func(ver string) {
		raw, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer raw.Close()
		if _, err := raw.Exec(`CREATE TABLE IF NOT EXISTS alembic_version (version_num VARCHAR(32) NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec(`DELETE FROM alembic_version`); err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec(`INSERT INTO alembic_version (version_num) VALUES (?)`, ver); err != nil {
			t.Fatal(err)
		}
	}

	seed(expectedAlembicSQLite)
	dbOK, st, err := Open(path)
	if err != nil {
		t.Fatalf("open current: %v", err)
	}
	dbOK.Close()
	if st.State != "ok" {
		t.Fatalf("state=%q", st.State)
	}

	seed("ffffdeadbeef")
	dbAhead, st, err := Open(path)
	if err != nil {
		t.Fatalf("open ahead: %v", err)
	}
	dbAhead.Close()
	if st.State != "warn" {
		t.Fatalf("ahead state=%q", st.State)
	}

	older := alembicOrder[len(alembicOrder)-4]
	seed(older)
	if _, _, err := Open(path); err == nil {
		t.Fatal("older db must fail open")
	}
}
