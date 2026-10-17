package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	var n int
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
	if _, err := st.UpdateScript(ctx, sc.ID, ScriptUpdate{Graph: []byte(`{"nodes":[],"edges":[]}`), ExpectedVersion: 1}); err == nil {
		t.Fatal("an empty graph must be refused on update")
	}
	if got, _ := st.GetScript(ctx, sc.ID); got.Version != 1 {
		t.Fatalf("a refused update changed the version: %+v", got)
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
