package execution

import (
	"math"
	"strconv"
)

// A formula is a typed expression over the other named values in scope. The value's type is the result type.
// Integer / and % round toward -∞, so (score - 10) / 2 is an ability modifier.
//
//	int, float   + - * / %   and unary -
//	bool         ! && ||     comparisons give a bool
//	str          +           concatenation
const maxFormula = 1024

type valKind int

const (
	vkInt valKind = iota
	vkFloat
	vkBool
	vkStr
)

type formVal struct {
	kind valKind
	i    int64
	f    float64
	b    bool
	s    string
}

type formEval struct {
	rows     map[string]StoredValue
	memo     map[string]formVal
	bad      map[string]string
	visiting map[string]bool
}

// EvalValues computes every formula in rows. A literal is left as stored. A broken formula keeps its text and
// sets Error; its Value is cleared so a caller does not read a stale number.
func EvalValues(rows []StoredValue) []StoredValue {
	e := &formEval{
		rows:     map[string]StoredValue{},
		memo:     map[string]formVal{},
		bad:      map[string]string{},
		visiting: map[string]bool{},
	}
	out := make([]StoredValue, len(rows))
	for i, row := range rows {
		e.rows[row.Name] = row
		out[i] = row
	}
	for i := range out {
		v, err := e.get(out[i].Name)
		if err != nil {
			out[i].Error = err.Error()
			out[i].Value = ""
			continue
		}
		out[i].Value = formatForm(v)
		out[i].Error = ""
	}
	return out
}

func (e *formEval) get(name string) (formVal, error) {
	if v, ok := e.memo[name]; ok {
		return v, nil
	}
	if msg, ok := e.bad[name]; ok {
		return formVal{}, &ValueError{Msg: msg}
	}
	row, ok := e.rows[name]
	if !ok {
		return formVal{}, &ValueError{Msg: "неизвестное значение «" + name + "»"}
	}
	if e.visiting[name] {
		return formVal{}, &ValueError{Msg: "значение «" + name + "» зациклено"}
	}
	e.visiting[name] = true
	defer delete(e.visiting, name)
	var (
		v   formVal
		err error
	)
	if row.Formula == "" {
		v, err = literalForm(row)
	} else {
		v, err = parseFormula(row.Formula, e.get)
	}
	if err == nil {
		v, err = coerceForm(row.Type, v)
	}
	if err != nil {
		e.bad[name] = err.Error()
		return formVal{}, err
	}
	e.memo[name] = v
	return v, nil
}

func literalForm(row StoredValue) (formVal, error) {
	switch row.Type {
	case "int":
		n, err := strconv.ParseInt(row.Value, 10, 64)
		if err != nil {
			return formVal{}, &ValueError{Msg: "целое вне диапазона"}
		}
		return formVal{kind: vkInt, i: n}, nil
	case "float":
		f, err := strconv.ParseFloat(row.Value, 64)
		if err != nil {
			return formVal{}, &ValueError{Msg: "дробное вне диапазона"}
		}
		return formVal{kind: vkFloat, f: f}, nil
	case "bool":
		return formVal{kind: vkBool, b: row.Value == "true"}, nil
	case "str":
		return formVal{kind: vkStr, s: row.Value}, nil
	default:
		return formVal{}, &ValueError{Msg: "тип: int, float, str или bool"}
	}
}

func coerceForm(typ string, v formVal) (formVal, error) {
	switch typ {
	case "int":
		if v.kind != vkInt {
			return formVal{}, &ValueError{Msg: "формула даёт " + kindWord(v.kind) + ", а тип — число"}
		}
		if outOfAbs(float64(v.i)) {
			return formVal{}, &ValueError{Msg: "целое вне диапазона"}
		}
		return v, nil
	case "float":
		if v.kind == vkInt {
			v = formVal{kind: vkFloat, f: float64(v.i)}
		}
		if v.kind != vkFloat || math.IsNaN(v.f) || math.IsInf(v.f, 0) || outOfAbs(v.f) {
			if v.kind != vkFloat {
				return formVal{}, &ValueError{Msg: "формула даёт " + kindWord(v.kind) + ", а тип — дробное"}
			}
			return formVal{}, &ValueError{Msg: "дробное вне диапазона"}
		}
		return v, nil
	case "bool":
		if v.kind != vkBool {
			return formVal{}, &ValueError{Msg: "формула даёт " + kindWord(v.kind) + ", а тип — да/нет"}
		}
		return v, nil
	case "str":
		if v.kind != vkStr {
			return formVal{}, &ValueError{Msg: "формула даёт " + kindWord(v.kind) + ", а тип — строка"}
		}
		if len(v.s) > maxValueStr {
			return formVal{}, &ValueError{Msg: "строка длиннее 500 символов"}
		}
		return v, nil
	default:
		return formVal{}, &ValueError{Msg: "тип: int, float, str или bool"}
	}
}

func kindWord(k valKind) string {
	switch k {
	case vkInt:
		return "число"
	case vkFloat:
		return "дробное"
	case vkBool:
		return "да/нет"
	default:
		return "строку"
	}
}

func formatForm(v formVal) string {
	switch v.kind {
	case vkInt:
		return strconv.FormatInt(v.i, 10)
	case vkFloat:
		return strconv.FormatFloat(v.f, 'f', -1, 64)
	case vkBool:
		if v.b {
			return "true"
		}
		return "false"
	default:
		return v.s
	}
}

func outOfAbs(f float64) bool {
	return f > maxValueAbs || f < -maxValueAbs
}
