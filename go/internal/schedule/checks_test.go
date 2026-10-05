package schedule

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/timeutil"
)

const checksSchema = `
CREATE TABLE questcategory (
	id INTEGER PRIMARY KEY,
	slug TEXT NOT NULL,
	label TEXT NOT NULL,
	sort_order INTEGER NOT NULL DEFAULT 0,
	color TEXT NOT NULL DEFAULT '#9a9a9a',
	created_at DATETIME NOT NULL
);
CREATE TABLE questline (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	category_id INTEGER,
	color TEXT NOT NULL DEFAULT '#9a9a9a',
	icon TEXT NOT NULL DEFAULT 'document',
	custom_icon TEXT,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE TABLE quest (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	significance TEXT NOT NULL,
	pinned INTEGER NOT NULL DEFAULT 0,
	sort_order INTEGER NOT NULL DEFAULT 0,
	deadline_at DATETIME,
	duration_seconds INTEGER,
	category_id INTEGER,
	questline_id INTEGER,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	completed_at DATETIME,
	template_id INTEGER,
	period_key TEXT,
	automated INTEGER NOT NULL DEFAULT 0,
	source TEXT
);
CREATE TABLE queststep (
	id INTEGER PRIMARY KEY,
	quest_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	progress_current INTEGER NOT NULL DEFAULT 0,
	progress_total INTEGER NOT NULL DEFAULT 1,
	sort_order INTEGER NOT NULL DEFAULT 0,
	check_command TEXT,
	check_interval_seconds INTEGER,
	check_last_run_at DATETIME,
	wait_previous INTEGER NOT NULL DEFAULT 0,
	run_mode TEXT NOT NULL DEFAULT 'poll',
	run_status TEXT
);
CREATE TABLE questchangelog (
	id INTEGER PRIMARY KEY,
	at DATETIME NOT NULL,
	kind TEXT NOT NULL,
	quest_id INTEGER,
	title TEXT NOT NULL DEFAULT '',
	detail TEXT NOT NULL DEFAULT '',
	significance TEXT,
	revision INTEGER,
	comment TEXT,
	entity_type TEXT NOT NULL DEFAULT 'quest',
	entity_id INTEGER,
	parent_type TEXT,
	parent_id INTEGER
);
CREATE TABLE tag (
	id INTEGER PRIMARY KEY,
	slug TEXT NOT NULL UNIQUE,
	label TEXT NOT NULL,
	color TEXT NOT NULL DEFAULT '#9a9a9a',
	created_at DATETIME NOT NULL
);
CREATE TABLE quest_tag (
	quest_id INTEGER NOT NULL,
	tag_id INTEGER NOT NULL,
	PRIMARY KEY (quest_id, tag_id)
);
CREATE TABLE secret (
	id INTEGER PRIMARY KEY,
	questline_id INTEGER, template_id INTEGER, quest_id INTEGER, step_id INTEGER,
	key TEXT NOT NULL,
	value TEXT NOT NULL,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX ux_secret_questline_id_key ON secret (questline_id, key) WHERE questline_id IS NOT NULL;
CREATE UNIQUE INDEX ux_secret_quest_id_key ON secret (quest_id, key) WHERE quest_id IS NOT NULL;
CREATE UNIQUE INDEX ux_secret_step_id_key ON secret (step_id, key) WHERE step_id IS NOT NULL;
CREATE TABLE template_tag (
	template_id INTEGER NOT NULL,
	tag_id INTEGER NOT NULL,
	PRIMARY KEY (template_id, tag_id)
);
`

func openChecksDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := sql.Open("sqlite", "file:checks_"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(checksSchema); err != nil {
		t.Fatal(err)
	}
	return &store.Store{DB: db}
}

func TestParseCheckStdout(t *testing.T) {
	n, ok := ParseCheckStdout("3\n")
	if !ok || n != 3 {
		t.Fatalf("got %d %v", n, ok)
	}
	n, ok = ParseCheckStdout("files: 12")
	if !ok || n != 12 {
		t.Fatalf("got %d %v", n, ok)
	}
	if _, ok := ParseCheckStdout("nope"); ok {
		t.Fatal("expected miss")
	}
}

