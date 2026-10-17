package graph

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeHost answers sources with fixed values and records what the sinks were given.
type fakeHost struct {
	sinks    []sinkCall
	sinkFail map[string]error
}

type sinkCall struct {
	typ string
	in  map[string]any
}

func (h *fakeHost) Source(_ context.Context, n Node) (map[string]any, error) {
	switch n.Type {
	case "source.step":
		return map[string]any{"ref": Ref{Kind: "step", ID: 7}, "secrets": Secrets{}, "title": "Шаг", "progress": 2, "total": 10, "done": false}, nil
	case "source.note":
		return map[string]any{"ref": Ref{Kind: "note", ID: 3}, "secrets": Secrets{}, "title": "Заметка"}, nil
	case "source.template":
		return map[string]any{"ref": Ref{Kind: "template", ID: 5, Key: "2026-10-17"}, "secrets": Secrets{}, "title": "Шаблон", "period": "2026-10-17"}, nil
	case "source.quest":
		return map[string]any{"ref": Ref{Kind: "quest", ID: 9}, "status": "completed"}, nil
	}
	return nil, nil
}

func (h *fakeHost) Sink(_ context.Context, n Node, in map[string]Token) error {
	call := sinkCall{typ: n.Type, in: map[string]any{}}
	for k, t := range in {
		call.in[k] = t.Value
	}
	h.sinks = append(h.sinks, call)
	return h.sinkFail[n.Type]
}

func (h *fakeHost) last(t *testing.T) sinkCall {
	t.Helper()
	if len(h.sinks) == 0 {
		t.Fatal("no sink ran")
	}
	return h.sinks[len(h.sinks)-1]
}

// run drives a graph: advance, deliver the outcome to the one Code node, and return the host.
func runGraph(t *testing.T, g Graph, o *CodeOutcome) (*Run, *fakeHost, []CodeRequest) {
	t.Helper()
	r, err := NewRun(g, Builtin())
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHost{}
	reqs, err := r.Advance(context.Background(), h)
	if err != nil {
		t.Fatal(err)
	}
	if o != nil {
		if len(reqs) != 1 {
			t.Fatalf("expected one code request, got %d", len(reqs))
		}
		if _, err := r.Resume(context.Background(), h, reqs[0].Node, *o); err != nil {
			t.Fatal(err)
		}
	}
	return r, h, reqs
}

func stdout(s string) *CodeOutcome { return &CodeOutcome{Port: "stdout", Text: s} }

func TestStepPollParsesStrictly(t *testing.T) {
	g := legacyGraphs()["step poll"]
	// The schema in the fixture has progress, total (min 1) and description.
	for _, tc := range []struct {
		name, out string
		want      map[string]any // nil = the sink must not run
	}{
		{"a bare integer", "5", map[string]any{"progress": int64(5)}},
		{"an object", `{"progress":3,"total":10,"description":"half"}`, map[string]any{"progress": int64(3), "total": int64(10), "description": "half"}},
		{"a log above the answer", "checking…\nall good\n7", map[string]any{"progress": int64(7)}},
		{"a multi-line object", "{\n  \"progress\": 4\n}\n", map[string]any{"progress": int64(4)}},
		{"an integral float", `{"progress": 5.0}`, map[string]any{"progress": int64(5)}},
		{"total below the minimum is left out", `{"progress":1,"total":0}`, map[string]any{"progress": int64(1)}},
		{"only a total", `{"total":4}`, map[string]any{"total": int64(4)}},
		{"free text is not an answer", "готово: 5", nil},
		{"a wrong type is left out, nothing remains", `{"progress":"many"}`, nil},
		{"an empty answer", "", nil},
		{"an array", `[1,2]`, nil},
	} {
		r, h, _ := runGraph(t, g, stdout(tc.out))
		if !r.Finished() {
			t.Errorf("%s: the run did not finish", tc.name)
		}
		switch {
		case tc.want == nil && len(h.sinks) != 0:
			t.Errorf("%s: the sink ran with %v", tc.name, h.sinks)
		case tc.want != nil && (len(h.sinks) != 1 || !reflect.DeepEqual(h.last(t).in["progress"], tc.want["progress"]) ||
			!reflect.DeepEqual(h.last(t).in["total"], tc.want["total"]) || !reflect.DeepEqual(h.last(t).in["description"], tc.want["description"])):
			t.Errorf("%s: sink got %v, want %v", tc.name, h.sinks, tc.want)
		}
	}
}

