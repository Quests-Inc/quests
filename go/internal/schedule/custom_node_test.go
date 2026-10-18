package schedule

import (
	"fmt"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
)

// A custom node (quest=458) is expanded into its inner nodes when the run starts and again when its job comes back: a
// step whose script holds one moves just like one whose script holds the plain nodes.
func TestScriptWithACustomNodeRuns(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := bg()
	node, err := st.Execution.CreateCustomNode(ctx, graph.Custom{
		Title:   "Число командой",
		Inputs:  []graph.CustomPort{{Name: "secrets", Node: 1, Port: "secrets"}},
		Outputs: []graph.CustomPort{{Name: "progress", Node: 2, Port: "progress"}},
		Fields:  []graph.CustomField{{Name: "cmd", Label: "Команда", Type: "str", Default: "echo 3", Node: 1, Param: "text"}},
		Graph: graph.Graph{
			Nodes: []graph.Node{
				{ID: 1, Type: "code", Params: []byte(`{"text":"echo 3"}`)},
				{ID: 2, Type: "parse", Params: []byte(`{"schema":[{"name":"progress","type":"int","required":true}]}`)},
			},
			Edges: []graph.Edge{{From: graph.End{Node: 1, Port: "stdout"}, To: graph.End{Node: 2, Port: "in"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	outer := []byte(fmt.Sprintf(`{"nodes":[
		{"id":1,"type":"source.step"},
		{"id":2,"type":"custom.%d","params":{"cmd":"echo 4"}},
		{"id":3,"type":"sink.step_progress"}],
	 "edges":[
		{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},
		{"from":{"node":2,"port":"progress"},"to":{"node":3,"port":"progress"}},
		{"from":{"node":1,"port":"ref"},"to":{"node":3,"port":"ref"}}]}`, node.ID))
	step := remoteStep("poll", "true", testLabel, journal.RunModePoll)
	q := seedQuest(t, st, []journal.Step{step})
	sc, err := st.Execution.CreateScript(ctx, "со своей нодой", "", outer)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Execution.BindScript(ctx, execution.SubjectStep, q.Steps[0].ID, sc.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.Journal.ClearStepCommand(ctx, q.Steps[0].ID); err != nil {
		t.Fatal(err)
	}
	r := NewCheckRunner(st, events.New())
	r.Tick(ctx)
	jobs, _ := st.Execution.ListRunnerJobs(ctx, execution.JobFilter{Label: testLabel})
	if len(jobs) != 1 || jobs[0].Command != "echo 4" {
		t.Fatalf("the field reached the code: %+v", jobs)
	}
	settle(t, st, r)
	got, _ := st.Journal.GetQuest(ctx, q.ID)
	if got.Steps[0].ProgressCurrent != 4 {
		t.Fatalf("the step took the number from the custom node: %+v", got.Steps[0])
	}
}
