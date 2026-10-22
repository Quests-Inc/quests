package graph

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestExprScopeFillsNamesAndPortsWin(t *testing.T) {
	got, err := EvaluateScoped(context.Background(), DiceScript(), Builtin(), map[string]string{"_": "1+str"}, &ExprScope{Num: map[string]float64{"str": 3}})
	if err != nil || got != "4" {
		t.Fatalf("scope: %q %v", got, err)
	}
	rs := &resolved{node: Node{Params: json.RawMessage(`{"vars":["str"]}`)}}
	out, err := runExpr(rs, map[string][]Token{
		"expr": {{Type: TStr, Value: "str+1"}},
		"str":  {{Type: TFloat, Value: 10.0}},
	}, &budget{}, &ExprScope{Num: map[string]float64{"str": 3}})
	if err != nil || out["text"].Value != "11" {
		t.Fatalf("port should win: %+v %v", out, err)
	}
}

func TestExprScopeStringAndForeign(t *testing.T) {
	_, err := EvaluateScoped(context.Background(), DiceScript(), Builtin(), map[string]string{"_": "1+repo"}, &ExprScope{Str: map[string]bool{"repo": true}})
	if err == nil || !strings.Contains(err.Error(), "строка") {
		t.Fatal(err)
	}
	scope := &ExprScope{Foreign: func(kind string, id int64, name string) (float64, bool, bool, error) {
		if kind == "note" && id == 4 && name == "str" {
			return 5, false, true, nil
		}
		return 0, false, false, nil
	}}
	got, err := EvaluateScoped(context.Background(), DiceScript(), Builtin(), map[string]string{"_": "1+note=4.str"}, scope)
	if err != nil || got != "6" {
		t.Fatalf("foreign: %q %v", got, err)
	}
}
