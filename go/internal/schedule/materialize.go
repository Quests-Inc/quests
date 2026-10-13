package schedule

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/automation"
	"github.com/quests-inc/quests/go/internal/journal"
	"github.com/quests-inc/quests/go/internal/platform/events"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/store"
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

type templateRow = automation.TemplateRow

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
	templates, err := st.Automation.ListEnabledTemplateRows(ctx)
	if err != nil {
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

		if _, exists, err := st.Journal.QuestIDForPeriod(ctx, tmpl.ID, key); err != nil {
			return created, err
		} else if exists {
			continue
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
		createdQ, err := st.Journal.CreateQuestAppeared(ctx, q)
		if err != nil {
			return created, err
		}
		if err := attachTags(ctx, st, tmpl, createdQ.ID, spec); err != nil {
			return created, err
		}
		if surpriseRollID.Valid {
			st.Automation.MarkEmitRollMaterialized(ctx, surpriseRollID.Int64, now)
		}
		if usePool && poolRollID != 0 {
			st.Automation.MarkEmitRollMaterialized(ctx, poolRollID, now)
		}
		qid := createdQ.ID
		detail := "Период " + key
		if poolFailed {
			// Already-failed emit must surface like any other failure — quiet
			// quest_appeared would skip overlay/Telegram entirely (toast=false).
			hub.Publish("quest_failed", events.PublishOpts{
				Workspace:    wsctx.From(ctx),
				QuestID:      &qid,
				Title:        createdQ.Title,
				Description:  createdQ.Description,
				Detail:       "провалено",
				Toast:        true,
				Source:       "system",
				Significance: string(createdQ.Significance),
				Automated:    createdQ.Automated,
			})
		} else {
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
		}
		created = append(created, qid)
	}
	return created, nil
}

func strPtr(s string) *string { return &s }

func loadTemplateSteps(ctx context.Context, st *store.Store, tmpl templateRow, rng *rand.Rand) ([]journal.Step, error) {
	src, err := st.Automation.TemplateStepRows(ctx, tmpl.ID)
	if err != nil {
		return nil, err
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

// ErrTemplateNotFound is returned by MaterializeTemplateManual when id is missing.
var ErrTemplateNotFound = errors.New("template not found")

func loadTemplateRow(ctx context.Context, st *store.Store, id int64) (templateRow, error) {
	t, err := st.Automation.TemplateRowByID(ctx, id)
	if errors.Is(err, automation.ErrNotFound) {
		return templateRow{}, ErrTemplateNotFound
	}
	return t, err
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

	createdQ, err := st.Journal.CreateQuestAppeared(ctx, q)
	if err != nil {
		return 0, err
	}
	if err := attachTags(ctx, st, tmpl, createdQ.ID, spec); err != nil {
		return 0, err
	}
	if usePool && poolRollID != 0 {
		st.Automation.MarkEmitRollMaterialized(ctx, poolRollID, now)
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
