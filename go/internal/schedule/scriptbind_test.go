package schedule

import (
	"strconv"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
)

// A step that reads one number: the source, a command, a strict parse of `x`, the progress sink.
func progressScript(text, field string) []byte {
	return []byte(`{"nodes":[
		{"id":1,"type":"source.step"},
		{"id":2,"type":"code","params":{"text":"` + text + `"}},
		{"id":3,"type":"parse","params":{"schema":[{"name":"` + field + `","type":"int","required":true}]}},
		{"id":4,"type":"sink.step_progress"}],
	 "edges":[
		{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},
		{"from":{"node":2,"port":"stdout"},"to":{"node":3,"port":"in"}},
		{"from":{"node":3,"port":"` + field + `"},"to":{"node":4,"port":"progress"}},
		{"from":{"node":1,"port":"ref"},"to":{"node":4,"port":"ref"}}]}`)
}

// scriptedStepRig seeds a polled step, binds a script to it and takes its command away, as the API does.
func scriptedStepRig(t *testing.T, graphJSON []byte, runOn bool) (*CheckRunner, journal.Quest, execution.Script) {
	t.Helper()
	st := openRunnerChecksDB(t)
	ctx := bg()
	step := remoteStep("poll", "true", testLabel, journal.RunModePoll)
	if !runOn {
		step.RunOn = nil
	}
	q := seedQuest(t, st, []journal.Step{step})
	sc, err := st.Execution.CreateScript(ctx, "скрипт", "", graphJSON)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Execution.BindScript(ctx, execution.SubjectStep, q.Steps[0].ID, sc.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.Journal.ClearStepCommand(ctx, q.Steps[0].ID); err != nil {
		t.Fatal(err)
	}
	return NewCheckRunner(st, events.New()), q, sc
}

func TestScriptBoundStepRunsItsGraph(t *testing.T) {
	r, q, sc := scriptedStepRig(t, progressScript("echo 3", "progress"), true)
	ctx := bg()
	r.Tick(ctx)
	jobs, _ := r.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel})
	if len(jobs) != 1 || jobs[0].RunID == nil || jobs[0].Command != "echo 3" {
		t.Fatalf("one job from the script: %+v", jobs)
	}
	run, _ := r.Store.Execution.GetRun(ctx, *jobs[0].RunID)
	if run.GraphRef != "script:"+strconv.FormatInt(sc.ID, 10)+":1" || run.ScriptID == nil || *run.ScriptID != sc.ID {
		t.Fatalf("the run names its script and version: %+v", run)
	}
	settle(t, r.Store, r)
	got, _ := r.Store.Journal.GetQuest(ctx, q.ID)
	if got.Steps[0].ProgressCurrent != 3 || got.Steps[0].ScriptID == nil || got.Steps[0].CheckCommand != nil {
		t.Fatalf("the script moved the step: %+v", got.Steps[0])
	}
}

// A job that waits for its device runs the version it was queued with, whatever is saved meanwhile.
func TestScriptVersionIsPinnedWhileTheJobWaits(t *testing.T) {
	r, q, sc := scriptedStepRig(t, progressScript("echo 3", "progress"), true)
	ctx := bg()
	r.Tick(ctx)

	v2 := progressScript("echo 3", "other") // would not read a bare number as progress
	if _, err := r.Store.Execution.UpdateScript(ctx, sc.ID, execution.ScriptUpdate{Graph: v2, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	settle(t, r.Store, r)
	got, _ := r.Store.Journal.GetQuest(ctx, q.ID)
	if got.Steps[0].ProgressCurrent != 3 {
		t.Fatalf("version 1 answers the job queued with it: progress %d", got.Steps[0].ProgressCurrent)
	}

	// The next check uses the new version: its job carries it.
	if _, err := r.Store.DB.Exec(`UPDATE queststep SET check_last_run_at = NULL`); err != nil {
		t.Fatal(err)
	}
	r.Tick(ctx)
	jobs, _ := r.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel, States: []string{execution.JobQueued}})
	if len(jobs) != 1 {
		t.Fatalf("%+v", jobs)
	}
	run, _ := r.Store.Execution.GetRun(ctx, *jobs[0].RunID)
	if run.GraphRef != "script:"+strconv.FormatInt(sc.ID, 10)+":2" {
		t.Fatalf("the next job is on version 2: %s", run.GraphRef)
	}
}

