package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/kernel"
	"github.com/quests-inc/quests/go/internal/platform/db"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/tokens"
)

// Runner is a device that executes checks for us. It registers once with a
// one-time token, then only makes outgoing requests (long-poll for jobs).
type Runner struct {
	ID         int64      `json:"id"`
	Label      string     `json:"label"`
	Hostname   string     `json:"hostname"`
	Version    string     `json:"version"`
	Registered bool       `json:"registered"`
	Revoked    bool       `json:"revoked"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Job states.
const (
	JobQueued   = "queued"
	JobAssigned = "assigned"
	JobRunning  = "running"
	JobSuccess  = "success"
	JobFail     = "fail"
	JobTimeout  = "timeout"
	JobLost     = "lost"
	JobSkipped  = "skipped"
	// JobAbandoned: the runner went silent mid-run and never delivered a result in time.
	JobAbandoned = "abandoned"
	JobCanceled  = "canceled"
)

// Job kinds.
const (
	JobKindPoll     = "poll"
	JobKindOnce     = "once"
	JobKindEmitPool = "emit_pool"
)

// RunnerJob is one command for a runner.
type RunnerJob struct {
	ID              int64      `json:"id"`
	RunnerLabel     string     `json:"runner_label"`
	Kind            string     `json:"kind"`
	QuestID         *int64     `json:"quest_id"`
	StepID          *int64     `json:"step_id"`
	TemplateID      *int64     `json:"template_id"`
	NoteID          *int64     `json:"note_id"`
	Title           string     `json:"title"`
	DedupeKey       string     `json:"dedupe_key,omitempty"`
	Command         string     `json:"command"`
	State           string     `json:"state"`
	AttemptNo       int        `json:"attempt_no"`
	TimeoutSeconds  int        `json:"timeout_seconds"`
	LeaseID         string     `json:"lease_id,omitempty"`
	LeaseUntil      *time.Time `json:"lease_until"`
	ValidUntil      time.Time  `json:"valid_until"`
	CreatedAt       time.Time  `json:"created_at"`
	AssignedAt      *time.Time `json:"assigned_at"`
	StartedAt       *time.Time `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	ExitCode        *int       `json:"exit_code"`
	TimedOut        bool       `json:"timed_out"`
	Stdout          string     `json:"stdout"`
	Stderr          string     `json:"stderr"`
	ProgressCurrent *int       `json:"progress_current"`
	ProgressTotal   *int       `json:"progress_total"`
	LogTail         string     `json:"log_tail"`
	Reason          string     `json:"reason"`
	Cancel          bool       `json:"cancel"`
	// RunID / RunNode: the script run and the Code node this job is for (ADR-0003); nil for a job of the older kinds.
	RunID   *int64 `json:"run_id,omitempty"`
	RunNode *int   `json:"run_node,omitempty"`
	// Env is the values that came over links into the Code node. It goes to the runner with the secrets, not to the API.
	Env map[string]string `json:"-"`
}

// Open reports whether the job has not reached a final state.
func (j RunnerJob) Open() bool {
	return j.State == JobQueued || j.State == JobAssigned || j.State == JobRunning
}

var (
	ErrRunnerExists = errors.New("runner already exists")
	ErrRunnerToken  = errors.New("invalid or expired registration token")
	ErrRunnerAuth   = errors.New("invalid runner token")
	ErrLeaseLost    = errors.New("lease is no longer valid")
)

const runnerCols = `id, label, COALESCE(hostname,''), COALESCE(version,''), token_hash IS NOT NULL, revoked, last_seen_at, created_at`

func scanRunner(sc interface{ Scan(...any) error }) (Runner, error) {
	var r Runner
	var seen sql.NullString
	var created string
	if err := sc.Scan(&r.ID, &r.Label, &r.Hostname, &r.Version, &r.Registered, &r.Revoked, &seen, &created); err != nil {
		return Runner{}, err
	}
	if seen.Valid {
		t, _ := timeutil.ParseFlexible(seen.String)
		r.LastSeenAt = &t
	}
	r.CreatedAt, _ = timeutil.ParseFlexible(created)
	return r, nil
}

