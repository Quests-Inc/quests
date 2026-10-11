package schedule

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/automation"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/store"
)

func ensureSurpriseRoll(ctx context.Context, st *store.Store, tmpl templateRow, periodKey string, localNow time.Time, rng *rand.Rand) (int64, string, *time.Time, error) {
	roll, found, err := st.Automation().EmitRollFor(ctx, tmpl.ID, periodKey)
	if err != nil {
		return 0, "", nil, err
	}
	if found {
		var sched *time.Time
		if roll.ScheduledAt.Valid {
			if t, e := timeutil.ParseFlexible(roll.ScheduledAt.String); e == nil {
				sched = &t
			}
		}
		return roll.ID, roll.Outcome, sched, nil
	}

	chance := tmpl.EmitChance
	if chance < 0 {
		chance = 0
	}
	if chance > 1 {
		chance = 1
	}
	now := timeutil.NowUTC()
	if rng.Float64() >= chance {
		id, err := st.Automation().InsertEmitRoll(ctx, tmpl.ID, periodKey, "miss", 0, nil, nil, "", now)
		if err != nil {
			return 0, "", nil, err
		}
		return id, "miss", nil, nil
	}

	startRaw, endRaw := "", ""
	if tmpl.EmitWindowStart.Valid {
		startRaw = tmpl.EmitWindowStart.String
	}
	if tmpl.EmitWindowEnd.Valid {
		endRaw = tmpl.EmitWindowEnd.String
	}
	scheduledLocal := pickScheduledAt(localNow, startRaw, endRaw, rng)
	schedUTC := scheduledLocal.UTC()
	id, err := st.Automation().InsertEmitRoll(ctx, tmpl.ID, periodKey, "scheduled", 0, &schedUTC, nil, "", now)
	if err != nil {
		return 0, "", nil, err
	}
	return id, "scheduled", &schedUTC, nil
}

func pickScheduledAt(localDay time.Time, windowStart, windowEnd string, rng *rand.Rand) time.Time {
	sh, sm, ss, okS := parseClock(windowStart)
	eh, em, es, okE := parseClock(windowEnd)
	if !okS {
		sh, sm, ss = 0, 0, 0
	}
	if !okE {
		eh, em, es = 23, 59, 0
	}
	loc := localDay.Location()
	start := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), sh, sm, ss, 0, loc)
	end := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), eh, em, es, 0, loc)
	if end.Before(start) {
		start, end = end, start
	}
	span := int(end.Sub(start).Seconds())
	if span < 0 {
		span = 0
	}
	offset := 0
	if span > 0 {
		offset = rng.Intn(span + 1)
	}
	return start.Add(time.Duration(offset) * time.Second)
}

// emitResult is what one tick learns about a template's emit command.
// Spec is set when there is a quest to create now (the command printed one); nil means nothing this tick
// (pending retry, still running, or "no quest this period"). FailMsg is
// non-empty exactly when this call is the one that exhausted the attempts
// (outcome just became "error") — the caller then creates a failed quest
// carrying the trace, since nothing else surfaces it to the user.
type emitResult struct {
	Spec    *resolvedSpec
	RollID  int64
	FailMsg string
}

// poolOpts tunes resolveEmit. ignorePause lets a manual run ("materialize
// now") go ahead even while a retry pause is pending.
type poolOpts struct {
	ignorePause bool
}

