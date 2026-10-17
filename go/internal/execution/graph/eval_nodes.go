package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// coerce converts a token to the type of the input it feeds: the only conversions the type system allows implicitly.
func coerce(t Token, to PortType) Token {
	if t.Exit {
		return t
	}
	switch {
	case to == TFloat && t.Type == TInt:
		return Token{Type: TFloat, Value: float64(t.Value.(int64))}
	case to == TDoc && t.Type.scalar():
		b, _ := json.Marshal(t.Value)
		return Token{Type: TDoc, Value: json.RawMessage(b)}
	}
	return t
}

// fit checks that a value a host gave has the Go type the port type stands for, converting the obvious ones.
func fit(v any, t PortType) (any, error) {
	switch t {
	case TInt:
		switch n := v.(type) {
		case int64:
			return n, nil
		case int:
			return int64(n), nil
		}
	case TFloat:
		switch n := v.(type) {
		case float64:
			return n, nil
		case int64:
			return float64(n), nil
		case int:
			return float64(n), nil
		}
	case TStr:
		if s, ok := v.(string); ok {
			return s, nil
		}
	case TBool:
		if b, ok := v.(bool); ok {
			return b, nil
		}
	case TDoc:
		switch d := v.(type) {
		case json.RawMessage:
			return d, nil
		case []byte:
			return json.RawMessage(d), nil
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			return json.RawMessage(b), nil
		}
	case TSecret:
		if _, ok := v.(Secrets); ok {
			return v, nil
		}
	case TAny:
		return v, nil
	case TRefStep, TRefNote, TRefTemplate, TRefQuest, TRefWorkspace:
		if r, ok := v.(Ref); ok && PortType("ref<"+r.Kind+">") == t {
			return r, nil
		}
	}
	return nil, fmt.Errorf("значение %T не подходит типу %s", v, t)
}

// readJSONText reads the answer of a command as JSON: the whole trimmed text first (a multi-line object works), and if that
// is not JSON its last non-empty line (a script may write a log above its result).
func readJSONText(text string) (json.RawMessage, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return nil, fmt.Errorf("пустой вывод: нужен JSON")
	}
	if raw, ok := decodeOne(s); ok {
		return raw, nil
	}
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		if raw, ok := decodeOne(strings.TrimSpace(s[i+1:])); ok {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("вывод не JSON: %s", firstChars(s, 80))
}

func decodeOne(s string) (json.RawMessage, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil || dec.More() {
		return nil, false
	}
	return bytes.TrimSpace(raw), true
}

func firstChars(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// field converts one JSON value to the type of a schema field; ok=false means it does not fit (or is below the minimum),
// with why for a required field.
func (f parseField) convert(raw json.RawMessage) (any, bool, string) {
	s := string(bytes.TrimSpace(raw))
	bad := func(format string, a ...any) (any, bool, string) {
		return nil, false, fmt.Sprintf("поле %q: %s", f.Name, fmt.Sprintf(format, a...))
	}
	switch f.Type {
	case TDoc:
		return json.RawMessage(s), true, ""
	case TStr:
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return bad("ожидается строка, получено %s", firstChars(s, 40))
		}
		return v, true, ""
	case TBool:
		switch s {
		case "true", "1":
			return true, true, ""
		case "false", "0":
			return false, true, ""
		}
		return bad("ожидается true/false (или 0/1), получено %s", firstChars(s, 40))
	case TInt, TFloat:
		var num json.Number
		if err := json.Unmarshal(raw, &num); err != nil {
			return bad("ожидается число, получено %s", firstChars(s, 40))
		}
		fv, err := strconv.ParseFloat(string(num), 64)
		if err != nil || math.IsInf(fv, 0) || math.IsNaN(fv) {
			return bad("ожидается число, получено %s", firstChars(s, 40))
		}
		if f.Min != nil && fv < *f.Min {
			return bad("%s меньше минимума %v", s, *f.Min)
		}
		if f.Type == TFloat {
			return fv, true, ""
		}
		if n, err := strconv.ParseInt(string(num), 10, 64); err == nil {
			return n, true, ""
		}
		if fv == math.Trunc(fv) && math.Abs(fv) < 1<<53 {
			return int64(fv), true, ""
		}
		return bad("ожидается целое, получено %s", firstChars(s, 40))
	}
	return bad("неизвестный тип")
}

