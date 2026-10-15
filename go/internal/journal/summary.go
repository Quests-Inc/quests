package journal

import (
	"encoding/base64"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Page-size bounds for the compact quest list (GET /api/quests?summary=1).
const (
	SummaryDefaultLimit = 20
	SummaryMaxLimit     = 100
)

// ErrBadCursor is returned for a cursor that was not produced by SummarizeQuests.
var ErrBadCursor = errors.New("invalid cursor")

// NextStepRead is the first unfinished step of a quest — enough to act on it by id.
type NextStepRead struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

// QuestSummary is a quest without its description, steps or attachments: the shape
// for listings that must stay small however many quests there are.
type QuestSummary struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Status         QuestStatus   `json:"status"`
	Significance   Significance  `json:"significance"`
	Pinned         bool          `json:"pinned"`
	OverlayHidden  bool          `json:"overlay_hidden"`
	FailureInbox   bool          `json:"failure_inbox"`
	Automated      bool          `json:"automated"`
	DeadlineAt     *string       `json:"deadline_at"`
	CategoryID     *int64        `json:"category_id"`
	CategorySlug   *string       `json:"category_slug"`
	QuestlineID    *int64        `json:"questline_id"`
	QuestlineTitle *string       `json:"questline_title"`
	UpdatedAt      string        `json:"updated_at"`
	StepsDone      int           `json:"steps_done"`
	StepsTotal     int           `json:"steps_total"`
	ProgressLabel  string        `json:"progress_label"`
	NextStep       *NextStepRead `json:"next_step"`
	DescriptionLen int           `json:"description_len"`
}

// QuestSummaryPage is one page of summaries. Next is empty on the last page.
type QuestSummaryPage struct {
	Items     []QuestSummary `json:"items"`
	Total     int            `json:"total"`
	Truncated bool           `json:"truncated"`
	Next      string         `json:"next,omitempty"`
}

// SummaryQuery selects a page. Statuses empty = every status.
type SummaryQuery struct {
	Statuses []QuestStatus
	Limit    int
	Cursor   string
}

// ParseStatuses splits a comma-separated status list ("active,expired"), rejecting unknown values.
func ParseStatuses(raw string) ([]QuestStatus, error) {
	var out []QuestStatus
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		s := QuestStatus(p)
		if !s.Valid() {
			return nil, errors.New(InvalidStatusMsg(p))
		}
		out = append(out, s)
	}
	return out, nil
}

// SummarizeQuests filters, orders (newest update first, id as tiebreak) and pages quests.
// The cursor is the (updated_at, id) of the last item of the previous page, so a quest edited
// between two calls can move but never makes the walk repeat or skip an untouched one.
func SummarizeQuests(quests []Quest, q SummaryQuery) (QuestSummaryPage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = SummaryDefaultLimit
	}
	if limit > SummaryMaxLimit {
		limit = SummaryMaxLimit
	}
	var curAt int64
	var curID int64
	hasCursor := q.Cursor != ""
	if hasCursor {
		var err error
		if curAt, curID, err = decodeCursor(q.Cursor); err != nil {
			return QuestSummaryPage{}, err
		}
	}

	want := map[QuestStatus]bool{}
	for _, s := range q.Statuses {
		want[s] = true
	}
	rows := make([]Quest, 0, len(quests))
	for _, qu := range quests {
		if len(want) > 0 && !want[qu.Status] {
			continue
		}
		rows = append(rows, qu)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ai, aj := rows[i].UpdatedAt.UnixMicro(), rows[j].UpdatedAt.UnixMicro()
		if ai != aj {
			return ai > aj
		}
		return rows[i].ID > rows[j].ID
	})

	page := QuestSummaryPage{Total: len(rows), Items: []QuestSummary{}}
	var last Quest
	for _, qu := range rows {
		if hasCursor {
			at := qu.UpdatedAt.UnixMicro()
			if at > curAt || (at == curAt && qu.ID >= curID) {
				continue
			}
		}
		if len(page.Items) == limit {
			page.Truncated = true
			page.Next = encodeCursor(last.UpdatedAt.UnixMicro(), last.ID)
			break
		}
		page.Items = append(page.Items, toSummary(qu))
		last = qu
	}
	return page, nil
}

func toSummary(q Quest) QuestSummary {
	done, total, label := ProgressLabel(q.Steps)
	s := QuestSummary{
		ID:             q.ID,
		Title:          q.Title,
		Status:         q.Status,
		Significance:   q.Significance,
		Pinned:         q.Pinned,
		OverlayHidden:  q.OverlayHidden,
		FailureInbox:   q.FailureInbox,
		Automated:      q.Automated,
		DeadlineAt:     toISO(q.DeadlineAt),
		CategoryID:     q.CategoryID,
		CategorySlug:   q.CategorySlug,
		QuestlineID:    q.QuestlineID,
		QuestlineTitle: q.QuestlineTitle,
		UpdatedAt:      *toISO(&q.UpdatedAt),
		StepsDone:      done,
		StepsTotal:     total,
		ProgressLabel:  label,
		DescriptionLen: utf8.RuneCountInString(q.Description),
	}
	for _, st := range q.Steps {
		if st.ProgressCurrent < st.ProgressTotal {
			s.NextStep = &NextStepRead{ID: st.ID, Title: st.Title}
			break
		}
	}
	return s
}

func encodeCursor(updatedMicro, id int64) string {
	raw := strconv.FormatInt(updatedMicro, 10) + "." + strconv.FormatInt(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(c string) (updatedMicro, id int64, err error) {
	b, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return 0, 0, ErrBadCursor
	}
	a, bb, ok := strings.Cut(string(b), ".")
	if !ok {
		return 0, 0, ErrBadCursor
	}
	if updatedMicro, err = strconv.ParseInt(a, 10, 64); err != nil {
		return 0, 0, ErrBadCursor
	}
	if id, err = strconv.ParseInt(bb, 10, 64); err != nil {
		return 0, 0, ErrBadCursor
	}
	return updatedMicro, id, nil
}
