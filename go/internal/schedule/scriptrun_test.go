package schedule

import (
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
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
