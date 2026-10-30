package mcp

import (
	"strings"
	"testing"
)

func TestGraphLayoutPreviewAndConfirmation(t *testing.T) {
	f := &fakeAPI{routes: map[string]string{
		"GET /api/graph-layouts/7":   `{"id":7,"title":"Работа","state":{"entities":[{"key":"quest:1","title":"Было"}]}}`,
		"PATCH /api/graph-layouts/7": `{"id":7,"title":"Работа"}`,
	}}
	s, _ := newTestServer(t, f)
	proposed := `"state":{"entities":[{"key":"note:2","title":"Стало"}],"trees":{"note:2":{"depth":3,"layout":"radial"}},"zones":[{"id":"a","title":"Зона","color":"#6a8fbf","x":0,"y":0,"w":500,"h":300}]}`
	res := rpc(t, s, call(1, "graph_layout", `{"action":"update","id":7,`+proposed+`}`))
	text, isErr := result(t, res[0])
	if isErr || !strings.Contains(text, `"requires_confirmation":true`) || !strings.Contains(text, `"key":"quest:1"`) || !strings.Contains(text, `"key":"note:2"`) {
		t.Fatalf("bad preview: %s", text)
	}
	for _, r := range f.seen {
		if strings.HasPrefix(r, "PATCH ") {
			t.Fatal("preview wrote state")
		}
	}
	res = rpc(t, s, call(2, "graph_layout", `{"action":"update","id":7,"confirm":true,`+proposed+`}`))
	if text, isErr = result(t, res[0]); isErr {
		t.Fatal(text)
	}
	if !strings.HasPrefix(f.seen[len(f.seen)-1], "PATCH /api/graph-layouts/7") {
		t.Fatal(f.seen)
	}
	res = rpc(t, s, call(3, "graph_layout", `{"action":"delete","id":7}`))
	if text, isErr = result(t, res[0]); isErr || !strings.Contains(text, `"requires_confirmation":true`) {
		t.Fatal(text)
	}
	for _, r := range f.seen {
		if strings.HasPrefix(r, "DELETE ") {
			t.Fatal("preview deleted layout")
		}
	}
}

func TestGraphLayoutLegacyDiff(t *testing.T) {
	before := map[string]any{"positions": map[string]any{"note:2": map[string]any{}, "note:3": map[string]any{}, "cluster:quest:1": map[string]any{}}, "hidden": []string{"note:3"}}
	after := map[string]any{"entities": []layoutEntity{{"quest:1", "Квест"}, {"note:t1", "Черновик"}}}
	added, removed, legacy := graphLayoutDiff(before, after)
	if !legacy || len(added) != 1 || added[0].Key != "quest:1" || len(removed) != 1 || removed[0].Key != "note:2" {
		t.Fatalf("%v %v legacy=%v", added, removed, legacy)
	}
	empty := map[string]any{"entities": []layoutEntity{}, "positions": map[string]any{"note:2": map[string]any{}}}
	entities, legacy := graphLayoutEntities(empty)
	if legacy || len(entities) != 0 {
		t.Fatal("explicit empty manifest used legacy positions")
	}
}
