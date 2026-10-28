package schedule

import (
	"context"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// A step that follows a quest takes its progress at every change of the followed quest, through a hook and a script.
func TestStepFollowsAQuestThroughAHook(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	child := seedQuest(t, st, []journal.Step{
		{Title: "a", ProgressTotal: 1, SortOrder: 0},
		{Title: "b", ProgressTotal: 1, SortOrder: 1},
		{Title: "c", ProgressTotal: 1, SortOrder: 2},
	})
	parent := seedQuest(t, st, []journal.Step{{Title: "ждём дочерний", ProgressTotal: 1, SortOrder: 0}})
	stepID := parent.Steps[0].ID

	m, err := st.Execution.SetStepMirror(ctx, stepID, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Execution.StepMirror(ctx, stepID); got == nil || got.QuestID != child.ID || got.ParentQuestID != parent.ID {
		t.Fatalf("the link must be readable back: %+v", got)
	}
	r := NewCheckRunner(st, events.New())
	fire := func(rev float64) {
		r.hookGate = newHookGate() // each reading is its own: the test is not about the pause
		r.DispatchHooks(events.Payload{"kind": "quest_updated", "quest_id": float64(child.ID), "workspace": float64(1), "revision": rev, "title": "x"})
		settle(t, st, r)
	}
	progress := func() (int, int) {
		q, err := st.Journal.GetQuest(ctx, parent.ID)
		if err != nil {
			t.Fatal(err)
		}
		return q.Steps[0].ProgressCurrent, q.Steps[0].ProgressTotal
	}

	fire(1)
	if cur, total := progress(); cur != 0 || total != 3 {
		t.Fatalf("a quest with 3 steps, none done: step is %d/%d", cur, total)
	}
	c, _ := st.Journal.GetQuest(ctx, child.ID)
	c.Steps[0].ProgressCurrent = 1
	c.Steps[1].ProgressCurrent = 1
	if _, err := st.Journal.UpdateStep(ctx, c.Steps[0], c, "step_completed", "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Journal.UpdateStep(ctx, c.Steps[1], c, "step_completed", "b"); err != nil {
		t.Fatal(err)
	}
	fire(2)
	if cur, total := progress(); cur != 2 || total != 3 {
		t.Fatalf("two of three done: step is %d/%d", cur, total)
	}

	// Taking the link away stops it and removes its hook and script.
	if err := st.Execution.ClearStepMirror(ctx, stepID); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.Execution.StepMirror(ctx, stepID); got != nil {
		t.Fatalf("link still there: %+v", got)
	}
	if hooks, _ := st.Execution.ListHooks(ctx); len(hooks) != 0 {
		t.Fatalf("the hook of the link must go with it: %d left", len(hooks))
	}
	_ = m
}

func TestStepMirrorRefusesSelfAndCircles(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	a := seedQuest(t, st, []journal.Step{{Title: "a1", ProgressTotal: 1}})
	b := seedQuest(t, st, []journal.Step{{Title: "b1", ProgressTotal: 1}})
	c := seedQuest(t, st, []journal.Step{{Title: "c1", ProgressTotal: 1}})

	if _, err := st.Execution.SetStepMirror(ctx, a.Steps[0].ID, a.ID); err == nil || !strings.Contains(err.Error(), "своим квестом") {
		t.Fatalf("a step of A following A: %v", err)
	}
	if _, err := st.Execution.SetStepMirror(ctx, a.Steps[0].ID, b.ID); err != nil { // A follows B
		t.Fatal(err)
	}
	if _, err := st.Execution.SetStepMirror(ctx, b.Steps[0].ID, a.ID); err == nil || !strings.Contains(err.Error(), "круг") {
		t.Fatalf("B following A while A follows B: %v", err)
	}
	if _, err := st.Execution.SetStepMirror(ctx, b.Steps[0].ID, c.ID); err != nil { // B follows C
		t.Fatal(err)
	}
	if _, err := st.Execution.SetStepMirror(ctx, c.Steps[0].ID, a.ID); err == nil || !strings.Contains(err.Error(), "круг") {
		t.Fatalf("C following A closes A→B→C→A: %v", err)
	}
	// Setting it again replaces, not stacks.
	if _, err := st.Execution.SetStepMirror(ctx, a.Steps[0].ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if hooks, _ := st.Execution.ListHooks(ctx); len(hooks) != 2 {
		t.Fatalf("two links in all (A→C, B→C), got %d hooks", len(hooks))
	}
}

// The hub gives hooks the event as it built it (the quest is a pointer, not a JSON number): a hook with an area of one quest
// must still be woken by it.
func TestQuestAreaHookIsWokenByARealHubEvent(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	child := seedQuest(t, st, []journal.Step{{Title: "a", ProgressTotal: 1}, {Title: "b", ProgressTotal: 1}})
	parent := seedQuest(t, st, []journal.Step{{Title: "ждём", ProgressTotal: 1}})
	if _, err := st.Execution.SetStepMirror(ctx, parent.Steps[0].ID, child.ID); err != nil {
		t.Fatal(err)
	}
	hub := events.New()
	r := NewCheckRunner(st, hub)
	done := make(chan struct{}, 1)
	hub.OnPublish(func(p events.Payload) { r.DispatchHooks(p); done <- struct{}{} })
	qid := child.ID
	hub.Publish("quest_updated", events.PublishOpts{Workspace: 1, QuestID: &qid, Toast: false, Source: "system"})
	<-done
	settle(t, st, r)
	q, err := st.Journal.GetQuest(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if q.Steps[0].ProgressTotal != 2 {
		t.Fatalf("the step must take the followed quest's 2 steps from a real event, got total %d", q.Steps[0].ProgressTotal)
	}
}
