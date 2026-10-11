package automation

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/valirum/quests/go/internal/journal"
	"github.com/valirum/quests/go/internal/platform/timeutil"
)

// TemplateRead is a quest template as the API sends it.
type TemplateRead struct {
	ID               int64          `json:"id"`
	Title            string         `json:"title"`
	Description      string         `json:"description"`
	Pinned           bool           `json:"pinned"`
	OverlayHidden    bool           `json:"overlay_hidden"`
	SortOrder        int            `json:"sort_order"`
	Freq             string         `json:"freq"`
	Weekdays         string         `json:"weekdays"`
	Enabled          bool           `json:"enabled"`
	Timezone         string         `json:"timezone"`
	Significance     string         `json:"significance"`
	EmitMode         string         `json:"emit_mode"`
	EmitChance       float64        `json:"emit_chance"`
	EmitLimits       map[string]any `json:"emit_limits"`
	DurationSecs     *int           `json:"duration_seconds"`
	DeadlineTime     *string        `json:"deadline_time"`
	EmitWindowFrom   *string        `json:"emit_window_start"`
	EmitWindowTo     *string        `json:"emit_window_end"`
	EmitPoolCmd      *string        `json:"emit_pool_command"`
	EmitRunOn        *string        `json:"emit_run_on"`
	CategoryID       *int64         `json:"category_id"`
	QuestlineID      *int64         `json:"questline_id"`
	CategorySlug     *string        `json:"category_slug"`
	CategoryLabel    *string        `json:"category_label"`
	CategoryColor    *string        `json:"category_color"`
	QuestlineTitle   *string        `json:"questline_title"`
	QuestlineColor   *string        `json:"questline_color"`
	QuestlineIcon    *string        `json:"questline_icon"`
	QuestlineIconURL *string        `json:"questline_icon_url"`
	Automated        bool           `json:"automated"`
	// EmitPoolLastOutcome is the outcome of the latest emit_pool roll, once there was one.
	EmitPoolLastOutcome *string            `json:"emit_pool_last_outcome"`
	CreatedAt           *string            `json:"created_at"`
	UpdatedAt           *string            `json:"updated_at"`
	Steps               []TemplateStepRead `json:"steps"`
	Tags                []journal.Tag      `json:"tags"`
}

// TemplateStepRead is one step of a template.
type TemplateStepRead struct {
	ID                   int64   `json:"id"`
	TemplateID           int64   `json:"template_id"`
	Title                string  `json:"title"`
	Description          string  `json:"description"`
	SortOrder            int     `json:"sort_order"`
	ProgressMin          int     `json:"progress_min"`
	ProgressMax          int     `json:"progress_max"`
	CheckCommand         *string `json:"check_command"`
	CheckIntervalSeconds *int    `json:"check_interval_seconds"`
	WaitPrevious         bool    `json:"wait_previous"`
	RunMode              string  `json:"run_mode"`
	RunOn                *string `json:"run_on"`
}

