package schedule

import (
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
)

// A note check is a run of a graph: one run and one job per check, the run keeps what each node did, and an open job for
// the note keeps the next tick from queueing another.
func TestNoteCheckIsARunOfAGraph(t *testing.T) {
	r, ctx := noteCheckRig(t)
	cmd, label := "echo 1", testLabel
	if _, err := r.Store.Knowledge.SetNoteCheck(ctx, 1, &cmd, nil, &label); err != nil {
		t.Fatal(err)
	}

	r.TickNotes(ctx)
	jobs, _ := r.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel})
	if len(jobs) != 1 || jobs[0].RunID == nil || jobs[0].RunNode == nil || *jobs[0].RunNode != 2 || jobs[0].NoteID == nil {
		t.Fatalf("one job bound to the Code node of a run: %+v", jobs)
	}
	run, err := r.Store.Execution.GetRun(ctx, *jobs[0].RunID)
	if err != nil || run.State != execution.RunWaiting || run.GraphRef != "implicit:note_check" || run.SubjectType != "note" || run.SubjectID != 1 {
		t.Fatalf("the run waits for its job: %+v %v", run, err)
	}

	// Another tick while the job is open adds neither a job nor a run.
	if _, err := r.Store.DB.ExecContext(ctx, `UPDATE note SET check_last_run_at = NULL WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	r.TickNotes(ctx)
	var runs int
	_ = r.Store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM script_run`).Scan(&runs)
	if jobs, _ = r.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel}); len(jobs) != 1 || runs != 1 {
		t.Fatalf("nothing may pile up: %d jobs, %d runs", len(jobs), runs)
	}

	settle(t, r.Store, r)
	run, _ = r.Store.Execution.GetRun(ctx, run.ID)
	if run.State != execution.RunDone || run.FinishedAt == nil {
		t.Fatalf("the run is done: %+v", run)
	}
	st := run.Snapshot.States
	if st[4] != graph.StateRan || st[5] != graph.StateSkipped || st[6] != graph.StateSkipped {
		t.Fatalf("the verdict ran, the error nodes did not: %v", st)
	}
	if got, _ := r.Store.Knowledge.GetNoteCheck(ctx, 1); !got.Outdated {
		t.Fatalf("the verdict reached the note: %+v", got)
	}
}

// Whatever goes wrong with the check ends on one of the error nodes, with the reason in words.
func TestNoteCheckFailuresEndOnTheErrorNodes(t *testing.T) {
	r, ctx := noteCheckRig(t)
	for _, tc := range []struct{ cmd, want string }{
		{`echo "boom" >&2; exit 3`, "код 3: boom"},
		{`echo maybe`, "не JSON"},
		{`echo '{"reason":"x"}'`, `"outdated"`},
		{`echo '{"outdated": 5}'`, "true/false"},
	} {
		got := runNoteCheck(t, r, ctx, tc.cmd)
		if !strings.Contains(got.Error, tc.want) {
			t.Errorf("%s: error %q, want it to contain %q", tc.cmd, got.Error, tc.want)
		}
	}
}

func TestOutcomeOfAJob(t *testing.T) {
	exit3 := 3
	for _, tc := range []struct {
		job  execution.RunnerJob
		port string
		text string
	}{
		{execution.RunnerJob{State: execution.JobSuccess, Stdout: "42"}, "stdout", "42"},
		{execution.RunnerJob{State: execution.JobFail, ExitCode: &exit3, Stderr: "no luck"}, "failed", "код 3: no luck"},
		{execution.RunnerJob{State: execution.JobFail, ExitCode: &exit3, TimedOut: true}, "timeout", "таймаут"},
		{execution.RunnerJob{State: execution.JobTimeout}, "timeout", "таймаут"},
		{execution.RunnerJob{State: execution.JobSkipped, RunnerLabel: "pc"}, "unreachable", "устройство «pc» не вышло на связь"},
		{execution.RunnerJob{State: execution.JobAbandoned, RunnerLabel: "pc"}, "abandoned", "пропало во время выполнения"},
		{execution.RunnerJob{State: execution.JobCanceled}, "canceled", ""},
	} {
		o, ok := outcomeOf(tc.job)
		if !ok || o.Port != tc.port || !strings.Contains(o.Text, tc.text) {
			t.Errorf("%s: got %+v ok=%v, want port %s with %q", tc.job.State, o, ok, tc.port, tc.text)
		}
	}
	// A lost lease says nothing: the device may still deliver.
	if _, ok := outcomeOf(execution.RunnerJob{State: execution.JobLost}); ok {
		t.Error("a lost job must not resume the run")
	}
}