// CreateRunner adds a runner and returns its one-time registration token.
func (s *Store) CreateRunner(ctx context.Context, label string) (string, error) {
	label = strings.TrimSpace(label)
	if !kernel.ValidRunnerLabel(label) {
		return "", ErrBadRunnerLabel
	}
	secret, hash, err := tokens.New()
	if err != nil {
		return "", err
	}
	now := timeutil.NowUTC()
	_, err = s.DB.ExecContext(ctx,
		`INSERT INTO runner (label, reg_hash, reg_expires_at, revoked, created_at) VALUES (?, ?, ?, 0, ?)`,
		label, hash, timeutil.ToDBUTC(now.Add(RegistrationTTL)), timeutil.ToDBUTC(now))
	if err != nil {
		if db.IsUniqueViolation(err) {
			return "", ErrRunnerExists
		}
		return "", err
	}
	return secret, nil
}

// RegisterRunner swaps a registration token for the runner's permanent token.
func (s *Store) RegisterRunner(ctx context.Context, regToken, hostname, version string) (token string, r Runner, err error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+runnerCols+`, reg_expires_at FROM runner WHERE reg_hash = ? AND revoked = 0`, tokens.Hash(regToken))
	var seen sql.NullString
	var created string
	var exp sql.NullString
	if scanErr := row.Scan(&r.ID, &r.Label, &r.Hostname, &r.Version, &r.Registered, &r.Revoked, &seen, &created, &exp); scanErr != nil {
		return "", Runner{}, ErrRunnerToken
	}
	if exp.Valid {
		if t, perr := timeutil.ParseFlexible(exp.String); perr == nil && timeutil.NowUTC().After(t) {
			return "", Runner{}, ErrRunnerToken
		}
	}
	secret, hash, err := tokens.New()
	if err != nil {
		return "", Runner{}, err
	}
	now := timeutil.NowUTC()
	res, err := s.DB.ExecContext(ctx,
		`UPDATE runner SET token_hash = ?, reg_hash = NULL, reg_expires_at = NULL, hostname = ?, version = ?, last_seen_at = ? WHERE id = ? AND reg_hash IS NOT NULL`,
		hash, trimTo(hostname, 128), trimTo(version, 32), timeutil.ToDBUTC(now), r.ID)
	if err != nil {
		return "", Runner{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", Runner{}, ErrRunnerToken
	}
	r.Hostname, r.Version, r.Registered = trimTo(hostname, 128), trimTo(version, 32), true
	r.LastSeenAt = &now
	return secret, r, nil
}

// AuthRunner finds the runner behind a permanent token and records that it was just seen.
func (s *Store) AuthRunner(ctx context.Context, token string) (Runner, error) {
	if token == "" {
		return Runner{}, ErrRunnerAuth
	}
	r, err := scanRunner(s.DB.QueryRowContext(ctx, `SELECT `+runnerCols+` FROM runner WHERE token_hash = ? AND revoked = 0`, tokens.Hash(token)))
	if err != nil {
		return Runner{}, ErrRunnerAuth
	}
	now := timeutil.NowUTC()
	_, _ = s.DB.ExecContext(ctx, `UPDATE runner SET last_seen_at = ? WHERE id = ?`, timeutil.ToDBUTC(now), r.ID)
	r.LastSeenAt = &now
	return r, nil
}

