package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

// This file is the evaluator of ADR-0003: it walks a validated graph, passing tokens along the links. A token is a value or
// an exit signal; a node fires when its required inputs hold values, and passes an exit signal on when one of them holds an
// exit signal. The evaluator runs everything that is ours (sources, parse, compare, sinks) and *suspends* at a Code node,
// handing out what the runner needs; the outcome of the job resumes it. It touches no database: the host supplies the
// sources and the sinks, and the run keeps its state in a Snapshot the host stores.

// Ref names an entity a source points at; Key is what else identifies it (a template's period).
type Ref struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id"`
	Key  string `json:"key,omitempty"`
}

// Secrets is the opaque token of a source's secrets: the evaluator only notes that a Code node takes them, the host puts
// the values into the job's environment when the runner claims it. It names the entity the secrets are those of, so a
// source with an id of its own hands over the secrets of that entity.
type Secrets struct {
	Of Ref `json:"of"`
}

// Token is what a link carries.
type Token struct {
	Exit  bool
	Type  PortType
	Value any
}

func exitToken() Token { return Token{Exit: true} }

// MaxEnvBytes caps the values that travel into a Code node's environment.
const MaxEnvBytes = 32 << 10

// CodeRequest is a Code node that is ready to run: the host queues a job from it.
type CodeRequest struct {
	Node     int
	Text     string
	Language string
	// Env is the values that came over links, by variable name (QUESTS_IN_…).
	Env map[string]string
	// Secrets: the node takes the secrets of the source; the host resolves them when the job is claimed.
	Secrets bool
	// SecretsOf is the entity whose secrets those are (the source's ref); zero when the source did not name it.
	SecretsOf Ref
}

// CodeOutcome is how a Code node's job ended: the port it leaves by and its text (stdout for `stdout`, the reason for
// the others).
type CodeOutcome struct {
	Port string `json:"port"`
	Text string `json:"text"`
}

// Host supplies what the evaluator cannot: the entities behind sources and the effects of sinks.
type Host interface {
	// Source returns the values of a source node's output ports; a port left out (or nil) carries an exit signal.
	Source(ctx context.Context, n Node) (map[string]any, error)
	// Sink performs a sink node's effect with the tokens of its inputs (an unwired or exit optional input is absent).
	Sink(ctx context.Context, n Node, in map[string]Token) error
}

// NodeState is where a node stands in a run.
type NodeState string

const (
	StateRan     NodeState = "ran"
	StateSkipped NodeState = "skipped" // an exit signal on a required input
	StateFailed  NodeState = "failed"
	StateWaiting NodeState = "waiting" // a Code node whose job is out
)

func (s NodeState) settled() bool { return s == StateRan || s == StateSkipped || s == StateFailed }

// Run is one pass of a graph over its tokens.
type Run struct {
	nodes    map[int]*resolved
	order    []int
	feeds    map[int]map[string][]Edge
	states   map[int]NodeState
	failures map[int]string
	tokens   map[End]Token
	outcomes map[int]CodeOutcome
}

