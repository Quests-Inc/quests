package schedule

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/secretbox"
	"github.com/valirum/quests/go/internal/store"
	"github.com/valirum/quests/go/internal/wsctx"
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

const noteCheckSchema = `
CREATE TABLE workspace (id INTEGER PRIMARY KEY, name TEXT, trusted INTEGER NOT NULL DEFAULT 0, deleted_at TEXT);
INSERT INTO workspace (id, name, trusted) VALUES (1, 'trusted', 1), (2, 'untrusted', 0);
CREATE TABLE note (
	id INTEGER PRIMARY KEY, title TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
	pinned INTEGER NOT NULL DEFAULT 0, is_readme INTEGER NOT NULL DEFAULT 0, is_private INTEGER NOT NULL DEFAULT 0,
	is_category INTEGER NOT NULL DEFAULT 0, sort_order INTEGER NOT NULL DEFAULT 0, parent_id INTEGER,
	color TEXT, icon TEXT, custom_icon TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
	check_command TEXT, check_interval_seconds INTEGER, check_last_run_at TEXT, run_on TEXT,
	outdated INTEGER NOT NULL DEFAULT 0, outdated_at TEXT, check_reason TEXT NOT NULL DEFAULT '',
	check_error TEXT NOT NULL DEFAULT '', check_error_at TEXT);
INSERT INTO note (id, title, created_at, updated_at) VALUES (1, 'guide', '2026-10-01 00:00:00', '2026-10-01 00:00:00');
CREATE TABLE secret (id INTEGER PRIMARY KEY, questline_id INTEGER, template_id INTEGER, quest_id INTEGER, step_id INTEGER, note_id INTEGER,
	key TEXT NOT NULL, value TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE UNIQUE INDEX ux_secret_note_id_key ON secret (note_id, key) WHERE note_id IS NOT NULL;
CREATE TABLE questchangelog (id INTEGER PRIMARY KEY, kind TEXT, entity_type TEXT, entity_id INTEGER, title TEXT, detail TEXT, comment TEXT, quest_id INTEGER,
	parent_type TEXT, parent_id INTEGER, created_at TEXT, significance TEXT, count INTEGER DEFAULT 1);
`

func noteCheckRig(t *testing.T) (*CheckRunner, context.Context) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:notecheck_"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(noteCheckSchema); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUESTS_SECRET_KEY", "")
	box, _ := secretbox.Load(t.TempDir())
	// PGURL only switches the feature on: the same SQL runs on SQLite.
	st := &store.Store{DB: db, PGURL: "stub", Box: box}
	r := NewCheckRunner(st, events.New())
	return r, wsctx.With(context.Background(), 1)
}

// runNoteCheck sets the check, ticks the scheduler once and waits for the run.
func runNoteCheck(t *testing.T, r *CheckRunner, ctx context.Context, cmd string) store.NoteCheck {
	t.Helper()
	c := cmd
	if _, err := r.Store.SetNoteCheck(ctx, 1, &c, nil, nil); err != nil {
		t.Fatalf("set check: %v", err)
	}
	r.TickNotes(ctx)
	r.Wait()
	got, err := r.Store.GetNoteCheck(ctx, 1)
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

func TestNoteCheckSecretsAreTheNotesOwn(t *testing.T) {
	r, ctx := noteCheckRig(t)
	if err := r.Store.SetSecret(ctx, store.SecretOwner{Kind: store.SecretOwnerNote, ID: 1}, "TOKEN", "abc-secret-value"); err != nil {
		t.Fatal(err)
	}
	got := runNoteCheck(t, r, ctx, `[ "$TOKEN" = "abc-secret-value" ] && [ "$QUESTS_NOTE_ID" = "1" ] && echo 1 || echo 0`)
	if !got.Outdated {
		t.Fatalf("the note's secret and QUESTS_NOTE_ID must reach the command: %+v", got)
	}
	// A secret of the note is masked in what the command prints back.
	got = runNoteCheck(t, r, ctx, `echo "$TOKEN" >&2; exit 2`)
	if got.Error == "" || containsStr(got.Error, "abc-secret") {
		t.Fatalf("the secret leaked into the error: %q", got.Error)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestNoteCheckHonoursIntervalAndRunNow(t *testing.T) {
	r, ctx := noteCheckRig(t)
	runNoteCheck(t, r, ctx, `echo 0`)
	now := time.Now().UTC()
	due, _ := r.Store.DueNoteChecks(ctx, now)
	if len(due) != 0 {
		t.Fatalf("a check that just ran must wait its interval, got %+v", due)
	}
	if err := r.Store.RequestNoteCheckNow(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if due, _ := r.Store.DueNoteChecks(ctx, now); len(due) != 1 {
		t.Fatalf("\"check now\" must make it due at the next tick, got %+v", due)
	}
	if due, _ := r.Store.DueNoteChecks(ctx, now.Add(2*time.Hour)); len(due) != 1 {
		t.Fatal("after the interval it is due again")
	}
}

func TestNoteCheckServerCommandsNeedATrustedWorkspace(t *testing.T) {
	r, _ := noteCheckRig(t)
	untrusted := wsctx.With(context.Background(), 2)
	cmd := "echo 1"
	if _, err := r.Store.SetNoteCheck(untrusted, 1, &cmd, nil, nil); err == nil {
		t.Fatal("a command for the server was accepted in an untrusted workspace")
	}
	// With a runner it is accepted; it just waits for the device.
	label := "laptop"
	if _, err := r.Store.SetNoteCheck(untrusted, 1, &cmd, nil, &label); err != nil {
		t.Fatalf("a runner command must be accepted anywhere: %v", err)
	}
	// And whatever sits in the table is never run on the server of an untrusted workspace.
	if _, err := r.Store.DB.Exec(`UPDATE note SET run_on = NULL WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	r.TickNotes(untrusted)
	r.Wait()
	got, _ := r.Store.GetNoteCheck(untrusted, 1)
	if got.Outdated || got.Error == "" {
		t.Fatalf("it must refuse and say why, without a verdict: %+v", got)
	}
}

func TestNoteCheckInterval(t *testing.T) {
	r, ctx := noteCheckRig(t)
	cmd := "echo 0"
	low := 3
	c, err := r.Store.SetNoteCheck(ctx, 1, &cmd, &low, nil)
	if err != nil || c.IntervalSeconds == nil || *c.IntervalSeconds != store.MinCheckInterval {
		t.Fatalf("below the floor must become %d: %+v %v", store.MinCheckInterval, c, err)
	}
	c, _ = r.Store.SetNoteCheck(ctx, 1, &cmd, nil, nil)
	if c.IntervalSeconds == nil || *c.IntervalSeconds != store.DefaultNoteCheckInterval {
		t.Fatalf("no interval → default: %+v", c)
	}
	// Removing the check removes its verdict: nobody is renewing it.
	runNoteCheck(t, r, ctx, `echo 1`)
	empty := ""
	c, _ = r.Store.SetNoteCheck(ctx, 1, &empty, nil, nil)
	if c.Active() || c.Outdated || c.Reason != "" || c.Error != "" {
		t.Fatalf("removing the check must clear its state: %+v", c)
	}
}
