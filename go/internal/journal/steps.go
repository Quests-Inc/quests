package journal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

func (s *Store) loadSteps(ctx context.Context, questID int64) ([]Step, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, quest_id, title, description, progress_current, progress_total, sort_order,
			check_command, check_interval_seconds, check_last_run_at,
			wait_previous, run_mode, run_status, run_on,
			(SELECT script_id FROM script_binding b WHERE b.subject_type = 'step' AND b.subject_id = queststep.id)
		FROM queststep WHERE quest_id = ? ORDER BY sort_order, id`, questID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Step
	for rows.Next() {
		var st Step
		var cmd sql.NullString
		var interval sql.NullInt64
		var last sql.NullString
		var qid sql.NullInt64
		var waitPrev int
		var runMode sql.NullString
		var runStatus, runOn sql.NullString
		var script sql.NullInt64
		if err := rows.Scan(
			&st.ID, &qid, &st.Title, &st.Description, &st.ProgressCurrent, &st.ProgressTotal, &st.SortOrder,
			&cmd, &interval, &last, &waitPrev, &runMode, &runStatus, &runOn, &script,
		); err != nil {
			return nil, err
		}
		if qid.Valid {
			st.QuestID = qid.Int64
		}
		if script.Valid {
			v := script.Int64
			st.ScriptID = &v
		}
		if cmd.Valid {
			s := cmd.String
			st.CheckCommand = &s
		}
		if interval.Valid {
			v := int(interval.Int64)
			st.CheckIntervalSeconds = &v
		}
		if last.Valid {
			t, err := timeutil.ParseFlexible(last.String)
			if err != nil {
				return nil, err
			}
			st.CheckLastRunAt = &t
		}
		st.WaitPrevious = waitPrev != 0
		st.RunMode = RunModePoll
		if runMode.Valid && strings.TrimSpace(runMode.String) != "" {
			st.RunMode = NormalizeRunMode(runMode.String)
		}
		if runStatus.Valid && strings.TrimSpace(runStatus.String) != "" {
			s := strings.TrimSpace(runStatus.String)
			st.RunStatus = &s
		}
		if runOn.Valid && strings.TrimSpace(runOn.String) != "" {
			s := strings.TrimSpace(runOn.String)
			st.RunOn = &s
		}
		ClampStep(&st)
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) AddStep(ctx context.Context, questID int64, st Step, q Quest) (Quest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	NormalizeStepCheck(&st)
	ClampStep(&st)
	if err := s.guardCommand(ctx, st.CheckCommand, nil, st.RunOn); err != nil {
		return Quest{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO queststep (
			quest_id, title, description, progress_current, progress_total, sort_order,
			check_command, check_interval_seconds, check_last_run_at,
			wait_previous, run_mode, run_status, run_on
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		questID, st.Title, st.Description, st.ProgressCurrent, st.ProgressTotal, st.SortOrder,
		nullStr(st.CheckCommand), nullInt(st.CheckIntervalSeconds), nullTime(st.CheckLastRunAt),
		boolInt(st.WaitPrevious), NullRunMode(st.RunMode), nullStr(st.RunStatus), nullStr(st.RunOn),
	)
	if err != nil {
		return Quest{}, err
	}
	if err := persistQuestRow(tx, ctx, q); err != nil {
		return Quest{}, err
	}
	if err := stageChange(tx, ctx, "quest_updated", &questID, q.Title, "+шаги: 1", string(q.Significance)); err != nil {
		return Quest{}, err
	}
	if err := tx.Commit(); err != nil {
		return Quest{}, err
	}
	return s.GetQuest(ctx, questID)
}

func (s *Store) UpdateStep(ctx context.Context, st Step, q Quest, kind, detail string) (Quest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	NormalizeStepCheck(&st)
	ClampStep(&st)
	var oldCmd sql.NullString
	_ = tx.QueryRowContext(ctx, `SELECT check_command FROM queststep WHERE id = ? AND quest_id = ?`, st.ID, st.QuestID).Scan(&oldCmd)
	var oldPtr *string
	if oldCmd.Valid {
		oldPtr = &oldCmd.String
	}
	if err := s.guardCommand(ctx, st.CheckCommand, oldPtr, st.RunOn); err != nil {
		return Quest{}, err
	}
	if st.CheckCommand != nil && strings.TrimSpace(*st.CheckCommand) != "" {
		// A step runs a command or a script, not both: a command written on top of a script replaces it.
		if _, err := tx.ExecContext(ctx, `DELETE FROM script_binding WHERE subject_type = 'step' AND subject_id = ?`, st.ID); err != nil {
			return Quest{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE queststep SET
			title=?, description=?, progress_current=?, progress_total=?, sort_order=?,
			check_command=?, check_interval_seconds=?, check_last_run_at=?,
			wait_previous=?, run_mode=?, run_status=?, run_on=?
		WHERE id=? AND quest_id=?`,
		st.Title, st.Description, st.ProgressCurrent, st.ProgressTotal, st.SortOrder,
		nullStr(st.CheckCommand), nullInt(st.CheckIntervalSeconds), nullTime(st.CheckLastRunAt),
		boolInt(st.WaitPrevious), NullRunMode(st.RunMode), nullStr(st.RunStatus), nullStr(st.RunOn), st.ID, st.QuestID,
	)
	if err != nil {
		return Quest{}, err
	}
	if err := persistQuestRow(tx, ctx, q); err != nil {
		return Quest{}, err
	}
	if kind == "" {
		kind = "quest_updated"
	}
	if detail == "" {
		detail = "изменено"
	}
	if err := stageChangeComment(tx, ctx, kind, &q.ID, q.Title, detail, string(q.Significance), ""); err != nil {
		return Quest{}, err
	}
	if err := tx.Commit(); err != nil {
		return Quest{}, err
	}
	return s.GetQuest(ctx, q.ID)
}

