package schedule

import (
	"bytes"
	"os/exec"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/automation"
	"github.com/quests-inc/quests/go/internal/store"
)

// testLabel is the runner label the tests' commands are queued for.
const testLabel = "dev"

// runQueuedJobs plays the device: it claims every queued job of testLabel, runs its command with
// `sh -c` the way quests-runner would, completes the job and, when a CheckRunner is given, hands the
// result back to it (the server does the same when a runner reports). It returns how many jobs ran.
func runQueuedJobs(t *testing.T, st *store.Store, r *CheckRunner) int {
	t.Helper()
	ctx := bg()
	n := 0
	for {
		job, err := st.Automation.ClaimJob(ctx, testLabel, time.Minute)
		if err != nil {
			t.Fatalf("claim: %v", err)
		}
		if job == nil {
			return n
		}
		var stdout, stderr bytes.Buffer
		cmd := exec.Command("sh", "-c", job.Command)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		exit := 0
		if err := cmd.Run(); err != nil {
			exit = -1
			if ee, ok := err.(*exec.ExitError); ok {
				exit = ee.ExitCode()
			}
		}
		done, _, err := st.Automation.CompleteJob(ctx, job.ID, job.LeaseID, automation.JobResult{Exit: exit, Stdout: stdout.String(), Stderr: stderr.String()})
		if err != nil {
			t.Fatalf("complete job %d: %v", job.ID, err)
		}
		if r != nil {
			r.JobFinished(ctx, done)
		}
		n++
	}
}

// settle waits for the CheckRunner's own goroutines, then lets the fake device run what was queued.
func settle(t *testing.T, st *store.Store, r *CheckRunner) {
	t.Helper()
	r.Wait()
	runQueuedJobs(t, st, r)
}
