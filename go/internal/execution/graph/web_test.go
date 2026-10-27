package graph

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func weatherGraph(httpParams, jsonParams string) Graph {
	return Graph{
		Nodes: []Node{
			node(1, "args", `{"params":[{"name":"city","type":"str"}]}`),
			node(2, "http.get", httpParams),
			node(3, "json.get", jsonParams),
			node(4, "sink.result", ""),
		},
		Edges: []Edge{link(1, "city", 2, "path"), link(2, "body", 3, "in"), link(3, "value", 4, "value")},
	}
}

func TestHTTPGetBuildsTheURLFromWhatTheOwnerFixed(t *testing.T) {
	g := weatherGraph(`{"host":"API.Example.org","path":"/v1/{path}","query":{"units":"metric"}}`, `{"path":"main.name","type":"str"}`)
	var got FetchRequest
	fetch := func(_ context.Context, r FetchRequest) (FetchResult, error) {
		got = r
		return FetchResult{Status: 200, Body: []byte(`{"main":{"name":"Москва","temp":7.5}}`)}, nil
	}
	out, err := EvaluateFetch(t.Context(), g, Builtin(), map[string]string{"_": "Moscow City/../x"}, nil, fetch)
	if err != nil || out != "Москва" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	// The host is the owner's; what came from outside is one escaped piece of the path and cannot move to another host or path.
	if want := "https://api.example.org/v1/Moscow%20City%2F..%2Fx?units=metric"; got.URL != want {
		t.Fatalf("url = %q, want %q", got.URL, want)
	}
}

