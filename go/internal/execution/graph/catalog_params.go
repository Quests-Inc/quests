package graph

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// Limits of node parameters.
const (
	MaxCodeBytes  = 64 << 10
	MaxParseField = 32
)

var (
	identRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
	aliasRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,39}$`)
)

// strictParams decodes node parameters, refusing unknown fields. An empty parameter set is the zero value.
func strictParams(raw json.RawMessage, into any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("параметры: %w", err)
	}
	return nil
}

// codePorts: Code takes the secrets and any number of values over links (the environment of the job). It leaves by `stdout`
// (a clean exit: the text it printed) or by `failed` (anything else: a JSON document, see Failure).
func codePorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p struct {
		Language string `json:"language"`
		Text     string `json:"text"`
	}
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	switch p.Language {
	case "", "sh", "bash", "python", "node":
	default:
		return nil, nil, fmt.Errorf("язык %q не поддерживается (sh, bash, python, node)", p.Language)
	}
	if len(bytes.TrimSpace([]byte(p.Text))) == 0 {
		return nil, nil, errors.New("пустой текст кода")
	}
	if len(p.Text) > MaxCodeBytes {
		return nil, nil, fmt.Errorf("текст кода длиннее %d КБ", MaxCodeBytes>>10)
	}
	in := []Port{opt("secrets", TSecret), {Name: "in", Type: TValue, Optional: true, Multi: true}}
	out := []Port{req("stdout", TStr), req("failed", TDoc)}
	return in, out, nil
}

// parseParams: the schema, and Whole = the entire JSON value is the one field (a document that is itself the answer, as a
// template's quest), rather than an object to take the field from.
type parseParams struct {
	Schema []parseField `json:"schema"`
	Whole  bool         `json:"whole,omitempty"`
}

type parseField struct {
	Name     string   `json:"name"`
	Type     PortType `json:"type"`
	Required bool     `json:"required"`
	Min      *float64 `json:"min,omitempty"`
}

// parsePorts: one output per schema field, plus bad_output with the reason the text is not what the schema wants. It reads
// the text a command printed, or a document such as the one on a Code node's `failed` output.
func parsePorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p parseParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if len(p.Schema) == 0 || len(p.Schema) > MaxParseField {
		return nil, nil, fmt.Errorf("в схеме от 1 до %d полей", MaxParseField)
	}
	if p.Whole && len(p.Schema) != 1 {
		return nil, nil, errors.New("«всё значение целиком» — для схемы из одного поля")
	}
	seen := map[string]bool{"bad_output": true}
	out := make([]Port, 0, len(p.Schema)+1)
	for _, f := range p.Schema {
		switch {
		case !identRe.MatchString(f.Name):
			return nil, nil, fmt.Errorf("поле %q: имя из строчных латинских букв, цифр и _", f.Name)
		case seen[f.Name]:
			return nil, nil, fmt.Errorf("поле %q повторяется или зарезервировано", f.Name)
		case !f.Type.value():
			return nil, nil, fmt.Errorf("поле %q: тип int, float, str, bool или doc", f.Name)
		case f.Min != nil && f.Type != TInt && f.Type != TFloat:
			return nil, nil, fmt.Errorf("поле %q: минимум только у чисел", f.Name)
		}
		seen[f.Name] = true
		out = append(out, Port{Name: f.Name, Type: f.Type, Optional: !f.Required})
	}
	out = append(out, req("bad_output", TStr))
	return []Port{req("in", TValue)}, out, nil
}

// Operations of Compare per type.
var compareOps = map[PortType][]string{
	TInt:   {"eq", "ne", "lt", "le", "gt", "ge"},
	TFloat: {"eq", "ne", "lt", "le", "gt", "ge"},
	TStr:   {"eq", "ne", "contains", "prefix", "matches"},
	TBool:  {"eq"},
}

// comparePorts: the value goes to `yes` or to `no`, the other output gets an exit signal. The operand is checked against the
// type of the input here, so a wrong one never reaches a run.
func comparePorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p struct {
		Type    PortType        `json:"type"`
		Op      string          `json:"op"`
		Operand json.RawMessage `json:"operand"`
	}
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	ops, ok := compareOps[p.Type]
	if !ok {
		return nil, nil, errors.New("тип сравнения: int, float, str или bool")
	}
	found := false
	for _, o := range ops {
		found = found || o == p.Op
	}
	if !found {
		return nil, nil, fmt.Errorf("операция %q не подходит типу %s (есть %v)", p.Op, p.Type, ops)
	}
	if err := checkOperand(p.Type, p.Op, p.Operand); err != nil {
		return nil, nil, err
	}
	return []Port{req("in", p.Type)}, []Port{req("yes", p.Type), req("no", p.Type)}, nil
}

func checkOperand(t PortType, op string, raw json.RawMessage) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return errors.New("не задан операнд")
	}
	var err error
	switch t {
	case TInt:
		var v int64
		err = json.Unmarshal(raw, &v)
	case TFloat:
		var v float64
		err = json.Unmarshal(raw, &v)
	case TBool:
		var v bool
		err = json.Unmarshal(raw, &v)
	case TStr:
		var v string
		if err = json.Unmarshal(raw, &v); err == nil && op == "matches" {
			_, err = regexp.Compile(v) // RE2: linear time, no backtracking
		}
	}
	if err != nil {
		return fmt.Errorf("операнд не подходит типу %s: %v", t, err)
	}
	return nil
}

// anyOfPorts: any number of inputs of one type; the first (in link order) that holds a value is the output.
func anyOfPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p struct {
		Type PortType `json:"type"`
	}
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if !p.Type.value() {
		return nil, nil, errors.New("тип: int, float, str, bool или doc")
	}
	return []Port{{Name: "in", Type: p.Type, Multi: true}}, []Port{req("out", p.Type)}, nil
}

// allOfPorts: waits for every input; gives a trigger. The values stay on their own links.
func allOfPorts(raw json.RawMessage) ([]Port, []Port, error) {
	if err := strictParams(raw, &struct{}{}); err != nil {
		return nil, nil, err
	}
	return []Port{{Name: "in", Type: TAny, Multi: true}}, []Port{req("out", TAny)}, nil
}