func (s *Store) ListRunners(ctx context.Context) ([]Runner, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+runnerCols+` FROM runner ORDER BY label`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Runner{}
	for rows.Next() {
		r, err := scanRunner(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RevokeRunner stops a runner for good and cancels what it had queued.
func (s *Store) RevokeRunner(ctx context.Context, label string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE runner SET revoked = 1, token_hash = NULL, reg_hash = NULL WHERE label = ?`, label)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	_, err = s.DB.ExecContext(ctx,
		`UPDATE runner_job SET state = 'canceled', reason = 'раннер отозван', finished_at = ? WHERE runner_label = ? AND state IN ('queued','assigned','running')`,
		now, label)
	return err
}

// RunnerOnline: the runner polls every ~25s, so two missed polls mean it is away.
func RunnerOnline(r Runner, now time.Time) bool {
	return r.Registered && !r.Revoked && r.LastSeenAt != nil && now.Sub(*r.LastSeenAt) < 70*time.Second
}

// JobSpec is what the scheduler asks to run on a runner.
type JobSpec struct {
	RunnerLabel    string
	Kind           string
	QuestID        *int64
	StepID         *int64
	TemplateID     *int64
	NoteID         *int64
	Title          string
	DedupeKey      string // at most one unfinished job per key
	Command        string
	TimeoutSeconds int
	ValidFor       time.Duration // how long the job may wait for its runner
	// RunID / RunNode / Env: a Code node of a script run (ADR-0003) and its link values.
	RunID   *int64
	RunNode *int
	Env     map[string]string
}

const jobCols = `id, runner_label, kind, quest_id, step_id, template_id, title, COALESCE(dedupe_key,''), command, state, attempt_no,
	timeout_seconds, COALESCE(lease_id,''), lease_until, valid_until, created_at, assigned_at, started_at, finished_at,
	exit_code, timed_out, stdout, stderr, progress_current, progress_total, log_tail, reason, cancel, note_id, run_id, run_node, env`

func (s *Store) scanJob(sc interface{ Scan(...any) error }) (RunnerJob, error) {
	var j RunnerJob
	var note, runID, runNode sql.NullInt64
	var envJSON sql.NullString
	var quest, step, tmpl, exit, pc, pt sql.NullInt64
	var lease, assigned, started, finished sql.NullString
	var valid, created string
	dest := []any{&j.ID, &j.RunnerLabel, &j.Kind, &quest, &step, &tmpl, &j.Title, &j.DedupeKey, &j.Command, &j.State, &j.AttemptNo,
		&j.TimeoutSeconds, &j.LeaseID, &lease, &valid, &created, &assigned, &started, &finished,
		&exit, &j.TimedOut, &j.Stdout, &j.Stderr, &pc, &pt, &j.LogTail, &j.Reason, &j.Cancel, &note, &runID, &runNode, &envJSON}
	if err := sc.Scan(dest...); err != nil {
		return RunnerJob{}, err
	}
	if note.Valid {
		v := note.Int64
		j.NoteID = &v
	}
	if runID.Valid {
		v := runID.Int64
		j.RunID = &v
	}
	if runNode.Valid {
		v := int(runNode.Int64)
		j.RunNode = &v
	}
	if envJSON.Valid && envJSON.String != "" {
		_ = json.Unmarshal([]byte(envJSON.String), &j.Env)
	}
	if quest.Valid {
		v := quest.Int64
		j.QuestID = &v
	}
	if step.Valid {
		v := step.Int64
		j.StepID = &v
	}
	if tmpl.Valid {
		v := tmpl.Int64
		j.TemplateID = &v
	}
	if exit.Valid {
		v := int(exit.Int64)
		j.ExitCode = &v
	}
	if pc.Valid {
		v := int(pc.Int64)
		j.ProgressCurrent = &v
	}
	if pt.Valid {
		v := int(pt.Int64)
		j.ProgressTotal = &v
	}
	j.LeaseUntil = parseNullTime(lease)
	j.ValidUntil, _ = timeutil.ParseFlexible(valid)
	j.CreatedAt, _ = timeutil.ParseFlexible(created)
	j.AssignedAt = parseNullTime(assigned)
	j.StartedAt = parseNullTime(started)
	j.FinishedAt = parseNullTime(finished)
	return j, nil
}

// EnqueueJob queues a job. With a DedupeKey that already has an unfinished job,
// that job is returned instead and created is false.
func (s *Store) EnqueueJob(ctx context.Context, sp JobSpec) (job RunnerJob, created bool, err error) {
	labels := kernel.SplitRunOn(sp.RunnerLabel)
	if len(labels) == 0 || strings.TrimSpace(sp.Command) == "" {
		return RunnerJob{}, false, errors.New("runner label and command required")
	}
	if sp.TimeoutSeconds <= 0 {
		sp.TimeoutSeconds = 60
	}
	if sp.ValidFor <= 0 {
		sp.ValidFor = 24 * time.Hour
	}
	now := timeutil.NowUTC()
	var dk any
	if sp.DedupeKey != "" {
		dk = sp.DedupeKey
	}
	// Several runners: the first one that is online gets the job, the rest stay as the fallback.
	var all any
	if len(labels) > 1 {
		all = strings.Join(labels, ",")
	}
	label := s.firstOnline(ctx, labels, now)
	var envJSON any
	if len(sp.Env) > 0 {
		b, _ := json.Marshal(sp.Env)
		envJSON = string(b)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO runner_job
		(runner_label, runner_labels, kind, quest_id, step_id, template_id, title, dedupe_key, command, state, attempt_no, timeout_seconds, valid_until, created_at, note_id, run_id, run_node, env)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?, ?, ?, ?)`,
		label, all, sp.Kind, sp.QuestID, sp.StepID, sp.TemplateID, trimTo(sp.Title, 200), dk, sp.Command,
		s.nextAttempt(ctx, sp.DedupeKey), sp.TimeoutSeconds, timeutil.ToDBUTC(now.Add(sp.ValidFor)), timeutil.ToDBUTC(now), sp.NoteID,
		sp.RunID, sp.RunNode, envJSON)
	if err != nil {
		if sp.DedupeKey != "" && db.IsUniqueViolation(err) {
			j, gerr := s.OpenJobByKey(ctx, sp.DedupeKey)
			return j, false, gerr
		}
		return RunnerJob{}, false, err
	}
	id, _ := res.LastInsertId()
	j, err := s.GetRunnerJob(ctx, id)
	return j, true, err
}

// firstOnline is the first label of the list whose runner polls right now; none online — the first one,
// the job waits for it (or is rerouted by RerouteQueued when another one shows up).
func (s *Store) firstOnline(ctx context.Context, labels []string, now time.Time) string {
	if len(labels) > 1 {
		for _, l := range labels {
			r, err := scanRunner(s.DB.QueryRowContext(ctx, `SELECT `+runnerCols+` FROM runner WHERE label = ?`, l))
			if err == nil && RunnerOnline(r, now) {
				return l
			}
		}
	}
	return labels[0]
}

// RerouteQueued moves a waiting job to the next runner of its list when the one it waits for is away
// and another one is online. Jobs on a single runner are left alone.
func (s *Store) RerouteQueued(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, runner_label, runner_labels FROM runner_job
		WHERE state = 'queued' AND runner_labels IS NOT NULL`)
	if err != nil {
		return err
	}
	type waiting struct {
		id      int64
		current string
		all     []string
	}
	var jobs []waiting
	for rows.Next() {
		var w waiting
		var all string
		if err := rows.Scan(&w.id, &w.current, &all); err != nil {
			rows.Close()
			return err
		}
		w.all = kernel.SplitRunOn(all)
		jobs = append(jobs, w)
	}
	rows.Close()
	now := timeutil.NowUTC()
	for _, w := range jobs {
		if r, err := scanRunner(s.DB.QueryRowContext(ctx, `SELECT `+runnerCols+` FROM runner WHERE label = ?`, w.current)); err == nil && RunnerOnline(r, now) {
			continue
		}
		next := s.firstOnline(ctx, w.all, now)
		if next == w.current {
			continue
		}
		if r, err := scanRunner(s.DB.QueryRowContext(ctx, `SELECT `+runnerCols+` FROM runner WHERE label = ?`, next)); err != nil || !RunnerOnline(r, now) {
			continue // nobody online: stay where we are
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE runner_job SET runner_label = ? WHERE id = ? AND state = 'queued'`, next, w.id); err != nil {
			return err
		}
	}
	return nil
}