func (s *Store) DeleteStep(ctx context.Context, questID, stepID int64, q Quest) (Quest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `DELETE FROM queststep WHERE id=? AND quest_id=?`, stepID, questID)
	if err != nil {
		return Quest{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Quest{}, ErrNotFound
	}
	if err := persistQuestRow(tx, ctx, q); err != nil {
		return Quest{}, err
	}
	if err := stageChange(tx, ctx, "quest_updated", &questID, q.Title, "−шаги: 1", string(q.Significance)); err != nil {
		return Quest{}, err
	}
	if err := tx.Commit(); err != nil {
		return Quest{}, err
	}
	return s.GetQuest(ctx, questID)
}

// MoveStep re-parents a step to another quest, at the end of its steps. The
// caller passes both quests already updated (step removed / added, status
// synced); the step's dependency on "the previous step" is cleared because
// its predecessor changes.
func (s *Store) MoveStep(ctx context.Context, stepID int64, from, to Quest, newOrder int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`UPDATE queststep SET quest_id=?, sort_order=?, wait_previous=0 WHERE id=? AND quest_id=?`,
		to.ID, newOrder, stepID, from.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if err := persistQuestRow(tx, ctx, from); err != nil {
		return err
	}
	if err := persistQuestRow(tx, ctx, to); err != nil {
		return err
	}
	if err := stageChange(tx, ctx, "quest_updated", &from.ID, from.Title,
		fmt.Sprintf("−шаги: 1 (в quest=%d)", to.ID), string(from.Significance)); err != nil {
		return err
	}
	if err := stageChange(tx, ctx, "quest_updated", &to.ID, to.Title,
		fmt.Sprintf("+шаги: 1 (из quest=%d)", from.ID), string(to.Significance)); err != nil {
		return err
	}
	return tx.Commit()
}

func NormalizeCheck(cmd *string, interval *int) (*string, *int) {
	if cmd == nil {
		return nil, nil
	}
	c := strings.TrimSpace(*cmd)
	if c == "" {
		return nil, nil
	}
	if interval == nil {
		return &c, nil
	}
	v := *interval
	if v < 15 {
		v = 15
	}
	return &c, &v
}

func NormalizeRunMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case RunModeOnce:
		return RunModeOnce
	case RunModeWatch:
		return RunModeWatch
	}
	return RunModePoll
}

func NormalizeStepCheck(st *Step) {
	if st.RunMode == "" {
		st.RunMode = RunModePoll
	} else {
		st.RunMode = NormalizeRunMode(st.RunMode)
	}
	if (st.CheckCommand == nil || strings.TrimSpace(*st.CheckCommand) == "") && st.ScriptID == nil {
		st.CheckCommand = nil
		st.CheckIntervalSeconds = nil
		st.WaitPrevious = false
		st.RunMode = RunModePoll
		st.RunStatus = nil
		st.RunOn = nil
		return
	}
	st.RunOn = NormalizeRunOn(st.RunOn)
	if (st.RunMode == RunModePoll || st.RunMode == RunModeWatch) && st.CheckIntervalSeconds == nil {
		v := 15
		st.CheckIntervalSeconds = &v
	}
	if st.RunMode == RunModeOnce && (st.RunStatus == nil || strings.TrimSpace(*st.RunStatus) == "") {
		idle := RunIdle
		st.RunStatus = &idle
	}
}

func NullRunMode(s string) string {
	if strings.TrimSpace(s) == "" {
		return RunModePoll
	}
	return NormalizeRunMode(s)
}

func (s *Store) UpdateStepComment(ctx context.Context, st Step, q Quest, kind, detail, comment string) (Quest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	NormalizeStepCheck(&st)
	ClampStep(&st)
	_, err = tx.ExecContext(ctx, `
		UPDATE queststep SET
			title=?, description=?, progress_current=?, progress_total=?, sort_order=?,
			check_command=?, check_interval_seconds=?, check_last_run_at=?,
			wait_previous=?, run_mode=?, run_status=?, run_on=?
		WHERE id=? AND quest_id=?`,
		st.Title, st.Description, st.ProgressCurrent, st.ProgressTotal, st.SortOrder,
		nullStr(st.CheckCommand), nullInt(st.CheckIntervalSeconds), nullTime(st.CheckLastRunAt),
		boolInt(st.WaitPrevious), NullRunMode(st.RunMode), nullStr(st.RunStatus), nullStr(st.RunOn), st.ID, st.QuestID,
	)
	if err != nil {
		return Quest{}, err
	}
	if err := persistQuestRow(tx, ctx, q); err != nil {
		return Quest{}, err
	}
	if kind == "" {
		kind = "quest_updated"
	}
	if detail == "" {
		detail = "изменено"
	}
	if err := stageChangeComment(tx, ctx, kind, &q.ID, q.Title, detail, string(q.Significance), comment); err != nil {
		return Quest{}, err
	}
	if err := tx.Commit(); err != nil {
		return Quest{}, err
	}
	return s.GetQuest(ctx, q.ID)
}

// ClearStepCommand removes the step's own command (the one place a script takes over from it).
func (s *Store) ClearStepCommand(ctx context.Context, stepID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE queststep SET check_command = NULL WHERE id = ?`, stepID)
	return err
}
