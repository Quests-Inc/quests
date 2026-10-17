package schedule

import (
	"context"
	"log"
	"strconv"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// What a finished check does to its step. The older path (apply) and the graph sinks (scripthost.go) share these.

// applyProgress writes a reading of a polled check onto its step: a new progress, a new total (re-sizing the step) and/or a
// new description, each only when the reading had it. A reading that changes nothing still records that the check ran.
func (r *CheckRunner) applyProgress(ctx context.Context, q journal.Quest, st *journal.Step, progress, total *int, description *string) {
	now := timeutil.NowUTC()
	st.CheckLastRunAt = &now
	if description != nil {
		st.Description = *description
	}
	totalChanged := total != nil && *total != st.ProgressTotal
	if totalChanged {
		st.ProgressTotal = *total
	}
	if progress == nil && !totalChanged {
		_, _ = r.Store.Journal.UpdateStep(ctx, *st, q, "step_progress", st.Title)
		return
	}
	n := st.ProgressCurrent // a total-only reading keeps the progress it had
	if progress != nil {
		n = *progress
	}
	if n < 0 {
		n = 0
	}
	if n > st.ProgressTotal {
		n = st.ProgressTotal
	}
	progressSame := n == st.ProgressCurrent
	descTouched := description != nil
	if progressSame && !descTouched && !totalChanged {
		_, _ = r.Store.Journal.UpdateStep(ctx, *st, q, "step_progress", st.Title)
		return
	}
	st.ProgressCurrent = n
	journal.ClampStep(st)
	journal.SyncStatusFromSteps(&q, now)
	q.UpdatedAt = now
	kind := "step_progress"
	detail := st.Title
	toast := false
	if st.Done {
		kind = "step_completed"
		toast = true
	}
	if q.Status == journal.StatusCompleted {
		kind = "quest_completed"
		detail = "завершено"
		toast = true
	}
	updated, err := r.Store.Journal.UpdateStep(ctx, *st, q, kind, detail)
	if err != nil {
		log.Printf("checks: poll save step %d: %v", st.ID, err)
		return
	}
	qid := updated.ID
	r.Hub.Publish(kind, events.PublishOpts{
		Workspace: wsctx.From(ctx),
		QuestID:   &qid, Title: updated.Title, Description: updated.Description,
		Detail: detail, Toast: toast, Source: "system",
		Significance: string(updated.Significance), StepTitle: st.Title,
		Automated: updated.Automated,
	})
}

func (r *CheckRunner) applyOnce(ctx context.Context, q journal.Quest, st *journal.Step, exit int, stdout, stderr string, timedOut bool) {
	now := timeutil.NowUTC()
	st.CheckLastRunAt = &now
	if exit == 0 && !timedOut {
		r.onceSucceeded(ctx, q, st)
		return
	}
	why := "код " + strconv.Itoa(exit)
	if timedOut {
		why = "таймаут"
	}
	comment := "автошаг «" + st.Title + "»: " + why
	if blob := trimCheckOut(stderr, stdout); blob != "" {
		comment += "\n" + blob
	}
	r.onceFailed(ctx, q, st, comment)
}

// onceSucceeded closes a one-shot step whose command exited cleanly.
func (r *CheckRunner) onceSucceeded(ctx context.Context, q journal.Quest, st *journal.Step) {
	now := timeutil.NowUTC()
	ok := journal.RunSuccess
	st.RunStatus = &ok
	st.ProgressCurrent = st.ProgressTotal
	journal.ClampStep(st)
	journal.SyncStatusFromSteps(&q, now)
	q.UpdatedAt = now
	kind := "step_completed"
	detail := st.Title
	if q.Status == journal.StatusCompleted {
		kind = "quest_completed"
		detail = "завершено"
	}
	updated, err := r.Store.Journal.UpdateStep(ctx, *st, q, kind, detail)
	if err != nil {
		log.Printf("checks: once ok step %d: %v", st.ID, err)
		return
	}
	qid := updated.ID
	r.Hub.Publish(kind, events.PublishOpts{
		Workspace: wsctx.From(ctx),
		QuestID:   &qid, Title: updated.Title, Description: updated.Description,
		Detail: detail, Toast: true, Source: "system",
		Significance: string(updated.Significance), StepTitle: st.Title,
		Automated: updated.Automated,
	})
}

// onceFailed fails the quest of a one-shot step whose command did not do its job, with the comment in the quest log.
func (r *CheckRunner) onceFailed(ctx context.Context, q journal.Quest, st *journal.Step, comment string) {
	now := timeutil.NowUTC()
	fail := journal.RunFail
	st.RunStatus = &fail
	q.Status = journal.StatusFailed
	q.UpdatedAt = now
	q.CompletedAt = nil
	updated, err := r.Store.Journal.UpdateStepComment(ctx, *st, q, "quest_failed", "провалено", comment)
	if err != nil {
		log.Printf("checks: once fail step %d: %v", st.ID, err)
		return
	}
	qid := updated.ID
	r.Hub.Publish("quest_failed", events.PublishOpts{
		Workspace: wsctx.From(ctx),
		QuestID:   &qid, Title: updated.Title, Description: updated.Description,
		Detail: "провалено", Toast: true, Source: "system",
		Significance: string(updated.Significance), StepTitle: st.Title,
		Automated: updated.Automated,
	})
}
