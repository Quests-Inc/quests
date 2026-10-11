package journal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/valirum/quests/go/internal/platform/timeutil"
)

func (s *Store) ListCategories(ctx context.Context) ([]Category, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, slug, label, sort_order, COALESCE(color, '#9a9a9a')
		FROM questcategory ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Category
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Slug, &c.Label, &c.SortOrder, &c.Color); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CategoryExists(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM questcategory WHERE id = ?`, id).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) QuestlineExists(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM questline WHERE id = ?`, id).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

type ListFilter struct {
	Status      *QuestStatus
	Pinned      *bool
	QuestlineID *int64
	NewestFirst bool
}

func (s *Store) ListQuests(ctx context.Context, f ListFilter) ([]Quest, error) {
	q := `
		SELECT q.id, q.title, q.description, q.status, q.significance, q.pinned, q.sort_order,
			q.deadline_at, q.duration_seconds, q.category_id, q.questline_id,
			q.created_at, q.updated_at, q.completed_at, q.template_id, q.period_key, q.automated, q.source,
			c.slug, c.label, c.color,
			l.title, l.color, l.icon, l.custom_icon, l.updated_at
		FROM quest q
		LEFT JOIN questcategory c ON c.id = q.category_id
		LEFT JOIN questline l ON l.id = q.questline_id
		WHERE 1=1`
	args := []any{}
	if f.Status != nil {
		q += ` AND q.status = ?`
		args = append(args, string(*f.Status))
	}
	if f.Pinned != nil {
		// "not pinned" includes quests hidden from the HUD (pinned = -1).
		if *f.Pinned {
			q += ` AND q.pinned = 1`
		} else {
			q += ` AND q.pinned <= 0`
		}
	}
	if f.QuestlineID != nil {
		q += ` AND q.questline_id = ?`
		args = append(args, *f.QuestlineID)
	}
	if f.NewestFirst {
		q += ` ORDER BY q.id DESC`
	} else {
		q += ` ORDER BY q.sort_order, q.id`
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Quest
	for rows.Next() {
		quest, err := scanQuest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, quest)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Must close before nested queries — MaxOpenConns(1).
	_ = rows.Close()
	for i := range out {
		steps, err := s.loadSteps(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Steps = steps
	}
	if err := s.attachTagsToQuests(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetQuest(ctx context.Context, id int64) (Quest, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT q.id, q.title, q.description, q.status, q.significance, q.pinned, q.sort_order,
			q.deadline_at, q.duration_seconds, q.category_id, q.questline_id,
			q.created_at, q.updated_at, q.completed_at, q.template_id, q.period_key, q.automated, q.source,
			c.slug, c.label, c.color,
			l.title, l.color, l.icon, l.custom_icon, l.updated_at
		FROM quest q
		LEFT JOIN questcategory c ON c.id = q.category_id
		LEFT JOIN questline l ON l.id = q.questline_id
		WHERE q.id = ?`, id)
	quest, err := scanQuest(row)
	if err == sql.ErrNoRows {
		return Quest{}, ErrNotFound
	}
	if err != nil {
		return Quest{}, err
	}
	steps, err := s.loadSteps(ctx, quest.ID)
	if err != nil {
		return Quest{}, err
	}
	quest.Steps = steps
	qs := []Quest{quest}
	if err := s.attachTagsToQuests(ctx, qs); err != nil {
		return Quest{}, err
	}
	return qs[0], nil
}

func scanQuest(row rowScanner) (Quest, error) {
	var q Quest
	var deadline, completed, created, updated sql.NullString
	var duration sql.NullInt64
	var period, source sql.NullString
	var catID, lineID, tmplID sql.NullInt64
	var pinned, automated int
	var cSlug, cLabel, cColor sql.NullString
	var lTitle, lColor, lIcon, lCustom sql.NullString
	var lUpdated sql.NullString
	err := row.Scan(
		&q.ID, &q.Title, &q.Description, &q.Status, &q.Significance, &pinned, &q.SortOrder,
		&deadline, &duration, &catID, &lineID,
		&created, &updated, &completed, &tmplID, &period, &automated, &source,
		&cSlug, &cLabel, &cColor,
		&lTitle, &lColor, &lIcon, &lCustom, &lUpdated,
	)
	if err != nil {
		return q, err
	}
	q.Pinned, q.OverlayHidden = SplitPinState(pinned)
	q.Automated = automated != 0
	if source.Valid {
		v := source.String
		q.Source = &v
	}
	if deadline.Valid {
		t, err := timeutil.ParseFlexible(deadline.String)
		if err != nil {
			return q, err
		}
		q.DeadlineAt = &t
	}
	if duration.Valid {
		v := int(duration.Int64)
		q.DurationSeconds = &v
	}
	if catID.Valid {
		v := catID.Int64
		q.CategoryID = &v
	}
	if lineID.Valid {
		v := lineID.Int64
		q.QuestlineID = &v
	}
	if tmplID.Valid {
		v := tmplID.Int64
		q.TemplateID = &v
	}
	if period.Valid {
		s := period.String
		q.PeriodKey = &s
	}
	if created.Valid {
		t, err := timeutil.ParseFlexible(created.String)
		if err != nil {
			return q, err
		}
		q.CreatedAt = t
	}
	if updated.Valid {
		t, err := timeutil.ParseFlexible(updated.String)
		if err != nil {
			return q, err
		}
		q.UpdatedAt = t
	}
	if completed.Valid {
		t, err := timeutil.ParseFlexible(completed.String)
		if err != nil {
			return q, err
		}
		q.CompletedAt = &t
	}
	if cSlug.Valid {
		s := cSlug.String
		q.CategorySlug = &s
	}
	if cLabel.Valid {
		s := cLabel.String
		q.CategoryLabel = &s
	}
	if cColor.Valid {
		s := cColor.String
		q.CategoryColor = &s
	}
	if lTitle.Valid {
		s := lTitle.String
		q.QuestlineTitle = &s
	}
	if lColor.Valid {
		s := lColor.String
		q.QuestlineColor = &s
	}
	if lIcon.Valid {
		s := lIcon.String
		q.QuestlineIcon = &s
	}
	if lCustom.Valid {
		s := lCustom.String
		q.CustomIcon = &s
	}
	if lUpdated.Valid {
		t, err := timeutil.ParseFlexible(lUpdated.String)
		if err != nil {
			return q, err
		}
		q.QuestlineUpdatedAt = &t
	}
	return q, nil
}

func (s *Store) loadSteps(ctx context.Context, questID int64) ([]Step, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, quest_id, title, description, progress_current, progress_total, sort_order,
			check_command, check_interval_seconds, check_last_run_at,
			wait_previous, run_mode, run_status, run_on
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
		if err := rows.Scan(
			&st.ID, &qid, &st.Title, &st.Description, &st.ProgressCurrent, &st.ProgressTotal, &st.SortOrder,
			&cmd, &interval, &last, &waitPrev, &runMode, &runStatus, &runOn,
		); err != nil {
			return nil, err
		}
		if qid.Valid {
			st.QuestID = qid.Int64
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

func (s *Store) CreateQuest(ctx context.Context, q Quest) (Quest, error) {
	return s.createQuest(ctx, q, "quest_created", "создано задание")
}

// CreateQuestAppeared inserts a template instance and stages quest_appeared.
func (s *Store) CreateQuestAppeared(ctx context.Context, q Quest) (Quest, error) {
	detail := "Период"
	if q.PeriodKey != nil {
		detail = "Период " + *q.PeriodKey
	}
	return s.createQuest(ctx, q, "quest_appeared", detail)
}

func (s *Store) createQuest(ctx context.Context, q Quest, changeKind, changeDetail string) (Quest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quest{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO quest (
			title, description, status, significance, pinned, sort_order,
			deadline_at, duration_seconds, category_id, questline_id,
			created_at, updated_at, completed_at, template_id, period_key, automated, source
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		q.Title, q.Description, string(q.Status), string(q.Significance), PinState(q.Pinned, q.OverlayHidden), q.SortOrder,
		nullTime(q.DeadlineAt), nullInt(q.DurationSeconds), nullI64(q.CategoryID), nullI64(q.QuestlineID),
		timeutil.ToDBUTC(q.CreatedAt), timeutil.ToDBUTC(q.UpdatedAt), nullTime(q.CompletedAt), nullI64(q.TemplateID), nullStr(q.PeriodKey), boolInt(q.Automated), nullStr(q.Source),
	)
	if err != nil {
		return Quest{}, err
	}
	qid, err := res.LastInsertId()
	if err != nil {
		return Quest{}, err
	}
	for i := range q.Steps {
		st := q.Steps[i]
		NormalizeStepCheck(&st)
		ClampStep(&st)
		if err := s.guardAuthor(ctx, st.CheckCommand, nil); err != nil {
			return Quest{}, err
		}
		if err := s.guardExec(ctx, st.CheckCommand, st.RunOn); err != nil {
			return Quest{}, err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO queststep (
				quest_id, title, description, progress_current, progress_total, sort_order,
				check_command, check_interval_seconds, check_last_run_at,
				wait_previous, run_mode, run_status, run_on
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			qid, st.Title, st.Description, st.ProgressCurrent, st.ProgressTotal, st.SortOrder,
			nullStr(st.CheckCommand), nullInt(st.CheckIntervalSeconds), nullTime(st.CheckLastRunAt),
			boolInt(st.WaitPrevious), NullRunMode(st.RunMode), nullStr(st.RunStatus), nullStr(st.RunOn),
		)
		if err != nil {
			return Quest{}, err
		}
	}
	if err := stageChange(tx, ctx, changeKind, &qid, q.Title, changeDetail, string(q.Significance)); err != nil {
		return Quest{}, err
	}
	if err := tx.Commit(); err != nil {
		return Quest{}, err
	}
	return s.GetQuest(ctx, qid)
}

func (s *Store) UpdateQuest(ctx context.Context, q Quest, changeKind, changeDetail string) (Quest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quest{}, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		UPDATE quest SET
			title=?, description=?, status=?, significance=?, pinned=?, sort_order=?,
			deadline_at=?, duration_seconds=?, category_id=?, questline_id=?,
			updated_at=?, completed_at=?, automated=?
		WHERE id=?`,
		q.Title, q.Description, string(q.Status), string(q.Significance), PinState(q.Pinned, q.OverlayHidden), q.SortOrder,
		nullTime(q.DeadlineAt), nullInt(q.DurationSeconds), nullI64(q.CategoryID), nullI64(q.QuestlineID),
		timeutil.ToDBUTC(q.UpdatedAt), nullTime(q.CompletedAt), boolInt(q.Automated), q.ID,
	)
	if err != nil {
		return Quest{}, err
	}
	if changeKind != "" {
		if err := stageChangeComment(tx, ctx, changeKind, &q.ID, q.Title, changeDetail, string(q.Significance), ""); err != nil {
			return Quest{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Quest{}, err
	}
	return s.GetQuest(ctx, q.ID)
}

func (s *Store) DeleteQuest(ctx context.Context, id int64, title string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM queststep WHERE quest_id = ?`, id); err != nil {
		return err
	}
	if err := stageChange(tx, ctx, "quest_deleted", &id, title, "удалено", ""); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM quest WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) AddStep(ctx context.Context, questID int64, st Step, q Quest) (Quest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quest{}, err
	}
	defer func() { _ = tx.Rollback() }()
	NormalizeStepCheck(&st)
	ClampStep(&st)
	if err := s.guardAuthor(ctx, st.CheckCommand, nil); err != nil {
		return Quest{}, err
	}
	if err := s.guardExec(ctx, st.CheckCommand, st.RunOn); err != nil {
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
	if err := s.guardAuthor(ctx, st.CheckCommand, oldPtr); err != nil {
		return Quest{}, err
	}
	if err := s.guardExec(ctx, st.CheckCommand, st.RunOn); err != nil {
		return Quest{}, err
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

func persistQuestRow(tx *sql.Tx, ctx context.Context, q Quest) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE quest SET status=?, updated_at=?, completed_at=? WHERE id=?`,
		string(q.Status), timeutil.ToDBUTC(q.UpdatedAt), nullTime(q.CompletedAt), q.ID,
	)
	return err
}

func stageChange(tx *sql.Tx, ctx context.Context, kind string, questID *int64, title, detail, significance string) error {
	return stageChangeComment(tx, ctx, kind, questID, title, detail, significance, "")
}

func stageChangeComment(tx *sql.Tx, ctx context.Context, kind string, questID *int64, title, detail, significance, comment string) error {
	skip := map[string]bool{"startup": true, "step_progress": true, "quest_started": true}
	if skip[kind] || kind == "" {
		return nil
	}
	if len(title) > 200 {
		title = title[:200]
	}
	if len(detail) > 500 {
		detail = detail[:500]
	}
	if len(kind) > 32 {
		kind = kind[:32]
	}
	if len(comment) > 2000 {
		comment = comment[:2000]
	}
	var sig any
	if significance != "" {
		if len(significance) > 16 {
			significance = significance[:16]
		}
		sig = significance
	}
	var cmt any
	if strings.TrimSpace(comment) != "" {
		cmt = comment
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO questchangelog (at, kind, quest_id, title, detail, significance, revision, comment, entity_type, entity_id)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, 'quest', ?)`,
		timeutil.ToDBUTC(timeutil.NowUTC()), kind, nullI64(questID), title, detail, sig, cmt, nullI64(questID),
	)
	return err
}

// ApplyQuestlineCategory copies category from questline when assigned.
func (s *Store) QuestlineCategory(ctx context.Context, lineID int64) (*int64, error) {
	var cat sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT category_id FROM questline WHERE id = ?`, lineID).Scan(&cat)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !cat.Valid {
		return nil, nil
	}
	v := cat.Int64
	return &v, nil
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
	if st.CheckCommand == nil || strings.TrimSpace(*st.CheckCommand) == "" {
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

func (s *Store) UpdateQuestComment(ctx context.Context, q Quest, changeKind, changeDetail, comment string) (Quest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Quest{}, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		UPDATE quest SET
			title=?, description=?, status=?, significance=?, pinned=?, sort_order=?,
			deadline_at=?, duration_seconds=?, category_id=?, questline_id=?,
			updated_at=?, completed_at=?, automated=?
		WHERE id=?`,
		q.Title, q.Description, string(q.Status), string(q.Significance), PinState(q.Pinned, q.OverlayHidden), q.SortOrder,
		nullTime(q.DeadlineAt), nullInt(q.DurationSeconds), nullI64(q.CategoryID), nullI64(q.QuestlineID),
		timeutil.ToDBUTC(q.UpdatedAt), nullTime(q.CompletedAt), boolInt(q.Automated), q.ID,
	)
	if err != nil {
		return Quest{}, err
	}
	if changeKind != "" {
		if err := stageChangeComment(tx, ctx, changeKind, &q.ID, q.Title, changeDetail, string(q.Significance), comment); err != nil {
			return Quest{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Quest{}, err
	}
	return s.GetQuest(ctx, q.ID)
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

func ValidateCategoryID(ctx context.Context, s *Store, id *int64) (*int64, error) {
	if id == nil {
		return nil, nil
	}
	ok, err := s.CategoryExists(ctx, *id)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Python soft-validates to None
		return nil, nil
	}
	return id, nil
}

func FmtErr(err error) string {
	return fmt.Sprintf("%v", err)
}