func TestStepOnceFollowsTheKindOfTheFailure(t *testing.T) {
	g := legacyGraphs()["step once"]
	exit3 := 3
	for _, tc := range []struct {
		name    string
		outcome CodeOutcome
		result  string // which sink ran, told apart by what it was asked to do
		why     string
	}{
		{"a clean exit", CodeOutcome{Port: "stdout"}, "done", ""},
		{"an error exit", Failure{Kind: FailExit, Message: "код 3: boom", ExitCode: &exit3}.Outcome(), "failed", "код 3: boom"},
		{"a timeout", Failure{Kind: FailTimeout, Message: "таймаут"}.Outcome(), "failed", "таймаут"},
		{"a device that never came", Failure{Kind: FailUnreachable, Message: "устройство не вышло на связь"}.Outcome(), "not_started", "устройство не вышло на связь"},
		{"a cancel by hand", Failure{Kind: FailCanceled, Message: "запуск отменён"}.Outcome(), "stopped", "запуск отменён"},
		{"a device that vanished", Failure{Kind: FailAbandoned, Message: "пропало"}.Outcome(), "failed", "пропало"},
	} {
		o := tc.outcome
		r, h, _ := runGraph(t, g, &o)
		if len(h.sinks) != 1 || !r.Finished() {
			t.Errorf("%s: %d sinks ran, want exactly one", tc.name, len(h.sinks))
			continue
		}
		var want string
		for _, n := range g.Nodes {
			if n.Type == "sink.step_result" && r.State(n.ID) == StateRan {
				want = strings.TrimSuffix(strings.TrimPrefix(string(n.Params), `{"result":"`), `"}`)
			}
		}
		if want != tc.result {
			t.Errorf("%s: the %q sink ran, want %q", tc.name, want, tc.result)
		}
		if why, _ := h.last(t).in["why"].(string); tc.why != "" && why != tc.why {
			t.Errorf("%s: why = %q, want %q", tc.name, why, tc.why)
		}
	}
}

func TestFailureIsADocumentWithAKind(t *testing.T) {
	exit := 7
	o := Failure{Kind: FailExit, Message: "код 7: boom", ExitCode: &exit, Stderr: "boom"}.Outcome()
	if o.Port != "failed" {
		t.Fatalf("port %q", o.Port)
	}
	var back Failure
	if err := json.Unmarshal([]byte(o.Text), &back); err != nil || back.Kind != "failed" || back.ExitCode == nil || *back.ExitCode != 7 || back.Stderr != "boom" {
		t.Fatalf("%s %+v %v", o.Text, back, err)
	}
	// Only what the kind has is in it.
	if strings.Contains(Failure{Kind: FailCanceled, Message: "x"}.Outcome().Text, "exit_code") {
		t.Fatal("a cancel has no exit code")
	}
}

func TestNoteVerdictAndErrors(t *testing.T) {
	g := legacyGraphs()["note"]
	_, h, _ := runGraph(t, g, stdout(`{"outdated": 1, "reason": "старо"}`))
	if c := h.last(t); c.typ != "sink.note_verdict" || c.in["outdated"] != true || c.in["reason"] != "старо" {
		t.Fatalf("verdict: %+v", c)
	}
	_, h, _ = runGraph(t, g, stdout("0"))
	if c := h.last(t); c.typ != "sink.note_verdict" || c.in["outdated"] != false || c.in["reason"] != nil {
		t.Fatalf("a bare 0: %+v", c)
	}
	// Not an answer: the verdict stays, the error is recorded with the reason.
	_, h, _ = runGraph(t, g, stdout("maybe"))
	if len(h.sinks) != 1 || h.last(t).typ != "sink.note_error" || !strings.Contains(h.last(t).in["message"].(string), "не JSON") {
		t.Fatalf("a bad answer: %+v", h.sinks)
	}
	_, h, _ = runGraph(t, g, stdout(`{"reason":"x"}`))
	if h.last(t).typ != "sink.note_error" || !strings.Contains(h.last(t).in["message"].(string), `"outdated"`) {
		t.Fatalf("a missing required field: %+v", h.sinks)
	}
	// Every ending but a clean exit is an error on the note, with the message the failure carries.
	for _, f := range []Failure{{Kind: FailExit, Message: "код 2"}, {Kind: FailTimeout, Message: "таймаут"}, {Kind: FailUnreachable, Message: "нет устройства"}, {Kind: FailCanceled, Message: "запуск отменён"}} {
		o := f.Outcome()
		_, h, _ = runGraph(t, g, &o)
		if len(h.sinks) != 1 || h.last(t).typ != "sink.note_error" || h.last(t).in["message"] != f.Message {
			t.Fatalf("%s: %+v", f.Kind, h.sinks)
		}
	}
}