// NewRun prepares a run of the graph; a graph that does not validate cannot run.
func NewRun(g Graph, reg Registry) (*Run, error) {
	if err := g.Check(reg); err != nil {
		return nil, err
	}
	for _, n := range g.Nodes {
		if reg[n.Type].Kind == KindCustom {
			return nil, fmt.Errorf("нода %d: пользовательские ноды разворачиваются до запуска (Expand)", n.ID)
		}
	}
	r := &Run{
		nodes: map[int]*resolved{}, feeds: map[int]map[string][]Edge{},
		states: map[int]NodeState{}, failures: map[int]string{}, tokens: map[End]Token{}, outcomes: map[int]CodeOutcome{},
	}
	indeg := map[int]int{}
	for _, n := range g.Nodes {
		t := reg[n.Type]
		in, out, _ := t.Ports(n.Params)
		rs := &resolved{node: n, typ: t, in: map[string]Port{}, out: map[string]Port{}, inList: in, outList: out}
		for _, p := range in {
			rs.in[p.Name] = p
		}
		for _, p := range out {
			rs.out[p.Name] = p
		}
		r.nodes[n.ID] = rs
		indeg[n.ID] = 0
	}
	succ := map[int][]int{}
	for _, e := range g.Edges {
		if r.feeds[e.To.Node] == nil {
			r.feeds[e.To.Node] = map[string][]Edge{}
		}
		r.feeds[e.To.Node][e.To.Port] = append(r.feeds[e.To.Node][e.To.Port], e)
		succ[e.From.Node] = append(succ[e.From.Node], e.To.Node)
		indeg[e.To.Node]++
	}
	ids := make([]int, 0, len(r.nodes))
	for id := range r.nodes {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	var queue []int
	for _, id := range ids {
		if indeg[id] == 0 {
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		r.order = append(r.order, id)
		for _, s := range succ[id] {
			if indeg[s]--; indeg[s] == 0 {
				queue = append(queue, s)
			}
		}
	}
	return r, nil
}

// Finished reports whether every node has settled.
func (r *Run) Finished() bool {
	for _, id := range r.order {
		if !r.states[id].settled() {
			return false
		}
	}
	return true
}

// State is the state of a node ("" = not reached yet).
func (r *Run) State(node int) NodeState { return r.states[node] }

// Failure is why a node failed.
func (r *Run) Failure(node int) string { return r.failures[node] }

// Waiting lists the Code nodes whose jobs are out.
func (r *Run) Waiting() []int {
	var out []int
	for _, id := range r.order {
		if r.states[id] == StateWaiting {
			out = append(out, id)
		}
	}
	return out
}

// Advance runs every node that can run and returns the Code nodes that became ready. A sink or a source that fails is
// recorded on its node and does not stop the rest; the errors come back joined.
func (r *Run) Advance(ctx context.Context, h Host) ([]CodeRequest, error) {
	var reqs []CodeRequest
	var errs []error
	for _, id := range r.order {
		st := r.states[id]
		if st.settled() {
			continue
		}
		rs := r.nodes[id]
		if st == StateWaiting {
			if o, ok := r.outcomes[id]; ok {
				r.finishCode(rs, o)
			}
			continue
		}
		ins, blocked := r.gather(rs)
		if blocked {
			continue
		}
		fire, reason := r.fires(rs, ins)
		if !fire {
			r.settle(rs, StateSkipped, reason)
			continue
		}
		req, err := r.run(ctx, h, rs, ins)
		if err != nil {
			r.settle(rs, StateFailed, err.Error())
			errs = append(errs, fmt.Errorf("нода %d: %w", id, err))
			continue
		}
		if req != nil {
			r.states[id] = StateWaiting
			reqs = append(reqs, *req)
		}
	}
	return reqs, errors.Join(errs...)
}

// Resume delivers the outcome of a Code node's job and runs on.
func (r *Run) Resume(ctx context.Context, h Host, node int, o CodeOutcome) ([]CodeRequest, error) {
	if r.states[node] != StateWaiting {
		return nil, fmt.Errorf("нода %d не ждёт результата", node)
	}
	if _, ok := r.nodes[node].out[o.Port]; !ok {
		return nil, fmt.Errorf("у ноды кода нет выхода %q", o.Port)
	}
	r.outcomes[node] = o
	return r.Advance(ctx, h)
}

func (r *Run) finishCode(rs *resolved, o CodeOutcome) {
	for _, p := range rs.outList {
		if p.Name == o.Port {
			var v any = o.Text
			if p.Type == TDoc {
				v = json.RawMessage(o.Text)
			}
			r.tokens[End{rs.node.ID, p.Name}] = Token{Type: p.Type, Value: v}
		} else {
			r.tokens[End{rs.node.ID, p.Name}] = exitToken()
		}
	}
	r.states[rs.node.ID] = StateRan
}

// settle ends a node without outputs of its own: every output carries an exit signal.
func (r *Run) settle(rs *resolved, st NodeState, why string) {
	r.states[rs.node.ID] = st
	if st == StateFailed {
		r.failures[rs.node.ID] = why
	}
	for _, p := range rs.outList {
		r.tokens[End{rs.node.ID, p.Name}] = exitToken()
	}
}

// gather collects the tokens of every input, in link order, converted to the input's type. It is blocked while a node it
// reads from has not settled.
func (r *Run) gather(rs *resolved) (map[string][]Token, bool) {
	ins := map[string][]Token{}
	for _, p := range rs.inList {
		for _, e := range r.feeds[rs.node.ID][p.Name] {
			if !r.states[e.From.Node].settled() {
				return nil, true
			}
			tok, ok := r.tokens[e.From]
			if !ok {
				tok = exitToken()
			}
			ins[p.Name] = append(ins[p.Name], coerce(tok, p.Type))
		}
	}
	return ins, false
}

// fires applies the firing rule of ADR-0003 to the gathered inputs.
func (r *Run) fires(rs *resolved, ins map[string][]Token) (bool, string) {
	groups := map[string]bool{}
	seenGroup := map[string]bool{}
	for _, p := range rs.inList {
		toks := ins[p.Name]
		switch {
		case p.Multi:
			vals := 0
			for i, t := range toks {
				if !t.Exit {
					vals++
				} else if rs.typ.Name == codeType && r.feeds[rs.node.ID][p.Name][i].Optional {
					vals++ // an optional value that is absent does not hold the node back
				}
			}
			switch rs.typ.Name {
			case "any_of":
				if vals == 0 {
					return false, "ни на одном входе нет значения"
				}
			case "all_of", codeType:
				if vals < len(toks) {
					return false, "на входе сигнал выхода"
				}
			}
		case p.Group != "":
			seenGroup[p.Group] = true
			if len(toks) == 1 && !toks[0].Exit {
				groups[p.Group] = true
			}
		case !p.Optional:
			if len(toks) == 0 || toks[0].Exit {
				return false, fmt.Sprintf("на обязательном входе %q сигнал выхода", p.Name)
			}
		}
	}
	for g := range seenGroup {
		if !groups[g] {
			return false, "ни на одном входе группы нет значения"
		}
	}
	return true, ""
}

// run executes a node whose inputs allow it; a Code node answers with its request instead of outputs.
func (r *Run) run(ctx context.Context, h Host, rs *resolved, ins map[string][]Token) (*CodeRequest, error) {
	id := rs.node.ID
	set := func(out map[string]Token) {
		for _, p := range rs.outList {
			if t, ok := out[p.Name]; ok {
				r.tokens[End{id, p.Name}] = t
			} else {
				r.tokens[End{id, p.Name}] = exitToken()
			}
		}
		r.states[id] = StateRan
	}
	single := func() map[string]Token {
		m := map[string]Token{}
		for name, toks := range ins {
			if len(toks) == 1 && !toks[0].Exit {
				m[name] = toks[0]
			}
		}
		return m
	}
	switch rs.typ.Kind {
	case KindSource:
		vals, err := h.Source(ctx, rs.node)
		if err != nil {
			return nil, err
		}
		out := map[string]Token{}
		for _, p := range rs.outList {
			v, ok := vals[p.Name]
			if !ok || v == nil {
				continue
			}
			fitted, err := fit(v, p.Type)
			if err != nil {
				return nil, fmt.Errorf("выход %q: %w", p.Name, err)
			}
			out[p.Name] = Token{Type: p.Type, Value: fitted}
		}
		set(out)
	case KindCode:
		return r.codeRequest(rs, ins)
	case KindParse:
		out, err := runParse(rs, ins["in"][0])
		if err != nil {
			return nil, err
		}
		set(out)
	case KindControl:
		var out map[string]Token
		var err error
		switch rs.typ.Name {
		case "compare":
			out, err = runCompare(rs, ins["in"][0])
		case "any_of":
			for _, t := range ins["in"] {
				if !t.Exit {
					out = map[string]Token{"out": t}
					break
				}
			}
		case "all_of":
			out = map[string]Token{"out": {Type: TAny, Value: true}}
		}
		if err != nil {
			return nil, err
		}
		set(out)
	case KindSink:
		if err := h.Sink(ctx, rs.node, single()); err != nil {
			return nil, err
		}
		set(nil)
	}
	return nil, nil
}

// codeRequest builds what the runner needs from the values that arrived over links.
func (r *Run) codeRequest(rs *resolved, ins map[string][]Token) (*CodeRequest, error) {
	var p codeParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	req := &CodeRequest{Node: rs.node.ID, Text: p.Text, Language: p.Language, Env: map[string]string{}}
	size := 0
	for name, val := range p.Vars {
		req.Env[name] = val // fixed variables first: a link into the same name cannot be there (QUESTS_IN_ is not a valid alias)
		size += len(name) + len(val)
	}
	for i, tok := range ins["in"] {
		if tok.Exit {
			continue // an optional value that did not come
		}
		e := r.feeds[rs.node.ID]["in"][i]
		name, val := EnvName(e), envString(tok)
		size += len(name) + len(val)
		req.Env[name] = val
	}
	if size > MaxEnvBytes {
		return nil, fmt.Errorf("значения для кода больше %d КБ", MaxEnvBytes>>10)
	}
	if toks := ins["secrets"]; len(toks) == 1 && !toks[0].Exit {
		req.Secrets = true
		if sec, ok := toks[0].Value.(Secrets); ok {
			req.SecretsOf = sec.Of
		}
	}
	return req, nil
}

// envString is the text of a value in a process environment.
func envString(t Token) string {
	switch v := t.Value.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case json.RawMessage:
		return string(v)
	}
	return fmt.Sprint(t.Value)
}
