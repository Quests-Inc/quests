package graph

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func runnerWeather(params string) Graph {
	return Graph{
		Nodes: []Node{
			node(1, "args", `{"params":[{"name":"city","type":"str"}]}`),
			node(2, "http.runner", params),
			node(3, "json.get", `{"path":"t","type":"float"}`),
			node(4, "expr", `{"vars":["t"]}`),
			node(5, "sink.result", ""),
		},
		Edges: []Edge{link(1, "city", 2, "path"), link(2, "body", 3, "in"), link(1, "city", 4, "expr"), link(3, "value", 4, "t"), link(4, "text", 5, "value")},
	}
}

func TestRunnerHTTPMakesAJobAndReadsTheAnswer(t *testing.T) {
	g := runnerWeather(`{"host":"192.168.1.5:8080","scheme":"http","path":"/v1/{path}","query":{"u":"c"},"allow":["192.168.1.5"],"headers":{"X-Test":"1"}}`)
	if !g.RunsOnRunner() {
		t.Fatal("a runner node needs a runner")
	}
	run, err := NewRun(g, Builtin())
	if err != nil {
		t.Fatal(err)
	}
	var out string
	h := &embedHost{args: map[string]string{"city": "t"}, result: &out}
	reqs, err := run.Advance(context.Background(), h)
	if err != nil || len(reqs) != 1 {
		t.Fatalf("reqs=%v err=%v", reqs, err)
	}
	lines := strings.SplitN(reqs[0].Text, "\n", 2)
	if lines[0] != RunnerHTTPMarker {
		t.Fatalf("marker: %q", lines[0])
	}
	var job RunnerHTTPJob
	if err := json.Unmarshal([]byte(lines[1]), &job); err != nil {
		t.Fatal(err)
	}
	if job.URL != "http://192.168.1.5:8080/v1/t?u=c" || job.TimeoutSec != 20 || job.MaxBytes != defaultRunnerHTTPSize || job.Allow[0] != "192.168.1.5" || job.Headers["X-Test"] != "1" {
		t.Fatalf("job: %+v", job)
	}
	ans := `{"status":200,"body":"{\"t\":3.6}"}`
	if _, err := run.Resume(context.Background(), h, reqs[0].Node, CodeOutcome{Port: "stdout", Text: ans}); err != nil {
		t.Fatal(err)
	}
	if out != "3.6" {
		t.Fatalf("out=%q", out)
	}
}

func TestRunnerHTTPFailuresLeaveByFailed(t *testing.T) {
	for name, o := range map[string]CodeOutcome{
		"job failed": {Port: "failed", Text: `{"kind":"exit","message":"код 1: connection refused"}`},
		"status":     {Port: "stdout", Text: `{"status":503,"body":"down"}`},
		"too large":  {Port: "stdout", Text: `{"status":200,"cut":true}`},
		"old runner": {Port: "stdout", Text: `not json`},
	} {
		run, err := NewRun(runnerWeather(`{"host":"api.example.org"}`), Builtin())
		if err != nil {
			t.Fatal(err)
		}
		var out string
		h := &embedHost{args: map[string]string{"city": "t"}, result: &out}
		reqs, _ := run.Advance(context.Background(), h)
		if _, err := run.Resume(context.Background(), h, reqs[0].Node, o); err != nil {
			t.Fatal(name, err)
		}
		if out != "" || run.State(5) == StateRan {
			t.Errorf("%s: a failed request must not reach Result (out=%q)", name, out)
		}
	}
}

func TestRunnerHTTPParams(t *testing.T) {
	reg := Builtin()
	for _, bad := range []string{
		`{"host":"a.org","scheme":"ftp"}`,
		`{"host":"a.org","allow":["bad host"]}`,
		`{"host":"a.org","timeout_sec":999}`,
		`{"host":"a.org","max_bytes":99999999}`,
		`{"host":"a.org","headers":{"Cookie":"x"}}`,
		`{"host":"https://a.org"}`,
	} {
		if _, _, err := reg["http.runner"].Ports(json.RawMessage(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	in, _, err := reg["http.runner"].Ports(json.RawMessage(`{"host":"a.org","allow":["*.a.org","b.org"],"auth":{"header":"X-Key","secret":"KEY"}}`))
	if err != nil {
		t.Fatal(err)
	}
	has := false
	for _, p := range in {
		has = has || p.Name == "secrets"
	}
	if !has {
		t.Error("auth needs the secrets input")
	}
}

func TestSecretsReachRequestNodesWithAuth(t *testing.T) {
	g := Graph{
		Nodes: []Node{
			node(1, "source.note", `{"id":1}`),
			node(2, "http.get", `{"host":"api.example.org","auth":{"header":"X-Key","secret":"KEY"}}`),
			node(3, "json.get", `{"path":"a","type":"str"}`),
			node(4, "sink.value", `{"name":"v","type":"str"}`),
		},
		Edges: []Edge{link(1, "secrets", 2, "secrets"), link(1, "ref", 4, "ref"), link(2, "body", 3, "in"), link(3, "value", 4, "value")},
	}
	if errs := g.Validate(Builtin()); len(errs) != 0 {
		t.Fatalf("secrets into http.get: %v", errs)
	}
	g.Nodes[1] = node(2, "compare", `{"type":"int","op":"ge","operand":1}`)
	g.Edges = []Edge{link(1, "secrets", 2, "in")}
	if errs := g.Validate(Builtin()); len(errs) == 0 {
		t.Fatal("secrets are not for a compare node")
	}
}
