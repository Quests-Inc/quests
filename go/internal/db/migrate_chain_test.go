package db

import "testing"

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
