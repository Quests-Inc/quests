package graph

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildTypedJSON(t *testing.T) {
	g := Graph{Nodes: []Node{node(1, "args", `{"params":[{"name":"s","type":"str"},{"name":"n","type":"int"},{"name":"b","type":"bool"}]}`), node(2, "json.build", `{"inputs":[{"name":"s","type":"str"},{"name":"n","type":"int"},{"name":"b","type":"bool"}],"template":{"text":{"$input":"s"},"nested":[{"$input":"n"},{"$input":"b"},null],"large":9007199254740993}}`), node(3, "sink.result", "")}, Edges: []Edge{{From: End{1, "s"}, To: End{2, "s"}}, {From: End{1, "n"}, To: End{2, "n"}}, {From: End{1, "b"}, To: End{2, "b"}}, {From: End{2, "value"}, To: End{3, "value"}}}}
	s := "кавычки \" и\nUnicode Я"
	out, err := Evaluate(t.Context(), g, Builtin(), map[string]string{"s": s, "n": "0", "b": "false"})
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]json.RawMessage
	if err = json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if string(v["large"]) != "9007199254740993" || string(v["nested"]) != "[0,false,null]" {
		t.Fatal(out)
	}
	var got string
	_ = json.Unmarshal(v["text"], &got)
	if got != s {
		t.Fatal(out)
	}
}

const sampleTable = "<!-- table-id: example -->\n\n| Number | Choice |\n|---|---|\n| <!-- row-id: a -->1 | **one** \\| two |\n| <!-- row-id: b -->2 | — |\n"

func TestMarkdownTable(t *testing.T) {
	table, err := readMarkdownTable(sampleTable, "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(table.Rows) != 2 || table.Rows[0].ID != "a" || table.Rows[0].Cells[1] != "one | two" {
		t.Fatalf("%+v", table)
	}
	rs := &resolved{node: node(1, "markdown.table", `{"table":"example","column":2,"omit":["—"]}`)}
	out, err := runMarkdownTable(rs, Token{Type: TStr, Value: sampleTable})
	if err != nil {
		t.Fatal(err)
	}
	if string(out["items"].Value.(json.RawMessage)) != `[{"id":"a","value":"one | two"}]` {
		t.Fatal(out)
	}
	for _, s := range []string{strings.ReplaceAll(sampleTable, "row-id: b", "row-id: a"), strings.ReplaceAll(sampleTable, "<!-- row-id: a -->", ""), sampleTable + "\n" + sampleTable, "<!-- table-id: example -->\n\n| a | b |\n|---|---|\n"} {
		if _, err := readMarkdownTable(s, "example"); err == nil {
			t.Fatalf("accepted invalid table: %s", s)
		}
	}
}
func TestEmbeddedReadAndTarget(t *testing.T) {
	g := Graph{Nodes: []Node{node(1, "source.note", `{"id":9}`), node(2, "markdown.table", `{"table":"example"}`), node(3, "random", `{"kind":"pick"}`), node(4, "json.get", `{"path":"id","type":"str"}`), node(5, "json.build", `{"inputs":[{"name":"row","type":"str"}],"template":{"kind":"markdown-target","version":1,"text":"Chosen","target":{"table":"example","row":{"$input":"row"}}}}`), node(6, "sink.result", "")}, Edges: []Edge{{From: End{1, "description"}, To: End{2, "text"}}, {From: End{2, "rows"}, To: End{3, "list"}}, {From: End{3, "value"}, To: End{4, "in"}}, {From: End{4, "value"}, To: End{5, "row"}}, {From: End{5, "value"}, To: End{6, "value"}}}}
	reader := func(_ context.Context, id int64) (string, string, error) {
		if id != 9 {
			t.Fatal(id)
		}
		return "Example", sampleTable, nil
	}
	for range 20 {
		out, err := EvaluateFetch(t.Context(), g, Builtin(), nil, nil, nil, reader)
		if err != nil {
			t.Fatal(err)
		}
		var v struct{ Target struct{ Row string } }
		if json.Unmarshal([]byte(out), &v) != nil || (v.Target.Row != "a" && v.Target.Row != "b") {
			t.Fatal(out)
		}
	}
	if _, err := Evaluate(t.Context(), g, Builtin(), nil); err == nil {
		t.Fatal("read without host allowed")
	}
}
func TestTextBuild(t *testing.T) {
	rs := &resolved{node: node(1, "text.build", `{"inputs":[{"name":"s","type":"doc"}],"template":"Value: {{s}}"}`)}
	out, err := runBuild(rs, map[string][]Token{"s": {{Type: TDoc, Value: json.RawMessage(`"hello\\nworld"`)}}})
	if err != nil {
		t.Fatal(err)
	}
	if out["value"].Value != "Value: hello\\nworld" {
		t.Fatal(out)
	}
	for _, p := range []string{`{"inputs":[],"template":{"$input":"missing"}}`, `{"inputs":[{"name":"a","type":"secrets"}],"template":{}}`} {
		if _, _, err := buildPorts([]byte(p), false); err == nil {
			t.Fatal(p)
		}
	}
}

func TestTableColumnInputOverridesParameter(t *testing.T) {
	rs := &resolved{node: node(1, "markdown.table", `{"table":"example","column":1,"omit":["—"]}`)}
	out, err := runMarkdownTable(rs, Token{Type: TStr, Value: sampleTable}, Token{Type: TInt, Value: int64(2)})
	if err != nil {
		t.Fatal(err)
	}
	if string(out["items"].Value.(json.RawMessage)) != `[{"id":"a","value":"one | two"}]` {
		t.Fatal(out)
	}
	for _, col := range []int64{-1, 3, 101} {
		if _, err := runMarkdownTable(rs, Token{Type: TStr, Value: sampleTable}, Token{Type: TInt, Value: col}); err == nil {
			t.Fatal(col)
		}
	}
}
