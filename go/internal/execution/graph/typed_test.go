package graph

import (
	"encoding/json"
	"testing"
)

func TestTypedConstantAndConversions(t *testing.T) {
	for _, p := range []string{`{"type":"int","value":9007199254740993}`, `{"type":"str","value":"Я"}`, `{"type":"bool","value":false}`, `{"type":"doc","value":null}`} {
		g := Graph{Nodes: []Node{node(1, "constant", p), node(2, "sink.result", "")}, Edges: []Edge{{From: End{1, "value"}, To: End{2, "value"}}}}
		if _, err := Evaluate(t.Context(), g, Builtin(), nil); err != nil {
			t.Fatal(p, err)
		}
	}
	if _, _, err := constantPorts(json.RawMessage(`{"type":"int","value":null}`)); err == nil {
		t.Fatal("null int accepted")
	}
	cases := []struct {
		p    string
		in   Token
		want any
		bad  bool
	}{
		{`{"from":"float","to":"int"}`, Token{Type: TFloat, Value: 1.5}, nil, true},
		{`{"from":"float","to":"int","round":"floor"}`, Token{Type: TFloat, Value: -1.5}, int64(-2), false},
		{`{"from":"str","to":"bool"}`, Token{Type: TStr, Value: "false"}, false, false},
		{`{"from":"str","to":"bool"}`, Token{Type: TStr, Value: "0"}, nil, true},
		{`{"from":"int","to":"float"}`, Token{Type: TInt, Value: int64(9007199254740993)}, nil, true},
		{`{"from":"doc","to":"int"}`, Token{Type: TDoc, Value: json.RawMessage(`9007199254740993`)}, int64(9007199254740993), false},
		{`{"from":"doc","to":"str","mode":"stringify"}`, Token{Type: TDoc, Value: json.RawMessage(` {"a":false} `)}, `{"a":false}`, false},
	}
	for _, tc := range cases {
		out, err := runConvert(&resolved{node: node(1, "convert", tc.p)}, tc.in)
		if (err != nil) != tc.bad {
			t.Fatal(tc.p, err)
		}
		if !tc.bad && out["value"].Value != tc.want {
			t.Fatal(tc.p, out)
		}
	}
}
func TestDynamicJSONPathAndTable(t *testing.T) {
	rs := &resolved{node: node(1, "json.get", `{"path":"fallback","type":"str"}`)}
	out, err := runJSONGet(rs, Token{Type: TDoc, Value: json.RawMessage(`{"items":[{"name":"Я"}]}`)}, Token{Type: TStr, Value: "items.0.name"})
	if err != nil || out["value"].Value != "Я" {
		t.Fatal(out, err)
	}
	rs = &resolved{node: node(1, "markdown.table", `{"table":"fallback","column":2}`)}
	out, err = runMarkdownTableInputs(rs, map[string][]Token{"text": {{Type: TStr, Value: sampleTable}}, "table": {{Type: TStr, Value: "example"}}})
	if err != nil || out["items"].Type != TDoc {
		t.Fatal(out, err)
	}
}

func TestDynamicJSONIndex(t *testing.T) {
	rs := &resolved{node: node(1, "json.get", `{"path":"fallback","type":"int"}`)}
	out, err := runJSONGet(rs, Token{Type: TDoc, Value: json.RawMessage(`{"items":[3,5]}`)}, Token{Type: TStr, Value: "items"}, Token{Type: TInt, Value: int64(1)})
	if err != nil || out["value"].Value != int64(5) {
		t.Fatal(out, err)
	}
}
