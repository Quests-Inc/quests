package db

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestExpectedAlembicIsHead keeps expectedAlembic/alembicOrder in step with
// the migrations in alembic/versions: forgetting to bump them makes a freshly
// migrated DB look "ahead of the binary" in the health probe.
//
// Two chains exist: the legacy SQLite one (alembic/versions, frozen at the
// baseline of the PostgreSQL one) and the PostgreSQL one (alembic/versions_pg).
// expectedAlembic must be the PG head and expectedAlembicSQLite the SQLite one.
func TestExpectedAlembicIsHead(t *testing.T) {
	checkAlembicChain(t, "../../../alembic/versions_pg/*.py", expectedAlembic)
	checkAlembicChain(t, "../../../alembic/versions/*.py", expectedAlembicSQLite)
}

func checkAlembicChain(t *testing.T, glob, wantHead string) {
	t.Helper()
	files, err := filepath.Glob(glob)
	if err != nil || len(files) == 0 {
		t.Skip(glob + " not found")
	}
	revRe := regexp.MustCompile(`(?m)^revision(?:: str)? = ["']([0-9a-f]+)["']`)
	downRe := regexp.MustCompile(`(?m)^down_revision(?:: [^=]+)? = ["']([0-9a-f]+)["']`)
	revs := map[string]bool{}
	downs := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if m := revRe.FindSubmatch(b); m != nil {
			revs[string(m[1])] = true
		}
		if m := downRe.FindSubmatch(b); m != nil {
			downs[string(m[1])] = true
		}
	}
	var heads []string
	for r := range revs {
		if !downs[r] {
			heads = append(heads, r)
		}
	}
	if len(heads) != 1 || heads[0] != wantHead {
		t.Fatalf("%s: alembic heads = %v, want %s", glob, heads, wantHead)
	}
	if alembicOrder[len(alembicOrder)-1] != expectedAlembic {
		t.Fatalf("alembicOrder must end with expectedAlembic")
	}
	for r := range revs {
		found := false
		for _, o := range alembicOrder {
			found = found || o == r
		}
		if !found {
			t.Errorf("revision %s missing from alembicOrder", r)
		}
	}
}