func TestParseCheckOutputJSON(t *testing.T) {
	got := ParseCheckOutput(`{"progress": 4, "description": "половина"}`)
	if !got.ProgressOK || got.Progress != 4 {
		t.Fatalf("progress=%d ok=%v", got.Progress, got.ProgressOK)
	}
	if got.Description == nil || *got.Description != "половина" {
		t.Fatalf("description=%v", got.Description)
	}

	bare := ParseCheckOutput("2\n")
	if !bare.ProgressOK || bare.Progress != 2 || bare.Description != nil {
		t.Fatalf("bare=%+v", bare)
	}

	broken := ParseCheckOutput(`{not json`)
	if broken.ProgressOK || broken.Description != nil {
		t.Fatalf("broken should not update: %+v", broken)
	}

	noProgress := ParseCheckOutput(`{"description": "только текст"}`)
	if noProgress.ProgressOK {
		t.Fatal("missing progress must not set ProgressOK")
	}
	if noProgress.Description == nil || *noProgress.Description != "только текст" {
		t.Fatalf("description=%v", noProgress.Description)
	}

	badProgress := ParseCheckOutput(`{"progress": "x", "description": "keep?"}`)
	if badProgress.ProgressOK {
		t.Fatal("non-int progress must not set ProgressOK")
	}

	withTotal := ParseCheckOutput(`{"progress": 2, "total": 7}`)
	if !withTotal.ProgressOK || !withTotal.TotalOK || withTotal.Total != 7 {
		t.Fatalf("total not parsed: %+v", withTotal)
	}
	if only := ParseCheckOutput(`{"total": 3}`); only.ProgressOK || !only.TotalOK || only.Total != 3 {
		t.Fatalf("total-only: %+v", only)
	}
	for _, bad := range []string{`{"total": 0}`, `{"total": -2}`, `{"total": "x"}`, `{"total": 1.5}`} {
		if got := ParseCheckOutput(bad); got.TotalOK {
			t.Fatalf("%s must not set TotalOK: %+v", bad, got)
		}
	}
}

