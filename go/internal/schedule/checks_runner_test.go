package schedule

import (
	"context"
	"testing"
	"time"

	"github.com/valirum/quests/go/internal/domain"
	"github.com/valirum/quests/go/internal/events"
	"github.com/valirum/quests/go/internal/store"
)

const runnerJobSchema = `
CREATE TABLE runner (
	id INTEGER PRIMARY KEY, label TEXT NOT NULL UNIQUE, reg_hash TEXT, reg_expires_at DATETIME, token_hash TEXT,
	hostname TEXT, version TEXT, last_seen_at DATETIME, revoked BOOLEAN NOT NULL DEFAULT 0, created_at DATETIME NOT NULL
);
CREATE TABLE runner_job (
	id INTEGER PRIMARY KEY, runner_label TEXT NOT NULL, kind TEXT NOT NULL, quest_id INTEGER, step_id INTEGER, template_id INTEGER,
	title TEXT NOT NULL DEFAULT '', dedupe_key TEXT, command TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'queued',
	attempt_no INTEGER NOT NULL DEFAULT 1, timeout_seconds INTEGER NOT NULL, lease_id TEXT, lease_until DATETIME,
	valid_until DATETIME NOT NULL, created_at DATETIME NOT NULL, assigned_at DATETIME, started_at DATETIME, finished_at DATETIME,
	exit_code INTEGER, timed_out BOOLEAN NOT NULL DEFAULT 0, stdout TEXT NOT NULL DEFAULT '', stderr TEXT NOT NULL DEFAULT '',
	progress_current INTEGER, progress_total INTEGER, log_tail TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '',
	cancel BOOLEAN NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX ux_runner_job_open_dedupe ON runner_job (dedupe_key)
	WHERE dedupe_key IS NOT NULL AND state IN ('queued','assigned','running');
`

func openRunnerChecksDB(t *testing.T) *store.Store {
	t.Helper()
	st := openChecksDB(t)
	if _, err := st.DB.Exec(runnerJobSchema); err != nil {
		t.Fatal(err)
	}
	return st
}

func remoteStep(title, cmd, label, mode string) domain.Step {
	iv := 15
	step := domain.Step{Title: title, ProgressTotal: 5, CheckCommand: &cmd, RunOn: &label, RunMode: mode}
	if mode == domain.RunModePoll {
		step.CheckIntervalSeconds = &iv
	}
	if mode == domain.RunModeOnce {
		step.ProgressTotal = 1
	}
	return step
}

// A step with run_on queues a job for its runner instead of running here, and
// does not pile up jobs while the device is away.
func TestRemotePollQueuesOneJobAndAppliesTheResult(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := context.Background()
	q := seedQuest(t, st, []domain.Step{remoteStep("poll", "echo 3", "home", domain.RunModePoll)})
	r := NewCheckRunner(st, events.New())

	r.Tick(ctx)
	r.Wait()
	jobs, _ := st.ListRunnerJobs(ctx, store.JobFilter{Label: "home"})
	if len(jobs) != 1 || jobs[0].State != store.JobQueued || jobs[0].Command != "echo 3" {
		t.Fatalf("jobs after the first tick: %+v", jobs)
	}
	got, _ := st.GetQuest(ctx, q.ID)
	if got.Steps[0].ProgressCurrent != 0 {
		t.Fatalf("nothing ran here, progress=%d", got.Steps[0].ProgressCurrent)
	}

	// The device is offline: later ticks (past the interval) add no jobs.
	if _, err := st.DB.Exec(`UPDATE queststep SET check_last_run_at = NULL`); err != nil {
		t.Fatal(err)
	}
	r.Tick(ctx)
	r.Wait()
	if jobs, _ = st.ListRunnerJobs(ctx, store.JobFilter{Label: "home"}); len(jobs) != 1 {
		t.Fatalf("jobs piled up while offline: %d", len(jobs))
	}

	// The runner comes back, runs it, and the result lands on the step.
	job, err := st.ClaimJob(ctx, "home", time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim: %v %v", job, err)
	}
	done, _, err := st.CompleteJob(ctx, job.ID, job.LeaseID, store.JobResult{Exit: 0, Stdout: "3\n"})
	if err != nil {
		t.Fatal(err)
	}
	r.JobFinished(ctx, done)
	got, _ = st.GetQuest(ctx, q.ID)
	if got.Steps[0].ProgressCurrent != 3 {
		t.Fatalf("progress=%d want 3", got.Steps[0].ProgressCurrent)
	}
}

