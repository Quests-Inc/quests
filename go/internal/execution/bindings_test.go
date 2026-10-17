package execution

import (
	"context"
	"errors"
	"testing"
)

func noteScript(t *testing.T, st *Store) Script {
	t.Helper()
	sc, err := st.CreateScript(context.Background(), "заметка", "", []byte(`{"nodes":[
		{"id":1,"type":"source.note"},{"id":2,"type":"sink.note_error"},{"id":3,"type":"code","params":{"text":"echo 1"}},{"id":4,"type":"parse","params":{"schema":[{"name":"message","type":"str","required":true}]}}],
		"edges":[{"from":{"node":1,"port":"ref"},"to":{"node":2,"port":"ref"}},{"from":{"node":3,"port":"failed"},"to":{"node":4,"port":"in"}},{"from":{"node":4,"port":"message"},"to":{"node":2,"port":"message"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	return sc
}

func TestBindingLifecycle(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	step := scriptGraph("echo 1")
	sc, err := st.CreateScript(ctx, "шаг", "", step)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO note (id, title, description, created_at, updated_at) VALUES (60, 'N', '', now(), now())`); err != nil {
		t.Fatal(err)
	}

	if err := st.BindScript(ctx, SubjectStep, 1000, sc.ID); err != nil {
		t.Fatal(err)
	}
	b, err := st.BindingFor(ctx, SubjectStep, 1000)
	if err != nil || b == nil || b.ScriptID != sc.ID || b.Version != 1 || !b.NeedsRunner {
		t.Fatalf("binding: %+v %v", b, err)
	}
	if none, _ := st.BindingFor(ctx, SubjectStep, 999); none != nil {
		t.Fatalf("an unbound step has no binding: %+v", none)
	}
	all, _ := st.Bindings(ctx, SubjectStep)
	if len(all) != 1 || all[1000].ScriptID != sc.ID {
		t.Fatalf("bindings: %+v", all)
	}

	// A script reads one kind of entity: a note script does not fit a step, and the other way round.
	ns := noteScript(t, st)
	if err := st.BindScript(ctx, SubjectStep, 1000, ns.ID); err == nil {
		t.Fatal("a note script must not bind to a step")
	}
	if err := st.BindScript(ctx, SubjectNote, 60, sc.ID); err == nil {
		t.Fatal("a step script must not bind to a note")
	}
	if err := st.BindScript(ctx, "quest", 1, sc.ID); err == nil {
		t.Fatal("only steps and notes take a script")
	}
	if err := st.BindScript(ctx, SubjectStep, 1000, 404); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an unknown script: %v", err)
	}
	if b, _ := st.BindingFor(ctx, SubjectStep, 1000); b.ScriptID != sc.ID {
		t.Fatal("a refused bind must leave the binding as it was")
	}

	// Binding again replaces; a script in use cannot be deleted, and says where.
	sc2, _ := st.CreateScript(ctx, "шаг 2", "", scriptGraph("echo 2"))
	if err := st.BindScript(ctx, SubjectStep, 1000, sc2.ID); err != nil {
		t.Fatal(err)
	}
	var inUse *ScriptInUse
	if err := st.DeleteScript(ctx, sc2.ID); !errors.As(err, &inUse) || len(inUse.Uses) != 1 || inUse.Uses[0].SubjectID != 1000 || inUse.Uses[0].Title != "step" {
		t.Fatalf("delete a bound script: %v", err)
	}
	if err := st.DeleteScript(ctx, sc.ID); err != nil {
		t.Fatalf("the replaced script is free: %v", err)
	}
	if uses, _ := st.ScriptUses(ctx, ns.ID); len(uses) != 0 {
		t.Fatalf("%v", uses)
	}

	if err := st.UnbindScript(ctx, SubjectStep, 1000); err != nil {
		t.Fatal(err)
	}
	if err := st.UnbindScript(ctx, SubjectStep, 1000); err != nil {
		t.Fatalf("unbinding twice: %v", err)
	}
	if err := st.DeleteScript(ctx, sc2.ID); err != nil {
		t.Fatalf("an unbound script can go: %v", err)
	}
}

func TestOrphanBindingsAreSwept(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	sc, _ := st.CreateScript(ctx, "шаг", "", scriptGraph("echo 1"))
	if err := st.BindScript(ctx, SubjectStep, 1000, sc.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := st.PurgeOrphanBindings(ctx); err != nil || n != 0 {
		t.Fatalf("a live binding stays: %d %v", n, err)
	}
	if _, err := st.DB.Exec(`DELETE FROM queststep WHERE id = 1000`); err != nil {
		t.Fatal(err)
	}
	if n, err := st.PurgeOrphanBindings(ctx); err != nil || n != 1 {
		t.Fatalf("the binding of a deleted step goes: %d %v", n, err)
	}
	if err := st.DeleteScript(ctx, sc.ID); err != nil {
		t.Fatalf("and frees the script: %v", err)
	}
}
