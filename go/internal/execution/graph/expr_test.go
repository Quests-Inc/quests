package graph

import "testing"

func TestEvalExpr(t *testing.T) {
	vars := map[string]float64{"a": 3, "b": 4}
	ok := map[string]float64{
		"5+19": 24, "2 + 3*4": 14, "(2+3)*4": 20, "-a + b": 1, "a*b % 5": 2, "max(1, a, b) - min(a, b)": 1,
		"floor(7/2)": 3, "round(2.5)": 3, "abs(-4)": 4, ".5+.5": 1,
	}
	for s, want := range ok {
		got, err := EvalExpr(s, vars)
		if err != nil || got != want {
			t.Errorf("%q = %v, %v; want %v", s, got, err, want)
		}
	}
	bad := []string{"", "1/0", "1 +", "(1", "1)", "x", "foo(1)", "1 2", "floor(1,2)", "5+3к8",
		"((((((((((((((((((((((((((((((((((1))))))))))))))))))))))))))))))))))"}
	for _, s := range bad {
		if v, err := EvalExpr(s, vars); err == nil {
			t.Errorf("%q must fail, got %v", s, v)
		}
	}
}
