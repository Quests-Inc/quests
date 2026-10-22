package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
)

// Nodes for a script called from a text (`script=N`, ADR-0003 §6b): Args and Result are its entry and exit, Expr, Random and
// Regex compute. All of them are trusted and run on the server, so a graph made of them needs no runner.

// Limits of one embedded evaluation, so the nodes cannot be used to burn the server.
const (
	MaxRolls      = 1000 // repeats in one Random node or one `sum(n, …)`, and per evaluation: MaxRollsTotal
	MaxRollsTotal = 5000 //
	MaxTextBytes  = 64 << 10
	MaxArgsParams = 16
	maxBoundAbs   = 1_000_000_000
)

var embedName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,23}$`)

func registerEmbed(add func(NodeType)) {
	add(NodeType{Name: "args", Title: "Аргументы", Kind: KindSource, Trusted: true, Ports: argsPorts})
	add(NodeType{Name: "sink.result", Title: "Результат", Kind: KindSink, Trusted: true,
		Ports: func(json.RawMessage) ([]Port, []Port, error) { return []Port{req("value", TStr)}, nil, nil }})
	add(NodeType{Name: "expr", Title: "Выражение", Kind: KindControl, Trusted: true, Ports: exprPorts})
	add(NodeType{Name: "random", Title: "Случайное", Kind: KindControl, Trusted: true, Ports: randomPorts})
	add(NodeType{Name: "regex", Title: "Регулярка", Kind: KindControl, Trusted: true, Ports: regexPorts})
}

// ArgParam is one named argument of an Args node.
type ArgParam struct {
	Name string   `json:"name"`
	Type PortType `json:"type"`
}

func argsPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p struct {
		Params []ArgParam `json:"params"`
	}
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if len(p.Params) == 0 || len(p.Params) > MaxArgsParams {
		return nil, nil, fmt.Errorf("аргументов от 1 до %d", MaxArgsParams)
	}
	seen := map[string]bool{}
	out := make([]Port, 0, len(p.Params))
	for _, a := range p.Params {
		if !embedName.MatchString(a.Name) || seen[a.Name] {
			return nil, nil, fmt.Errorf("аргумент %q: имя [a-z][a-z0-9_]*, без повторов", a.Name)
		}
		if !a.Type.scalar() {
			return nil, nil, fmt.Errorf("аргумент %q: тип str, int, float или bool", a.Name)
		}
		seen[a.Name] = true
		out = append(out, req(a.Name, a.Type))
	}
	return nil, out, nil
}

// ---- Expr

type exprParams struct {
	Expr      string     `json:"expr"`
	Vars      []string   `json:"vars"`
	Operators []Operator `json:"operators,omitempty"`
}

func exprPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p exprParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if len(p.Vars) > 8 {
		return nil, nil, errors.New("не больше 8 переменных")
	}
	var in []Port
	if strings.TrimSpace(p.Expr) == "" {
		in = append(in, req("expr", TStr)) // the expression arrives as a value
	} else if len(p.Expr) > MaxExprLen {
		return nil, nil, fmt.Errorf("выражение длиннее %d символов", MaxExprLen)
	}
	seen := map[string]bool{"expr": true}
	if _, err := compileOperators(p.Operators); err != nil {
		return nil, nil, err
	}
	for _, v := range p.Vars {
		if !embedName.MatchString(v) || seen[v] {
			return nil, nil, fmt.Errorf("переменная %q: имя [a-z][a-z0-9_]*, без повторов, не expr", v)
		}
		seen[v] = true
		in = append(in, req(v, TFloat))
	}
	return in, []Port{req("value", TFloat), req("text", TStr)}, nil
}

func runExpr(rs *resolved, ins map[string][]Token, b *budget, scope *ExprScope) (map[string]Token, error) {
	var p exprParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	src := p.Expr
	if strings.TrimSpace(src) == "" {
		src, _ = ins["expr"][0].Value.(string)
	}
	src, err := rewriteForeign(src, scope)
	if err != nil {
		return nil, err
	}
	vars := map[string]float64{}
	for _, name := range p.Vars {
		if t := ins[name]; len(t) == 1 && !t[0].Exit {
			switch n := t[0].Value.(type) {
			case float64:
				vars[name] = n
			case int64:
				vars[name] = float64(n)
			}
		}
	}
	strs := map[string]bool{}
	if scope != nil {
		for name, n := range scope.Num {
			if _, set := vars[name]; !set {
				vars[name] = n
			}
		}
		for name := range scope.Str {
			if _, set := vars[name]; !set {
				strs[name] = true
			}
		}
	}
	ops, err := compileOperators(p.Operators)
	if err != nil {
		return nil, err
	}
	v, err := evalExprWith(src, vars, ops, b, strs)
	if err != nil {
		return nil, err
	}
	return map[string]Token{"value": {Type: TFloat, Value: v}, "text": {Type: TStr, Value: FormatNumber(v)}}, nil
}

var foreignValue = regexp.MustCompile(`\b(note|quest|questline|step|template)=(\d+)\.([a-z][a-z0-9_]{0,23})\b`)

// rewriteForeign turns note=12.str into the number that note's formulas would see. Eight lookups per expression.
func rewriteForeign(src string, scope *ExprScope) (string, error) {
	if scope == nil || scope.Foreign == nil || !foreignValue.MatchString(src) {
		return src, nil
	}
	n := 0
	var fail error
	out := foreignValue.ReplaceAllStringFunc(src, func(tok string) string {
		if fail != nil {
			return tok
		}
		n++
		if n > 8 {
			fail = errors.New("слишком много чужих значений (больше 8)")
			return tok
		}
		m := foreignValue.FindStringSubmatch(tok)
		id, _ := strconv.ParseInt(m[2], 10, 64)
		num, isStr, ok, err := scope.Foreign(m[1], id, m[3])
		if err != nil {
			fail = err
			return tok
		}
		if !ok {
			fail = fmt.Errorf("нет значения %s", tok)
			return tok
		}
		if isStr {
			fail = fmt.Errorf("значение %q — строка, в выражение не подставляется", m[3])
			return tok
		}
		return FormatNumber(num)
	})
	if fail != nil {
		return "", fail
	}
	return out, nil
}

// ---- Random

type randomParams struct {
	Kind      string   `json:"kind"` // int (default), float, pick
	Lo        *float64 `json:"lo,omitempty"`
	Hi        *float64 `json:"hi,omitempty"`
	Count     int      `json:"count,omitempty"`
	Aggregate string   `json:"aggregate,omitempty"` // sum (default), min, max, list; pick: first (default), list
}

func (p randomParams) norm() randomParams {
	if p.Kind == "" {
		p.Kind = "int"
	}
	if p.Count == 0 {
		p.Count = 1
	}
	if p.Aggregate == "" {
		p.Aggregate = "sum"
		if p.Kind == "pick" {
			p.Aggregate = "first"
		}
	}
	return p
}

func randomPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p randomParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	p = p.norm()
	if p.Count < 1 || p.Count > MaxRolls {
		return nil, nil, fmt.Errorf("count от 1 до %d", MaxRolls)
	}
	num := TInt
	switch p.Kind {
	case "int":
	case "float":
		num = TFloat
	case "pick":
	default:
		return nil, nil, errors.New("kind: int, float или pick")
	}
	switch p.Aggregate {
	case "list":
	case "first":
		if p.Kind != "pick" {
			return nil, nil, errors.New("aggregate first только у pick")
		}
	case "sum", "min", "max":
		if p.Kind == "pick" {
			return nil, nil, errors.New("у pick aggregate: first или list")
		}
	default:
		return nil, nil, errors.New("aggregate: sum, min, max или list (у pick first, list)")
	}
	if p.Kind == "pick" {
		return []Port{req("list", TDoc), opt("count", TInt)}, []Port{req("value", TDoc)}, nil
	}
	for _, b := range []*float64{p.Lo, p.Hi} {
		if b != nil && (*b > maxBoundAbs || *b < -maxBoundAbs) {
			return nil, nil, fmt.Errorf("границы не больше %d по модулю", maxBoundAbs)
		}
	}
	out := num
	if p.Aggregate == "list" {
		out = TDoc
	}
	return []Port{opt("lo", num), opt("hi", num), opt("count", TInt)}, []Port{req("value", out)}, nil
}

func runRandom(rs *resolved, ins map[string][]Token, b *budget) (map[string]Token, error) {
	var p randomParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	p = p.norm()
	count := p.Count
	if t := ins["count"]; len(t) == 1 && !t[0].Exit {
		count = int(t[0].Value.(int64))
	}
	if count < 1 || count > MaxRolls {
		return nil, fmt.Errorf("count от 1 до %d (получено %d)", MaxRolls, count)
	}
	if b.rolls += count; b.rolls > MaxRollsTotal {
		return nil, fmt.Errorf("слишком много бросков за раз (больше %d)", MaxRollsTotal)
	}
	if p.Kind == "pick" {
		var list []json.RawMessage
		raw, _ := ins["list"][0].Value.(json.RawMessage)
		if err := json.Unmarshal(raw, &list); err != nil || len(list) == 0 {
			return nil, errors.New("pick: нужен непустой JSON-массив")
		}
		picked := make([]json.RawMessage, count)
		for i := range picked {
			picked[i] = list[rand.IntN(len(list))]
		}
		if p.Aggregate == "first" || count == 1 && p.Aggregate != "list" {
			return map[string]Token{"value": {Type: TDoc, Value: picked[0]}}, nil
		}
		out, _ := json.Marshal(picked)
		return map[string]Token{"value": {Type: TDoc, Value: json.RawMessage(out)}}, nil
	}
	bound := func(name string, static *float64) (float64, error) {
		if t := ins[name]; len(t) == 1 && !t[0].Exit {
			switch v := t[0].Value.(type) {
			case int64:
				return float64(v), nil
			case float64:
				return v, nil
			}
		}
		if static != nil {
			return *static, nil
		}
		return 0, fmt.Errorf("не задана граница %q (параметр или вход)", name)
	}
	lo, err := bound("lo", p.Lo)
	if err != nil {
		return nil, err
	}
	hi, err := bound("hi", p.Hi)
	if err != nil {
		return nil, err
	}
	if hi < lo || lo < -maxBoundAbs || hi > maxBoundAbs {
		return nil, fmt.Errorf("диапазон %v..%v неверен", lo, hi)
	}
	vals := make([]float64, count)
	for i := range vals {
		if p.Kind == "int" {
			vals[i] = float64(int64(lo) + rand.Int64N(int64(hi)-int64(lo)+1))
		} else {
			vals[i] = lo + rand.Float64()*(hi-lo)
		}
	}
	if p.Aggregate == "list" {
		out, _ := json.Marshal(vals)
		if p.Kind == "int" {
			ints := make([]int64, len(vals))
			for i, v := range vals {
				ints[i] = int64(v)
			}
			out, _ = json.Marshal(ints)
		}
		return map[string]Token{"value": {Type: TDoc, Value: json.RawMessage(out)}}, nil
	}
	acc := vals[0]
	for _, v := range vals[1:] {
		switch p.Aggregate {
		case "sum":
			acc += v
		case "min":
			acc = min(acc, v)
		case "max":
			acc = max(acc, v)
		}
	}
	if p.Kind == "int" {
		return map[string]Token{"value": {Type: TInt, Value: int64(acc)}}, nil
	}
	return map[string]Token{"value": {Type: TFloat, Value: acc}}, nil
}

// ---- Regex

type regexParams struct {
	Mode    string `json:"mode"` // match, groups
	Pattern string `json:"pattern"`
}

const maxGroups = 9

func (p regexParams) compile() (*regexp.Regexp, error) {
	if p.Pattern == "" || len(p.Pattern) > 500 {
		return nil, errors.New("шаблон: от 1 до 500 символов")
	}
	return regexp.Compile(p.Pattern) // RE2: linear time, no backtracking
}

func regexPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p regexParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	re, err := p.compile()
	if err != nil {
		return nil, nil, err
	}
	switch p.Mode {
	case "match":
		return []Port{req("in", TStr)}, []Port{req("yes", TStr), req("no", TStr)}, nil
	case "groups":
		if re.NumSubexp() > maxGroups {
			return nil, nil, fmt.Errorf("не больше %d групп", maxGroups)
		}
		out := []Port{req("g0", TStr)}
		for i := 1; i <= re.NumSubexp(); i++ {
			out = append(out, req("g"+strconv.Itoa(i), TStr))
		}
		return []Port{req("in", TStr)}, out, nil
	}
	return nil, nil, errors.New("mode: match или groups")
}

func runRegex(rs *resolved, in Token) (map[string]Token, error) {
	var p regexParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	re, err := p.compile()
	if err != nil {
		return nil, err
	}
	s, _ := in.Value.(string)
	if p.Mode == "match" {
		if re.MatchString(s) {
			return map[string]Token{"yes": in}, nil
		}
		return map[string]Token{"no": in}, nil
	}
	m := re.FindStringSubmatch(s)
	if m == nil {
		return nil, nil // no match: exit signals on every output
	}
	out := map[string]Token{}
	for i, g := range m {
		out["g"+strconv.Itoa(i)] = Token{Type: TStr, Value: g}
	}
	return out, nil
}

// ---- Evaluate

// budget is what one evaluation may spend.
type budget struct {
	rolls, evals int
}

type embedHost struct {
	args   map[string]string
	result *string
}

func (h *embedHost) Source(_ context.Context, n Node) (map[string]any, error) {
	if n.Type != "args" {
		return nil, fmt.Errorf("источник %s здесь не работает", n.Type)
	}
	var p struct {
		Params []ArgParam `json:"params"`
	}
	_ = json.Unmarshal(n.Params, &p)
	out := map[string]any{}
	for _, a := range p.Params {
		raw, ok := h.args[a.Name]
		if !ok {
			continue
		}
		switch a.Type {
		case TStr:
			out[a.Name] = raw
		case TInt:
			v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("аргумент %q: не целое число", a.Name)
			}
			out[a.Name] = v
		case TFloat:
			v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil {
				return nil, fmt.Errorf("аргумент %q: не число", a.Name)
			}
			out[a.Name] = v
		case TBool:
			v, err := strconv.ParseBool(strings.TrimSpace(raw))
			if err != nil {
				return nil, fmt.Errorf("аргумент %q: не да/нет", a.Name)
			}
			out[a.Name] = v
		}
	}
	return out, nil
}

func (h *embedHost) Sink(_ context.Context, n Node, in map[string]Token) error {
	if n.Type != "sink.result" {
		return fmt.Errorf("приёмник %s здесь не работает", n.Type)
	}
	s, _ := in["value"].Value.(string)
	*h.result = s
	return nil
}

// Evaluate runs a graph that holds only trusted nodes (Args … Result) on the server and returns what Result received.
// The graph must be expanded (no custom nodes). Arguments are given as text and read as the types Args declares.
func Evaluate(ctx context.Context, g Graph, reg Registry, args map[string]string) (string, error) {
	return evaluate(ctx, g, reg, withFirstArg(g, args), &budget{}, nil)
}

// EvaluateScoped is Evaluate with the named values of the text the script was clicked in.
func EvaluateScoped(ctx context.Context, g Graph, reg Registry, args map[string]string, scope *ExprScope) (string, error) {
	return evaluate(ctx, g, reg, withFirstArg(g, args), &budget{}, scope)
}

func evaluate(ctx context.Context, g Graph, reg Registry, args map[string]string, b *budget, scope *ExprScope) (string, error) {
	if g.RunsOnRunner() {
		return "", errors.New("в графе есть нода кода: встроить можно только скрипт без неё")
	}
	run, err := NewRun(g, reg)
	if err != nil {
		return "", err
	}
	var out string
	got := false
	h := &embedHost{args: args, result: &out}
	run.budget = b
	run.Scope = scope
	reqs, err := run.Advance(ctx, h)
	if err != nil {
		return "", err
	}
	if len(reqs) > 0 || !run.Finished() {
		return "", errors.New("граф не завершился на сервере")
	}
	for _, id := range run.order {
		if run.nodes[id].node.Type == "sink.result" && run.State(id) == StateRan {
			got = true
		}
	}
	if !got {
		return "", errors.New("результата нет: на пути оказался сигнал выхода")
	}
	return out, nil
}

// withFirstArg: the short form `script=N{text}` sends the text as argument "_", which is the first parameter of Args.
func withFirstArg(g Graph, args map[string]string) map[string]string {
	text, ok := args["_"]
	if !ok {
		return args
	}
	out := map[string]string{}
	for k, v := range args {
		if k != "_" {
			out[k] = v
		}
	}
	for _, n := range g.Nodes {
		if n.Type != "args" {
			continue
		}
		var p struct {
			Params []ArgParam `json:"params"`
		}
		if json.Unmarshal(n.Params, &p) == nil && len(p.Params) > 0 {
			if _, set := out[p.Params[0].Name]; !set {
				out[p.Params[0].Name] = text
			}
		}
		break
	}
	return out
}
