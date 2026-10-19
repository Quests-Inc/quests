package schedule

import (
	"errors"
	"math/rand"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
)

func fastEmit(t *testing.T) {
	t.Helper()
	w, p := emitNowWait, emitNowPoll
	emitNowWait, emitNowPoll = 5*time.Second, 40*time.Millisecond
	t.Cleanup(func() { emitNowWait, emitNowPoll = w, p })
}

// A manual emit of a dynamic template waits for the device and makes the quest the script printed, not the template's own steps.
func TestManualEmitWaitsForTheScriptsQuest(t *testing.T) {
	fastEmit(t)
	st, tmpl := scriptedTemplate(t, `echo '{"title":"Из скрипта","steps":[{"title":"Письмо 1"},{"title":"Письмо 2"}]}'`)
	ctx := bg()
	var stopped atomic.Bool
	go func() {
		for !stopped.Load() {
			runQueuedJobs(t, st, nil)
			time.Sleep(50 * time.Millisecond)
		}
	}()
	defer stopped.Store(true)

	qid, err := materializeManual(ctx, st, events.New(), tmpl.ID, time.Now(), rand.New(rand.NewSource(1)), emitNowWait)
	if err != nil {
		t.Fatal(err)
	}
	q, _ := st.Journal.GetQuest(ctx, qid)
	if q.Title != "Из скрипта" || len(q.Steps) != 2 || q.Steps[0].Title != "Письмо 1" {
		t.Fatalf("the quest the script printed: %+v", q)
	}
}

// Nothing printed, a failure and a device that does not answer make no quest, and the error says which.
func TestManualEmitMakesNoQuestWithoutAnAnswer(t *testing.T) {
	for _, tc := range []struct {
		name, cmd string
		device    bool
		code      int
		outcome   string
	}{
		{"nothing", `echo null`, true, http.StatusOK, OutcomeMiss},
		{"failure", `echo oops >&2; exit 3`, true, http.StatusBadGateway, OutcomeFailed},
		{"no device", `echo '{"title":"x"}'`, false, http.StatusAccepted, OutcomePending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fastEmit(t)
			if !tc.device {
				emitNowWait = 400 * time.Millisecond
			}
			st, tmpl := scriptedTemplate(t, tc.cmd)
			ctx := bg()
			var stopped atomic.Bool
			if tc.device {
				go func() {
					for !stopped.Load() {
						runQueuedJobs(t, st, nil)
						time.Sleep(50 * time.Millisecond)
					}
				}()
				defer stopped.Store(true)
			}
			_, err := materializeManual(ctx, st, events.New(), tmpl.ID, time.Now(), rand.New(rand.NewSource(1)), emitNowWait)
			var es *EmitStatusError
			if !errors.As(err, &es) || es.Code != tc.code || es.Outcome != tc.outcome {
				t.Fatalf("want %d %s, got %v", tc.code, tc.outcome, err)
			}
			n, _ := st.Journal.ListQuests(ctx, journal.ListFilter{})
			if len(n) != 0 {
				t.Fatalf("a quest was made: %+v", n)
			}
		})
	}
}

// A test emit shows the quest and creates none; a template with nothing to run is told so.
func TestPreviewEmitCreatesNothing(t *testing.T) {
	fastEmit(t)
	st, tmpl := scriptedTemplate(t, `echo '{"title":"Из скрипта","steps":[{"title":"А"},{"title":"Б","description":"описание"}],"junk":1}'`)
	ctx := bg()
	var stopped atomic.Bool
	go func() {
		for !stopped.Load() {
			runQueuedJobs(t, st, nil)
			time.Sleep(50 * time.Millisecond)
		}
	}()
	defer stopped.Store(true)

	p, err := PreviewEmit(ctx, st, tmpl.ID, time.Now())
	if err != nil || p.Outcome != OutcomeQuest || p.Title != "Из скрипта" || len(p.Steps) != 2 || p.Steps[1].Description != "описание" || len(p.Ignored) != 1 {
		t.Fatalf("preview: %+v %v", p, err)
	}
	if n, _ := st.Journal.ListQuests(ctx, journal.ListFilter{}); len(n) != 0 {
		t.Fatalf("a test made a quest: %+v", n)
	}

	plain, err := st.Automation.CreateTemplate(ctx, map[string]any{"title": "Обычный", "enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	var es *EmitStatusError
	if _, err := PreviewEmit(ctx, st, plain.ID, time.Now()); !errors.As(err, &es) || es.Code != http.StatusUnprocessableEntity {
		t.Fatalf("a template with no script or command: %v", err)
	}
}

// The manual emit of a dynamic template does not wait: it answers «queued» at once, and the quest appears when the device
// has answered.
func TestManualEmitIsQueuedAndTheQuestAppearsLater(t *testing.T) {
	fastEmit(t)
	st, tmpl := scriptedTemplate(t, `echo '{"title":"Из скрипта","steps":[{"title":"Письмо"}]}'`)
	ctx := bg()
	qid, err := MaterializeTemplateManual(ctx, st, events.New(), tmpl.ID, time.Now(), rand.New(rand.NewSource(1)))
	var es *EmitStatusError
	if !errors.As(err, &es) || es.Code != http.StatusAccepted || es.Outcome != OutcomeQueued || qid != 0 {
		t.Fatalf("want queued, got %d %v", qid, err)
	}
	if n, _ := st.Journal.ListQuests(ctx, journal.ListFilter{}); len(n) != 0 {
		t.Fatalf("a quest before the device answered: %+v", n)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		runQueuedJobs(t, st, nil)
		if n, _ := st.Journal.ListQuests(ctx, journal.ListFilter{}); len(n) == 1 {
			if n[0].Title != "Из скрипта" {
				t.Fatalf("the quest: %+v", n[0])
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the quest did not appear")
}
