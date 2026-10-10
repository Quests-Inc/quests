package schedule

import (
	"context"
	"database/sql"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/store"
)

func openEmitPoolDB(t *testing.T) *store.Store {
	t.Helper()
	return openChecksDB(t)
}

// poolTemplate: the trailing pick argument predates the one-quest format and is ignored.
func poolTemplate(id int64, command string, _ ...int) templateRow {
	return templateRow{
		ID:              id,
		EmitPoolCommand: sql.NullString{String: command, Valid: true},
	}
}

// resolveEmitPool / resolveEmitPoolOpts keep the older tests' shape: the steps
// of the quest the command printed (nil when there is none this tick).
func resolveEmitPool(ctx context.Context, st *store.Store, tmpl templateRow, period string, now time.Time, rng *rand.Rand) ([]domain.Step, int64, string, error) {
	return resolveEmitPoolOpts(ctx, st, tmpl, period, now, rng, poolOpts{})
}

func resolveEmitPoolOpts(ctx context.Context, st *store.Store, tmpl templateRow, period string, now time.Time, _ *rand.Rand, opts poolOpts) ([]domain.Step, int64, string, error) {
	res, err := resolveEmit(ctx, st, tmpl, period, now, opts)
	var steps []domain.Step
	if res.Spec != nil {
		steps = res.Spec.Steps
	}
	return steps, res.RollID, res.FailMsg, err
}

// emitOnce runs one tick of resolveEmit and fails the test on a storage error.
func emitOnce(t *testing.T, st *store.Store, tmpl templateRow, period string, now time.Time) emitResult {
	t.Helper()
	res, err := resolveEmit(bg(), st, tmpl, period, now, poolOpts{})
	if err != nil {
		t.Fatalf("resolveEmit: %v", err)
	}
	return res
}

// A command that prints a quest object yields a spec; "no quest" outcomes
// (null, {}, nothing, or nothing at all) become a miss that stays a miss
// for the rest of the period and reuses the same roll row.
func TestResolveEmitMissIsStickyWithinPeriod(t *testing.T) {
	for _, cmd := range []string{`echo null`, `echo '{}'`, `true`} {
		st := openEmitPoolDB(t)
		tmpl := poolTemplate(1, cmd)
		first := emitOnce(t, st, tmpl, "p", time.Now())
		if first.Spec != nil || first.FailMsg != "" || first.RollID == 0 {
			t.Fatalf("%q: want a quiet miss with a roll row, got %+v", cmd, first)
		}
		var outcome string
		if err := st.DB.QueryRow(`SELECT outcome FROM templateemitroll WHERE id = ?`, first.RollID).Scan(&outcome); err != nil || outcome != "miss" {
			t.Fatalf("%q: outcome = %q (%v), want miss", cmd, outcome, err)
		}
		second := emitOnce(t, st, poolTemplate(1, `echo '{"title":"later"}'`), "p", time.Now())
		if second.Spec != nil || second.RollID != first.RollID {
			t.Fatalf("%q: a miss must hold for the period, got %+v", cmd, second)
		}
		_ = st.DB.Close()
	}
}

func TestResolveEmitQuestObject(t *testing.T) {
	st := openEmitPoolDB(t)
	tmpl := poolTemplate(1, `echo '{"title":"Счёт","significance":"epic","steps":[{"title":"Оплатить"},{"title":"Ждать","progress_total":3,"check_command":"true","run_mode":"watch"}]}'`)
	res := emitOnce(t, st, tmpl, "p", time.Now())
	if res.Spec == nil {
		t.Fatalf("want a spec, got %+v", res)
	}
	if res.Spec.Title != "Счёт" || res.Spec.Significance != "epic" || len(res.Spec.Steps) != 2 {
		t.Fatalf("unexpected spec: %+v", res.Spec)
	}
	if st2 := res.Spec.Steps[1]; st2.ProgressTotal != 3 || st2.RunMode != "watch" || st2.CheckCommand == nil {
		t.Fatalf("step fields lost: %+v", st2)
	}
	// the same period does not run again once a quest was scheduled for it
	var outcome string
	_ = st.DB.QueryRow(`SELECT outcome FROM templateemitroll WHERE id = ?`, res.RollID).Scan(&outcome)
	if outcome != "scheduled" {
		t.Fatalf("outcome = %q, want scheduled until the quest is created", outcome)
	}
}