func TestHTTPGetFailuresAreData(t *testing.T) {
	// A failure leaves by `failed`, so the value never arrives and the run ends without a Result: the caller sees that.
	for name, fetch := range map[string]Fetcher{
		"denied": func(context.Context, FetchRequest) (FetchResult, error) {
			return FetchResult{}, &FetchFailure{Kind: FetchDenied, Message: "нельзя"}
		},
		"status": func(context.Context, FetchRequest) (FetchResult, error) {
			return FetchResult{Status: 503, Body: []byte("down")}, nil
		},
		"plain error": func(context.Context, FetchRequest) (FetchResult, error) { return FetchResult{}, errors.New("boom") },
	} {
		g := weatherGraph(`{"host":"api.example.org"}`, `{"path":"a","type":"str"}`)
		if _, err := EvaluateFetch(t.Context(), g, Builtin(), map[string]string{"_": "x"}, nil, fetch); err == nil || !strings.Contains(err.Error(), "результата нет") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// With nothing to send through, the node fails instead of reaching anywhere.
	g := weatherGraph(`{"host":"api.example.org"}`, `{"path":"a","type":"str"}`)
	if _, err := EvaluateFetch(t.Context(), g, Builtin(), map[string]string{"_": "x"}, nil, nil); err == nil {
		t.Error("no fetcher, no request")
	}
}

func TestHTTPGetFailedDocument(t *testing.T) {
	g := Graph{
		Nodes: []Node{
			node(1, "args", `{"params":[{"name":"q","type":"str"}]}`),
			node(2, "http.get", `{"host":"api.example.org"}`),
			node(3, "parse", `{"schema":[{"name":"kind","type":"str","required":true},{"name":"message","type":"str","required":true}]}`),
			node(4, "sink.result", ""),
		},
		Edges: []Edge{link(1, "q", 2, "path"), link(2, "failed", 3, "in"), link(3, "kind", 4, "value")},
	}
	fetch := func(context.Context, FetchRequest) (FetchResult, error) {
		return FetchResult{}, &FetchFailure{Kind: FetchTooLarge, Message: "большой"}
	}
	out, err := EvaluateFetch(t.Context(), g, Builtin(), map[string]string{"_": "x"}, nil, fetch)
	if err != nil || out != FetchTooLarge {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestHTTPGetParams(t *testing.T) {
	reg := Builtin()
	ok := []string{`{"host":"api.example.org"}`, `{"host":"a.b-c.example.org:8443","path":"/x/{path}"}`,
		`{"host":"api.example.org","auth":{"header":"Authorization","secret":"API_TOKEN","prefix":"Bearer "}}`}
	for _, p := range ok {
		if _, _, err := reg["http.get"].Ports(json.RawMessage(p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	for _, p := range []string{
		`{}`, `{"host":""}`, `{"host":"https://api.example.org"}`, `{"host":"api.example.org/x"}`, `{"host":"user@api.example.org"}`, `{"host":"-bad.org"}`,
		`{"host":"api.example.org","path":"x"}`, `{"host":"api.example.org","path":"/x?y=1"}`, `{"host":"api.example.org","path":"/a b"}`,
		`{"host":"api.example.org","auth":{"header":"Host","secret":"X"}}`, `{"host":"api.example.org","auth":{"header":"Cookie","secret":"X"}}`,
		`{"host":"api.example.org","auth":{"header":"Authorization","secret":"has space"}}`, `{"host":"api.example.org","extra":1}`,
	} {
		if _, _, err := reg["http.get"].Ports(json.RawMessage(p)); err == nil {
			t.Errorf("%s must be refused", p)
		}
	}
	// The secret only goes into a header the node names, so the `secrets` input exists exactly then.
	in, _, _ := reg["http.get"].Ports(json.RawMessage(ok[2]))
	has := false
	for _, p := range in {
		has = has || p.Name == "secrets"
	}
	if !has {
		t.Error("auth needs the secrets input")
	}
	if names := (Graph{Nodes: []Node{node(1, "http.get", ok[0]), node(2, "json.get", `{"path":"a","type":"str"}`)}}).UsesNetwork(reg); len(names) != 1 || names[0] != "http.get" {
		t.Errorf("UsesNetwork = %v", names)
	}
}

func TestJSONGet(t *testing.T) {
	reg := Builtin()
	run := func(path, typ, doc string) map[string]Token {
		rs := &resolved{node: node(1, "json.get", `{"path":"`+path+`","type":"`+typ+`"}`), typ: reg["json.get"]}
		out, err := runJSONGet(rs, Token{Type: TDoc, Value: json.RawMessage(doc)})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	const doc = `{"current":{"t":7.25,"n":3,"ok":true,"name":"x","list":[{"id":10},{"id":20}]}}`
	if v := run("current.t", "float", doc)["value"].Value; v != 7.25 {
		t.Errorf("float: %v", v)
	}
	if v := run("current.n", "int", doc)["value"].Value; v != int64(3) {
		t.Errorf("int: %v", v)
	}
	if v := run("current.ok", "bool", doc)["value"].Value; v != true {
		t.Errorf("bool: %v", v)
	}
	if v := run("current.list.1.id", "int", doc)["value"].Value; v != int64(20) {
		t.Errorf("index: %v", v)
	}
	if v := run("current.list", "doc", doc)["value"].Value.(json.RawMessage); !strings.HasPrefix(string(v), `[{"id":10}`) {
		t.Errorf("doc: %s", v)
	}
	for _, c := range [][3]string{
		{"current.zzz", "int", doc}, {"current.t", "int", doc}, {"current.name", "float", doc}, {"current.list.5", "int", doc},
		{"current.n.x", "int", doc}, {"current.t", "int", `not json`},
	} {
		out := run(c[0], c[1], c[2])
		if _, got := out["value"]; got || out["bad"].Value == nil {
			t.Errorf("%v must be bad: %v", c, out)
		}
	}
	for _, p := range []string{`{"path":"","type":"int"}`, `{"path":"a..b","type":"int"}`, `{"path":"a b","type":"int"}`, `{"path":"a","type":"list"}`} {
		if _, _, err := reg["json.get"].Ports(json.RawMessage(p)); err == nil {
			t.Errorf("%s must be refused", p)
		}
	}
}

func TestQueryValuesTakeTheArrivingPiece(t *testing.T) {
	g := weatherGraph(`{"host":"api.example.org","path":"/v1/forecast","query":{"lat":"{path.0}","lon":"{path.1}","q":"{path}","z":"{path.5}"}}`, `{"path":"main.t","type":"str"}`)
	var got FetchRequest
	fetch := func(_ context.Context, r FetchRequest) (FetchResult, error) {
		got = r
		return FetchResult{Status: 200, Body: []byte(`{"main":{"t":3.6}}`)}, nil
	}
	out, err := EvaluateFetch(t.Context(), g, Builtin(), map[string]string{"_": "55.75, 37.62"}, nil, fetch)
	if err != nil || out != "3.6" {
		t.Fatalf("a number reads as text: out=%q err=%v", out, err)
	}
	// The piece goes only where the query says; it is not added to the path as well, and it is escaped by the encoder.
	if want := "https://api.example.org/v1/forecast?lat=55.75&lon=37.62&q=55.75%2C+37.62&z="; got.URL != want {
		t.Fatalf("url = %q, want %q", got.URL, want)
	}
}
