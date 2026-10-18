package graph

import (
	"encoding/json"
	"strings"
	"testing"
)

func node(id int, typ, params string) Node {
	n := Node{ID: id, Type: typ}
	if params != "" {
		n.Params = json.RawMessage(params)
	}
	return n
}

func link(fromNode int, fromPort string, toNode int, toPort string) Edge {
	return Edge{From: End{fromNode, fromPort}, To: End{toNode, toPort}}
}

const (
	codeEcho   = `{"text":"echo 1"}`
	stepSchema = `{"schema":[{"name":"progress","type":"int"},{"name":"total","type":"int","min":1},{"name":"description","type":"str"}]}`
	noteSchema = `{"schema":[{"name":"outdated","type":"bool","required":true},{"name":"reason","type":"str"}]}`
)

// The checks that exist today, wired as ADR-0003 section 9 says: they have to be valid graphs.
func legacyGraphs() map[string]Graph {
	stepPoll := Graph{
		Nodes: []Node{node(1, "source.step", ""), node(2, "code", codeEcho), node(3, "parse", stepSchema), node(4, "sink.step_progress", "")},
		Edges: []Edge{link(1, "secrets", 2, "secrets"), link(2, "stdout", 3, "in"), link(3, "progress", 4, "progress"),
			link(3, "total", 4, "total"), link(3, "description", 4, "description"), link(1, "ref", 4, "ref")},
	}
	// A template's emit: the document is the whole answer. A failed run is an attempt that is retried, except a device that
	// never came and a cancel, which close the period.
	return map[string]Graph{"step poll": stepPoll, "step once": StepOnce("echo 1"), "note": NoteCheck("echo 1"), "template": TemplateEmit("echo 1")}
}

func TestTodaysChecksAreValidGraphs(t *testing.T) {
	for name, g := range legacyGraphs() {
		if err := g.Check(Builtin()); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if !g.RunsOnRunner() {
			t.Errorf("%s: has a Code node, needs a runner", name)
		}
	}
	if got := legacyGraphs()["note"].Subjects(); len(got) != 1 || got[0] != "note" {
		t.Errorf("subjects = %v", got)
	}
}

func TestServerOnlyGraphNeedsNoRunner(t *testing.T) {
	// A step that follows a quest, with no code at all (quest=298): the quest's status is compared on the server.
	g := Graph{
		Nodes: []Node{node(1, "source.quest", ""), node(2, "compare", `{"type":"str","op":"eq","operand":"completed"}`),
			node(3, "source.step", ""), node(4, "sink.step_result", `{"result":"done"}`)},
		Edges: []Edge{link(1, "status", 2, "in"), link(3, "ref", 4, "ref"), link(2, "yes", 4, "trigger")},
	}
	if err := g.Check(Builtin()); err != nil {
		t.Fatal(err)
	}
	if g.RunsOnRunner() {
		t.Fatal("no Code node: no runner")
	}
}

