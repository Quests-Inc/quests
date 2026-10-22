package execution

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

type formParser struct {
	s   string
	i   int
	get func(string) (formVal, error)
}

func parseFormula(src string, get func(string) (formVal, error)) (formVal, error) {
	if utf8.RuneCountInString(src) > maxFormula {
		return formVal{}, &ValueError{Msg: "формула длиннее 1024 символов"}
	}
	p := formParser{s: src, get: get}
	v, err := p.or()
	if err != nil {
		return formVal{}, err
	}
	p.skip()
	if p.i != len(p.s) {
		return formVal{}, &ValueError{Msg: "формула: лишний текст"}
	}
	return v, nil
}

func (p *formParser) skip() {
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return
		}
		p.i++
	}
}

func (p *formParser) take(op string) bool {
	p.skip()
	if p.i+len(op) > len(p.s) || p.s[p.i:p.i+len(op)] != op {
		return false
	}
	p.i += len(op)
	return true
}

func (p *formParser) or() (formVal, error) {
	left, err := p.and()
	if err != nil {
		return formVal{}, err
	}
	for p.take("||") {
		right, err := p.and()
		if err != nil {
			return formVal{}, err
		}
		left, err = applyBool("||", left, right)
		if err != nil {
			return formVal{}, err
		}
	}
	return left, nil
}

func (p *formParser) and() (formVal, error) {
	left, err := p.cmp()
	if err != nil {
		return formVal{}, err
	}
	for p.take("&&") {
		right, err := p.cmp()
		if err != nil {
			return formVal{}, err
		}
		left, err = applyBool("&&", left, right)
		if err != nil {
			return formVal{}, err
		}
	}
	return left, nil
}

func (p *formParser) cmp() (formVal, error) {
	left, err := p.sum()
	if err != nil {
		return formVal{}, err
	}
	for {
		op := ""
		switch {
		case p.take("=="):
			op = "=="
		case p.take("!="):
			op = "!="
		case p.take("<="):
			op = "<="
		case p.take(">="):
			op = ">="
		case p.take("<"):
			op = "<"
		case p.take(">"):
			op = ">"
		default:
			return left, nil
		}
		right, err := p.sum()
		if err != nil {
			return formVal{}, err
		}
		left, err = applyCmp(op, left, right)
		if err != nil {
			return formVal{}, err
		}
	}
}

func (p *formParser) sum() (formVal, error) {
	left, err := p.prod()
	if err != nil {
		return formVal{}, err
	}
	for {
		op := ""
		switch {
		case p.take("+"):
			op = "+"
		case p.take("-"):
			op = "-"
		default:
			return left, nil
		}
		right, err := p.prod()
		if err != nil {
			return formVal{}, err
		}
		left, err = applyAdd(op, left, right)
		if err != nil {
			return formVal{}, err
		}
	}
}

func (p *formParser) prod() (formVal, error) {
	left, err := p.unary()
	if err != nil {
		return formVal{}, err
	}
	for {
		op := ""
		switch {
		case p.take("*"):
			op = "*"
		case p.take("/"):
			op = "/"
		case p.take("%"):
			op = "%"
		default:
			return left, nil
		}
		right, err := p.unary()
		if err != nil {
			return formVal{}, err
		}
		left, err = applyMul(op, left, right)
		if err != nil {
			return formVal{}, err
		}
	}
}

func (p *formParser) unary() (formVal, error) {
	p.skip()
	if p.i >= len(p.s) {
		return formVal{}, &ValueError{Msg: "формула: оборвана"}
	}
	if p.s[p.i] == '!' && (p.i+1 >= len(p.s) || p.s[p.i+1] != '=') {
		p.i++
		v, err := p.unary()
		if err != nil {
			return formVal{}, err
		}
		if v.kind != vkBool {
			return formVal{}, &ValueError{Msg: "формула: ! ждёт да/нет"}
		}
		return formVal{kind: vkBool, b: !v.b}, nil
	}
	if p.s[p.i] == '-' {
		p.i++
		v, err := p.unary()
		if err != nil {
			return formVal{}, err
		}
		return applyNeg(v)
	}
	return p.primary()
}