// resolveEmit drives one template's emit_pool_command for the current period:
// runs it (retrying with pauses on failure, up to emitPoolMaxAttempts),
// validates the quest it printed against the template's limits and persists the
// outcome in templateemitroll so repeated ticks don't re-run or re-create.
func resolveEmit(ctx context.Context, st *store.Store, tmpl templateRow, periodKey string, now time.Time, opts poolOpts) (emitResult, error) {
	roll, found, qerr := st.Automation().EmitRollFor(ctx, tmpl.ID, periodKey)
	if qerr != nil {
		return emitResult{}, qerr
	}
	id, outcome, attempts, retryAt := roll.ID, roll.Outcome, roll.Attempts, roll.RetryAt
	isNew := !found
	if !isNew && (outcome == "materialized" || outcome == "error" || outcome == "miss") {
		return emitResult{RollID: id}, nil
	}
	// A failed run scheduled its own retry: until then the tick leaves it alone.
	if !isNew && retryAt.Valid && !opts.ignorePause {
		if at, perr := timeutil.ParseFlexible(retryAt.String); perr == nil && now.Before(at) {
			return emitResult{RollID: id}, nil
		}
	}

	attemptNo := attempts + 1
	var out emitOutput
	var info execInfo
	var execErr error
	if tmpl.EmitRunOn.Valid && strings.TrimSpace(tmpl.EmitRunOn.String) != "" {
		// The command runs on a runner device: queue it, collect the result on a later tick.
		var ready bool
		out, info, execErr, ready = execEmitRemote(ctx, st, tmpl, periodKey)
		if !ready {
			return emitResult{RollID: id}, nil
		}
	} else {
		// Commands run on runners only (see store.ErrNoRunner).
		info.Status = "error"
		execErr = store.ErrNoRunner
	}
	rec := automation.EmitAttempt{
		TemplateID: tmpl.ID, PeriodKey: periodKey, At: now, Attempt: attemptNo,
		Status: info.Status, DurationMS: info.Duration.Milliseconds(),
		Message: info.Stderr,
	}
	// Logging never fails the roll: it's diagnostics, not state.
	record := func() {
		if rerr := st.Automation().RecordEmitAttempt(ctx, rec); rerr != nil {
			log.Printf("emit attempt log (template %d): %v", tmpl.ID, rerr)
		}
	}

	var resolved *resolvedSpec
	var note string
	if execErr == nil {
		spec := out.Spec
		if spec != nil {
			lim, lerr := automation.ParseEmitLimits(tmpl.EmitLimits.String)
			if lerr != nil {
				lim = automation.DefaultEmitLimits()
				log.Printf("emit limits (template %d): %v; using defaults", tmpl.ID, lerr)
			}
			var rerr error
			resolved, rerr = resolveEmitSpec(ctx, st, spec, lim)
			if rerr != nil {
				execErr = fmt.Errorf("invalid quest: %w", rerr)
				info.Status = "bad_spec"
				rec.Status = "bad_spec"
			}
		}
	}

	if execErr != nil {
		attempts++
		newOutcome := "scheduled"
		var nextRetry *time.Time
		if attempts >= emitPoolMaxAttempts {
			newOutcome = "error"
			rec.Message = "giving up: attempts exhausted\n" + execErr.Error()
		} else {
			delay := emitPoolRetryDelays[min(attempts-1, len(emitPoolRetryDelays)-1)]
			at := now.Add(delay)
			nextRetry = &at
			rec.Message = fmt.Sprintf("retry in %s\n", delay) + execErr.Error()
		}
		record()
		if isNew {
			var err error
			if id, err = st.Automation().InsertEmitRoll(ctx, tmpl.ID, periodKey, newOutcome, attempts, nil, nextRetry, "", now); err != nil {
				return emitResult{}, err
			}
		} else if err := st.Automation().UpdateEmitRollAttempt(ctx, id, newOutcome, attempts, nextRetry, now); err != nil {
			return emitResult{}, err
		}
		if newOutcome == "error" {
			return emitResult{RollID: id, FailMsg: execErr.Error()}, nil
		}
		return emitResult{RollID: id}, nil
	}

	if resolved == nil {
		rec.Message = joinNote("miss: the command printed no quest", rec.Message)
		record()
		id, err := persistEmitPoolOutcome(ctx, st, id, isNew, tmpl.ID, periodKey, "miss", attempts, "", now)
		if err != nil {
			return emitResult{}, err
		}
		return emitResult{RollID: id}, nil
	}

	rec.Items = len(resolved.Steps)
	rec.Picked = 1
	title := resolved.Title
	if title == "" {
		title = tmpl.Title
	}
	msg := fmt.Sprintf("quest «%s», %d step(s)", title, len(resolved.Steps))
	if note != "" {
		msg += "; " + note
	}
	if len(resolved.Ignored) > 0 {
		msg += "; ignored fields: " + strings.Join(resolved.Ignored, ", ")
	}
	rec.Message = joinNote(msg, rec.Message)
	record()
	id, err := persistEmitPoolOutcome(ctx, st, id, isNew, tmpl.ID, periodKey, "scheduled", attempts, "", now)
	if err != nil {
		return emitResult{}, err
	}
	return emitResult{Spec: resolved, RollID: id}, nil
}

// persistEmitPoolOutcome returns the row's id — for a new row this is the
// freshly inserted id, which callers need (e.g. to later mark it
// materialized); returning it here means they don't have to duplicate the
// isNew branching themselves.
func persistEmitPoolOutcome(ctx context.Context, st *store.Store, id int64, isNew bool, templateID int64, periodKey, outcome string, attempts int, pickedRefsJSON string, now time.Time) (int64, error) {
	if isNew {
		return st.Automation().InsertEmitRoll(ctx, templateID, periodKey, outcome, attempts, nil, nil, pickedRefsJSON, now)
	}
	return id, st.Automation().UpdateEmitRollPicked(ctx, id, outcome, pickedRefsJSON, now)
}

// execInfo is what one emit_pool_command run leaves behind for the attempt
// log. Status is "ok" or, for a failed run, "error" | "timeout" | "bad_json".
// Stderr/Message never contain template secret values.
type execInfo struct {
	Status   string
	Duration time.Duration
	Stderr   string
}

// joinNote prefixes the attempt's own verdict to whatever stderr it produced.
func joinNote(note, stderr string) string {
	if stderr == "" {
		return note
	}
	return note + "\nstderr:\n" + stderr
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