func TestValidateRefusals(t *testing.T) {
	base := func() Graph { return legacyGraphs()["step poll"] }
	cases := []struct {
		name   string
		mutate func(g *Graph)
		want   string
	}{
		{"empty", func(g *Graph) { *g = Graph{} }, "граф пуст"},
		{"unknown type", func(g *Graph) { g.Nodes = append(g.Nodes, node(9, "nope", "")) }, "неизвестный тип"},
		{"duplicate id", func(g *Graph) { g.Nodes = append(g.Nodes, node(2, "source.note", "")) }, "повторяется"},
		{"bad id", func(g *Graph) { g.Nodes = append(g.Nodes, node(0, "source.note", "")) }, "положительным"},
		{"missing output", func(g *Graph) { g.Edges = append(g.Edges, link(2, "nothing", 4, "ref")) }, "нет выхода"},
		{"missing input", func(g *Graph) { g.Edges = append(g.Edges, link(1, "title", 4, "nothing")) }, "нет входа"},
		{"missing node", func(g *Graph) { g.Edges = append(g.Edges, link(1, "title", 77, "x")) }, "несуществующей ноде 77"},
		{"type mismatch", func(g *Graph) {
			g.Edges = append(g.Edges, link(1, "title", 3, "in"), link(1, "done", 4, "description"))
		}, "не принимает"},
		{"input fed twice", func(g *Graph) { g.Edges = append(g.Edges, link(1, "progress", 4, "progress")) }, "принимает одну связь"},
		{"required open", func(g *Graph) { g.Edges = g.Edges[:len(g.Edges)-1] }, "обязательный вход \"ref\""},
		{"group empty", func(g *Graph) {
			g.Edges = []Edge{link(1, "secrets", 2, "secrets"), link(2, "stdout", 3, "in"), link(1, "ref", 4, "ref")}
		}, "хотя бы один из входов группы"},
		{"secrets to a sink", func(g *Graph) { g.Edges = append(g.Edges, link(1, "secrets", 4, "description")) }, "не принимает secrets"},
		{"cycle", func(g *Graph) {
			g.Nodes = append(g.Nodes, node(5, "any_of", `{"type":"str"}`), node(6, "any_of", `{"type":"str"}`))
			g.Edges = append(g.Edges, link(1, "title", 5, "in"), link(6, "out", 5, "in"), link(1, "title", 6, "in"), link(5, "out", 6, "in"))
		}, "цикл"},
		{"no sink", func(g *Graph) { *g = Graph{Nodes: g.Nodes[:2], Edges: g.Edges[:1]} }, "нет ни одного приёмника"},
		{"no source", func(g *Graph) {
			*g = Graph{Nodes: []Node{node(2, "code", codeEcho), node(4, "sink.note_error", "")}, Edges: []Edge{link(2, "stdout", 4, "message")}}
		}, "нет ни одного источника"},
		{"alias on a plain link", func(g *Graph) { g.Edges[3].Alias = "X" }, "только у значений, идущих в код"},
		{"empty code", func(g *Graph) { g.Nodes[1] = node(2, "code", `{"text":"  "}`) }, "пустой текст кода"},
		{"code language", func(g *Graph) { g.Nodes[1] = node(2, "code", `{"language":"cobol","text":"x"}`) }, "не поддерживается"},
		{"code too long", func(g *Graph) {
			g.Nodes[1] = node(2, "code", `{"text":"`+strings.Repeat("x", MaxCodeBytes+1)+`"}`)
		}, "длиннее 64 КБ"},
		{"unknown param", func(g *Graph) { g.Nodes[1] = node(2, "code", `{"text":"x","shell":"zsh"}`) }, "unknown field"},
	}
	for _, tc := range cases {
		g := base()
		tc.mutate(&g)
		ps := g.Validate(Builtin())
		var all []string
		for _, p := range ps {
			all = append(all, p.Error())
		}
		if !strings.Contains(strings.Join(all, "\n"), tc.want) {
			t.Errorf("%s: want a problem with %q, got %v", tc.name, tc.want, all)
		}
	}
}