func (p *formParser) primary() (formVal, error) {
	p.skip()
	if p.i >= len(p.s) {
		return formVal{}, &ValueError{Msg: "формула: оборвана"}
	}
	switch c := p.s[p.i]; {
	case c == '(':
		p.i++
		v, err := p.or()
		if err != nil {
			return formVal{}, err
		}
		p.skip()
		if p.i >= len(p.s) || p.s[p.i] != ')' {
			return formVal{}, &ValueError{Msg: "формула: незакрытая скобка"}
		}
		p.i++
		return v, nil
	case c == '"':
		return p.str()
	case c >= '0' && c <= '9':
		return p.number()
	case c >= 'a' && c <= 'z':
		return p.ident()
	default:
		return formVal{}, &ValueError{Msg: "формула: неожиданный символ"}
	}
}

func (p *formParser) ident() (formVal, error) {
	start := p.i
	for p.i < len(p.s) {
		c := p.s[p.i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			p.i++
			continue
		}
		break
	}
	word := p.s[start:p.i]
	switch word {
	case "true":
		return formVal{kind: vkBool, b: true}, nil
	case "false":
		return formVal{kind: vkBool, b: false}, nil
	}
	if !valueNameRe.MatchString(word) {
		return formVal{}, &ValueError{Msg: "формула: имя «" + word + "»"}
	}
	return p.get(word)
}

func (p *formParser) number() (formVal, error) {
	start := p.i
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	if p.i < len(p.s) && p.s[p.i] == '.' {
		p.i++
		frac := p.i
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
		}
		if p.i == frac {
			return formVal{}, &ValueError{Msg: "формула: дробное без цифр после точки"}
		}
		f, err := strconv.ParseFloat(p.s[start:p.i], 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || outOfAbs(f) {
			return formVal{}, &ValueError{Msg: "дробное вне диапазона"}
		}
		return formVal{kind: vkFloat, f: f}, nil
	}
	n, err := strconv.ParseInt(p.s[start:p.i], 10, 64)
	if err != nil || outOfAbs(float64(n)) {
		return formVal{}, &ValueError{Msg: "целое вне диапазона"}
	}
	return formVal{kind: vkInt, i: n}, nil
}

func (p *formParser) str() (formVal, error) {
	p.i++
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		p.i++
		switch c {
		case '"':
			if b.Len() > maxValueStr {
				return formVal{}, &ValueError{Msg: "строка длиннее 500 символов"}
			}
			return formVal{kind: vkStr, s: b.String()}, nil
		case '\\':
			if p.i >= len(p.s) {
				return formVal{}, &ValueError{Msg: "формула: оборванная строка"}
			}
			n := p.s[p.i]
			p.i++
			if n != '\\' && n != '"' {
				return formVal{}, &ValueError{Msg: "формула: неизвестная последовательность в строке"}
			}
			b.WriteByte(n)
		case '\n':
			return formVal{}, &ValueError{Msg: "формула: оборванная строка"}
		default:
			b.WriteByte(c)
		}
	}
	return formVal{}, &ValueError{Msg: "формула: оборванная строка"}
}

func applyBool(op string, l, r formVal) (formVal, error) {
	if l.kind != vkBool || r.kind != vkBool {
		return formVal{}, &ValueError{Msg: "формула: " + op + " ждёт да/нет"}
	}
	out := l.b || r.b
	if op == "&&" {
		out = l.b && r.b
	}
	return formVal{kind: vkBool, b: out}, nil
}

