package schedule

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/pgtest"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/store"
)

// bg is the context tests run in: workspace 1, the one the helpers below set up.
func bg() context.Context { return wsctx.With(context.Background(), 1) }

// openChecksDB is a real database with templates 1..40 already there for the emit tables that
// reference them.
func openChecksDB(t *testing.T) *store.Store {
	t.Helper()
	pool, url := pgtest.NewWithURL(t)
	for id := 1; id <= 40; id++ {
		if _, err := pool.Exec(`INSERT INTO questtemplate (id, title, created_at, updated_at) VALUES (?, 't', now(), now())`, id); err != nil {
			t.Fatal(err)
		}
	}
	pgtest.FixSequences(t, url)
	return &store.Store{DB: pool, DatabaseURL: url}
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
	label := testLabel
	for i := range steps { // commands run on runners only: the fake device in these tests carries them out
		if steps[i].CheckCommand != nil && steps[i].RunOn == nil {
			steps[i].RunOn = &label
		}
	}
	q, err := st.CreateQuest(bg(), domain.Quest{
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
	r.Tick(bg())
	settle(t, st, r)
	got, err := st.GetQuest(bg(), q.ID)
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
	r.Tick(bg())
	settle(t, st, r)
	got, err := st.GetQuest(bg(), q.ID)
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
	r.Tick(bg())
	settle(t, st, r)
	got, err := st.GetQuest(bg(), q.ID)
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
	r.Tick(bg())
	settle(t, st, r)
	got, err := st.GetQuest(bg(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Steps[0].Done {
		t.Fatal("first step should complete on first tick")
	}
	if got.Steps[1].Done {
		t.Fatal("second step must wait for previous")
	}
	r.Tick(bg())
	settle(t, st, r)
	got, err = st.GetQuest(bg(), q.ID)
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
	r.Tick(bg())
	settle(t, st, r)
	got, err := st.GetQuest(bg(), q.ID)
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

// A one-shot with no runner that was "running" when the server stopped (a step stored before commands
// were limited to runners) can never finish: it fails. One on a runner keeps waiting for its result.
func TestFailStaleRunning(t *testing.T) {
	st := openChecksDB(t)
	cmd := "sleep 99"
	run := domain.RunRunning
	q := seedQuest(t, st, []domain.Step{{
		Title: "stuck", ProgressTotal: 1, CheckCommand: &cmd, RunMode: domain.RunModeOnce, RunStatus: &run,
	}})
	// the way such a step is stored from before: a command and no runner
	if _, err := st.DB.Exec(`UPDATE queststep SET run_on = NULL WHERE quest_id = ?`, q.ID); err != nil {
		t.Fatal(err)
	}
	r := NewCheckRunner(st, events.New())
	r.FailStaleRunning(bg())
	got, err := st.GetQuest(bg(), q.ID)
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
	r.Tick(bg())
	settle(t, st, r)
	got, err := st.GetQuest(bg(), q.ID)
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
	label := testLabel
	return domain.Step{
		Title: "mirror", ProgressTotal: total, ProgressCurrent: current, SortOrder: 0,
		CheckCommand: &cmd, CheckIntervalSeconds: &iv, RunMode: mode, RunOn: &label,
	}
}

func tickAndGet(t *testing.T, st *store.Store, id int64) domain.Quest {
	t.Helper()
	r := NewCheckRunner(st, events.New())
	r.Tick(bg())
	settle(t, st, r)
	got, err := st.GetQuest(bg(), id)
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
	q, err := st.CreateQuest(bg(), domain.Quest{
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
	ctx := bg()
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
