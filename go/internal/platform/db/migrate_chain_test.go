package db

import "testing"

func TestEmbeddedMigrations(t *testing.T) {
	// Existing databases were created by Alembic at this baseline: the chain must keep it as its root.
	if pgMigrations[0].Revision != "1a2b3c4d5e6f" {
		t.Fatalf("chain must start at the baseline 1a2b3c4d5e6f, got %s", pgMigrations[0].Revision)
	}
	if revisionOrder[len(revisionOrder)-1] != headRevision {
		t.Fatalf("revisionOrder must end with headRevision")
	}
	seen := map[string]bool{}
	for _, r := range revisionOrder {
		if seen[r] {
			t.Errorf("revision %s twice in revisionOrder", r)
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
