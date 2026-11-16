package httpapi

import (
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/refs"
	"net/http/httptest"
	"testing"
)

func TestBatchNoteRefTitlesRespectVisibilityAndOrder(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	ctx := wsctx.With(t.Context(), 1)
	n, err := s.Store.Knowledge.CreateNote(ctx, knowledge.Note{Title: "Visible", Description: "body is not needed"})
	if err != nil {
		t.Fatal(err)
	}
	list := []refs.Ref{{Kind: "note", ID: n.ID}, {Kind: "note", ID: 999999}, {Kind: "note", ID: n.ID}}
	check := func(workspace int64, visible bool) {
		t.Helper()
		req := httptest.NewRequest("GET", "/", nil).WithContext(wsctx.With(t.Context(), workspace))
		out := s.resolveRefs(req, list)
		if len(out) != 3 || out[0].ID != n.ID || out[1].ID != 999999 || out[2].ID != n.ID {
			t.Fatal(out)
		}
		for _, i := range []int{0, 2} {
			if visible && (out[i].Title == nil || *out[i].Title != "Visible") {
				t.Fatal(out)
			}
			if !visible && out[i].Title != nil {
				t.Fatal(out)
			}
		}
		if out[1].Title != nil {
			t.Fatal(out)
		}
	}
	check(1, true)
	check(2, false)
	if err := s.Store.Knowledge.TrashNote(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	check(1, false)
}