func TestTemplateDocumentIsTheWholeOutput(t *testing.T) {
	g := legacyGraphs()["template"]
	_, h, _ := runGraph(t, g, stdout(`{"title":"Квест","steps":[{"title":"s"}]}`))
	doc, _ := h.last(t).in["doc"].(json.RawMessage)
	if h.last(t).typ != "sink.template_create" || !strings.Contains(string(doc), `"Квест"`) {
		t.Fatalf("create: %+v", h.sinks)
	}
	_, h, _ = runGraph(t, g, stdout("null"))
	if h.last(t).typ != "sink.template_create" || string(h.last(t).in["doc"].(json.RawMessage)) != "null" {
		t.Fatalf("null means nothing this period, and still reaches the sink: %+v", h.sinks)
	}
	_, h, _ = runGraph(t, g, stdout("not json"))
	if h.last(t).typ != "sink.template_error" {
		t.Fatalf("bad output: %+v", h.sinks)
	}
	// A failed run is an attempt that failed, with the message the failure carries.
	o := Failure{Kind: FailTimeout, Message: "таймаут"}.Outcome()
	_, h, _ = runGraph(t, g, &o)
	if len(h.sinks) != 1 || h.last(t).typ != "sink.template_error" || h.last(t).in["message"] != "таймаут" {
		t.Fatalf("a failed run: %+v", h.sinks)
	}
}

// A graph with no Code node runs to the end at once: a step that follows a quest (quest=298).
func TestServerOnlyGraphRunsAtOnce(t *testing.T) {
	g := Graph{
		Nodes: []Node{node(1, "source.quest", ""), node(2, "compare", `{"type":"str","op":"eq","operand":"completed"}`),
			node(3, "source.step", ""), node(4, "sink.step_result", `{"result":"done"}`)},
		Edges: []Edge{link(1, "status", 2, "in"), link(3, "ref", 4, "ref"), link(2, "yes", 4, "trigger")},
	}
	r, h, reqs := runGraph(t, g, nil)
	if len(reqs) != 0 || !r.Finished() || len(h.sinks) != 1 {
		t.Fatalf("reqs %v finished %v sinks %v", reqs, r.Finished(), h.sinks)
	}
	g.Nodes[1] = node(2, "compare", `{"type":"str","op":"eq","operand":"failed"}`)
	r, h, _ = runGraph(t, g, nil)
	if !r.Finished() || len(h.sinks) != 0 || r.State(4) != StateSkipped {
		t.Fatalf("the other branch must not run: state %v sinks %v", r.State(4), h.sinks)
	}
}

func TestExitSignalTravelsDownAndGuardsTheCode(t *testing.T) {
	// A guard in front of the code: the step's total must be over 100, else no job at all.
	g := Graph{
		Nodes: []Node{node(1, "source.step", ""), node(2, "compare", `{"type":"int","op":"gt","operand":100}`), node(3, "code", codeEcho),
			node(4, "sink.step_result", `{"result":"done"}`)},
		Edges: []Edge{link(1, "total", 2, "in"), link(2, "yes", 3, "in"), link(1, "ref", 4, "ref"), link(3, "stdout", 4, "trigger")},
	}
	r, h, reqs := runGraph(t, g, nil)
	if len(reqs) != 0 || !r.Finished() || r.State(3) != StateSkipped || r.State(4) != StateSkipped || len(h.sinks) != 0 {
		t.Fatalf("a guarded code must not be asked for: %v states %v %v", reqs, r.State(3), r.State(4))
	}
	g.Nodes[1] = node(2, "compare", `{"type":"int","op":"gt","operand":5}`)
	_, _, reqs = runGraph(t, g, nil)
	if len(reqs) != 1 || reqs[0].Env["QUESTS_IN_N2_YES"] != "10" {
		t.Fatalf("the code is asked for with its inputs: %+v", reqs)
	}
}

