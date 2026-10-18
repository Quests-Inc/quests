package graph

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Problem is one thing wrong with a graph; Node is 0 for the graph as a whole.
type Problem struct {
	Node    int    `json:"node,omitempty"`
	Message string `json:"message"`
}

func (p Problem) Error() string {
	if p.Node == 0 {
		return p.Message
	}
	return fmt.Sprintf("нода %d: %s", p.Node, p.Message)
}

// Check validates the graph against the registry and joins the problems into one error.
func (g Graph) Check(reg Registry) error {
	ps := g.Validate(reg)
	if len(ps) == 0 {
		return nil
	}
	errs := make([]error, len(ps))
	for i, p := range ps {
		errs[i] = p
	}
	return errors.Join(errs...)
}

type resolved struct {
	node    Node
	typ     NodeType
	in      map[string]Port
	out     map[string]Port
	inList  []Port // in the order the node type declares them
	outList []Port
}

// Validate finds every problem it can: unknown types and wrong parameters, links between ports that do not exist or do not
// fit, inputs fed twice, required inputs left open, cycles, a graph with no source or no sink.
func (g Graph) Validate(reg Registry) []Problem { return g.validate(reg, false) }

// validate checks the graph; a fragment (the inside of a custom node) may leave inputs open, since the outer graph wires
// them, and needs neither a source nor a sink of its own.
func (g Graph) validate(reg Registry, fragment bool) []Problem {
	var ps []Problem
	add := func(node int, format string, a ...any) {
		ps = append(ps, Problem{Node: node, Message: fmt.Sprintf(format, a...)})
	}

	if len(g.Nodes) == 0 {
		return []Problem{{Message: "граф пуст"}}
	}
	if len(g.Nodes) > MaxNodes || len(g.Edges) > MaxEdges {
		return []Problem{{Message: fmt.Sprintf("не больше %d нод и %d связей", MaxNodes, MaxEdges)}}
	}

	nodes := map[int]*resolved{}
	kinds := map[Kind]int{}
	for _, n := range g.Nodes {
		if n.ID <= 0 {
			add(0, "номер ноды должен быть положительным (получен %d)", n.ID)
			continue
		}
		if nodes[n.ID] != nil {
			add(n.ID, "номер повторяется")
			continue
		}
		t, ok := reg[n.Type]
		if !ok {
			add(n.ID, "неизвестный тип %q", n.Type)
			nodes[n.ID] = nil
			continue
		}
		in, out, err := t.Ports(n.Params)
		if err != nil {
			add(n.ID, "%s: %v", t.Title, err)
			nodes[n.ID] = nil
			continue
		}
		r := &resolved{node: n, typ: t, in: map[string]Port{}, out: map[string]Port{}}
		for _, p := range in {
			r.in[p.Name] = p
		}
		for _, p := range out {
			r.out[p.Name] = p
		}
		nodes[n.ID] = r
		kinds[t.Kind]++
		for k, c := range t.Contains {
			kinds[k] += c
		}
	}

	// Links.
	feeds := map[int]map[string][]Edge{} // node → input → edges
	for _, e := range g.Edges {
		from, to := nodes[e.From.Node], nodes[e.To.Node]
		if _, ok := nodes[e.From.Node]; !ok {
			add(0, "связь начинается в несуществующей ноде %d", e.From.Node)
			continue
		}
		if _, ok := nodes[e.To.Node]; !ok {
			add(0, "связь кончается в несуществующей ноде %d", e.To.Node)
			continue
		}
		if from == nil || to == nil {
			continue // that node's own problem is already reported
		}
		op, ok := from.out[e.From.Port]
		if !ok {
			add(e.From.Node, "нет выхода %q", e.From.Port)
			continue
		}
		ip, ok := to.in[e.To.Port]
		if !ok {
			add(e.To.Node, "нет входа %q", e.To.Port)
			continue
		}
		if !Assignable(op.Type, ip.Type) {
			add(e.To.Node, "вход %q (%s) не принимает %s из ноды %d, выход %q", ip.Name, ip.Type, op.Type, e.From.Node, op.Name)
			continue
		}
		if op.Type == TSecret && to.typ.Name != codeType && to.typ.Kind != KindCustom {
			add(e.To.Node, "секреты принимает только нода кода")
			continue
		}
		if to.typ.Name == codeType && ip.Name == "in" && (e.Alias != "" && !aliasRe.MatchString(e.Alias)) {
			add(e.To.Node, "псевдоним %q: заглавные латинские буквы, цифры и _", e.Alias)
			continue
		}
		if (e.Alias != "" || e.Optional) && (to.typ.Name != codeType || ip.Name != "in") {
			add(e.To.Node, "псевдоним и необязательность задаются только у значений, идущих в код")
			continue
		}
		if feeds[e.To.Node] == nil {
			feeds[e.To.Node] = map[string][]Edge{}
		}
		feeds[e.To.Node][e.To.Port] = append(feeds[e.To.Node][e.To.Port], e)
	}

	ids := make([]int, 0, len(nodes))
	for id, r := range nodes {
		if r != nil {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)

	// Inputs: one link each unless the input takes many; required ones wired; a group wired at least once.
	for _, id := range ids {
		r := nodes[id]
		groups := map[string]bool{}
		for _, p := range sortedPorts(r.in) {
			es := feeds[id][p.Name]
			switch {
			case len(es) > 1 && !p.Multi:
				add(id, "вход %q принимает одну связь, пришло %d", p.Name, len(es))
			case len(es) == 0 && !p.Optional && !fragment:
				add(id, "не подключён обязательный вход %q", p.Name)
			}
			if p.Group != "" && len(es) > 0 {
				groups[p.Group] = true
			}
		}
		for _, p := range r.in {
			if p.Group != "" && !groups[p.Group] && !fragment {
				add(id, "нужно подключить хотя бы один из входов группы (%s)", groupNames(r.in, p.Group))
				break
			}
		}
		switch r.typ.Name {
		case "any_of", "all_of":
			if len(feeds[id]["in"]) < 2 && !fragment {
				add(id, "%s: нужно не меньше двух входов", r.typ.Title)
			}
		case codeType:
			names := map[string]bool{}
			for _, e := range feeds[id]["in"] {
				if n := EnvName(e); names[n] {
					add(id, "переменная %s задана дважды", n)
				} else {
					names[n] = true
				}
			}
		}
	}

	if kinds[KindSource] == 0 && !fragment {
		add(0, "нет ни одного источника")
	}
	if kinds[KindSink] == 0 && !fragment {
		add(0, "нет ни одного приёмника")
	}

	// Cycles. Code nodes may follow one another: each waits for its own job and passes its output on.
	preds := map[int][]int{}
	succ := map[int][]int{}
	indeg := map[int]int{}
	for _, e := range g.Edges {
		if nodes[e.From.Node] == nil || nodes[e.To.Node] == nil {
			continue
		}
		preds[e.To.Node] = append(preds[e.To.Node], e.From.Node)
		succ[e.From.Node] = append(succ[e.From.Node], e.To.Node)
		indeg[e.To.Node]++
	}
	var queue, order []int
	for _, id := range ids {
		if indeg[id] == 0 {
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)
		for _, s := range succ[id] {
			if indeg[s]--; indeg[s] == 0 {
				queue = append(queue, s)
			}
		}
	}
	if len(order) < len(ids) {
		add(0, "в графе есть цикл")
		return ps
	}
	return ps
}

func sortedPorts(m map[string]Port) []Port {
	out := make([]Port, 0, len(m))
	for _, p := range m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func groupNames(m map[string]Port, group string) string {
	var names []string
	for _, p := range sortedPorts(m) {
		if p.Group == group {
			names = append(names, p.Name)
		}
	}
	return strings.Join(names, ", ")
}
