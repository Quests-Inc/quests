package schedule

import (
	"math/rand"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
)

// Exhausted emit_pool creates an already-failed quest, puts it in the
// failure inbox, and publishes quest_failed with toast=true (not a quiet
// quest_appeared that HUD/Telegram would drop).
func TestMaterializeDuePoolFailNotifiesAndInboxes(t *testing.T) {
	st := openEmitSpecDB(t)
	insertTemplate(t, st, 9, `title, emit_pool_command, automated`,
		`'Почта', 'exit 7', 1`)

	hub := events.New()
	rng := rand.New(rand.NewSource(1))
	now := time.Now()
	var ids []int64
	for i := 0; i < emitPoolMaxAttempts+2; i++ {
		if i > 0 {
			now = now.Add(emitPoolRetryDelays[min(i-1, len(emitPoolRetryDelays)-1)] + time.Second)
		}
		if _, err := MaterializeDue(bg(), st, hub, now, rng); err != nil {
			t.Fatal(err)
		}
		runQueuedJobs(t, st, nil)
		got, err := MaterializeDue(bg(), st, hub, now, rng)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) > 0 {
			ids = got
			break
		}
	}
	if len(ids) != 1 {
		t.Fatalf("want one failed quest after pool exhaustion, got %v", ids)
	}
	q, err := st.Journal.GetQuest(bg(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if q.Status != journal.StatusFailed {
		t.Fatalf("status=%s want failed", q.Status)
	}
	if !q.FailureInbox {
		t.Fatal("failed emit quest must be in failure inbox")
	}
	var kind string
	if err := st.DB.QueryRow(`SELECT kind FROM questchangelog WHERE quest_id = ? ORDER BY id DESC LIMIT 1`, q.ID).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "quest_failed" {
		t.Fatalf("changelog kind=%q want quest_failed", kind)
	}

	evs := hub.EventsSince(0, 1)
	var sawFail bool
	for _, ev := range evs {
		if ev["kind"] == "quest_failed" && ev["toast"] == true {
			sawFail = true
			break
		}
	}
	if !sawFail {
		t.Fatalf("want quest_failed toast=true in hub events, got %#v", evs)
	}
}
