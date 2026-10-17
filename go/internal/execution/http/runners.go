package executionhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/httpx"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// Runner protocol. A runner only makes outgoing requests:
//
//	POST /api/runner/register              registration token → permanent token
//	GET  /api/runner/jobs?wait=25          long-poll for a job (200 job / 204 nothing)
//	POST /api/runner/jobs/{id}/heartbeat   keep the lease, report progress / log tail
//	POST /api/runner/jobs/{id}/complete    deliver the result
//
// Everything but register carries `Authorization: Bearer <runner token>`.
// Users manage runners through /api/runners and /api/runner-jobs (normal auth).

const (
	// runnerLease is how long a job stays leased without a heartbeat.
	runnerLease = 90 * time.Second
	// runnerMaxWait caps one long-poll.
	runnerMaxWait = 30 * time.Second
	// runnerPollStep is how often a waiting long-poll looks at the queue.
	runnerPollStep = time.Second
)

func (h *Handler) registerRunner(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/runner/register", h.runnerRegister)
	mux.HandleFunc("GET /api/runner/jobs", h.runnerPoll)
	mux.HandleFunc("POST /api/runner/jobs/{id}/heartbeat", h.runnerHeartbeat)
	mux.HandleFunc("POST /api/runner/jobs/{id}/complete", h.runnerComplete)

	mux.HandleFunc("GET /api/runners", h.listRunners)
	mux.HandleFunc("POST /api/runners", h.createRunner)
	mux.HandleFunc("DELETE /api/runners/{label}", h.deleteRunner)
	mux.HandleFunc("GET /api/runner-jobs", h.listRunnerJobs)
	mux.HandleFunc("POST /api/runner-jobs/{id}/cancel", h.cancelRunnerJob)
}

func (h *Handler) runnerFrom(w http.ResponseWriter, r *http.Request) (execution.Runner, bool) {
	run, err := h.Store.AuthRunner(r.Context(), httpx.BearerToken(r))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid runner token")
		return execution.Runner{}, false
	}
	return run, true
}

func (h *Handler) runnerRegister(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"registration_token"`
		Hostname string `json:"hostname"`
		Version  string `json:"version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	ctx := r.Context()
	ws, werr := h.Store.RunnerWorkspaceByReg(ctx, strings.TrimSpace(in.Token))
	if werr != nil {
		writeErr(w, http.StatusUnauthorized, werr.Error())
		return
	}
	ctx = wsctx.With(ctx, ws)

	token, run, err := h.Store.RegisterRunner(ctx, strings.TrimSpace(in.Token), in.Hostname, in.Version)
	if errors.Is(err, execution.ErrRunnerToken) {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"token": token, "label": run.Label})
}

// jobPayload is what a runner receives: the command with the secrets it may use.
type jobPayload struct {
	ID             int64             `json:"id"`
	LeaseID        string            `json:"lease_id"`
	LeaseSeconds   int               `json:"lease_seconds"`
	Kind           string            `json:"kind"`
	Title          string            `json:"title"`
	Command        string            `json:"command"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	AttemptNo      int               `json:"attempt_no"`
	QuestID        *int64            `json:"quest_id,omitempty"`
	StepID         *int64            `json:"step_id,omitempty"`
	TemplateID     *int64            `json:"template_id,omitempty"`
	NoteID         *int64            `json:"note_id,omitempty"`
	Env            map[string]string `json:"env"`
}

// jobSecrets resolves the secrets of what the job belongs to (step, else template).
func (h *Handler) jobSecrets(ctx context.Context, j execution.RunnerJob) map[string]string {
	var owner execution.SecretOwner
	switch {
	case j.StepID != nil:
		owner = execution.SecretOwner{Kind: execution.SecretOwnerStep, ID: *j.StepID}
	case j.TemplateID != nil:
		owner = execution.SecretOwner{Kind: execution.SecretOwnerTemplate, ID: *j.TemplateID}
	case j.NoteID != nil:
		owner = execution.SecretOwner{Kind: execution.SecretOwnerNote, ID: *j.NoteID}
	default:
		return map[string]string{}
	}
	m, err := h.Store.ResolveSecrets(ctx, owner)
	if err != nil || m == nil {
		return map[string]string{}
	}
	return m
}

func (h *Handler) runnerPoll(w http.ResponseWriter, r *http.Request) {
	run, ok := h.runnerFrom(w, r)
	if !ok {
		return
	}
	wait := 25 * time.Second
	if v, err := strconv.Atoi(r.URL.Query().Get("wait")); err == nil && v >= 0 {
		wait = time.Duration(v) * time.Second
	}
	if wait > runnerMaxWait {
		wait = runnerMaxWait
	}
	deadline := time.Now().Add(wait)
	for {
		job, err := h.Store.ClaimJob(r.Context(), run.Label, runnerLease)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if job != nil {
			writeJSON(w, 200, jobPayload{
				ID: job.ID, LeaseID: job.LeaseID, LeaseSeconds: int(runnerLease / time.Second),
				Kind: job.Kind, Title: job.Title, Command: job.Command, TimeoutSeconds: job.TimeoutSeconds,
				AttemptNo: job.AttemptNo, QuestID: job.QuestID, StepID: job.StepID, TemplateID: job.TemplateID, NoteID: job.NoteID,
				Env: h.jobSecrets(r.Context(), *job),
			})
			return
		}
		if !time.Now().Add(runnerPollStep).Before(deadline) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(runnerPollStep):
			// Count the wait as a sign of life too.
			if _, err := h.Store.AuthRunner(r.Context(), httpx.BearerToken(r)); err != nil {
				writeErr(w, http.StatusUnauthorized, "invalid runner token")
				return
			}
		}
	}
}

func (h *Handler) runnerHeartbeat(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.runnerFrom(w, r); !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		LeaseID         string `json:"lease_id"`
		ProgressCurrent *int   `json:"progress_current"`
		ProgressTotal   *int   `json:"progress_total"`
		LogTail         string `json:"log_tail"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	var prog *[2]int
	if in.ProgressCurrent != nil && in.ProgressTotal != nil && *in.ProgressTotal >= 1 {
		prog = &[2]int{*in.ProgressCurrent, *in.ProgressTotal}
	}
	job, gerr := h.Store.GetRunnerJob(r.Context(), id)
	tail := in.LogTail
	if gerr == nil {
		tail = execution.MaskSecrets(tail, h.jobSecrets(r.Context(), job))
	}
	cancel, err := h.Store.JobHeartbeat(r.Context(), id, in.LeaseID, prog, tail, runnerLease)
	if errors.Is(err, execution.ErrLeaseLost) {
		writeErr(w, http.StatusConflict, "lease lost")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"cancel": cancel, "lease_seconds": int(runnerLease / time.Second)})
}

