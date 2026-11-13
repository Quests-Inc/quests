package httpapi

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestMapDraftsAreIsolatedByLayout(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	create := func(scope, title string) int64 {
		w := doJSON(t, h, "POST", "/api/graph-draft/ops?layout="+scope, map[string]any{"kind": "create_note", "payload": map[string]any{"fields": map[string]any{"title": title}}}, nil)
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		var op struct{ ID int64 }
		_ = json.Unmarshal(w.Body.Bytes(), &op)
		return op.ID
	}
	a := create("boardA", "A")
	_ = create("boardB", "B")
	read := func(scope string) []map[string]any {
		w := doJSON(t, h, "GET", "/api/graph-draft?layout="+scope, nil, nil)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
		var out struct{ Ops []map[string]any }
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return out.Ops
	}
	if len(read("boardA")) != 1 || len(read("boardB")) != 1 || len(read("")) != 0 {
		t.Fatal("shared draft")
	}
	w := doJSON(t, h, "DELETE", fmt.Sprintf("/api/graph-draft/ops/%d?layout=boardB", a), nil, nil)
	if w.Code == 200 && len(read("boardA")) != 1 {
		t.Fatal("cross-layout delete")
	}
	w = doJSON(t, h, "POST", "/api/graph-draft/commit?layout=boardA", nil, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if len(read("boardA")) != 0 || len(read("boardB")) != 1 {
		t.Fatal("commit crossed layout boundary")
	}
	w = doJSON(t, h, "PUT", "/api/graph-draft/roots?layout=boardA", map[string]any{"roots": []map[string]any{{"type": "note", "id": 1}}}, nil)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	other := doJSON(t, h, "GET", "/api/graph-draft?layout=boardB", nil, nil)
	var out struct{ Roots []any }
	_ = json.Unmarshal(other.Body.Bytes(), &out)
	if len(out.Roots) != 0 {
		t.Fatal("roots crossed scope")
	}
}
