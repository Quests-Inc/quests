package schedule

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/knowledge"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/store"
)

func TestParseNoteCheckOutput(t *testing.T) {
	good := []struct {
		in       string
		outdated bool
		reason   string
	}{
		{"0", false, ""},
		{"1\n", true, ""},
		{"checking…\nsome log line\n1", true, ""}, // earlier lines are log noise
		{`{"outdated": 1, "reason": "изменился admin.go: abc → def"}`, true, "изменился admin.go: abc → def"},
		{`{"outdated": 0}`, false, ""},
		{`{"outdated": true, "reason": "  r  "}`, true, "r"},
		{`{"outdated": false, "reason": 5}`, false, ""}, // a reason that is not text is dropped, not an error
	}
	for _, c := range good {
		got, err := ParseNoteCheckOutput(c.in)
		if err != nil || got.Outdated != c.outdated || got.Reason != c.reason {
			t.Errorf("%q → %+v, %v (want outdated=%v reason=%q)", c.in, got, err, c.outdated, c.reason)
		}
	}
	for _, bad := range []string{
		"", "  \n", "yes", "2", "-1", "outdated", "1 file changed", "0\nstale", // last line is what counts
		"{not json", `{"reason": "no verdict"}`, `{"outdated": 2}`, `{"outdated": "1"}`, `{"outdated": null}`, `[1]`,
	} {
		if got, err := ParseNoteCheckOutput(bad); err == nil {
			t.Errorf("%q must be an error, got %+v", bad, got)
		}
	}
}

func noteCheckRig(t *testing.T) (*CheckRunner, context.Context) {
	t.Helper()
	pool, url := pgtest.NewWithURL(t)
	// A second workspace with a note of its own.
	pgtest.ExecOwner(t, url, `INSERT INTO workspace (id, name) VALUES (2, 'other');
INSERT INTO note (id, title, workspace_id, created_at, updated_at) VALUES (2, 'elsewhere', 2, now(), now());`)
	if _, err := pool.Exec(`INSERT INTO note (id, title, created_at, updated_at) VALUES (1, 'guide', '2026-10-01 00:00:00', '2026-10-01 00:00:00')`); err != nil {
		t.Fatal(err)
	}
	pgtest.FixSequences(t, url)
	t.Setenv("QUESTS_SECRET_KEY", "")
	box, _ := secretbox.Load(t.TempDir())
	st := store.New(pool, url, box)
	r := NewCheckRunner(st, events.New())
	return r, wsctx.With(context.Background(), 1)
}

