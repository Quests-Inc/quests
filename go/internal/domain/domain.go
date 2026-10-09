package domain

import (
	"fmt"
	"strings"
	"time"
)

type QuestStatus string

const (
	StatusActive    QuestStatus = "active"
	StatusExpired   QuestStatus = "expired"
	StatusFrozen    QuestStatus = "frozen" // manual "заморожено" — parked by the user, not the auto-overdue StatusExpired
	StatusCompleted QuestStatus = "completed"
	StatusFailed    QuestStatus = "failed"
	StatusArchived  QuestStatus = "archived"
)

type Significance string

const (
	SigInsignificant Significance = "insignificant"
	SigCommon        Significance = "common"
	SigUncommon      Significance = "uncommon"
	SigEpic          Significance = "epic"
	SigLegendary     Significance = "legendary"
)

// SignificanceValues lists the valid levels, lowest first.
var SignificanceValues = []Significance{SigInsignificant, SigCommon, SigUncommon, SigEpic, SigLegendary}

// Valid reports whether s is one of the known significance levels.
func (s Significance) Valid() bool {
	for _, v := range SignificanceValues {
		if s == v {
			return true
		}
	}
	return false
}

// StatusValues lists the valid quest statuses.
var StatusValues = []QuestStatus{StatusActive, StatusExpired, StatusFrozen, StatusCompleted, StatusFailed, StatusArchived}

// Valid reports whether s is one of the known quest statuses.
func (s QuestStatus) Valid() bool {
	for _, v := range StatusValues {
		if s == v {
			return true
		}
	}
	return false
}

// InvalidSignificanceMsg is the 422 detail for an unknown significance. It
// lists the valid values so an API/MCP caller can fix the request in one retry.
func InvalidSignificanceMsg(got string) string {
	return fmt.Sprintf("invalid significance %q; expected one of: %s", got, joinValues(SignificanceValues))
}

// InvalidStatusMsg is the 422 detail for an unknown quest status.
func InvalidStatusMsg(got string) string {
	msg := fmt.Sprintf("invalid status %q; expected one of: %s", got, joinValues(StatusValues))
	if got == "delayed" {
		msg += ` ("delayed" was renamed to "frozen")`
	}
	return msg
}

func joinValues[T ~string](vals []T) string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = string(v)
	}
	return strings.Join(out, ", ")
}

type Category struct {
	ID        int64  `json:"id"`
	Slug      string `json:"slug"`
	Label     string `json:"label"`
	SortOrder int    `json:"sort_order"`
	Color     string `json:"color"`
}

// Tag is a cross-cutting work-surface label (frontend, api, mcp…).
type Tag struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Label string `json:"label"`
	Color string `json:"color"`
}

const (
	RunModePoll = "poll"
	RunModeOnce = "once"
	// RunModeWatch is poll that keeps going after the step reached its maximum
	// (a step that follows another quest and may be re-opened when it grows).
	RunModeWatch = "watch"

	RunIdle    = "idle"
	RunRunning = "running"
	RunSuccess = "success"
	RunFail    = "fail"
)

type Step struct {
	ID                   int64      `json:"id"`
	QuestID              int64      `json:"quest_id"`
	Title                string     `json:"title"`
	Description          string     `json:"description"`
	ProgressCurrent      int        `json:"progress_current"`
	ProgressTotal        int        `json:"progress_total"`
	SortOrder            int        `json:"sort_order"`
	CheckCommand         *string    `json:"check_command"`
	CheckIntervalSeconds *int       `json:"check_interval_seconds"`
	CheckLastRunAt       *time.Time `json:"-"`
	WaitPrevious         bool       `json:"wait_previous"`
	RunMode              string     `json:"run_mode"`
	RunStatus            *string    `json:"run_status"`
	// RunOn names the runner (device) that executes the check; nil = this server.
	RunOn *string `json:"run_on"`
	Done  bool    `json:"done"`
}

type Quest struct {
	ID           int64
	Title        string
	Description  string
	Status       QuestStatus
	Significance Significance
	Pinned       bool
	// OverlayHidden: never shown in the HUD. Stored in the same column as Pinned
	// (1 always, 0 default, -1 never), so the two are mutually exclusive.
	OverlayHidden   bool
	SortOrder       int
	DeadlineAt      *time.Time
	DurationSeconds *int
	CategoryID      *int64
	QuestlineID     *int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
	TemplateID      *int64
	PeriodKey       *string
	Source          *string // 'template:<id>' or the creating client; nil = unknown
	Automated       bool

	CategorySlug       *string
	CategoryLabel      *string
	CategoryColor      *string
	QuestlineTitle     *string
	QuestlineColor     *string
	QuestlineIcon      *string
	CustomIcon         *string
	QuestlineUpdatedAt *time.Time

	Steps []Step
	Tags  []Tag
}