// body is the template in the shape a create/update request has, so that a read can be merged with
// a patch or copied.
func (t TemplateRead) body() map[string]any {
	raw, _ := json.Marshal(t)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

func (s *Store) ListTemplates(ctx context.Context, enabled *bool) ([]TemplateRead, error) {
	q := `
		SELECT t.id, t.title, t.description, t.pinned, t.sort_order, t.duration_seconds, t.freq, t.weekdays,
			t.enabled, t.timezone, t.deadline_time, t.significance, t.emit_mode, t.emit_chance,
			t.emit_window_start, t.emit_window_end, t.emit_pool_command, t.emit_limits, t.emit_run_on,
			t.category_id, t.questline_id,
			t.created_at, t.updated_at, t.automated,
			c.slug, c.label, c.color, l.title, l.color, l.icon, l.custom_icon, l.updated_at, l.id
		FROM questtemplate t
		LEFT JOIN questcategory c ON c.id = t.category_id
		LEFT JOIN questline l ON l.id = t.questline_id
		WHERE 1=1`
	args := []any{}
	if enabled != nil {
		q += ` AND t.enabled = ?`
		if *enabled {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}
	q += ` ORDER BY t.sort_order, t.id`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := make([]TemplateRead, 0)
	for rows.Next() {
		tr, err := scanTemplate(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, tr)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	_ = rows.Close()
	for i := range out {
		id := out[i].ID
		steps, err := s.loadTemplateStepsRead(ctx, id)
		if err != nil {
			return nil, err
		}
		out[i].Steps = steps
		if out[i].EmitPoolCmd != nil {
			outcome, err := s.latestEmitRollOutcome(ctx, id)
			if err != nil {
				return nil, err
			}
			if outcome != "" {
				out[i].EmitPoolLastOutcome = &outcome
			}
		}
	}
	ids := make([]int64, len(out))
	for i := range out {
		ids[i] = out[i].ID
	}
	tagMap, err := s.Journal().TagsForTemplates(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		tags := tagMap[out[i].ID]
		if tags == nil {
			tags = []journal.Tag{}
		}
		out[i].Tags = tags
	}
	return out, nil
}

// latestEmitRollOutcome reports the outcome of the most recent emit_pool
// roll attempt for a template — surfaced in the UI so a command that's
// stably failing (outcome=error, retries exhausted for the period) doesn't
// go unnoticed until someone stumbles on the resulting failed quest.
func (s *Store) latestEmitRollOutcome(ctx context.Context, templateID int64) (string, error) {
	var outcome string
	err := s.DB.QueryRowContext(ctx, `
		SELECT outcome FROM templateemitroll
		WHERE template_id = ? ORDER BY id DESC LIMIT 1`, templateID).Scan(&outcome)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return outcome, nil
}

func (s *Store) GetTemplate(ctx context.Context, id int64) (TemplateRead, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT t.id, t.title, t.description, t.pinned, t.sort_order, t.duration_seconds, t.freq, t.weekdays,
			t.enabled, t.timezone, t.deadline_time, t.significance, t.emit_mode, t.emit_chance,
			t.emit_window_start, t.emit_window_end, t.emit_pool_command, t.emit_limits, t.emit_run_on,
			t.category_id, t.questline_id,
			t.created_at, t.updated_at, t.automated,
			c.slug, c.label, c.color, l.title, l.color, l.icon, l.custom_icon, l.updated_at, l.id
		FROM questtemplate t
		LEFT JOIN questcategory c ON c.id = t.category_id
		LEFT JOIN questline l ON l.id = t.questline_id
		WHERE t.id = ?`, id)
	tr, err := scanTemplate(row)
	if err == sql.ErrNoRows {
		return TemplateRead{}, ErrNotFound
	}
	if err != nil {
		return TemplateRead{}, err
	}
	steps, err := s.loadTemplateStepsRead(ctx, id)
	if err != nil {
		return TemplateRead{}, err
	}
	tr.Steps = steps
	if tr.EmitPoolCmd != nil {
		outcome, err := s.latestEmitRollOutcome(ctx, id)
		if err != nil {
			return TemplateRead{}, err
		}
		if outcome != "" {
			tr.EmitPoolLastOutcome = &outcome
		}
	}
	tagMap, err := s.Journal().TagsForTemplates(ctx, []int64{id})
	if err != nil {
		return TemplateRead{}, err
	}
	tags := tagMap[id]
	if tags == nil {
		tags = []journal.Tag{}
	}
	tr.Tags = tags
	return tr, nil
}

func (s *Store) CreateTemplate(ctx context.Context, body map[string]any) (TemplateRead, error) {
	now := timeutil.NowUTC()
	title, _ := body["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		return TemplateRead{}, errBad("title required")
	}
	desc, _ := asString(body["description"])
	pinned := journal.PinState(asBool(body["pinned"], false), asBool(body["overlay_hidden"], false))
	sortOrder := asInt(body["sort_order"], 0)
	freq := asStringDef(body["freq"], "daily")
	weekdays := asStringDef(body["weekdays"], "0,1,2,3,4,5,6")
	enabled := asBool(body["enabled"], true)
	tz := asStringDef(body["timezone"], "Europe/Moscow")
	sig := asStringDef(body["significance"], "common")
	if !journal.Significance(sig).Valid() {
		return TemplateRead{}, errBad(journal.InvalidSignificanceMsg(sig))
	}
	emitMode := asStringDef(body["emit_mode"], "fixed")
	emitChance := asFloat(body["emit_chance"], 1.0)
	emitLimits, err := emitLimitsColumn(body["emit_limits"])
	if err != nil {
		return TemplateRead{}, errBad(err.Error())
	}
	colorCat := nullI64(asI64Ptr(body["category_id"]))
	lineID := nullI64(asI64Ptr(body["questline_id"]))
	emitRunOn := NormalizeRunOn(asStringPtr(body["emit_run_on"]))
	if !ValidRunOn(emitRunOn) {
		return TemplateRead{}, errBad("emit_run_on: " + ErrBadRunnerLabel.Error())
	}

	if err := s.guardAuthor(ctx, asStringPtr(body["emit_pool_command"]), nil); err != nil {
		return TemplateRead{}, err
	}
	if err := s.guardExec(ctx, asStringPtr(body["emit_pool_command"]), emitRunOn); err != nil {
		return TemplateRead{}, err
	}
	if raw, ok := body["steps"]; ok {
		if steps, ok := raw.([]any); ok {
			if err := s.guardTemplateSteps(ctx, 0, steps); err != nil {
				return TemplateRead{}, err
			}
		}
	}
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO questtemplate (
			title, description, pinned, sort_order, duration_seconds, freq, weekdays, enabled, timezone,
			deadline_time, significance, emit_mode, emit_chance, emit_window_start, emit_window_end,
			emit_pool_command, emit_limits, emit_run_on,
			category_id, questline_id, created_at, updated_at, automated
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		title, desc, pinned, sortOrder, nullInt(asIntPtr(body["duration_seconds"])), freq, weekdays, boolInt(enabled), tz,
		nullStr(asStringPtr(body["deadline_time"])), sig, emitMode, emitChance,
		nullStr(asStringPtr(body["emit_window_start"])), nullStr(asStringPtr(body["emit_window_end"])),
		nullStr(asStringPtr(body["emit_pool_command"])), emitLimits, nullStr(emitRunOn),
		colorCat, lineID,
		timeutil.ToDBUTC(now), timeutil.ToDBUTC(now), boolInt(asBool(body["automated"], false)),
	)
	if err != nil {
		return TemplateRead{}, err
	}
	tid, _ := res.LastInsertId()
	if err := s.replaceTemplateSteps(ctx, tid, body["steps"]); err != nil {
		return TemplateRead{}, err
	}
	if err := s.applyTemplateTagIDs(ctx, tid, body); err != nil {
		return TemplateRead{}, err
	}
	return s.GetTemplate(ctx, tid)
}

func (s *Store) UpdateTemplate(ctx context.Context, id int64, body map[string]any) (TemplateRead, error) {
	if v, ok := body["significance"]; ok {
		if sg, _ := v.(string); !journal.Significance(sg).Valid() {
			return TemplateRead{}, errBad(journal.InvalidSignificanceMsg(sg))
		}
	}
	cur, err := s.GetTemplate(ctx, id)
	if err != nil {
		return TemplateRead{}, err
	}
	merged := cur.body()
	for k, v := range body {
		merged[k] = v
	}
	// pinned and overlay_hidden share a column: the one just switched on wins.
	if asBool(body["overlay_hidden"], false) {
		merged["pinned"] = false
	} else if asBool(body["pinned"], false) {
		merged["overlay_hidden"] = false
	}
	now := timeutil.NowUTC()
	emitLimits, lerr := emitLimitsColumn(merged["emit_limits"])
	if lerr != nil {
		return TemplateRead{}, errBad(lerr.Error())
	}
	emitRunOn := NormalizeRunOn(asStringPtr(merged["emit_run_on"]))
	if !ValidRunOn(emitRunOn) {
		return TemplateRead{}, errBad("emit_run_on: " + ErrBadRunnerLabel.Error())
	}
	if err := s.guardAuthor(ctx, asStringPtr(merged["emit_pool_command"]), cur.EmitPoolCmd); err != nil {
		return TemplateRead{}, err
	}
	if err := s.guardExec(ctx, asStringPtr(merged["emit_pool_command"]), emitRunOn); err != nil {
		return TemplateRead{}, err
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE questtemplate SET
			title=?, description=?, pinned=?, sort_order=?, duration_seconds=?, freq=?, weekdays=?,
			enabled=?, timezone=?, deadline_time=?, significance=?, emit_mode=?, emit_chance=?,
			emit_window_start=?, emit_window_end=?, emit_pool_command=?, emit_limits=?, emit_run_on=?,
			category_id=?, questline_id=?,
			updated_at=?, automated=?
		WHERE id=?`,
		asStringDef(merged["title"], ""), asStringDef(merged["description"], ""),
		journal.PinState(asBool(merged["pinned"], false), asBool(merged["overlay_hidden"], false)), asInt(merged["sort_order"], 0),
		nullInt(asIntPtr(merged["duration_seconds"])), asStringDef(merged["freq"], "daily"),
		asStringDef(merged["weekdays"], "0,1,2,3,4,5,6"), boolInt(asBool(merged["enabled"], true)),
		asStringDef(merged["timezone"], "Europe/Moscow"), nullStr(asStringPtr(merged["deadline_time"])),
		asStringDef(merged["significance"], "common"), asStringDef(merged["emit_mode"], "fixed"),
		asFloat(merged["emit_chance"], 1.0), nullStr(asStringPtr(merged["emit_window_start"])),
		nullStr(asStringPtr(merged["emit_window_end"])), nullStr(asStringPtr(merged["emit_pool_command"])), emitLimits, nullStr(emitRunOn),
		nullI64(asI64Ptr(merged["category_id"])), nullI64(asI64Ptr(merged["questline_id"])),
		timeutil.ToDBUTC(now), boolInt(asBool(merged["automated"], false)), id,
	)
	if err != nil {
		return TemplateRead{}, err
	}
	if _, ok := body["steps"]; ok {
		if err := s.replaceTemplateSteps(ctx, id, body["steps"]); err != nil {
			return TemplateRead{}, err
		}
	}
	if _, ok := body["tag_ids"]; ok {
		if err := s.applyTemplateTagIDs(ctx, id, body); err != nil {
			return TemplateRead{}, err
		}
	} else if _, ok := body["tags"]; ok {
		if err := s.applyTemplateTagIDs(ctx, id, body); err != nil {
			return TemplateRead{}, err
		}
	}
	return s.GetTemplate(ctx, id)
}

func (s *Store) DeleteTemplate(ctx context.Context, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Detach, don't touch: quests this template already materialized are
	// independent records once created — quest.template_id has no ON DELETE
	// clause, so leaving it pointing at a row we're about to delete would
	// trip the FK constraint and fail the whole delete (this only started
	// showing up once emit_pool made a fresh template materialize a quest
	// immediately on create, so a same-session delete-right-after-testing
	// now always hits it).
	if err := copyTemplateSecretsToLiveQuests(ctx, tx, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE quest SET template_id=NULL WHERE template_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM templateemitroll WHERE template_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM questtemplatestep WHERE template_id=?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM questtemplate WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) CopyTemplate(ctx context.Context, id int64) (TemplateRead, error) {
	src, err := s.GetTemplate(ctx, id)
	if err != nil {
		return TemplateRead{}, err
	}
	body := src.body()
	body["title"] = src.Title + " (копия)"
	body["enabled"] = false
	delete(body, "id")
	delete(body, "created_at")
	delete(body, "updated_at")
	// Keep tags so CreateTemplate can re-link via applyTemplateTagIDs.
	return s.CreateTemplate(ctx, body)
}

func (s *Store) applyTemplateTagIDs(ctx context.Context, templateID int64, body map[string]any) error {
	ids, ok, err := parseTagIDsFromBody(body)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return s.Journal().SetTemplateTags(ctx, templateID, ids)
}

// guardTemplateSteps checks the commands of a template's incoming steps; tid is the
// template being edited (0 = new): commands it already has stay allowed for anyone.
func (s *Store) guardTemplateSteps(ctx context.Context, tid int64, steps []any) error {
	existing := map[string]bool{}
	if tid > 0 {
		rows, err := s.DB.QueryContext(ctx, `SELECT check_command FROM questtemplatestep WHERE template_id = ? AND check_command IS NOT NULL`, tid)
		if err != nil {
			return err
		}
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err == nil {
				existing[strings.TrimSpace(c)] = true
			}
		}
		rows.Close()
	}
	for _, item := range steps {
		m, _ := item.(map[string]any)
		if m == nil {
			b, _ := json.Marshal(item)
			_ = json.Unmarshal(b, &m)
		}
		cmd := asStringPtr(m["check_command"])
		var old *string
		if cmd != nil && existing[strings.TrimSpace(*cmd)] {
			old = cmd
		}
		if err := s.guardAuthor(ctx, cmd, old); err != nil {
			return err
		}
		if err := s.guardExec(ctx, cmd, NormalizeRunOn(asStringPtr(m["run_on"]))); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) replaceTemplateSteps(ctx context.Context, tid int64, raw any) error {
	steps, ok := raw.([]any)
	if !ok {
		// try []map from json decode into map[string]any
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, &steps)
	}
	// Refuse server-side commands before touching the existing steps.
	if err := s.guardTemplateSteps(ctx, tid, steps); err != nil {
		return err
	}
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM questtemplatestep WHERE template_id=?`, tid)
	if len(steps) == 0 {
		_, err := s.DB.ExecContext(ctx, `
			INSERT INTO questtemplatestep (template_id, title, description, sort_order, progress_min, progress_max)
			VALUES (?, ?, '', 0, 1, 1)`, tid, "Шаг")
		return err
	}
	for i, item := range steps {
		m, _ := item.(map[string]any)
		if m == nil {
			b, _ := json.Marshal(item)
			_ = json.Unmarshal(b, &m)
		}
		title := strings.TrimSpace(asStringDef(m["title"], ""))
		if title == "" {
			continue
		}
		pmin := asInt(m["progress_min"], 0)
		pmax := asInt(m["progress_max"], 0)
		if pmin == 0 && pmax == 0 {
			pt := asInt(m["progress_total"], 1)
			pmin, pmax = pt, pt
		}
		if pmin < 1 {
			pmin = 1
		}
		if pmax < 1 {
			pmax = pmin
		}
		ord := asInt(m["sort_order"], i)
		_, err := s.DB.ExecContext(ctx, `
			INSERT INTO questtemplatestep (
				template_id, title, description, sort_order, progress_min, progress_max,
				check_command, check_interval_seconds, wait_previous, run_mode, run_on
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			tid, title, asStringDef(m["description"], ""), ord, pmin, pmax,
			nullStr(asStringPtr(m["check_command"])), nullInt(asIntPtr(m["check_interval_seconds"])),
			boolInt(asBool(m["wait_previous"], false)), journal.NullRunMode(asStringDef(m["run_mode"], journal.RunModePoll)),
			nullStr(NormalizeRunOn(asStringPtr(m["run_on"]))),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) loadTemplateStepsRead(ctx context.Context, tid int64) ([]TemplateStepRead, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, template_id, title, description, sort_order, progress_min, progress_max,
			check_command, check_interval_seconds, wait_previous, run_mode, run_on
		FROM questtemplatestep WHERE template_id=? ORDER BY sort_order, id`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TemplateStepRead, 0)
	for rows.Next() {
		var m TemplateStepRead
		var cmd sql.NullString
		var iv sql.NullInt64
		var waitPrev int
		var runMode, runOn sql.NullString
		if err := rows.Scan(&m.ID, &m.TemplateID, &m.Title, &m.Description, &m.SortOrder, &m.ProgressMin, &m.ProgressMax, &cmd, &iv, &waitPrev, &runMode, &runOn); err != nil {
			return nil, err
		}
		m.WaitPrevious = waitPrev != 0
		m.RunMode = journal.RunModePoll
		if runOn.Valid && strings.TrimSpace(runOn.String) != "" {
			v := strings.TrimSpace(runOn.String)
			m.RunOn = &v
		}
		if cmd.Valid {
			m.CheckCommand = &cmd.String
		}
		if iv.Valid {
			v := int(iv.Int64)
			m.CheckIntervalSeconds = &v
		}
		if runMode.Valid && strings.TrimSpace(runMode.String) != "" {
			m.RunMode = journal.NormalizeRunMode(runMode.String)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