func TestRemoteOnceWaitsThenReleasesWithoutFailingTheQuest(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := context.Background()
	idle := domain.RunIdle
	step := remoteStep("once", "true", "laptop", domain.RunModeOnce)
	step.RunStatus = &idle
	q := seedQuest(t, st, []domain.Step{step})
	r := NewCheckRunner(st, events.New())
	r.Tick(ctx)
	r.Wait()

	got, _ := st.GetQuest(ctx, q.ID)
	if got.Steps[0].RunStatus == nil || *got.Steps[0].RunStatus != domain.RunRunning {
		t.Fatalf("run_status=%v, want running (queued for the device)", got.Steps[0].RunStatus)
	}
	// Nobody picked it up for a day: the step is released, the quest stays active.
	if _, err := st.DB.Exec(`UPDATE runner_job SET valid_until = '2000-01-01 00:00:00.000000'`); err != nil {
		t.Fatal(err)
	}
	r.ExpireRunnerJobs(ctx)
	got, _ = st.GetQuest(ctx, q.ID)
	if got.Status != domain.StatusActive {
		t.Fatalf("quest status=%s: an unreachable device must not fail it", got.Status)
	}
	if rs := got.Steps[0].RunStatus; rs != nil && *rs != domain.RunIdle {
		t.Fatalf("run_status=%v, want released (idle) so it is queued again", *rs)
	}
}

func TestRemoteOnceLostMidRunFailsClearly(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := context.Background()
	idle := domain.RunIdle
	step := remoteStep("once", "true", "laptop", domain.RunModeOnce)
	step.RunStatus = &idle
	q := seedQuest(t, st, []domain.Step{step})
	r := NewCheckRunner(st, events.New())
	r.Tick(ctx)
	r.Wait()

	job, _ := st.ClaimJob(ctx, "laptop", time.Minute)
	if job == nil {
		t.Fatal("no job")
	}
	// The runner vanishes: lease lapses (lost), then the deadline passes (abandoned).
	if _, err := st.DB.Exec(`UPDATE runner_job SET lease_until = '2000-01-01 00:00:00.000000'`); err != nil {
		t.Fatal(err)
	}
	r.ExpireRunnerJobs(ctx)
	got, _ := st.GetQuest(ctx, q.ID)
	if got.Status != domain.StatusActive {
		t.Fatalf("lost lease alone must not fail the quest, status=%s", got.Status)
	}
	if _, err := st.DB.Exec(`UPDATE runner_job SET valid_until = '2000-01-01 00:00:00.000000'`); err != nil {
		t.Fatal(err)
	}
	r.ExpireRunnerJobs(ctx)
	got, _ = st.GetQuest(ctx, q.ID)
	if got.Status != domain.StatusFailed {
		t.Fatalf("status=%s want failed (result unknown, not re-run)", got.Status)
	}
}

func TestRemoteStepSurvivesServerRestart(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := context.Background()
	running := domain.RunRunning
	step := remoteStep("once", "true", "laptop", domain.RunModeOnce)
	step.RunStatus = &running
	q := seedQuest(t, st, []domain.Step{step})
	NewCheckRunner(st, events.New()).FailStaleRunning(ctx)
	got, _ := st.GetQuest(ctx, q.ID)
	if got.Status != domain.StatusActive {
		t.Fatalf("a job on a device outlives the server restart, status=%s", got.Status)
	}
}

// Deleting the quest (or step) takes its jobs out of the queue; a running one is told to stop.
func TestDeletedQuestCancelsItsRunnerJobs(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := context.Background()
	idle := domain.RunIdle
	a := remoteStep("waits", "true", "ghost", domain.RunModeOnce)
	a.RunStatus = &idle
	b := remoteStep("runs", "true", "home", domain.RunModeOnce)
	b.RunStatus = &idle
	q := seedQuest(t, st, []domain.Step{a, b})
	r := NewCheckRunner(st, events.New())
	r.Tick(ctx)
	r.Wait()
	if job, _ := st.ClaimJob(ctx, "home", time.Minute); job == nil {
		t.Fatal("no running job")
	}

	if err := st.DeleteQuest(ctx, q.ID, q.Title); err != nil {
		t.Fatal(err)
	}
	r.ExpireRunnerJobs(ctx)

	jobs, _ := st.ListRunnerJobs(ctx, store.JobFilter{})
	if len(jobs) != 2 {
		t.Fatalf("jobs: %+v", jobs)
	}
	for _, j := range jobs {
		switch j.RunnerLabel {
		case "ghost":
			if j.State != store.JobCanceled {
				t.Fatalf("waiting job of a deleted quest: %s", j.State)
			}
		case "home":
			if !j.Cancel {
				t.Fatalf("running job was not asked to stop: %+v", j)
			}
		}
	}
}
