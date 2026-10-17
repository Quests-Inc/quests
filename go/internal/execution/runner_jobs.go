package execution

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

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
		state, res.Exit, boolInt(res.TimedOut), res.Stdout, res.Stderr, timeutil.ToDBUTC(now), timeutil.ToDBUTC(now), id, leaseID)
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
	if err := s.RerouteQueued(ctx); err != nil {
		return nil, err
	}
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

// CancelOrphanJobs drops the jobs of steps that no longer exist (the quest or
// step was deleted, however that happened): a waiting job is cancelled at
// once, one already running is asked to stop. Returns how many it touched.
func (s *Store) CancelOrphanJobs(ctx context.Context) (int64, error) {
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	gone := `(step_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM queststep JOIN quest ON quest.id = queststep.quest_id WHERE queststep.id = runner_job.step_id))
		OR (note_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM note WHERE note.id = runner_job.note_id))`

	res, err := s.DB.ExecContext(ctx, `UPDATE runner_job SET state = 'canceled', finished_at = ?, reason = 'квест, шаг или заметка удалены'
		WHERE state = 'queued' AND (`+gone+`)`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	res, err = s.DB.ExecContext(ctx, `UPDATE runner_job SET cancel = 1 WHERE state IN ('assigned','running') AND cancel = 0 AND (`+gone+`)`)
	if err != nil {
		return n, err
	}
	m, _ := res.RowsAffected()
	return n + m, nil
}

// PurgeRunnerJobs forgets finished jobs older than the cutoff.
func (s *Store) PurgeRunnerJobs(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM runner_job WHERE state NOT IN ('queued','assigned','running') AND finished_at < ?`, timeutil.ToDBUTC(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