type StepCreate struct {
	Title                string  `json:"title"`
	Description          string  `json:"description"`
	ProgressCurrent      int     `json:"progress_current"`
	ProgressTotal        int     `json:"progress_total"`
	SortOrder            *int    `json:"sort_order"`
	CheckCommand         *string `json:"check_command"`
	CheckIntervalSeconds *int    `json:"check_interval_seconds"`
	WaitPrevious         *bool   `json:"wait_previous"`
	RunMode              *string `json:"run_mode"`
	RunOn                *string `json:"run_on"`
}

type StepUpdate struct {
	Title                *string `json:"title"`
	Description          *string `json:"description"`
	ProgressCurrent      *int    `json:"progress_current"`
	ProgressTotal        *int    `json:"progress_total"`
	SortOrder            *int    `json:"sort_order"`
	CheckCommand         *string `json:"check_command"`
	CheckIntervalSeconds *int    `json:"check_interval_seconds"`
	WaitPrevious         *bool   `json:"wait_previous"`
	RunMode              *string `json:"run_mode"`
	RunOn                *string `json:"run_on"`
}

type QuestCreate struct {
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	Status          QuestStatus  `json:"status"`
	Significance    Significance `json:"significance"`
	Pinned          bool         `json:"pinned"`
	OverlayHidden   bool         `json:"overlay_hidden"`
	SortOrder       int          `json:"sort_order"`
	DeadlineAt      *string      `json:"deadline_at"`
	DurationSeconds *int         `json:"duration_seconds"`
	CategoryID      *int64       `json:"category_id"`
	QuestlineID     *int64       `json:"questline_id"`
	Automated       bool         `json:"automated"`
	Steps           []StepCreate `json:"steps"`
	TagIDs          []int64      `json:"tag_ids"`
}

func ClampStep(s *Step) {
	if s.ProgressTotal < 1 {
		s.ProgressTotal = 1
	}
	if s.ProgressCurrent < 0 {
		s.ProgressCurrent = 0
	}
	if s.ProgressCurrent > s.ProgressTotal {
		s.ProgressCurrent = s.ProgressTotal
	}
	s.Done = s.ProgressCurrent >= s.ProgressTotal
}

func SyncStatusFromSteps(q *Quest, now time.Time) {
	if len(q.Steps) == 0 {
		return
	}
	allDone := true
	for _, s := range q.Steps {
		if s.ProgressCurrent < s.ProgressTotal {
			allDone = false
			break
		}
	}
	if allDone && (q.Status == StatusActive || q.Status == StatusExpired || q.Status == StatusFrozen || q.Status == StatusFailed) {
		q.Status = StatusCompleted
	} else if !allDone && q.Status == StatusCompleted {
		q.Status = StatusActive
	}
	if q.Status == StatusCompleted && q.CompletedAt == nil {
		t := now
		q.CompletedAt = &t
	}
	if q.Status != StatusCompleted {
		q.CompletedAt = nil
	}
}

func ProgressLabel(steps []Step) (done, total int, label string) {
	if len(steps) == 0 {
		return 0, 0, "0 / 0"
	}
	if len(steps) == 1 {
		s := steps[0]
		d := 0
		if s.ProgressCurrent >= s.ProgressTotal {
			d = 1
		}
		return d, 1, itoa(s.ProgressCurrent) + " / " + itoa(s.ProgressTotal)
	}
	for _, s := range steps {
		total++
		if s.ProgressCurrent >= s.ProgressTotal {
			done++
		}
	}
	return done, total, itoa(done) + " / " + itoa(total)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// PinState is the value of the pinned column: 1 always in the HUD, -1 never, 0 default.
func PinState(pinned, overlayHidden bool) int {
	switch {
	case pinned:
		return 1
	case overlayHidden:
		return -1
	}
	return 0
}

// SplitPinState is PinState's inverse.
func SplitPinState(v int) (pinned, overlayHidden bool) { return v > 0, v < 0 }