// nextAttempt numbers attempts of the same thing (same dedupe key).
func (s *Store) nextAttempt(ctx context.Context, key string) int {
	if key == "" {
		return 1
	}
	var n sql.NullInt64
	_ = s.DB.QueryRowContext(ctx, `SELECT MAX(attempt_no) FROM runner_job WHERE dedupe_key = ?`, key).Scan(&n)
	return int(n.Int64) + 1
}

func (s *Store) OpenJobByKey(ctx context.Context, key string) (RunnerJob, error) {
	return s.scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+` FROM runner_job WHERE dedupe_key = ? AND state IN ('queued','assigned','running')`, key))
}

// LatestJobByKey is the newest job (any state) queued under a dedupe key.
func (s *Store) LatestJobByKey(ctx context.Context, key string) (RunnerJob, bool, error) {
	j, err := s.scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+` FROM runner_job WHERE dedupe_key = ? ORDER BY id DESC LIMIT 1`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return RunnerJob{}, false, nil
	}
	return j, err == nil, err
}

// DeleteRunnerJob forgets a finished job (an emit result is handed out once).
func (s *Store) DeleteRunnerJob(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM runner_job WHERE id = ? AND state NOT IN ('queued','assigned','running')`, id)
	return err
}

func (s *Store) GetRunnerJob(ctx context.Context, id int64) (RunnerJob, error) {
	j, err := s.scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+` FROM runner_job WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return RunnerJob{}, ErrNotFound
	}
	return j, err
}

