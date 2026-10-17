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

// NoteCheck: the note is read from its source, the command runs, the answer must be an `outdated` verdict; every way
// the check can go wrong is recorded on the note as an error, and the verdict stays as it was.
func NoteCheck(text string) Graph {
	return buildGraph([]nodeSpec{
		{1, "source.note", nil},
		{2, "code", kv{"text": text}},
		{3, "parse", kv{"schema": []kv{{"name": "outdated", "type": "bool", "required": true}, {"name": "reason", "type": "str"}}}},
		{4, "sink.note_verdict", nil},
		{5, "sink.note_error", nil}, {6, "sink.note_error", nil}, {7, "sink.note_error", nil}, {8, "sink.note_error", nil}, {9, "sink.note_error", nil},
	},
		[4]any{1, "secrets", 2, "secrets"}, [4]any{2, "stdout", 3, "in"},
		[4]any{1, "ref", 4, "ref"}, [4]any{3, "outdated", 4, "outdated"}, [4]any{3, "reason", 4, "reason"},
		[4]any{1, "ref", 5, "ref"}, [4]any{3, "bad_output", 5, "message"},
		[4]any{1, "ref", 6, "ref"}, [4]any{2, "failed", 6, "message"},
		[4]any{1, "ref", 7, "ref"}, [4]any{2, "timeout", 7, "message"},
		[4]any{1, "ref", 8, "ref"}, [4]any{2, "unreachable", 8, "message"},
		[4]any{1, "ref", 9, "ref"}, [4]any{2, "abandoned", 9, "message"},
	)
}

// StepOnce: a one-shot step. A clean exit closes the step; a failure, a timeout or a device that vanished mid-run fails
// the quest with the reason; a device that never showed up puts the step back to wait; a cancel stops it without failing
// the quest.
func StepOnce(text string) Graph {
	return buildGraph([]nodeSpec{
		{1, "source.step", nil},
		{2, "code", kv{"text": text}},
		{3, "sink.step_result", kv{"result": "done"}},
		{4, "sink.step_result", kv{"result": "failed"}},
		{5, "sink.step_result", kv{"result": "failed"}},
		{6, "sink.step_result", kv{"result": "failed"}},
		{7, "sink.step_result", kv{"result": "not_started"}},
		{8, "sink.step_result", kv{"result": "stopped"}},
	},
		[4]any{1, "secrets", 2, "secrets"},
		[4]any{1, "ref", 3, "ref"}, [4]any{2, "stdout", 3, "trigger"},
		[4]any{1, "ref", 4, "ref"}, [4]any{2, "failed", 4, "trigger"}, [4]any{2, "failed", 4, "why"},
		[4]any{1, "ref", 5, "ref"}, [4]any{2, "timeout", 5, "trigger"}, [4]any{2, "timeout", 5, "why"},
		[4]any{1, "ref", 6, "ref"}, [4]any{2, "abandoned", 6, "trigger"}, [4]any{2, "abandoned", 6, "why"},
		[4]any{1, "ref", 7, "ref"}, [4]any{2, "unreachable", 7, "trigger"}, [4]any{2, "unreachable", 7, "why"},
		[4]any{1, "ref", 8, "ref"}, [4]any{2, "canceled", 8, "trigger"}, [4]any{2, "canceled", 8, "why"},
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
	}
	return Graph{}, false
}
