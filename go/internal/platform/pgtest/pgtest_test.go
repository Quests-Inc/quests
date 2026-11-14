package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/db"
)

func TestSplitStatementsAndHelpers(t *testing.T) {
	got := splitStatements("a; b ; 'c;d'; \n e")
	if len(got) != 4 || got[0] != "a" || got[1] != "b" || got[2] != "'c;d'" || got[3] != "e" {
		t.Fatalf("%q", got)
	}
	if firstLine("one\ntwo") != "one" || firstLine("solo") != "solo" {
		t.Fatal(firstLine("one\ntwo"), firstLine("solo"))
	}
	if trimSpace(" \n\tx\r\n ") != "x" || trimSpace("") != "" {
		t.Fatal(trimSpace(" \n\tx\r\n "))
	}
	if withDatabase(t, "postgres://u:p@localhost:5432/postgres?sslmode=disable", "qt_x") !=
		"postgres://u:p@localhost:5432/qt_x?sslmode=disable" {
		t.Fatal(withDatabase(t, "postgres://u:p@localhost:5432/postgres?sslmode=disable", "qt_x"))
	}
}

func TestURLSkipsWithoutEnv(t *testing.T) {
	t.Setenv(envURL, "")
	t.Run("missing", func(t *testing.T) {
		URL(t)
		t.Fatal("expected skip when QUESTS_TEST_PG_URL is empty")
	})
}

func TestEnsureNamedTemplateBuildAndCache(t *testing.T) {
	base := URL(t)
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx := context.Background()
	tpl := fmt.Sprintf("qt_tpl_cov_%d_%d", os.Getpid(), time.Now().UnixNano()%1_000_000_000)
	key := int64(0x7174636f) // "qtco" — separate from the shared suite lock
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + tpl + ` WITH (FORCE)`)
		_, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + tpl + `_building WITH (FORCE)`)
	})
	_, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + tpl + ` WITH (FORCE)`)
	_, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + tpl + `_building WITH (FORCE)`)

	got, err := ensureNamedTemplate(ctx, admin, base, tpl, key)
	if err != nil || got != tpl {
		t.Fatalf("build: got=%q err=%v", got, err)
	}
	got, err = ensureNamedTemplate(ctx, admin, base, tpl, key)
	if err != nil || got != tpl {
		t.Fatalf("cache: got=%q err=%v", got, err)
	}
	// Shared ensureTemplate path (exists=true once the suite template is present).
	shared, err := ensureTemplate(ctx, admin, base)
	if err != nil || shared != "qt_tpl_"+db.ExpectedRevision() {
		t.Fatalf("shared: got=%q err=%v", shared, err)
	}
}

func TestSeedHelpersAndOwner(t *testing.T) {
	pool, dbURL := NewWithURL(t)
	Seed(t, pool, "SELECT 1; SELECT 'a;b'")
	qid := SeedQuest(t, pool, "pgtest-q")
	tid := SeedTemplate(t, pool, "pgtest-t")
	if qid <= 0 || tid <= 0 {
		t.Fatalf("ids q=%d t=%d", qid, tid)
	}
	owner := Owner(t, dbURL)
	if err := owner.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	FixSequences(t, dbURL)
	ExecOwner(t, dbURL, `SELECT 1`)
}
