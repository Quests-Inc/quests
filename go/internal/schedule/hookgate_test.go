package schedule

import (
	"strings"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
)

func gateAt(now *time.Time) *hookGate {
	g := newHookGate()
	g.now = func() time.Time { return *now }
	return g
}

func TestHookGatePausesPerEntityAndKeepsTheLatest(t *testing.T) {
	now := time.Now()
	g := gateAt(&now)
	h := execution.Hook{ID: 1, CooldownSec: 10, MaxPerHour: 100}
	var ran []string
	run := func(tag string) func(string) {
		return func(over string) {
			if over != "" {
				t.Errorf("unexpected switch off: %s", over)
			}
			ran = append(ran, tag)
		}
	}
	g.submit(h, "quest_completed", "quest=5", run("first"))
	g.submit(h, "quest_completed", "quest=5", run("second"))
	g.submit(h, "quest_completed", "quest=5", run("third"))
	g.submit(h, "quest_completed", "quest=6", run("other quest"))
	g.submit(h, "quest_failed", "quest=5", run("other kind"))
	if strings.Join(ran, ",") != "first,other quest,other kind" {
		t.Fatalf("ran = %v", ran)
	}
	// The pause is over: what waited runs once, and it is the newest event.
	now = now.Add(11 * time.Second)
	g.release(h, "1|quest_completed|quest=5")
	if strings.Join(ran, ",") != "first,other quest,other kind,third" {
		t.Fatalf("after the pause: %v", ran)
	}
	g.release(h, "1|quest_completed|quest=5")
	if len(ran) != 4 {
		t.Fatalf("a release with nothing waiting runs nothing: %v", ran)
	}
}

func TestHookGateKindsThatComeInBurstsHaveAFloor(t *testing.T) {
	now := time.Now()
	g := gateAt(&now)
	h := execution.Hook{ID: 2, CooldownSec: 0, MaxPerHour: 100}
	n := 0
	run := func(string) { n++ }
	g.submit(h, "note_updated", "note=1", run)
	now = now.Add(20 * time.Second)
	g.submit(h, "note_updated", "note=1", run)
	if n != 1 {
		t.Fatalf("a note is not run for more often than every 30 s, whatever the hook says: %d", n)
	}
	now = now.Add(15 * time.Second)
	g.submit(h, "note_updated", "note=1", run)
	if n != 2 {
		t.Fatalf("after the floor: %d", n)
	}
}

func TestHookGateSwitchesOffAHookThatGoesOverTheHour(t *testing.T) {
	now := time.Now()
	g := gateAt(&now)
	h := execution.Hook{ID: 3, CooldownSec: 0, MaxPerHour: 3}
	var started int
	var reason string
	run := func(over string) {
		if over != "" {
			reason = over
			return
		}
		started++
	}
	for i := 0; i < 5; i++ {
		g.submit(h, "quest_created", "quest="+string(rune('1'+i)), run)
	}
	if started != 3 || reason == "" || !strings.Contains(reason, "3") {
		t.Fatalf("started %d, reason %q", started, reason)
	}
	// An hour later the count starts over.
	now = now.Add(61 * time.Minute)
	reason = ""
	g.submit(h, "quest_created", "quest=9", run)
	if reason != "" || started != 4 {
		t.Fatalf("a new hour: started %d, reason %q", started, reason)
	}
}
