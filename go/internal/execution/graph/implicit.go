package graph

import "encoding/json"

// The graphs of today's checks. The Code node holds the check_command as written.
type nodeSpec struct {
	id     int
	typ    string
	params any
}

func buildGraph(nodes []nodeSpec, edges ...[4]any) Graph {
	var g Graph
	for _, n := range nodes {
		node := Node{ID: n.id, Type: n.typ}
		if n.params != nil {
			node.Params, _ = json.Marshal(n.params)
		}
		g.Nodes = append(g.Nodes, node)
	}
	for _, e := range edges {
		g.Edges = append(g.Edges, Edge{
			From: End{Node: e[0].(int), Port: e[1].(string)},
			To:   End{Node: e[2].(int), Port: e[3].(string)},
		})
	}
	return g
}

type kv = map[string]any

// NoteCheck: the note is read from its source, the command runs, the answer must be an `outdated` verdict. A run that goes
// wrong (an error exit, a timeout, no device, a device that vanished, a cancel) leaves by `failed` and is recorded on the note as
// an error with the message it carries; the verdict stays as it was.
func NoteCheck(text string) Graph {
	return buildGraph([]nodeSpec{
		{1, "source.note", nil},
		{2, "code", kv{"text": text}},
		{3, "parse", kv{"schema": []kv{{"name": "outdated", "type": "bool", "required": true}, {"name": "reason", "type": "str"}}}},
		{4, "sink.note_verdict", nil},
		{5, "parse", kv{"schema": []kv{{"name": "message", "type": "str", "required": true}}}},
		{6, "sink.note_error", nil},
		{7, "sink.note_error", nil},
	},
		[4]any{1, "secrets", 2, "secrets"}, [4]any{2, "stdout", 3, "in"},
		[4]any{1, "ref", 4, "ref"}, [4]any{3, "outdated", 4, "outdated"}, [4]any{3, "reason", 4, "reason"},
		[4]any{1, "ref", 6, "ref"}, [4]any{3, "bad_output", 6, "message"},
		[4]any{2, "failed", 5, "in"}, [4]any{1, "ref", 7, "ref"}, [4]any{5, "message", 7, "message"},
	)
}

// TemplateEmit: a template makes a quest from what the command prints. The answer is one JSON document (the quest, shaped as
// the emit's limits allow: nothing printed, `{}` or `null` means no quest this time). A command that fails, or an answer
// that is not JSON, ends as an attempt error of the template, which the emit retries. A device that never showed up, and a
// cancel by hand, close the period instead: no attempt is logged and nothing is queued again.
func TemplateEmit(text string) Graph {
	return buildGraph([]nodeSpec{
		{1, "source.template", nil},
		{2, "code", kv{"text": text}},
		{3, "parse", kv{"schema": []kv{{"name": "quest", "type": "doc", "required": true}}, "whole": true}},
		{4, "sink.template_create", nil},
		{5, "parse", kv{"schema": []kv{{"name": "kind", "type": "str", "required": true}, {"name": "message", "type": "str", "required": true}}}},
		{6, "compare", kv{"type": "str", "op": "eq", "operand": FailUnreachable}},
		{7, "sink.template_error", kv{"kind": "closed", "gate": true}},
		{8, "compare", kv{"type": "str", "op": "eq", "operand": FailCanceled}},
		{9, "sink.template_error", kv{"kind": "closed", "gate": true}},
		{10, "compare", kv{"type": "str", "op": "eq", "operand": FailTimeout}},
		{11, "sink.template_error", kv{"kind": "timeout", "gate": true}},
		{12, "sink.template_error", kv{"kind": "error", "gate": true}},
		{13, "sink.template_error", kv{"kind": "bad_json"}},
	},
		[4]any{1, "secrets", 2, "secrets"}, [4]any{2, "stdout", 3, "in"},
		[4]any{1, "ref", 4, "ref"}, [4]any{3, "quest", 4, "doc"},
		[4]any{2, "failed", 5, "in"}, [4]any{5, "kind", 6, "in"},
		[4]any{1, "ref", 7, "ref"}, [4]any{6, "yes", 7, "trigger"}, [4]any{5, "message", 7, "message"},
		[4]any{6, "no", 8, "in"},
		[4]any{1, "ref", 9, "ref"}, [4]any{8, "yes", 9, "trigger"}, [4]any{5, "message", 9, "message"},
		[4]any{8, "no", 10, "in"},
		[4]any{1, "ref", 11, "ref"}, [4]any{10, "yes", 11, "trigger"}, [4]any{5, "message", 11, "message"},
		[4]any{1, "ref", 12, "ref"}, [4]any{10, "no", 12, "trigger"}, [4]any{5, "message", 12, "message"},
		[4]any{1, "ref", 13, "ref"}, [4]any{3, "bad_output", 13, "message"},
	)
}

