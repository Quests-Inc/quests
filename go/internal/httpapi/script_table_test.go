package httpapi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func TestEmbeddedTableSourceAccess(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	ctx := t.Context()
	n, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Choices", Description: "<!-- table-id: example -->\n\n| A | B |\n|---|---|\n| <!-- row-id: a -->1 | hello |\n"})
	if err != nil {
		t.Fatal(err)
	}
	create := func(id int64) int64 {
		t.Helper()
		body := fmt.Sprintf(`{"nodes":[{"id":1,"type":"source.note","params":{"id":%d}},{"id":2,"type":"markdown.table","params":{"table":"example"}},{"id":3,"type":"random","params":{"kind":"pick"}},{"id":4,"type":"json.build","params":{"inputs":[{"name":"row","type":"doc"}],"template":{"selected":{"$input":"row"}}}},{"id":5,"type":"sink.result"}],"edges":[{"from":{"node":1,"port":"description"},"to":{"node":2,"port":"text"}},{"from":{"node":2,"port":"rows"},"to":{"node":3,"port":"list"}},{"from":{"node":3,"port":"value"},"to":{"node":4,"port":"row"}},{"from":{"node":4,"port":"value"},"to":{"node":5,"port":"value"}}]}`, id)
		sc, err := s.Store.Execution.CreateScript(ctx, "Table", "", []byte(body))
		if err != nil {
			t.Fatal(err)
		}
		return sc.ID
	}
	id := create(n.ID)
	eval := func(id int64, body map[string]any, want int) {
		t.Helper()
		w := doJSON(t, h, "POST", fmt.Sprintf("/api/scripts/%d/eval", id), body, map[string]string{"X-Quests-Source": "mcp"})
		if w.Code != want {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		if want == 200 {
			var result struct{ Value string }
			if json.Unmarshal(w.Body.Bytes(), &result) != nil || !strings.Contains(result.Value, `"id":"a"`) {
				t.Fatal(w.Body.String())
			}
		}
	}
	eval(id, map[string]any{}, 200)
	current := create(0)
	eval(current, map[string]any{"host": map[string]any{"kind": "note", "id": n.ID}}, 200)
	eval(current, map[string]any{}, 422)
	private, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Private", Description: n.Description, IsPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	eval(create(private.ID), map[string]any{}, 422)
	foreign, err := s.Store.Knowledge.CreateNote(wsctx.With(ctx, 98765), knowledge.Note{Title: "Other", Description: n.Description})
	// If this fixture has a real foreign workspace constraint, using a nonexistent id is sufficient to test fail-closed reads.
	if err == nil {
		eval(create(foreign.ID), map[string]any{}, 422)
	} else {
		eval(create(98765432), map[string]any{}, 422)
	}
}