// runNoteCheck sets the check, ticks the scheduler once and waits for the run.
func runNoteCheck(t *testing.T, r *CheckRunner, ctx context.Context, cmd string) knowledge.NoteCheck {
	t.Helper()
	c, label := cmd, testLabel
	if _, err := r.Store.Knowledge.SetNoteCheck(ctx, 1, &c, nil, &label); err != nil {
		t.Fatalf("set check: %v", err)
	}
	r.TickNotes(ctx)
	settle(t, r.Store, r)
	got, err := r.Store.Knowledge.GetNoteCheck(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestNoteCheckVerdicts(t *testing.T) {
	r, ctx := noteCheckRig(t)

	got := runNoteCheck(t, r, ctx, `echo 1`)
	if !got.Outdated || got.Error != "" || got.LastRunAt == nil || got.OutdatedAt == nil {
		t.Fatalf("1 must mark the note outdated: %+v", got)
	}

	// A broken run is an error and changes nothing else — the verdict stays.
	got = runNoteCheck(t, r, ctx, `echo "boom" >&2; exit 3`)
	if !got.Outdated || got.Error == "" {
		t.Fatalf("a failing command must keep the verdict and record an error: %+v", got)
	}
	got = runNoteCheck(t, r, ctx, `echo maybe`)
	if !got.Outdated || got.Error == "" {
		t.Fatalf("an unreadable answer must keep the verdict and record an error: %+v", got)
	}

	// A good answer clears the error and moves the verdict, with a reason when given.
	got = runNoteCheck(t, r, ctx, `echo '{"outdated": 0, "reason": "ничего не менялось"}'`)
	if got.Outdated || got.Error != "" || got.Reason != "ничего не менялось" || got.OutdatedAt != nil {
		t.Fatalf("0 must clear the mark and the error: %+v", got)
	}
	got = runNoteCheck(t, r, ctx, `echo '{"outdated": 1, "reason": "изменился admin.go"}'`)
	if !got.Outdated || got.Reason != "изменился admin.go" {
		t.Fatalf("JSON verdict with a reason: %+v", got)
	}
}

func TestNoteCheckHonoursIntervalAndRunNow(t *testing.T) {
	r, ctx := noteCheckRig(t)
	runNoteCheck(t, r, ctx, `echo 0`)
	now := time.Now().UTC()
	due, _ := r.Store.Knowledge.DueNoteChecks(ctx, now)
	if len(due) != 0 {
		t.Fatalf("a check that just ran must wait its interval, got %+v", due)
	}
	if err := r.Store.Knowledge.RequestNoteCheckNow(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if due, _ := r.Store.Knowledge.DueNoteChecks(ctx, now); len(due) != 1 {
		t.Fatalf("\"check now\" must make it due at the next tick, got %+v", due)
	}
	if due, _ := r.Store.Knowledge.DueNoteChecks(ctx, now.Add(2*time.Hour)); len(due) != 1 {
		t.Fatal("after the interval it is due again")
	}
}

// A command needs a runner: saving one without it is refused, and one that sits in the table without it
// (stored before commands were limited to runners) is reported on the note, never run.
func TestNoteCheckNeedsARunner(t *testing.T) {
	r, ctx := noteCheckRig(t)
	cmd := "echo 1"
	if _, err := r.Store.Knowledge.SetNoteCheck(ctx, 1, &cmd, nil, nil); !errors.Is(err, store.ErrNoRunner) {
		t.Fatalf("a command without a runner must be refused, got %v", err)
	}
	label := testLabel
	if _, err := r.Store.Knowledge.SetNoteCheck(ctx, 1, &cmd, nil, &label); err != nil {
		t.Fatalf("a runner command must be accepted: %v", err)
	}
	if _, err := r.Store.DB.ExecContext(ctx, `UPDATE note SET run_on = NULL WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	r.TickNotes(ctx)
	settle(t, r.Store, r)
	got, _ := r.Store.Knowledge.GetNoteCheck(ctx, 1)
	if got.Outdated || got.Error == "" {
		t.Fatalf("it must refuse and say why, without a verdict: %+v", got)
	}
}

func TestNoteCheckInterval(t *testing.T) {
	r, ctx := noteCheckRig(t)
	cmd, label := "echo 0", testLabel
	low := 3
	c, err := r.Store.Knowledge.SetNoteCheck(ctx, 1, &cmd, &low, &label)
	if err != nil || c.IntervalSeconds == nil || *c.IntervalSeconds != knowledge.MinCheckInterval {
		t.Fatalf("below the floor must become %d: %+v %v", knowledge.MinCheckInterval, c, err)
	}
	c, _ = r.Store.Knowledge.SetNoteCheck(ctx, 1, &cmd, nil, &label)
	if c.IntervalSeconds == nil || *c.IntervalSeconds != knowledge.DefaultNoteCheckInterval {
		t.Fatalf("no interval → default: %+v", c)
	}
	// Removing the check removes its verdict: nobody is renewing it.
	runNoteCheck(t, r, ctx, `echo 1`)
	empty := ""
	c, _ = r.Store.Knowledge.SetNoteCheck(ctx, 1, &empty, nil, nil)
	if c.Active() || c.Outdated || c.Reason != "" || c.Error != "" {
		t.Fatalf("removing the check must clear its state: %+v", c)
	}
}
