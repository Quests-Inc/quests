package schedule

import (
	"github.com/quests-inc/quests/go/internal/platform/events"
	"testing"
	"time"
)

// Persisted periods reach the scheduler's actual automatic emit gate; repeated
// ticks remain deduplicated per calendar day, even for a multi-day cycle.
func TestMaterializeCalendarPeriods(t *testing.T) {
	st := openEmitSpecDB(t)
	for _, body := range []map[string]any{
		{"title": "monthly", "freq": "monthly", "monthdays": "15,30", "timezone": "UTC"},
		{"title": "2/2", "freq": "cycle", "cycle_length": 4, "cycle_start": "2026-02-15", "cycle_days": "1,2", "timezone": "UTC"},
	} {
		if _, err := st.Automation.CreateTemplate(bg(), body); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		date string
		want int
	}{{"2026-02-14", 0}, {"2026-02-15", 2}, {"2026-02-15", 0}, {"2026-02-16", 1}, {"2026-02-17", 0}, {"2026-02-18", 0}, {"2026-02-19", 1}, {"2026-02-28", 1}, {"2026-03-02", 0}} {
		now, _ := time.Parse("2006-01-02", tc.date)
		ids, err := MaterializeDue(bg(), st, events.New(), now, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != tc.want {
			t.Errorf("%s: made %d want %d", tc.date, len(ids), tc.want)
		}
	}
}
