package kernel

import (
	"reflect"
	"strings"
	"testing"
)

func p(s string) *string { return &s }

func TestValidRunOn(t *testing.T) {
	good := []*string{nil, p("laptop"), p("a,b"), p("main-pc,laptop-n95,x_1")}
	for _, v := range good {
		if !ValidRunOn(v) {
			t.Errorf("%v should be valid", v)
		}
	}
	bad := []string{"", "A", "a,,b", "a,a", "a b", ",a", "a,", strings.Repeat("a,", 8) + "a", strings.Repeat("x", 33)}
	for _, v := range bad {
		if ValidRunOn(&v) {
			t.Errorf("%q should be invalid", v)
		}
	}
	nine := "a,b,c,d,e,f,g,h,i"
	if ValidRunOn(&nine) {
		t.Error("more than MaxRunOnLabels must be refused")
	}
}

func TestNormalizeRunOn(t *testing.T) {
	cases := []struct {
		in   *string
		want string
	}{
		{p("  Laptop "), "laptop"},
		{p("Main-PC , laptop,, MAIN-pc"), "main-pc,laptop"},
		{p("a,b"), "a,b"},
	}
	for _, c := range cases {
		got := NormalizeRunOn(c.in)
		if got == nil || *got != c.want {
			t.Errorf("Normalize(%q)=%v want %q", *c.in, got, c.want)
		}
	}
	for _, e := range []*string{nil, p(""), p("  , ")} {
		if NormalizeRunOn(e) != nil {
			t.Errorf("empty must stay nil: %v", e)
		}
	}
}

func TestSplitRunOn(t *testing.T) {
	if got := SplitRunOn("a, b ,,c"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("%v", got)
	}
	if SplitRunOn("") != nil {
		t.Fatal("empty list expected")
	}
}
