package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
)

func questHookGraph() []byte {
	return []byte(`{"nodes":[
		{"id":1,"type":"source.quest"},
		{"id":2,"type":"code","params":{"text":"echo ok"}},
		{"id":3,"type":"sink.done"}],
	 "edges":[
		{"from":{"node":1,"port":"secrets"},"to":{"node":2,"port":"secrets"}},
		{"from":{"node":2,"port":"stdout"},"to":{"node":3,"port":"trigger"}}]}`)
}

func TestHookLifecycle(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	sc, err := st.CreateScript(ctx, "на событие", "", questHookGraph())
	if err != nil {
		t.Fatal(err)
	}
	qid := pgtest.SeedQuest(t, st.DB, "цель")

	_, err = st.CreateHook(ctx, "пинг", []string{"complete"}, sc.ID, nil, "", 30, true)
	if !errors.Is(err, kernel.ErrNoRunner) {
		t.Fatalf("script needs a runner: %v", err)
	}

	h, err := st.CreateHook(ctx, "пинг", []string{"complete"}, sc.ID, &qid, "home", 30, true)
	if err != nil {
		t.Fatal(err)
	}
	if !h.Enabled || len(h.Events) != 1 || h.Events[0] != "quest_completed" || h.ScriptTitle != "на событие" || h.QuestID == nil || *h.QuestID != qid {
		t.Fatalf("created: %+v", h)
	}

	list, err := st.ListHooks(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	matched, err := st.HooksForEvent(ctx, "quest_completed", &qid)
	if err != nil || len(matched) != 1 {
		t.Fatalf("match: %+v %v", matched, err)
	}
	other := qid + 1
	if matched, _ = st.HooksForEvent(ctx, "quest_completed", &other); len(matched) != 0 {
		t.Fatalf("quest filter: %+v", matched)
	}
	if matched, _ = st.HooksForEvent(ctx, "quest_failed", &qid); len(matched) != 0 {
		t.Fatalf("kind filter: %+v", matched)
	}

	off := false
	h2, err := st.UpdateHook(ctx, h.ID, HookIn{Enabled: &off})
	if err != nil || h2.Enabled {
		t.Fatalf("disable: %+v %v", h2, err)
	}
	if matched, _ = st.HooksForEvent(ctx, "quest_completed", &qid); len(matched) != 0 {
		t.Fatalf("disabled must not match: %+v", matched)
	}

	uses, err := st.ScriptUses(ctx, sc.ID)
	if err != nil || len(uses) != 1 || uses[0].SubjectType != "hook" {
		t.Fatalf("uses: %+v %v", uses, err)
	}
	if err := st.DeleteScript(ctx, sc.ID); err == nil {
		t.Fatal("a used script must refuse delete")
	}

	if err := st.DeleteHook(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetHook(ctx, h.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("gone: %v", err)
	}
	if err := st.DeleteScript(ctx, sc.ID); err != nil {
		t.Fatal(err)
	}
}

func TestHookEventsCatalog(t *testing.T) {
	cat := HookEventsCatalog()
	aliases, _ := cat["aliases"].(map[string][]string)
	kinds, _ := cat["kinds"].([]string)
	if len(aliases["complete"]) == 0 || len(kinds) < 5 {
		t.Fatalf("catalog: %+v", cat)
	}
}
