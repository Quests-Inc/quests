package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type constantParams struct {
	Type  PortType        `json:"type"`
	Value json.RawMessage `json:"value"`
}

func constantValue(raw json.RawMessage) (Token, error) {
	var p constantParams
	if err := strictParams(raw, &p); err != nil {
		return Token{}, err
	}
	if !p.Type.value() || len(p.Value) == 0 || len(p.Value) > MaxTextBytes || (p.Type != TDoc && bytes.Equal(bytes.TrimSpace(p.Value), []byte("null"))) {
		return Token{}, fmt.Errorf("нужны type и value до 64 КБ")
	}
	var v any
	if p.Type == TDoc {
		if !json.Valid(p.Value) {
			return Token{}, fmt.Errorf("нужен JSON")
		}
		v = p.Value
	} else {
		switch p.Type {
		case TInt:
			var n int64
			if err := json.Unmarshal(p.Value, &n); err != nil {
				return Token{}, err
			}
			v = n
		case TFloat:
			var n float64
			if bytes.Equal(bytes.TrimSpace(p.Value), []byte("null")) {
				return Token{}, fmt.Errorf("null не число")
			}
			if err := json.Unmarshal(p.Value, &n); err != nil {
				return Token{}, err
			}
			v = n
		case TStr:
			var n string
			if bytes.Equal(bytes.TrimSpace(p.Value), []byte("null")) {
				return Token{}, fmt.Errorf("null не строка")
			}
			if err := json.Unmarshal(p.Value, &n); err != nil {
				return Token{}, err
			}
			v = n
		case TBool:
			var n bool
			if bytes.Equal(bytes.TrimSpace(p.Value), []byte("null")) {
				return Token{}, fmt.Errorf("null не флаг")
			}
			if err := json.Unmarshal(p.Value, &n); err != nil {
				return Token{}, err
			}
			v = n
		}
	}
	return Token{Type: p.Type, Value: v}, nil
}
func constantPorts(raw json.RawMessage) ([]Port, []Port, error) {
	v, err := constantValue(raw)
	if err != nil {
		return nil, nil, err
	}
	return nil, []Port{req("value", v.Type)}, nil
}

type convertParams struct {
	From  PortType `json:"from"`
	To    PortType `json:"to"`
	Mode  string   `json:"mode,omitempty"`
	Round string   `json:"round,omitempty"`
}

func convertPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p convertParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if !p.From.value() || !p.To.value() {
		return nil, nil, fmt.Errorf("типы: str/int/float/bool/doc")
	}
	switch p.Mode {
	case "", "value":
	case "parse_json":
		if p.From != TStr || p.To != TDoc {
			return nil, nil, fmt.Errorf("parse_json: str → doc")
		}
	case "stringify":
		if p.From != TDoc || p.To != TStr {
			return nil, nil, fmt.Errorf("stringify: doc → str")
		}
	default:
		return nil, nil, fmt.Errorf("неизвестный режим")
	}
	switch p.Round {
	case "", "exact", "floor", "ceil", "trunc", "nearest":
	default:
		return nil, nil, fmt.Errorf("округление: exact/floor/ceil/trunc/nearest")
	}
	return []Port{req("in", p.From)}, []Port{req("value", p.To)}, nil
}
func runConvert(rs *resolved, in Token) (map[string]Token, error) {
	var p convertParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	v := in.Value
	switch p.Mode {
	case "parse_json":
		s, ok := v.(string)
		if !ok || len(s) > MaxTextBytes || !json.Valid([]byte(s)) {
			return nil, fmt.Errorf("нужна JSON-строка до 64 КБ")
		}
		v = json.RawMessage(s)
	case "stringify":
		raw, ok := v.(json.RawMessage)
		if !ok || !json.Valid(raw) {
			return nil, fmt.Errorf("нужен JSON-документ")
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err != nil {
			return nil, err
		}
		v = buf.String()
	default:
		if p.From == TDoc {
			raw, ok := v.(json.RawMessage)
			if !ok {
				return nil, fmt.Errorf("нужен документ")
			}
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.UseNumber()
			if err := dec.Decode(&v); err != nil {
				return nil, err
			}
		}
		fail := func() (map[string]Token, error) {
			return nil, fmt.Errorf("значение не приводится к %s без потери точности", p.To)
		}
		switch p.To {
		case TDoc:
			if p.From == TDoc {
				return map[string]Token{"value": in}, nil
			}
			raw, err := json.Marshal(v)
			if err != nil {
				return nil, err
			}
			v = json.RawMessage(raw)
		case TStr:
			switch x := v.(type) {
			case string:
				v = x
			case int64:
				v = strconv.FormatInt(x, 10)
			case float64:
				v = strconv.FormatFloat(x, 'g', -1, 64)
			case bool:
				v = strconv.FormatBool(x)
			case json.Number:
				v = x.String()
			default:
				return fail()
			}
		case TInt:
			switch x := v.(type) {
			case int64:
				v = x
			case string:
				n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
				if err != nil {
					return fail()
				}
				v = n
			case json.Number:
				n, err := strconv.ParseInt(x.String(), 10, 64)
				if err != nil {
					return fail()
				}
				v = n
			case float64:
				n := x
				switch p.Round {
				case "floor":
					n = math.Floor(n)
				case "ceil":
					n = math.Ceil(n)
				case "trunc":
					n = math.Trunc(n)
				case "nearest":
					n = math.Round(n)
				}
				if math.IsNaN(n) || math.IsInf(n, 0) || n >= 9223372036854775808.0 || n < -9223372036854775808.0 || math.Trunc(n) != n {
					return fail()
				}
				v = int64(n)
			default:
				return fail()
			}
		case TFloat:
			switch x := v.(type) {
			case float64:
				v = x
			case int64:
				if x > 9007199254740992 || x < -9007199254740992 {
					return fail()
				}
				v = float64(x)
			case string:
				n, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
				if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
					return fail()
				}
				v = n
			case json.Number:
				n, err := strconv.ParseFloat(x.String(), 64)
				if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
					return fail()
				}
				if i, e := strconv.ParseInt(x.String(), 10, 64); e == nil && (i > 9007199254740992 || i < -9007199254740992) {
					return fail()
				}
				v = n
			default:
				return fail()
			}
		case TBool:
			switch x := v.(type) {
			case bool:
				v = x
			case string:
				if x != "true" && x != "false" {
					return fail()
				}
				v = x == "true"
			default:
				return fail()
			}
		}
	}
	return map[string]Token{"value": {Type: p.To, Value: v}}, nil
}
