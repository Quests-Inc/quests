package graph

import (
	"context"
	"strings"
	"testing"
)

func TestParseHeads(t *testing.T) {
	scope, err := ParseHeads("note=530+\nnote=540, note=530, note=531+2, note=532+!, note=533+1")
	if err != nil {
		t.Fatal(err)
	}
	want := []Head{{530, 1}, {540, 0}, {531, 2}, {532, HeadAll}, {533, 1}}
	if len(scope.Heads) != len(want) {
		t.Fatalf("heads: %+v", scope.Heads)
	}
	for i, h := range want {
		if scope.Heads[i] != h {
			t.Fatalf("head %d: %+v", i, scope.Heads[i])
		}
	}
	for _, raw := range []string{"", "530+", "note=0", "note=12++", "note=12 +", "note=12+0", "note=12+33", "note=12+!x"} {
		if _, err := ParseHeads(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestNotesByRowMatchesHeads(t *testing.T) {
	card := func(classes string) string {
		return "<!-- table: left -->\n<!-- table-id: spell -->\n| | |\n|---|---|\n| <!-- row-id: time -->Время | 1 действие |\n| <!-- row-id: classes -->Классы | " + classes + " |\n"
	}
	notes := []TableNote{
		{ID: 10, Title: "Огненный шар", Description: card("note=7, note=8"), ParentTitle: "3 уровень", ParentSort: 3},
		{ID: 11, Title: "Лечение", Description: card("note=70"), ParentTitle: "1 уровень", ParentSort: 1},
		{ID: 12, Title: "Битая", Description: "<!-- table-id: spell -->\n| | |\n|---|---|\n| Время | 1 |", ParentTitle: "1 уровень", ParentSort: 1},
	}
	ctx := WithHostNote(WithTableNotes(context.Background(), func(_ context.Context, table string, scope HeadScope) ([]TableNote, error) {
		if table != "spell" || len(scope.Heads) != 1 || scope.Heads[0].ID != 530 || scope.Heads[0].Depth != 1 {
			t.Fatalf("query %s %+v", table, scope)
		}
		return notes, nil
	}), 7)
	g := buildGraph([]nodeSpec{
		{1, "source.note", nil},
		{2, "notes.by_row", kv{"table": "spell", "row": "classes", "heads": "note=530+"}},
		{3, "sink.result", nil},
	}, [4]any{1, "ref", 2, "ref"}, [4]any{2, "value", 3, "value"})
	reader := func(_ context.Context, id int64) (string, string, error) {
		if id != 7 {
			t.Fatal(id)
		}
		return "Волшебник", "", nil
	}
	out, err := EvaluateFetch(ctx, g, Builtin(), nil, nil, nil, reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "### 3 уровень") || !strings.Contains(out, "- note=10") {
		t.Fatal(out)
	}
	if strings.Contains(out, "Лечение") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "### Не разобраны") || !strings.Contains(out, "- note=12") {
		t.Fatal(out)
	}
}
