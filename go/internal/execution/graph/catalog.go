package graph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// NodeType describes a kind of node. Ports may depend on the node's parameters (a Parse node has a port per schema field).
type NodeType struct {
	Name  string
	Title string
	Kind  Kind
	// Trusted: the node is our own code and runs on the server. Everything built in is trusted but Code, which is a
	// user's text that only a runner may execute. Letting an admin trust more is later (ADR-0003).
	Trusted bool
	// Network: the node sends a request out. The administrator's policy and the workspace's grant decide whether a graph with it
	// may be saved or run (ADR-0004).
	Network bool
	// Contains: what a custom node holds inside (kind → number of nodes), so the outer graph still sees its source or sink.
	Contains map[Kind]int
	// Description and Fields describe a custom node to an editor.
	Description string
	Fields      []CustomField
	// Ports returns the inputs and outputs for the given parameters, or says they are wrong.
	Ports func(params json.RawMessage) (in, out []Port, err error)
}

// Registry maps a node type name to its description.
type Registry map[string]NodeType

func req(name string, t PortType) Port { return Port{Name: name, Type: t} }
func opt(name string, t PortType) Port { return Port{Name: name, Type: t, Optional: true} }
func grp(name string, t PortType) Port {
	return Port{Name: name, Type: t, Optional: true, Group: "data"}
}

// Builtin is the set of nodes of v1 (ADR-0003, section 5). A fresh map, so a test may extend it.
func Builtin() Registry {
	r := Registry{}
	add := func(t NodeType) { r[t.Name] = t }

	// Sources: the reference to the entity, its secrets and its fields; a run started by an event also has the event.
	source := func(entity string, ref PortType, fields ...Port) {
		out := append([]Port{req("ref", ref), req("secrets", TSecret)}, fields...)
		out = append(out, req("event", TDoc))
		add(NodeType{Name: "source." + entity, Title: "Источник: " + entity, Kind: KindSource, Trusted: true, Ports: sourcePorts(out)})
	}
	source("step", TRefStep, req("title", TStr), req("description", TStr), req("progress", TInt), req("total", TInt), req("done", TBool))
	source("note", TRefNote, req("title", TStr))
	source("template", TRefTemplate, req("title", TStr), req("period", TStr))
	source("quest", TRefQuest, req("title", TStr), req("status", TStr), req("description", TStr))
	source("workspace", TRefWorkspace, req("name", TStr))

	add(NodeType{Name: codeType, Title: "Код (раннер)", Kind: KindCode, Trusted: false, Ports: codePorts})
	add(NodeType{Name: "parse", Title: "Разбор", Kind: KindParse, Trusted: true, Ports: parsePorts})
	add(NodeType{Name: "compare", Title: "Сравнить", Kind: KindControl, Trusted: true, Ports: comparePorts})
	add(NodeType{Name: "any_of", Title: "Любой из", Kind: KindControl, Trusted: true, Ports: anyOfPorts})
	add(NodeType{Name: "all_of", Title: "Каждый из", Kind: KindControl, Trusted: true, Ports: allOfPorts})

	registerEmbed(add)
	add(NodeType{Name: "http.get", Title: "HTTP: запрос", Kind: KindControl, Trusted: true, Network: true, Ports: httpPorts})
	add(NodeType{Name: "json.get", Title: "JSON: достать", Kind: KindControl, Trusted: true, Ports: jsonGetPorts})

	sink := func(name, title string, params func(json.RawMessage) error, in ...Port) {
		ports := func(p json.RawMessage) ([]Port, []Port, error) {
			if params != nil {
				if err := params(p); err != nil {
					return nil, nil, err
				}
			}
			return in, nil, nil
		}
		add(NodeType{Name: name, Title: title, Kind: KindSink, Trusted: true, Ports: ports})
	}
	sink("sink.step_progress", "Шаг: задать прогресс", nil,
		req("ref", TRefStep), grp("progress", TInt), grp("total", TInt), grp("description", TStr))
	sink("sink.step_result", "Шаг: итог запуска", oneOf("result", "done", "not_started", "stopped", "failed"),
		req("ref", TRefStep), req("trigger", TAny), opt("why", TStr))
	sink("sink.note_verdict", "Заметка: вердикт", nil,
		req("ref", TRefNote), req("outdated", TBool), opt("reason", TStr))
	sink("sink.note_error", "Заметка: ошибка проверки", nil,
		req("ref", TRefNote), req("message", TStr))
	sink("sink.template_create", "Шаблон: создать квест", nil,
		req("ref", TRefTemplate), req("doc", TDoc))
	add(NodeType{Name: "sink.template_error", Title: "Шаблон: ошибка попытки", Kind: KindSink, Trusted: true, Ports: templateErrorPorts})
	// A named value of an entity (a note, a step, a quest, a template, the workspace) takes what the script computed. The name
	// and type are fixed in the graph; a value that is a formula is not overwritten.
	add(NodeType{Name: "sink.value", Title: "Переменная: записать", Kind: KindSink, Trusted: true, Ports: valueSinkPorts})
	// Done: the run finished (a hook's Code node already did the work on the runner).
	sink("sink.done", "Готово", nil, req("trigger", TAny))
	// HTTP / Socket: delivery for event hooks (quest=438). Trusted server nodes; body is JSON or text.
	add(NodeType{Name: "sink.http", Title: "HTTP-запрос", Kind: KindSink, Trusted: true, Ports: httpSinkPorts})
	add(NodeType{Name: "sink.socket", Title: "Сокет (unix)", Kind: KindSink, Trusted: true, Ports: socketSinkPorts})
	return r
}

