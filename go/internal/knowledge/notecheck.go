package knowledge

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/platform/timeutil"
)

// A note's check (quest=370) mirrors a step's: a command, an interval, where it runs (the server in
// a trusted workspace, or a runner), secrets of its own. The command answers "is this note out of
// date?"; the result is the one new state (`outdated`), a reason that rides with it, and — when a
// run breaks — an error that leaves `outdated` as it was.

const (
	// DefaultNoteCheckInterval is used when a check is set without an interval: notes drift over
	// hours, not seconds. The floor (15 s, the scheduler's tick) is the same as for steps.
	DefaultNoteCheckInterval = 3600
	MinCheckInterval         = 15
	maxReasonLen             = 300
	maxErrorLen              = 600
)

// NoteCheck is what a note's check looks like to the API.
type NoteCheck struct {
	Command         *string    `json:"command"`
	IntervalSeconds *int       `json:"interval_seconds"`
	RunOn           *string    `json:"run_on"`
	LastRunAt       *time.Time `json:"last_run_at"`
	Outdated        bool       `json:"outdated"`
	OutdatedAt      *time.Time `json:"outdated_at"`
	Reason          string     `json:"reason"`
	Error           string     `json:"error"`
	ErrorAt         *time.Time `json:"error_at"`
}

// Active reports whether the note has a check command.
func (c NoteCheck) Active() bool { return c.Command != nil && strings.TrimSpace(*c.Command) != "" }

const noteCheckCols = `id, check_command, check_interval_seconds, run_on, check_last_run_at, outdated, outdated_at, check_reason, check_error, check_error_at`

func scanNoteCheck(sc interface{ Scan(...any) error }) (int64, NoteCheck, error) {
	var id int64
	var c NoteCheck
	var cmd, runOn, lastRun, outAt, errAt sql.NullString
	var iv sql.NullInt64
	var outdated int
	if err := sc.Scan(&id, &cmd, &iv, &runOn, &lastRun, &outdated, &outAt, &c.Reason, &c.Error, &errAt); err != nil {
		return 0, c, err
	}
	if cmd.Valid && strings.TrimSpace(cmd.String) != "" {
		v := cmd.String
		c.Command = &v
	}
	if iv.Valid {
		v := int(iv.Int64)
		c.IntervalSeconds = &v
	}
	if runOn.Valid && runOn.String != "" {
		v := runOn.String
		c.RunOn = &v
	}
	c.LastRunAt, c.OutdatedAt, c.ErrorAt = parseNullTime(lastRun), parseNullTime(outAt), parseNullTime(errAt)
	c.Outdated = outdated != 0
	return id, c, nil
}

// NoteChecks returns the check state of every note that has a check or is marked outdated, keyed by
// note id (notes without either are absent).
func (s *Store) NoteChecks(ctx context.Context) (map[int64]NoteCheck, error) {
	out := map[int64]NoteCheck{}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+noteCheckCols+` FROM note WHERE check_command IS NOT NULL OR outdated <> 0 OR check_error <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		id, c, err := scanNoteCheck(rows)
		if err != nil {
			return nil, err
		}
		out[id] = c
	}
	return out, rows.Err()
}

// GetNoteCheck returns one note's check state.
func (s *Store) GetNoteCheck(ctx context.Context, id int64) (NoteCheck, error) {
	_, c, err := scanNoteCheck(s.DB.QueryRowContext(ctx, `SELECT `+noteCheckCols+` FROM note WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return NoteCheck{}, ErrNotFound
	}
	return c, err
}