func TestCodeInputsAndEnvNames(t *testing.T) {
	g := Graph{
		Nodes: []Node{node(1, "source.step", ""), node(2, "source.note", ""), node(3, "code", codeEcho), node(4, "sink.step_result", `{"result":"done"}`)},
		Edges: []Edge{
			link(1, "title", 3, "in"), link(2, "title", 3, "in"), link(1, "ref", 4, "ref"), link(3, "stdout", 4, "trigger"),
		},
	}
	if err := g.Check(Builtin()); err != nil {
		t.Fatal(err)
	}
	// The same name from two nodes does not clash: the node number is part of it.
	if a, b := EnvName(g.Edges[0]), EnvName(g.Edges[1]); a != "QUESTS_IN_N1_TITLE" || b != "QUESTS_IN_N2_TITLE" {
		t.Fatalf("env names %s %s", a, b)
	}
	g.Edges[0].Alias = "NAME"
	g.Edges[1].Alias = "NAME"
	if err := g.Check(Builtin()); err == nil || !strings.Contains(err.Error(), "задана дважды") {
		t.Fatalf("a repeated alias must be refused: %v", err)
	}
	g.Edges[1].Alias = "lower"
	if err := g.Check(Builtin()); err == nil || !strings.Contains(err.Error(), "заглавные") {
		t.Fatalf("a lowercase alias must be refused: %v", err)
	}
	// A document and a scalar may travel into the code, a reference or a secret in a value slot may not.
	g.Edges[1].Alias = ""
	g.Edges = append(g.Edges, link(1, "ref", 3, "in"))
	if err := g.Check(Builtin()); err == nil {
		t.Fatal("a reference must not go into the values of Code")
	}
}

func TestCompareChecksItsOperand(t *testing.T) {
	for _, tc := range []struct{ params, want string }{
		{`{"type":"int","op":"gt","operand":5}`, ""},
		{`{"type":"int","op":"gt","operand":"5"}`, "операнд не подходит"},
		{`{"type":"int","op":"gt","operand":1.5}`, "операнд не подходит"},
		{`{"type":"int","op":"contains","operand":1}`, "не подходит типу int"},
		{`{"type":"str","op":"matches","operand":"^a+$"}`, ""},
		{`{"type":"str","op":"matches","operand":"("}`, "операнд не подходит"},
		{`{"type":"bool","op":"eq","operand":true}`, ""},
		{`{"type":"bool","op":"lt","operand":true}`, "не подходит типу bool"},
		{`{"type":"doc","op":"eq","operand":1}`, "тип сравнения"},
		{`{"type":"float","op":"ge"}`, "не задан операнд"},
	} {
		g := Graph{
			Nodes: []Node{node(1, "source.step", ""), node(2, "compare", tc.params), node(3, "sink.step_progress", "")},
			Edges: []Edge{link(1, "ref", 3, "ref")},
		}
		kind := "progress"
		if strings.Contains(tc.params, `"str"`) {
			kind = "title"
		}
		if strings.Contains(tc.params, `"bool"`) {
			kind = "done"
		}
		if strings.Contains(tc.params, `"float"`) {
			kind = "progress"
		}
		g.Edges = append(g.Edges, link(1, kind, 2, "in"))
		err := g.Check(Builtin())
		switch {
		case tc.want == "" && err != nil && strings.Contains(err.Error(), "Сравнить"):
			t.Errorf("%s: %v", tc.params, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: want %q, got %v", tc.params, tc.want, err)
		}
	}
}

func TestParseSchemaIsChecked(t *testing.T) {
	for _, tc := range []struct{ params, want string }{
		{`{"schema":[]}`, "от 1 до"},
		{`{"schema":[{"name":"Bad","type":"int"}]}`, "имя из строчных"},
		{`{"schema":[{"name":"a","type":"int"},{"name":"a","type":"str"}]}`, "повторяется"},
		{`{"schema":[{"name":"bad_output","type":"str"}]}`, "зарезервировано"},
		{`{"schema":[{"name":"a","type":"secrets"}]}`, "тип int, float"},
		{`{"schema":[{"name":"a","type":"str","min":1}]}`, "минимум только у чисел"},
	} {
		_, _, err := parsePorts(json.RawMessage(tc.params))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want %q, got %v", tc.params, tc.want, err)
		}
	}
}

func TestDecode(t *testing.T) {
	good, _ := json.Marshal(legacyGraphs()["note"])
	g, err := Decode(good)
	if err != nil || g.Check(Builtin()) != nil {
		t.Fatalf("round trip: %v", err)
	}
	for _, bad := range []string{`{"nodes":[],"edges":[],"extra":1}`, `{"nodes":[]} {}`, `nope`, strings.Repeat(" ", MaxBytes+1)} {
		if _, err := Decode([]byte(bad)); err == nil {
			t.Errorf("%.30q must not decode", bad)
		}
	}
}

