package schedule

import (
	"context"
	"time"

	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/store"
)

// How far after UpdatedAt a window_start may land and still count as
// "deadline was set with the window already open" (site/APK postpone:
// deadline=now+N, duration=N → window_start≈updated_at). Covers scheduler
// tick lag of a couple seconds without swallowing a natural crossing that
// happened hours after the deadline was written.
const windowStartSynthSkew = 3 * time.Second

// deadlineSetWithOpenWindow reports whether the urgent window was already
// open (within skew) at the quest's last update — typical of postponeQuest.
// Natural case: deadline written earlier, clock later crosses window_start
// → window_start is well after UpdatedAt → false → fire quest_started.
func deadlineSetWithOpenWindow(q domain.Quest) bool {
	if q.DeadlineAt == nil || q.DurationSeconds == nil || *q.DurationSeconds <= 0 {
		return false
	}
	start := timeutil.WindowStart(*q.DeadlineAt, *q.DurationSeconds)
	return !start.After(timeutil.EnsureUTC(q.UpdatedAt).Add(windowStartSynthSkew))
}

// ExpireOverdue marks active quests past deadline as expired and publishes events.
func ExpireOverdue(ctx context.Context, st *store.Store, hub *events.Hub) ([]int64, error) {
	now := timeutil.NowUTC()
	nowDB := timeutil.ToDBUTC(now)
	rows, err := st.DB.QueryContext(ctx, `
		SELECT id FROM quest
		WHERE status = 'active' AND deadline_at IS NOT NULL AND deadline_at <= ?
			-- duration=0/NULL means "no window": deadline is informational only,
			-- don't auto-flip the quest to expired just because it passed.
			AND duration_seconds IS NOT NULL AND duration_seconds > 0
		ORDER BY id`, nowDB)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	expired := make([]int64, 0, len(ids))
	for _, id := range ids {
		q, err := st.GetQuest(ctx, id)
		if err != nil {
			continue
		}
		if q.Status != domain.StatusActive {
			continue
		}
		q.Status = domain.StatusExpired
		q.UpdatedAt = now
		updated, err := st.UpdateQuest(ctx, q, "quest_expired", "просрочено")
		if err != nil {
			return expired, err
		}
		qid := updated.ID
		hub.Publish("quest_expired", events.PublishOpts{
			Workspace:    wsctx.From(ctx),
			QuestID:      &qid,
			Title:        updated.Title,
			Description:  updated.Description,
			Detail:       "просрочено",
			Toast:        true,
			Source:       "system",
			Significance: string(updated.Significance),
			Automated:    updated.Automated,
		})
		expired = append(expired, qid)
	}
	return expired, nil
}

// WindowNotifier fires quest_started once when the urgent window opens. It
// remembers what it has already announced per workspace: the maintenance pass
// visits every workspace in turn, and one shared set would be wiped by the next
// workspace's pass, re-announcing everything on the following tick.
type WindowNotifier struct {
	spaces map[int64]*windowSpace
}

type windowSpace struct {
	notified map[int64]struct{}
	seeded   bool
}

func NewWindowNotifier() *WindowNotifier {
	return &WindowNotifier{spaces: map[int64]*windowSpace{}}
}

func (w *WindowNotifier) Notify(ctx context.Context, st *store.Store, hub *events.Hub) ([]int64, error) {
	quests, err := st.ListQuests(ctx, store.ListFilter{})
	if err != nil {
		return nil, err
	}
	ws := wsctx.From(ctx)
	sp := w.spaces[ws]
	if sp == nil {
		sp = &windowSpace{notified: map[int64]struct{}{}}
		w.spaces[ws] = sp
	}
	now := timeutil.NowUTC()
	inWindow := make([]domain.Quest, 0)
	live := map[int64]struct{}{}
	for _, q := range quests {
		if q.Status != domain.StatusActive || q.DeadlineAt == nil || q.DurationSeconds == nil {
			continue
		}
		rem := timeutil.RemainingSeconds(q.DeadlineAt, now)
		if rem != nil && *rem <= 0 {
			continue
		}
		if timeutil.IsInUrgentWindow(q.DeadlineAt, q.DurationSeconds, now) {
			inWindow = append(inWindow, q)
			live[q.ID] = struct{}{}
		}
	}
	for id := range sp.notified {
		if _, ok := live[id]; !ok {
			delete(sp.notified, id)
		}
	}
	if !sp.seeded {
		for id := range live {
			sp.notified[id] = struct{}{}
		}
		sp.seeded = true
		return nil, nil
	}
	fired := make([]int64, 0)
	for _, q := range inWindow {
		if _, ok := sp.notified[q.ID]; ok {
			continue
		}
		// Always remember we've seen this window occupancy so a postponed
		// quest does not fire on a later tick while still inside the window.
		sp.notified[q.ID] = struct{}{}
		if deadlineSetWithOpenWindow(q) {
			continue
		}
		qid := q.ID
		hub.Publish("quest_started", events.PublishOpts{
			Workspace:    wsctx.From(ctx),
			QuestID:      &qid,
			Title:        q.Title,
			Description:  q.Description,
			Detail:       "началось задание",
			Toast:        true,
			Source:       "system",
			Significance: string(q.Significance),
			Automated:    q.Automated,
		})
		fired = append(fired, qid)
	}
	return fired, nil
}
