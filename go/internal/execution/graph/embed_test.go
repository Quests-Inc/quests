package graph

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func e(fn int, fp string, tn int, tp string) Edge { return Edge{From: End{fn, fp}, To: End{tn, tp}} }

func run(t *testing.T, g Graph, args map[string]string) (string, error) {
	t.Helper()
	if err := g.Check(Builtin()); err != nil {
		t.Fatal(err)
	}
	return Evaluate(context.Background(), g, Builtin(), args)
}

func TestDiceScript(t *testing.T) {
	g := DiceScript()
	for i := 0; i < 300; i++ {
		out, err := run(t, g, map[string]string{"_": "5+3к8"})
		if err != nil {
			t.Fatal(err)
		}
		if n, err := strconv.Atoi(out); err != nil || n < 8 || n > 29 {
			t.Fatalf("5+3к8 = %q, want 8..29", out)
		}
	}
	cases := map[string]struct{ lo, hi int }{
		"2*(1к1+4)": {10, 10}, "3d6": {3, 18}, "2к6+1к4+3": {6, 19}, "-1к1": {-1, -1}, "к6": {1, 6}, "2*к4": {2, 8},
		"(1к4)к1": {1, 4}, "1к6к1": {1, 6},
	}
	for expr, want := range cases {
		for i := 0; i < 50; i++ {
			out, err := run(t, g, map[string]string{"_": expr})
			n, aerr := strconv.Atoi(out)
			if err != nil || aerr != nil || n < want.lo || n > want.hi {
				t.Fatalf("%s = %q, %v; want %d..%d", expr, out, err, want.lo, want.hi)
			}
		}
	}
}

func TestOperatorPrecedenceAndAssociativity(t *testing.T) {
	c, err := compileOperators([]Operator{
		{Symbol: "#", Arity: 2, Prec: 4, Def: "a-b"},              // left-associative, tighter than * and +
		{Symbol: "^", Arity: 2, Prec: 4, Right: true, Def: "a-b"}, // right-associative
		{Symbol: "~", Arity: 1, Prec: 6, Def: "a*10"},
		{Symbol: "@", Arity: 2, Prec: 1, Def: "a*100+b"}, // as loose as +
	})
	if err != nil {
		t.Fatal(err)
	}
	for expr, want := range map[string]float64{
		"10#3#2": 5,  // (10-3)-2
		"10^3^2": 9,  // 10-(3-2)
		"1+2#3":  0,  // 1+(2-3)
		"2*5#3":  4,  // 2*(5-3)
		"~2+1":   21, // (~2)+1: a prefix operator binds to the operand next to it
		"-~2":    -20,
		"~2#1":   19, // the operand of a prefix operator takes in only what binds tighter: (~2)#1
	} {
		got, err := evalExprWith(expr, nil, c, &budget{})
		if err != nil {
			t.Fatalf("%s: %v", expr, err)
		}
		if got != want {
			t.Errorf("%s = %v, want %v", expr, got, want)
		}
	}
}

