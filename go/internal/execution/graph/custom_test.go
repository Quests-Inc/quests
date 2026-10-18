package graph

import (
	"context"
	"strings"
	"testing"
)

// codeNode: a custom node that is one Code node with a fixed variable REF exposed as a field.
func codeNode() Custom {
	return Custom{
		ID: 7, Title: "Проверка коммита",
		Inputs:  []CustomPort{{Name: "secrets", Node: 1, Port: "secrets"}, {Name: "title", Node: 1, Port: "in"}},
		Outputs: []CustomPort{{Name: "out", Node: 1, Port: "stdout"}, {Name: "fail", Node: 1, Port: "failed"}},
		Fields:  []CustomField{{Name: "ref", Label: "Коммит", Type: "str", Default: "aaa", Node: 1, Param: "vars.REF"}},
		Graph:   Graph{Nodes: []Node{node(1, "code", `{"text":"echo $REF $TITLE","vars":{"REF":"aaa"}}`)}},
	}
}

func outer(params string) Graph {
	return Graph{
		Nodes: []Node{node(1, "source.step", ""), node(2, "custom.7", params), node(3, "sink.done", "")},
		Edges: []Edge{link(1, "secrets", 2, "secrets"), link(1, "title", 2, "title"), link(2, "out", 3, "trigger")},
	}
}

func TestCustomNodeExpandsAndRuns(t *testing.T) {
	reg, errs := Builtin().WithCustoms([]Custom{codeNode()})
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	g := outer(`{"ref":"bbb"}`)
	if err := g.Check(reg); err != nil {
		t.Fatalf("outer graph: %v", err)
	}
	if reg["custom.7"].Trusted {
		t.Fatal("a node that holds code is not trusted")
	}
	ex, err := Expand(g, []Custom{codeNode()}, reg)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRun(ex, reg)
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHost{}
	reqs, err := r.Advance(context.Background(), h)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("%v %+v", err, reqs)
	}
	if got := reqs[0].Env["REF"]; got != "bbb" {
		t.Fatalf("REF = %q", got)
	}
	if got := reqs[0].Env["QUESTS_IN_TITLE"]; got != "Шаг" {
		t.Fatalf("title input: %v", reqs[0].Env)
	}
	if _, err := r.Resume(context.Background(), h, reqs[0].Node, *stdout("ok")); err != nil || !r.Finished() || len(h.sinks) != 1 {
		t.Fatalf("finish: %v finished=%v sinks=%d", err, r.Finished(), len(h.sinks))
	}
	// A field left out takes the baked default.
	ex, _ = Expand(outer(``), []Custom{codeNode()}, reg)
	r, _ = NewRun(ex, reg)
	reqs, _ = r.Advance(context.Background(), &fakeHost{})
	if reqs[0].Env["REF"] != "aaa" {
		t.Fatalf("default: %v", reqs[0].Env)
	}
}

func TestRunRefusesAnUnexpandedCustomNode(t *testing.T) {
	reg, _ := Builtin().WithCustoms([]Custom{codeNode()})
	if _, err := NewRun(outer(``), reg); err == nil || !strings.Contains(err.Error(), "Expand") {
		t.Fatalf("got %v", err)
	}
}

