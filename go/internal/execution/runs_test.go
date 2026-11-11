package execution

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

func TestRunStoreAndItsJobs(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	snap := graph.Snapshot{States: map[int]graph.NodeState{2: graph.StateWaiting}}

	id, err := st.CreateRun(ctx, NewScriptRun{GraphRef: "implicit:step_poll", SubjectType: "step", SubjectID: 1000, Snapshot: snap})
	if err != nil || id == 0 {
		t.Fatalf("create: %d %v", id, err)
	}
	run, err := st.GetRun(ctx, id)
	if err != nil || run.State != RunWaiting || run.GraphRef != "implicit:step_poll" || run.Snapshot.States[2] != graph.StateWaiting || run.FinishedAt != nil {
		t.Fatalf("get: %+v %v", run, err)
	}

	// A job for the run keeps the values that came over links, and they come back with it.
	node := 2
	job, created, err := st.EnqueueJob(ctx, JobSpec{RunnerLabel: "pc", Kind: JobKindPoll, Title: "t", Command: "echo 1", RunID: &id, RunNode: &node,
		Env: map[string]string{"QUESTS_IN_N1_TITLE": "Шаг"}})
	if err != nil || !created {
		t.Fatalf("enqueue: %v %v", created, err)
	}
	got, err := st.GetRunnerJob(ctx, job.ID)
	if err != nil || got.RunID == nil || *got.RunID != id || got.RunNode == nil || *got.RunNode != 2 || got.Env["QUESTS_IN_N1_TITLE"] != "Шаг" {
		t.Fatalf("job: %+v %v", got, err)
	}
	// A job of the older kinds has none of it.
	plain, _, _ := st.EnqueueJob(ctx, JobSpec{RunnerLabel: "pc", Kind: JobKindPoll, Title: "p", Command: "echo 1"})
	if g, _ := st.GetRunnerJob(ctx, plain.ID); g.RunID != nil || g.Env != nil {
		t.Fatalf("a plain job: %+v", g)
	}

	if err := st.SaveRun(ctx, id, RunDone, snap, ""); err != nil {
		t.Fatal(err)
	}
	if run, _ = st.GetRun(ctx, id); run.State != RunDone || run.FinishedAt == nil {
		t.Fatalf("saved: %+v", run)
	}
	if err := st.SaveRun(ctx, 999, RunDone, snap, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("saving a missing run: %v", err)
	}
	if _, err := st.GetRun(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing run: %v", err)
	}

	// Purging: a waiting run stays however old, a finished one goes, and takes its job along.
	waiting, _ := st.CreateRun(ctx, NewScriptRun{GraphRef: "implicit:x", SubjectType: "step", SubjectID: 1000, Snapshot: snap})
	if n, err := st.PurgeRuns(ctx, timeutil.NowUTC().Add(time.Hour), 0); err != nil || n != 1 {
		t.Fatalf("purge: %d %v", n, err)
	}
	if _, err := st.GetRun(ctx, waiting); err != nil {
		t.Fatalf("a waiting run must survive: %v", err)
	}
	if _, err := st.GetRunnerJob(ctx, job.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the job of a purged run must go with it: %v", err)
	}
	if err := st.DeleteRun(ctx, waiting); err != nil {
		t.Fatal(err)
	}
}

func TestOrphanRunReconciliation(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	snap := graph.Snapshot{States: map[int]graph.NodeState{2: graph.StateWaiting}}
	create := func() int64 {
		t.Helper()
		id, err := st.CreateRun(ctx, NewScriptRun{GraphRef: "implicit:x", SubjectType: "step", SubjectID: 1000, Snapshot: snap})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	orphan, recent, live, terminal := create(), create(), create(), create()
	old := timeutil.NowUTC().Add(-time.Hour)
	for _, id := range []int64{orphan, live, terminal} {
		if _, err := st.DB.ExecContext(ctx, `UPDATE script_run SET updated_at = ? WHERE id = ?`, timeutil.ToDBUTC(old), id); err != nil {
			t.Fatal(err)
		}
	}
	node := 2
	for _, id := range []int64{live, terminal} {
		j, _, err := st.EnqueueJob(ctx, JobSpec{RunnerLabel: "pc", Kind: JobKindPoll, Command: "true", RunID: &id, RunNode: &node})
		if err != nil {
			t.Fatal(err)
		}
		if id == terminal {
			if _, err := st.DB.ExecContext(ctx, `UPDATE runner_job SET state = 'success', finished_at = ? WHERE id = ?`, timeutil.ToDBUTC(old), j.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if n, err := st.PurgeRunnerJobs(ctx, timeutil.NowUTC()); err != nil || n != 0 {
		t.Fatalf("must retain jobs of waiting runs: %d %v", n, err)
	}
	if n, err := st.FailOrphanRuns(ctx, timeutil.NowUTC().Add(-10*time.Minute)); err != nil || n != 1 {
		t.Fatalf("reconcile: %d %v", n, err)
	}
	for _, id := range []int64{recent, live, terminal} {
		r, _ := st.GetRun(ctx, id)
		if r.State != RunWaiting {
			t.Fatalf("changed valid run %d", id)
		}
	}
	r, err := st.GetRun(ctx, orphan)
	if err != nil || r.State != RunFailed || r.FinishedAt == nil || r.Error == "" || r.Snapshot.States[2] != graph.StateWaiting {
		t.Fatalf("orphan: %+v %v", r, err)
	}
	if n, err := st.FailOrphanRuns(ctx, timeutil.NowUTC()); err != nil || n != 1 {
		t.Fatalf("only recent jobless run remains: %d %v", n, err)
	}
}