func seedQuest(t *testing.T, st *store.Store, steps []domain.Step) domain.Quest {
	t.Helper()
	now := timeutil.NowUTC()
	q, err := st.CreateQuest(context.Background(), domain.Quest{
		Title: "auto", Status: domain.StatusActive, Significance: domain.SigCommon,
		CreatedAt: now, UpdatedAt: now, Automated: true, Steps: steps,
	})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestCheckPollProgress(t *testing.T) {
	st := openChecksDB(t)
	cmd := "echo 2"
	iv := 15
	q := seedQuest(t, st, []domain.Step{{
		Title: "poll", ProgressTotal: 5, SortOrder: 0,
		CheckCommand: &cmd, CheckIntervalSeconds: &iv, RunMode: domain.RunModePoll,
	}})
	r := NewCheckRunner(st, events.New())
	r.Tick(context.Background())
	r.Wait()
	got, err := st.GetQuest(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Steps[0].ProgressCurrent != 2 {
		t.Fatalf("progress=%d want 2", got.Steps[0].ProgressCurrent)
	}
}

func TestCheckPollJSON(t *testing.T) {
	st := openChecksDB(t)
	cmd := `printf '{"progress":1,"description":"из JSON"}'`
	iv := 15
	q := seedQuest(t, st, []domain.Step{{
		Title: "poll-json", ProgressTotal: 2, SortOrder: 0,
		CheckCommand: &cmd, CheckIntervalSeconds: &iv, RunMode: domain.RunModePoll,
	}})
	r := NewCheckRunner(st, events.New())
	r.Tick(context.Background())
	r.Wait()
	got, err := st.GetQuest(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Steps[0].ProgressCurrent != 1 {
		t.Fatalf("progress=%d", got.Steps[0].ProgressCurrent)
	}
	if got.Steps[0].Description != "из JSON" {
		t.Fatalf("description=%q", got.Steps[0].Description)
	}
	if got.Status != domain.StatusActive {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestCheckPollJSONBrokenKeepsDescription(t *testing.T) {
	st := openChecksDB(t)
	cmd := `printf 'not-json'`
	iv := 15
	q := seedQuest(t, st, []domain.Step{{
		Title: "poll-broken", Description: "оставить", ProgressTotal: 2, SortOrder: 0,
		CheckCommand: &cmd, CheckIntervalSeconds: &iv, RunMode: domain.RunModePoll,
		ProgressCurrent: 1,
	}})
	r := NewCheckRunner(st, events.New())
	r.Tick(context.Background())
	r.Wait()
	got, err := st.GetQuest(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Steps[0].Description != "оставить" {
		t.Fatalf("description wiped: %q", got.Steps[0].Description)
	}
	if got.Steps[0].ProgressCurrent != 1 {
		t.Fatalf("progress changed: %d", got.Steps[0].ProgressCurrent)
	}
}

func TestCheckWaitPrevious(t *testing.T) {
	st := openChecksDB(t)
	cmd1 := "true"
	cmd2 := "true"
	idle := domain.RunIdle
	q := seedQuest(t, st, []domain.Step{
		{
			Title: "one", ProgressTotal: 1, SortOrder: 0,
			CheckCommand: &cmd1, RunMode: domain.RunModeOnce, RunStatus: &idle,
		},
		{
			Title: "two", ProgressTotal: 1, SortOrder: 1,
			CheckCommand: &cmd2, RunMode: domain.RunModeOnce, RunStatus: &idle,
			WaitPrevious: true,
		},
	})
	r := NewCheckRunner(st, events.New())
	r.Tick(context.Background())
	r.Wait()
	got, err := st.GetQuest(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Steps[0].Done {
		t.Fatal("first step should complete on first tick")
	}
	if got.Steps[1].Done {
		t.Fatal("second step must wait for previous")
	}
	r.Tick(context.Background())
	r.Wait()
	got, err = st.GetQuest(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Steps[1].Done {
		t.Fatal("second step should run after previous is done")
	}
}

func TestCheckOnceFailQuest(t *testing.T) {
	st := openChecksDB(t)
	cmd := "false"
	idle := domain.RunIdle
	q := seedQuest(t, st, []domain.Step{{
		Title: "boom", ProgressTotal: 1, SortOrder: 0,
		CheckCommand: &cmd, RunMode: domain.RunModeOnce, RunStatus: &idle,
	}})
	r := NewCheckRunner(st, events.New())
	r.Tick(context.Background())
	r.Wait()
	got, err := st.GetQuest(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFailed {
		t.Fatalf("status=%s want failed", got.Status)
	}
	if got.Steps[0].RunStatus == nil || *got.Steps[0].RunStatus != domain.RunFail {
		t.Fatalf("run_status=%v", got.Steps[0].RunStatus)
	}
	var comment sql.NullString
	err = st.DB.QueryRow(`SELECT comment FROM questchangelog WHERE kind='quest_failed' ORDER BY id DESC LIMIT 1`).Scan(&comment)
	if err != nil || !comment.Valid || comment.String == "" {
		t.Fatalf("expected changelog comment, err=%v comment=%v", err, comment)
	}
}

func TestFailStaleRunning(t *testing.T) {
	st := openChecksDB(t)
	cmd := "sleep 99"
	run := domain.RunRunning
	q := seedQuest(t, st, []domain.Step{{
		Title: "stuck", ProgressTotal: 1, SortOrder: 0,
		CheckCommand: &cmd, RunMode: domain.RunModeOnce, RunStatus: &run,
	}})
	r := NewCheckRunner(st, events.New())
	r.FailStaleRunning(context.Background())
	got, err := st.GetQuest(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.StatusFailed {
		t.Fatalf("status=%s", got.Status)
	}
	_ = time.Second
}

func pollStep(t *testing.T, st *store.Store, cmd string, total, current int) domain.Quest {
	t.Helper()
	iv := 15
	q := seedQuest(t, st, []domain.Step{{
		Title: "mirror", ProgressTotal: total, ProgressCurrent: current, SortOrder: 0,
		CheckCommand: &cmd, CheckIntervalSeconds: &iv, RunMode: domain.RunModePoll,
	}})
	r := NewCheckRunner(st, events.New())
	r.Tick(context.Background())
	r.Wait()
	got, err := st.GetQuest(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A step that mirrors another quest follows it while it is open: when the
// mirrored quest gains steps the maximum grows and progress is applied
// against the new maximum. (A step already at its maximum is final, as before.)
func TestCheckPollTotalGrows(t *testing.T) {
	st := openChecksDB(t)
	got := pollStep(t, st, `printf '{"progress":3,"total":5}'`, 4, 2)
	step := got.Steps[0]
	if step.ProgressTotal != 5 || step.ProgressCurrent != 3 || step.Done {
		t.Fatalf("want 3/5 not done, got %d/%d done=%v", step.ProgressCurrent, step.ProgressTotal, step.Done)
	}
}

func TestCheckPollTotalOnlyKeepsProgress(t *testing.T) {
	st := openChecksDB(t)
	got := pollStep(t, st, `printf '{"total":6}'`, 4, 2)
	step := got.Steps[0]
	if step.ProgressTotal != 6 || step.ProgressCurrent != 2 {
		t.Fatalf("want 2/6, got %d/%d", step.ProgressCurrent, step.ProgressTotal)
	}
}

func TestCheckPollTotalShrinkClampsProgressAndCompletes(t *testing.T) {
	st := openChecksDB(t)
	got := pollStep(t, st, `printf '{"progress":5,"total":2}'`, 6, 1)
	step := got.Steps[0]
	if step.ProgressTotal != 2 || step.ProgressCurrent != 2 || !step.Done {
		t.Fatalf("want 2/2 done, got %d/%d done=%v", step.ProgressCurrent, step.ProgressTotal, step.Done)
	}
	if got.Status != domain.StatusCompleted {
		t.Fatalf("status=%s, want completed", got.Status)
	}
}

// Bad total (zero) is ignored; progress still applies against the old max.
func TestCheckPollInvalidTotalIgnored(t *testing.T) {
	st := openChecksDB(t)
	got := pollStep(t, st, `printf '{"progress":2,"total":0}'`, 5, 0)
	step := got.Steps[0]
	if step.ProgressTotal != 5 || step.ProgressCurrent != 2 {
		t.Fatalf("want 2/5, got %d/%d", step.ProgressCurrent, step.ProgressTotal)
	}
}

func watchStep(cmd string, total, current int, mode string) domain.Step {
	iv := 15
	return domain.Step{
		Title: "mirror", ProgressTotal: total, ProgressCurrent: current, SortOrder: 0,
		CheckCommand: &cmd, CheckIntervalSeconds: &iv, RunMode: mode,
	}
}

func tickAndGet(t *testing.T, st *store.Store, id int64) domain.Quest {
	t.Helper()
	r := NewCheckRunner(st, events.New())
	r.Tick(context.Background())
	r.Wait()
	got, err := st.GetQuest(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A watch step that already reached its maximum keeps being polled: when the
// followed quest grows, the step re-opens (and so does its quest).
func TestCheckWatchReopensClosedStepWhenTotalGrows(t *testing.T) {
	st := openChecksDB(t)
	q := seedQuest(t, st, []domain.Step{watchStep(`printf '{"progress":3,"total":5}'`, 3, 3, domain.RunModeWatch)})
	got := tickAndGet(t, st, q.ID)
	step := got.Steps[0]
	if step.ProgressTotal != 5 || step.ProgressCurrent != 3 || step.Done {
		t.Fatalf("want 3/5 re-opened, got %d/%d done=%v", step.ProgressCurrent, step.ProgressTotal, step.Done)
	}
	if got.Status != domain.StatusActive {
		t.Fatalf("status=%s, want active", got.Status)
	}
}

// Plain poll steps stay final once closed — the old behaviour.
func TestCheckPollKeepsClosedStepFinal(t *testing.T) {
	st := openChecksDB(t)
	q := seedQuest(t, st, []domain.Step{watchStep(`printf '{"progress":3,"total":5}'`, 3, 3, domain.RunModePoll)})
	step := tickAndGet(t, st, q.ID).Steps[0]
	if step.ProgressTotal != 3 || !step.Done {
		t.Fatalf("closed poll step must not be polled, got %d/%d done=%v", step.ProgressCurrent, step.ProgressTotal, step.Done)
	}
}

// Watching is limited to active quests: a completed quest's watch step is not
// polled (this bounds the polling load, deliberately).
func TestCheckWatchIgnoresInactiveQuests(t *testing.T) {
	st := openChecksDB(t)
	now := timeutil.NowUTC()
	step := watchStep(`printf '{"progress":3,"total":5}'`, 3, 3, domain.RunModeWatch)
	step.Done = true
	q, err := st.CreateQuest(context.Background(), domain.Quest{
		Title: "done-parent", Status: domain.StatusCompleted, Significance: domain.SigCommon,
		CreatedAt: now, UpdatedAt: now, Automated: true, Steps: []domain.Step{step},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := tickAndGet(t, st, q.ID)
	if got.Steps[0].ProgressTotal != 3 || got.Status != domain.StatusCompleted {
		t.Fatalf("completed quest must be left alone, got total=%d status=%s", got.Steps[0].ProgressTotal, got.Status)
	}
}

// A closed watch step polls at most once a minute, an open one at its interval.
func TestCheckWatchClosedStepIsPolledLazily(t *testing.T) {
	st := openChecksDB(t)
	q := seedQuest(t, st, []domain.Step{watchStep(`printf '{"progress":3,"total":5}'`, 3, 3, domain.RunModeWatch)})
	ctx := context.Background()
	recent := timeutil.NowUTC().Add(-30 * time.Second) // past 15s, not past 60s
	if _, err := st.DB.ExecContext(ctx, `UPDATE queststep SET check_last_run_at = ? WHERE quest_id = ?`, timeutil.ToDBUTC(recent), q.ID); err != nil {
		t.Skipf("cannot seed check_last_run_at: %v", err)
	}
	step := tickAndGet(t, st, q.ID).Steps[0]
	if step.ProgressTotal != 3 {
		t.Fatalf("closed watch step polled after only 30s, total=%d", step.ProgressTotal)
	}
	old := timeutil.NowUTC().Add(-90 * time.Second)
	if _, err := st.DB.ExecContext(ctx, `UPDATE queststep SET check_last_run_at = ? WHERE quest_id = ?`, timeutil.ToDBUTC(old), q.ID); err != nil {
		t.Fatal(err)
	}
	if got := tickAndGet(t, st, q.ID).Steps[0]; got.ProgressTotal != 5 {
		t.Fatalf("closed watch step should be polled after 90s, total=%d", got.ProgressTotal)
	}
}

func TestNormalizeRunModeKeepsWatch(t *testing.T) {
	for in, want := range map[string]string{"watch": "watch", " WATCH ": "watch", "once": "once", "poll": "poll", "": "poll", "bogus": "poll"} {
		if got := store.NormalizeRunMode(in); got != want {
			t.Errorf("NormalizeRunMode(%q) = %q, want %q", in, got, want)
		}
	}
}

// A check command sees the secrets of its questline (inherited) and of the step
// itself (most specific wins), and what it prints has them masked.
func TestCheckCommandGetsInheritedSecretsAndMasksOutput(t *testing.T) {
	st := openChecksDB(t)
	ctx := context.Background()
	if _, err := st.DB.ExecContext(ctx, `INSERT INTO questline (id, title, created_at, updated_at) VALUES (7, 'line', datetime('now'), datetime('now'))`); err != nil {
		t.Fatal(err)
	}
	cmd := `printf '{"progress":%s,"description":"site said %s / %s"}' "$([ "$SITE_PASS" = step-pass ] && [ "$SITE_USER" = line-user ] && echo 1 || echo 0)" "$SITE_PASS" "$SITE_USER"`
	iv := 15
	now := timeutil.NowUTC()
	q, err := st.CreateQuest(ctx, domain.Quest{
		Title: "q", Status: domain.StatusActive, Significance: domain.SigCommon,
		CreatedAt: now, UpdatedAt: now, Automated: true,
		QuestlineID: func() *int64 { v := int64(7); return &v }(),
		Steps:       []domain.Step{{Title: "s", ProgressTotal: 2, SortOrder: 0, CheckCommand: &cmd, CheckIntervalSeconds: &iv, RunMode: domain.RunModePoll}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stepID := q.Steps[0].ID
	if err := st.SetSecret(ctx, store.SecretOwner{Kind: store.SecretOwnerQuestline, ID: 7}, "SITE_USER", "line-user"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSecret(ctx, store.SecretOwner{Kind: store.SecretOwnerQuestline, ID: 7}, "SITE_PASS", "line-pass"); err != nil {
		t.Fatal(err)
	}
	// the step overrides the questline's value
	if err := st.SetSecret(ctx, store.SecretOwner{Kind: store.SecretOwnerStep, ID: stepID}, "SITE_PASS", "step-pass"); err != nil {
		t.Fatal(err)
	}

	got := tickAndGet(t, st, q.ID)
	step := got.Steps[0]
	if step.ProgressCurrent != 1 {
		t.Fatalf("command did not see the inherited/overridden secrets: progress=%d", step.ProgressCurrent)
	}
	if step.Description != "site said *** / ***" {
		t.Fatalf("secret values must be masked in the step description, got %q", step.Description)
	}
}
