package schedule

import (
	"context"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func TestDispatchHooksQueuesAJob(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	hub := events.New()
	r := NewCheckRunner(st, hub)

	graph := []byte(`{"nodes":[
		{"id":1,"type":"source.quest"},
		{"id":2,"type":"code","params":{"text":"echo ok"}},
		{"id":3,"type":"sink.done"}],
	 "edges":[
		{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},
		{"from":{"node":2,"port":"stdout"},"to":{"node":3,"port":"trigger"}}]}`)
	sc, err := st.Execution.CreateScript(ctx, "хук", "", graph)
	if err != nil {
		t.Fatal(err)
	}
	qid := pgtest.SeedQuest(t, st.DB, "цель")
	_, err = st.Execution.CreateHook(ctx, "на complete", []string{"complete"}, sc.ID, &qid, "home", 30, true)
	if err != nil {
		t.Fatal(err)
	}

	r.DispatchHooks(events.Payload{
		"kind": "quest_completed", "quest_id": float64(qid), "workspace": float64(1),
		"revision": float64(42), "title": "цель",
	})

	jobs, err := st.Execution.ListRunnerJobs(ctx, execution.JobFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.Kind == execution.JobKindHook && j.RunID != nil {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a hook job, got %+v", jobs)
	}
}
