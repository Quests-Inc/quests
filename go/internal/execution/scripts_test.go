package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// A step poll: the source, a command, a strict parse, the sink.
func scriptGraph(text string) []byte {
	return []byte(`{"nodes":[
		{"id":1,"type":"source.step"},
		{"id":2,"type":"code","params":{"text":"` + text + `"}},
		{"id":3,"type":"parse","params":{"schema":[{"name":"progress","type":"int","required":true}]}},
		{"id":4,"type":"sink.step_progress"}],
	 "edges":[
		{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},
		{"from":{"node":2,"port":"stdout"},"to":{"node":3,"port":"in"}},
		{"from":{"node":3,"port":"progress"},"to":{"node":4,"port":"progress"}},
		{"from":{"node":1,"port":"ref"},"to":{"node":4,"port":"ref"}}]}`)
}

func TestScriptLifecycleAndVersions(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()

	sc, err := st.CreateScript(ctx, "  Счётчик  ", "считает", scriptGraph("echo 1"))
	if err != nil {
		t.Fatal(err)
	}
	if sc.Title != "Счётчик" || sc.Version != 1 || !sc.NeedsRunner || len(sc.Subjects) != 1 || sc.Subjects[0] != "step" || len(sc.Graph) == 0 {
		t.Fatalf("created: %+v", sc)
	}
	list, err := st.ListScripts(ctx)
	if err != nil || len(list) != 1 || len(list[0].Graph) != 0 {
		t.Fatalf("a list carries no graphs: %+v %v", list, err)
	}

	// Text only: the same version, the same graph.
	newTitle := "Счётчик 2"
	sc2, err := st.UpdateScript(ctx, sc.ID, ScriptUpdate{Title: &newTitle, ExpectedVersion: 1})
	if err != nil || sc2.Version != 1 || sc2.Title != newTitle {
		t.Fatalf("text update: %+v %v", sc2, err)
	}
	// The same graph again is not a new version either.
	if sc2, err = st.UpdateScript(ctx, sc.ID, ScriptUpdate{Graph: scriptGraph("echo 1"), ExpectedVersion: 1}); err != nil || sc2.Version != 1 {
		t.Fatalf("an unchanged graph must keep the version: %+v %v", sc2, err)
	}
	// Moving nodes about is not a new version: the current one is rewritten with the new positions.
	moved := bytes.Replace(scriptGraph("echo 1"), []byte(`{"id":1,"type":"source.step"}`), []byte(`{"id":1,"type":"source.step","x":300,"y":40}`), 1)
	sc2, err = st.UpdateScript(ctx, sc.ID, ScriptUpdate{Graph: moved, ExpectedVersion: 1})
	if err != nil || sc2.Version != 1 || !strings.Contains(string(sc2.Graph), `"x":300`) {
		t.Fatalf("a move must keep the version and store the position: %+v %v", sc2, err)
	}
	// A different graph is version 2, and version 1 stays readable.
	sc3, err := st.UpdateScript(ctx, sc.ID, ScriptUpdate{Graph: scriptGraph("echo 2"), ExpectedVersion: 1})
	if err != nil || sc3.Version != 2 || !strings.Contains(string(sc3.Graph), "echo 2") {
		t.Fatalf("graph update: %+v %v", sc3, err)
	}
	old, err := st.ScriptGraph(ctx, sc.ID, 1)
	if err != nil || !strings.Contains(string(old), "echo 1") {
		t.Fatalf("version 1: %s %v", old, err)
	}
	if _, err := st.ScriptGraph(ctx, sc.ID, 9); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing version: %v", err)
	}

	// An editor that read version 1 must not overwrite version 2.
	if _, err := st.UpdateScript(ctx, sc.ID, ScriptUpdate{Graph: scriptGraph("echo 3"), ExpectedVersion: 1}); !errors.Is(err, ErrScriptVersion) {
		t.Fatalf("a stale write: %v", err)
	}

	if err := st.DeleteScript(ctx, sc.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteScript(ctx, sc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting twice: %v", err)
	}
	// The trash keeps the versions; erasing the script for good takes them.
	var n int
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM script_version WHERE script_id = ?`, sc.ID).Scan(&n)
	if n != 2 {
		t.Fatalf("%d versions in the trash, want 2", n)
	}
	if err := st.PurgeTrashed(ctx, "script", sc.ID); err != nil {
		t.Fatal(err)
	}
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM script_version WHERE script_id = ?`, sc.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("%d versions survived the script", n)
	}
}