// SourceParams are the parameters of a source node: the id of the entity it reads. Without one the source reads the entity
// the run was triggered for (the script's binding), which is what scripts saved before the parameter still do.
type SourceParams struct {
	ID int64 `json:"id,omitempty"`
}

func sourcePorts(out []Port) func(json.RawMessage) ([]Port, []Port, error) {
	return func(raw json.RawMessage) ([]Port, []Port, error) {
		var p SourceParams
		if err := strictParams(raw, &p); err != nil {
			return nil, nil, err
		}
		if p.ID < 0 {
			return nil, nil, fmt.Errorf("параметр %q: положительное число", "id")
		}
		return nil, out, nil
	}
}

func httpSinkPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p struct {
		URL    string `json:"url"`
		Method string `json:"method"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, nil, fmt.Errorf("параметры: %w", err)
		}
	}
	if strings.TrimSpace(p.URL) == "" {
		return nil, nil, fmt.Errorf("параметр %q: нужен URL", "url")
	}
	m := strings.ToUpper(strings.TrimSpace(p.Method))
	if m == "" {
		m = "POST"
	}
	switch m {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return nil, nil, fmt.Errorf("параметр %q: GET, POST, PUT, PATCH или DELETE", "method")
	}
	return []Port{req("body", TDoc), opt("trigger", TAny)}, nil, nil
}

// ValueSinkParams are the parameters of sink.value.
type ValueSinkParams struct {
	Name string   `json:"name"`
	Type PortType `json:"type"`
}

func valueSinkPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p ValueSinkParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if !embedName.MatchString(p.Name) || p.Name == "expr" || p.Name == "true" || p.Name == "false" {
		return nil, nil, fmt.Errorf("параметр %q: имя значения из строчных латинских букв, цифр и _, до 24 символов", "name")
	}
	if !p.Type.scalar() {
		return nil, nil, fmt.Errorf("параметр %q: int, float, str или bool", "type")
	}
	return []Port{req("ref", TRef), req("value", p.Type)}, nil, nil
}

func socketSinkPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p struct {
		Path string `json:"path"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, nil, fmt.Errorf("параметры: %w", err)
		}
	}
	if strings.TrimSpace(p.Path) == "" {
		return nil, nil, fmt.Errorf("параметр %q: нужен путь сокета", "path")
	}
	return []Port{req("body", TDoc), opt("trigger", TAny)}, nil, nil
}

// templateErrorPorts: kind is which attempt this is (error, timeout, bad JSON) or "closed" (the period ends and nothing
// is retried: the device never came, or somebody cancelled). gate adds a trigger, so the sink runs only on the compare
// branch that picked it; without gate the message alone fires it, which is what a graph saved before gating still does.
func templateErrorPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p struct {
		Kind string `json:"kind"`
		Gate bool   `json:"gate"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, nil, fmt.Errorf("параметры: %w", err)
		}
	}
	switch p.Kind {
	case "error", "timeout", "bad_json", "closed":
	default:
		return nil, nil, fmt.Errorf("параметр %q: одно из error, timeout, bad_json, closed", "kind")
	}
	in := []Port{req("ref", TRefTemplate), req("message", TStr)}
	if p.Gate {
		in = append(in, req("trigger", TAny))
	}
	return in, nil, nil
}

// oneOf checks that a string parameter has one of the listed values.
func oneOf(key string, allowed ...string) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var m map[string]json.RawMessage
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &m); err != nil {
				return fmt.Errorf("параметры: %w", err)
			}
		}
		var v string
		if b, ok := m[key]; ok {
			if err := json.Unmarshal(b, &v); err != nil {
				return fmt.Errorf("параметр %q: нужна строка", key)
			}
		}
		for _, a := range allowed {
			if v == a {
				return nil
			}
		}
		return fmt.Errorf("параметр %q: одно из %v", key, allowed)
	}
}

// Infos lists the node types, in a stable order.
func (r Registry) Infos() []NodeInfo {
	out := make([]NodeInfo, 0, len(r))
	for _, t := range r {
		out = append(out, NodeInfo{Name: t.Name, Title: t.Title, Kind: t.Kind, Trusted: t.Trusted, Network: t.Network, Description: t.Description, Fields: t.Fields})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return kindOrder[out[i].Kind] < kindOrder[out[j].Kind]
		}
		return out[i].Name < out[j].Name
	})
	return out
}

var kindOrder = map[Kind]int{KindSource: 0, KindCode: 1, KindParse: 2, KindControl: 3, KindSink: 4, KindCustom: 5}

// Ports returns the inputs and outputs a node of the type would have with the parameters, or why the parameters are wrong.
func (r Registry) Ports(typ string, params json.RawMessage) (in, out []Port, err error) {
	t, ok := r[typ]
	if !ok {
		return nil, nil, fmt.Errorf("неизвестный тип %q", typ)
	}
	return t.Ports(params)
}
