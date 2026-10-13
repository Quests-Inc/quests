package journal

import (
	"context"
	"testing"

	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

func openFailureInboxDB(t *testing.T) *Store {
	t.Helper()
	pool, _ := pgtest.NewWithURL(t)
	return &Store{DB: pool}
}

func bg() context.Context { return wsctx.With(context.Background(), 1) }

func TestFailureInboxOnCreateAndDismiss(t *testing.T) {
	st := openFailureInboxDB(t)
	ctx := bg()
	now := timeutil.NowUTC()

	active, err := st.CreateQuest(ctx, Quest{
		Title: "active", Status: StatusActive, Significance: SigCommon,
		CreatedAt: now, UpdatedAt: now,
		Steps: []Step{{Title: "s", ProgressTotal: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if active.FailureInbox {
		t.Fatal("active quest must not be in failure inbox")
	}

	failed, err := st.CreateQuest(ctx, Quest{
		Title: "failed", Status: StatusFailed, Significance: SigCommon,
		CreatedAt: now, UpdatedAt: now,
		Steps: []Step{{Title: "s", ProgressTotal: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !failed.FailureInbox {
		t.Fatal("create-as-failed must put quest in failure inbox")
	}

	if err := st.DismissFailureInbox(ctx, failed.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetQuest(ctx, failed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed {
		t.Fatalf("status=%s want failed", got.Status)
	}
	if got.FailureInbox {
		t.Fatal("dismissed failure must leave inbox")
	}

	// Idempotent dismiss.
	if err := st.DismissFailureInbox(ctx, failed.ID); err != nil {
		t.Fatal(err)
	}
}

func TestFailureInboxOnStatusPatch(t *testing.T) {
	st := openFailureInboxDB(t)
	ctx := bg()
	now := timeutil.NowUTC()
	q, err := st.CreateQuest(ctx, Quest{
		Title: "to-fail", Status: StatusActive, Significance: SigCommon,
		CreatedAt: now, UpdatedAt: now,
		Steps: []Step{{Title: "s", ProgressTotal: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	q.Status = StatusFailed
	q.UpdatedAt = timeutil.NowUTC()
	updated, err := st.UpdateQuest(ctx, q, "quest_failed", "провалено")
	if err != nil {
		t.Fatal(err)
	}
	if !updated.FailureInbox {
		t.Fatal("PATCH to failed must enter failure inbox")
	}
}
