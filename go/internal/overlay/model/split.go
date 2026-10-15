package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/overlay/api"
)

// StepView is an open step line in the HUD.
type StepView struct {
	Title    string
	Progress string
}

// QuestView is one HUD row (favorite / urgent / category).
type QuestView struct {
	QuestID         int64
	Title           string
	Steps           []StepView
	TimerLabel      string
	TimerTone       string
	DeadlineAt      *string
	DurationSeconds *int
	Overdue         bool
}

// SplitQuests mirrors Python split_hud_quests: pinned → urgent → category.
func SplitQuests(items []api.Quest, categorySlug string, now time.Time) (favorites, urgent, category []QuestView) {
	taken := map[int64]struct{}{}
	slug := strings.ToLower(strings.TrimSpace(categorySlug))

	visible := make([]api.Quest, 0, len(items))
	for _, q := range items {
		if q.OverlayHidden {
			continue
		}
		visible = append(visible, q)
	}

	for _, q := range visible {
		if closedStatus(q.Status) {
			continue
		}
		steps := openSteps(q)
		if len(steps) == 0 {
			continue
		}
		if q.Pinned {
			entry := withTimer(q, steps, now)
			favorites = append(favorites, entry)
			taken[q.ID] = struct{}{}
		}
	}

	for _, q := range visible {
		if closedStatus(q.Status) {
			continue
		}
		if _, ok := taken[q.ID]; ok {
			continue
		}
		steps := openSteps(q)
		if len(steps) == 0 {
			continue
		}
		flag := q.Urgent
		if !flag {
			flag = IsUrgent(q.DeadlineAt, q.DurationSeconds, now)
		}
		if q.Status == "expired" {
			flag = true
		} else if rem, ok := RemainingSeconds(q.DeadlineAt, now); ok && rem <= 0 {
			flag = true
		}
		if !flag {
			continue
		}
		entry := withTimer(q, steps, now)
		urgent = append(urgent, entry)
		taken[q.ID] = struct{}{}
	}

	if slug != "" {
		for _, q := range visible {
			if q.Status != "active" && q.Status != "expired" {
				continue
			}
			if _, ok := taken[q.ID]; ok {
				continue
			}
			if strings.ToLower(strings.TrimSpace(q.CategorySlug)) != slug {
				continue
			}
			steps := openSteps(q)
			if len(steps) == 0 {
				continue
			}
			category = append(category, withTimer(q, steps, now))
		}
	}
	return favorites, urgent, category
}

// ResolveCategory picks slug/label from preferred or first available.
func ResolveCategory(cats []api.Category, preferred string) (slug, label string) {
	if len(cats) == 0 {
		return "", ""
	}
	want := strings.ToLower(strings.TrimSpace(preferred))
	for _, c := range cats {
		s := strings.TrimSpace(c.Slug)
		if s == "" {
			continue
		}
		l := strings.TrimSpace(c.Label)
		if l == "" {
			l = s
		}
		if want != "" && strings.ToLower(s) == want {
			return s, l
		}
	}
	for _, c := range cats {
		s := strings.TrimSpace(c.Slug)
		if s == "" {
			continue
		}
		l := strings.TrimSpace(c.Label)
		if l == "" {
			l = s
		}
		return s, l
	}
	return "", ""
}

func closedStatus(status string) bool {
	switch status {
	case "completed", "failed", "archived", "frozen":
		return true
	default:
		return false
	}
}

func openSteps(q api.Quest) []StepView {
	out := make([]StepView, 0, len(q.Steps))
	for _, s := range q.Steps {
		total := s.ProgressTotal
		if total < 1 {
			total = 1
		}
		if s.Done || s.ProgressCurrent >= total {
			continue
		}
		title := s.Title
		if title == "" {
			title = "?"
		}
		out = append(out, StepView{
			Title:    title,
			Progress: fmt.Sprintf("%d / %d", s.ProgressCurrent, total),
		})
	}
	return out
}

func withTimer(q api.Quest, steps []StepView, now time.Time) QuestView {
	view := QuestView{
		QuestID:         q.ID,
		Title:           q.Title,
		Steps:           steps,
		DeadlineAt:      q.DeadlineAt,
		DurationSeconds: q.DurationSeconds,
	}
	if view.Title == "" {
		view.Title = "?"
	}
	overdue := q.Status == "expired"
	if rem, ok := RemainingSeconds(q.DeadlineAt, now); ok {
		if rem <= 0 {
			overdue = true
		} else {
			view.TimerTone = TimerTone(q.DeadlineAt, q.DurationSeconds, now)
			view.TimerLabel = FormatRemaining(rem)
			view.Overdue = false
			return view
		}
	}
	if overdue {
		view.Overdue = true
		view.TimerTone = "overdue"
		view.TimerLabel = "просрочено"
	}
	return view
}