// A command that always fails must retry up to emitPoolMaxAttempts, then
// flip outcome to "error" and return a non-empty FailMsg exactly once —
// the call that exhausts attempts, so the caller can materialize a failed
// quest carrying the trace.
func TestResolveEmitRetryThenError(t *testing.T) {
	st := openEmitPoolDB(t)
	tmpl := poolTemplate(1, `exit 7`)

	var lastRollID int64
	now := time.Now()
	for i := 1; i <= emitPoolMaxAttempts; i++ {
		if i > 1 { // each retry only runs once its pause has passed
			now = now.Add(emitPoolRetryDelays[i-2] + time.Second)
		}
		res := emitOnce(t, st, tmpl, "p", now)
		if res.Spec != nil {
			t.Fatalf("attempt %d: want no quest from a failing command, got %+v", i, res.Spec)
		}
		lastRollID = res.RollID
		var outcome string
		var attempts int
		if err := st.DB.QueryRow(`SELECT outcome, attempts FROM templateemitroll WHERE id = ?`, res.RollID).
			Scan(&outcome, &attempts); err != nil {
			t.Fatal(err)
		}
		if attempts != i {
			t.Fatalf("attempt %d: want attempts=%d, got %d", i, i, attempts)
		}
		if i < emitPoolMaxAttempts {
			if outcome != "scheduled" || res.FailMsg != "" {
				t.Fatalf("attempt %d: want scheduled and no FailMsg while retrying, got %q / %q", i, outcome, res.FailMsg)
			}
		} else if outcome != "error" || res.FailMsg == "" {
			t.Fatalf("attempt %d: want error and a FailMsg once attempts are exhausted, got %q / %q", i, outcome, res.FailMsg)
		}
	}

	// Once outcome=error, further calls in the same period are a no-op.
	res := emitOnce(t, st, tmpl, "p", time.Now())
	if res.Spec != nil || res.FailMsg != "" || res.RollID != lastRollID {
		t.Fatalf("post-error call should be a silent no-op on the same roll, got %+v", res)
	}
}

// An output that is not a valid quest (limits, bad names, bad values) is a
// failed attempt like a crash: logged with status bad_spec and retried.
func TestResolveEmitInvalidQuestIsAFailedAttempt(t *testing.T) {
	st := openEmitPoolDB(t)
	tmpl := poolTemplate(1, `echo '{"title":"x","significance":"medium"}'`)
	res := emitOnce(t, st, tmpl, "p", time.Now())
	if res.Spec != nil || res.FailMsg != "" {
		t.Fatalf("want a quiet retry, got %+v", res)
	}
	var attempts int
	var retry sql.NullString
	if err := st.DB.QueryRow(`SELECT attempts, retry_at FROM templateemitroll WHERE id = ?`, res.RollID).Scan(&attempts, &retry); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || !retry.Valid {
		t.Fatalf("attempts=%d retry_at=%v, want 1 and a pause", attempts, retry)
	}
	var status, msg string
	if err := st.DB.QueryRow(`SELECT status, message FROM templateemitattempt WHERE template_id = 1`).Scan(&status, &msg); err != nil {
		t.Fatal(err)
	}
	if status != "bad_spec" || !strings.Contains(msg, "insignificant") {
		t.Fatalf("status=%q message=%q, want bad_spec naming the valid values", status, msg)
	}
}

// emitPoolOpenAt: the check window opens duration_seconds before the
// deadline, not at the deadline itself and not before that offset.
func TestEmitPoolOpenAt(t *testing.T) {
	deadline := time.Date(2026, 9, 25, 13, 0, 0, 0, time.UTC)
	openAt := emitPoolOpenAt(deadline, 3600)
	want := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	if !openAt.Equal(want) {
		t.Fatalf("want openAt=%v, got %v", want, openAt)
	}

	before := deadline.Add(-2 * time.Hour)
	after := deadline.Add(-30 * time.Minute)
	if !before.Before(openAt) {
		t.Fatalf("sanity: %v should be before openAt %v", before, openAt)
	}
	if before.Before(openAt) == after.Before(openAt) {
		t.Fatalf("openAt should separate before (%v) from after (%v)", before, after)
	}

	// deadline_time set but no duration_seconds: the gate must default the
	// missing duration to 0 (check right at deadline_time), not fall back to
	// fixedDeadline's own midnight-to-deadline placeholder — that placeholder
	// is for the resulting quest's own duration field, unrelated to the gate.
	if got := emitPoolOpenAt(deadline, 0); !got.Equal(deadline) {
		t.Fatalf("want openAt=deadline (%v) when duration=0, got %v", deadline, got)
	}
}
