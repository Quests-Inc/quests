package schedule

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/store"
)

// emitJobValidFor is how long an emit_pool_command may wait for its device.
const emitJobValidFor = 24 * time.Hour

// emitJobValidity is how long a job may wait for its device. A manual emit or a test (periods `manual-…`, `test-…`) is
// waited for by somebody right now, so its job must not linger and run a day later.
func emitJobValidity(period string) time.Duration {
	if strings.HasPrefix(period, manualPeriodPrefix) || strings.HasPrefix(period, testPeriodPrefix) {
		return 3 * time.Minute
	}
	return emitJobValidFor
}

// takeLegacyEmit reads a template job that was queued before emit went through a graph (no run). The tick still pulls it:
// a device that never shows up leaves the job skipped, as the marker for the period, and a finished job is taken once.
func takeLegacyEmit(ctx context.Context, st *store.Store, tmpl templateRow, job execution.RunnerJob) (out emitOutput, info execInfo, err error, ready bool) {
	label := strings.ToLower(strings.TrimSpace(tmpl.EmitRunOn.String))
	info.Status = "error"
	switch job.State {
	case execution.JobQueued, execution.JobAssigned, execution.JobRunning, execution.JobLost:
		return emitOutput{}, info, nil, false // still waiting for the device (or a late result)
	case execution.JobSkipped, execution.JobCanceled:
		return emitOutput{}, info, nil, false // the period passes without it; no retry storm
	}
	if derr := st.Execution.DeleteRunnerJob(ctx, job.ID); derr != nil {
		log.Printf("emit remote: delete job %d: %v", job.ID, derr)
	}
	if job.StartedAt != nil && job.FinishedAt != nil {
		info.Duration = job.FinishedAt.Sub(*job.StartedAt)
	}
	info.Stderr = truncate(job.Stderr, 4000)
	switch {
	case job.State == execution.JobAbandoned:
		return emitOutput{}, info, fmt.Errorf("устройство «%s» пропало во время выполнения, результат неизвестен", label), true
	case job.State == execution.JobTimeout || job.TimedOut:
		info.Status = "timeout"
		return emitOutput{}, info, fmt.Errorf("timeout on %q\nstderr:\n%s", label, info.Stderr), true
	case job.ExitCode != nil && *job.ExitCode != 0:
		return emitOutput{}, info, fmt.Errorf("exit status %d on %q\nstderr:\n%s", *job.ExitCode, label, info.Stderr), true
	}
	parsed, perr := parseEmitOutput([]byte(job.Stdout))
	if perr != nil {
		info.Status = "bad_json"
		return emitOutput{}, info, fmt.Errorf("%s\nstdout:\n%s", perr.Error(), truncate(job.Stdout, 4000)), true
	}
	info.Status = "ok"
	return parsed, info, nil, true
}

// poolJobKey identifies one emit run: the template, the period and the command text, so a changed command
// is a new job and a repeated tick finds the old one.
func poolJobKey(templateID int64, periodKey, command string) string {
	sum := sha256.Sum256([]byte(command))
	return strconv.FormatInt(templateID, 10) + "|" + periodKey + "|" + hex.EncodeToString(sum[:8])
}