func TestBakeCommand(t *testing.T) {
	for _, tc := range []struct{ text, lang, want string }{
		{"echo 1", "", "echo 1"},
		{"echo 1", "sh", "echo 1"},
		{"print(1)", "python", "#!/usr/bin/env python3\nprint(1)"},
		{"x", "node", "#!/usr/bin/env node\nx"},
		{"echo 1", "bash", "#!/usr/bin/env bash\necho 1"},
		{"#!/bin/sh\necho 1", "python", "#!/bin/sh\necho 1"}, // the line in the text wins
		{"\n  #!/usr/bin/perl\nprint 1", "python", "\n  #!/usr/bin/perl\nprint 1"},
	} {
		if got := bakeCommand(tc.text, tc.lang); got != tc.want {
			t.Errorf("bake(%q,%q) = %q, want %q", tc.text, tc.lang, got, tc.want)
		}
	}
}

// A run whose script was deleted while the job waited ends failed, with the reason, instead of hanging.
func TestRunWithADeletedScriptFails(t *testing.T) {
	r, ctx := noteCheckRig(t)
	id, err := r.Store.Execution.CreateRun(ctx, execution.NewScriptRun{GraphRef: "script:99:1", SubjectType: "note", SubjectID: 1})
	if err != nil {
		t.Fatal(err)
	}
	node := 2
	job := execution.RunnerJob{State: execution.JobSuccess, Stdout: "1", RunID: &id, RunNode: &node}
	r.resumeRun(ctx, job)
	run, _ := r.Store.Execution.GetRun(ctx, id)
	if run.State != execution.RunFailed || !strings.Contains(run.Error, "скрипт удалён") {
		t.Fatalf("%+v", run)
	}
}

func onceRig(t *testing.T, cmd string) (*CheckRunner, journal.Quest) {
	t.Helper()
	st := openRunnerChecksDB(t)
	idle := journal.RunIdle
	step := remoteStep("once", cmd, testLabel, journal.RunModeOnce)
	step.RunStatus = &idle
	q := seedQuest(t, st, []journal.Step{step})
	return NewCheckRunner(st, events.New()), q
}

