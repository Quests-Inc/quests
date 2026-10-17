package schedule

import (
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
)

// Jobs queued before script runs existed carry no run. They may still be waiting for a device when the server is upgraded,
// so the older kinds go on serving them until the next release removes them. These tests keep that promise honest.

func legacyJob(t *testing.T, r *CheckRunner, sp execution.JobSpec) {
	t.Helper()
	sp.RunnerLabel, sp.TimeoutSeconds, sp.ValidFor = testLabel, 60, time.Hour
	if _, created, err := r.Store.Execution.EnqueueJob(bg(), sp); err != nil || !created {
		t.Fatalf("enqueue: created=%v %v", created, err)
	}
}

func TestLegacyNoteJobStillSetsTheVerdict(t *testing.T) {
	r, ctx := noteCheckRig(t)
	cmd, label := "echo 1", testLabel
	if _, err := r.Store.Knowledge.SetNoteCheck(ctx, 1, &cmd, nil, &label); err != nil {
		t.Fatal(err)
	}
	id := int64(1)
	legacyJob(t, r, execution.JobSpec{Kind: execution.JobKindPoll, NoteID: &id, Title: "n", DedupeKey: "note:1:poll", Command: "echo 1"})
	runQueuedJobs(t, r.Store, r)
	if got, _ := r.Store.Knowledge.GetNoteCheck(ctx, 1); !got.Outdated || got.Error != "" {
		t.Fatalf("an old note job must still deliver its verdict: %+v", got)
	}

	// A broken run is recorded as an error, and so are a device that never came and one that vanished.
	for _, state := range []string{execution.JobFail, execution.JobTimeout, execution.JobSkipped, execution.JobAbandoned} {
		code := 3
		r.JobFinished(ctx, execution.RunnerJob{NoteID: &id, State: state, ExitCode: &code, RunnerLabel: "dev", Stderr: "boom"})
		got, _ := r.Store.Knowledge.GetNoteCheck(ctx, 1)
		if got.Error == "" {
			t.Errorf("%s: the error must be recorded", state)
		}
		_ = r.Store.Knowledge.RecordNoteCheckError(ctx, 1, "", time.Now())
	}
	// A cancel says nothing.
	r.JobFinished(ctx, execution.RunnerJob{NoteID: &id, State: execution.JobCanceled})
}

func TestLegacyPollJobStillReadsTheLastInteger(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := bg()
	q := seedQuest(t, st, []journal.Step{remoteStep("poll", "true", testLabel, journal.RunModePoll)})
	r := NewCheckRunner(st, events.New())
	qid, sid := q.ID, q.Steps[0].ID
	// The reader that was in place when the job was queued is the one that answers it.
	legacyJob(t, r, execution.JobSpec{Kind: execution.JobKindPoll, QuestID: &qid, StepID: &sid, Title: "t", DedupeKey: "step:poll", Command: `echo "готово: 4"`})
	runQueuedJobs(t, st, r)
	got, _ := st.Journal.GetQuest(ctx, q.ID)
	if got.Steps[0].ProgressCurrent != 4 || got.Steps[0].CheckLastRunAt == nil {
		t.Fatalf("an old poll job: %+v", got.Steps[0])
	}
}

func TestLegacyOnceJobsStillCloseOrFailTheQuest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cmd    string
		status journal.QuestStatus
	}{
		{"a clean exit", "true", journal.StatusCompleted},
		{"a failing command", "echo oops >&2; exit 2", journal.StatusFailed},
	} {
		st := openRunnerChecksDB(t)
		ctx := bg()
		idle := journal.RunIdle
		step := remoteStep("once", "true", testLabel, journal.RunModeOnce)
		step.RunStatus = &idle
		q := seedQuest(t, st, []journal.Step{step})
		r := NewCheckRunner(st, events.New())
		qid, sid := q.ID, q.Steps[0].ID
		legacyJob(t, r, execution.JobSpec{Kind: execution.JobKindOnce, QuestID: &qid, StepID: &sid, Title: "t", DedupeKey: "step:once", Command: tc.cmd})
		runQueuedJobs(t, st, r)
		if got, _ := st.Journal.GetQuest(ctx, q.ID); got.Status != tc.status {
			t.Errorf("%s: status %s, want %s", tc.name, got.Status, tc.status)
		}
	}
}

func TestLegacyOnceJobsThatNeverRanOrVanished(t *testing.T) {
	for _, tc := range []struct {
		state  string
		status journal.QuestStatus
		run    string
	}{
		{execution.JobSkipped, journal.StatusActive, journal.RunIdle},   // no device in a day: wait again
		{execution.JobCanceled, journal.StatusActive, journal.RunFail},  // stopped by hand: not run again by itself
		{execution.JobAbandoned, journal.StatusFailed, journal.RunFail}, // vanished mid-run: result unknown
	} {
		st := openRunnerChecksDB(t)
		ctx := bg()
		running := journal.RunRunning
		step := remoteStep("once", "true", testLabel, journal.RunModeOnce)
		step.RunStatus = &running
		q := seedQuest(t, st, []journal.Step{step})
		r := NewCheckRunner(st, events.New())
		qid, sid := q.ID, q.Steps[0].ID
		r.JobFinished(ctx, execution.RunnerJob{QuestID: &qid, StepID: &sid, Kind: execution.JobKindOnce, State: tc.state, RunnerLabel: testLabel})
		got, _ := st.Journal.GetQuest(ctx, q.ID)
		if got.Status != tc.status || got.Steps[0].RunStatus == nil || *got.Steps[0].RunStatus != tc.run {
			t.Errorf("%s: status %s run %v, want %s %s", tc.state, got.Status, got.Steps[0].RunStatus, tc.status, tc.run)
		}
	}
}