func TestCodeRequestCarriesInputsAndSecrets(t *testing.T) {
	g := Graph{
		Nodes: []Node{node(1, "source.step", ""), node(2, "code", `{"language":"python","text":"print(1)"}`), node(3, "sink.step_result", `{"result":"done"}`)},
		Edges: []Edge{link(1, "secrets", 2, "secrets"), link(1, "title", 2, "in"), link(1, "done", 2, "in"), link(1, "ref", 3, "ref"), link(2, "stdout", 3, "trigger")},
	}
	g.Edges[1].Alias = "NAME"
	_, _, reqs := runGraph(t, g, nil)
	if len(reqs) != 1 {
		t.Fatalf("%v", reqs)
	}
	want := map[string]string{"QUESTS_IN_NAME": "Шаг", "QUESTS_IN_N1_DONE": "false"}
	if !reflect.DeepEqual(reqs[0].Env, want) || !reqs[0].Secrets || reqs[0].Language != "python" || reqs[0].Text != "print(1)" {
		t.Fatalf("request: %+v", reqs[0])
	}
	// No secrets link: the node does not take them.
	g.Edges = append(g.Edges[:0:0], g.Edges[1:]...)
	_, _, reqs = runGraph(t, g, nil)
	if reqs[0].Secrets {
		t.Fatal("secrets are taken only when wired")
	}
}

func TestSnapshotSurvivesTheJob(t *testing.T) {
	g := legacyGraphs()["step poll"]
	r, err := NewRun(g, Builtin())
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHost{}
	reqs, err := r.Advance(context.Background(), h)
	if err != nil || len(reqs) != 1 || r.Finished() || len(r.Waiting()) != 1 {
		t.Fatalf("suspended at the code: %v %v", reqs, err)
	}
	snap, err := r.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(snap)

	// Later, in another process: thaw and deliver the outcome.
	var back Snapshot
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	r2, err := Restore(g, Builtin(), back)
	if err != nil {
		t.Fatal(err)
	}
	h2 := &fakeHost{}
	if _, err := r2.Resume(context.Background(), h2, reqs[0].Node, CodeOutcome{Port: "stdout", Text: "9"}); err != nil {
		t.Fatal(err)
	}
	// The sink needs the ref the source produced before the freeze.
	if !r2.Finished() || len(h2.sinks) != 1 || h2.last(t).in["progress"] != int64(9) || h2.last(t).in["ref"] != (Ref{Kind: "step", ID: 7}) {
		t.Fatalf("resumed: finished %v sinks %+v", r2.Finished(), h2.sinks)
	}
	if _, err := r2.Resume(context.Background(), h2, reqs[0].Node, CodeOutcome{Port: "stdout"}); err == nil {
		t.Fatal("a finished node takes no second outcome")
	}
	if _, err := r.Resume(context.Background(), h, reqs[0].Node, CodeOutcome{Port: "nope"}); err == nil {
		t.Fatal("an unknown outcome port must be refused")
	}
}

