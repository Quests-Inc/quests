package automation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// TemplateRow is a template as the scheduler needs it to decide what is due and to build a quest from it.
type TemplateRow struct {
	ID              int64
	Title           string
	Description     string
	Pinned          bool
	OverlayHidden   bool
	SortOrder       int
	DurationSeconds sql.NullInt64
	Freq            string
	Weekdays        string
	Enabled         bool
	Timezone        string
	DeadlineTime    sql.NullString
	Significance    string
	EmitMode        string
	EmitChance      float64
	EmitWindowStart sql.NullString
	EmitWindowEnd   sql.NullString
	EmitPoolCommand sql.NullString
	EmitRunOn       sql.NullString
	EmitLimits      sql.NullString
	CategoryID      sql.NullInt64
	QuestlineID     sql.NullInt64
	Automated       bool
}

// TemplateStepRow is one step of a template.
type TemplateStepRow struct {
	Title                string
	Description          string
	SortOrder            int
	ProgressMin          int
	ProgressMax          int
	CheckCommand         sql.NullString
	CheckIntervalSeconds sql.NullInt64
	WaitPrevious         bool
	RunMode              string
	RunOn                sql.NullString
}

const templateRowColumns = `id, title, description, pinned, sort_order, duration_seconds, freq, weekdays,
			enabled, timezone, deadline_time, significance, emit_mode, emit_chance,
			emit_window_start, emit_window_end, emit_pool_command, emit_limits, emit_run_on,
			category_id, questline_id, automated`

type rowScan interface{ Scan(dest ...any) error }

func scanTemplateRow(r rowScan) (TemplateRow, error) {
	var t TemplateRow
	var pinned, enabled, automated int
	if err := r.Scan(
		&t.ID, &t.Title, &t.Description, &pinned, &t.SortOrder, &t.DurationSeconds, &t.Freq, &t.Weekdays,
		&enabled, &t.Timezone, &t.DeadlineTime, &t.Significance, &t.EmitMode, &t.EmitChance,
		&t.EmitWindowStart, &t.EmitWindowEnd, &t.EmitPoolCommand, &t.EmitLimits, &t.EmitRunOn,
		&t.CategoryID, &t.QuestlineID, &automated,
	); err != nil {
		return TemplateRow{}, err
	}
	t.Pinned, t.OverlayHidden = journal.SplitPinState(pinned)
	t.Enabled = enabled != 0
	t.Automated = automated != 0
	return t, nil
}

// ListEnabledTemplateRows returns every enabled template, in display order.
func (s *Store) ListEnabledTemplateRows(ctx context.Context) ([]TemplateRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+templateRowColumns+` FROM questtemplate WHERE enabled = 1 ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TemplateRow
	for rows.Next() {
		t, err := scanTemplateRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TemplateRowByID returns one template for the scheduler; ErrNotFound when there is none.
func (s *Store) TemplateRowByID(ctx context.Context, id int64) (TemplateRow, error) {
	t, err := scanTemplateRow(s.DB.QueryRowContext(ctx, `SELECT `+templateRowColumns+` FROM questtemplate WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return TemplateRow{}, ErrNotFound
	}
	return t, err
}

// TemplateStepRows returns the steps of a template in order.
func (s *Store) TemplateStepRows(ctx context.Context, templateID int64) ([]TemplateStepRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT title, description, sort_order, progress_min, progress_max, check_command, check_interval_seconds,
			wait_previous, run_mode, run_on
		FROM questtemplatestep WHERE template_id = ? ORDER BY sort_order, id`, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TemplateStepRow
	for rows.Next() {
		var st TemplateStepRow
		var waitPrev int
		var runMode sql.NullString
		if err := rows.Scan(&st.Title, &st.Description, &st.SortOrder, &st.ProgressMin, &st.ProgressMax, &st.CheckCommand, &st.CheckIntervalSeconds, &waitPrev, &runMode, &st.RunOn); err != nil {
			return nil, err
		}
		st.WaitPrevious = waitPrev != 0
		st.RunMode = journal.RunModePoll
		if runMode.Valid {
			st.RunMode = journal.NormalizeRunMode(runMode.String)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// EmitRoll is the record of what a template did in one period (rolled a miss, scheduled a time, ran its command…).
type EmitRoll struct {
	ID          int64
	Outcome     string
	Attempts    int
	RetryAt     sql.NullString
	ScheduledAt sql.NullString
}

// EmitRollFor returns the roll of a template for a period; ok=false when there is none yet.
func (s *Store) EmitRollFor(ctx context.Context, templateID int64, periodKey string) (r EmitRoll, ok bool, err error) {
	err = s.DB.QueryRowContext(ctx, `
		SELECT id, outcome, attempts, retry_at, scheduled_at FROM templateemitroll
		WHERE template_id = ? AND period_key = ?`, templateID, periodKey).
		Scan(&r.ID, &r.Outcome, &r.Attempts, &r.RetryAt, &r.ScheduledAt)
	if errors.Is(err, sql.ErrNoRows) {
		return EmitRoll{}, false, nil
	}
	return r, err == nil, err
}

// InsertEmitRoll records a new roll. scheduledAt, retryAt and pickedRefs are optional.
func (s *Store) InsertEmitRoll(ctx context.Context, templateID int64, periodKey, outcome string, attempts int, scheduledAt, retryAt *time.Time, pickedRefs string, now time.Time) (int64, error) {
	var sched, retry, picked any
	if scheduledAt != nil {
		sched = timeutil.ToDBUTC(*scheduledAt)
	}
	if retryAt != nil {
		retry = timeutil.ToDBUTC(*retryAt)
	}
	if pickedRefs != "" {
		picked = pickedRefs
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO templateemitroll (template_id, period_key, outcome, attempts, scheduled_at, retry_at, picked_refs, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		templateID, periodKey, outcome, attempts, sched, retry, picked, timeutil.ToDBUTC(now), timeutil.ToDBUTC(now))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateEmitRollAttempt records a failed or retried run: the outcome, how many tries so far and when to try next.
func (s *Store) UpdateEmitRollAttempt(ctx context.Context, id int64, outcome string, attempts int, retryAt *time.Time, now time.Time) error {
	var retry any
	if retryAt != nil {
		retry = timeutil.ToDBUTC(*retryAt)
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE templateemitroll SET outcome = ?, attempts = ?, retry_at = ?, updated_at = ? WHERE id = ?`,
		outcome, attempts, retry, timeutil.ToDBUTC(now), id)
	return err
}

// UpdateEmitRollPicked records what a finished pool run picked and clears the pending retry.
func (s *Store) UpdateEmitRollPicked(ctx context.Context, id int64, outcome, pickedRefs string, now time.Time) error {
	var picked any
	if pickedRefs != "" {
		picked = pickedRefs
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE templateemitroll SET outcome = ?, picked_refs = ?, retry_at = NULL, updated_at = ? WHERE id = ?`,
		outcome, picked, timeutil.ToDBUTC(now), id)
	return err
}

// MarkEmitRollMaterialized notes that the quest of a roll has been created.
func (s *Store) MarkEmitRollMaterialized(ctx context.Context, id int64, now time.Time) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE templateemitroll SET outcome = 'materialized', updated_at = ? WHERE id = ?`, timeutil.ToDBUTC(now), id)
}