// SetNoteCheck sets (or, with an empty command, removes) the note's check. Only an owner of the
// workspace sets a command (as for steps), and a command that would run on the server needs a
// trusted workspace. Removing the check clears its result too: a verdict nobody renews is stale.
func (s *Store) SetNoteCheck(ctx context.Context, id int64, command *string, interval *int, runOn *string) (NoteCheck, error) {
	cur, err := s.GetNoteCheck(ctx, id)
	if err != nil {
		return NoteCheck{}, err
	}
	runOn = NormalizeRunOn(runOn)
	if !ValidRunOn(runOn) {
		return NoteCheck{}, errBad("run_on: " + ErrBadRunnerLabel.Error())
	}
	if command == nil || strings.TrimSpace(*command) == "" {
		_, err = s.DB.ExecContext(ctx, `UPDATE note SET check_command = NULL, check_interval_seconds = NULL, check_last_run_at = NULL, run_on = NULL,
			outdated = 0, outdated_at = NULL, check_reason = '', check_error = '', check_error_at = NULL WHERE id = ?`, id)
		if err != nil {
			return NoteCheck{}, err
		}
		return s.GetNoteCheck(ctx, id)
	}
	cmd := strings.TrimSpace(*command)
	if len(cmd) > 8000 {
		return NoteCheck{}, errBad("команда длиннее 8000 символов")
	}
	if err := s.guardAuthor(ctx, &cmd, cur.Command); err != nil {
		return NoteCheck{}, err
	}
	if err := s.guardExec(ctx, &cmd, runOn); err != nil {
		return NoteCheck{}, err
	}
	iv := DefaultNoteCheckInterval
	if interval != nil {
		iv = *interval
	}
	if iv < MinCheckInterval {
		iv = MinCheckInterval
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE note SET check_command = ?, check_interval_seconds = ?, run_on = ?, check_last_run_at = NULL WHERE id = ?`,
		cmd, iv, nullStr(runOn), id)
	if err != nil {
		return NoteCheck{}, err
	}
	return s.GetNoteCheck(ctx, id)
}

// RequestNoteCheckNow makes the next scheduler tick run the check (the "check now" button).
func (s *Store) RequestNoteCheckNow(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE note SET check_last_run_at = NULL WHERE id = ? AND check_command IS NOT NULL`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DueNoteChecks lists the notes whose check should run now, in the context's workspace.
func (s *Store) DueNoteChecks(ctx context.Context, now time.Time) ([]DueNote, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+noteCheckCols+`, title FROM note WHERE check_command IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DueNote
	for rows.Next() {
		var id int64
		var c NoteCheck
		var title string
		var cmd, runOn, lastRun, outAt, errAt sql.NullString
		var iv sql.NullInt64
		var outdated int
		if err := rows.Scan(&id, &cmd, &iv, &runOn, &lastRun, &outdated, &outAt, &c.Reason, &c.Error, &errAt, &title); err != nil {
			return nil, err
		}
		if !cmd.Valid || strings.TrimSpace(cmd.String) == "" {
			continue
		}
		every := DefaultNoteCheckInterval
		if iv.Valid && int(iv.Int64) >= MinCheckInterval {
			every = int(iv.Int64)
		}
		last := parseNullTime(lastRun)
		if last != nil && now.Sub(*last) < time.Duration(every)*time.Second {
			continue
		}
		d := DueNote{ID: id, Title: title, Command: strings.TrimSpace(cmd.String)}
		if runOn.Valid && runOn.String != "" {
			v := runOn.String
			d.RunOn = &v
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DueNote is a note whose check is due.
type DueNote struct {
	ID      int64
	Title   string
	Command string
	RunOn   *string
}

// MarkNoteCheckStarted stamps the run time (so the next tick leaves the note alone for an interval).
func (s *Store) MarkNoteCheckStarted(ctx context.Context, id int64, at time.Time) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE note SET check_last_run_at = ? WHERE id = ?`, timeutil.ToDBUTC(at), id)
	return err
}

// NoteCheckResult is what a finished, well-formed run said.
type NoteCheckResult struct {
	Outdated bool
	Reason   string
}

// ApplyNoteCheckResult records a good answer: the verdict and its reason, and clears an earlier error.
// It reports whether the verdict changed (the caller logs the transition).
func (s *Store) ApplyNoteCheckResult(ctx context.Context, id int64, res NoteCheckResult, at time.Time) (changed bool, err error) {
	var was int
	if err := s.DB.QueryRowContext(ctx, `SELECT outdated FROM note WHERE id = ?`, id).Scan(&was); err != nil {
		return false, err
	}
	changed = (was != 0) != res.Outdated
	now := timeutil.ToDBUTC(at)
	reason := trimTo(res.Reason, maxReasonLen)
	if res.Outdated {
		if changed {
			_, err = s.DB.ExecContext(ctx, `UPDATE note SET outdated = 1, outdated_at = ?, check_reason = ?, check_error = '', check_error_at = NULL, check_last_run_at = ? WHERE id = ?`, now, reason, now, id)
		} else {
			_, err = s.DB.ExecContext(ctx, `UPDATE note SET check_reason = ?, check_error = '', check_error_at = NULL, check_last_run_at = ? WHERE id = ?`, reason, now, id)
		}
	} else {
		_, err = s.DB.ExecContext(ctx, `UPDATE note SET outdated = 0, outdated_at = NULL, check_reason = ?, check_error = '', check_error_at = NULL, check_last_run_at = ? WHERE id = ?`, reason, now, id)
	}
	return changed, err
}

// RecordNoteCheckError notes a broken run. The verdict stays what it was.
func (s *Store) RecordNoteCheckError(ctx context.Context, id int64, msg string, at time.Time) error {
	now := timeutil.ToDBUTC(at)
	_, err := s.DB.ExecContext(ctx, `UPDATE note SET check_error = ?, check_error_at = ?, check_last_run_at = ? WHERE id = ?`, trimTo(msg, maxErrorLen), now, now, id)
	return err
}

// SetNoteOutdated is the hand-set flag, the same one a check's verdict writes: a person (or an agent)
// who knows the note is stale says so, with an optional reason. A check that runs later gives its own verdict.
func (s *Store) SetNoteOutdated(ctx context.Context, id int64, outdated bool, reason string, at time.Time) error {
	now := timeutil.ToDBUTC(at)
	var res sql.Result
	var err error
	if outdated {
		res, err = s.DB.ExecContext(ctx, `UPDATE note SET outdated = 1, outdated_at = CASE WHEN outdated = 0 THEN ? ELSE outdated_at END, check_reason = ? WHERE id = ?`, now, trimTo(reason, maxReasonLen), id)
	} else {
		res, err = s.DB.ExecContext(ctx, `UPDATE note SET outdated = 0, outdated_at = NULL, check_reason = '' WHERE id = ?`, id)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
