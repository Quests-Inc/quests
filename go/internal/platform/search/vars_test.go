package search

import "testing"

func TestNormalizeNoteVars(t *testing.T) {
	got := NormalizeNoteVars(Request{Query: "огонь var:hp=12 -var:slot var:=", Types: []string{"note"}})
	if got.Query != "огонь var:=" {
		t.Fatalf("query %q", got.Query)
	}
	if len(got.Filters) != 2 || got.Filters[0].Kind != "note" || got.Filters[0].Key != "var" || len(got.Filters[0].Values) != 1 || got.Filters[0].Values[0] != "hp=12" {
		t.Fatalf("positive: %+v", got.Filters)
	}
	if !got.Filters[1].Not || got.Filters[1].Values[0] != "slot" {
		t.Fatalf("negative: %+v", got.Filters[1])
	}
	again := NormalizeNoteVars(got)
	if len(again.Filters) != 2 || again.Query != got.Query {
		t.Fatalf("second pass changed the request: %+v %q", again.Filters, again.Query)
	}
	scoped := NormalizeNoteVars(Request{Filters: []Filter{{Key: "var", Values: []string{"ac"}}}})
	if scoped.Filters[0].Kind != "note" {
		t.Fatalf("kind: %+v", scoped.Filters[0])
	}
}
