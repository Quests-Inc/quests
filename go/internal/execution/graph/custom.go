package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Custom nodes (quest=458): a node the workspace's owner makes, either baked from other nodes or written as one Code node.
// It is a *definition* (a graph fragment, which of its inner ports are the node's inputs and outputs, which inner parameters
// are editable fields) and nothing the evaluator knows: before a run the graph is expanded, every custom node replaced by
// its inner nodes, so what runs is plain nodes.

// CustomPrefix starts the type name of a custom node: `custom.<id>`.
const CustomPrefix = "custom."

const (
	// innerBase renumbers inner nodes: node N of the outer graph holds inner node M as N*innerBase+M. Inner numbers are
	// below it; a custom node inside a custom node is expanded one level at a time, so numbers stay unique.
	innerBase = 10000
	// MaxCustomDepth is how deep custom nodes may hold custom nodes.
	MaxCustomDepth = 3
	// MaxCustomFields caps the editable fields of one custom node.
	MaxCustomFields = 40
)

var customNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// CustomPort names one input or output of a custom node and the inner port it is.
type CustomPort struct {
	Name string `json:"name"`
	Node int    `json:"node"`
	Port string `json:"port"`
}

// CustomField is a parameter of an inner node the user edits on the custom node itself. Param is a top-level key of the
// inner node's parameters, or `vars.NAME` for a fixed variable of a Code node. Default is the baked value.
type CustomField struct {
	Name    string `json:"name"`
	Label   string `json:"label,omitempty"`
	Type    string `json:"type"` // str, int, bool
	Default any    `json:"default,omitempty"`
	Node    int    `json:"node"`
	Param   string `json:"param"`
}

// Custom is the stored definition of a custom node.
type Custom struct {
	ID          int64         `json:"id"`
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	Version     int           `json:"version,omitempty"`
	Inputs      []CustomPort  `json:"inputs"`
	Outputs     []CustomPort  `json:"outputs"`
	Fields      []CustomField `json:"fields"`
	Graph       Graph         `json:"graph"`
}

// TypeName is the node type of the custom node in a graph.
func (c Custom) TypeName() string { return CustomPrefix + strconv.FormatInt(c.ID, 10) }

// CustomID reads the id out of a node type name.
func CustomID(typ string) (int64, bool) {
	s, ok := strings.CutPrefix(typ, CustomPrefix)
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil && id > 0
}

// inner is the definition resolved against a registry: the ports of the custom node and what its inside holds.
type inner struct {
	in, out  []Port
	contains map[Kind]int
	fields   map[string]CustomField
}

