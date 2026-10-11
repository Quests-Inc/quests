package schedule

import (
	"strings"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/automation"
	"github.com/quests-inc/quests/go/internal/store"
)

func openRemoteEmitDB(t *testing.T) *store.Store {
	t.Helper()
	return openEmitPoolDB(t)
}

func remoteTemplate(command, label string) templateRow {
	t := poolTemplate(7, command)
	t.EmitRunOn.String, t.EmitRunOn.Valid = label, true
	t.Title = "Noctalia"
	return t
}

// The command is queued for the device, nothing runs here, and the printed
// quest is picked up on a later tick once the runner delivered it.
func TestEmitRemoteQueuesThenCollects(t *testing.T) {
	st := openRemoteEmitDB(t)
	ctx := bg()
	tmpl := remoteTemplate(`#!/bin/sh
echo '{"title":"Вернуться на пакет"}'`, "home")

	first := emitTick(t, st, tmpl, "2026-10-06", time.Now())
	if first.Spec != nil || first.FailMsg != "" {
		t.Fatalf("first tick must only queue: %+v", first)
	}
	jobs, _ := st.Automation().ListRunnerJobs(ctx, automation.JobFilter{Label: "home"})
	if len(jobs) != 1 || jobs[0].Kind != automation.JobKindEmitPool || jobs[0].TemplateID == nil || *jobs[0].TemplateID != 7 || !strings.HasPrefix(jobs[0].Command, "#!/bin/sh") {
		t.Fatalf("queued jobs: %+v", jobs)
	}
	// Ticks while the device is away neither queue again nor fail the template.
	for i := 0; i < 3; i++ {
		if r := emitTick(t, st, tmpl, "2026-10-06", time.Now()); r.Spec != nil || r.FailMsg != "" {
			t.Fatalf("waiting tick %d: %+v", i, r)
		}
	}
	if jobs, _ = st.Automation().ListRunnerJobs(ctx, automation.JobFilter{Label: "home"}); len(jobs) != 1 {
		t.Fatalf("jobs piled up: %d", len(jobs))
	}

	// The runner runs it and delivers.
	job, _ := st.Automation().ClaimJob(ctx, "home", time.Minute)
	if job == nil {
		t.Fatal("no job to claim")
	}
	if _, _, err := st.Automation().CompleteJob(ctx, job.ID, job.LeaseID, automation.JobResult{Exit: 0, Stdout: `{"title":"Вернуться на пакет"}`}); err != nil {
		t.Fatal(err)
	}
	got := emitTick(t, st, tmpl, "2026-10-06", time.Now())
	if got.Spec == nil || got.Spec.Title != "Вернуться на пакет" {
		t.Fatalf("result not collected: %+v", got)
	}
	// Handed out once: the finished job is gone.
	if jobs, _ = st.Automation().ListRunnerJobs(ctx, automation.JobFilter{Label: "home"}); len(jobs) != 0 {
		t.Fatalf("job kept after collecting: %+v", jobs)
	}
}

func TestEmitRemoteFailureRetriesLikeALocalOne(t *testing.T) {
	st := openRemoteEmitDB(t)
	ctx := bg()
	tmpl := remoteTemplate(`exit 3`, "home")
	emitTick(t, st, tmpl, "p", time.Now())
	job, _ := st.Automation().ClaimJob(ctx, "home", time.Minute)
	_, _, _ = st.Automation().CompleteJob(ctx, job.ID, job.LeaseID, automation.JobResult{Exit: 3, Stderr: "no pacman"})
	res := emitTick(t, st, tmpl, "p", time.Now())
	var outcome string
	var attempts int
	if err := st.DB.QueryRow(`SELECT outcome, attempts FROM templateemitroll WHERE id = ?`, res.RollID).Scan(&outcome, &attempts); err != nil {
		t.Fatal(err)
	}
	if outcome != "scheduled" || attempts != 1 {
		t.Fatalf("outcome=%s attempts=%d, want a scheduled retry after the first failure", outcome, attempts)
	}
	var msg string
	_ = st.DB.QueryRow(`SELECT message FROM templateemitattempt ORDER BY id DESC LIMIT 1`).Scan(&msg)
	if !strings.Contains(msg, "no pacman") {
		t.Fatalf("stderr of the device missing from the attempt log: %q", msg)
	}
}

// A device that never came online leaves the job skipped: no retry storm, no error outcome.
func TestEmitRemoteOfflineDeviceDoesNotFailTheTemplate(t *testing.T) {
	st := openRemoteEmitDB(t)
	ctx := bg()
	tmpl := remoteTemplate(`echo '{"title":"x"}'`, "laptop")
	emitTick(t, st, tmpl, "p", time.Now())
	if _, err := st.DB.Exec(`UPDATE runner_job SET valid_until = '2000-01-01 00:00:00.000000'`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Automation().ExpireJobs(ctx); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if r := emitTick(t, st, tmpl, "p", time.Now()); r.Spec != nil || r.FailMsg != "" {
			t.Fatalf("tick %d after skip: %+v", i, r)
		}
	}
	jobs, _ := st.Automation().ListRunnerJobs(ctx, automation.JobFilter{Label: "laptop"})
	if len(jobs) != 1 || jobs[0].State != automation.JobSkipped {
		t.Fatalf("jobs: %+v", jobs)
	}
	var n int
	_ = st.DB.QueryRow(`SELECT COUNT(*) FROM templateemitroll`).Scan(&n)
	if n != 0 {
		t.Fatalf("an unreachable device wrote a roll row (%d): the period must stay untouched", n)
	}
}