func TestOperatorDefinitionsAreChecked(t *testing.T) {
	bad := map[string]Operator{
		"empty symbol":   {Symbol: "", Arity: 2, Prec: 5, Def: "a"},
		"digit symbol":   {Symbol: "1", Arity: 2, Prec: 5, Def: "a"},
		"builtin symbol": {Symbol: "+", Arity: 2, Prec: 5, Def: "a"},
		"func name":      {Symbol: "min", Arity: 2, Prec: 5, Def: "a"},
		"arity":          {Symbol: "к", Arity: 3, Prec: 5, Def: "a"},
		"prec":           {Symbol: "к", Arity: 2, Prec: 10, Def: "a"},
		"empty def":      {Symbol: "к", Arity: 2, Prec: 5, Def: ""},
		"unknown var":    {Symbol: "к", Arity: 2, Prec: 5, Def: "a+c"},
		"b in prefix":    {Symbol: "к", Arity: 1, Prec: 5, Def: "a+b"},
		"bad function":   {Symbol: "к", Arity: 2, Prec: 5, Def: "nope(a)"},
		"self call":      {Symbol: "к", Arity: 2, Prec: 5, Def: "a к b"}, // its own operators are not in scope in a definition
		"syntax":         {Symbol: "к", Arity: 2, Prec: 5, Def: "a +"},
	}
	for name, o := range bad {
		if _, err := compileOperators([]Operator{o}); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	twice := []Operator{{Symbol: "к", Arity: 2, Prec: 5, Def: "a"}, {Symbol: "к", Arity: 2, Prec: 6, Def: "b"}}
	if _, err := compileOperators(twice); err == nil {
		t.Error("the same symbol and arity twice must be refused")
	}
	if _, err := compileOperators(DiceOperators()); err != nil {
		t.Errorf("the dice operators must compile: %v", err)
	}
}

func TestEvaluateLimits(t *testing.T) {
	g := DiceScript()
	for name, expr := range map[string]string{
		"too many repeats":   "2000к6",
		"too many in total":  "900к6+900к6+900к6+900к6+900к6+900к6",
		"division by zero":   "1/0",
		"syntax":             "5+",
		"unknown":            "5+x",
		"zero-sided":         "3к0",
		"fractional repeats": "2.5к6",
		"nested too deep":    strings.Repeat("(", 40) + "1" + strings.Repeat(")", 40),
		"too long":           strings.Repeat("1+", 300) + "1",
	} {
		if out, err := run(t, g, map[string]string{"_": expr}); err == nil {
			t.Errorf("%s (%s) must fail, got %q", name, expr, out)
		}
	}
	if _, err := run(t, g, nil); err == nil {
		t.Error("a missing argument must be an error")
	}
	withCode := Graph{Nodes: []Node{{ID: 1, Type: "args", Params: mustJSON(map[string]any{"params": []ArgParam{{"a", TStr}}})}, {ID: 2, Type: "code"}}}
	if _, err := Evaluate(context.Background(), withCode, Builtin(), map[string]string{"a": "x"}); err == nil {
		t.Error("a graph with code must be refused")
	}
}

func TestExprNodeWithVarsAndInputs(t *testing.T) {
	g := Graph{
		Nodes: []Node{
			{ID: 1, Type: "args", Params: mustJSON(map[string]any{"params": []ArgParam{{"n", TInt}, {"s", TInt}}})},
			{ID: 2, Type: "expr", Params: mustJSON(map[string]any{"expr": "n * s + 1", "vars": []string{"n", "s"}})},
			{ID: 3, Type: "sink.result"},
		},
		Edges: []Edge{e(1, "n", 2, "n"), e(1, "s", 2, "s"), e(2, "text", 3, "value")},
	}
	if out, err := run(t, g, map[string]string{"n": "3", "s": "8"}); err != nil || out != "25" {
		t.Fatalf("got %q, %v", out, err)
	}
	if _, err := run(t, g, map[string]string{"n": "три", "s": "8"}); err == nil {
		t.Error("a non-number for an int argument must be an error")
	}
}

func TestRegexGroups(t *testing.T) {
	g := Graph{
		Nodes: []Node{
			{ID: 1, Type: "args", Params: mustJSON(map[string]any{"params": []ArgParam{{"s", TStr}}})},
			{ID: 2, Type: "regex", Params: mustJSON(map[string]any{"mode": "groups", "pattern": `^(\w+)-(\d+)$`})},
			{ID: 3, Type: "sink.result"},
		},
		Edges: []Edge{e(1, "s", 2, "in"), e(2, "g2", 3, "value")},
	}
	if out, err := run(t, g, map[string]string{"s": "abc-42"}); err != nil || out != "42" {
		t.Fatalf("got %q, %v", out, err)
	}
	if _, err := run(t, g, map[string]string{"s": "nope"}); err == nil {
		t.Error("no match must leave the result empty (an error here)")
	}
	bad := Node{ID: 2, Type: "regex", Params: mustJSON(map[string]any{"mode": "replace", "pattern": "a"})}
	if _, _, err := Builtin()["regex"].Ports(bad.Params); err == nil {
		t.Error("the replace mode is gone")
	}
}

// random → result, through Expr's text (a number has no implicit way into a str), for every kind and aggregate.
func randomGraph(params map[string]any, inputs ...Edge) Graph {
	g := Graph{
		Nodes: []Node{
			{ID: 1, Type: "args", Params: mustJSON(map[string]any{"params": []ArgParam{{"lo", TInt}, {"hi", TInt}, {"n", TInt}, {"list", TStr}}})},
			{ID: 2, Type: "random", Params: mustJSON(params)},
			{ID: 3, Type: "expr", Params: mustJSON(map[string]any{"expr": "x", "vars": []string{"x"}})},
			{ID: 4, Type: "sink.result"},
		},
		Edges: append([]Edge{e(2, "value", 3, "x"), e(3, "text", 4, "value")}, inputs...),
	}
	return g
}

func TestRandomNode(t *testing.T) {
	ctx := context.Background()
	roll := func(g Graph, args map[string]string) (int, error) {
		out, err := Evaluate(ctx, g, Builtin(), args)
		if err != nil {
			return 0, err
		}
		return strconv.Atoi(out)
	}
	// bounds from the parameters, repeated and folded
	for _, agg := range []string{"sum", "min", "max"} {
		g := randomGraph(map[string]any{"kind": "int", "lo": 1, "hi": 6, "count": 4, "aggregate": agg})
		g.Edges = g.Edges[:2]
		g.Nodes = g.Nodes[1:] // no args node: nothing to feed, the node reads only its parameters
		g.Nodes = append([]Node{{ID: 1, Type: "args", Params: mustJSON(map[string]any{"params": []ArgParam{{"unused", TStr}}})}}, g.Nodes...)
		lo, hi := 1, 6
		if agg == "sum" {
			lo, hi = 4, 24
		}
		for i := 0; i < 60; i++ {
			if n, err := roll(g, map[string]string{"unused": "x"}); err != nil || n < lo || n > hi {
				t.Fatalf("%s: %d, %v", agg, n, err)
			}
		}
	}
	// bounds and count from the inputs override nothing they should not: lo/hi/count wired from Args
	g := randomGraph(map[string]any{"kind": "int", "aggregate": "sum"}, e(1, "lo", 2, "lo"), e(1, "hi", 2, "hi"), e(1, "n", 2, "count"))
	for i := 0; i < 60; i++ {
		if n, err := roll(g, map[string]string{"lo": "10", "hi": "12", "n": "3", "list": "-"}); err != nil || n < 30 || n > 36 {
			t.Fatalf("wired bounds: %d, %v", n, err)
		}
	}
	for name, args := range map[string]map[string]string{
		"reversed range": {"lo": "5", "hi": "1", "n": "1", "list": "-"},
		"count zero":     {"lo": "1", "hi": "6", "n": "0", "list": "-"},
		"count too big":  {"lo": "1", "hi": "6", "n": "5000", "list": "-"},
		"huge bound":     {"lo": "1", "hi": "99999999999", "n": "1", "list": "-"},
	} {
		if _, err := roll(g, args); err == nil {
			t.Errorf("%s must fail", name)
		}
	}
	// float
	gf := randomGraph(map[string]any{"kind": "float", "lo": 1, "hi": 2, "aggregate": "max"})
	gf.Nodes = gf.Nodes[1:]
	gf.Nodes = append([]Node{{ID: 1, Type: "args", Params: mustJSON(map[string]any{"params": []ArgParam{{"u", TStr}}})}}, gf.Nodes...)
	gf.Edges = gf.Edges[:2]
	if out, err := Evaluate(ctx, gf, Builtin(), map[string]string{"u": "x"}); err != nil || out == "" {
		t.Fatalf("float: %q %v", out, err)
	}
	// pick: one element, or a list (a doc cannot feed a str port, so the node is called directly)
	pickNode := func(params map[string]any, list string) (Token, error) {
		rs := &resolved{node: Node{ID: 1, Type: "random", Params: mustJSON(params)}}
		out, err := runRandom(rs, map[string][]Token{"list": {{Type: TDoc, Value: json.RawMessage(list)}}}, &budget{})
		return out["value"], err
	}
	tok, err := pickNode(map[string]any{"kind": "pick", "count": 2, "aggregate": "list"}, `["a","b","c"]`)
	if raw, _ := tok.Value.(json.RawMessage); err != nil || !strings.HasPrefix(string(raw), "[") || strings.Count(string(raw), ",") != 1 {
		t.Fatalf("pick list: %v %v", tok.Value, err)
	}
	for i := 0; i < 30; i++ {
		tok, err = pickNode(map[string]any{"kind": "pick"}, `["a","b","c"]`)
		if raw, _ := tok.Value.(json.RawMessage); err != nil || !strings.Contains(`"a" "b" "c"`, string(raw)) {
			t.Fatalf("pick one: %v %v", tok.Value, err)
		}
	}
	if _, err = pickNode(map[string]any{"kind": "pick"}, `[]`); err == nil {
		t.Error("an empty list must fail")
	}
	if _, err = pickNode(map[string]any{"kind": "pick"}, `nope`); err == nil {
		t.Error("a non-array must fail")
	}
}

func TestEmbedNodeParametersAreChecked(t *testing.T) {
	bad := map[string]map[string]any{
		"args none":       {"args": nil},
		"args bad name":   {"args": map[string]any{"params": []ArgParam{{"Bad", TStr}}}},
		"args duplicate":  {"args": map[string]any{"params": []ArgParam{{"a", TStr}, {"a", TInt}}}},
		"args doc":        {"args": map[string]any{"params": []ArgParam{{"a", TDoc}}}},
		"expr many vars":  {"expr": map[string]any{"vars": []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}}},
		"expr var expr":   {"expr": map[string]any{"vars": []string{"expr"}}},
		"expr bad op":     {"expr": map[string]any{"operators": []Operator{{Symbol: "", Arity: 2, Prec: 1, Def: "a"}}}},
		"expr too long":   {"expr": map[string]any{"expr": strings.Repeat("1", MaxExprLen+1)}},
		"random kind":     {"random": map[string]any{"kind": "nope"}},
		"random count":    {"random": map[string]any{"count": 5000}},
		"random aggr":     {"random": map[string]any{"aggregate": "first"}},
		"random pick sum": {"random": map[string]any{"kind": "pick", "aggregate": "sum"}},
		"random bound":    {"random": map[string]any{"lo": 1e12}},
		"regex mode":      {"regex": map[string]any{"mode": "x", "pattern": "a"}},
		"regex pattern":   {"regex": map[string]any{"mode": "match", "pattern": "("}},
		"regex none":      {"regex": map[string]any{"mode": "match"}},
		"regex groups":    {"regex": map[string]any{"mode": "groups", "pattern": "(a)(a)(a)(a)(a)(a)(a)(a)(a)(a)"}},
	}
	for name, one := range bad {
		for typ, params := range one {
			if _, _, err := Builtin()[typ].Ports(mustJSON(params)); err == nil {
				t.Errorf("%s must be refused", name)
			}
		}
	}
	for name, graph := range map[string]Graph{
		"args types":  {Nodes: []Node{{ID: 1, Type: "args", Params: mustJSON(map[string]any{"params": []ArgParam{{"s", TStr}, {"i", TInt}, {"f", TFloat}, {"b", TBool}}})}}},
		"regex match": {Nodes: []Node{{ID: 1, Type: "regex", Params: mustJSON(map[string]any{"mode": "match", "pattern": "a"})}}},
	} {
		n := graph.Nodes[0]
		if _, _, err := Builtin()[n.Type].Ports(n.Params); err != nil {
			t.Errorf("%s must be valid: %v", name, err)
		}
	}
}