// resolve checks the definition against the registry (which holds every custom node it may use) and works out the ports.
func (c Custom) resolve(reg Registry) (*inner, error) {
	if t := strings.TrimSpace(c.Title); t == "" || len([]rune(t)) > 200 {
		return nil, errors.New("название ноды: от 1 до 200 знаков")
	}
	if len(c.Graph.Nodes) == 0 {
		return nil, errors.New("внутри нет нод")
	}
	if len(c.Fields) > MaxCustomFields {
		return nil, fmt.Errorf("полей не больше %d", MaxCustomFields)
	}
	for _, n := range c.Graph.Nodes {
		if n.ID >= innerBase {
			return nil, fmt.Errorf("номер внутренней ноды меньше %d", innerBase)
		}
		if id, ok := CustomID(n.Type); ok && id == c.ID {
			return nil, errors.New("нода не может содержать саму себя")
		}
	}
	if ps := c.Graph.validate(reg, true); len(ps) > 0 {
		return nil, ps[0]
	}
	types := map[int]NodeType{}
	ports := map[int][2]map[string]Port{} // node → [inputs, outputs]
	contains := map[Kind]int{}
	for _, n := range c.Graph.Nodes {
		t := reg[n.Type]
		in, out, err := t.Ports(n.Params)
		if err != nil {
			return nil, fmt.Errorf("нода %d: %v", n.ID, err)
		}
		types[n.ID] = t
		pi, po := map[string]Port{}, map[string]Port{}
		for _, p := range in {
			pi[p.Name] = p
		}
		for _, p := range out {
			po[p.Name] = p
		}
		ports[n.ID] = [2]map[string]Port{pi, po}
		contains[t.Kind]++
		for k, v := range t.Contains {
			contains[k] += v
		}
	}
	res := &inner{contains: contains, fields: map[string]CustomField{}}
	fed := map[End]bool{}
	for _, e := range c.Graph.Edges {
		fed[e.To] = true
	}
	exposed := map[End]bool{}
	names := map[string]bool{}
	for _, p := range c.Inputs {
		if !customNameRe.MatchString(p.Name) || names["in:"+p.Name] {
			return nil, fmt.Errorf("вход %q: имя из строчных латинских букв, цифр и _, без повторов", p.Name)
		}
		names["in:"+p.Name] = true
		ip, ok := ports[p.Node][0][p.Port]
		if !ok {
			return nil, fmt.Errorf("вход %q: у ноды %d нет входа %q", p.Name, p.Node, p.Port)
		}
		end := End{p.Node, p.Port}
		if fed[end] && !ip.Multi {
			return nil, fmt.Errorf("вход %q: вход ноды %d уже занят внутренней связью", p.Name, p.Node)
		}
		if exposed[end] && !ip.Multi {
			return nil, fmt.Errorf("вход ноды %d, порт %q вынесен дважды", p.Node, p.Port)
		}
		exposed[end] = true
		if ip.Multi && p.Port == "in" && nodeType(c.Graph, p.Node) == codeType {
			ip.Multi = false // each input of a code node is one variable, named after the input (see expandOnce)
		}
		ip.Name = p.Name
		res.in = append(res.in, ip)
	}
	for _, p := range c.Outputs {
		if !customNameRe.MatchString(p.Name) || names["out:"+p.Name] {
			return nil, fmt.Errorf("выход %q: имя из строчных латинских букв, цифр и _, без повторов", p.Name)
		}
		names["out:"+p.Name] = true
		op, ok := ports[p.Node][1][p.Port]
		if !ok {
			return nil, fmt.Errorf("выход %q: у ноды %d нет выхода %q", p.Name, p.Node, p.Port)
		}
		op.Name = p.Name
		res.out = append(res.out, op)
	}
	for _, f := range c.Fields {
		if !customNameRe.MatchString(f.Name) || names["field:"+f.Name] {
			return nil, fmt.Errorf("поле %q: имя из строчных латинских букв, цифр и _, без повторов", f.Name)
		}
		names["field:"+f.Name] = true
		switch f.Type {
		case "str", "int", "bool":
		default:
			return nil, fmt.Errorf("поле %q: тип str, int или bool", f.Name)
		}
		if _, ok := ports[f.Node]; !ok {
			return nil, fmt.Errorf("поле %q: нет ноды %d", f.Name, f.Node)
		}
		if f.Param == "" || strings.Count(f.Param, ".") > 1 || (strings.Contains(f.Param, ".") && !strings.HasPrefix(f.Param, "vars.")) {
			return nil, fmt.Errorf("поле %q: параметр — ключ параметров ноды или vars.ИМЯ", f.Name)
		}
		if f.Default != nil {
			if _, err := fieldValue(f, f.Default); err != nil {
				return nil, err
			}
		}
		res.fields[f.Name] = f
	}
	// An inner input that needs a value must be wired inside or be one of the node's inputs.
	for _, n := range c.Graph.Nodes {
		for _, p := range ports[n.ID][0] {
			end := End{n.ID, p.Name}
			if !p.Optional && !p.Multi && !fed[end] && !exposed[end] {
				return nil, fmt.Errorf("нода %d: обязательный вход %q ни к чему не подключён — вынесите его входом", n.ID, p.Name)
			}
		}
	}
	return res, nil
}

func nodeType(g Graph, id int) string {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n.Type
		}
	}
	return ""
}

