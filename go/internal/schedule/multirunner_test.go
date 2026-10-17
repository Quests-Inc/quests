package schedule

import (
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/store"
)

// seedRunner registers a runner that last polled `ago` ago (nil: never).
func seedRunner(t *testing.T, st *store.Store, label string, ago *time.Duration) {
	t.Helper()
	now := timeutil.NowUTC()
	var seen any
	if ago != nil {
		seen = timeutil.ToDBUTC(now.Add(-*ago))
	}
	if _, err := st.DB.Exec(`INSERT INTO runner (label, token_hash, revoked, last_seen_at, created_at) VALUES (?, 'x', 0, ?, ?)`,
		label, seen, timeutil.ToDBUTC(now)); err != nil {
		t.Fatal(err)
	}
}

func jobLabel(t *testing.T, st *store.Store) string {
	t.Helper()
	jobs, err := st.Execution.ListRunnerJobs(bg(), execution.JobFilter{})
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs: %v %+v", err, jobs)
	}
	return jobs[0].RunnerLabel
}

// Several runners: the job goes to the first one that is online, not blindly to the first listed.
func TestMultiRunnerPicksFirstOnline(t *testing.T) {
	st := openRunnerChecksDB(t)
	away, fresh := 10*time.Minute, 5*time.Second
	seedRunner(t, st, "main-pc", &away)
	seedRunner(t, st, "laptop", &fresh)
	seedRunner(t, st, "phone", &fresh)
	seedQuest(t, st, []journal.Step{remoteStep("poll", "echo 1", "main-pc,laptop,phone", journal.RunModePoll)})

	NewCheckRunner(st, events.New()).Tick(bg())
	if got := jobLabel(t, st); got != "laptop" {
		t.Fatalf("job on %q, want laptop (main-pc is away)", got)
	}
}

// Nobody online: it waits for the first one; when another shows up it is moved there.
func TestMultiRunnerReroutesQueuedJob(t *testing.T) {
	st := openRunnerChecksDB(t)
	away := 10 * time.Minute
	seedRunner(t, st, "main-pc", &away)
	seedRunner(t, st, "laptop", &away)
	seedQuest(t, st, []journal.Step{remoteStep("poll", "echo 1", "main-pc,laptop", journal.RunModePoll)})

	NewCheckRunner(st, events.New()).Tick(bg())
	if got := jobLabel(t, st); got != "main-pc" {
		t.Fatalf("nobody online: job on %q, want the first (main-pc)", got)
	}

	if _, err := st.DB.Exec(`UPDATE runner SET last_seen_at = ? WHERE label = 'laptop'`, timeutil.ToDBUTC(timeutil.NowUTC())); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Execution.ExpireJobs(bg()); err != nil {
		t.Fatal(err)
	}
	if got := jobLabel(t, st); got != "laptop" {
		t.Fatalf("after laptop came online job on %q, want laptop", got)
	}
	job, err := st.Execution.ClaimJob(bg(), "laptop", time.Minute)
	if err != nil || job == nil {
		t.Fatalf("laptop must be able to claim it: %v %v", job, err)
	}
}

// One runner behaves exactly as before: no rerouting, whatever its state.
func TestSingleRunnerStaysPut(t *testing.T) {
	st := openRunnerChecksDB(t)
	away := 10 * time.Minute
	seedRunner(t, st, "home", &away)
	seedQuest(t, st, []journal.Step{remoteStep("poll", "echo 1", "home", journal.RunModePoll)})
	NewCheckRunner(st, events.New()).Tick(bg())
	if _, err := st.Execution.ExpireJobs(bg()); err != nil {
		t.Fatal(err)
	}
	if got := jobLabel(t, st); got != "home" {
		t.Fatalf("job on %q", got)
	}
}