// A one-shot step is a run too: the job carries the run, and the outcome decides between closing the step and failing the
// quest, with the reason written in the quest log.
func TestOnceStepOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, cmd string
		status    journal.QuestStatus
		run       string
		comment   string
	}{
		{"clean exit closes the step", "true", journal.StatusCompleted, journal.RunSuccess, ""},
		{"a failing command fails the quest", `echo "boom" >&2; exit 4`, journal.StatusFailed, journal.RunFail, "код 4: boom"},
	} {
		r, q := onceRig(t, tc.cmd)
		ctx := bg()
		r.Tick(ctx)
		jobs, _ := r.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel})
		if len(jobs) != 1 || jobs[0].RunID == nil || jobs[0].StepID == nil || jobs[0].Kind != execution.JobKindOnce {
			t.Fatalf("%s: the job belongs to a run: %+v", tc.name, jobs)
		}
		settle(t, r.Store, r)
		got, _ := r.Store.Journal.GetQuest(ctx, q.ID)
		if got.Status != tc.status || got.Steps[0].RunStatus == nil || *got.Steps[0].RunStatus != tc.run {
			t.Errorf("%s: status %s run %v", tc.name, got.Status, got.Steps[0].RunStatus)
		}
		if tc.comment != "" {
			var found bool
			rows, _ := r.Store.DB.QueryContext(ctx, `SELECT COALESCE(comment,'') FROM questchangelog WHERE quest_id = ?`, q.ID)
			for rows.Next() {
				var c string
				_ = rows.Scan(&c)
				found = found || strings.Contains(c, tc.comment)
			}
			rows.Close()
			if !found {
				t.Errorf("%s: no log comment with %q", tc.name, tc.comment)
			}
		}
		var open int
		_ = r.Store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM script_run WHERE state = 'waiting'`).Scan(&open)
		if open != 0 {
			t.Errorf("%s: %d runs still wait", tc.name, open)
		}
	}
}

// A cancel by hand stops the step without failing the quest, and a second tick does not start it again.
func TestOnceStepCancelStopsItWithoutFailingTheQuest(t *testing.T) {
	r, q := onceRig(t, "sleep 30")
	ctx := bg()
	r.Tick(ctx)
	jobs, _ := r.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel})
	if len(jobs) != 1 {
		t.Fatalf("%+v", jobs)
	}
	if err := r.Store.Execution.CancelRunnerJob(ctx, jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	done, _ := r.Store.Execution.GetRunnerJob(ctx, jobs[0].ID)
	r.JobFinished(ctx, done)
	got, _ := r.Store.Journal.GetQuest(ctx, q.ID)
	if got.Status != journal.StatusActive || got.Steps[0].RunStatus == nil || *got.Steps[0].RunStatus != journal.RunFail {
		t.Fatalf("stopped, not failed: status %s run %v", got.Status, got.Steps[0].RunStatus)
	}
	r.Tick(ctx)
	if again, _ := r.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel}); len(again) != 1 {
		t.Fatalf("a stopped step must not start again by itself: %d jobs", len(again))
	}
}

func pollRig(t *testing.T, cmd string) (*CheckRunner, journal.Quest) {
	t.Helper()
	st := openRunnerChecksDB(t)
	q := seedQuest(t, st, []journal.Step{remoteStep("poll", cmd, testLabel, journal.RunModePoll)})
	return NewCheckRunner(st, events.New()), q
}

// A polled step is a run of a graph: whatever the answer carries lands on the step, and the check is recorded as run either
// way, also when the command failed or the answer was not one.
func TestPolledStepReadsItsAnswer(t *testing.T) {
	for _, tc := range []struct {
		name, cmd string
		progress  int
		total     int
		desc      string
	}{
		{"a bare integer", `echo 3`, 3, 5, ""},
		{"a log above the integer", `echo working; echo 4`, 4, 5, ""},
		{"an object", `echo '{"progress": 2, "total": 8, "description": "half"}'`, 2, 8, "half"},
		{"progress clamped to the total", `echo 99`, 5, 5, ""},
		{"only a total", `echo '{"total": 9}'`, 0, 9, ""},
		{"a failing command changes nothing", `echo 7; exit 2`, 0, 5, ""},
		{"a total below one is ignored", `echo '{"progress": 1, "total": 0}'`, 1, 5, ""},
		{"free text is not an answer", `echo "готово: 5"`, 0, 5, ""},
		{"an empty answer", `true`, 0, 5, ""},
	} {
		r, q := pollRig(t, tc.cmd)
		ctx := bg()
		r.Tick(ctx)
		settle(t, r.Store, r)
		got, _ := r.Store.Journal.GetQuest(ctx, q.ID)
		st := got.Steps[0]
		if st.CheckLastRunAt == nil {
			t.Errorf("%s: the check must be recorded as run", tc.name)
		}
		if st.ProgressCurrent != tc.progress || st.ProgressTotal != tc.total || st.Description != tc.desc {
			t.Errorf("%s: progress %d total %d desc %q, want %d %d %q", tc.name, st.ProgressCurrent, st.ProgressTotal, st.Description, tc.progress, tc.total, tc.desc)
		}
	}
}

func TestPolledStepQueuesOneRunAtATime(t *testing.T) {
	r, _ := pollRig(t, `echo 1`)
	ctx := bg()
	r.Tick(ctx)
	if _, err := r.Store.DB.Exec(`UPDATE queststep SET check_last_run_at = NULL`); err != nil {
		t.Fatal(err)
	}
	r.Tick(ctx)
	r.Wait()
	jobs, _ := r.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel})
	var runs int
	_ = r.Store.DB.QueryRow(`SELECT COUNT(*) FROM script_run`).Scan(&runs)
	if len(jobs) != 1 || runs != 1 || jobs[0].RunID == nil {
		t.Fatalf("one job and one run while the device is away: %d jobs, %d runs", len(jobs), runs)
	}
}