// fieldValue converts a JSON value to the field's type.
func fieldValue(f CustomField, v any) (any, error) {
	switch f.Type {
	case "str":
		if s, ok := v.(string); ok {
			return s, nil
		}
	case "int":
		switch x := v.(type) {
		case float64:
			if x == float64(int64(x)) {
				return int64(x), nil
			}
		case int64:
			return x, nil
		case int:
			return int64(x), nil
		case json.Number:
			if i, err := x.Int64(); err == nil {
				return i, nil
			}
		}
	case "bool":
		if b, ok := v.(bool); ok {
			return b, nil
		}
	}
	return nil, fmt.Errorf("поле %q: нужно значение типа %s", f.Name, f.Type)
}

// Check validates a definition against a registry that holds the custom nodes it may use (not itself).
func (c Custom) Check(reg Registry) error {
	_, err := c.resolve(reg)
	return err
}

// WithCustoms is the registry extended with the custom nodes that resolve; the others come back in the error map by id
// (a node whose inside uses one that is gone, for instance), and a graph that uses them is told it is unknown.
func (r Registry) WithCustoms(customs []Custom) (Registry, map[int64]error) {
	out := make(Registry, len(r)+len(customs))
	for k, v := range r {
		out[k] = v
	}
	pending := append([]Custom(nil), customs...)
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })
	errs := map[int64]error{}
	for progress := true; progress && len(pending) > 0; {
		progress = false
		var rest []Custom
		for _, c := range pending {
			res, err := c.resolve(out)
			if err != nil {
				rest = append(rest, c)
				errs[c.ID] = err
				continue
			}
			delete(errs, c.ID)
			progress = true
			out[c.TypeName()] = customType(c, res)
		}
		pending = rest
	}
	return out, errs
}

func customType(c Custom, res *inner) NodeType {
	return NodeType{
		Name:        c.TypeName(),
		Title:       c.Title,
		Kind:        KindCustom,
		Trusted:     res.contains[KindCode] == 0,
		Contains:    res.contains,
		Description: c.Description,
		Fields:      c.Fields,
		Ports: func(raw json.RawMessage) ([]Port, []Port, error) {
			if _, err := customValues(c, res, raw); err != nil {
				return nil, nil, err
			}
			return res.in, res.out, nil
		},
	}
}

// customValues reads the values of a custom node's fields from its parameters: an object {name: value}.
func customValues(c Custom, res *inner, raw json.RawMessage) (map[string]any, error) {
	vals := map[string]any{}
	if len(strings.TrimSpace(string(raw))) == 0 || string(raw) == "null" {
		return vals, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("параметры: %w", err)
	}
	for k, v := range m {
		f, ok := res.fields[k]
		if !ok {
			return nil, fmt.Errorf("у ноды %q нет поля %q", c.Title, k)
		}
		fv, err := fieldValue(f, v)
		if err != nil {
			return nil, err
		}
		vals[k] = fv
	}
	return vals, nil
}

// Expand replaces every custom node of the graph by its inner nodes, as often as nodes inside hold more. The result has
// only built-in nodes, with the fields written into the inner parameters. reg must be the registry WithCustoms made.
func Expand(g Graph, customs []Custom, reg Registry) (Graph, error) {
	byID := map[int64]Custom{}
	for _, c := range customs {
		byID[c.ID] = c
	}
	for depth := 0; ; depth++ {
		has := false
		for _, n := range g.Nodes {
			if _, ok := CustomID(n.Type); ok {
				has = true
				break
			}
		}
		if !has {
			return g, nil
		}
		if depth >= MaxCustomDepth {
			return Graph{}, fmt.Errorf("ноды вложены глубже %d уровней", MaxCustomDepth)
		}
		next, err := expandOnce(g, byID, reg)
		if err != nil {
			return Graph{}, err
		}
		g = next
	}
}

