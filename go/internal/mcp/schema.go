package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Typed builds a Tool whose argument schema is derived from A's fields. Field tags:
//
//	json:"name,omitempty"   omitempty or a pointer type = optional
//	desc:"…"                the argument's description
//	enum:"a,b,c"            allowed string values
//	min:"1" max:"100"       numeric bounds
func Typed[A any](name, desc string, run func(ctx context.Context, a A) (any, error)) Tool {
	var zero A
	return Tool{
		Name: name, Description: desc, Schema: objectSchema(reflect.TypeOf(zero)),
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var a A
			dec := json.NewDecoder(strings.NewReader(string(raw)))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&a); err != nil {
				return nil, fmt.Errorf("400: bad arguments: %v", err)
			}
			return run(ctx, a)
		},
	}
}

func objectSchema(t reflect.Type) map[string]any {
	props := map[string]any{}
	var required []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Anonymous && f.Tag.Get("json") == "" && f.Type.Kind() == reflect.Struct {
			// encoding/json flattens embedded structs, so does the schema.
			sub := objectSchema(f.Type)
			for k, v := range sub["properties"].(map[string]any) {
				props[k] = v
			}
			if r, ok := sub["required"].([]string); ok {
				required = append(required, r...)
			}
			continue
		}
		if !f.IsExported() {
			continue
		}
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		s := typeSchema(f.Type)
		if d := f.Tag.Get("desc"); d != "" {
			s["description"] = d
		}
		if e := f.Tag.Get("enum"); e != "" {
			s["enum"] = strings.Split(e, ",")
		}
		if v := f.Tag.Get("min"); v != "" {
			s["minimum"] = atoiTag(v)
		}
		if v := f.Tag.Get("max"); v != "" {
			s["maximum"] = atoiTag(v)
		}
		props[name] = s
		if f.Type.Kind() != reflect.Pointer && !strings.Contains(opts, "omitempty") {
			required = append(required, name)
		}
	}
	out := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func typeSchema(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.Pointer:
		return typeSchema(t.Elem())
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": typeSchema(t.Elem())}
	case reflect.Struct:
		return objectSchema(t)
	default: // interface{}, maps: anything goes
		return map[string]any{}
	}
}

func atoiTag(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}