func TestScriptRefusals(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()

	var ge *GraphError
	if _, err := st.CreateScript(ctx, "x", "", []byte(`{"nodes":[{"id":1,"type":"nope"}],"edges":[]}`)); !errors.As(err, &ge) || len(ge.Problems) == 0 {
		t.Fatalf("an invalid graph must be a GraphError: %v", err)
	}
	for name, tc := range map[string]struct {
		title, desc string
		graph       []byte
	}{
		"no title":    {"  ", "", scriptGraph("echo 1")},
		"long title":  {strings.Repeat("я", MaxScriptTitle+1), "", scriptGraph("echo 1")},
		"long desc":   {"x", strings.Repeat("я", MaxScriptDescription+1), scriptGraph("echo 1")},
		"not json":    {"x", "", []byte(`nope`)},
		"extra field": {"x", "", []byte(`{"nodes":[],"edges":[],"x":1}`)},
	} {
		if _, err := st.CreateScript(ctx, tc.title, tc.desc, tc.graph); err == nil {
			t.Errorf("%s: must be refused", name)
		}
	}
	if list, _ := st.ListScripts(ctx); len(list) != 0 {
		t.Fatalf("nothing refused may be stored: %+v", list)
	}
	if _, err := st.GetScript(ctx, 404); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing script: %v", err)
	}
	if _, err := st.UpdateScript(ctx, 404, ScriptUpdate{ExpectedVersion: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("updating a missing script: %v", err)
	}
	// A bad graph on update changes nothing.
	sc, err := st.CreateScript(ctx, "ok", "", scriptGraph("echo 1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateScript(ctx, sc.ID, ScriptUpdate{Graph: []byte(`{"nodes":[{"id":1,"type":"nope"}],"edges":[]}`), ExpectedVersion: 1}); err == nil {
		t.Fatal("a bad graph must be refused on update")
	}
	if got, _ := st.GetScript(ctx, sc.ID); got.Version != 1 {
		t.Fatalf("a refused update changed the version: %+v", got)
	}

	// An empty script is allowed (a new one starts empty); it has no subject, so it cannot be bound.
	empty, err := st.CreateScript(ctx, "Новый скрипт", "", []byte(`{"nodes":[],"edges":[]}`))
	if err != nil || empty.NeedsRunner || len(empty.Subjects) != 0 {
		t.Fatalf("empty script: %+v %v", empty, err)
	}
	if err := st.BindScript(ctx, SubjectStep, 1, empty.ID); err == nil {
		t.Fatal("an empty script was bound")
	}
	if _, err := graph.NewRun(graph.Graph{}, graph.Builtin()); err == nil {
		t.Fatal("an empty graph ran")
	}
}

func TestScriptsAreIsolatedByWorkspace(t *testing.T) {
	st := openSecretsDB(t)
	if _, err := st.DB.Exec(`INSERT INTO workspace (id, name) VALUES (2, 'two')`); err != nil {
		t.Fatal(err)
	}
	ws1, ws2 := wsctx.With(context.Background(), 1), wsctx.With(context.Background(), 2)
	sc, err := st.CreateScript(ws1, "mine", "", scriptGraph("echo 1"))
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := st.ListScripts(ws2); len(list) != 0 {
		t.Fatalf("another workspace sees %+v", list)
	}
	if _, err := st.GetScript(ws2, sc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another workspace reads it: %v", err)
	}
	if err := st.DeleteScript(ws2, sc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another workspace deletes it: %v", err)
	}
}

func TestMarkTemplatesIgnoresParametersAndPositions(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	sm := graph.Samples()[0]
	raw, _ := json.Marshal(sm.Graph)
	same, err := st.CreateScript(ctx, "как образец", "", raw)
	if err != nil {
		t.Fatal(err)
	}
	// Moved nodes and another text of the code are still the sample's skeleton.
	tuned := sm.Graph
	tuned.Nodes = append([]graph.Node(nil), tuned.Nodes...)
	for i := range tuned.Nodes {
		tuned.Nodes[i].X, tuned.Nodes[i].Y = 500, 40
		if tuned.Nodes[i].Type == "code" {
			tuned.Nodes[i].Params = json.RawMessage(`{"text":"echo 1"}`)
		}
	}
	raw, _ = json.Marshal(tuned)
	moved, err := st.CreateScript(ctx, "подправлен", "", raw)
	if err != nil {
		t.Fatal(err)
	}
	// A different skeleton is nobody's template.
	own, err := st.CreateScript(ctx, "свой", "", scriptGraph("echo 1"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := st.ListScripts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MarkTemplates(ctx, list); err != nil {
		t.Fatal(err)
	}
	got := map[int64]string{}
	for _, s := range list {
		got[s.ID] = s.Template
	}
	if got[same.ID] != sm.ID || got[moved.ID] != sm.ID || got[own.ID] != "" {
		t.Fatalf("templates: %v (sample %s)", got, sm.ID)
	}
}

func TestScriptPurpose(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	sc, err := st.CreateScriptFor(ctx, "для заметок", "", "note", scriptGraph("echo 1"))
	if err != nil || sc.Purpose != "note" {
		t.Fatalf("create: %+v %v", sc, err)
	}
	if _, err := st.CreateScriptFor(ctx, "x", "", "bogus", scriptGraph("echo 1")); err == nil {
		t.Fatal("an unknown purpose was accepted")
	}
	p := "hook"
	sc2, err := st.UpdateScript(ctx, sc.ID, ScriptUpdate{Purpose: &p, ExpectedVersion: 1})
	if err != nil || sc2.Purpose != "hook" || sc2.Version != 1 {
		t.Fatalf("update: %+v %v", sc2, err)
	}
	plain, err := st.CreateScript(ctx, "без назначения", "", scriptGraph("echo 1"))
	if err != nil || plain.Purpose != "" {
		t.Fatalf("default purpose: %+v %v", plain, err)
	}
}
