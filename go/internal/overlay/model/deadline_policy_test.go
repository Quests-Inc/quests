package model

import (
	"github.com/quests-inc/quests/go/internal/overlay/api"
	"testing"
	"time"
)

func TestHUDDeadlinePoliciesAndNeutralWindows(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour).Format(time.RFC3339)
	future := now.Add(30 * time.Minute).Format(time.RFC3339)
	yes, no := true, false
	window, zero := 3600, 0
	soft := api.Quest{ID: 1, Title: "Ориентир", Status: "active", DeadlineAt: &past, DurationSeconds: &window, ExpireOnDeadline: &no, CategorySlug: "work", Steps: []api.Step{{Title: "Дело", ProgressTotal: 1}}}
	// A stale API urgency snapshot must not keep an informational target urgent forever.
	soft.Urgent = true
	_, urgent, category := SplitQuests([]api.Quest{soft}, "work", now)
	if len(urgent) != 0 || len(category) != 1 || category[0].TimerLabel != "срок наступил" || category[0].TimerTone != "" {
		t.Fatalf("soft target: %v %v", urgent, category)
	}
	hard := soft
	hard.ID = 2
	hard.Title = "Срок"
	hard.ExpireOnDeadline = &yes
	hard.DurationSeconds = &zero
	_, urgent, _ = SplitQuests([]api.Quest{hard}, "", now)
	if len(urgent) != 1 || urgent[0].TimerTone != "overdue" || !urgent[0].Overdue {
		t.Fatalf("expiry without window: %v", urgent)
	}
	neutral := soft
	neutral.ID = 3
	neutral.DeadlineAt = &future
	neutral.DurationSeconds = &zero
	neutral.Urgent = false
	neutral.Pinned = true
	pinned, urgent, _ := SplitQuests([]api.Quest{neutral}, "", now)
	if len(pinned) != 1 || pinned[0].TimerLabel != "30:00" || pinned[0].TimerTone != "" || len(urgent) != 0 {
		t.Fatalf("neutral countdown: %v %v", pinned, urgent)
	}
	blank := "invalid"
	neutral.DeadlineAt = &blank
	pinned, _, _ = SplitQuests([]api.Quest{neutral}, "", now)
	if pinned[0].TimerLabel != "" {
		t.Fatal("invalid date made a timer")
	}
	if _, ok := RemainingSeconds(&blank, now); ok {
		t.Fatal("invalid date parsed")
	}
	empty := ""
	if _, ok := RemainingSeconds(&empty, now); ok {
		t.Fatal("empty date parsed")
	}
}
