package db

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The embedded PostgreSQL chain began as a copy of alembic/versions_pg, which is kept
// as an archive (alembic/README.md). While the archive is there, make sure the copy
// is faithful: same revisions in the same order, byte-identical SQL.
func TestEmbeddedChainMatchesAlembicArchive(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "alembic", "versions_pg")
	files, _ := filepath.Glob(filepath.Join(dir, "*.py"))
	if len(files) == 0 {
		t.Skip("alembic archive not present")
	}
	revRe := regexp.MustCompile(`(?m)^revision(?:: str)? = ["']([0-9a-f]+)["']`)
	downRe := regexp.MustCompile(`(?m)^down_revision(?:: [^=]+)? = (?:["']([0-9a-f]+)["']|None)`)
	down := map[string]string{} // revision -> down_revision
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		r, d := revRe.FindSubmatch(b), downRe.FindSubmatch(b)
		if r == nil || d == nil {
			t.Fatalf("%s: no revision/down_revision", f)
		}
		down[string(r[1])] = string(d[1])
	}
	archived := 0
	prev := ""
	for _, m := range pgMigrations {
		if len(m.Revision) != 12 { // Go-era revision, not in the archive
			continue
		}
		archived++
		if down[m.Revision] != prev {
			t.Errorf("%s: archive says it follows %q, embedded chain has %q", m.Revision, down[m.Revision], prev)
		}
		prev = m.Revision
		src := m.Name
		path := filepath.Join(dir, m.Revision+"_"+src+".sql")
		if m.Seq == 1 {
			path = filepath.Join(dir, "baseline.sql")
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != m.SQL {
			t.Errorf("%s: SQL differs from the archive copy %s", m.Revision, path)
		}
	}
	if archived != len(files) {
		t.Errorf("archive has %d revisions, embedded chain has %d of them", len(files), archived)
	}
}

func TestEmbeddedMigrations(t *testing.T) {
	if len(sqliteMigrations) != 1 || sqliteMigrations[0].Revision != expectedAlembicSQLite {
		t.Fatalf("sqlite chain must be the single baseline %s", expectedAlembicSQLite)
	}
	if pgMigrations[0].Revision != expectedAlembicSQLite {
		t.Fatalf("pg chain must start at the shared baseline, got %s", pgMigrations[0].Revision)
	}
	if alembicOrder[len(alembicOrder)-1] != expectedAlembic {
		t.Fatalf("alembicOrder must end with expectedAlembic")
	}
	seen := map[string]bool{}
	for _, r := range alembicOrder {
		if seen[r] {
			t.Errorf("revision %s twice in alembicOrder", r)
		}
		seen[r] = true
		if len(r) > 32 {
			t.Errorf("revision %q does not fit alembic_version(32)", r)
		}
	}
}

func TestPending(t *testing.T) {
	ms := []Migration{{Revision: "a"}, {Revision: "b"}, {Revision: "c"}}
	if todo, ok := pending(ms, ""); !ok || len(todo) != 3 {
		t.Fatalf("empty db: want all 3, got %d ok=%v", len(todo), ok)
	}
	if todo, ok := pending(ms, "b"); !ok || len(todo) != 1 || todo[0].Revision != "c" {
		t.Fatalf("at b: want [c], got %v ok=%v", todo, ok)
	}
	if todo, ok := pending(ms, "c"); !ok || len(todo) != 0 {
		t.Fatalf("at head: want nothing, got %v", todo)
	}
	if _, ok := pending(ms, "zzz"); ok {
		t.Fatal("unknown revision must be reported as ahead")
	}
}