func TestAnyOfAndAllOf(t *testing.T) {
	// Any of: the first link that holds a value, here the second (the first branch is cut by a Compare).
	g := Graph{
		Nodes: []Node{node(1, "source.step", ""), node(2, "compare", `{"type":"int","op":"gt","operand":100}`),
			node(3, "any_of", `{"type":"int"}`), node(4, "sink.step_progress", "")},
		Edges: []Edge{link(1, "total", 2, "in"), link(2, "yes", 3, "in"), link(1, "progress", 3, "in"), link(3, "out", 4, "progress"), link(1, "ref", 4, "ref")},
	}
	_, h, _ := runGraph(t, g, nil)
	if len(h.sinks) != 1 || h.last(t).in["progress"] != int64(2) {
		t.Fatalf("any of: %+v", h.sinks)
	}
	// All of: waits for every input, so one cut input cuts the output.
	g = Graph{
		Nodes: []Node{node(1, "source.step", ""), node(2, "compare", `{"type":"int","op":"gt","operand":100}`),
			node(3, "all_of", ""), node(4, "sink.step_result", `{"result":"done"}`)},
		Edges: []Edge{link(1, "total", 2, "in"), link(2, "yes", 3, "in"), link(1, "progress", 3, "in"), link(3, "out", 4, "trigger"), link(1, "ref", 4, "ref")},
	}
	r, h, _ := runGraph(t, g, nil)
	if len(h.sinks) != 0 || r.State(4) != StateSkipped {
		t.Fatalf("all of with a cut input: %+v", h.sinks)
	}
	g.Nodes[1] = node(2, "compare", `{"type":"int","op":"lt","operand":100}`)
	if _, h, _ = runGraph(t, g, nil); len(h.sinks) != 1 {
		t.Fatalf("all of with every input: %+v", h.sinks)
	}
}

func TestCompareOperations(t *testing.T) {
	for _, tc := range []struct {
		params string
		in     string // the source field wired in
		yes    bool
	}{
		{`{"type":"int","op":"ge","operand":10}`, "total", true},
		{`{"type":"int","op":"lt","operand":10}`, "total", false},
		{`{"type":"int","op":"ne","operand":10}`, "total", false},
		{`{"type":"float","op":"gt","operand":9.5}`, "total", true}, // int widens to float
		{`{"type":"str","op":"contains","operand":"аг"}`, "title", true},
		{`{"type":"str","op":"prefix","operand":"Ша"}`, "title", true},
		{`{"type":"str","op":"matches","operand":"^Ш.г$"}`, "title", true},
		{`{"type":"str","op":"eq","operand":"x"}`, "title", false},
		{`{"type":"bool","op":"eq","operand":false}`, "done", true},
	} {
		g := Graph{
			Nodes: []Node{node(1, "source.step", ""), node(2, "compare", tc.params), node(3, "sink.step_result", `{"result":"done"}`)},
			Edges: []Edge{link(1, tc.in, 2, "in"), link(1, "ref", 3, "ref"), link(2, "yes", 3, "trigger")},
		}
		_, h, _ := runGraph(t, g, nil)
		if (len(h.sinks) == 1) != tc.yes {
			t.Errorf("%s on %s: yes=%v, sinks %d", tc.params, tc.in, tc.yes, len(h.sinks))
		}
	}
}

func TestSinkFailureIsRecordedAndTheRestRuns(t *testing.T) {
	g := legacyGraphs()["note"]
	r, err := NewRun(g, Builtin())
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHost{sinkFail: map[string]error{"sink.note_verdict": errors.New("база недоступна")}}
	reqs, _ := r.Advance(context.Background(), h)
	_, err = r.Resume(context.Background(), h, reqs[0].Node, *stdout(`{"outdated":1}`))
	if err == nil || !strings.Contains(err.Error(), "база недоступна") {
		t.Fatalf("the sink's error must come back: %v", err)
	}
	if r.State(4) != StateFailed || r.Failure(4) == "" || !r.Finished() {
		t.Fatalf("state %v failure %q finished %v", r.State(4), r.Failure(4), r.Finished())
	}
}

func TestEnvTooBigFailsTheCodeNode(t *testing.T) {
	big := strings.Repeat("я", MaxEnvBytes)
	g := Graph{
		Nodes: []Node{node(1, "source.step", ""), node(2, "code", codeEcho), node(3, "sink.step_result", `{"result":"done"}`)},
		Edges: []Edge{link(1, "title", 2, "in"), link(1, "ref", 3, "ref"), link(2, "stdout", 3, "trigger")},
	}
	r, _ := NewRun(g, Builtin())
	h := &bigHost{big: big}
	_, err := r.Advance(context.Background(), h)
	if err == nil || r.State(2) != StateFailed {
		t.Fatalf("an oversized environment must fail the node: %v state %v", err, r.State(2))
	}
}

type bigHost struct {
	fakeHost
	big string
}

func (h *bigHost) Source(ctx context.Context, n Node) (map[string]any, error) {
	m, err := h.fakeHost.Source(ctx, n)
	m["title"] = h.big
	return m, err
}
