package knowledge

import (
	"context"
	"errors"
	"testing"

	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func trashStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	pool, _ := pgtest.NewWithURL(t)
	return &Store{DB: pool}, wsctx.With(context.Background(), 1)
}

func newNote(t *testing.T, st *Store, ctx context.Context, title string, parent *int64) Note {
	t.Helper()
	n, err := st.CreateNote(ctx, Note{Title: title, Description: "text of " + title, ParentID: parent})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func parentOf(t *testing.T, st *Store, ctx context.Context, id int64) *int64 {
	t.Helper()
	n, err := st.GetNote(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return n.ParentID
}

func TestTrashNoteHidesAndRestoreRebuildsTheTree(t *testing.T) {
	st, ctx := trashStore(t)
	root := newNote(t, st, ctx, "root", nil)
	mid := newNote(t, st, ctx, "mid", &root.ID)
	kid1 := newNote(t, st, ctx, "kid1", &mid.ID)
	kid2 := newNote(t, st, ctx, "kid2", &mid.ID)

	if err := st.TrashNote(ctx, mid.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetNote(ctx, mid.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a trashed note must not be readable: %v", err)
	}
	if ok, _ := st.NoteExists(ctx, mid.ID); ok {
		t.Fatal("a trashed note must not exist for parent validation")
	}
	all, err := st.ListNotes(ctx, NoteFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range all {
		if n.ID == mid.ID {
			t.Fatal("the list must not show the trashed note")
		}
	}
	// the children were lifted to the root, like a hard delete does
	if parentOf(t, st, ctx, kid1.ID) != nil || parentOf(t, st, ctx, kid2.ID) != nil {
		t.Fatal("children must be lifted to the root while the parent is in the trash")
	}

	trash, err := st.ListTrashedNotes(ctx)
	if err != nil || len(trash) != 1 || trash[0].ID != mid.ID || trash[0].Children != 2 {
		t.Fatalf("trash list: %+v %v", trash, err)
	}

	// one child was moved elsewhere meanwhile: only the one still at the root comes back
	other := newNote(t, st, ctx, "other", nil)
	if _, err := st.UpdateNote(ctx, kid2.ID, map[string]any{"parent_id": other.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RestoreNote(ctx, mid.ID); err != nil {
		t.Fatal(err)
	}
	if p := parentOf(t, st, ctx, mid.ID); p == nil || *p != root.ID {
		t.Fatalf("the note goes back under its parent, got %v", p)
	}
	if p := parentOf(t, st, ctx, kid1.ID); p == nil || *p != mid.ID {
		t.Fatalf("kid1 goes back under the note, got %v", p)
	}
	if p := parentOf(t, st, ctx, kid2.ID); p == nil || *p != other.ID {
		t.Fatalf("kid2 stays where the user put it, got %v", p)
	}
}

func TestRestoreNoteWhoseParentIsGoneGoesToTheRoot(t *testing.T) {
	st, ctx := trashStore(t)
	parent := newNote(t, st, ctx, "parent", nil)
	child := newNote(t, st, ctx, "child", &parent.ID)
	if err := st.TrashNote(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.TrashNote(ctx, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RestoreNote(ctx, child.ID); err != nil {
		t.Fatal(err)
	}
	if p := parentOf(t, st, ctx, child.ID); p != nil {
		t.Fatalf("the parent is still in the trash: the note must go to the root, got %v", *p)
	}
}

func TestPurgeNoteOnlyTrashed(t *testing.T) {
	st, ctx := trashStore(t)
	live, dead := newNote(t, st, ctx, "live", nil), newNote(t, st, ctx, "dead", nil)
	if err := st.PurgeNote(ctx, live.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a live note cannot be purged: %v", err)
	}
	if err := st.TrashNote(ctx, dead.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.PurgeNote(ctx, dead.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.ListTrashedNotes(ctx); len(left) != 0 {
		t.Fatalf("purged note still listed: %+v", left)
	}
	if _, err := st.GetNote(ctx, live.ID); err != nil {
		t.Fatal(err)
	}
}
