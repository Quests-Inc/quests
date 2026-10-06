package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/valirum/quests/go/internal/auth"
	"github.com/valirum/quests/go/internal/timeutil"
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
}

// Open reports whether the job has not reached a final state.
func (j RunnerJob) Open() bool {
	return j.State == JobQueued || j.State == JobAssigned || j.State == JobRunning
}

var (
	ErrRunnerExists   = errors.New("runner already exists")
	ErrRunnerToken    = errors.New("invalid or expired registration token")
	ErrRunnerAuth     = errors.New("invalid runner token")
	ErrLeaseLost      = errors.New("lease is no longer valid")
	ErrBadRunnerLabel = errors.New("label: 1-32 chars of a-z, 0-9, _ or -")
)

var runnerLabelRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// ValidRunOn: nil (run here) or a well-formed runner label.
func ValidRunOn(v *string) bool { return v == nil || runnerLabelRe.MatchString(*v) }

// RegistrationTTL is how long a fresh registration token stays valid.
const RegistrationTTL = 24 * time.Hour

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
	if !runnerLabelRe.MatchString(label) {
		return "", ErrBadRunnerLabel
	}
	secret, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	now := timeutil.NowUTC()
	_, err = s.DB.ExecContext(ctx,
		`INSERT INTO runner (label, reg_hash, reg_expires_at, revoked, created_at) VALUES (?, ?, ?, 0, ?)`,
		label, hash, timeutil.ToDBUTC(now.Add(RegistrationTTL)), timeutil.ToDBUTC(now))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return "", ErrRunnerExists
		}
		return "", err
	}
	return secret, nil
}

// RegisterRunner swaps a registration token for the runner's permanent token.
func (s *Store) RegisterRunner(ctx context.Context, regToken, hostname, version string) (token string, r Runner, err error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+runnerCols+`, reg_expires_at FROM runner WHERE reg_hash = ? AND revoked = 0`, auth.HashToken(regToken))
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
	secret, hash, err := auth.NewToken()
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

func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

// AuthRunner finds the runner behind a permanent token and records that it was just seen.
func (s *Store) AuthRunner(ctx context.Context, token string) (Runner, error) {
	if token == "" {
		return Runner{}, ErrRunnerAuth
	}
	r, err := scanRunner(s.DB.QueryRowContext(ctx, `SELECT `+runnerCols+` FROM runner WHERE token_hash = ? AND revoked = 0`, auth.HashToken(token)))
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
	Title          string
	DedupeKey      string // at most one unfinished job per key
	Command        string
	TimeoutSeconds int
	ValidFor       time.Duration // how long the job may wait for its runner
}

const jobCols = `id, runner_label, kind, quest_id, step_id, template_id, title, COALESCE(dedupe_key,''), command, state, attempt_no,
	timeout_seconds, COALESCE(lease_id,''), lease_until, valid_until, created_at, assigned_at, started_at, finished_at,
	exit_code, timed_out, stdout, stderr, progress_current, progress_total, log_tail, reason, cancel`

func parseNullTime(n sql.NullString) *time.Time {
	if !n.Valid || n.String == "" {
		return nil
	}
	t, err := timeutil.ParseFlexible(n.String)
	if err != nil {
		return nil
	}
	return &t
}

func scanJob(sc interface{ Scan(...any) error }) (RunnerJob, error) {
	var j RunnerJob
	var quest, step, tmpl, exit, pc, pt sql.NullInt64
	var lease, assigned, started, finished sql.NullString
	var valid, created string
	if err := sc.Scan(&j.ID, &j.RunnerLabel, &j.Kind, &quest, &step, &tmpl, &j.Title, &j.DedupeKey, &j.Command, &j.State, &j.AttemptNo,
		&j.TimeoutSeconds, &j.LeaseID, &lease, &valid, &created, &assigned, &started, &finished,
		&exit, &j.TimedOut, &j.Stdout, &j.Stderr, &pc, &pt, &j.LogTail, &j.Reason, &j.Cancel); err != nil {
		return RunnerJob{}, err
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
	if sp.RunnerLabel == "" || strings.TrimSpace(sp.Command) == "" {
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
	res, err := s.DB.ExecContext(ctx, `INSERT INTO runner_job
		(runner_label, kind, quest_id, step_id, template_id, title, dedupe_key, command, state, attempt_no, timeout_seconds, valid_until, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'queued', ?, ?, ?, ?)`,
		sp.RunnerLabel, sp.Kind, sp.QuestID, sp.StepID, sp.TemplateID, trimTo(sp.Title, 200), dk, sp.Command,
		s.nextAttempt(ctx, sp.DedupeKey), sp.TimeoutSeconds, timeutil.ToDBUTC(now.Add(sp.ValidFor)), timeutil.ToDBUTC(now))
	if err != nil {
		if sp.DedupeKey != "" && strings.Contains(err.Error(), "UNIQUE") {
			j, gerr := s.OpenJobByKey(ctx, sp.DedupeKey)
			return j, false, gerr
		}
		return RunnerJob{}, false, err
	}
	id, _ := res.LastInsertId()
	j, err := s.GetRunnerJob(ctx, id)
	return j, true, err
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
	return scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+` FROM runner_job WHERE dedupe_key = ? AND state IN ('queued','assigned','running')`, key))
}

