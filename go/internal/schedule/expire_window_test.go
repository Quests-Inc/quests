package schedule

import (
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/store"
)

func TestDeadlineSetWithOpenWindow(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	dur := 1800 // 30 min

	t.Run("postpone_style", func(t *testing.T) {
		// deadline=now+N, duration=N → window_start==now==updated_at
		deadline := now.Add(30 * time.Minute)
		q := journal.Quest{
			DeadlineAt:      &deadline,
			DurationSeconds: &dur,
			UpdatedAt:       now,
		}
		if !deadlineSetWithOpenWindow(q) {
			t.Fatal("postpone-style window should be treated as already open")
		}
	})

	t.Run("natural_crossing", func(t *testing.T) {
		// Deadline written yesterday; window opens now.
		deadline := now.Add(30 * time.Minute)
		q := journal.Quest{
			DeadlineAt:      &deadline,
			DurationSeconds: &dur,
			UpdatedAt:       now.Add(-24 * time.Hour),
		}
		if deadlineSetWithOpenWindow(q) {
			t.Fatal("natural window crossing must still fire quest_started")
		}
	})

	t.Run("within_skew", func(t *testing.T) {
		deadline := now.Add(30 * time.Minute)
		q := journal.Quest{
			DeadlineAt:      &deadline,
			DurationSeconds: &dur,
			// Scheduler tick a couple of seconds after the PATCH.
			UpdatedAt: now.Add(-2 * time.Second),
		}
		if !deadlineSetWithOpenWindow(q) {
			t.Fatal("2s lag after postpone should still suppress")
		}
	})

	t.Run("beyond_skew", func(t *testing.T) {
		deadline := now.Add(30 * time.Minute)
		q := journal.Quest{
			DeadlineAt:      &deadline,
			DurationSeconds: &dur,
			UpdatedAt:       now.Add(-10 * time.Second),
		}
		if deadlineSetWithOpenWindow(q) {
			t.Fatal("10s before window_start is a real crossing, not postpone")
		}
	})
}

func countStarted(hub *events.Hub) int {
	n := 0
	for _, e := range hub.EventsSince(0, 0) {
		if e["kind"] == "quest_started" {
			n++
		}
	}
	return n
}

func seedWindowQuest(t *testing.T, st *store.Store, deadline time.Time, duration int, updatedAt time.Time) journal.Quest {
	t.Helper()
	q, err := st.Journal.CreateQuest(bg(), journal.Quest{
		Title:           "window",
		Status:          journal.StatusActive,
		Significance:    journal.SigCommon,
		DeadlineAt:      &deadline,
		DurationSeconds: &duration,
		CreatedAt:       updatedAt,
		UpdatedAt:       updatedAt,
		Steps:           []journal.Step{{Title: "s", ProgressTotal: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// CreateQuest overwrites UpdatedAt with NowUTC — force the times we care about.
	_, err = st.DB.ExecContext(bg(),
		`UPDATE quest SET deadline_at=?, duration_seconds=?, updated_at=?, created_at=? WHERE id=?`,
		timeutil.ToDBUTC(deadline), duration, timeutil.ToDBUTC(updatedAt), timeutil.ToDBUTC(updatedAt), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.Journal.GetQuest(bg(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestWindowNotifier_PostponeSuppressesStarted(t *testing.T) {
	st := openChecksDB(t)
	hub := events.New()
	w := NewWindowNotifier()
	ctx := bg()

	// Seed empty so the next tick is "live".
	if _, err := w.Notify(ctx, st, hub); err != nil {
		t.Fatal(err)
	}

	now := timeutil.NowUTC()
	dur := 1800
	// Postpone: window already open at update time.
	q := seedWindowQuest(t, st, now.Add(30*time.Minute), dur, now)

	fired, err := w.Notify(ctx, st, hub)
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 0 {
		t.Fatalf("postpone must not fire quest_started, got %v", fired)
	}
	if countStarted(hub) != 0 {
		t.Fatalf("hub got %d quest_started events", countStarted(hub))
	}
	// Still in window on next tick — must stay silent (marked notified).
	fired, err = w.Notify(ctx, st, hub)
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 0 {
		t.Fatalf("second tick must stay silent, got %v (quest %d)", fired, q.ID)
	}
}

func TestWindowNotifier_NaturalCrossingFires(t *testing.T) {
	st := openChecksDB(t)
	hub := events.New()
	w := NewWindowNotifier()
	ctx := bg()

	if _, err := w.Notify(ctx, st, hub); err != nil {
		t.Fatal(err)
	}

	now := timeutil.NowUTC()
	dur := 1800
	// Deadline set long ago; window is open now → natural start.
	q := seedWindowQuest(t, st, now.Add(30*time.Minute), dur, now.Add(-24*time.Hour))

	fired, err := w.Notify(ctx, st, hub)
	if err != nil {
		t.Fatal(err)
	}
	if len(fired) != 1 || fired[0] != q.ID {
		t.Fatalf("expected fire for quest %d, got %v", q.ID, fired)
	}
	if countStarted(hub) != 1 {
		t.Fatalf("hub quest_started count=%d", countStarted(hub))
	}
}

// The maintenance pass visits workspaces one after another with one notifier;
// each workspace must keep its own "already announced" memory.
func TestWindowNotifier_PerWorkspaceMemory(t *testing.T) {
	st := openChecksDB(t)
	hub := events.New()
	w := NewWindowNotifier()
	now := timeutil.NowUTC()
	seedWindowQuest(t, st, now.Add(30*time.Minute), 1800, now.Add(-24*time.Hour))

	for tick := 0; tick < 3; tick++ {
		for _, ws := range []int64{1, 2} {
			if _, err := w.Notify(wsctx.With(bg(), ws), st, hub); err != nil {
				t.Fatal(err)
			}
		}
	}
	if n := countStarted(hub); n != 0 {
		t.Fatalf("quests already open at first sight must stay silent, got %d quest_started", n)
	}
}
