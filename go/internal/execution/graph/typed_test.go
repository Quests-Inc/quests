package graph

import (
	"encoding/json"
	"testing"
)

func TestTypedConstantAndConversions(t *testing.T) {
	for _, p := range []string{`{"type":"int","value":9007199254740993}`, `{"type":"str","value":"Я"}`, `{"type":"bool","value":false}`, `{"type":"doc","value":null}`, `{"type":"float","value":1.25}`} {
		g := Graph{Nodes: []Node{node(1, "constant", p), node(2, "sink.result", "")}, Edges: []Edge{{From: End{1, "value"}, To: End{2, "value"}}}}
		if _, err := Evaluate(t.Context(), g, Builtin(), nil); err != nil {
			t.Fatal(p, err)
		}
	}
	for _, bad := range []string{
		`{"type":"int","value":null}`,
		`{"type":"float","value":null}`,
		`{"type":"str","value":null}`,
		`{"type":"bool","value":null}`,
		`{"type":"doc","value":not-json}`,
		`{"type":"int"}`,
	} {
		if _, err := constantValue(json.RawMessage(bad)); err == nil {
			t.Fatal("accepted", bad)
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
		{`{"from":"float","to":"int","round":"ceil"}`, Token{Type: TFloat, Value: 1.2}, int64(2), false},
		{`{"from":"float","to":"int","round":"trunc"}`, Token{Type: TFloat, Value: -1.8}, int64(-1), false},
		{`{"from":"float","to":"int","round":"nearest"}`, Token{Type: TFloat, Value: 1.5}, int64(2), false},
		{`{"from":"str","to":"bool"}`, Token{Type: TStr, Value: "false"}, false, false},
		{`{"from":"str","to":"bool"}`, Token{Type: TStr, Value: "0"}, nil, true},
		{`{"from":"bool","to":"bool"}`, Token{Type: TBool, Value: true}, true, false},
		{`{"from":"int","to":"float"}`, Token{Type: TInt, Value: int64(9007199254740993)}, nil, true},
		{`{"from":"int","to":"float"}`, Token{Type: TInt, Value: int64(42)}, float64(42), false},
		{`{"from":"int","to":"str"}`, Token{Type: TInt, Value: int64(7)}, "7", false},
		{`{"from":"float","to":"str"}`, Token{Type: TFloat, Value: 1.5}, "1.5", false},
		{`{"from":"bool","to":"str"}`, Token{Type: TBool, Value: false}, "false", false},
		{`{"from":"str","to":"int"}`, Token{Type: TStr, Value: "12"}, int64(12), false},
		{`{"from":"str","to":"int"}`, Token{Type: TStr, Value: "x"}, nil, true},
		{`{"from":"str","to":"float"}`, Token{Type: TStr, Value: "1.5"}, 1.5, false},
		{`{"from":"float","to":"float"}`, Token{Type: TFloat, Value: 2.0}, 2.0, false},
		{`{"from":"int","to":"doc"}`, Token{Type: TInt, Value: int64(3)}, json.RawMessage(`3`), false},
		{`{"from":"doc","to":"doc"}`, Token{Type: TDoc, Value: json.RawMessage(`{"a":1}`)}, json.RawMessage(`{"a":1}`), false},
		{`{"from":"doc","to":"int"}`, Token{Type: TDoc, Value: json.RawMessage(`9007199254740993`)}, int64(9007199254740993), false},
		{`{"from":"doc","to":"float"}`, Token{Type: TDoc, Value: json.RawMessage(`1.25`)}, 1.25, false},
		{`{"from":"doc","to":"str","mode":"stringify"}`, Token{Type: TDoc, Value: json.RawMessage(` {"a":false} `)}, `{"a":false}`, false},
		{`{"from":"str","to":"doc","mode":"parse_json"}`, Token{Type: TStr, Value: `{"k":1}`}, json.RawMessage(`{"k":1}`), false},
		{`{"from":"str","to":"doc","mode":"parse_json"}`, Token{Type: TStr, Value: `not-json`}, nil, true},
		{`{"from":"doc","to":"str","mode":"stringify"}`, Token{Type: TDoc, Value: json.RawMessage(`not`)}, nil, true},
	}
	for _, tc := range cases {
		out, err := runConvert(&resolved{node: node(1, "convert", tc.p)}, tc.in)
		if (err != nil) != tc.bad {
			t.Fatal(tc.p, err)
		}
		if tc.bad {
			continue
		}
		got := out["value"].Value
		if raw, ok := tc.want.(json.RawMessage); ok {
			gotRaw, ok := got.(json.RawMessage)
			if !ok || string(gotRaw) != string(raw) {
				t.Fatal(tc.p, out)
			}
			continue
		}
		if got != tc.want {
			t.Fatal(tc.p, out)
		}
	}
}

func TestConvertPorts(t *testing.T) {
	in, out, err := convertPorts(json.RawMessage(`{"from":"str","to":"int"}`))
	if err != nil || len(in) != 1 || in[0].Type != TStr || len(out) != 1 || out[0].Type != TInt {
		t.Fatal(in, out, err)
	}
	if _, _, err := convertPorts(json.RawMessage(`{"from":"str","to":"doc","mode":"parse_json"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := convertPorts(json.RawMessage(`{"from":"doc","to":"str","mode":"stringify"}`)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"from":"nope","to":"int"}`,
		`{"from":"str","to":"int","mode":"weird"}`,
		`{"from":"int","to":"doc","mode":"parse_json"}`,
		`{"from":"str","to":"int","mode":"stringify"}`,
		`{"from":"float","to":"int","round":"up"}`,
		`{`,
	} {
		if _, _, err := convertPorts(json.RawMessage(bad)); err == nil {
			t.Fatal("accepted", bad)
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
