package graph

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Build templates substitute values, never executable expressions or JSON fragments.
// A JSON placeholder is an entire object {"$input":"name"}; all other values are literals.
type buildParams struct {
	Inputs   []ArgParam      `json:"inputs"`
	Template json.RawMessage `json:"template"`
}

var textSlot = regexp.MustCompile(`\{\{([a-z][a-z0-9_]{0,23})\}\}`)

func buildPorts(raw json.RawMessage, text bool) ([]Port, []Port, error) {
	var p buildParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if len(p.Inputs) > MaxArgsParams || len(p.Template) == 0 || len(p.Template) > MaxTextBytes {
		return nil, nil, fmt.Errorf("шаблон обязателен, до 64 КБ; входов до %d", MaxArgsParams)
	}
	in := []Port{}
	names := map[string]bool{}
	for _, a := range p.Inputs {
		if !embedName.MatchString(a.Name) || names[a.Name] || !a.Type.value() {
			return nil, nil, fmt.Errorf("неверный вход %q или тип", a.Name)
		}
		names[a.Name] = true
		in = append(in, req(a.Name, a.Type))
	}
	var v any
	if err := json.Unmarshal(p.Template, &v); err != nil {
		return nil, nil, err
	}
	if text {
		s, ok := v.(string)
		if !ok {
			return nil, nil, fmt.Errorf("текстовый шаблон должен быть строкой")
		}
		for _, m := range textSlot.FindAllStringSubmatch(s, -1) {
			if !names[m[1]] {
				return nil, nil, fmt.Errorf("неизвестный вход %q", m[1])
			}
		}
	} else if err := checkSlots(v, names, 0); err != nil {
		return nil, nil, err
	}
	typ := TDoc
	if text {
		typ = TStr
	}
	return in, []Port{req("value", typ)}, nil
}
func checkSlots(v any, names map[string]bool, depth int) error {
	if depth > 32 {
		return fmt.Errorf("вложенность шаблона больше 32")
	}
	switch x := v.(type) {
	case map[string]any:
		if name, ok := x["$input"]; ok {
			s, valid := name.(string)
			if len(x) != 1 || !valid || !names[s] {
				return fmt.Errorf("неверная подстановка $input")
			}
			return nil
		}
		for _, a := range x {
			if err := checkSlots(a, names, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, a := range x {
			if err := checkSlots(a, names, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
func inputJSON(t Token) (json.RawMessage, error) {
	if t.Type == TDoc {
		v, ok := t.Value.(json.RawMessage)
		if !ok || !json.Valid(v) {
			return nil, fmt.Errorf("вход не JSON")
		}
		return v, nil
	}
	return json.Marshal(t.Value)
}

// ResultText preserves strings and serializes typed values as JSON.
func ResultText(t Token) (string, error) {
	if s, ok := t.Value.(string); ok {
		return s, nil
	}
	raw, err := inputJSON(t)
	return string(raw), err
}
func runBuild(rs *resolved, ins map[string][]Token) (map[string]Token, error) {
	var p buildParams
	_ = strictParams(rs.node.Params, &p)
	values := map[string]json.RawMessage{}
	for _, a := range p.Inputs {
		raw, err := inputJSON(ins[a.Name][0])
		if err != nil {
			return nil, err
		}
		values[a.Name] = raw
	}
	var raw []byte
	typ := TDoc
	if rs.node.Type == "text.build" {
		var s string
		_ = json.Unmarshal(p.Template, &s)
		s = textSlot.ReplaceAllStringFunc(s, func(slot string) string {
			name := textSlot.FindStringSubmatch(slot)[1]
			v := values[name]
			var str string
			if json.Unmarshal(v, &str) == nil {
				return str
			}
			return string(v)
		})
		raw = []byte(s)
		typ = TStr
	} else {
		var v any
		dec := json.NewDecoder(strings.NewReader(string(p.Template)))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		var replace func(any) any
		replace = func(v any) any {
			switch x := v.(type) {
			case map[string]any:
				if name, ok := x["$input"].(string); ok {
					return values[name]
				}
				for k, a := range x {
					x[k] = replace(a)
				}
			case []any:
				for i, a := range x {
					x[i] = replace(a)
				}
			}
			return v
		}
		var err error
		raw, err = json.Marshal(replace(v))
		if err != nil {
			return nil, err
		}
	}
	if len(raw) > MaxTextBytes {
		return nil, fmt.Errorf("результат больше 64 КБ")
	}
	var value any = json.RawMessage(raw)
	if typ == TStr {
		value = string(raw)
	}
	return map[string]Token{"value": {Type: typ, Value: value}}, nil
}