// A script with no Code node needs no runner and runs at once: here a step that closes itself when its progress is enough.
func TestServerOnlyScriptRunsWithoutARunner(t *testing.T) {
	g := []byte(`{"nodes":[
		{"id":1,"type":"source.step"},
		{"id":2,"type":"compare","params":{"type":"int","op":"ge","operand":2}},
		{"id":3,"type":"sink.step_result","params":{"result":"done"}}],
	 "edges":[
		{"from":{"node":1,"port":"progress"},"to":{"node":2,"port":"in"}},
		{"from":{"node":1,"port":"ref"},"to":{"node":3,"port":"ref"}},
		{"from":{"node":2,"port":"yes"},"to":{"node":3,"port":"trigger"}}]}`)
	r, q, _ := scriptedStepRig(t, g, false)
	ctx := bg()
	if _, err := r.Store.DB.Exec(`UPDATE queststep SET progress_current = 1 WHERE id = ?`, q.Steps[0].ID); err != nil {
		t.Fatal(err)
	}
	r.Tick(ctx)
	if got, _ := r.Store.Journal.GetQuest(ctx, q.ID); got.Steps[0].Done {
		t.Fatal("progress 1 is not enough")
	}
	if _, err := r.Store.DB.Exec(`UPDATE queststep SET progress_current = 2, check_last_run_at = NULL WHERE id = ?`, q.Steps[0].ID); err != nil {
		t.Fatal(err)
	}
	r.Tick(ctx)
	got, _ := r.Store.Journal.GetQuest(ctx, q.ID)
	if !got.Steps[0].Done {
		t.Fatalf("progress 2 closes the step with no runner at all: %+v", got.Steps[0])
	}
	if jobs, _ := r.Store.Execution.ListRunnerJobs(ctx, execution.JobFilter{}); len(jobs) != 0 {
		t.Fatalf("no job for a graph without code: %+v", jobs)
	}
}

func TestScriptBoundNoteSetsTheVerdict(t *testing.T) {
	r, ctx := noteCheckRig(t)
	g := []byte(`{"nodes":[
		{"id":1,"type":"source.note"},
		{"id":2,"type":"code","params":{"text":"echo 1"}},
		{"id":3,"type":"parse","params":{"schema":[{"name":"outdated","type":"bool","required":true}]}},
		{"id":4,"type":"sink.note_verdict"}],
	 "edges":[
		{"from":{"node":2,"port":"stdout"},"to":{"node":3,"port":"in"}},
		{"from":{"node":1,"port":"ref"},"to":{"node":4,"port":"ref"}},
		{"from":{"node":3,"port":"outdated"},"to":{"node":4,"port":"outdated"}}]}`)
	sc, err := r.Store.Execution.CreateScript(ctx, "свежесть", "", g)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Store.Execution.BindScript(ctx, execution.SubjectNote, 1, sc.ID); err != nil {
		t.Fatal(err)
	}
	label := testLabel
	// Only the schedule is set: the note has a script and no command.
	check, err := r.Store.Knowledge.SetNoteCheck(ctx, 1, nil, nil, &label)
	if err != nil || check.ScriptID == nil || !check.Active() || check.RunOn == nil {
		t.Fatalf("a scripted note keeps its script when its schedule is set: %+v %v", check, err)
	}
	r.TickNotes(ctx)
	settle(t, r.Store, r)
	if got, _ := r.Store.Knowledge.GetNoteCheck(ctx, 1); !got.Outdated || got.Error != "" {
		t.Fatalf("the script's verdict: %+v", got)
	}

	// A command typed on top of the script replaces it.
	cmd := "echo 0"
	check, err = r.Store.Knowledge.SetNoteCheck(ctx, 1, &cmd, nil, &label)
	if err != nil || check.ScriptID != nil || check.Command == nil {
		t.Fatalf("a command replaces the script: %+v %v", check, err)
	}
	if b, _ := r.Store.Execution.BindingFor(ctx, execution.SubjectNote, 1); b != nil {
		t.Fatalf("the binding must be gone: %+v", b)
	}
}
