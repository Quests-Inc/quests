package schedule

import (
	"context"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// A test run from the editor reads a made-up event and lands in the run log as a test; a dry one writes nothing, a live one
// writes like a hook would.
func TestTryRunDryAndLive(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	r := NewCheckRunner(st, events.New())
	note, err := st.Knowledge.CreateNote(ctx, knowledge.Note{Title: "лист"})
	if err != nil {
		t.Fatal(err)
	}
	const g = `{"nodes":[
		{"id":1,"type":"source.event","params":{"family":"value","owner":"note"}},
		{"id":2,"type":"sink.value","params":{"name":"mirror","type":"str"}}],
	 "edges":[
		{"from":{"node":1,"port":"ref"},"to":{"node":2,"port":"ref"}},
		{"from":{"node":1,"port":"kind"},"to":{"node":2,"port":"value"}}]}`
	sc, err := st.Execution.CreateScript(ctx, "зеркало", "", []byte(g))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := graph.Decode([]byte(g))
	if err != nil {
		t.Fatal(err)
	}
	owner := execution.SecretOwner{Kind: execution.SecretOwnerNote, ID: note.ID}
	mirror := func() string {
		set, err := st.Execution.ResolveNamedValues(ctx, owner, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range set.Own {
			if v.Name == "mirror" {
				return v.Value
			}
		}
		return ""
	}
	in := execution.TryStart{ScriptID: sc.ID, Version: sc.Version, Graph: parsed, SubjectType: "note", SubjectID: note.ID, EventKind: "values_changed"}

	id, err := r.TryRun(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.Execution.GetRun(ctx, id)
	if err != nil || run.State != execution.RunDone || !run.Dry || run.SourceKind != execution.RunSourceTest || run.SourceID != sc.ID {
		t.Fatalf("dry run: %+v %v", run, err)
	}
	if run.Snapshot.States[2] != graph.StateRan {
		t.Fatalf("the sink ran (and only reported): %+v", run.Snapshot.States)
	}
	if got := mirror(); got != "" {
		t.Fatalf("a dry run wrote %q", got)
	}

	in.Live = true
	in.Inline = []byte(g)
	id, err = r.TryRun(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if run, _ = st.Execution.GetRun(ctx, id); run.Dry || run.GraphRef != execution.GraphRefInline {
		t.Fatalf("live inline run: %+v", run)
	}
	if got := mirror(); got != "values_changed" {
		t.Fatalf("a live run writes: %q", got)
	}

	list, err := st.Execution.ListRuns(ctx, execution.RunFilter{ScriptID: sc.ID})
	if err != nil || len(list) != 2 || list[0].ID != id || list[0].SourceTitle != "зеркало" || list[0].SubjectTitle != "лист" {
		t.Fatalf("run log: %+v %v", list, err)
	}
	if only, _ := st.Execution.ListRuns(ctx, execution.RunFilter{ScriptID: sc.ID, Before: id}); len(only) != 1 || !only[0].Dry {
		t.Fatalf("the page before the newest: %+v", only)
	}
	stats, err := st.Execution.RunStatsOf(ctx, execution.RunSourceTest)
	if err != nil || stats[sc.ID].LastID != id || stats[sc.ID].LastHour != 2 {
		t.Fatalf("stats: %+v %v", stats, err)
	}
}

func TestTryRunNeedsARunnerForCode(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	r := NewCheckRunner(st, events.New())
	g := graph.NoteCheck("echo 1")
	if _, err := r.TryRun(ctx, execution.TryStart{ScriptID: 1, Graph: g, SubjectType: "note", SubjectID: 1}); err == nil {
		t.Fatal("a graph with code needs a runner label")
	}
}

func TestTestEventCarriesTheSubject(t *testing.T) {
	for kind, key := range map[string]string{"quest_completed": "quest_id", "status_changed": "quest_id", "step_progress": "step_id", "note_updated": "note_id", "values_changed": "owner_id", "attachment_added": "owner_id"} {
		if ev := testEvent(kind, "note", 7); ev[key] != int64(7) || ev["kind"] != kind {
			t.Errorf("%s: %v", kind, ev)
		}
	}
}
