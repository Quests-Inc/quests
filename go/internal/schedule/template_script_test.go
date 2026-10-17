package schedule

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/store"
)

// scriptedTemplate seeds a template whose emit is a script made of the given command.
func scriptedTemplate(t *testing.T, command string) (*store.Store, templateRow) {
	t.Helper()
	st := openEmitPoolDB(t)
	ctx := bg()
	created, err := st.Automation.CreateTemplate(ctx, map[string]any{"title": "Ежедневный", "enabled": false, "emit_run_on": testLabel})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(graph.TemplateEmit(command))
	sc, err := st.Execution.CreateScript(ctx, "эмит", "", raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Execution.BindScript(ctx, execution.SubjectTemplate, created.ID, sc.ID); err != nil {
		t.Fatal(err)
	}
	tmpl, err := loadTemplateRow(ctx, st, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	return st, tmpl
}

// The script's code goes out as a job; what it prints comes back through the graph as the quest of the period.
func TestEmitScriptRunsTheGraphAndHandsOverTheQuest(t *testing.T) {
	st, tmpl := scriptedTemplate(t, `echo '{"title":"Из скрипта","steps":[{"title":"Шаг"}]}'`)
	r := NewCheckRunner(st, events.New())
	ctx := bg()

	if first := emitTick(t, st, tmpl, "2026-10-18", time.Now()); first.Spec != nil || first.FailMsg != "" {
		t.Fatalf("the first tick only queues: %+v", first)
	}
	jobs, _ := st.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel})
	if len(jobs) != 1 || jobs[0].RunID == nil || jobs[0].TemplateID == nil || *jobs[0].TemplateID != tmpl.ID {
		t.Fatalf("one job of the run: %+v", jobs)
	}
	// A tick while the device works neither queues again nor fails.
	if again := emitTick(t, st, tmpl, "2026-10-18", time.Now()); again.Spec != nil || again.FailMsg != "" {
		t.Fatalf("waiting tick: %+v", again)
	}
	if runQueuedJobs(t, st, r) != 1 {
		t.Fatal("the device had one job")
	}
	got := emitTick(t, st, tmpl, "2026-10-18", time.Now())
	if got.Spec == nil || got.Spec.Title != "Из скрипта" || len(got.Spec.Steps) != 1 {
		t.Fatalf("the quest did not come back: %+v", got)
	}
	var runs int
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM script_run`).Scan(&runs)
	if runs != 0 {
		t.Fatalf("a run that was taken stays (%d)", runs)
	}
}

// A command that fails is an attempt error of the template: the emit retries like it does for a plain command.
func TestEmitScriptFailureIsAnAttemptError(t *testing.T) {
	st, tmpl := scriptedTemplate(t, `echo "нет сети" >&2; exit 3`)
	r := NewCheckRunner(st, events.New())
	emitTick(t, st, tmpl, "p", time.Now())
	runQueuedJobs(t, st, r)
	res := emitTick(t, st, tmpl, "p", time.Now())
	var outcome string
	var attempts int
	if err := st.DB.QueryRow(`SELECT outcome, attempts FROM templateemitroll WHERE id = ?`, res.RollID).Scan(&outcome, &attempts); err != nil {
		t.Fatal(err)
	}
	if outcome != "scheduled" || attempts != 1 {
		t.Fatalf("outcome=%s attempts=%d, want a scheduled retry", outcome, attempts)
	}
	var msg string
	_ = st.DB.QueryRow(`SELECT message FROM templateemitattempt ORDER BY id DESC LIMIT 1`).Scan(&msg)
	if !strings.Contains(msg, "нет сети") {
		t.Fatalf("stderr missing from the attempt log: %q", msg)
	}
}

// An answer that is not JSON is the bad_json attempt; `{}` is "no quest this period".
func TestEmitScriptBadAnswerAndNoQuest(t *testing.T) {
	st, tmpl := scriptedTemplate(t, `echo не-json`)
	r := NewCheckRunner(st, events.New())
	emitTick(t, st, tmpl, "p", time.Now())
	runQueuedJobs(t, st, r)
	res := emitTick(t, st, tmpl, "p", time.Now())
	var status string
	_ = st.DB.QueryRow(`SELECT status FROM templateemitattempt WHERE template_id = ? ORDER BY id DESC LIMIT 1`, tmpl.ID).Scan(&status)
	if res.Spec != nil || status != "bad_json" {
		t.Fatalf("spec=%+v status=%q", res.Spec, status)
	}

	st2, tmpl2 := scriptedTemplate(t, `echo '{}'`)
	r2 := NewCheckRunner(st2, events.New())
	emitTick(t, st2, tmpl2, "p", time.Now())
	runQueuedJobs(t, st2, r2)
	if res := emitTick(t, st2, tmpl2, "p", time.Now()); res.Spec != nil || res.FailMsg != "" {
		t.Fatalf("{} makes no quest and no error: %+v", res)
	}
}

// A template step's script goes with the step into the quest it makes.
func TestMaterializedStepKeepsItsScript(t *testing.T) {
	st := openEmitPoolDB(t)
	ctx := bg()
	raw, _ := json.Marshal(graph.StepPoll("echo 1"))
	sc, err := st.Execution.CreateScript(ctx, "шаг", "", raw)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := st.Automation.CreateTemplate(ctx, map[string]any{
		"title": "С шагом", "enabled": false,
		"steps": []any{map[string]any{"title": "Скриптовый", "script_id": sc.ID, "run_on": testLabel, "run_mode": "poll"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tmpl.Steps) != 1 || tmpl.Steps[0].ScriptID == nil || *tmpl.Steps[0].ScriptID != sc.ID {
		t.Fatalf("the template step lost its script: %+v", tmpl.Steps)
	}
	qid, err := MaterializeTemplateManual(ctx, st, events.New(), tmpl.ID, time.Now(), rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatal(err)
	}
	q, _ := st.Journal.GetQuest(ctx, qid)
	if len(q.Steps) != 1 || q.Steps[0].ScriptID == nil || *q.Steps[0].ScriptID != sc.ID || q.Steps[0].CheckCommand != nil {
		t.Fatalf("the quest's step: %+v", q.Steps)
	}
	// The script is in use now, and the list says where.
	uses, _ := st.Execution.ScriptUses(ctx, sc.ID)
	kinds := map[string]bool{}
	for _, u := range uses {
		kinds[u.SubjectType] = true
	}
	if !kinds["template_step"] || !kinds["step"] {
		t.Fatalf("uses: %+v", uses)
	}
}

// A step created together with its quest is bound to the script it names, as long as that script reads steps.
func TestStepCreatedWithAScriptIsBound(t *testing.T) {
	st := openEmitPoolDB(t)
	ctx := bg()
	raw, _ := json.Marshal(graph.StepPoll("echo 1"))
	sc, _ := st.Execution.CreateScript(ctx, "шаг", "", raw)
	noteRaw, _ := json.Marshal(graph.NoteCheck("echo 0"))
	note, _ := st.Execution.CreateScript(ctx, "заметка", "", noteRaw)

	mk := func(scriptID int64) journal.Quest {
		now := timeutil.NowUTC()
		return journal.Quest{
			Title: "со скриптом", Status: journal.StatusActive, Significance: journal.SigCommon, CreatedAt: now, UpdatedAt: now,
			Steps: []journal.Step{{Title: "шаг", ProgressTotal: 1, ScriptID: &scriptID}},
		}
	}
	created, err := st.Journal.CreateQuest(ctx, mk(sc.ID))
	if err != nil {
		t.Fatal(err)
	}
	if created.Steps[0].ScriptID == nil || *created.Steps[0].ScriptID != sc.ID {
		t.Fatalf("not bound: %+v", created.Steps[0])
	}
	if _, err := st.Journal.CreateQuest(ctx, mk(note.ID)); err == nil {
		t.Fatal("a script that reads notes must not bind to a step")
	}
}
