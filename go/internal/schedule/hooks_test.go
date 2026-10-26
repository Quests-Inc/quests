package schedule

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/store"
)

func TestDispatchHooksQueuesAJob(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	hub := events.New()
	r := NewCheckRunner(st, hub)

	graph := []byte(`{"nodes":[
		{"id":1,"type":"source.quest"},
		{"id":2,"type":"code","params":{"text":"echo ok"}},
		{"id":3,"type":"sink.done"}],
	 "edges":[
		{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},
		{"from":{"node":2,"port":"stdout"},"to":{"node":3,"port":"trigger"}}]}`)
	sc, err := st.Execution.CreateScript(ctx, "хук", "", graph)
	if err != nil {
		t.Fatal(err)
	}
	qid := pgtest.SeedQuest(t, st.DB, "цель")
	_, err = st.Execution.CreateHook(ctx, "на complete", []string{"complete"}, sc.ID, &qid, "home", 30, true)
	if err != nil {
		t.Fatal(err)
	}

	r.DispatchHooks(events.Payload{
		"kind": "quest_completed", "quest_id": float64(qid), "workspace": float64(1),
		"revision": float64(42), "title": "цель",
	})

	jobs, err := st.Execution.ListRunnerJobs(ctx, execution.JobFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range jobs {
		if j.Kind == execution.JobKindHook && j.RunID != nil {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a hook job, got %+v", jobs)
	}
}

// hookJobs counts the jobs the hooks queued.
func hookJobs(t *testing.T, st *store.Store) int {
	t.Helper()
	jobs, err := st.Execution.ListRunnerJobs(wsctx.With(context.Background(), 1), execution.JobFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, j := range jobs {
		if j.Kind == execution.JobKindHook {
			n++
		}
	}
	return n
}

const echoHook = `{"nodes":[
	{"id":1,"type":"source.event","params":{"family":"note"}},
	{"id":2,"type":"code","params":{"text":"echo ok"}},
	{"id":3,"type":"sink.done"}],
 "edges":[
	{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},
	{"from":{"node":2,"port":"stdout"},"to":{"node":3,"port":"trigger"}}]}`

func TestHooksFollowTheirAreaAndSkipPrivateNotes(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	r := NewCheckRunner(st, events.New())
	sc, err := st.Execution.CreateScript(ctx, "заметка", "", []byte(echoHook))
	if err != nil {
		t.Fatal(err)
	}
	mk := func(title string, parent *int64, private bool) int64 {
		n, err := st.Knowledge.CreateNote(ctx, knowledge.Note{Title: title, ParentID: parent})
		if err != nil {
			t.Fatal(err)
		}
		if private {
			if _, err := st.DB.Exec(`UPDATE note SET is_private = 1 WHERE id = ?`, n.ID); err != nil {
				t.Fatal(err)
			}
		}
		return n.ID
	}
	root := mk("корень", nil, false)
	child := mk("потомок", &root, false)
	other := mk("чужая", nil, false)
	secret := mk("тайная", &root, true)

	tree := "note_tree"
	if _, err := st.Execution.CreateHookIn(ctx, "ветка", []string{"note_updated"}, sc.ID, nil, "home", 30, true, execution.HookIn{ScopeKind: &tree, ScopeID: &root}); err != nil {
		t.Fatal(err)
	}
	fire := func(id int64, rev float64) {
		r.DispatchHooks(events.Payload{"kind": "note_updated", "note_id": float64(id), "workspace": float64(1), "revision": rev, "title": "x"})
	}
	fire(other, 1)
	if n := hookJobs(t, st); n != 0 {
		t.Fatalf("a note outside the area woke the hook: %d", n)
	}
	fire(child, 2)
	if n := hookJobs(t, st); n != 1 {
		t.Fatalf("a note under the area: %d jobs", n)
	}
	fire(secret, 3)
	if n := hookJobs(t, st); n != 1 {
		t.Fatalf("a private note must not wake the hook by default: %d jobs", n)
	}
	yes := true
	list, _ := st.Execution.ListHooks(ctx)
	if _, err := st.Execution.UpdateHook(ctx, list[0].ID, execution.HookIn{IncludePrivate: &yes}); err != nil {
		t.Fatal(err)
	}
	r.hookGate = newHookGate() // a fresh pause: the test is about the area, not the clock
	fire(secret, 4)
	if n := hookJobs(t, st); n != 2 {
		t.Fatalf("with include_private: %d jobs", n)
	}
}

func TestHooksDoNotWakeThemselves(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	r := NewCheckRunner(st, events.New())
	sc, err := st.Execution.CreateScript(ctx, "заметка", "", []byte(echoHook))
	if err != nil {
		t.Fatal(err)
	}
	h, err := st.Execution.CreateHook(ctx, "сам", []string{"note_updated"}, sc.ID, nil, "home", 30, true)
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.Knowledge.CreateNote(ctx, knowledge.Note{Title: "н"})
	if err != nil {
		t.Fatal(err)
	}
	ev := func(origin string, rev float64) events.Payload {
		p := events.Payload{"kind": "note_updated", "note_id": float64(note.ID), "workspace": float64(1), "revision": rev}
		if origin != "" {
			p["origin"] = origin
		}
		return p
	}
	r.DispatchHooks(ev("hook:"+strconv.FormatInt(h.ID, 10), 1))
	if n := hookJobs(t, st); n != 0 {
		t.Fatalf("an event made by this hook woke it: %d", n)
	}
	r.DispatchHooks(ev("hook:900>hook:901>hook:902", 2))
	if n := hookJobs(t, st); n != 0 {
		t.Fatalf("a chain of hooks that long is a loop: %d", n)
	}
	r.DispatchHooks(ev("hook:900", 3))
	if n := hookJobs(t, st); n != 1 {
		t.Fatalf("another hook's work does wake this one: %d", n)
	}
}

// A hook that writes a variable on the thing it watches would wake itself forever; the chain of origins stops it.
func TestHookWritingAVariableDoesNotLoop(t *testing.T) {
	st := openChecksDB(t)
	ctx := wsctx.With(context.Background(), 1)
	hub := events.New()
	r := NewCheckRunner(st, hub)
	hub.OnPublish(func(p events.Payload) { r.DispatchHooks(p) })

	note, err := st.Knowledge.CreateNote(ctx, knowledge.Note{Title: "лист"})
	if err != nil {
		t.Fatal(err)
	}
	const g = `{"nodes":[
		{"id":1,"type":"source.event","params":{"family":"value","owner":"note"}},
		{"id":2,"type":"sink.value","params":{"name":"mirror","type":"str"}}],
	 "edges":[
		{"from":{"node":1,"port":"ref"},"to":{"node":2,"port":"ref"}},
		{"from":{"node":1,"port":"name"},"to":{"node":2,"port":"value"}}]}`
	sc, err := st.Execution.CreateScript(ctx, "зеркало", "", []byte(g))
	if err != nil {
		t.Fatal(err)
	}
	tree := "note"
	if _, err := st.Execution.CreateHookIn(ctx, "зеркало", []string{"value"}, sc.ID, nil, "", 30, true, execution.HookIn{ScopeKind: &tree, ScopeID: &note.ID}); err != nil {
		t.Fatal(err)
	}
	owner := execution.SecretOwner{Kind: execution.SecretOwnerNote, ID: note.ID}
	if _, err := st.Execution.SetNamedValue(ctx, owner, "hp", "int", "9"); err != nil {
		t.Fatal(err)
	}
	hub.ValuesChanged(1, events.ValueChange{OwnerKind: "note", OwnerID: note.ID, Name: "hp", Type: "int", Value: int64(9)})

	read := func() string {
		set, err := st.Execution.ResolveNamedValues(ctx, owner, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range set.Own {
			if v.Name == "mirror" {
				return v.Value
			}
		}
		return ""
	}
	// The hub hands events to its observers on goroutines of their own, so wait for the write.
	for i := 0; i < 40 && read() != "hp"; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if got := read(); got != "hp" {
		t.Fatalf("the hook wrote %q, want the name of the variable that changed", got)
	}
	// The write above told the hub too; had that woken the hook again, it would have written "mirror" once its pause was over.
	time.Sleep(2500 * time.Millisecond)
	if got := read(); got != "hp" {
		t.Fatalf("the hook woke itself: mirror = %q", got)
	}
}