func expandOnce(g Graph, byID map[int64]Custom, reg Registry) (Graph, error) {
	var out Graph
	type site struct {
		c   Custom
		in  map[string][]End  // custom input name → inner ends it feeds
		env map[string]string // custom input name → variable name, when it feeds the values of a Code node
		out map[string]End    // custom output name → inner end it is
	}
	sites := map[int]*site{}
	for _, n := range g.Nodes {
		id, ok := CustomID(n.Type)
		if !ok {
			out.Nodes = append(out.Nodes, n)
			continue
		}
		c, ok := byID[id]
		if !ok {
			return Graph{}, fmt.Errorf("нода %d: нет пользовательской ноды %d", n.ID, id)
		}
		res, err := c.resolve(reg)
		if err != nil {
			return Graph{}, fmt.Errorf("нода %d (%s): %w", n.ID, c.Title, err)
		}
		vals, err := customValues(c, res, n.Params)
		if err != nil {
			return Graph{}, fmt.Errorf("нода %d: %w", n.ID, err)
		}
		s := &site{c: c, in: map[string][]End{}, env: map[string]string{}, out: map[string]End{}}
		sites[n.ID] = s
		base := n.ID * innerBase
		for _, in := range c.Graph.Nodes {
			node := Node{ID: base + in.ID, Type: in.Type, X: n.X, Y: n.Y}
			params := in.Params
			for _, f := range c.Fields {
				if f.Node != in.ID {
					continue
				}
				v, set := vals[f.Name]
				if !set {
					if f.Default == nil {
						continue
					}
					if v, err = fieldValue(f, f.Default); err != nil {
						return Graph{}, err
					}
				}
				if params, err = setParam(params, f.Param, v); err != nil {
					return Graph{}, fmt.Errorf("нода %d, поле %q: %w", n.ID, f.Name, err)
				}
			}
			node.Params = params
			out.Nodes = append(out.Nodes, node)
		}
		for _, e := range c.Graph.Edges {
			e.From.Node += base
			e.To.Node += base
			out.Edges = append(out.Edges, e)
		}
		for _, p := range c.Inputs {
			s.in[p.Name] = append(s.in[p.Name], End{base + p.Node, p.Port})
			if p.Port == "in" && nodeType(c.Graph, p.Node) == codeType {
				s.env[p.Name] = strings.ToUpper(p.Name)
			}
		}
		for _, p := range c.Outputs {
			s.out[p.Name] = End{base + p.Node, p.Port}
		}
	}
	for _, e := range g.Edges {
		if s := sites[e.From.Node]; s != nil {
			end, ok := s.out[e.From.Port]
			if !ok {
				return Graph{}, fmt.Errorf("у ноды %d нет выхода %q", e.From.Node, e.From.Port)
			}
			e.From = end
		}
		if s := sites[e.To.Node]; s != nil {
			ends := s.in[e.To.Port]
			if len(ends) == 0 {
				return Graph{}, fmt.Errorf("у ноды %d нет входа %q", e.To.Node, e.To.Port)
			}
			for _, end := range ends {
				e2 := e
				e2.To = end
				if name, ok := s.env[e.To.Port]; ok && e2.Alias == "" {
					e2.Alias = name
				}
				out.Edges = append(out.Edges, e2)
			}
			continue
		}
		out.Edges = append(out.Edges, e)
	}
	return out, nil
}

// setParam writes a field value into a node's parameters: a top-level key, or `vars.NAME` (always stored as text).
func setParam(raw json.RawMessage, path string, v any) (json.RawMessage, error) {
	m := map[string]any{}
	if len(strings.TrimSpace(string(raw))) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
	}
	if name, ok := strings.CutPrefix(path, "vars."); ok {
		vars, _ := m["vars"].(map[string]any)
		if vars == nil {
			vars = map[string]any{}
		}
		vars[name] = fmt.Sprint(v)
		m["vars"] = vars
	} else {
		m[path] = v
	}
	return json.Marshal(m)
}

// CustomUses lists the custom node ids a graph uses directly.
func (g Graph) CustomUses() []int64 {
	seen := map[int64]bool{}
	var out []int64
	for _, n := range g.Nodes {
		if id, ok := CustomID(n.Type); ok && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
