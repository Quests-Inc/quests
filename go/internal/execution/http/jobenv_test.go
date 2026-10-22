package executionhttp

import (
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func TestJobEnvValuesOverrideCodeVarsSecretsWin(t *testing.T) {
	env := jobEnv(
		map[string]string{"TOKEN": "secret"},
		map[string]string{"REF": "baked", "QUESTS_IN_N1_X": "1"},
		map[string]string{"REF": "from-note", "STR": "3", "TOKEN": "nope", "QUESTS_SUBJECT_ID": "9"},
	)
	if env["REF"] != "from-note" || env["STR"] != "3" || env["TOKEN"] != "secret" || env["QUESTS_IN_N1_X"] != "1" {
		t.Fatalf("env = %v", env)
	}
	if _, ok := env["QUESTS_SUBJECT_ID"]; ok {
		t.Fatal("a value must not plant QUESTS_*")
	}
}

func TestJobEnvDropsValuesPastTheCap(t *testing.T) {
	env := jobEnv(nil, map[string]string{"KEEP": "1"}, map[string]string{"BIG": strings.Repeat("x", 40<<10)})
	if env["KEEP"] != "1" {
		t.Fatalf("link lost: %v", env)
	}
	if _, ok := env["BIG"]; ok {
		t.Fatal("oversized value kept")
	}
}

func TestJobValuesReadsTheNote(t *testing.T) {
	pool := pgtest.New(t)
	st := execution.New(pool, nil)
	ctx := wsctx.With(t.Context(), 1)
	res, err := st.DB.ExecContext(ctx, `INSERT INTO note (title, created_at, updated_at) VALUES ('n', '2026-01-01', '2026-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := st.SetNamedValue(ctx, execution.SecretOwner{Kind: execution.SecretOwnerNote, ID: id}, "str", "int", "3"); err != nil {
		t.Fatal(err)
	}
	h := &Handler{Store: st}
	got := h.jobValues(ctx, execution.RunnerJob{NoteID: &id})
	if got["STR"] != "3" {
		t.Fatalf("job values = %v", got)
	}
}
