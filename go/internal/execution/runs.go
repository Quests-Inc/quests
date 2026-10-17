package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// States of a script run.
const (
	RunWaiting = "waiting" // a Code node's job is out
	RunDone    = "done"
	RunFailed  = "failed" // a node failed or the run could not go on
)

// ScriptRun is one pass of a graph over an entity (ADR-0003). While a Code node's job is out it holds the evaluator's
// snapshot; afterwards it is the log of what each node did.
type ScriptRun struct {
	ID          int64          `json:"id"`
	GraphRef    string         `json:"graph_ref"`
	ScriptID    *int64         `json:"script_id,omitempty"`
	SubjectType string         `json:"subject_type"`
	SubjectID   int64          `json:"subject_id"`
	State       string         `json:"state"`
	Snapshot    graph.Snapshot `json:"snapshot"`
	Error       string         `json:"error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	// Key names a run that someone waits for by name (a template's emit: template, period, script version); Result is what its
	// sink handed back.
	Key    string `json:"key,omitempty"`
	Result string `json:"result,omitempty"`
}

// NewScriptRun describes a run to store.
type NewScriptRun struct {
	GraphRef    string
	ScriptID    *int64
	SubjectType string
	SubjectID   int64
	Snapshot    graph.Snapshot
	Key         string
}

const runCols = `id, graph_ref, script_id, subject_type, subject_id, state, snapshot, error, created_at, updated_at, finished_at, COALESCE(run_key, ''), COALESCE(result, '')`

// CreateRun stores a run, waiting.
func (s *Store) CreateRun(ctx context.Context, in NewScriptRun) (int64, error) {
	snap, err := json.Marshal(in.Snapshot)
	if err != nil {
		return 0, err
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	var key any
	if in.Key != "" {
		key = in.Key
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO script_run (graph_ref, script_id, subject_type, subject_id, state, snapshot, created_at, updated_at, run_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, in.GraphRef, in.ScriptID, in.SubjectType, in.SubjectID, RunWaiting, string(snap), now, now, key)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SaveRun writes the new state and snapshot of a run; a run that is no longer waiting gets its finish time.
func (s *Store) SaveRun(ctx context.Context, id int64, state string, snap graph.Snapshot, errText string) error {
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	var finished any
	if state != RunWaiting {
		finished = now
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE script_run SET state = ?, snapshot = ?, error = ?, updated_at = ?, finished_at = ? WHERE id = ?`,
		state, string(raw), trimTo(errText, 4000), now, finished, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetRun reads one run.
func (s *Store) GetRun(ctx context.Context, id int64) (ScriptRun, error) {
	var (
		r                      ScriptRun
		script                 sql.NullInt64
		snap, created, updated string
		finished               sql.NullString
	)
	err := s.DB.QueryRowContext(ctx, `SELECT `+runCols+` FROM script_run WHERE id = ?`, id).Scan(
		&r.ID, &r.GraphRef, &script, &r.SubjectType, &r.SubjectID, &r.State, &snap, &r.Error, &created, &updated, &finished, &r.Key, &r.Result)
	if errors.Is(err, sql.ErrNoRows) {
		return ScriptRun{}, ErrNotFound
	}
	if err != nil {
		return ScriptRun{}, err
	}
	if script.Valid {
		v := script.Int64
		r.ScriptID = &v
	}
	if err := json.Unmarshal([]byte(snap), &r.Snapshot); err != nil {
		return ScriptRun{}, err
	}
	r.CreatedAt, _ = timeutil.ParseFlexible(created)
	r.UpdatedAt, _ = timeutil.ParseFlexible(updated)
	r.FinishedAt = parseNullTime(finished)
	return r, nil
}

// RunByKey returns the newest run with this key.
func (s *Store) RunByKey(ctx context.Context, key string) (ScriptRun, bool, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM script_run WHERE run_key = ? ORDER BY id DESC LIMIT 1`, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ScriptRun{}, false, nil
	}
	if err != nil {
		return ScriptRun{}, false, err
	}
	r, err := s.GetRun(ctx, id)
	return r, err == nil, err
}

// SetRunResult records what a run's sink produced (a template's quest document, or why there is none).
func (s *Store) SetRunResult(ctx context.Context, id int64, result string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE script_run SET result = ? WHERE id = ?`, trimTo(result, 256<<10), id)
	return err
}

// DeleteRun removes a run and, by cascade, its jobs.
func (s *Store) DeleteRun(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM script_run WHERE id = ?`, id)
	return err
}

// PurgeRuns forgets finished runs older than the cutoff. A run that still waits is never touched.
func (s *Store) PurgeRuns(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM script_run WHERE state != ? AND finished_at < ?`, RunWaiting, timeutil.ToDBUTC(before))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
