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

	"github.com/quests-inc/quests/go/internal/automation"
	"github.com/quests-inc/quests/go/internal/store"
)

// emitJobValidFor is how long an emit_pool_command may wait for its device.
const emitJobValidFor = 24 * time.Hour

// execEmitRemote runs a template's emit_pool_command on a runner. It is
// asynchronous like poolRunner: the first call queues a job and answers
// "not ready"; a later tick finds the finished job, takes its result (once —
// the job is deleted) and the usual validation / retry logic takes over.
//
// A device that never shows up leaves the job `skipped`: that is kept as a
// marker for the period, so nothing is re-queued and the template is not
// marked as failing. A device that vanishes mid-run counts as a failed attempt.
func execEmitRemote(ctx context.Context, st *store.Store, tmpl templateRow, periodKey string) (out emitOutput, info execInfo, err error, ready bool) {
	label := strings.ToLower(strings.TrimSpace(tmpl.EmitRunOn.String))
	command := tmpl.EmitPoolCommand.String
	key := "emit:" + poolJobKey(tmpl.ID, periodKey, command)
	info.Status = "error"

	job, found, lerr := st.Automation.LatestJobByKey(ctx, key)
	if lerr != nil {
		return emitOutput{}, info, lerr, true
	}
	if !found {
		tid := tmpl.ID
		if _, _, qerr := st.Automation.EnqueueJob(ctx, automation.JobSpec{
			RunnerLabel:    label,
			Kind:           automation.JobKindEmitPool,
			TemplateID:     &tid,
			Title:          "шаблон: " + tmpl.Title,
			DedupeKey:      key,
			Command:        command,
			TimeoutSeconds: int(emitPoolTimeout / time.Second),
			ValidFor:       emitJobValidFor,
		}); qerr != nil {
			log.Printf("emit remote: enqueue template %d: %v", tmpl.ID, qerr)
		}
		return emitOutput{}, info, nil, false
	}

	switch job.State {
	case automation.JobQueued, automation.JobAssigned, automation.JobRunning, automation.JobLost:
		return emitOutput{}, info, nil, false // still waiting for the device (or a late result)
	case automation.JobSkipped, automation.JobCanceled:
		return emitOutput{}, info, nil, false // the period passes without it; no retry storm
	}

	// A result (or a vanished device): take it, once.
	if derr := st.Automation.DeleteRunnerJob(ctx, job.ID); derr != nil {
		log.Printf("emit remote: delete job %d: %v", job.ID, derr)
	}
	if job.StartedAt != nil && job.FinishedAt != nil {
		info.Duration = job.FinishedAt.Sub(*job.StartedAt)
	}
	info.Stderr = truncate(job.Stderr, 4000)
	switch {
	case job.State == automation.JobAbandoned:
		return emitOutput{}, info, fmt.Errorf("устройство «%s» пропало во время выполнения, результат неизвестен", label), true
	case job.State == automation.JobTimeout || job.TimedOut:
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
