package schedule

import (
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/automation"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/store"
)

func openRunnerChecksDB(t *testing.T) *store.Store {
	t.Helper()
	return openChecksDB(t)
}

func remoteStep(title, cmd, label, mode string) journal.Step {
	iv := 15
	step := journal.Step{Title: title, ProgressTotal: 5, CheckCommand: &cmd, RunOn: &label, RunMode: mode}
	if mode == journal.RunModePoll {
		step.CheckIntervalSeconds = &iv
	}
	if mode == journal.RunModeOnce {
		step.ProgressTotal = 1
	}
	return step
}

// A step with run_on queues a job for its runner instead of running here, and
// does not pile up jobs while the device is away.
func TestRemotePollQueuesOneJobAndAppliesTheResult(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := bg()
	q := seedQuest(t, st, []journal.Step{remoteStep("poll", "echo 3", "home", journal.RunModePoll)})
	r := NewCheckRunner(st, events.New())

	r.Tick(ctx)
	r.Wait()
	jobs, _ := st.Automation.ListRunnerJobs(ctx, automation.JobFilter{Label: "home"})
	if len(jobs) != 1 || jobs[0].State != automation.JobQueued || jobs[0].Command != "echo 3" {
		t.Fatalf("jobs after the first tick: %+v", jobs)
	}
	got, _ := st.Journal.GetQuest(ctx, q.ID)
	if got.Steps[0].ProgressCurrent != 0 {
		t.Fatalf("nothing ran here, progress=%d", got.Steps[0].ProgressCurrent)
	}

	// The device is offline: later ticks (past the interval) add no jobs.
	if _, err := st.DB.Exec(`UPDATE queststep SET check_last_run_at = NULL`); err != nil {
		t.Fatal(err)
	}
	r.Tick(ctx)
	r.Wait()
	if jobs, _ = st.Automation.ListRunnerJobs(ctx, automation.JobFilter{Label: "home"}); len(jobs) != 1 {
		t.Fatalf("jobs piled up while offline: %d", len(jobs))
	}

	// The runner comes back, runs it, and the result lands on the step.
	job, err := st.Automation.ClaimJob(ctx, "home", time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim: %v %v", job, err)
	}
	done, _, err := st.Automation.CompleteJob(ctx, job.ID, job.LeaseID, automation.JobResult{Exit: 0, Stdout: "3\n"})
	if err != nil {
		t.Fatal(err)
	}
	r.JobFinished(ctx, done)
	got, _ = st.Journal.GetQuest(ctx, q.ID)
	if got.Steps[0].ProgressCurrent != 3 {
		t.Fatalf("progress=%d want 3", got.Steps[0].ProgressCurrent)
	}
}

func TestRemoteOnceWaitsThenReleasesWithoutFailingTheQuest(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := bg()
	idle := journal.RunIdle
	step := remoteStep("once", "true", "laptop", journal.RunModeOnce)
	step.RunStatus = &idle
	q := seedQuest(t, st, []journal.Step{step})
	r := NewCheckRunner(st, events.New())
	r.Tick(ctx)
	r.Wait()

	got, _ := st.Journal.GetQuest(ctx, q.ID)
	if got.Steps[0].RunStatus == nil || *got.Steps[0].RunStatus != journal.RunRunning {
		t.Fatalf("run_status=%v, want running (queued for the device)", got.Steps[0].RunStatus)
	}
	// Nobody picked it up for a day: the step is released, the quest stays active.
	if _, err := st.DB.Exec(`UPDATE runner_job SET valid_until = '2000-01-01 00:00:00.000000'`); err != nil {
		t.Fatal(err)
	}
	r.ExpireRunnerJobs(ctx)
	got, _ = st.Journal.GetQuest(ctx, q.ID)
	if got.Status != journal.StatusActive {
		t.Fatalf("quest status=%s: an unreachable device must not fail it", got.Status)
	}
	if rs := got.Steps[0].RunStatus; rs != nil && *rs != journal.RunIdle {
		t.Fatalf("run_status=%v, want released (idle) so it is queued again", *rs)
	}
}

func TestRemoteOnceLostMidRunFailsClearly(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := bg()
	idle := journal.RunIdle
	step := remoteStep("once", "true", "laptop", journal.RunModeOnce)
	step.RunStatus = &idle
	q := seedQuest(t, st, []journal.Step{step})
	r := NewCheckRunner(st, events.New())
	r.Tick(ctx)
	r.Wait()

	job, _ := st.Automation.ClaimJob(ctx, "laptop", time.Minute)
	if job == nil {
		t.Fatal("no job")
	}
	// The runner vanishes: lease lapses (lost), then the deadline passes (abandoned).
	if _, err := st.DB.Exec(`UPDATE runner_job SET lease_until = '2000-01-01 00:00:00.000000'`); err != nil {
		t.Fatal(err)
	}
	r.ExpireRunnerJobs(ctx)
	got, _ := st.Journal.GetQuest(ctx, q.ID)
	if got.Status != journal.StatusActive {
		t.Fatalf("lost lease alone must not fail the quest, status=%s", got.Status)
	}
	if _, err := st.DB.Exec(`UPDATE runner_job SET valid_until = '2000-01-01 00:00:00.000000'`); err != nil {
		t.Fatal(err)
	}
	r.ExpireRunnerJobs(ctx)
	got, _ = st.Journal.GetQuest(ctx, q.ID)
	if got.Status != journal.StatusFailed {
		t.Fatalf("status=%s want failed (result unknown, not re-run)", got.Status)
	}
}

func TestRemoteStepSurvivesServerRestart(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := bg()
	running := journal.RunRunning
	step := remoteStep("once", "true", "laptop", journal.RunModeOnce)
	step.RunStatus = &running
	q := seedQuest(t, st, []journal.Step{step})
	NewCheckRunner(st, events.New()).FailStaleRunning(ctx)
	got, _ := st.Journal.GetQuest(ctx, q.ID)
	if got.Status != journal.StatusActive {
		t.Fatalf("a job on a device outlives the server restart, status=%s", got.Status)
	}
}

// Deleting the quest (or step) takes its jobs out of the queue; a running one is told to stop.
func TestDeletedQuestCancelsItsRunnerJobs(t *testing.T) {
	st := openRunnerChecksDB(t)
	ctx := bg()
	idle := journal.RunIdle
	a := remoteStep("waits", "true", "ghost", journal.RunModeOnce)
	a.RunStatus = &idle
	b := remoteStep("runs", "true", "home", journal.RunModeOnce)
	b.RunStatus = &idle
	q := seedQuest(t, st, []journal.Step{a, b})
	r := NewCheckRunner(st, events.New())
	r.Tick(ctx)
	r.Wait()
	if job, _ := st.Automation.ClaimJob(ctx, "home", time.Minute); job == nil {
		t.Fatal("no running job")
	}

	if err := st.Journal.DeleteQuest(ctx, q.ID, q.Title); err != nil {
		t.Fatal(err)
	}
	r.ExpireRunnerJobs(ctx)

	jobs, _ := st.Automation.ListRunnerJobs(ctx, automation.JobFilter{})
	if len(jobs) != 2 {
		t.Fatalf("jobs: %+v", jobs)
	}
	for _, j := range jobs {
		switch j.RunnerLabel {
		case "ghost":
			if j.State != automation.JobCanceled {
				t.Fatalf("waiting job of a deleted quest: %s", j.State)
			}
		case "home":
			if !j.Cancel {
				t.Fatalf("running job was not asked to stop: %+v", j)
			}
		}
	}
}