// LatestJobByKey is the newest job (any state) queued under a dedupe key.
func (s *Store) LatestJobByKey(ctx context.Context, key string) (RunnerJob, bool, error) {
	j, err := scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+` FROM runner_job WHERE dedupe_key = ? ORDER BY id DESC LIMIT 1`, key))
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
	j, err := scanJob(s.DB.QueryRowContext(ctx, `SELECT `+jobCols+` FROM runner_job WHERE id = ?`, id))
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
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ClaimJob hands the runner its oldest waiting job (a one-shot before a poll)
// and gives it a lease. At most one runner can win a given job. nil, nil = nothing to do.
func (s *Store) ClaimJob(ctx context.Context, label string, lease time.Duration) (*RunnerJob, error) {
	now := timeutil.NowUTC()
	for attempt := 0; attempt < 3; attempt++ {
		var id int64
		err := s.DB.QueryRowContext(ctx, `SELECT id FROM runner_job
			WHERE runner_label = ? AND state = 'queued' AND valid_until > ?
			ORDER BY CASE kind WHEN 'once' THEN 0 ELSE 1 END, id LIMIT 1`, label, timeutil.ToDBUTC(now)).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		res, err := s.DB.ExecContext(ctx, `UPDATE runner_job SET state = 'assigned', lease_id = ?, lease_until = ?, assigned_at = ?
			WHERE id = ? AND state = 'queued'`, uuid.NewString(), timeutil.ToDBUTC(now.Add(lease)), timeutil.ToDBUTC(now), id)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // another runner took it first
		}
		j, err := s.GetRunnerJob(ctx, id)
		return &j, err
	}
	return nil, nil
}

