package journal

import (
	"strconv"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
	"github.com/quests-inc/quests/go/internal/refs"
)

// QuestRead is the wire JSON shape matching Python serializers.quest_to_read.
type QuestRead struct {
	ID                 int64        `json:"id"`
	Title              string       `json:"title"`
	Description        string       `json:"description"`
	Status             QuestStatus  `json:"status"`
	Significance       Significance `json:"significance"`
	Pinned             bool         `json:"pinned"`
	OverlayHidden      bool         `json:"overlay_hidden"`
	SortOrder          int          `json:"sort_order"`
	DeadlineAt         *string      `json:"deadline_at"`
	DurationSeconds    *int         `json:"duration_seconds"`
	ExpireOnDeadline   bool         `json:"expire_on_deadline"`
	EventID            *int64       `json:"event_id"`
	EventEdge          string       `json:"event_edge"`
	EventDir           string       `json:"event_dir"`
	EventOffsetSeconds *int         `json:"event_offset_seconds"`
	EventTitle         *string      `json:"event_title"`
	CategoryID         *int64       `json:"category_id"`
	CategorySlug       *string      `json:"category_slug"`
	CategoryLabel      *string      `json:"category_label"`
	CategoryColor      *string      `json:"category_color"`
	QuestlineID        *int64       `json:"questline_id"`
	QuestlineTitle     *string      `json:"questline_title"`
	QuestlineColor     *string      `json:"questline_color"`
	QuestlineIcon      *string      `json:"questline_icon"`
	QuestlineIconURL   *string      `json:"questline_icon_url"`
	CreatedAt          string       `json:"created_at"`
	UpdatedAt          string       `json:"updated_at"`
	CompletedAt        *string      `json:"completed_at"`
	TemplateID         *int64       `json:"template_id"`
	PeriodKey          *string      `json:"period_key"`
	Source             *string      `json:"source"`
	Automated          bool         `json:"automated"`
	FailureInbox       bool         `json:"failure_inbox"`
	Steps              []StepRead   `json:"steps"`
	StepsDone          int          `json:"steps_done"`
	StepsTotal         int          `json:"steps_total"`
	ProgressLabel      string       `json:"progress_label"`
	RemainingSeconds   *int         `json:"remaining_seconds"`
	TimerTone          *string      `json:"timer_tone"`
	Urgent             bool         `json:"urgent"`
	Tags               []Tag        `json:"tags"`
	// Refs is populated only for a single-quest fetch (see Server.getQuest) —
	// resolving note=N/attachment=N/... mentions for every row in a list
	// response would be an N+1 query fan-out nobody needs there.
	Refs []refs.Resolved `json:"refs,omitempty"`
}

type StepRead struct {
	ID                   int64   `json:"id"`
	QuestID              int64   `json:"quest_id"`
	Title                string  `json:"title"`
	Description          string  `json:"description"`
	ProgressCurrent      int     `json:"progress_current"`
	ProgressTotal        int     `json:"progress_total"`
	SortOrder            int     `json:"sort_order"`
	CheckCommand         *string `json:"check_command"`
	CheckIntervalSeconds *int    `json:"check_interval_seconds"`
	CheckLastRunAt       *string `json:"check_last_run_at"`
	WaitPrevious         bool    `json:"wait_previous"`
	RunMode              string  `json:"run_mode"`
	RunStatus            *string `json:"run_status"`
	RunOn                *string `json:"run_on"`
	Done                 bool    `json:"done"`
	ScriptID             *int64  `json:"script_id"`
}

func ToQuestRead(q Quest, now time.Time) QuestRead {
	done, total, label := ProgressLabel(q.Steps)
	state := timeutil.DeadlineAt(q.DeadlineAt, q.DurationSeconds, ExpiresOnDeadline(q), now)
	rem, tone, urgent := state.Remaining, state.Tone, state.Urgent
	if q.Status == StatusExpired {
		t := "overdue"
		rem = nil
		tone = &t
		urgent = true
	}

	var iconURL *string
	if q.CustomIcon != nil && *q.CustomIcon != "" && q.QuestlineID != nil {
		u := "/api/questlines/" + strconv.FormatInt(*q.QuestlineID, 10) + "/icon"
		if q.QuestlineUpdatedAt != nil {
			if iso := toISO(q.QuestlineUpdatedAt); iso != nil {
				u += "?v=" + *iso
			}
		}
		iconURL = &u
	}

	out := QuestRead{
		ID:                 q.ID,
		Title:              q.Title,
		Description:        q.Description,
		Status:             q.Status,
		Significance:       q.Significance,
		Pinned:             q.Pinned,
		OverlayHidden:      q.OverlayHidden,
		SortOrder:          q.SortOrder,
		DeadlineAt:         toISO(q.DeadlineAt),
		DurationSeconds:    q.DurationSeconds,
		ExpireOnDeadline:   ExpiresOnDeadline(q),
		EventID:            q.EventID,
		EventEdge:          q.EventEdge,
		EventDir:           q.EventDir,
		EventOffsetSeconds: q.EventOffset,
		EventTitle:         q.EventTitle,
		CategoryID:         q.CategoryID,
		CategorySlug:       q.CategorySlug,
		CategoryLabel:      q.CategoryLabel,
		CategoryColor:      q.CategoryColor,
		QuestlineID:        q.QuestlineID,
		QuestlineTitle:     q.QuestlineTitle,
		QuestlineColor:     q.QuestlineColor,
		QuestlineIcon:      q.QuestlineIcon,
		QuestlineIconURL:   iconURL,
		CreatedAt:          *toISO(&q.CreatedAt),
		UpdatedAt:          *toISO(&q.UpdatedAt),
		CompletedAt:        toISO(q.CompletedAt),
		TemplateID:         q.TemplateID,
		PeriodKey:          q.PeriodKey,
		Source:             q.Source,
		Automated:          q.Automated,
		FailureInbox:       q.FailureInbox,
		StepsDone:          done,
		StepsTotal:         total,
		ProgressLabel:      label,
		RemainingSeconds:   rem,
		TimerTone:          tone,
		Urgent:             urgent,
		Tags:               q.Tags,
	}
	if out.Tags == nil {
		out.Tags = []Tag{}
	}
	out.Steps = make([]StepRead, 0, len(q.Steps))
	for _, s := range q.Steps {
		mode := s.RunMode
		if mode == "" {
			mode = RunModePoll
		}
		out.Steps = append(out.Steps, StepRead{
			ID:                   s.ID,
			QuestID:              s.QuestID,
			Title:                s.Title,
			Description:          s.Description,
			ProgressCurrent:      s.ProgressCurrent,
			ProgressTotal:        s.ProgressTotal,
			SortOrder:            s.SortOrder,
			CheckCommand:         s.CheckCommand,
			CheckIntervalSeconds: s.CheckIntervalSeconds,
			CheckLastRunAt:       toISO(s.CheckLastRunAt),
			WaitPrevious:         s.WaitPrevious,
			RunMode:              mode,
			RunStatus:            s.RunStatus,
			RunOn:                s.RunOn,
			Done:                 s.Done,
			ScriptID:             s.ScriptID,
		})
	}
	return out
}

// Hooks filled from timeutil in httpapi to avoid import cycles — local copies:
func toISO(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	tt := t.UTC()
	s := tt.Format("2006-01-02T15:04:05.000000Z")
	return &s
}
