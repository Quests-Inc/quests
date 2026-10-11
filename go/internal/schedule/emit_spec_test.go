package schedule

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/journal"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/pgtest"
	"github.com/valirum/quests/go/internal/store"
)

func TestParseEmitOutput(t *testing.T) {
	cases := map[string]string{ // input -> kind
		"":              "nothing",
		"  \n":          "nothing",
		"null":          "nothing",
		"{}":            "nothing",
		`{"title":"a"}`: "spec",
	}
	for in, want := range cases {
		out, err := parseEmitOutput([]byte(in))
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		got := "nothing"
		switch {
		case out.Spec != nil:
			got = "spec"
		}
		if got != want {
			t.Errorf("%q: got %s, want %s", in, got, want)
		}
	}
	for _, bad := range []string{`"text"`, `42`, `[]`, `[{"title":"a"}]`, `{not json`, `{"title": 5}`, `{"steps": {"a":1}}`, `{"steps":[{"title":"x","progress_total":"many"}]}`} {
		if _, err := parseEmitOutput([]byte(bad)); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

// Fields outside the format never fail a run: they are collected so the
// attempt log can name them (what makes the format forward compatible).
func TestParseEmitOutputCollectsIgnoredFields(t *testing.T) {
	out, err := parseEmitOutput([]byte(`{"title":"a","ref":"mail:1","weight":3,"steps":[{"title":"s","owner":"x"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ref", "steps[0].owner", "weight"}
	if !reflect.DeepEqual(out.Spec.Ignored, want) {
		t.Fatalf("ignored = %v, want %v", out.Spec.Ignored, want)
	}
}

func openEmitSpecDB(t *testing.T) *store.Store {
	t.Helper()
	pool, url := pgtest.NewWithURL(t)
	if _, err := pool.Exec(`
DELETE FROM questcategory;
INSERT INTO questcategory (id, slug, label, created_at) VALUES (1, 'work', 'Работа', now()), (2, 'home', 'Дом', now());
INSERT INTO questline (id, title, category_id, created_at, updated_at) VALUES
	(1, 'Сайт Рефкул', 1, now(), now()),
	(2, 'Рутина', 2, now(), now()),
	(3, 'Рутина дома', 2, now(), now());
INSERT INTO tag (id, slug, label, created_at) VALUES (1, 'infra', 'infra', now()), (2, 'api', 'api', now());`); err != nil {
		t.Fatal(err)
	}
	pgtest.FixSequences(t, url)
	return &store.Store{DB: pool, DatabaseURL: url}
}

func resolveJSON(t *testing.T, st *store.Store, js string, lim store.EmitLimits) (*resolvedSpec, error) {
	t.Helper()
	out, err := parseEmitOutput([]byte(js))
	if err != nil || out.Spec == nil {
		t.Fatalf("parse %q: spec=%v err=%v", js, out.Spec, err)
	}
	return resolveEmitSpec(bg(), st, out.Spec, lim)
}

func TestResolveEmitSpecNamesAndTags(t *testing.T) {
	st := openEmitSpecDB(t)
	lim := store.DefaultEmitLimits()
	rs, err := resolveJSON(t, st, `{"questline":"рефкул","category":"home","tags":["infra",2]}`, lim)
	if err != nil {
		t.Fatal(err)
	}
	if rs.QuestlineID == nil || *rs.QuestlineID != 1 || rs.CategoryID == nil || *rs.CategoryID != 2 || !rs.TagsSet || len(rs.TagIDs) != 2 {
		t.Fatalf("resolved = %+v", rs)
	}
	// exact title beats a longer title containing it
	if rs, err = resolveJSON(t, st, `{"questline":"Рутина"}`, lim); err != nil || *rs.QuestlineID != 2 {
		t.Fatalf("exact match must win: %+v %v", rs, err)
	}
	for _, bad := range []string{`{"questline":"нет такого"}`, `{"questline":"рути"}`, `{"category":"nope"}`, `{"tags":["no-such-tag"]}`, `{"significance":"medium"}`, `{"deadline_at":"tomorrow-ish"}`, `{"duration_seconds":0}`} {
		if _, err := resolveJSON(t, st, bad, lim); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
}

func TestResolveEmitSpecLimits(t *testing.T) {
	st := openEmitSpecDB(t)
	lim := store.EmitLimits{MaxSteps: 2, MaxTitle: 5, MaxDescription: 10, MaxCommand: 8}
	for js, want := range map[string]string{
		`{"steps":[{"title":"a"},{"title":"b"},{"title":"c"}]}`:  "3 steps, limit 2",
		`{"title":"toolongtitle"}`:                               "title is 12 characters, limit 5",
		`{"description":"01234567890"}`:                          "description is 11 characters, limit 10",
		`{"steps":[{"title":"a","check_command":"0123456789"}]}`: "check_command is 10 characters, limit 8",
		`{"steps":[{"title":" "}]}`:                              "title required",
	} {
		if _, err := resolveJSON(t, st, js, lim); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", js, err, want)
		}
	}
	// the limits JSON of a template overrides the defaults key by key, typos fail
	got, err := store.ParseEmitLimits(`{"max_steps": 3}`)
	if err != nil || got.MaxSteps != 3 || got.MaxTitle != 200 {
		t.Fatalf("ParseEmitLimits = %+v, %v", got, err)
	}
	for _, bad := range []string{`{"max_step": 3}`, `{"max_steps": 0}`, `{"max_steps": "x"}`, `[1]`} {
		if _, err := store.ParseEmitLimits(bad); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
}

func insertTemplate(t *testing.T, st *store.Store, id int64, cols, vals string) {
	t.Helper()
	if strings.Contains(cols, "emit_pool_command") { // commands run on runners only: the fake device carries them out
		cols, vals = cols+", emit_run_on", vals+", '"+testLabel+"'"
	}
	if _, err := st.DB.Exec(`INSERT INTO questtemplate (id, created_at, updated_at, `+cols+`) VALUES (?, now(), now(), `+vals+`)`, id); err != nil {
		t.Fatalf("insert template: %v", err)
	}
}

// materialize is a tick, the fake device running what it queued, and the tick that collects the result.
func materialize(t *testing.T, st *store.Store) ([]int64, error) {
	t.Helper()
	now := time.Now()
	if _, err := MaterializeDue(bg(), st, events.New(), now, rand.New(rand.NewSource(1))); err != nil {
		return nil, err
	}
	runQueuedJobs(t, st, nil)
	return MaterializeDue(bg(), st, events.New(), now, rand.New(rand.NewSource(1)))
}

// End to end: the quest the command prints is created with the template's
// defaults underneath, tagged with its source, and the period is closed.
func TestMaterializeDueCreatesTheQuestTheCommandPrinted(t *testing.T) {
	st := openEmitSpecDB(t)
	cmd := `echo '{"title":"Счёт МТС","significance":"uncommon","questline":"Сайт Рефкул","tags":["infra"],"deadline_at":"2030-01-02T10:00:00Z","duration_seconds":600,"ref":"ignored","steps":[{"title":"Оплатить"},{"title":"Квитанция","progress_total":2}]}'`
	insertTemplate(t, st, 7, `title, description, pinned, emit_pool_command, automated`,
		`'Шаблон', 'описание шаблона', 1, '`+strings.ReplaceAll(cmd, "'", "''")+`', 1`)
	if _, err := st.DB.Exec(`INSERT INTO template_tag (template_id, tag_id) VALUES (7, 2)`); err != nil {
		t.Fatal(err)
	}

	ids, err := materialize(t, st)
	if err != nil || len(ids) != 1 {
		t.Fatalf("created %v, err %v", ids, err)
	}
	q, err := st.GetQuest(bg(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if q.Title != "Счёт МТС" || q.Significance != journal.SigUncommon || !q.Pinned || !q.Automated {
		t.Fatalf("title/significance/template defaults wrong: %+v", q)
	}
	if q.Description != "описание шаблона" {
		t.Fatalf("a quest without its own description keeps the template's, got %q", q.Description)
	}
	if q.QuestlineID == nil || *q.QuestlineID != 1 || q.CategoryID == nil || *q.CategoryID != 1 {
		t.Fatalf("questline by name must bring its category: line=%v cat=%v", q.QuestlineID, q.CategoryID)
	}
	if q.DeadlineAt == nil || q.DeadlineAt.Year() != 2030 || q.DurationSeconds == nil || *q.DurationSeconds != 600 {
		t.Fatalf("absolute deadline not applied: %v %v", q.DeadlineAt, q.DurationSeconds)
	}
	if q.Source == nil || *q.Source != "template:7" || q.TemplateID == nil || *q.TemplateID != 7 {
		t.Fatalf("source/template wrong: %v %v", q.Source, q.TemplateID)
	}
	if len(q.Steps) != 2 || q.Steps[1].ProgressTotal != 2 {
		t.Fatalf("steps = %+v", q.Steps)
	}
	if len(q.Tags) != 1 || q.Tags[0].Slug != "infra" {
		t.Fatalf("the command's tags replace the template's, got %+v", q.Tags)
	}
	// the period is closed: a second tick creates nothing
	if ids, err := materialize(t, st); err != nil || len(ids) != 0 {
		t.Fatalf("second tick: %v %v", ids, err)
	}
	// and the attempt log names the ignored field
	var msg string
	if err := st.DB.QueryRow(`SELECT message FROM templateemitattempt WHERE template_id = 7`).Scan(&msg); err != nil || !strings.Contains(msg, "ignored fields: ref") {
		t.Fatalf("attempt message = %q (%v)", msg, err)
	}
}

// A quest in a questline carries the questline's category, as in the REST API:
// neither the template's own category nor one the command prints may drift away.
func TestMaterializeDueQuestlineCategoryWins(t *testing.T) {
	for name, cmd := range map[string]string{
		"template category": `echo '{"title":"a"}'`,
		"printed category":  `echo '{"title":"a","category":"home"}'`,
	} {
		t.Run(name, func(t *testing.T) {
			st := openEmitSpecDB(t)
			insertTemplate(t, st, 8, `title, emit_pool_command, questline_id, category_id`,
				`'Шаблон', '`+strings.ReplaceAll(cmd, "'", "''")+`', 1, 2`)
			ids, err := materialize(t, st)
			if err != nil || len(ids) != 1 {
				t.Fatalf("created %v, err %v", ids, err)
			}
			q, err := st.GetQuest(bg(), ids[0])
			if err != nil {
				t.Fatal(err)
			}
			if q.CategoryID == nil || *q.CategoryID != 1 {
				t.Errorf("category = %v, want the questline's (1)", q.CategoryID)
			}
		})
	}
}

// Nothing printed means no quest and no failure; the template's own steps are
// not used as a fallback when a command is configured.
func TestMaterializeDueNothingPrintedCreatesNothing(t *testing.T) {
	st := openEmitSpecDB(t)
	insertTemplate(t, st, 8, `title, emit_pool_command`, `'Пусто', 'echo null'`)
	ids, err := materialize(t, st)
	if err != nil || len(ids) != 0 {
		t.Fatalf("created %v, err %v", ids, err)
	}
}

// A template's own emit_limits apply to its command's output.
func TestMaterializeDueHonoursTemplateLimits(t *testing.T) {
	st := openEmitSpecDB(t)
	insertTemplate(t, st, 9, `title, emit_pool_command, emit_limits`,
		`'Лимит', 'echo ''{"steps":[{"title":"a"},{"title":"b"}]}''', '{"max_steps":1}'`)
	ids, err := materialize(t, st)
	if err != nil || len(ids) != 0 {
		t.Fatalf("over the limit must not create a quest: %v %v", ids, err)
	}
	var status, msg string
	if err := st.DB.QueryRow(`SELECT status, message FROM templateemitattempt WHERE template_id = 9`).Scan(&status, &msg); err != nil || status != "bad_spec" || !strings.Contains(msg, "2 steps, limit 1") {
		t.Fatalf("attempt = %q %q (%v)", status, msg, err)
	}
}
