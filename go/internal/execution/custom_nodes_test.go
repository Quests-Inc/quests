package execution

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/quests-inc/quests/go/internal/execution/graph"
)

func customCode() graph.Custom {
	return graph.Custom{
		Title:   "Проверка коммита",
		Inputs:  []graph.CustomPort{{Name: "secrets", Node: 1, Port: "secrets"}},
		Outputs: []graph.CustomPort{{Name: "out", Node: 1, Port: "stdout"}},
		Fields:  []graph.CustomField{{Name: "ref", Type: "str", Default: "aaa", Node: 1, Param: "vars.REF"}},
		Graph: graph.Graph{Nodes: []graph.Node{
			{ID: 1, Type: "code", Params: []byte(`{"text":"echo $REF","vars":{"REF":"aaa"}}`)},
		}},
	}
}

func usingGraph(id int64) []byte {
	return []byte(fmt.Sprintf(`{"nodes":[{"id":1,"type":"source.step"},{"id":2,"type":"custom.%d","params":{"ref":"bbb"}},{"id":3,"type":"sink.done"}],
		"edges":[{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},{"from":{"node":2,"port":"out"},"to":{"node":3,"port":"trigger"}}]}`, id))
}

func TestCustomNodeLifecycle(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()

	n, err := st.CreateCustomNode(ctx, customCode())
	if err != nil || n.ID == 0 || n.Version != 1 || !n.NeedsRunner || n.Problem != "" {
		t.Fatalf("create: %+v %v", n, err)
	}

	// A script may use it; it needs a runner because the node holds code.
	sc, err := st.CreateScript(ctx, "со своей нодой", "", usingGraph(n.ID))
	if err != nil || !sc.NeedsRunner {
		t.Fatalf("script: %+v %v", sc, err)
	}
	// Before the node exists a script cannot name it.
	if _, err := st.CreateScript(ctx, "нет такой", "", usingGraph(n.ID+100)); err == nil {
		t.Fatal("a script used a node that does not exist")
	}

	// In use: not deleted.
	var inUse *CustomNodeInUse
	if err := st.DeleteCustomNode(ctx, n.ID); !errors.As(err, &inUse) {
		t.Fatalf("delete while used: %v", err)
	}

	// A stale version is refused; a good one bumps it.
	upd := customCode()
	upd.Title = "Новое имя"
	if _, err := st.UpdateCustomNode(ctx, n.ID, upd, 5); !errors.Is(err, ErrCustomVersion) {
		t.Fatalf("stale: %v", err)
	}
	n2, err := st.UpdateCustomNode(ctx, n.ID, upd, 1)
	if err != nil || n2.Version != 2 || n2.Title != "Новое имя" {
		t.Fatalf("update: %+v %v", n2, err)
	}

	// A node cannot use itself, nor be saved broken.
	loop := customCode()
	loop.Graph.Nodes = append(loop.Graph.Nodes, graph.Node{ID: 2, Type: fmt.Sprintf("custom.%d", n.ID)})
	if _, err := st.UpdateCustomNode(ctx, n.ID, loop, 2); err == nil {
		t.Fatal("a node was saved holding itself")
	}
	bad := customCode()
	bad.Outputs[0].Port = "nope"
	if _, err := st.CreateCustomNode(ctx, bad); err == nil {
		t.Fatal("a broken definition was saved")
	}

	// A node that holds it is a use as well; once nothing does, it goes.
	outer := graph.Custom{
		Title:   "Обёртка",
		Inputs:  []graph.CustomPort{{Name: "secrets", Node: 1, Port: "secrets"}},
		Outputs: []graph.CustomPort{{Name: "out", Node: 1, Port: "out"}},
		Graph:   graph.Graph{Nodes: []graph.Node{{ID: 1, Type: fmt.Sprintf("custom.%d", n.ID)}}},
	}
	w, err := st.CreateCustomNode(ctx, outer)
	if err != nil {
		t.Fatalf("nested: %v", err)
	}
	if err := st.DeleteCustomNode(ctx, w.ID); err != nil {
		t.Fatalf("delete wrapper: %v", err)
	}
	if err := st.DeleteScript(ctx, sc.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteCustomNode(ctx, n.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetCustomNode(ctx, n.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