func TestArgsAreReadByType(t *testing.T) {
	g := Graph{
		Nodes: []Node{
			{ID: 1, Type: "args", Params: mustJSON(map[string]any{"params": []ArgParam{{"i", TInt}, {"f", TFloat}, {"b", TBool}, {"s", TStr}}})},
			{ID: 2, Type: "expr", Params: mustJSON(map[string]any{"expr": "i + f", "vars": []string{"i", "f"}})},
			{ID: 3, Type: "sink.result"},
		},
		Edges: []Edge{e(1, "i", 2, "i"), e(1, "f", 2, "f"), e(2, "text", 3, "value")},
	}
	if out, err := run(t, g, map[string]string{"i": " 2 ", "f": "0.5", "b": "true", "s": "x"}); err != nil || out != "2.5" {
		t.Fatalf("got %q, %v", out, err)
	}
	for name, args := range map[string]map[string]string{
		"int":   {"i": "2.5", "f": "1", "b": "true"},
		"float": {"i": "2", "f": "x", "b": "true"},
		"bool":  {"i": "2", "f": "1", "b": "maybe"},
	} {
		if _, err := Evaluate(context.Background(), g, Builtin(), args); err == nil {
			t.Errorf("a bad %s argument must fail", name)
		}
	}
}

func TestRegexMatchBranches(t *testing.T) {
	g := Graph{
		Nodes: []Node{
			{ID: 1, Type: "args", Params: mustJSON(map[string]any{"params": []ArgParam{{"s", TStr}}})},
			{ID: 2, Type: "regex", Params: mustJSON(map[string]any{"mode": "match", "pattern": `^\d+$`})},
			{ID: 3, Type: "sink.result"},
		},
		Edges: []Edge{e(1, "s", 2, "in"), e(2, "yes", 3, "value")},
	}
	if out, err := run(t, g, map[string]string{"s": "123"}); err != nil || out != "123" {
		t.Fatalf("yes: %q %v", out, err)
	}
	if _, err := run(t, g, map[string]string{"s": "12a"}); err == nil {
		t.Error("the no branch leaves the result empty")
	}
}