func TestAssignable(t *testing.T) {
	yes := [][2]PortType{{TInt, TInt}, {TInt, TFloat}, {TInt, TDoc}, {TStr, TDoc}, {TBool, TAny}, {TDoc, TAny}, {TDoc, TValue}, {TInt, TValue}}
	no := [][2]PortType{{TFloat, TInt}, {TDoc, TStr}, {TSecret, TAny}, {TSecret, TValue}, {TRefStep, TRefNote}, {TRefStep, TValue}, {TStr, TInt}}
	for _, p := range yes {
		if !Assignable(p[0], p[1]) {
			t.Errorf("%s → %s must fit", p[0], p[1])
		}
	}
	for _, p := range no {
		if Assignable(p[0], p[1]) {
			t.Errorf("%s → %s must not fit", p[0], p[1])
		}
	}
}

func TestTrustedFlag(t *testing.T) {
	for name, nt := range Builtin() {
		if want := name != "code"; nt.Trusted != want {
			t.Errorf("%s: trusted = %v", name, nt.Trusted)
		}
	}
}

func TestSamplesAndImplicitGraphsAreValid(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Samples() {
		if seen[s.ID] || s.Title == "" || s.Description == "" {
			t.Errorf("sample %q: a unique id, a title and a description are needed", s.ID)
		}
		seen[s.ID] = true
		if err := s.Graph.Check(Builtin()); err != nil {
			t.Errorf("sample %q: %v", s.ID, err)
		}
		if got := s.Graph.Subjects(); len(got) != 1 || got[0] != s.Subject {
			t.Errorf("sample %q: reads %v, says %s", s.ID, got, s.Subject)
		}
		if s.Graph.RunsOnRunner() != s.NeedsRunner {
			t.Errorf("sample %q: needs_runner says %v", s.ID, s.NeedsRunner)
		}
	}
	for _, name := range []string{"note_check", "step_once", "step_poll"} {
		g, ok := Implicit(name, "true")
		if !ok || g.Check(Builtin()) != nil {
			t.Errorf("implicit graph %q: ok=%v %v", name, ok, g.Check(Builtin()))
		}
	}
	if _, ok := Implicit("nope", "x"); ok {
		t.Error("an unknown implicit graph must be refused")
	}
}

func TestWithoutLayoutIgnoresPositions(t *testing.T) {
	a := Graph{Nodes: []Node{{ID: 1, Type: "source.step", X: 10, Y: 20}}}
	b := Graph{Nodes: []Node{{ID: 1, Type: "source.step", X: 500, Y: 90}}}
	ja, _ := json.Marshal(a.WithoutLayout())
	jb, _ := json.Marshal(b.WithoutLayout())
	if string(ja) != string(jb) {
		t.Fatalf("graphs that differ only in positions must compare equal: %s %s", ja, jb)
	}
	if a.Nodes[0].X != 10 {
		t.Fatal("the original must keep its positions")
	}
}

func TestSourceTakesAnEntityID(t *testing.T) {
	g := Graph{Nodes: []Node{node(1, "source.step", `{"id":5}`), node(2, "sink.done", "")}, Edges: []Edge{link(1, "ref", 2, "trigger")}}
	if err := g.Check(Builtin()); err != nil {
		t.Fatalf("explicit id: %v", err)
	}
	g.Nodes[0].Params = json.RawMessage(`{"id":-1}`)
	if err := g.Check(Builtin()); err == nil {
		t.Fatal("a negative id passed")
	}
	g.Nodes[0].Params = json.RawMessage(`{"ident":1}`)
	if err := g.Check(Builtin()); err == nil {
		t.Fatal("an unknown parameter passed")
	}
}
