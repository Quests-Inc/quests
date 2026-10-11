package schedule_test

import (
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/schedule"
)

func TestParseWeekdaysPythonStyle(t *testing.T) {
	// Expose via due-today: Mon-only template on a Monday.
	monday := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) // Aug 10 2026 is Monday
	if monday.Weekday() != time.Monday {
		t.Fatalf("fixture weekday %v", monday.Weekday())
	}
	if !schedule.TemplateDueTodayForTest("weekly", "0", monday) {
		t.Fatal("Monday should match weekdays=0")
	}
	if schedule.TemplateDueTodayForTest("weekly", "1", monday) {
		t.Fatal("Monday should not match weekdays=1 (Tue)")
	}
}
