package execution

import (
	"strings"
	"testing"
)

func formOut(t *testing.T, typ, formula string, env ...StoredValue) StoredValue {
	t.Helper()
	rows := []StoredValue{{Name: "out", Type: typ, Formula: formula}}
	rows = append(rows, env...)
	got := EvalValues(rows)
	for _, row := range got {
		if row.Name == "out" {
			return row
		}
	}
	t.Fatal("missing out")
	return StoredValue{}
}

func TestFormulaAbilityModifier(t *testing.T) {
	score := StoredValue{Name: "score", Type: "int", Value: "10"}
	for _, tc := range []struct{ score, mod string }{
		{"1", "-5"}, {"8", "-1"}, {"9", "-1"}, {"10", "0"}, {"11", "0"}, {"12", "1"}, {"15", "2"},
	} {
		score.Value = tc.score
		got := formOut(t, "int", "(score - 10) / 2", score)
		if got.Error != "" || got.Value != tc.mod {
			t.Fatalf("score %s: %q err %s", tc.score, got.Value, got.Error)
		}
	}
}

func TestFormulaOperations(t *testing.T) {
	num := StoredValue{Name: "n", Type: "int", Value: "3"}
	flag := StoredValue{Name: "flag", Type: "bool", Value: "true"}
	word := StoredValue{Name: "word", Type: "str", Value: "волк"}
	cases := []struct {
		typ, formula, want string
		env                []StoredValue
	}{
		{"int", "1 + 2 * 3", "7", nil},
		{"int", "(1 + 2) * 3", "9", nil},
		{"int", "-5 % 2", "1", nil},
		{"int", "10 - -2", "12", nil},
		{"int", "n * 4", "12", []StoredValue{num}},
		{"float", "1 / 2.0", "0.5", nil},
		{"float", "n + 0.5", "3.5", []StoredValue{num}},
		{"bool", "!flag", "false", []StoredValue{flag}},
		{"bool", "true || false && false", "true", nil},
		{"bool", "n < 10", "true", []StoredValue{num}},
		{"bool", "flag == false", "false", []StoredValue{flag}},
		{"str", `word + " и " + "лес"`, "волк и лес", []StoredValue{word}},
		{"str", `"a\"b"`, `a"b`, nil},
	}
	for _, tc := range cases {
		got := formOut(t, tc.typ, tc.formula, tc.env...)
		if got.Error != "" || got.Value != tc.want {
			t.Fatalf("%s: %q err %s", tc.formula, got.Value, got.Error)
		}
	}
}

func TestFormulaRejects(t *testing.T) {
	a := StoredValue{Name: "a", Type: "int", Formula: "b + 1"}
	b := StoredValue{Name: "b", Type: "int", Formula: "a + 1"}
	got := EvalValues([]StoredValue{a, b})
	for _, row := range got {
		if !strings.Contains(row.Error, "зациклено") {
			t.Fatalf("cycle: %+v", row)
		}
	}
	cases := []struct{ typ, formula, part string }{
		{"int", "missing + 1", "неизвестное"},
		{"int", "1 / 0", "деление на ноль"},
		{"int", `"x"`, "строку"},
		{"int", "1 + 2.0", "дробное"},
		{"str", `1 + "a"`, "два числа или две строки"},
		{"bool", "!1", "да/нет"},
		{"int", "(1", "скобка"},
		{"int", "1000000000 * 2", "диапазона"},
	}
	for _, tc := range cases {
		got := formOut(t, tc.typ, tc.formula)
		if !strings.Contains(got.Error, tc.part) {
			t.Fatalf("%s: %q", tc.formula, got.Error)
		}
	}
}