func applyCmp(op string, l, r formVal) (formVal, error) {
	if l.kind == vkBool || r.kind == vkBool {
		if l.kind != r.kind {
			return formVal{}, &ValueError{Msg: "формула: нельзя сравнить разные типы"}
		}
		if op != "==" && op != "!=" {
			return formVal{}, &ValueError{Msg: "формула: сравнение " + op + " ждёт числа или строки"}
		}
		same := l.b == r.b
		return formVal{kind: vkBool, b: same == (op == "==")}, nil
	}
	if l.kind == vkStr || r.kind == vkStr {
		if l.kind != r.kind {
			return formVal{}, &ValueError{Msg: "формула: нельзя сравнить разные типы"}
		}
		return formVal{kind: vkBool, b: cmpOrd(op, strings.Compare(l.s, r.s))}, nil
	}
	if l.kind == vkInt && r.kind == vkInt {
		return formVal{kind: vkBool, b: cmpOrd(op, cmpInt(l.i, r.i))}, nil
	}
	lf, okL := asFloat(l)
	rf, okR := asFloat(r)
	if !okL || !okR {
		return formVal{}, &ValueError{Msg: "формула: нельзя сравнить разные типы"}
	}
	return formVal{kind: vkBool, b: cmpOrd(op, cmpFloat(lf, rf))}, nil
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpOrd(op string, ord int) bool {
	switch op {
	case "==":
		return ord == 0
	case "!=":
		return ord != 0
	case "<":
		return ord < 0
	case "<=":
		return ord <= 0
	case ">":
		return ord > 0
	default:
		return ord >= 0
	}
}

func applyAdd(op string, l, r formVal) (formVal, error) {
	if op == "+" && l.kind == vkStr && r.kind == vkStr {
		if len(l.s)+len(r.s) > maxValueStr {
			return formVal{}, &ValueError{Msg: "строка длиннее 500 символов"}
		}
		return formVal{kind: vkStr, s: l.s + r.s}, nil
	}
	if op == "+" && (l.kind == vkStr || r.kind == vkStr) {
		return formVal{}, &ValueError{Msg: "формула: + ждёт два числа или две строки"}
	}
	return applyNum(op, l, r)
}

func applyMul(op string, l, r formVal) (formVal, error) {
	return applyNum(op, l, r)
}

func applyNum(op string, l, r formVal) (formVal, error) {
	if !isNum(l) || !isNum(r) {
		return formVal{}, &ValueError{Msg: "формула: " + op + " ждёт два числа"}
	}
	if l.kind == vkInt && r.kind == vkInt {
		n, err := intOp(op, l.i, r.i)
		if err != nil {
			return formVal{}, err
		}
		return formVal{kind: vkInt, i: n}, nil
	}
	lf, _ := asFloat(l)
	rf, _ := asFloat(r)
	f, err := floatOp(op, lf, rf)
	if err != nil {
		return formVal{}, err
	}
	return formVal{kind: vkFloat, f: f}, nil
}

func isNum(v formVal) bool { return v.kind == vkInt || v.kind == vkFloat }

func asFloat(v formVal) (float64, bool) {
	switch v.kind {
	case vkInt:
		return float64(v.i), true
	case vkFloat:
		return v.f, true
	default:
		return 0, false
	}
}

func applyNeg(v formVal) (formVal, error) {
	switch v.kind {
	case vkInt:
		if v.i == math.MinInt64 || outOfAbs(float64(-v.i)) {
			return formVal{}, &ValueError{Msg: "целое вне диапазона"}
		}
		return formVal{kind: vkInt, i: -v.i}, nil
	case vkFloat:
		if outOfAbs(-v.f) {
			return formVal{}, &ValueError{Msg: "дробное вне диапазона"}
		}
		return formVal{kind: vkFloat, f: -v.f}, nil
	default:
		return formVal{}, &ValueError{Msg: "формула: - ждёт число"}
	}
}

func intOp(op string, a, b int64) (int64, error) {
	var n int64
	switch op {
	case "+":
		n = a + b
	case "-":
		n = a - b
	case "*":
		n = a * b
	case "/":
		q, err := floorDiv(a, b)
		if err != nil {
			return 0, err
		}
		n = q
	case "%":
		q, err := floorDiv(a, b)
		if err != nil {
			return 0, err
		}
		n = a - b*q
	default:
		return 0, &ValueError{Msg: "формула: " + op + " ждёт два числа"}
	}
	if outOfAbs(float64(n)) {
		return 0, &ValueError{Msg: "целое вне диапазона"}
	}
	return n, nil
}

// floorDiv rounds toward -∞. The remainder a - b*q then has the sign of b.
func floorDiv(a, b int64) (int64, error) {
	if b == 0 {
		return 0, &ValueError{Msg: "деление на ноль"}
	}
	q := a / b
	r := a % b
	if r != 0 && ((a < 0) != (b < 0)) {
		q--
	}
	return q, nil
}

func floatOp(op string, a, b float64) (float64, error) {
	var f float64
	switch op {
	case "+":
		f = a + b
	case "-":
		f = a - b
	case "*":
		f = a * b
	case "/":
		if b == 0 {
			return 0, &ValueError{Msg: "деление на ноль"}
		}
		f = a / b
	case "%":
		if b == 0 {
			return 0, &ValueError{Msg: "деление на ноль"}
		}
		f = math.Mod(a, b)
	default:
		return 0, &ValueError{Msg: "формула: " + op + " ждёт два числа"}
	}
	if math.IsNaN(f) || math.IsInf(f, 0) || outOfAbs(f) {
		return 0, &ValueError{Msg: "дробное вне диапазона"}
	}
	return f, nil
}
