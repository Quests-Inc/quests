package schedule

import (
	"strconv"
	"sync"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
)

// hookGate keeps a hook from running in a storm. Per hook, event kind and entity it lets the first event through at once and
// holds the next ones until the pause is over, then runs the latest of them (so the last edit of a busy note is not lost). Per
// hook it counts the runs in the last hour; a hook that goes over the cap is switched off, with the reason on the hook.
type hookGate struct {
	mu      sync.Mutex
	fires   map[int64][]time.Time
	last    map[string]time.Time
	waiting map[string]func(string)
	now     func() time.Time
}

func newHookGate() *hookGate {
	return &hookGate{fires: map[int64][]time.Time{}, last: map[string]time.Time{}, waiting: map[string]func(string){}, now: time.Now}
}

// kindMinPause is the shortest pause for events that arrive in bursts, whatever the hook asks.
var kindMinPause = map[string]time.Duration{
	"note_updated":   30 * time.Second,
	"step_progress":  5 * time.Second,
	"values_changed": 2 * time.Second,
}

// submit runs `run` now, later, or never. `run("")` starts the hook; `run(reason)` switches it off.
func (g *hookGate) submit(h execution.Hook, kind, entity string, run func(string)) {
	key := strconv.FormatInt(h.ID, 10) + "|" + kind + "|" + entity
	pause := time.Duration(h.CooldownSec) * time.Second
	if m := kindMinPause[kind]; m > pause {
		pause = m
	}
	g.mu.Lock()
	now := g.now()
	if prev, ok := g.last[key]; ok && now.Sub(prev) < pause {
		_, queued := g.waiting[key]
		g.waiting[key] = run
		g.mu.Unlock()
		if !queued {
			time.AfterFunc(pause-now.Sub(prev), func() { g.release(h, key) })
		}
		return
	}
	g.mu.Unlock()
	g.fire(h, key, run)
}

func (g *hookGate) release(h execution.Hook, key string) {
	g.mu.Lock()
	run := g.waiting[key]
	delete(g.waiting, key)
	g.mu.Unlock()
	if run != nil {
		g.fire(h, key, run)
	}
}

func (g *hookGate) fire(h execution.Hook, key string, run func(string)) {
	g.mu.Lock()
	now := g.now()
	recent := g.fires[h.ID][:0]
	for _, t := range g.fires[h.ID] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= h.MaxPerHour {
		g.fires[h.ID] = recent
		g.mu.Unlock()
		run("больше " + strconv.Itoa(h.MaxPerHour) + " запусков за час: хук выключен, проверьте его и включите снова")
		return
	}
	g.fires[h.ID] = append(recent, now)
	g.last[key] = now
	if len(g.last) > 20000 {
		for k, t := range g.last {
			if now.Sub(t) > time.Hour {
				delete(g.last, k)
			}
		}
	}
	g.mu.Unlock()
	run("")
}
