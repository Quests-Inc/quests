package automation

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/valirum/quests/go/internal/platform/pgtest"
	"github.com/valirum/quests/go/internal/platform/secretbox"
)

const secretsTestSeed = `
INSERT INTO questline (id, title, created_at, updated_at) VALUES (1, 'line', '2026-01-01', '2026-01-01');
INSERT INTO questtemplate (id, title, questline_id, created_at, updated_at) VALUES (10, 'tmpl', 1, '2026-01-01', '2026-01-01');
INSERT INTO quest (id, title, description, status, pinned, sort_order, questline_id, template_id, created_at, updated_at)
	VALUES (100, 'quest', '', 'active', 0, 0, 1, 10, '2026-01-01', '2026-01-01');
INSERT INTO queststep (id, quest_id, title, description, progress_current, progress_total, sort_order, check_command)
	VALUES (1000, 100, 'step', '', 0, 1, 0, 'true');
`

func openSecretsDB(t *testing.T) *Store {
	t.Helper()
	pool := pgtest.New(t)
	pgtest.Seed(t, pool, secretsTestSeed)
	return &Store{DB: pool}
}

var (
	oLine  = SecretOwner{SecretOwnerQuestline, 1}
	oTmpl  = SecretOwner{SecretOwnerTemplate, 10}
	oQuest = SecretOwner{SecretOwnerQuest, 100}
	oStep  = SecretOwner{SecretOwnerStep, 1000}
)

func mustSet(t *testing.T, st *Store, o SecretOwner, k, v string) {
	t.Helper()
	if err := st.SetSecret(context.Background(), o, k, v); err != nil {
		t.Fatalf("SetSecret(%v, %s): %v", o, k, err)
	}
}

// step > quest > template > questline, per key.
func TestResolveSecretsPrecedence(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	mustSet(t, st, oLine, "A", "from-line")
	mustSet(t, st, oLine, "B", "from-line")
	mustSet(t, st, oLine, "C", "from-line")
	mustSet(t, st, oLine, "D", "from-line")
	mustSet(t, st, oTmpl, "B", "from-template")
	mustSet(t, st, oTmpl, "C", "from-template")
	mustSet(t, st, oTmpl, "D", "from-template")
	mustSet(t, st, oQuest, "C", "from-quest")
	mustSet(t, st, oQuest, "D", "from-quest")
	mustSet(t, st, oStep, "D", "from-step")

	got, err := st.ResolveSecrets(ctx, oStep)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "from-line", "B": "from-template", "C": "from-quest", "D": "from-step"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("step sees %v, want %v", got, want)
	}

	// An emit_pool_command (template) sees only template + questline.
	got, _ = st.ResolveSecrets(ctx, oTmpl)
	if got["B"] != "from-template" || got["A"] != "from-line" || got["C"] != "from-template" {
		t.Fatalf("template sees %v", got)
	}
	// The quest does not see its step's secrets.
	got, _ = st.ResolveSecrets(ctx, oQuest)
	if got["D"] != "from-quest" {
		t.Fatalf("quest sees %v", got)
	}
}

func TestListEffectiveSecretKeysNamesSources(t *testing.T) {
	st := openSecretsDB(t)
	mustSet(t, st, oLine, "SITE", "x-line")
	mustSet(t, st, oTmpl, "SITE", "x-template")
	mustSet(t, st, oQuest, "OWN", "x-quest")
	refs, err := st.ListEffectiveSecretKeys(context.Background(), oStep)
	if err != nil {
		t.Fatal(err)
	}
	want := []SecretRef{{"OWN", oQuest}, {"SITE", oTmpl}}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("effective keys %v, want %v", refs, want)
	}
	own, _ := st.ListSecretKeys(context.Background(), oStep)
	if len(own) != 0 {
		t.Fatalf("the step has no own secrets, got %v", own)
	}
}

// Deleting an owner removes its secrets (FK cascade), nothing else.
func TestSecretsCascadeWithOwner(t *testing.T) {
	st := openSecretsDB(t)
	mustSet(t, st, oStep, "S", "step-secret")
	mustSet(t, st, oQuest, "Q", "quest-secret")
	if _, err := st.DB.Exec(`DELETE FROM queststep WHERE id = 1000`); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM secret WHERE step_id = 1000`).Scan(&n)
	if n != 0 {
		t.Fatalf("step secret survived its step")
	}
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM secret WHERE quest_id = 100`).Scan(&n)
	if n != 1 {
		t.Fatalf("quest secret must stay")
	}
}

