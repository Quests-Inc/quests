package httpapi

import (
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"testing"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/store"
)

func TestGraphBatchSharesCardsAndKeepsIndependentTreeMembership(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	ctx := wsctx.With(t.Context(), 1)
	qid := seedQuest(t, s)
	parent, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Root", Description: "quest=" + strconv.FormatInt(qid, 10)})
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Child", ParentID: &parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	roots := []store.GraphTreeRequest{{Type: "note", ID: parent.ID, Depth: 1}, {Type: "note", ID: child.ID, Depth: 2}}
	batch, err := s.Store.BuildGraphBatch(ctx, roots, nil)
	if err != nil {
		t.Fatal(err)
	}
	union := map[string]bool{}
	for i, root := range roots {
		one, err := s.Store.BuildGraph(ctx, root.Type, root.ID, root.Depth)
		if err != nil {
			t.Fatal(err)
		}
		keys := []string{}
		for _, node := range one.Nodes {
			keys = append(keys, node.Key)
			union[node.Key] = true
		}
		got := append([]string{}, batch.Trees[i].Nodes...)
		sort.Strings(keys)
		sort.Strings(got)
		if !reflect.DeepEqual(keys, got) || batch.Trees[i].Root != one.Root || batch.Trees[i].Depth != one.Depth {
			t.Fatalf("tree %d differs: %v / %v", i, keys, got)
		}
	}
	if len(batch.Nodes) != len(union) {
		t.Fatalf("cards were duplicated: %d / %d", len(batch.Nodes), len(union))
	}
	for _, node := range batch.Nodes {
		if !union[node.Key] {
			t.Fatal(node.Key)
		}
		delete(union, node.Key)
	}
	// RLS and trash filtering still apply to the one shared corpus.
	other, err := s.Store.BuildGraphBatch(wsctx.With(t.Context(), 2), roots, nil)
	if err != nil || len(other.Nodes) != 0 {
		t.Fatalf("cross-workspace leak: %+v %v", other, err)
	}
	if err := s.Store.Knowledge.TrashNote(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	batch, err = s.Store.BuildGraphBatch(ctx, roots, nil)
	if err != nil || len(batch.Trees[1].Nodes) != 0 {
		t.Fatalf("trashed root: %+v %v", batch, err)
	}
	for _, query := range []string{"", "note:1", "bad:1:2", "note:x:2", "note:1:x"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/graph?batch="+query, nil))
		if w.Code != 400 {
			t.Fatalf("invalid batch %q: %d", query, w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/graph?batch=note:"+strconv.FormatInt(parent.ID, 10)+":1", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