// JobFilter narrows ListRunnerJobs.
type JobFilter struct {
	Label  string
	States []string
	StepID int64
	Limit  int
}

func (s *Store) ListRunnerJobs(ctx context.Context, f JobFilter) ([]RunnerJob, error) {
	q := `SELECT ` + jobCols + ` FROM runner_job WHERE 1=1`
	var args []any
	if f.Label != "" {
		q += ` AND runner_label = ?`
		args = append(args, f.Label)
	}
	if f.StepID != 0 {
		q += ` AND step_id = ?`
		args = append(args, f.StepID)
	}
	if len(f.States) > 0 {
		q += ` AND state IN (` + strings.TrimSuffix(strings.Repeat("?,", len(f.States)), ",") + `)`
		for _, st := range f.States {
			args = append(args, st)
		}
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	q += fmt.Sprintf(` ORDER BY id DESC LIMIT %d`, f.Limit)
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunnerJob{}
	for rows.Next() {
		j, err := s.scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

const logTailLimit = 4000

// RunnerWorkspaceByToken resolves the workspace of a runner's permanent token
// (the runner has not been placed in a workspace yet, so this goes through a
// SECURITY DEFINER function that bypasses row-level security).
func (s *Store) RunnerWorkspaceByToken(ctx context.Context, token string) (int64, error) {
	if token == "" {
		return 0, ErrRunnerAuth
	}
	var id sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT runner_workspace_by_token(?)`, tokens.Hash(token)).Scan(&id); err != nil || !id.Valid {
		return 0, ErrRunnerAuth
	}
	return id.Int64, nil
}

// RunnerWorkspaceByReg is the same for a one-time registration token.
func (s *Store) RunnerWorkspaceByReg(ctx context.Context, token string) (int64, error) {
	var id sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT runner_workspace_by_reg(?)`, tokens.Hash(token)).Scan(&id); err != nil || !id.Valid {
		return 0, ErrRunnerToken
	}
	return id.Int64, nil
}
