package execution

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestScriptTrashRoundTrip(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	sc, err := st.CreateScript(ctx, "в корзину", "", scriptGraph("echo 1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteScript(ctx, sc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetScript(ctx, sc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a trashed script is hidden: %v", err)
	}
	if err := st.DeleteScript(ctx, sc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("trashed twice: %v", err)
	}
	list, err := st.ListTrashed(ctx, "script")
	if err != nil || len(list) != 1 || list[0].ID != sc.ID || list[0].Title != "в корзину" {
		t.Fatalf("trash: %+v %v", list, err)
	}
	if title, err := st.RestoreTrashed(ctx, "script", sc.ID); err != nil || title != "в корзину" {
		t.Fatalf("restore: %q %v", title, err)
	}
	if got, err := st.GetScript(ctx, sc.ID); err != nil || got.Version != 1 {
		t.Fatalf("back with its version: %+v %v", got, err)
	}
	if _, err := st.RestoreTrashed(ctx, "script", sc.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a live script is not in the trash: %v", err)
	}

	if err := st.DeleteScript(ctx, sc.ID); err != nil {
		t.Fatal(err)
	}
	if ids, _ := st.TrashedIDsBefore(ctx, "script", time.Now().Add(-time.Hour)); len(ids) != 0 {
		t.Fatalf("fresh in the trash: %v", ids)
	}
	if ids, _ := st.TrashedIDsBefore(ctx, "script", time.Now().Add(time.Hour)); len(ids) != 1 {
		t.Fatalf("due for a purge: %v", ids)
	}
	if err := st.PurgeTrashed(ctx, "script", sc.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := st.ListTrashed(ctx, "script"); len(list) != 0 {
		t.Fatalf("purged: %+v", list)
	}
}

// A hook goes to the trash and stops firing; it cannot come back while its script is in the trash, and erasing the script
// takes the trashed hook along.
func TestHookTrashFollowsItsScript(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	sc, err := st.CreateScript(ctx, "на событие", "", questHookGraph())
	if err != nil {
		t.Fatal(err)
	}
	h, err := st.CreateHook(ctx, "пинг", []string{"complete"}, sc.ID, nil, "home", 30, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteScript(ctx, sc.ID); err == nil {
		t.Fatal("a script with a live hook stays")
	}
	if err := st.DeleteHook(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	if hooks, _ := st.EnabledHooks(ctx, "quest_completed"); len(hooks) != 0 {
		t.Fatalf("a trashed hook does not fire: %+v", hooks)
	}
	if err := st.DeleteScript(ctx, sc.ID); err != nil {
		t.Fatalf("a trashed hook does not hold its script: %v", err)
	}
	if _, err := st.RestoreTrashed(ctx, "hook", h.ID); !errors.Is(err, ErrScriptTrashed) {
		t.Fatalf("hook before its script: %v", err)
	}
	if err := st.PurgeTrashed(ctx, "script", sc.ID); err != nil {
		t.Fatalf("purge a script with a trashed hook: %v", err)
	}
	if list, _ := st.ListTrashed(ctx, "hook"); len(list) != 0 {
		t.Fatalf("the hook went with it: %+v", list)
	}
}

// Texts that embed script=N{…} are uses that do not hold the script.
func TestScriptTextUses(t *testing.T) {
	st := openSecretsDB(t)
	ctx := context.Background()
	sc, err := st.CreateScript(ctx, "кости", "", scriptGraph("echo 1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO note (id, title, description, created_at, updated_at) VALUES
		(61, 'бросок', 'урон: script=` + itoa(sc.ID) + `{2к6}', now(), now()),
		(62, 'не то', 'xscript=` + itoa(sc.ID) + `{1} и script=` + itoa(sc.ID) + `0{1}', now(), now())`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`UPDATE queststep SET description = 'script=` + itoa(sc.ID) + `{1}?auto' WHERE id = 1000`); err != nil {
		t.Fatal(err)
	}
	uses, err := st.ScriptUses(ctx, sc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(uses) != 2 || uses[0].SubjectType != "note" || uses[0].SubjectID != 61 || uses[1].SubjectType != "step" || uses[0].Via != UseViaText {
		t.Fatalf("text uses: %+v", uses)
	}
	if err := st.DeleteScript(ctx, sc.ID); err != nil {
		t.Fatalf("an embedding does not hold the script: %v", err)
	}
}