func (h *Handler) runnerComplete(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.runnerFrom(w, r); !ok {
		return
	}
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var in struct {
		LeaseID  string `json:"lease_id"`
		Exit     int    `json:"exit"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		TimedOut bool   `json:"timed_out"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	cur, err := h.Store.GetRunnerJob(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "job not found")
		return
	}
	secrets := h.jobSecrets(r.Context(), cur)
	job, done, err := h.Store.CompleteJob(r.Context(), id, in.LeaseID, execution.JobResult{
		Exit:     in.Exit,
		Stdout:   execution.MaskSecrets(in.Stdout, secrets),
		Stderr:   execution.MaskSecrets(in.Stderr, secrets),
		TimedOut: in.TimedOut,
	})
	if errors.Is(err, execution.ErrLeaseLost) {
		writeErr(w, http.StatusConflict, "lease lost")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if done && h.RunnerDone != nil {
		h.RunnerDone(r.Context(), job)
	}
	writeJSON(w, 200, map[string]any{"ok": true, "accepted": done})
}

// --- user-facing management ---

func (h *Handler) listRunners(w http.ResponseWriter, r *http.Request) {
	runners, err := h.Store.ListRunners(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	jobs, _ := h.Store.ListRunnerJobs(r.Context(), execution.JobFilter{States: []string{execution.JobQueued, execution.JobAssigned, execution.JobRunning}, Limit: 500})
	now := timeutil.NowUTC()
	out := make([]RunnerView, 0, len(runners))
	for _, run := range runners {
		v := RunnerView{Runner: run, Online: execution.RunnerOnline(run, now)}
		for _, j := range jobs {
			if j.RunnerLabel != run.Label {
				continue
			}
			if j.State == execution.JobQueued {
				v.Queued++
			} else {
				v.Running++
			}
		}
		out = append(out, v)
	}
	writeJSON(w, 200, out)
}

func (h *Handler) createRunner(w http.ResponseWriter, r *http.Request) {
	// A runner executes the workspace's commands (and receives their secrets).
	if !h.RequireOwner(w, r, "add a runner") {
		return
	}
	var in struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	token, err := h.Store.CreateRunner(r.Context(), in.Label)
	switch {
	case errors.Is(err, kernel.ErrBadRunnerLabel):
		writeErr(w, 422, err.Error())
	case errors.Is(err, execution.ErrRunnerExists):
		writeErr(w, 409, err.Error())
	case err != nil:
		writeErr(w, 500, err.Error())
	default:
		writeJSON(w, 201, map[string]any{"label": strings.TrimSpace(in.Label), "registration_token": token, "valid_hours": int(execution.RegistrationTTL / time.Hour)})
	}
}

func (h *Handler) deleteRunner(w http.ResponseWriter, r *http.Request) {
	if !h.RequireOwner(w, r, "remove a runner") {
		return
	}
	err := h.Store.RevokeRunner(r.Context(), r.PathValue("label"))
	if errors.Is(err, execution.ErrNotFound) {
		writeErr(w, 404, "runner not found")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (h *Handler) listRunnerJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := execution.JobFilter{Label: q.Get("label")}
	if v := q.Get("state"); v != "" {
		f.States = strings.Split(v, ",")
	}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil {
		f.Limit = v
	}
	if v, err := strconv.ParseInt(q.Get("step_id"), 10, 64); err == nil {
		f.StepID = v
	}
	jobs, err := h.Store.ListRunnerJobs(r.Context(), f)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, jobs)
}

func (h *Handler) cancelRunnerJob(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	err := h.Store.CancelRunnerJob(r.Context(), id)
	if errors.Is(err, execution.ErrNotFound) {
		writeErr(w, 404, "no such unfinished job")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}