func TestCustomDefinitionIsChecked(t *testing.T) {
	cases := []struct {
		name string
		edit func(c *Custom)
		want string
	}{
		{"unknown inner port", func(c *Custom) { c.Inputs[0].Port = "nope" }, "нет входа"},
		{"output port", func(c *Custom) { c.Outputs[0].Port = "nope" }, "нет выхода"},
		{"bad field type", func(c *Custom) { c.Fields[0].Type = "float" }, "тип str"},
		{"bad field param", func(c *Custom) { c.Fields[0].Param = "a.b" }, "параметр"},
		{"field of a missing node", func(c *Custom) { c.Fields[0].Node = 9 }, "нет ноды 9"},
		{"default of the wrong type", func(c *Custom) { c.Fields[0].Default = 5 }, "типа str"},
		{"bad name", func(c *Custom) { c.Inputs[1].Name = "Title" }, "имя"},
		{"duplicate name", func(c *Custom) { c.Inputs[1].Name = "secrets" }, "имя"},
		{"inner node number too big", func(c *Custom) { c.Graph.Nodes[0].ID = 10000 }, "меньше 10000"},
		{"open required input", func(c *Custom) {
			c.Graph.Nodes = append(c.Graph.Nodes, node(2, "sink.done", ""))
		}, "обязательный вход"},
		{"itself", func(c *Custom) { c.Graph.Nodes = append(c.Graph.Nodes, node(2, "custom.7", "")) }, "саму себя"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := codeNode()
			tc.edit(&c)
			err := c.Check(Builtin())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	if err := codeNode().Check(Builtin()); err != nil {
		t.Fatalf("a good definition: %v", err)
	}
}

func TestCustomNodesNest(t *testing.T) {
	// Node 8 wraps node 7 and exposes its field under another name.
	wrap := Custom{
		ID: 8, Title: "Обёртка",
		Inputs:  []CustomPort{{Name: "secrets", Node: 1, Port: "secrets"}, {Name: "title", Node: 1, Port: "title"}},
		Outputs: []CustomPort{{Name: "out", Node: 1, Port: "out"}},
		Fields:  []CustomField{{Name: "commit", Type: "str", Default: "ccc", Node: 1, Param: "ref"}},
		Graph:   Graph{Nodes: []Node{node(1, "custom.7", "")}},
	}
	all := []Custom{codeNode(), wrap}
	reg, errs := Builtin().WithCustoms(all)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	g := outer(`{"commit":"ddd"}`)
	g.Nodes[1].Type = "custom.8"
	if err := g.Check(reg); err != nil {
		t.Fatal(err)
	}
	ex, err := Expand(g, all, reg)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRun(ex, reg)
	if err != nil {
		t.Fatal(err)
	}
	reqs, _ := r.Advance(context.Background(), &fakeHost{})
	if len(reqs) != 1 {
		t.Fatalf("requests: %+v", reqs)
	}
	if reqs[0].Env["REF"] != "ddd" {
		t.Fatalf("the wrapper's field reaches the inner node's field: %v", reqs[0].Env)
	}
	if reqs[0].Node != (2*innerBase+1)*innerBase+1 {
		t.Fatalf("node number %d", reqs[0].Node)
	}
}

func TestCustomMissingInnerTypeIsReported(t *testing.T) {
	c := codeNode()
	c.Graph.Nodes = append(c.Graph.Nodes, node(2, "custom.99", ""))
	_, errs := Builtin().WithCustoms([]Custom{c})
	if errs[7] == nil {
		t.Fatalf("errors: %v", errs)
	}
}

func TestRemapIDsFollowsTheImport(t *testing.T) {
	g := Graph{Nodes: []Node{node(1, "source.step", `{"id":5}`), node(2, "custom.7", ``), node(3, "source.note", `{"id":9}`), node(4, "source.quest", ``)}}
	customs := func(id int64) (int64, bool) { return id + 100, id == 7 }
	ents := func(kind string, id int64) (int64, bool) { return id + 1000, kind == "step" }
	out, dangling := g.RemapIDs(customs, ents)
	if string(out.Nodes[0].Params) != `{"id":1005}` || out.Nodes[1].Type != "custom.107" {
		t.Fatalf("remapped: %+v", out.Nodes)
	}
	if out.Nodes[2].Params != nil || dangling != 1 {
		t.Fatalf("a source whose entity is not in the archive loses its id: %s %d", out.Nodes[2].Params, dangling)
	}
	if string(g.Nodes[0].Params) != `{"id":5}` {
		t.Fatal("the original graph was changed")
	}
	c := codeNode()
	c.Graph.Nodes = []Node{node(1, "source.step", `{"id":5}`)}
	c.Fields = []CustomField{{Name: "n", Type: "int", Default: float64(5), Node: 1, Param: "id"}}
	rc, _ := c.RemapIDs(customs, ents)
	if rc.Fields[0].Default != int64(1005) {
		t.Fatalf("field default: %v", rc.Fields[0].Default)
	}
}

func TestEverySampleIsAStarterNode(t *testing.T) {
	for _, s := range Samples() {
		c := Starter(s)
		c.ID = 1
		if err := c.Check(Builtin()); err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		reg, errs := Builtin().WithCustoms([]Custom{c})
		if len(errs) != 0 {
			t.Fatalf("%s: %v", s.ID, errs)
		}
		// A script that is the node alone is a whole script.
		one := Graph{Nodes: []Node{{ID: 1, Type: "custom.1"}}}
		if err := one.Check(reg); err != nil {
			t.Fatalf("%s as a script: %v", s.ID, err)
		}
		ex, err := Expand(one, []Custom{c}, reg)
		if err != nil {
			t.Fatalf("%s: %v", s.ID, err)
		}
		if _, err := NewRun(ex, reg); err != nil {
			t.Fatalf("%s expands to a graph that cannot run: %v", s.ID, err)
		}
	}
}
