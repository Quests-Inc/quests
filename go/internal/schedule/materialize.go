package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/valirum/quests/go/internal/automation"
	"github.com/valirum/quests/go/internal/journal"
	"github.com/valirum/quests/go/internal/platform/events"
	"github.com/valirum/quests/go/internal/platform/timeutil"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/store"
)

// emitPoolTimeout bounds how long emit_pool_command may run per attempt.
var emitPoolTimeout = 20 * time.Second

// emitPoolMaxAttempts caps retries of a failing/invalid emit_pool_command
// within one period before giving up (outcome=error) until the next period.
const emitPoolMaxAttempts = 4

// emitPoolRetryDelays is the pause after the 1st, 2nd, … failed run before
// the next one (len == emitPoolMaxAttempts-1): a brief outage of whatever the
// command talks to must not burn all attempts within a single minute.
var emitPoolRetryDelays = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

func defaultTZ() string {
	if v := strings.TrimSpace(os.Getenv("QUESTS_TZ")); v != "" {
		return v
	}
	return "Europe/Moscow"
}

type templateRow struct {
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

type templateStepRow struct {
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

// MaterializeDue creates quest instances for due templates (fixed + surprise).
func MaterializeDue(ctx context.Context, st *store.Store, hub *events.Hub, now time.Time, rng *rand.Rand) ([]int64, error) {
	return materializeDue(ctx, st, hub, now, rng)
}

func materializeDue(ctx context.Context, st *store.Store, hub *events.Hub, now time.Time, rng *rand.Rand) ([]int64, error) {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if now.IsZero() {
		now = timeutil.NowUTC()
	}
	rows, err := st.DB.QueryContext(ctx, `
		SELECT id, title, description, pinned, sort_order, duration_seconds, freq, weekdays,
			enabled, timezone, deadline_time, significance, emit_mode, emit_chance,
			emit_window_start, emit_window_end, emit_pool_command, emit_limits, emit_run_on,
			category_id, questline_id, automated
		FROM questtemplate WHERE enabled = 1 ORDER BY sort_order, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var templates []templateRow
	for rows.Next() {
		var t templateRow
		var pinned, enabled, automated int
		if err := rows.Scan(
			&t.ID, &t.Title, &t.Description, &pinned, &t.SortOrder, &t.DurationSeconds, &t.Freq, &t.Weekdays,
			&enabled, &t.Timezone, &t.DeadlineTime, &t.Significance, &t.EmitMode, &t.EmitChance,
			&t.EmitWindowStart, &t.EmitWindowEnd, &t.EmitPoolCommand, &t.EmitLimits, &t.EmitRunOn,
			&t.CategoryID, &t.QuestlineID, &automated,
		); err != nil {
			return nil, err
		}
		t.Pinned, t.OverlayHidden = journal.SplitPinState(pinned)
		t.Enabled = enabled != 0
		t.Automated = automated != 0
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	created := make([]int64, 0)
	for _, tmpl := range templates {
		tzName := tmpl.Timezone
		if tzName == "" {
			tzName = defaultTZ()
		}
		loc, err := time.LoadLocation(tzName)
		if err != nil {
			loc, _ = time.LoadLocation(defaultTZ())
			if loc == nil {
				loc = time.UTC
			}
		}
		localNow := now.In(loc)
		if !templateDueToday(tmpl.Freq, tmpl.Weekdays, localNow) {
			continue
		}
		key := localNow.Format("2006-01-02")

		var existing sql.NullInt64
		err = st.DB.QueryRowContext(ctx, `
			SELECT id FROM quest WHERE template_id = ? AND period_key = ? LIMIT 1`,
			tmpl.ID, key).Scan(&existing)
		if err == nil && existing.Valid {
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			return created, err
		}

		emitMode := strings.ToLower(strings.TrimSpace(tmpl.EmitMode))
		var surpriseRollID sql.NullInt64
		var deadline *time.Time
		var duration *int

		if emitMode == "surprise" || emitMode == "random" || emitMode == "chance" {
			rollID, outcome, scheduledAt, err := ensureSurpriseRoll(ctx, st, tmpl, key, localNow, rng)
			if err != nil {
				return created, err
			}
			if outcome == "miss" || outcome == "materialized" {
				continue
			}
			if outcome == "scheduled" && scheduledAt != nil && now.Before(*scheduledAt) {
				continue
			}
			surpriseRollID = sql.NullInt64{Int64: rollID, Valid: true}
			deadline, duration = surpriseDeadline(tmpl, now)
		} else {
			deadline, duration = fixedDeadline(tmpl, localNow, loc)
		}

		usePool := tmpl.EmitPoolCommand.Valid && strings.TrimSpace(tmpl.EmitPoolCommand.String) != ""

		// A fixed-mode pool template with deadline_time set describes its own
		// "check window": duration_seconds before deadline_time, defaulting to
		// 0 (check right at deadline_time) when duration_seconds isn't set —
		// NOT fixedDeadline's own fallback duration (deadline minus midnight),
		// which is a placeholder for the resulting quest's own duration field,
		// unrelated to when the check itself should run. Without this gate, a
		// freshness-sensitive command (unread mail etc.) ran at the period's
		// very first tick — right after local midnight — and its outcome (miss
		// included; see resolveEmitPool) locked in for the rest of the day, so
		// anything that showed up later had no chance to be seen until the next
		// period. Only fixed mode needs this: surprise mode already has its own
		// wait-for-scheduledAt gate above, and a template with no deadline_time
		// at all has no window to speak of, so it keeps firing at first tick.
		if usePool && emitMode != "surprise" && emitMode != "random" && emitMode != "chance" && deadline != nil {
			gateDuration := 0
			if tmpl.DurationSeconds.Valid {
				gateDuration = int(tmpl.DurationSeconds.Int64)
			}
			if now.Before(emitPoolOpenAt(*deadline, gateDuration)) {
				continue
			}
		}
		var poolRollID int64
		var poolFailed bool
		var poolFailMsg string
		var spec *resolvedSpec
		if usePool {
			res, perr := resolveEmit(ctx, st, tmpl, key, now, poolOpts{})
			if perr != nil {
				return created, perr
			}
			poolRollID = res.RollID
			switch {
			case res.FailMsg != "":
				// Attempts just ran out — materialize a failed quest with the last
				// attempt's trace instead of silently doing nothing.
				poolFailed = true
				poolFailMsg = res.FailMsg
			case res.Spec == nil:
				// retry pending, still running, or "no quest this period"
				continue
			default:
				spec = res.Spec
			}
		}
		var steps []journal.Step
		switch {
		case poolFailed:
			steps = []journal.Step{{
				Title:         "Почини команду пула шаблона «" + tmpl.Title + "»",
				ProgressTotal: 1,
			}}
		case spec != nil && len(spec.Steps) > 0:
			steps = spec.Steps
		default:
			steps, err = loadTemplateSteps(ctx, st, tmpl, rng)
			if err != nil {
				return created, err
			}
		}

		q := journal.Quest{
			Title:           tmpl.Title,
			Description:     tmpl.Description,
			Status:          journal.StatusActive,
			Significance:    journal.Significance(tmpl.Significance),
			Pinned:          tmpl.Pinned,
			OverlayHidden:   tmpl.OverlayHidden,
			SortOrder:       tmpl.SortOrder,
			DeadlineAt:      deadline,
			DurationSeconds: duration,
			CreatedAt:       now,
			UpdatedAt:       now,
			Automated:       tmpl.Automated,
			Steps:           steps,
		}
		if poolFailed {
			q.Status = journal.StatusFailed
			q.Description = fmt.Sprintf(
				"`emit_pool_command` провалилась %d раза подряд за этот период. Трейс последней попытки:\n\n```\n%s\n```",
				emitPoolMaxAttempts, poolFailMsg,
			)
		}
		if tmpl.Significance == "" {
			q.Significance = journal.SigCommon
		}
		if tmpl.CategoryID.Valid {
			v := tmpl.CategoryID.Int64
			q.CategoryID = &v
		}
		if tmpl.QuestlineID.Valid {
			v := tmpl.QuestlineID.Int64
			q.QuestlineID = &v
		}
		if spec != nil {
			applyResolvedSpec(ctx, st, &q, tmpl, spec, now)
		}
		followQuestlineCategory(ctx, st, &q)
		tid := tmpl.ID
		q.TemplateID = &tid
		pk := key
		q.PeriodKey = &pk
		source := fmt.Sprintf("template:%d", tmpl.ID)
		q.Source = &source

		// Create without auto quest_created — we emit quest_appeared.
		createdQ, err := st.Journal().CreateQuestAppeared(ctx, q)
		if err != nil {
			return created, err
		}
		if err := attachTags(ctx, st, tmpl, createdQ.ID, spec); err != nil {
			return created, err
		}
		if surpriseRollID.Valid {
			_, _ = st.DB.ExecContext(ctx, `
				UPDATE templateemitroll SET outcome = 'materialized', updated_at = ? WHERE id = ?`,
				timeutil.ToDBUTC(now), surpriseRollID.Int64)
		}
		if usePool && poolRollID != 0 {
			_, _ = st.DB.ExecContext(ctx, `
				UPDATE templateemitroll SET outcome = 'materialized', updated_at = ? WHERE id = ?`,
				timeutil.ToDBUTC(now), poolRollID)
		}
		qid := createdQ.ID
		detail := "Период " + key
		hub.Publish("quest_appeared", events.PublishOpts{
			Workspace:   wsctx.From(ctx),
			QuestID:     &qid,
			Title:       createdQ.Title,
			Description: createdQ.Description,
			Detail:      detail,
			// Periodic materialization is routine, not something to notice —
			// a fullscreen major toast for every daily template at midnight
			// is exactly the "pack of identical alerts" this was meant to fix.
			Toast:        false,
			Source:       "system",
			Significance: string(createdQ.Significance),
			Automated:    createdQ.Automated,
			Sound:        strPtr("quest_created"),
		})
		created = append(created, qid)
	}
	return created, nil
}

func strPtr(s string) *string { return &s }

func templateDueToday(freq, weekdays string, localNow time.Time) bool {
	if strings.ToLower(freq) == "daily" || freq == "" {
		return true
	}
	days := parseWeekdays(weekdays)
	if len(days) == 0 {
		return true
	}
	// Python weekday(): Mon=0 … Sun=6. Go: Sun=0 … Sat=6.
	py := (int(localNow.Weekday()) + 6) % 7
	_, ok := days[py]
	return ok
}

// TemplateDueTodayForTest exports weekday matching for unit tests.
func TemplateDueTodayForTest(freq, weekdays string, localNow time.Time) bool {
	return templateDueToday(freq, weekdays, localNow)
}

func parseWeekdays(raw string) map[int]struct{} {
	out := map[int]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 6 {
			continue
		}
		out[n] = struct{}{}
	}
	return out
}

func parseClock(raw string) (hour, min, sec int, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, 0, false
	}
	parts := strings.Split(raw, ":")
	if len(parts) < 2 {
		return 0, 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	s := 0
	if len(parts) > 2 {
		s, _ = strconv.Atoi(parts[2])
	}
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 || s < 0 || s > 59 {
		return 0, 0, 0, false
	}
	return h, m, s, true
}

func fixedDeadline(tmpl templateRow, localNow time.Time, loc *time.Location) (*time.Time, *int) {
	if !tmpl.DeadlineTime.Valid {
		return nil, nil
	}
	h, m, s, ok := parseClock(tmpl.DeadlineTime.String)
	if !ok {
		return nil, nil
	}
	deadlineLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), h, m, s, 0, loc)
	deadlineUTC := deadlineLocal.UTC()
	var duration int
	if tmpl.DurationSeconds.Valid {
		duration = int(tmpl.DurationSeconds.Int64)
		if duration < 1 {
			duration = 1
		}
	} else {
		startLocal := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
		if !deadlineLocal.After(startLocal) {
			startLocal = localNow
		}
		duration = int(deadlineLocal.Sub(startLocal).Seconds())
		if duration < 60 {
			duration = 60
		}
	}
	return &deadlineUTC, &duration
}

// emitPoolOpenAt is the moment a fixed-mode pool template's own check window
// opens: duration_seconds before its deadline. Before this moment,
// MaterializeDue skips the template entirely for the tick instead of running
// emit_pool_command — see the call site's comment for why.
func emitPoolOpenAt(deadline time.Time, durationSeconds int) time.Time {
	return deadline.Add(-time.Duration(durationSeconds) * time.Second)
}

func surpriseDeadline(tmpl templateRow, nowUTC time.Time) (*time.Time, *int) {
	if !tmpl.DurationSeconds.Valid {
		return nil, nil
	}
	duration := int(tmpl.DurationSeconds.Int64)
	if duration < 1 {
		duration = 1
	}
	deadline := nowUTC.Add(time.Duration(duration) * time.Second)
	return &deadline, &duration
}

func ensureSurpriseRoll(ctx context.Context, st *store.Store, tmpl templateRow, periodKey string, localNow time.Time, rng *rand.Rand) (int64, string, *time.Time, error) {
	var id int64
	var outcome string
	var scheduled sql.NullString
	err := st.DB.QueryRowContext(ctx, `
		SELECT id, outcome, scheduled_at FROM templateemitroll
		WHERE template_id = ? AND period_key = ?`, tmpl.ID, periodKey).Scan(&id, &outcome, &scheduled)
	if err == nil {
		var sched *time.Time
		if scheduled.Valid {
			t, e := timeutil.ParseFlexible(scheduled.String)
			if e == nil {
				sched = &t
			}
		}
		return id, outcome, sched, nil
	}
	if err != sql.ErrNoRows {
		return 0, "", nil, err
	}

	chance := tmpl.EmitChance
	if chance < 0 {
		chance = 0
	}
	if chance > 1 {
		chance = 1
	}
	now := timeutil.NowUTC()
	if rng.Float64() >= chance {
		res, err := st.DB.ExecContext(ctx, `
			INSERT INTO templateemitroll (template_id, period_key, outcome, scheduled_at, created_at, updated_at)
			VALUES (?, ?, 'miss', NULL, ?, ?)`, tmpl.ID, periodKey, timeutil.ToDBUTC(now), timeutil.ToDBUTC(now))
		if err != nil {
			return 0, "", nil, err
		}
		id, _ = res.LastInsertId()
		return id, "miss", nil, nil
	}

	startRaw, endRaw := "", ""
	if tmpl.EmitWindowStart.Valid {
		startRaw = tmpl.EmitWindowStart.String
	}
	if tmpl.EmitWindowEnd.Valid {
		endRaw = tmpl.EmitWindowEnd.String
	}
	scheduledLocal := pickScheduledAt(localNow, startRaw, endRaw, rng)
	schedUTC := scheduledLocal.UTC()
	res, err := st.DB.ExecContext(ctx, `
		INSERT INTO templateemitroll (template_id, period_key, outcome, scheduled_at, created_at, updated_at)
		VALUES (?, ?, 'scheduled', ?, ?, ?)`,
		tmpl.ID, periodKey, timeutil.ToDBUTC(schedUTC), timeutil.ToDBUTC(now), timeutil.ToDBUTC(now))
	if err != nil {
		return 0, "", nil, err
	}
	id, _ = res.LastInsertId()
	return id, "scheduled", &schedUTC, nil
}

func pickScheduledAt(localDay time.Time, windowStart, windowEnd string, rng *rand.Rand) time.Time {
	sh, sm, ss, okS := parseClock(windowStart)
	eh, em, es, okE := parseClock(windowEnd)
	if !okS {
		sh, sm, ss = 0, 0, 0
	}
	if !okE {
		eh, em, es = 23, 59, 0
	}
	loc := localDay.Location()
	start := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), sh, sm, ss, 0, loc)
	end := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), eh, em, es, 0, loc)
	if end.Before(start) {
		start, end = end, start
	}
	span := int(end.Sub(start).Seconds())
	if span < 0 {
		span = 0
	}
	offset := 0
	if span > 0 {
		offset = rng.Intn(span + 1)
	}
	return start.Add(time.Duration(offset) * time.Second)
}

func loadTemplateSteps(ctx context.Context, st *store.Store, tmpl templateRow, rng *rand.Rand) ([]journal.Step, error) {
	rows, err := st.DB.QueryContext(ctx, `
		SELECT title, description, sort_order, progress_min, progress_max, check_command, check_interval_seconds,
			wait_previous, run_mode, run_on
		FROM questtemplatestep WHERE template_id = ? ORDER BY sort_order, id`, tmpl.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var src []templateStepRow
	for rows.Next() {
		var s templateStepRow
		var waitPrev int
		var runMode sql.NullString
		if err := rows.Scan(&s.Title, &s.Description, &s.SortOrder, &s.ProgressMin, &s.ProgressMax, &s.CheckCommand, &s.CheckIntervalSeconds, &waitPrev, &runMode, &s.RunOn); err != nil {
			return nil, err
		}
		s.WaitPrevious = waitPrev != 0
		s.RunMode = journal.RunModePoll
		if runMode.Valid {
			s.RunMode = journal.NormalizeRunMode(runMode.String)
		}
		src = append(src, s)
	}
	if len(src) == 0 {
		return []journal.Step{{Title: tmpl.Title, ProgressTotal: 1, SortOrder: 0}}, nil
	}
	out := make([]journal.Step, 0, len(src))
	for i, s := range src {
		lo, hi := s.ProgressMin, s.ProgressMax
		if lo < 1 {
			lo = 1
		}
		if hi < 1 {
			hi = 1
		}
		if hi < lo {
			lo, hi = hi, lo
		}
		total := lo
		if hi > lo {
			total = lo + rng.Intn(hi-lo+1)
		}
		st := journal.Step{
			Title: s.Title, Description: s.Description,
			ProgressCurrent: 0, ProgressTotal: total, SortOrder: s.SortOrder,
		}
		if st.SortOrder == 0 && i > 0 {
			st.SortOrder = i
		}
		if s.CheckCommand.Valid && strings.TrimSpace(s.CheckCommand.String) != "" {
			c := s.CheckCommand.String
			st.CheckCommand = &c
			st.WaitPrevious = s.WaitPrevious
			st.RunMode = s.RunMode
			if s.RunOn.Valid {
				st.RunOn = store.NormalizeRunOn(&s.RunOn.String)
			}
			if st.RunMode == journal.RunModePoll || st.RunMode == journal.RunModeWatch {
				iv := 15
				if s.CheckIntervalSeconds.Valid {
					iv = int(s.CheckIntervalSeconds.Int64)
					if iv < 15 {
						iv = 15
					}
				}
				st.CheckIntervalSeconds = &iv
			} else if s.CheckIntervalSeconds.Valid {
				iv := int(s.CheckIntervalSeconds.Int64)
				if iv >= 15 {
					st.CheckIntervalSeconds = &iv
				}
			}
			if st.RunMode == journal.RunModeOnce {
				idle := journal.RunIdle
				st.RunStatus = &idle
			}
		}
		out = append(out, st)
	}
	return out, nil
}

// emitResult is what one tick learns about a template's emit command.
// Spec is set when there is a quest to create now (the command printed one); nil means nothing this tick
// (pending retry, still running, or "no quest this period"). FailMsg is
// non-empty exactly when this call is the one that exhausted the attempts
// (outcome just became "error") — the caller then creates a failed quest
// carrying the trace, since nothing else surfaces it to the user.
type emitResult struct {
	Spec    *resolvedSpec
	RollID  int64
	FailMsg string
}

// poolOpts tunes resolveEmit. ignorePause lets a manual run ("materialize
// now") go ahead even while a retry pause is pending.
type poolOpts struct {
	ignorePause bool
}

// resolveEmit drives one template's emit_pool_command for the current period:
// runs it (retrying with pauses on failure, up to emitPoolMaxAttempts),
// validates the quest it printed against the template's limits and persists the
// outcome in templateemitroll so repeated ticks don't re-run or re-create.
func resolveEmit(ctx context.Context, st *store.Store, tmpl templateRow, periodKey string, now time.Time, opts poolOpts) (emitResult, error) {
	var id int64
	var outcome string
	var attempts int
	var retryAt sql.NullString
	isNew := false
	qerr := st.DB.QueryRowContext(ctx, `
		SELECT id, outcome, attempts, retry_at FROM templateemitroll
		WHERE template_id = ? AND period_key = ?`, tmpl.ID, periodKey).
		Scan(&id, &outcome, &attempts, &retryAt)
	if qerr == sql.ErrNoRows {
		isNew = true
	} else if qerr != nil {
		return emitResult{}, qerr
	}
	if !isNew && (outcome == "materialized" || outcome == "error" || outcome == "miss") {
		return emitResult{RollID: id}, nil
	}
	// A failed run scheduled its own retry: until then the tick leaves it alone.
	if !isNew && retryAt.Valid && !opts.ignorePause {
		if at, perr := timeutil.ParseFlexible(retryAt.String); perr == nil && now.Before(at) {
			return emitResult{RollID: id}, nil
		}
	}

	attemptNo := attempts + 1
	var out emitOutput
	var info execInfo
	var execErr error
	if tmpl.EmitRunOn.Valid && strings.TrimSpace(tmpl.EmitRunOn.String) != "" {
		// The command runs on a runner device: queue it, collect the result on a later tick.
		var ready bool
		out, info, execErr, ready = execEmitRemote(ctx, st, tmpl, periodKey)
		if !ready {
			return emitResult{RollID: id}, nil
		}
	} else {
		// Commands run on runners only (see store.ErrNoRunner).
		info.Status = "error"
		execErr = store.ErrNoRunner
	}
	rec := automation.EmitAttempt{
		TemplateID: tmpl.ID, PeriodKey: periodKey, At: now, Attempt: attemptNo,
		Status: info.Status, DurationMS: info.Duration.Milliseconds(),
		Message: info.Stderr,
	}
	// Logging never fails the roll: it's diagnostics, not state.
	record := func() {
		if rerr := st.Automation().RecordEmitAttempt(ctx, rec); rerr != nil {
			log.Printf("emit attempt log (template %d): %v", tmpl.ID, rerr)
		}
	}

	var resolved *resolvedSpec
	var note string
	if execErr == nil {
		spec := out.Spec
		if spec != nil {
			lim, lerr := automation.ParseEmitLimits(tmpl.EmitLimits.String)
			if lerr != nil {
				lim = automation.DefaultEmitLimits()
				log.Printf("emit limits (template %d): %v; using defaults", tmpl.ID, lerr)
			}
			var rerr error
			resolved, rerr = resolveEmitSpec(ctx, st, spec, lim)
			if rerr != nil {
				execErr = fmt.Errorf("invalid quest: %w", rerr)
				info.Status = "bad_spec"
				rec.Status = "bad_spec"
			}
		}
	}

	if execErr != nil {
		attempts++
		newOutcome := "scheduled"
		var nextRetry any
		if attempts >= emitPoolMaxAttempts {
			newOutcome = "error"
			rec.Message = "giving up: attempts exhausted\n" + execErr.Error()
		} else {
			delay := emitPoolRetryDelays[min(attempts-1, len(emitPoolRetryDelays)-1)]
			nextRetry = timeutil.ToDBUTC(now.Add(delay))
			rec.Message = fmt.Sprintf("retry in %s\n", delay) + execErr.Error()
		}
		record()
		if isNew {
			res, err := st.DB.ExecContext(ctx, `
				INSERT INTO templateemitroll (template_id, period_key, outcome, attempts, retry_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				tmpl.ID, periodKey, newOutcome, attempts, nextRetry, timeutil.ToDBUTC(now), timeutil.ToDBUTC(now))
			if err != nil {
				return emitResult{}, err
			}
			id, _ = res.LastInsertId()
		} else {
			if _, err := st.DB.ExecContext(ctx, `
				UPDATE templateemitroll SET outcome = ?, attempts = ?, retry_at = ?, updated_at = ? WHERE id = ?`,
				newOutcome, attempts, nextRetry, timeutil.ToDBUTC(now), id); err != nil {
				return emitResult{}, err
			}
		}
		if newOutcome == "error" {
			return emitResult{RollID: id, FailMsg: execErr.Error()}, nil
		}
		return emitResult{RollID: id}, nil
	}

	if resolved == nil {
		rec.Message = joinNote("miss: the command printed no quest", rec.Message)
		record()
		id, err := persistEmitPoolOutcome(ctx, st, id, isNew, tmpl.ID, periodKey, "miss", attempts, "", now)
		if err != nil {
			return emitResult{}, err
		}
		return emitResult{RollID: id}, nil
	}

	rec.Items = len(resolved.Steps)
	rec.Picked = 1
	title := resolved.Title
	if title == "" {
		title = tmpl.Title
	}
	msg := fmt.Sprintf("quest «%s», %d step(s)", title, len(resolved.Steps))
	if note != "" {
		msg += "; " + note
	}
	if len(resolved.Ignored) > 0 {
		msg += "; ignored fields: " + strings.Join(resolved.Ignored, ", ")
	}
	rec.Message = joinNote(msg, rec.Message)
	record()
	id, err := persistEmitPoolOutcome(ctx, st, id, isNew, tmpl.ID, periodKey, "scheduled", attempts, "", now)
	if err != nil {
		return emitResult{}, err
	}
	return emitResult{Spec: resolved, RollID: id}, nil
}

// persistEmitPoolOutcome returns the row's id — for a new row this is the
// freshly inserted id, which callers need (e.g. to later mark it
// materialized); returning it here means they don't have to duplicate the
// isNew branching themselves.
func persistEmitPoolOutcome(ctx context.Context, st *store.Store, id int64, isNew bool, templateID int64, periodKey, outcome string, attempts int, pickedRefsJSON string, now time.Time) (int64, error) {
	if isNew {
		res, err := st.DB.ExecContext(ctx, `
			INSERT INTO templateemitroll (template_id, period_key, outcome, attempts, picked_refs, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			templateID, periodKey, outcome, attempts, nullStrIfEmpty(pickedRefsJSON), timeutil.ToDBUTC(now), timeutil.ToDBUTC(now))
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	_, err := st.DB.ExecContext(ctx, `
		UPDATE templateemitroll SET outcome = ?, picked_refs = ?, retry_at = NULL, updated_at = ? WHERE id = ?`,
		outcome, nullStrIfEmpty(pickedRefsJSON), timeutil.ToDBUTC(now), id)
	return id, err
}

func nullStrIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// execInfo is what one emit_pool_command run leaves behind for the attempt
// log. Status is "ok" or, for a failed run, "error" | "timeout" | "bad_json".
// Stderr/Message never contain template secret values.
type execInfo struct {
	Status   string
	Duration time.Duration
	Stderr   string
}

// joinNote prefixes the attempt's own verdict to whatever stderr it produced.
func joinNote(note, stderr string) string {
	if stderr == "" {
		return note
	}
	return note + "\nstderr:\n" + stderr
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ErrTemplateNotFound is returned by MaterializeTemplateManual when id is missing.
var ErrTemplateNotFound = errors.New("template not found")

func loadTemplateRow(ctx context.Context, st *store.Store, id int64) (templateRow, error) {
	var t templateRow
	var pinned, enabled, automated int
	err := st.DB.QueryRowContext(ctx, `
		SELECT id, title, description, pinned, sort_order, duration_seconds, freq, weekdays,
			enabled, timezone, deadline_time, significance, emit_mode, emit_chance,
			emit_window_start, emit_window_end, emit_pool_command, emit_limits, emit_run_on,
			category_id, questline_id, automated
		FROM questtemplate WHERE id = ?`, id).Scan(
		&t.ID, &t.Title, &t.Description, &pinned, &t.SortOrder, &t.DurationSeconds, &t.Freq, &t.Weekdays,
		&enabled, &t.Timezone, &t.DeadlineTime, &t.Significance, &t.EmitMode, &t.EmitChance,
		&t.EmitWindowStart, &t.EmitWindowEnd, &t.EmitPoolCommand, &t.EmitLimits, &t.EmitRunOn,
		&t.CategoryID, &t.QuestlineID, &automated,
	)
	if err == sql.ErrNoRows {
		return templateRow{}, ErrTemplateNotFound
	}
	if err != nil {
		return templateRow{}, err
	}
	t.Pinned, t.OverlayHidden = journal.SplitPinState(pinned)
	t.Enabled = enabled != 0
	t.Automated = automated != 0
	return t, nil
}

// MaterializeTemplateManual creates one quest from a template immediately (GUI
// testing). It skips weekday schedule, the daily period_key slot, surprise
// chance/window rolls, and emit-pool time gates. Each call uses period_key
// manual-<unix> so repeated emits never collide. When emit_pool_command is set
// it still runs once for that key; an empty pool or command error falls back to
// template steps instead of inserting a failed quest like the scheduler does.
func MaterializeTemplateManual(ctx context.Context, st *store.Store, hub *events.Hub, templateID int64, now time.Time, rng *rand.Rand) (int64, error) {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if now.IsZero() {
		now = timeutil.NowUTC()
	}
	tmpl, err := loadTemplateRow(ctx, st, templateID)
	if err != nil {
		return 0, err
	}
	tzName := tmpl.Timezone
	if tzName == "" {
		tzName = defaultTZ()
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc, _ = time.LoadLocation(defaultTZ())
		if loc == nil {
			loc = time.UTC
		}
	}
	localNow := now.In(loc)
	key := fmt.Sprintf("manual-%d", now.Unix())

	emitMode := strings.ToLower(strings.TrimSpace(tmpl.EmitMode))
	var deadline *time.Time
	var duration *int
	if emitMode == "surprise" || emitMode == "random" || emitMode == "chance" {
		deadline, duration = surpriseDeadline(tmpl, now)
	} else {
		deadline, duration = fixedDeadline(tmpl, localNow, loc)
	}

	usePool := tmpl.EmitPoolCommand.Valid && strings.TrimSpace(tmpl.EmitPoolCommand.String) != ""
	var poolRollID int64
	var spec *resolvedSpec
	if usePool {
		res, perr := resolveEmit(ctx, st, tmpl, key, now, poolOpts{ignorePause: true})
		if perr != nil {
			return 0, perr
		}
		poolRollID = res.RollID
		spec = res.Spec
	}
	var steps []journal.Step
	if spec != nil && len(spec.Steps) > 0 {
		steps = spec.Steps
	} else {
		steps, err = loadTemplateSteps(ctx, st, tmpl, rng)
		if err != nil {
			return 0, err
		}
	}

	q := journal.Quest{
		Title:           tmpl.Title,
		Description:     tmpl.Description,
		Status:          journal.StatusActive,
		Significance:    journal.Significance(tmpl.Significance),
		Pinned:          tmpl.Pinned,
		SortOrder:       tmpl.SortOrder,
		DeadlineAt:      deadline,
		DurationSeconds: duration,
		CreatedAt:       now,
		UpdatedAt:       now,
		Automated:       tmpl.Automated,
		Steps:           steps,
	}
	if tmpl.Significance == "" {
		q.Significance = journal.SigCommon
	}
	if tmpl.CategoryID.Valid {
		v := tmpl.CategoryID.Int64
		q.CategoryID = &v
	}
	if tmpl.QuestlineID.Valid {
		v := tmpl.QuestlineID.Int64
		q.QuestlineID = &v
	}
	if spec != nil {
		applyResolvedSpec(ctx, st, &q, tmpl, spec, now)
	}
	followQuestlineCategory(ctx, st, &q)
	tid := tmpl.ID
	q.TemplateID = &tid
	pk := key
	q.PeriodKey = &pk
	source := fmt.Sprintf("template:%d", tmpl.ID)
	q.Source = &source

	createdQ, err := st.Journal().CreateQuestAppeared(ctx, q)
	if err != nil {
		return 0, err
	}
	if err := attachTags(ctx, st, tmpl, createdQ.ID, spec); err != nil {
		return 0, err
	}
	if usePool && poolRollID != 0 {
		_, _ = st.DB.ExecContext(ctx, `
			UPDATE templateemitroll SET outcome = 'materialized', updated_at = ? WHERE id = ?`,
			timeutil.ToDBUTC(now), poolRollID)
	}
	qid := createdQ.ID
	if hub != nil {
		hub.Publish("quest_appeared", events.PublishOpts{
			Workspace:    wsctx.From(ctx),
			QuestID:      &qid,
			Title:        createdQ.Title,
			Description:  createdQ.Description,
			Detail:       "Ручной эмит (" + key + ")",
			Toast:        true,
			Source:       "system",
			Significance: string(createdQ.Significance),
			Automated:    createdQ.Automated,
			Sound:        strPtr("quest_created"),
		})
	}
	return qid, nil
}