// StepOnce: a one-shot step. A clean exit closes the step. Whatever else happens leaves by `failed`, and its `kind` decides: a
// device that never showed up puts the step back to wait; a cancel by hand stops it without failing the quest; anything else
// (an error exit, a timeout, a device that vanished mid-run) fails the quest with the message.
func StepOnce(text string) Graph {
	return buildGraph([]nodeSpec{
		{1, "source.step", nil},
		{2, "code", kv{"text": text}},
		{3, "sink.step_result", kv{"result": "done"}},
		{4, "parse", kv{"schema": []kv{{"name": "kind", "type": "str", "required": true}, {"name": "message", "type": "str", "required": true}}}},
		{5, "compare", kv{"type": "str", "op": "eq", "operand": FailUnreachable}},
		{6, "sink.step_result", kv{"result": "not_started"}},
		{7, "compare", kv{"type": "str", "op": "eq", "operand": FailCanceled}},
		{8, "sink.step_result", kv{"result": "stopped"}},
		{9, "sink.step_result", kv{"result": "failed"}},
	},
		[4]any{1, "secrets", 2, "secrets"},
		[4]any{1, "ref", 3, "ref"}, [4]any{2, "stdout", 3, "trigger"},
		[4]any{2, "failed", 4, "in"}, [4]any{4, "kind", 5, "in"},
		[4]any{1, "ref", 6, "ref"}, [4]any{5, "yes", 6, "trigger"}, [4]any{4, "message", 6, "why"},
		[4]any{5, "no", 7, "in"},
		[4]any{1, "ref", 8, "ref"}, [4]any{7, "yes", 8, "trigger"}, [4]any{4, "message", 8, "why"},
		[4]any{1, "ref", 9, "ref"}, [4]any{7, "no", 9, "trigger"}, [4]any{4, "message", 9, "why"},
	)
}

// StepPoll: a polled step. The answer must be a progress number or an object with progress, total and/or description;
// whatever it has is written to the step. A command that fails, or an answer that is not one, writes nothing (the engine
// still notes that the check ran).
func StepPoll(text string) Graph {
	return buildGraph([]nodeSpec{
		{1, "source.step", nil},
		{2, "code", kv{"text": text}},
		{3, "parse", kv{"schema": []kv{{"name": "progress", "type": "int"}, {"name": "total", "type": "int", "min": 1}, {"name": "description", "type": "str"}}}},
		{4, "sink.step_progress", nil},
	},
		[4]any{1, "secrets", 2, "secrets"}, [4]any{2, "stdout", 3, "in"}, [4]any{1, "ref", 4, "ref"},
		[4]any{3, "progress", 4, "progress"}, [4]any{3, "total", 4, "total"}, [4]any{3, "description", 4, "description"},
	)
}

// Implicit builds the graph a plain check_command stands for (ADR-0003, section 9) by name; text is the command. A step or a
// note with a command and no script is read as one of these, so there is one machinery for both.
func Implicit(name, text string) (Graph, bool) {
	switch name {
	case "note_check":
		return NoteCheck(text), true
	case "step_once":
		return StepOnce(text), true
	case "step_poll":
		return StepPoll(text), true
	case "template_emit":
		return TemplateEmit(text), true
	}
	return Graph{}, false
}
