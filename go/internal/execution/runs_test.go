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
	if n, err := st.PurgeRuns(ctx, timeutil.NowUTC().Add(time.Hour)); err != nil || n != 1 {
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
