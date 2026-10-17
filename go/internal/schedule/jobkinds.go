package schedule

import (
	"context"

	"github.com/quests-inc/quests/go/internal/execution"
)

// Kinds are the sources of user code the scheduler queues jobs for (ADR-0002). Each queues through
// execution.Store.EnqueueJob and gets its finished jobs back here; the order is the order they are asked in.
func (r *CheckRunner) Kinds() execution.Kinds {
	return execution.Kinds{noteKind{r}, stepKind{r}, emitKind{}}
}

// noteKind: a note's freshness check.
type noteKind struct{ r *CheckRunner }

func (noteKind) Name() string                    { return "note" }
func (noteKind) Owns(j execution.RunnerJob) bool { return j.NoteID != nil }
func (k noteKind) Finished(ctx context.Context, j execution.RunnerJob) {
	k.r.noteJobFinished(ctx, j)
}

// stepKind: a step's check (poll or one-shot).
type stepKind struct{ r *CheckRunner }

func (stepKind) Name() string { return "step" }
func (stepKind) Owns(j execution.RunnerJob) bool {
	return j.StepID != nil && j.QuestID != nil
}
func (k stepKind) Finished(ctx context.Context, j execution.RunnerJob) {
	k.r.stepJobFinished(ctx, j)
}

// emitKind: a template's emit command. Its result is not pushed here: the emit tick finds the finished job by its
// key and takes it (execEmitRemote), so a finished job needs no action.
type emitKind struct{}

func (emitKind) Name() string                                  { return "emit" }
func (emitKind) Owns(j execution.RunnerJob) bool               { return j.TemplateID != nil }
func (emitKind) Finished(context.Context, execution.RunnerJob) {}
