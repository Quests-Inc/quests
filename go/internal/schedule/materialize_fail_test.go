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
	// The retries walk the clock ~21 minutes ahead; a daily period must not roll over meanwhile,
	// so start at noon Moscow time (the templates' default zone) rather than at the wall clock.
	today := time.Now().UTC()
	now := time.Date(today.Year(), today.Month(), today.Day(), 9, 0, 0, 0, time.UTC)
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
	var sawEmitFailed bool
	for _, ev := range evs {
		if ev["kind"] == "template_emit_failed" && ev["template_id"] == int64(9) {
			sawEmitFailed = true
		}
		if ev["kind"] == "template_emitted" {
			t.Fatalf("a failed emit must not announce a quest: %#v", ev)
		}
	}
	if !sawEmitFailed {
		t.Fatalf("want template_emit_failed for hooks, got %#v", evs)
	}
}

// A template that gives its quest tells the hooks (template_emitted), with the template and the period.
func TestMaterializeDueAnnouncesTheTemplateEmit(t *testing.T) {
	st := openEmitSpecDB(t)
	insertTemplate(t, st, 10, `title`, `'Обычный'`)
	hub := events.New()
	today := time.Now().UTC()
	now := time.Date(today.Year(), today.Month(), today.Day(), 9, 0, 0, 0, time.UTC)
	ids, err := MaterializeDue(bg(), st, hub, now, rand.New(rand.NewSource(1)))
	if err != nil || len(ids) != 1 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	for _, ev := range hub.EventsSince(0, 1) {
		if ev["kind"] == "template_emitted" && ev["template_id"] == int64(10) && ev["period"] != "" {
			return
		}
	}
	t.Fatalf("want template_emitted, got %#v", hub.EventsSince(0, 1))
}
