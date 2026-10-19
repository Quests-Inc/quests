package graph

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The arithmetic of the Expr node (ADR-0003 §6b): numbers, + - * / %, parentheses, unary minus, named variables, a few
// functions, and the operators the owner defines (`к`: `sum(a, rand(1, b))` makes `3к8` a roll of three eight-sided dice).
// Its own precedence-climbing parser: nothing is ever handed to an interpreter.

// Limits of an expression.
const (
	MaxExprLen   = 500
	maxExprDepth = 32
	MaxOperators = 8
	MaxEvals     = 100000 // evaluations of subexpressions per call, so `sum` cannot be used to burn the server
)

// Operator is a user-defined operator: a symbol, how many operands (1 = prefix, 2 = infix), where it stands in the order of
// operations (1..9; + and - are 1, * / % are 2, unary minus 3) and its definition: an expression over `a` (and `b`) made of
// the safe core, evaluated afresh on every use.
type Operator struct {
	Symbol string `json:"symbol"`
	Arity  int    `json:"arity"`
	Prec   int    `json:"prec"`
	Right  bool   `json:"right,omitempty"` // right-associative (infix only)
	Def    string `json:"def"`
}

type compiledOp struct {
	Operator
	def *ast
}

type astKind int

const (
	kNum astKind = iota
	kVar
	kNeg
	kBin  // builtin when op == nil
	kPre  // user prefix operator
	kCall // function call
)

type ast struct {
	kind astKind
	num  float64
	name string
	args []*ast
	op   *compiledOp
}

// funcArity: -1 = at least one argument.
var funcArity = map[string]int{
	"floor": 1, "ceil": 1, "round": 1, "abs": 1, "min": -1, "max": -1,
	"rand": 2, "randf": 2, // a number in lo..hi (whole / any)
	"sum": 2, "rmax": 2, "rmin": 2, // repeat the second argument n times, fold: n is evaluated once, the repeated one afresh each time
}

type exprParser struct {
	s     string
	pos   int
	ops   []compiledOp
	depth int
}

func parseExpr(src string, ops []compiledOp) (*ast, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil, errors.New("пустое выражение")
	}
	if len(src) > MaxExprLen {
		return nil, fmt.Errorf("выражение длиннее %d символов", MaxExprLen)
	}
	p := &exprParser{s: src, ops: ops}
	a, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	p.skip()
	if p.pos < len(p.s) {
		return nil, fmt.Errorf("лишнее в позиции %d: %q", p.pos+1, firstChars(p.s[p.pos:], 12))
	}
	return a, nil
}

func (p *exprParser) skip() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) peek() byte {
	p.skip()
	if p.pos >= len(p.s) {
		return 0
	}
	return p.s[p.pos]
}

// userOp finds the longest user operator of the arity that starts at the current position.
func (p *exprParser) userOp(arity int) *compiledOp {
	var best *compiledOp
	for i := range p.ops {
		o := &p.ops[i]
		if o.Arity == arity && strings.HasPrefix(p.s[p.pos:], o.Symbol) && (best == nil || len(o.Symbol) > len(best.Symbol)) {
			best = o
		}
	}
	return best
}

// infix says which operator stands next, with its precedence and associativity.
func (p *exprParser) infix() (sym string, prec int, right bool, user *compiledOp, ok bool) {
	p.skip()
	if p.pos >= len(p.s) {
		return
	}
	if o := p.userOp(2); o != nil {
		return o.Symbol, o.Prec, o.Right, o, true
	}
	switch p.s[p.pos] {
	case '+', '-':
		return p.s[p.pos : p.pos+1], 1, false, nil, true
	case '*', '/', '%':
		return p.s[p.pos : p.pos+1], 2, false, nil, true
	}
	return
}

func (p *exprParser) expr(minPrec int) (*ast, error) {
	if p.depth++; p.depth > maxExprDepth {
		return nil, errors.New("слишком глубокая вложенность")
	}
	defer func() { p.depth-- }()
	left, err := p.operand()
	if err != nil {
		return nil, err
	}
	for {
		sym, prec, right, user, ok := p.infix()
		if !ok || prec < minPrec {
			return left, nil
		}
		p.pos += len(sym)
		next := prec + 1
		if right {
			next = prec
		}
		r, err := p.expr(next)
		if err != nil {
			return nil, err
		}
		left = &ast{kind: kBin, name: sym, op: user, args: []*ast{left, r}}
	}
}

