package model

import (
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/overlay/api"
)

func TestSplitPinnedAndUrgent(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 5, 0, 0, time.UTC)
	deadline := "2026-10-14T12:10:00Z"
	dur := 600
	items := []api.Quest{
		{
			ID: 1, Title: "Pinned", Status: "active", Pinned: true,
			Steps: []api.Step{{Title: "a", ProgressCurrent: 0, ProgressTotal: 1}},
		},
		{
			ID: 2, Title: "Urgent", Status: "active",
			DeadlineAt: &deadline, DurationSeconds: &dur,
			Steps: []api.Step{{Title: "b", ProgressCurrent: 0, ProgressTotal: 1}},
		},
		{
			ID: 3, Title: "Hidden", Status: "active", OverlayHidden: true, Pinned: true,
			Steps: []api.Step{{Title: "c", ProgressCurrent: 0, ProgressTotal: 1}},
		},
		{
			ID: 4, Title: "Done step", Status: "active", Pinned: true,
			Steps: []api.Step{{Title: "d", Done: true, ProgressCurrent: 1, ProgressTotal: 1}},
		},
	}
	fav, urg, cat := SplitQuests(items, "", now)
	if len(fav) != 1 || fav[0].QuestID != 1 {
		t.Fatalf("favorites=%v", fav)
	}
	if len(urg) != 1 || urg[0].QuestID != 2 {
		t.Fatalf("urgent=%v", urg)
	}
	if len(cat) != 0 {
		t.Fatalf("category=%v", cat)
	}
}