func TestSetSecretUpsertsAndDeleteIsIdempotent(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	mustSet(t, st, oQuest, "K", "one")
	mustSet(t, st, oQuest, "K", "two")
	got, _ := st.ResolveSecrets(ctx, oQuest)
	if got["K"] != "two" {
		t.Fatalf("upsert failed: %v", got)
	}
	if err := st.DeleteSecret(ctx, oQuest, "K"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSecret(ctx, oQuest, "K"); err != nil {
		t.Fatalf("deleting a missing key must not fail: %v", err)
	}
	if err := st.SetSecret(ctx, SecretOwner{"bogus", 1}, "K", "v"); err == nil {
		t.Fatal("unknown owner kind must be rejected")
	}
}

// When a template goes away its emitted quests lose template_id; the ones that
// are still open and have an auto-check keep the template's secrets.
func TestTemplateSecretsCopiedToLiveQuestsWithChecks(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	mustSet(t, st, oTmpl, "SITE_PASS", "template-pass")
	// quest 101: open, no check; quest 102: completed with a check
	_, _ = st.DB.Exec(`INSERT INTO quest (id, questline_id, template_id, status) VALUES (101, 1, 10, 'active'), (102, 1, 10, 'completed')`)
	_, _ = st.DB.Exec(`INSERT INTO queststep (id, quest_id, check_command) VALUES (1010, 101, NULL), (1020, 102, 'true')`)

	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := copyTemplateSecretsToLiveQuests(ctx, tx, 10); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]int{100: 1, 101: 0, 102: 0} {
		var n int
		_ = st.DB.QueryRow(`SELECT COUNT(*) FROM secret WHERE quest_id = ?`, id).Scan(&n)
		if n != want {
			t.Errorf("quest %d: %d copied secrets, want %d", id, n, want)
		}
	}
}

func TestMaskSecrets(t *testing.T) {
	secrets := map[string]string{"A": "hunter2", "B": "xy", "C": "tok-12345"}
	got := MaskSecrets("login hunter2 failed, xy ok, tok-12345!", secrets)
	if got != "login *** failed, xy ok, ***!" {
		t.Fatalf("got %q (values under 4 chars must be left alone)", got)
	}
}

func TestSecretsAreSealedAtRest(t *testing.T) {
	t.Setenv("QUESTS_SECRET_KEY", "")
	box, err := secretbox.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st := openSecretsDB(t)
	st.Box = box
	ctx := context.Background()

	mustSet(t, st, oLine, "TOKEN", "hunter2-plain")
	var raw string
	if err := st.DB.QueryRow(`SELECT value FROM secret WHERE key = 'TOKEN'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw == "hunter2-plain" || !strings.HasPrefix(raw, "v1:") {
		t.Fatalf("stored in the clear: %q", raw)
	}
	got, err := st.ResolveSecrets(ctx, oStep)
	if err != nil || got["TOKEN"] != "hunter2-plain" {
		t.Fatalf("resolve: %v %v", got, err)
	}

	// A row from before sealing is read as is, then sealed by the start-up pass; a plain value
	// that merely begins with the marker is sealed too.
	if _, err := st.DB.Exec(`INSERT INTO secret (quest_id, key, value, created_at, updated_at) VALUES
		(100, 'OLD', 'legacy-value', now(), now()),
		(100, 'ODD', 'v1:looks-sealed-but-is-not', now(), now())`); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.ResolveSecrets(ctx, oQuest); got["OLD"] != "legacy-value" {
		t.Fatalf("legacy row unreadable before sealing: %v", got)
	}
	n, err := st.SealLegacySecrets(ctx)
	if err != nil || n != 2 {
		t.Fatalf("SealLegacySecrets = %d, %v (want 2)", n, err)
	}
	got, _ = st.ResolveSecrets(ctx, oQuest)
	if got["OLD"] != "legacy-value" || got["ODD"] != "v1:looks-sealed-but-is-not" || got["TOKEN"] != "hunter2-plain" {
		t.Fatalf("after sealing: %v", got)
	}
	if n, _ := st.SealLegacySecrets(ctx); n != 0 {
		t.Fatalf("second pass re-sealed %d rows", n)
	}

	// Sealed under another key: left out, never handed over as ciphertext.
	other, _ := secretbox.Load(t.TempDir())
	st2 := &Store{DB: st.DB, Box: other}
	if got, _ := st2.ResolveSecrets(ctx, oQuest); len(got) != 0 {
		t.Fatalf("values readable under the wrong key: %v", got)
	}
}

func TestSealedHealthTellsWhichKeySealed(t *testing.T) {
	t.Setenv("QUESTS_SECRET_KEY", "")
	box, _ := secretbox.Load(t.TempDir())
	st := openSecretsDB(t)
	st.Box = box
	ctx := context.Background()
	mustSet(t, st, oLine, "A", "one")
	mustSet(t, st, oQuest, "B", "two")
	if _, err := st.DB.Exec(`INSERT INTO secret (step_id, key, value, created_at, updated_at) VALUES (1000, 'OLD', 'plain', now(), now())`); err != nil {
		t.Fatal(err)
	}
	secretRow := func() SealedCounts {
		h, err := st.SealedHealth(ctx)
		if err != nil {
			t.Fatalf("health: %v", err)
		}
		for _, r := range h {
			if r.Table == "secret" {
				return r
			}
		}
		t.Fatalf("no secret row in %+v", h)
		return SealedCounts{}
	}
	if c := secretRow(); c.Total != 3 || c.Readable != 2 || c.Plain != 1 || c.Unreadable != 0 {
		t.Fatalf("with the right key: %+v", c)
	}
	other, _ := secretbox.Load(t.TempDir())
	st.Box = other
	if c := secretRow(); c.Unreadable != 2 || c.Readable != 0 {
		t.Fatalf("with another key: %+v", c)
	}
}