func isIdentStart(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' }
func isDigit(c byte) bool      { return c >= '0' && c <= '9' }

func (p *exprParser) operand() (*ast, error) {
	c := p.peek()
	switch c {
	case '-', '+':
		p.pos++
		v, err := p.expr(3)
		if err != nil || c == '+' {
			return v, err
		}
		return &ast{kind: kNeg, args: []*ast{v}}, nil
	case 0:
		return nil, errors.New("выражение оборвано")
	}
	if o := p.userOp(1); o != nil {
		p.pos += len(o.Symbol)
		v, err := p.expr(o.Prec)
		if err != nil {
			return nil, err
		}
		return &ast{kind: kPre, name: o.Symbol, op: o, args: []*ast{v}}, nil
	}
	return p.atom(c)
}

func (p *exprParser) atom(c byte) (*ast, error) {
	switch {
	case c == '(':
		p.pos++
		v, err := p.expr(0)
		if err != nil {
			return nil, err
		}
		if p.peek() != ')' {
			return nil, fmt.Errorf("нет закрывающей скобки (позиция %d)", p.pos+1)
		}
		p.pos++
		return v, nil
	case isDigit(c) || c == '.':
		start := p.pos
		for p.pos < len(p.s) && (isDigit(p.s[p.pos]) || p.s[p.pos] == '.') {
			p.pos++
		}
		v, err := strconv.ParseFloat(p.s[start:p.pos], 64)
		if err != nil {
			return nil, fmt.Errorf("число %q", p.s[start:p.pos])
		}
		return &ast{kind: kNum, num: v}, nil
	case isIdentStart(c):
		start := p.pos
		for p.pos < len(p.s) && (isIdentStart(p.s[p.pos]) || isDigit(p.s[p.pos])) {
			p.pos++
		}
		name := p.s[start:p.pos]
		if p.peek() != '(' {
			return &ast{kind: kVar, name: name}, nil
		}
		return p.call(name)
	}
	return nil, fmt.Errorf("неожиданный символ %q (позиция %d)", string(c), p.pos+1)
}

func (p *exprParser) call(name string) (*ast, error) {
	want, known := funcArity[name]
	if !known {
		return nil, fmt.Errorf("неизвестная функция %q", name)
	}
	p.pos++ // (
	var args []*ast
	if p.peek() == ')' {
		p.pos++
	} else {
		for {
			v, err := p.expr(0)
			if err != nil {
				return nil, err
			}
			args = append(args, v)
			c := p.peek()
			p.pos++
			if c == ')' {
				break
			}
			if c != ',' {
				return nil, fmt.Errorf("в вызове %s ждали , или )", name)
			}
			if len(args) >= 16 {
				return nil, fmt.Errorf("в %s не больше 16 аргументов", name)
			}
		}
	}
	switch {
	case want == -1 && len(args) == 0:
		return nil, fmt.Errorf("%s: нужен хотя бы один аргумент", name)
	case want >= 0 && len(args) != want:
		return nil, fmt.Errorf("%s берёт аргументов: %d", name, want)
	}
	return &ast{kind: kCall, name: name, args: args}, nil
}

// ---- evaluation

type evalEnv struct {
	vars map[string]float64
	b    *budget
}

func (a *ast) eval(env *evalEnv) (float64, error) {
	if env.b.evals++; env.b.evals > MaxEvals {
		return 0, errors.New("слишком много вычислений за один вызов")
	}
	switch a.kind {
	case kNum:
		return a.num, nil
	case kVar:
		v, ok := env.vars[a.name]
		if !ok {
			return 0, fmt.Errorf("неизвестная переменная %q", a.name)
		}
		return v, nil
	case kNeg:
		v, err := a.args[0].eval(env)
		return -v, err
	case kBin:
		l, err := a.args[0].eval(env)
		if err != nil {
			return 0, err
		}
		r, err := a.args[1].eval(env)
		if err != nil {
			return 0, err
		}
		if a.op != nil {
			return a.op.def.eval(&evalEnv{vars: map[string]float64{"a": l, "b": r}, b: env.b})
		}
		switch a.name {
		case "+":
			return l + r, nil
		case "-":
			return l - r, nil
		case "*":
			return l * r, nil
		}
		if r == 0 {
			return 0, errors.New("деление на ноль")
		}
		if a.name == "/" {
			return l / r, nil
		}
		return math.Mod(l, r), nil
	case kPre:
		v, err := a.args[0].eval(env)
		if err != nil {
			return 0, err
		}
		return a.op.def.eval(&evalEnv{vars: map[string]float64{"a": v}, b: env.b})
	}
	return a.call(env)
}

func (a *ast) call(env *evalEnv) (float64, error) {
	switch a.name {
	case "sum", "rmax", "rmin":
		nf, err := a.args[0].eval(env)
		if err != nil {
			return 0, err
		}
		if nf != math.Trunc(nf) || nf < 0 || nf > MaxRolls {
			return 0, fmt.Errorf("%s: число повторов от 0 до %d (получено %s)", a.name, MaxRolls, FormatNumber(nf))
		}
		n := int(nf)
		if n == 0 && a.name != "sum" {
			return 0, fmt.Errorf("%s: нужен хотя бы один повтор", a.name)
		}
		acc := 0.0
		for i := 0; i < n; i++ {
			v, err := a.args[1].eval(env)
			if err != nil {
				return 0, err
			}
			switch {
			case a.name == "sum":
				acc += v
			case i == 0:
				acc = v
			case a.name == "rmax":
				acc = math.Max(acc, v)
			default:
				acc = math.Min(acc, v)
			}
		}
		return acc, nil
	}
	vals := make([]float64, len(a.args))
	for i, x := range a.args {
		v, err := x.eval(env)
		if err != nil {
			return 0, err
		}
		vals[i] = v
	}
	switch a.name {
	case "floor":
		return math.Floor(vals[0]), nil
	case "ceil":
		return math.Ceil(vals[0]), nil
	case "round":
		return math.Round(vals[0]), nil
	case "abs":
		return math.Abs(vals[0]), nil
	case "min", "max":
		v := vals[0]
		for _, x := range vals[1:] {
			if a.name == "min" {
				v = math.Min(v, x)
			} else {
				v = math.Max(v, x)
			}
		}
		return v, nil
	case "rand", "randf":
		lo, hi := vals[0], vals[1]
		if a.name == "rand" {
			lo, hi = math.Floor(lo), math.Floor(hi)
		}
		if hi < lo || lo < -maxBoundAbs || hi > maxBoundAbs {
			return 0, fmt.Errorf("%s: диапазон %s..%s неверен", a.name, FormatNumber(lo), FormatNumber(hi))
		}
		if env.b.rolls++; env.b.rolls > MaxRollsTotal {
			return 0, fmt.Errorf("слишком много бросков за раз (больше %d)", MaxRollsTotal)
		}
		if a.name == "rand" {
			return float64(int64(lo) + rand.Int64N(int64(hi)-int64(lo)+1)), nil
		}
		return lo + rand.Float64()*(hi-lo), nil
	}
	return 0, fmt.Errorf("неизвестная функция %q", a.name)
}

// ---- operators

// compileOperators checks the owner's operators and parses their definitions. A definition is made of the core only (its
// own operators are not in scope), so an operator can neither call itself nor close a loop.
func compileOperators(list []Operator) ([]compiledOp, error) {
	if len(list) > MaxOperators {
		return nil, fmt.Errorf("не больше %d операторов", MaxOperators)
	}
	seen := map[string]bool{}
	out := make([]compiledOp, 0, len(list))
	for _, o := range list {
		if n := utf8.RuneCountInString(o.Symbol); n < 1 || n > 3 || strings.ContainsAny(o.Symbol, " \t0123456789.,()+-*/%") {
			return nil, fmt.Errorf("оператор %q: от 1 до 3 символов, без пробелов, цифр и + - * / %% ( ) . ,", o.Symbol)
		}
		if _, isFn := funcArity[o.Symbol]; isFn {
			return nil, fmt.Errorf("оператор %q совпадает с именем функции", o.Symbol)
		}
		if o.Arity != 1 && o.Arity != 2 {
			return nil, fmt.Errorf("оператор %q: операндов 1 (префикс) или 2 (между)", o.Symbol)
		}
		if o.Prec < 1 || o.Prec > 9 {
			return nil, fmt.Errorf("оператор %q: приоритет от 1 до 9", o.Symbol)
		}
		if key := fmt.Sprintf("%s/%d", o.Symbol, o.Arity); seen[key] {
			return nil, fmt.Errorf("оператор %q (%d операнда) задан дважды", o.Symbol, o.Arity)
		} else {
			seen[key] = true
		}
		def, err := parseExpr(o.Def, nil)
		if err != nil {
			return nil, fmt.Errorf("оператор %q, определение: %w", o.Symbol, err)
		}
		allowed := map[string]bool{"a": true}
		if o.Arity == 2 {
			allowed["b"] = true
		}
		if name := freeVar(def, allowed); name != "" {
			return nil, fmt.Errorf("оператор %q, определение: переменная %q (доступны a%s)", o.Symbol, name, map[bool]string{true: ", b", false: ""}[o.Arity == 2])
		}
		out = append(out, compiledOp{Operator: o, def: def})
	}
	return out, nil
}

// freeVar returns a variable the expression reads that is not allowed ("" when all are).
func freeVar(a *ast, allowed map[string]bool) string {
	if a.kind == kVar && !allowed[a.name] {
		return a.name
	}
	for _, c := range a.args {
		if v := freeVar(c, allowed); v != "" {
			return v
		}
	}
	return ""
}

// DiceOperators are the operators of the dice example: `NкM` and `NdM` are a roll of N dice with M sides, `кM` / `dM` one die.
func DiceOperators() []Operator {
	return []Operator{
		{Symbol: "к", Arity: 2, Prec: 5, Def: "sum(a, rand(1, b))"},
		{Symbol: "d", Arity: 2, Prec: 5, Def: "sum(a, rand(1, b))"},
		{Symbol: "к", Arity: 1, Prec: 5, Def: "rand(1, a)"},
		{Symbol: "d", Arity: 1, Prec: 5, Def: "rand(1, a)"},
	}
}

// EvalExpr computes an expression over the given variables, with no operators of its own.
func EvalExpr(s string, vars map[string]float64) (float64, error) {
	return evalExprWith(s, vars, nil, &budget{})
}

func evalExprWith(s string, vars map[string]float64, ops []compiledOp, b *budget) (float64, error) {
	a, err := parseExpr(s, ops)
	if err != nil {
		return 0, err
	}
	v, err := a.eval(&evalEnv{vars: vars, b: b})
	if err != nil {
		return 0, err
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, errors.New("результат не число")
	}
	return v, nil
}

// FormatNumber prints a whole number without a fraction (24, not 24.000000).
func FormatNumber(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', 12, 64)
}
