package journal

import (
	"errors"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

func trashQuest(t *testing.T, st *Store, title string) Quest {
	t.Helper()
	now := timeutil.NowUTC()
	q, err := st.CreateQuest(bg(), Quest{
		Title: title, Status: StatusActive, Significance: SigCommon, CreatedAt: now, UpdatedAt: now,
		Steps: []Step{{Title: "a", ProgressTotal: 1}, {Title: "b", ProgressTotal: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func listIDs(t *testing.T, st *Store) map[int64]bool {
	t.Helper()
	qs, err := st.ListQuests(bg(), ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]bool{}
	for _, q := range qs {
		ids[q.ID] = true
	}
	return ids
}

func TestTrashHidesRestoreBringsBack(t *testing.T) {
	st := openFailureInboxDB(t)
	ctx := bg()
	keep, gone := trashQuest(t, st, "keep"), trashQuest(t, st, "gone")

	if err := st.TrashQuest(ctx, gone.ID, gone.Title); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetQuest(ctx, gone.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a trashed quest must not be readable, got %v", err)
	}
	ids := listIDs(t, st)
	if !ids[keep.ID] || ids[gone.ID] {
		t.Fatalf("list must show only the kept quest, got %v", ids)
	}
	if err := st.TrashQuest(ctx, gone.ID, gone.Title); !errors.Is(err, ErrNotFound) {
		t.Fatalf("trashing twice: %v", err)
	}

	trash, err := st.ListTrashedQuests(ctx)
	if err != nil || len(trash) != 1 || trash[0].ID != gone.ID || trash[0].StepsTotal != 2 {
		t.Fatalf("trash list: %+v %v", trash, err)
	}

	title, err := st.RestoreQuest(ctx, gone.ID)
	if err != nil || title != "gone" {
		t.Fatalf("restore: %q %v", title, err)
	}
	back, err := st.GetQuest(ctx, gone.ID)
	if err != nil || len(back.Steps) != 2 {
		t.Fatalf("restored quest: %+v %v", back, err)
	}
	if left, _ := st.ListTrashedQuests(ctx); len(left) != 0 {
		t.Fatalf("trash must be empty after restore: %+v", left)
	}
	if _, err := st.RestoreQuest(ctx, gone.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restoring a quest that is not trashed: %v", err)
	}
}

func TestTrashPurgeErasesOnlyTrashed(t *testing.T) {
	st := openFailureInboxDB(t)
	ctx := bg()
	live, dead := trashQuest(t, st, "live"), trashQuest(t, st, "dead")
	if err := st.PurgeQuest(ctx, live.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a live quest cannot be purged: %v", err)
	}
	if err := st.TrashQuest(ctx, dead.ID, dead.Title); err != nil {
		t.Fatal(err)
	}
	if err := st.PurgeQuest(ctx, dead.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.ListTrashedQuests(ctx); len(left) != 0 {
		t.Fatalf("purged quest still listed: %+v", left)
	}
	if _, err := st.GetQuest(ctx, live.ID); err != nil {
		t.Fatalf("the live quest must survive: %v", err)
	}
}

func TestTrashedIDsBefore(t *testing.T) {
	st := openFailureInboxDB(t)
	ctx := bg()
	q := trashQuest(t, st, "old")
	if err := st.TrashQuest(ctx, q.ID, q.Title); err != nil {
		t.Fatal(err)
	}
	if ids, _ := st.TrashedQuestIDsBefore(ctx, timeutil.NowUTC().Add(-time.Hour)); len(ids) != 0 {
		t.Fatalf("just trashed is not older than an hour: %v", ids)
	}
	if ids, _ := st.TrashedQuestIDsBefore(ctx, timeutil.NowUTC().Add(time.Minute)); len(ids) != 1 || ids[0] != q.ID {
		t.Fatalf("older than a minute ahead: %v", ids)
	}
}