// runParse reads a command's text against the schema: a value per field that came right, an exit signal for an optional one
// that did not, and the reason on `bad_output` (with every field an exit signal) when the text is not what the schema wants.
func runParse(rs *resolved, in Token) (map[string]Token, error) {
	var p parseParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	text := envString(in) // the text itself, or the JSON of a document, or a scalar written out
	reject := func(msg string) (map[string]Token, error) {
		return map[string]Token{"bad_output": {Type: TStr, Value: msg}}, nil
	}
	raw, err := readJSONText(text)
	if err != nil {
		return reject(err.Error())
	}
	out := map[string]Token{}
	put := func(f parseField, raw json.RawMessage) (string, bool) {
		v, ok, why := f.convert(raw)
		switch {
		case ok:
			out[f.Name] = Token{Type: f.Type, Value: v}
		case f.Required:
			return why, false
		}
		return "", true
	}

	if p.Whole {
		if why, ok := put(p.Schema[0], raw); !ok {
			return reject(why)
		}
		return out, nil
	}
	if len(raw) > 0 && raw[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return reject("битый JSON: " + err.Error())
		}
		for _, f := range p.Schema {
			v, present := obj[f.Name]
			if !present {
				if f.Required {
					return reject(fmt.Sprintf("нет обязательного поля %q", f.Name))
				}
				continue
			}
			if why, ok := put(f, v); !ok {
				return reject(why)
			}
		}
		return out, nil
	}
	if len(raw) > 0 && raw[0] == '[' {
		return reject("ожидался объект или одно значение, получен массив")
	}
	// A single value stands for the first field (`5` is `{"progress": 5}`).
	for i, f := range p.Schema {
		if i == 0 {
			if why, ok := put(f, raw); !ok {
				return reject(why)
			}
		} else if f.Required {
			return reject(fmt.Sprintf("нет обязательного поля %q", f.Name))
		}
	}
	return out, nil
}

// runCompare sends the value to `yes` or to `no` and an exit signal to the other.
func runCompare(rs *resolved, in Token) (map[string]Token, error) {
	var p struct {
		Type    PortType        `json:"type"`
		Op      string          `json:"op"`
		Operand json.RawMessage `json:"operand"`
	}
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	var yes bool
	switch p.Type {
	case TInt:
		var b int64
		_ = json.Unmarshal(p.Operand, &b)
		yes = cmpOrdered(in.Value.(int64), b, p.Op)
	case TFloat:
		var b float64
		_ = json.Unmarshal(p.Operand, &b)
		yes = cmpOrdered(in.Value.(float64), b, p.Op)
	case TBool:
		var b bool
		_ = json.Unmarshal(p.Operand, &b)
		yes = in.Value.(bool) == b
	case TStr:
		var b string
		_ = json.Unmarshal(p.Operand, &b)
		a := in.Value.(string)
		switch p.Op {
		case "eq":
			yes = a == b
		case "ne":
			yes = a != b
		case "contains":
			yes = strings.Contains(a, b)
		case "prefix":
			yes = strings.HasPrefix(a, b)
		case "matches":
			re, err := regexp.Compile(b)
			if err != nil {
				return nil, err
			}
			yes = re.MatchString(a)
		}
	}
	if yes {
		return map[string]Token{"yes": in}, nil
	}
	return map[string]Token{"no": in}, nil
}

func cmpOrdered[T int64 | float64](a, b T, op string) bool {
	switch op {
	case "eq":
		return a == b
	case "ne":
		return a != b
	case "lt":
		return a < b
	case "le":
		return a <= b
	case "gt":
		return a > b
	case "ge":
		return a >= b
	}
	return false
}