func TestExprFunctionsAndRepeaters(t *testing.T) {
	for expr, want := range map[string]float64{
		"sum(3, 2)": 6, "sum(0, 5)": 0, "rmax(5, 7)": 7, "rmin(5, 7)": 7, "ceil(1.2)": 2, "floor(-1.2)": -2, "abs(-3)": 3, "round(1.5)": 2,
		"min(3, 1, 2)": 1, "max(3, 1, 2)": 3, "7 % 4": 3, "+5": 5, "- -5": 5, "randf(2, 2)": 2, "rand(3, 3)": 3,
	} {
		if got, err := EvalExpr(expr, nil); err != nil || got != want {
			t.Errorf("%s = %v, %v; want %v", expr, got, err, want)
		}
	}
	for _, expr := range []string{
		"rmax(0, 1)", "sum(-1, 1)", "sum(1.5, 1)", "sum(2000, 1)", "rand(5, 1)", "rand(1)", "min()", "7 % 0", "1 +* 2", "(1", "1 2", "f(", "sum(1 2)",
		strings.Repeat("sum(2, ", 30) + "1" + strings.Repeat(")", 30), // deep, and 2^30 repeats if it were allowed
	} {
		if got, err := EvalExpr(expr, nil); err == nil {
			t.Errorf("%s must fail, got %v", expr, got)
		}
	}
	// nested repeats are bounded by the budget of evaluations, not by trust
	if _, err := EvalExpr("sum(1000, sum(1000, sum(1000, 1)))", nil); err == nil {
		t.Error("a billion evaluations must be refused")
	}
}

func TestEmbedHostRefusesOtherNodes(t *testing.T) {
	h := &embedHost{}
	if _, err := h.Source(context.Background(), Node{Type: "source.step"}); err == nil {
		t.Error("an entity source cannot run here")
	}
	if err := h.Sink(context.Background(), Node{Type: "sink.step_progress"}, nil); err == nil {
		t.Error("an entity sink cannot run here")
	}
}
