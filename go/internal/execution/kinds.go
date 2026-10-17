package execution

import "context"

// Kind is one source of user code (ADR-0002): a step check, a note check, a template's emit command, later a hook or a
// library script. It knows which jobs it queued and what a finished one means; the queue itself, the leases and the
// runners are shared. A new source implements Kind and queues through Store.EnqueueJob, nothing else.
type Kind interface {
	// Name labels the kind in logs.
	Name() string
	// Owns reports whether the job was queued by this kind.
	Owns(j RunnerJob) bool
	// Finished applies the outcome of a job that reached a final state (a result, a lost device, a cancel…).
	Finished(ctx context.Context, j RunnerJob)
}

// Kinds is the set of sources a server runs, asked in order: the first that owns a job gets its outcome.
type Kinds []Kind

// Finished hands the finished job to the kind that queued it and reports which one took it ("" = none owns it).
func (ks Kinds) Finished(ctx context.Context, j RunnerJob) string {
	for _, k := range ks {
		if k.Owns(j) {
			k.Finished(ctx, j)
			return k.Name()
		}
	}
	return ""
}