// JobHeartbeat extends the lease, notes progress / the log tail, and says
// whether the job should be cancelled. The first one moves assigned → running.
func (s *Store) JobHeartbeat(ctx context.Context, id int64, leaseID string, progress *[2]int, logTail string, lease time.Duration) (cancel bool, err error) {
	now := timeutil.NowUTC()
	var pc, pt any
	if progress != nil {
		pc, pt = progress[0], progress[1]
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE runner_job SET
		state = 'running', started_at = COALESCE(started_at, ?), lease_until = ?,
		progress_current = COALESCE(?, progress_current), progress_total = COALESCE(?, progress_total),
		log_tail = CASE WHEN ? = '' THEN log_tail ELSE ? END
		WHERE id = ? AND lease_id = ? AND state IN ('assigned','running')`,
		timeutil.ToDBUTC(now), timeutil.ToDBUTC(now.Add(lease)), pc, pt, trimTail(logTail), trimTail(logTail), id, leaseID)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, ErrLeaseLost
	}
	var c bool
	_ = s.DB.QueryRowContext(ctx, `SELECT cancel FROM runner_job WHERE id = ?`, id).Scan(&c)
	return c, nil
}

const logTailLimit = 4000

func trimTail(s string) string {
	if len(s) > logTailLimit {
		return s[len(s)-logTailLimit:]
	}
	return s
}

// JobResult is what a runner reports when the command ends.
type JobResult struct {
	Exit     int
	Stdout   string
	Stderr   string
	TimedOut bool
}

// CompleteJob records the outcome. It accepts a result for a job whose lease
// has lapsed (state lost) as long as the lease id matches and the job is still
// valid: a runner that came back online delivers what it finished meanwhile.
// A repeated delivery of the same result is a no-op (done=false).
func (s *Store) CompleteJob(ctx context.Context, id int64, leaseID string, res JobResult) (job RunnerJob, done bool, err error) {
	j, err := s.GetRunnerJob(ctx, id)
	if err != nil {
		return RunnerJob{}, false, err
	}
	if j.LeaseID == "" || j.LeaseID != leaseID {
		return j, false, ErrLeaseLost
	}
	now := timeutil.NowUTC()
	switch j.State {
	case JobAssigned, JobRunning:
	case JobLost:
		if now.After(j.ValidUntil) {
			return j, false, ErrLeaseLost
		}
	case JobSuccess, JobFail, JobTimeout:
		return j, false, nil
	default:
		return j, false, ErrLeaseLost
	}
	state := JobFail
	if res.TimedOut {
		state = JobTimeout
	} else if res.Exit == 0 {
		state = JobSuccess
	}
	r, err := s.DB.ExecContext(ctx, `UPDATE runner_job SET state = ?, exit_code = ?, timed_out = ?, stdout = ?, stderr = ?,
		finished_at = ?, started_at = COALESCE(started_at, ?), reason = '' WHERE id = ? AND lease_id = ? AND state IN ('assigned','running','lost')`,
		state, res.Exit, res.TimedOut, res.Stdout, res.Stderr, timeutil.ToDBUTC(now), timeutil.ToDBUTC(now), id, leaseID)
	if err != nil {
		return RunnerJob{}, false, err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return j, false, nil
	}
	j, err = s.GetRunnerJob(ctx, id)
	return j, true, err
}

// ExpireJobs moves overdue jobs on: a lease nobody renews → lost, a job that
// waited past valid_until for its runner → skipped, a lost job whose result
// never came before valid_until → abandoned. Returns what reached a final state.
func (s *Store) ExpireJobs(ctx context.Context) ([]RunnerJob, error) {
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM runner_job WHERE
		(state IN ('assigned','running') AND lease_until < ?) OR (state IN ('queued','lost') AND valid_until < ?)`, now, now)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	var out []RunnerJob
	for _, id := range ids {
		// Lost: the result may still arrive until valid_until (see CompleteJob). Reported once, now.
		res, err := s.DB.ExecContext(ctx, `UPDATE runner_job SET state = 'lost', finished_at = ?, reason = 'раннер не отвечает: аренда истекла'
			WHERE id = ? AND state IN ('assigned','running') AND lease_until < ?`, now, id, now)
		if err != nil {
			return out, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			if j, err := s.GetRunnerJob(ctx, id); err == nil {
				out = append(out, j)
			}
			continue
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE runner_job SET state = 'abandoned', finished_at = ?, reason = 'раннер так и не вернул результат до конца срока'
			WHERE id = ? AND state = 'lost' AND valid_until < ?`, now, id, now); err != nil {
			return out, err
		}
		if _, err := s.DB.ExecContext(ctx, `UPDATE runner_job SET state = 'skipped', finished_at = ?, reason = 'устройство не вышло на связь до конца срока'
			WHERE id = ? AND state = 'queued' AND valid_until < ?`, now, id, now); err != nil {
			return out, err
		}
		if j, err := s.GetRunnerJob(ctx, id); err == nil && (j.State == JobSkipped || j.State == JobAbandoned) {
			out = append(out, j)
		}
	}
	return out, nil
}

// CancelRunnerJob drops a waiting job, or asks a running one to stop (it hears it in its next heartbeat).
func (s *Store) CancelRunnerJob(ctx context.Context, id int64) error {
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	res, err := s.DB.ExecContext(ctx, `UPDATE runner_job SET state = 'canceled', finished_at = ?, reason = 'отменено' WHERE id = ? AND state = 'queued'`, now, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	res, err = s.DB.ExecContext(ctx, `UPDATE runner_job SET cancel = 1 WHERE id = ? AND state IN ('assigned','running')`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PurgeRunnerJobs forgets finished jobs older than the cutoff.
func (s *Store) PurgeRunnerJobs(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM runner_job WHERE state NOT IN ('queued','assigned','running') AND finished_at < ?`, timeutil.ToDBUTC(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
